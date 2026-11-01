package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gocraft/auth"
	"gocraft/proto"
	"gocraft/world"
)

// The multiplayer client: protocol 47, offline mode. A reader goroutine turns packets into
// closures; the game thread runs them between frames, so the world, the meshes and the
// player are only ever touched from one thread.

type netClient struct {
	threshold atomic.Int32 // the compression threshold; -1 while off
	dim       atomic.Int32 // the dimension the server last put the player in: Chunk Data has no sky light outside the Overworld
	w         io.Writer    // the connection, or its encrypting end in online mode
	conn      net.Conn
	name      string
	inbox     chan func(*game) // what is cheap: players, chat, health. Applied before each tick, in order
	heavy     chan heavyMsg    // what is not: columns and block changes, applied within a budget each frame
	epoch     atomic.Uint32    // counts Join Game and Respawn: columns of an old world must not reach a new one
	out       chan []byte
	done      chan struct{}
	encrypted bool // the connection is encrypted: the server checks accounts, and the list shows heads
}

// heavyMsg is a packet that changes the world: a column, a block. It carries the epoch it was read in.
type heavyMsg struct {
	f     func(*game)
	epoch uint32
}

// heavyPacket says which clientbound packets go to the budgeted queue: Chunk Data, Multi Block Change,
// Block Change, Map Chunk Bulk.
func heavyPacket(id int32) bool { return id == 0x21 || id == 0x22 || id == 0x23 || id == 0x26 }

// remotePlayer is another player as the server tells of them.
type remotePlayer struct {
	eid                          int32
	uuid, name                   string
	pos, prev, target            vec3
	yaw, pitch                   float32
	ls, lsa                      float32
	swing                        int
	hurt                         int  // ticks of red tint after a blow
	dead                         bool // lying dead until the server shows them again
	sneaking                     bool
	sprinting, using             bool // metadata flags: running, and eating/blocking/drawing a bow
	useTicks                     int  // how long the item has been in use: a bow shows how far it is drawn
	invisible, onFire            bool
	tyaw, tpitch                 float32 // where the server turned them; they get there over the same ticks as the position
	inc                          int
	headYaw, thead               float32
	prevYaw, prevHead, prevPitch float32 // the angles a tick ago: what is drawn is between them and the current ones
	body, prevBody               float32 // the body's yaw, which follows the head with a lag (renderYawOffset)
	held                         slot
	heldX                        itemExtra
	armor                        [4]slot // boots, leggings, chestplate, helmet
	armorX                       [4]itemExtra
	deathTime                    int
}

// netState is what the game keeps for a connection.
type netState struct {
	maps           map[int32]*mapData // what Map Data said about each map
	c              *netClient
	received       map[world.ChunkPos]bool
	remote         map[int32]*remotePlayer
	ents           map[int32]*netEntity  // mobs the server spawned, for their names
	transaction0   int16                 // the player's window's transaction number, which counts up with each click
	border         *worldBorder          // the world border, once a server has sent one
	lastCorrection int                   // the tick of the last Player Position And Look, for the log
	lastKnock      int                   // and of the last knockback
	hitPending     bool                  // the player hit another one and the tick has not yet paid for it (pvp.go)
	items          map[int32]*itemEntity // dropped items the server spawned; they are in g.items too
	pickups        []*pickupFX           // items just collected, flying to who took them
	extra          [45]itemExtra         // names and lore of the items in the player's window, by slot
	sb             *scoreboard           // objectives, scores and teams the server keeps
	heavyQ         []heavyMsg            // columns and blocks read but not yet applied
	pumped         time.Time             // when the cheap packets were last applied
	epochApplied   uint32                // the newest world the game has begun
	score          int32                 // the score the death screen shows: the player's metadata 18
	names          map[string]string     // tab list: uuid -> name
	tab            map[string]*tabEntry  // the player list, by uuid
	header, footer []styledRun           // the list's header and footer
	eid            int32
	addr           string     // the address the player typed, which names the server in the options
	packAsk        *packOffer // a resource pack the player has not answered yet
	// what the server last heard from us
	sentPos         vec3
	sentYaw, sentPt float32
	sentGround      bool
	sinceFull       int
	sentHeld        int
	sentHotbar      [9]slot
	sentArmor       [4]slot
	hotbarKnown     bool
	sentSprint      bool
	sentBlocking    bool
	sentUsing       bool // a bow or a meal is in use as far as the server knows
	joined          bool // the Join Game arrived, so the game mode is known
	sentSneak       bool
	terrainDone     bool // the first position from the server came: the "downloading terrain" wait is over
	waitTicks       int
	gocraft         bool // the server said it is a gocraft server, which may be told the inventory
	syncVia         int  // how the inventory last went out: 0 not yet, 1 creative packet, 2 gocraft channel
}

// dial connects, logs in with the given name and returns once the server accepts. Play packets
// are read from then on.
// loginAuth is what a connection may use to prove who the player is to an online-mode server.
type loginAuth struct {
	Client  *auth.Client
	Session *auth.Session
}

// dial connects, logs in and, when the server wants the player authenticated (online mode), encrypts
// the connection after telling the session server this player is joining. la is nil for offline play.
func dial(addr, name string, la *loginAuth) (*netClient, error) {
	host, port := addr, 25565
	if h, p, err := net.SplitHostPort(addr); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("bad port %q", p)
		}
		host, port = h, n
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 8*time.Second)
	if err != nil {
		return nil, err
	}
	hs := newPacket(0)
	hs.VarInt(47)
	hs.String(host)
	hs.UShort(uint16(port))
	hs.VarInt(2)
	login := newPacket(0)
	login.String(name)
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(append(hs.Frame(), login.Frame()...)); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReaderSize(conn, 1<<16)
	var out io.Writer = conn
	encrypted := false
	threshold := -1
login:
	for {
		id, r, err := proto.ReadPacketZ(br, threshold)
		if err != nil {
			conn.Close()
			return nil, err
		}
		switch id {
		case 0x02: // login success
			break login
		case 0x03: // set compression: every packet from here on is framed with a data length
			threshold = int(r.VarInt())
			if r.Err() != nil {
				conn.Close()
				return nil, r.Err()
			}
		case 0x00: // disconnect
			conn.Close()
			return nil, fmt.Errorf("refused: %s", chatText(r.String(32767)))
		case 0x01: // encryption request: the server is in online mode
			rd, wr, err := answerEncryption(conn, r, la)
			if err != nil {
				conn.Close()
				return nil, err
			}
			br, out = bufio.NewReaderSize(rd, 1<<16), wr
			encrypted = true
		default:
			conn.Close()
			return nil, fmt.Errorf("unexpected login packet 0x%02x", id)
		}
	}
	conn.SetDeadline(time.Time{})
	c := &netClient{conn: conn, w: out, name: name, encrypted: encrypted, inbox: make(chan func(*game), 8192), heavy: make(chan heavyMsg, 8192), out: make(chan []byte, 1024), done: make(chan struct{})}
	c.threshold.Store(int32(threshold))
	go c.writeLoop()
	go c.readLoop(br)
	return c, nil
}

// writeLoop sends what the game queued. Everything that is waiting goes out in one write: a tick that sends a
// movement, a held-slot change and an action is one segment, not three, and an encrypted connection runs its
// cipher once.
func (c *netClient) writeLoop() {
	var buf []byte
	for {
		select {
		case f := <-c.out:
			buf = append(buf[:0], f...)
		gather:
			for len(buf) < 32<<10 {
				select {
				case f := <-c.out:
					buf = append(buf, f...)
				default:
					break gather
				}
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := c.w.Write(buf); err != nil {
				c.conn.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// send queues a packet; when the queue is full (the server is not reading) it is dropped.
func (c *netClient) send(w *proto.Writer) {
	select {
	case c.out <- w.FrameZ(int(c.threshold.Load())):
	default:
	}
}

func (c *netClient) close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.conn.Close()
}

func (c *netClient) readLoop(br *bufio.Reader) {
	for {
		id, r, err := proto.ReadPacketZ(br, int(c.threshold.Load()))
		if err != nil {
			select {
			case <-c.done:
			default:
				c.inbox <- func(g *game) { g.netLost(err.Error()) }
			}
			return
		}
		if id == 0x00 { // keep-alive: answered right here, whatever the game is doing
			k := newPacket(0x00)
			k.VarInt(r.VarInt())
			c.send(k)
			continue
		}
		if id == 0x46 { // set compression, in play
			c.threshold.Store(r.VarInt())
			continue
		}
		f := parsePacketIn(id, r, &c.dim)
		switch {
		case f == nil:
		case heavyPacket(id):
			c.heavy <- heavyMsg{f, c.epoch.Load()}
		case id == 0x01 || id == 0x07: // a new world begins: what is heavy from now on belongs to it
			ep := c.epoch.Add(1)
			c.inbox <- func(g *game) {
				g.net.epochApplied = ep
				f(g)
			}
		default:
			c.inbox <- f
		}
	}
}

// chatText flattens a chat component's JSON into plain text.
func chatText(js string) string {
	var v any
	if json.Unmarshal([]byte(js), &v) != nil {
		return js
	}
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			b.WriteString(t)
		case map[string]any:
			if s, ok := t["text"].(string); ok {
				b.WriteString(s)
			}
			if tr, ok := t["translate"].(string); ok {
				b.WriteString(tr)
			}
			if e, ok := t["extra"].([]any); ok {
				for _, x := range e {
					walk(x)
				}
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(v)
	return b.String()
}

func uuidString(b []byte) string { return hex.EncodeToString(b) }

// decodeColumn reads one column's sections from data (blocks, then block light, then sky light when
// the dimension has it, then the biomes of a ground-up column) and says how many bytes it used.
func decodeColumn(mask uint16, data []byte, skylight, ground bool) (*world.Chunk, int, bool) {
	sections := 0
	for s := 0; s < 16; s++ {
		if mask&(1<<s) != 0 {
			sections++
		}
	}
	need := sections*8192 + sections*2048
	if skylight {
		need += sections * 2048
	}
	if ground {
		need += 256
	}
	if len(data) < need {
		return nil, 0, false
	}
	c := &world.Chunk{Light: make([]uint8, 65536)}
	for i := range c.Light {
		c.Light[i] = 0xF0 // sections not sent: open sky
	}
	off := 0
	idx := make([]int, 0, sections)
	for s := 0; s < 16; s++ {
		if mask&(1<<s) == 0 {
			continue
		}
		idx = append(idx, s)
		for i := 0; i < 4096; i++ {
			c.Blocks[s*4096+i] = uint16(data[off]) | uint16(data[off+1])<<8
			off += 2
		}
	}
	for _, s := range idx { // block light
		for i := 0; i < 4096; i++ {
			c.Light[s*4096+i] = data[off+i>>1] >> (uint(i&1) * 4) & 15
		}
		off += 2048
	}
	if skylight {
		for _, s := range idx {
			for i := 0; i < 4096; i++ {
				v := data[off+i>>1] >> (uint(i&1) * 4) & 15
				c.Light[s*4096+i] = c.Light[s*4096+i]&15 | v<<4
			}
			off += 2048
		}
	} else {
		for _, s := range idx { // no sky in this dimension
			for i := 0; i < 4096; i++ {
				c.Light[s*4096+i] &= 15
			}
		}
	}
	if ground {
		c.Biomes = append([]uint8(nil), data[off:off+256]...)
		off += 256
	}
	return c, off, true
}

type columnUpdate struct {
	cp world.ChunkPos
	c  *world.Chunk // nil: the column goes
}

// parseChunk decodes a Chunk Data packet; a ground-up column with no sections is an unload. Only the
// Overworld sends sky light (1.8's Chunk Data has no flag for it: the dimension says).
func parseChunk(r *proto.Reader, skylight bool) (columnUpdate, bool) {
	cp := world.ChunkPos{X: r.Int(), Z: r.Int()}
	ground := r.Bool()
	mask := r.UShort()
	data := r.Bytes(int(r.VarInt()))
	if r.Err() != nil {
		return columnUpdate{}, false
	}
	if mask == 0 && ground {
		return columnUpdate{cp: cp}, true
	}
	c, _, ok := decodeColumn(mask, data, skylight, ground)
	return columnUpdate{cp, c}, ok
}

// parseBulk decodes Map Chunk Bulk, which real 1.8 servers use to send several columns at once.
func parseBulk(r *proto.Reader) ([]columnUpdate, bool) {
	sky := r.Bool()
	n := int(r.VarInt())
	if r.Err() != nil || n < 0 || n > 4096 {
		return nil, false
	}
	type meta struct {
		cp   world.ChunkPos
		mask uint16
	}
	metas := make([]meta, 0, n)
	for i := 0; i < n && r.Err() == nil; i++ {
		metas = append(metas, meta{world.ChunkPos{X: r.Int(), Z: r.Int()}, r.UShort()})
	}
	data := r.Bytes(r.Len())
	if r.Err() != nil {
		return nil, false
	}
	out := make([]columnUpdate, 0, n)
	for _, m := range metas {
		c, used, ok := decodeColumn(m.mask, data, sky, true)
		if !ok {
			return out, len(out) > 0
		}
		data = data[used:]
		out = append(out, columnUpdate{m.cp, c})
	}
	return out, true
}

// parsePacket turns one clientbound play packet into what the game does with it; nil to ignore it.
func parsePacket(id int32, r *proto.Reader) func(*game) { return parsePacketIn(id, r, nil) }

// parsePacketIn is parsePacket for a connection that tracks its dimension in dim (nil: always the Overworld).
func parsePacketIn(id int32, r *proto.Reader, dim *atomic.Int32) func(*game) {
	switch id {
	case 0x01: // join game
		eid := r.Int()
		mode := r.Byte()
		d := int32(int8(r.Byte()))
		if r.Err() != nil {
			return nil
		}
		if dim != nil {
			dim.Store(d)
		}
		return func(g *game) {
			g.net.eid = eid
			g.netGameMode(mode)
			g.net.joined = true
			g.netDimension(int(d))
			g.sendClientInfo()
		}
	case 0x07: // respawn: a death, or the proxy moving the player to another server
		d := int32(r.Int())
		r.Byte() // difficulty
		mode := r.Byte()
		if r.Err() != nil {
			return nil
		}
		if dim != nil {
			dim.Store(d)
		}
		return func(g *game) { g.netRespawn(int(d), mode) }
	case 0x39: // player abilities: the server can allow flying and set the player flying (a client check does)
		flags := r.Byte()
		fly, walk := r.Float(), r.Float()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			g.flyCap, g.walkCap = fly, walk
			g.allowFly = flags&4 != 0
			g.abilFlags = flags & (1 | 8)
			g.flying = flags&2 != 0
			if g.flying {
				g.onGround = false // in the air, as the game has it: a flight that has just begun is not ended by landing
			}
		}
	case 0x3B: // scoreboard objective
		name := r.String(256)
		mode := r.Byte()
		display, hearts := "", false
		if mode != 1 {
			display = r.String(256)
			hearts = r.String(64) == "hearts" // type: integer or hearts
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.net.board().objective(name, mode, display, hearts) }
	case 0x3C: // update score
		entry := r.String(256)
		action := r.Byte()
		objective := r.String(256)
		var value int32
		if action != 1 {
			value = r.VarInt()
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.net.board().setScore(entry, objective, value, action == 1) }
	case 0x3D: // display scoreboard
		slot := int(r.Byte())
		name := r.String(256)
		if r.Err() != nil || slot < 0 || slot > 18 {
			return nil
		}
		return func(g *game) { g.net.board().slots[slot] = name }
	case 0x3E: // teams
		name := r.String(256)
		mode := r.Byte()
		t := scoreTeam{colour: -1}
		var players []string
		if mode == 0 || mode == 2 {
			t.display = r.String(256)
			t.prefix = r.String(256)
			t.suffix = r.String(256)
			t.flags = r.Byte()
			t.tagVisibility = r.String(256)
			t.colour = int8(r.Byte())
		}
		if mode == 0 || mode == 3 || mode == 4 {
			for i, k := 0, int(r.VarInt()); i < k && i < 4096 && r.Err() == nil; i++ {
				players = append(players, r.String(256))
			}
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.net.board().teamUpdate(name, mode, t, players) }
	case 0x1F: // set experience
		bar := r.Float()
		level := int(r.VarInt())
		r.VarInt() // total
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.xpBar, g.xpLevel = bar, level }
	case 0x20: // entity properties: the player's maximum health
		eid := r.VarInt()
		count := int(r.Int())
		var maxHP float32
		found := false
		for i := 0; i < count && i < 64 && r.Err() == nil; i++ {
			key := r.String(256)
			base := r.Double()
			var adds, mulBase, mulTotal = 0.0, 0.0, 1.0
			for m, nm := 0, int(r.VarInt()); m < nm && m < 256 && r.Err() == nil; m++ {
				r.Bytes(16) // the modifier's uuid
				amount := r.Double()
				switch r.Byte() {
				case 0:
					adds += amount
				case 1:
					mulBase += amount
				case 2:
					mulTotal *= 1 + amount
				}
			}
			if key == "generic.maxHealth" {
				v := base + adds
				v = (v + v*mulBase) * mulTotal
				maxHP, found = float32(v), true
			}
		}
		if r.Err() != nil || !found {
			return nil
		}
		return func(g *game) {
			if eid == g.net.eid {
				g.maxHP = maxHP
			}
		}
	case 0x1D: // entity effect
		eid := r.VarInt()
		id := int(r.Byte())
		amp := int(r.Byte())
		dur := int(r.VarInt())
		r.Bool() // hide particles
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if eid == g.net.eid {
				if g.effects == nil {
					g.effects = map[int]potionEffect{}
				}
				g.effects[id] = potionEffect{amp, dur}
			}
		}
	case 0x1E: // remove entity effect
		eid := r.VarInt()
		id := int(r.Byte())
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if eid == g.net.eid {
				delete(g.effects, id)
			}
		}
	case 0x29: // sound effect: what the server makes others hear (blows, bows, steps)
		name := r.String(256)
		x, y, z := float64(r.Int())/8, float64(r.Int())/8, float64(r.Int())/8
		vol := r.Float()
		pitch := float32(r.Byte()) / 63
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if g.snd != nil {
				g.sound(name, vec3{x, y, z}, vol, pitch)
			}
		}
	case 0x2B: // change game state
		reason := r.Byte()
		val := r.Float()
		if r.Err() != nil || reason != 3 {
			return nil
		}
		return func(g *game) { g.netGameMode(byte(int(val))) }
	case 0x3F: // plugin message: a server's brand
		channel := r.String(20)
		if channel != "MC|Brand" || r.Err() != nil {
			return nil
		}
		brand := r.String(64)
		if r.Err() != nil || brand != "gocraft" {
			return nil
		}
		return func(g *game) { g.net.gocraft = true }
	case 0x02: // chat
		runs := parseChat(r.String(32767))
		pos := r.Byte()
		if r.Err() != nil {
			return nil
		}
		if pos == 2 { // the action bar: over the hotbar for three seconds
			return func(g *game) { g.titles.action, g.titles.actionTicks = runs, 60 }
		}
		return func(g *game) { g.addChatRuns(runs) }
	case 0x45: // title: a big message in the middle of the screen, a subtitle under it
		action := r.VarInt()
		var text []styledRun
		var in, stay, out int
		switch action {
		case 0, 1:
			text = parseChat(r.String(32767))
		case 2:
			in, stay, out = int(r.Int()), int(r.Int()), int(r.Int())
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netTitle(action, text, in, stay, out) }
	case 0x03: // time
		r.Long()
		t := r.Long()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.worldTime = int64(math.Abs(float64(t))) }
	case 0x06: // health
		hp := r.Float()
		food := r.VarInt()
		sat := r.Float()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			g.health, g.food, g.saturation = hp, int(food), sat
			if hp <= 0 && g.thirdParty() && g.screen != screenDead { // another server waits for the player to ask to respawn
				g.closeServerWindow(true)
				g.openScreen(screenDead)
			}
		}
	case 0x08: // position
		x, y, z := r.Double(), r.Double(), r.Double()
		yaw, pitch := r.Float(), r.Float()
		flags := r.Byte()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			p := g.feet
			g.logCorrection(vec3{x, y, z}, flags)
			// NetHandlerPlayClient.handlePlayerPosLook: an axis the server gives relative is added to where the player
			// is and keeps its motion; one it gives absolute is where the player goes and loses its motion.
			rel := func(bit byte, v *float64, now float64, axis int) {
				if flags&bit != 0 {
					*v += now
				} else {
					g.vel[axis] = 0
				}
			}
			rel(1, &x, p[0], 0)
			rel(2, &y, p[1], 1)
			rel(4, &z, p[2], 2)
			g.snapView(p, vec3{x, y, z})
			g.feet = vec3{x, y, z}
			g.prevFeet = g.feet
			// The angles may be relative too, added to where the player looks: 0x08 is the yaw (EnumFlags.Y_ROT, the
			// turn about the y axis) and 0x10 the pitch (X_ROT).
			if flags&8 != 0 {
				g.yaw += yaw * math.Pi / 180
			} else {
				g.yaw = yaw * math.Pi / 180
			}
			if flags&16 != 0 {
				g.pitch += pitch * math.Pi / 180
			} else {
				g.pitch = pitch * math.Pi / 180
			}
			g.fall = 0
			// handlePlayerPosLook answers at once with where the player now is, on the ground or not.
			w := newPacket(0x06)
			w.Double(float64(g.feet[0]))
			w.Double(float64(g.feet[1]))
			w.Double(float64(g.feet[2]))
			w.Float(g.yawDeg())
			w.Float(g.pitch * 180 / math.Pi)
			w.Bool(false)
			g.net.c.send(w)
			// The answer does not count as the last position reported: as in the game, the next tick sends the
			// position again when it differs from the one before the teleport (a server's client check sees it).
			g.net.terrainDone = true
		}
	case 0x0C: // spawn player
		eid := r.VarInt()
		uuid := uuidString(r.Bytes(16))
		x, y, z := float32(r.Int())/32, float32(r.Int())/32, float32(r.Int())/32
		yaw, pitch := float32(int8(r.Byte()))*360/256, float32(int8(r.Byte()))*360/256
		item := r.Short()
		meta, _ := readMeta(r)
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			p := vec3{float64(x), float64(y), float64(z)}
			rp := &remotePlayer{eid: eid, uuid: uuid, name: g.net.names[uuid], pos: p, prev: p, target: p, yaw: yaw, pitch: pitch, tyaw: yaw, tpitch: pitch, headYaw: yaw, thead: yaw, body: yaw, prevBody: yaw, prevYaw: yaw, prevHead: yaw, prevPitch: pitch}
			if item > 0 {
				rp.held = slot{ID: uint16(item), N: 1}
			}
			rp.applyMeta(meta)
			g.net.remote[eid] = rp
		}
	case 0x04: // entity equipment: what a player holds and wears
		eid := r.VarInt()
		sl := int(r.Short())
		it, ex, ok := readSlotX(r)
		if r.Err() != nil || !ok {
			return nil
		}
		return func(g *game) {
			if p := g.net.remote[eid]; p != nil && sl >= 0 && sl <= 4 {
				if sl == 0 {
					p.held, p.heldX = it, ex
				} else {
					p.armor[sl-1], p.armorX[sl-1] = it, ex
				}
			} else if e := g.net.entity(eid); e != nil && e.mob != nil && sl >= 0 && sl <= 4 {
				if sl == 0 {
					e.mob.m.held = it
				} else {
					e.mob.m.armor[sl-1], e.mob.m.armorX[sl-1] = it, ex
				}
			} else if e := g.net.entity(eid); e != nil && e.stand != nil && sl >= 0 && sl <= 4 {
				if sl == 0 {
					e.stand.held, e.stand.heldX = it, ex
				} else {
					e.stand.armor[sl-1], e.stand.armorX[sl-1] = it, ex
				}
				if sl == 4 { // a skull on the head
					e.stand.head = headSkull{}
					if it.ID == idSkullItem {
						e.stand.head = headSkull{on: true, typ: int(it.Dmg), skin: skullItemSkin(ex.nbt)}
						g.wantSkinURL(e.stand.head.skin)
					}
				}
			}
		}
	case 0x12: // entity velocity
		eid := r.VarInt()
		vx, vy, vz := r.Short(), r.Short(), r.Short()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if eid == g.net.eid {
				g.netVelocity(vx, vy, vz)
			} else if e := g.net.entity(eid); e != nil {
				e.vel, e.stuck = vec3{float64(vx) / 8000, float64(vy) / 8000, float64(vz) / 8000}, false
			} else if it := g.net.netItem(eid); it != nil {
				it.vel = vec3{float64(vx) / 8000, float64(vy) / 8000, float64(vz) / 8000}
			}
		}
	case 0x1A: // entity status
		eid := r.Int()
		status := r.Byte()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netStatus(eid, status) }
	case 0x13: // destroy entities
		n := int(r.VarInt())
		ids := make([]int32, 0, n)
		for i := 0; i < n && r.Err() == nil; i++ {
			ids = append(ids, r.VarInt())
		}
		return func(g *game) {
			for _, e := range ids {
				delete(g.net.remote, e)
				delete(g.net.ents, e)
				g.net.removeNetItem(e)
			}
		}
	case 0x18: // teleport
		eid := r.VarInt()
		x, y, z := float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32 // doubles, as the game reads them
		yaw, pitch := float32(int8(r.Byte()))*360/256, float32(int8(r.Byte()))*360/256
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if p := g.net.remote[eid]; p != nil {
				p.moveTo(vec3{x, y, z}, yaw, pitch)
			} else if e := g.net.entity(eid); e != nil {
				e.look(yaw, pitch)
				e.moveTo(vec3{x, y, z})
			} else if it := g.net.netItem(eid); it != nil {
				g.moveNetItem(it, vec3{x, y, z})
			}
		}
	case 0x0D: // collect item: someone took an item (or an arrow, or an orb) off the ground
		item, collector := r.VarInt(), r.VarInt()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netCollect(item, collector) }
	case 0x0B: // animation
		eid := r.VarInt()
		anim := r.Byte()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if anim == 4 || anim == 5 { // a critical hit, and an enchanted weapon's: sparks around whoever took it
				g.netCritSparks(eid, anim == 5)
				return
			}
			if p := g.net.remote[eid]; p != nil {
				switch anim {
				case 0:
					p.swing = 6
				case 1: // take damage
					p.hurt = 10
				}
			} else if e := g.net.entity(eid); e != nil && e.mob != nil && anim == 1 {
				e.mob.m.hurtTime = 10 // performHurtAnimation
			}
		}
	case 0x09: // held item change: the server picks the hotbar slot (a minigame handing out its kit)
		slot := int(int8(r.Byte()))
		if r.Err() != nil || slot < 0 || slot > 8 {
			return nil
		}
		return func(g *game) { g.sel = slot } // the next tick tells the server back, as syncCurrentPlayItem does
	case 0x44: // world border
		action := r.VarInt()
		var x, z, from, to float64
		var ms int64
		switch action {
		case 0:
			from = r.Double()
		case 1:
			from, to, ms = r.Double(), r.Double(), r.VarLong()
		case 2:
			x, z = r.Double(), r.Double()
		case 3:
			x, z, from, to, ms = r.Double(), r.Double(), r.Double(), r.Double(), r.VarLong()
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netWorldBorder(action, x, z, from, to, ms) }
	case 0x3A: // tab-complete: what the chat line could continue with
		n := int(r.VarInt())
		if r.Err() != nil || n < 0 || n > 4096 {
			return nil
		}
		answers := make([]string, 0, n)
		for i := 0; i < n && r.Err() == nil; i++ {
			answers = append(answers, r.String(32767))
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netTabComplete(answers) }
	case 0x24: // block action: a chest's lid, a note block's note
		x, y, z := proto.UnpackPos(r.Long())
		id, param := r.Byte(), r.Byte()
		blk := r.VarInt()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netBlockAction([3]int32{x, y, z}, id, param, blk) }
	case 0x1B: // attach entity: a leash (1) from a holder, or a rider on a mount (0, which is not drawn yet)
		eid, holder, leash := r.Int(), r.Int(), r.Byte()
		if r.Err() != nil || leash != 1 {
			return nil
		}
		return func(g *game) {
			if e := g.net.entity(eid); e != nil {
				if holder < 0 {
					holder = 0 // untied
				}
				e.leash = holder
			}
		}
	case 0x36: // open sign editor: the player is to write on this sign
		x, y, z := proto.UnpackPos(r.Long())
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.openSignEditor([3]int32{x, y, z}) }
	case 0x34: // map data: the colours of a map, whole or a rectangle of it
		id, scale, icons, cols, rows, x0, z0, data, ok := parseMap(r)
		if !ok {
			return nil
		}
		return func(g *game) { g.netMap(id, scale, icons, cols, rows, x0, z0, data) }
	case 0x35: // update block entity: a skull's type, turn and owner
		x, y, z := proto.UnpackPos(r.Long())
		action := r.Byte()
		nbt := append([]byte(nil), r.Rest()...)
		if r.Err() != nil || action != 4 && action != 1 {
			return nil
		}
		return func(g *game) { g.netBlockEntity([3]int32{x, y, z}, action, nbt) }
	case 0x33: // update sign: its four lines
		x, y, z := proto.UnpackPos(r.Long())
		var lines [4]string
		for i := range lines {
			lines[i] = r.String(32767)
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netUpdateSign([3]int32{x, y, z}, lines) }
	case 0x25: // block break animation: someone else's mining cracks a block
		eid := r.VarInt()
		x, y, z := proto.UnpackPos(r.Long())
		stage := int(int8(r.Byte()))
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.setDamage(eid, [3]int32{x, y, z}, stage) }
	case 0x2A: // particle
		id := r.Int()
		far := r.Bool()
		at := vec3{float64(r.Float()), float64(r.Float()), float64(r.Float())}
		off := vec3{float64(r.Float()), float64(r.Float()), float64(r.Float())}
		speed, count := r.Float(), r.Int()
		var args []int32
		for i := 0; i < particleArgs(id); i++ {
			args = append(args, r.VarInt())
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netParticles(id, far, at, off, speed, count, args) }
	case 0x27: // explosion
		at := vec3{float64(r.Float()), float64(r.Float()), float64(r.Float())}
		strength := r.Float()
		n := int(r.Int())
		bx, by, bz := int32(at[0]), int32(at[1]), int32(at[2]) // the records are relative to the truncated middle
		blocks := make([][3]int32, 0, min(max(n, 0), 4096))
		for i := 0; i < n && r.Err() == nil; i++ {
			dx, dy, dz := int32(int8(r.Byte())), int32(int8(r.Byte())), int32(int8(r.Byte()))
			if len(blocks) < cap(blocks) {
				blocks = append(blocks, [3]int32{bx + dx, by + dy, bz + dz})
			}
		}
		push := vec3{float64(r.Float()), float64(r.Float()), float64(r.Float())}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netExplosion(at, strength, blocks, push) }
	case 0x28: // effect: the sounds and particles of the world (a block broken, a door, a potion)
		effect := r.Int()
		x, y, z := proto.UnpackPos(r.Long())
		data := r.Int()
		r.Bool() // heard everywhere: the wither's and the dragon's, which gocraft does not play
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.netEffect(effect, x, y, z, data) }
	case 0x38: // player list: names, skins, ping, game modes
		return parseTabList(r)
	case 0x47: // player list header and footer
		header, footer := r.String(32767), r.String(32767)
		if r.Err() != nil {
			return nil
		}
		hr, fr := parseChat(header), parseChat(footer)
		return func(g *game) { g.net.header, g.net.footer = hr, fr }
	case 0x21: // chunk
		u, ok := parseChunk(r, dim == nil || dim.Load() == 0)
		if !ok {
			return nil
		}
		return func(g *game) { g.netChunk(u.cp, u.c) }
	case 0x26: // chunk bulk
		us, ok := parseBulk(r)
		if !ok {
			return nil
		}
		return func(g *game) {
			for _, u := range us {
				g.netChunk(u.cp, u.c)
			}
		}
	case 0x22: // several blocks of one column
		cx, cz := r.Int(), r.Int()
		n := int(r.VarInt())
		type rec struct {
			x, y, z int32
			st      uint16
		}
		recs := make([]rec, 0, n)
		for i := 0; i < n && r.Err() == nil; i++ {
			h := r.Byte()
			y := r.Byte()
			st := r.VarInt()
			recs = append(recs, rec{cx*16 + int32(h>>4), int32(y), cz*16 + int32(h&15), uint16(st)})
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			for _, b := range recs {
				g.changeBlock(b.x, b.y, b.z, b.st, false)
			}
		}
	case 0x15, 0x16, 0x17: // relative move, look, both
		eid := r.VarInt()
		var dx, dy, dz float32
		var yaw, pitch float32
		hasMove, hasLook := id != 0x16, id != 0x15
		if hasMove {
			dx, dy, dz = float32(int8(r.Byte()))/32, float32(int8(r.Byte()))/32, float32(int8(r.Byte()))/32
		}
		if hasLook {
			yaw, pitch = float32(int8(r.Byte()))*360/256, float32(int8(r.Byte()))*360/256
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if p := g.net.remote[eid]; p != nil {
				t, ty, tp := p.target, p.tyaw, p.tpitch
				if hasMove {
					t = t.add(vec3{float64(dx), float64(dy), float64(dz)})
				}
				if hasLook {
					ty, tp = yaw, pitch
				}
				p.moveTo(t, ty, tp)
			} else if e := g.net.entity(eid); e != nil && (hasMove || e.mob != nil || hasLook && e.stand != nil) {
				if e.mob != nil { // the turn goes with the move; a move alone keeps the way it faces now
					if !hasLook {
						yaw, pitch = e.mob.yaw, e.mob.pitch
					}
					e.mob.look(yaw, pitch)
				} else if hasLook {
					e.look(yaw, pitch)
				}
				e.moveTo(e.target.add(vec3{float64(dx), float64(dy), float64(dz)}))
			} else if it := g.net.netItem(eid); it != nil && hasMove {
				g.moveNetItem(it, it.srv.add(vec3{float64(dx), float64(dy), float64(dz)}))
			}
		}
	case 0x19: // head look
		eid := r.VarInt()
		head := float32(int8(r.Byte())) * 360 / 256
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if p := g.net.remote[eid]; p != nil {
				p.thead = head
				if p.inc == 0 {
					p.inc = 3
				}
			} else if e := g.net.entity(eid); e != nil && e.mob != nil {
				e.mob.head = head // EntityLivingBase.setRotationYawHead: at once
			}
		}
	case 0x23: // block change
		x, y, z := proto.UnpackPos(r.Long())
		st := r.VarInt()
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.changeBlock(x, y, z, uint16(st), false) }
	case 0x2F: // set slot
		win := int8(r.Byte())
		idx := int(r.Short())
		it, ex, ok := readSlotX(r)
		if r.Err() != nil || !ok {
			return nil
		}
		return func(g *game) { g.netSetSlot(win, idx, it, ex) }
	case 0x30: // window items
		win := int8(r.Byte())
		n := int(r.Short())
		if r.Err() != nil || n < 0 || n > 128 {
			return nil
		}
		items := make([]slot, 0, n)
		extras := make([]itemExtra, 0, n)
		for i := 0; i < n; i++ {
			it, ex, ok := readSlotX(r)
			if !ok {
				return nil
			}
			items, extras = append(items, it), append(extras, ex)
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			for i := range items {
				g.netSetSlot(win, i, items[i], extras[i])
			}
		}
	case 0x31: // window property: a furnace's burn and cook times, an enchanting table's levels
		win, prop, val := int8(r.Byte()), int(r.Short()), int(r.Short())
		if r.Err() != nil || prop < 0 || prop >= 16 {
			return nil
		}
		return func(g *game) {
			if g.swin != nil && g.swin.id == win {
				g.swin.props[prop] = val
			}
		}
	case 0x32: // confirm transaction: a refused one wants the same answer back, as the game gives it
		win, action, accepted := r.Byte(), r.Short(), r.Bool()
		if r.Err() != nil || accepted {
			return nil
		}
		return func(g *game) {
			w := newPacket(0x0F)
			w.Byte(win)
			w.UShort(uint16(action))
			w.Bool(true)
			g.net.c.send(w)
		}
	case 0x48: // resource pack offered: the player is asked, and it is downloaded and laid over their packs
		url := r.String(32767)
		hash := r.String(40)
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.offerServerPack(url, hash) }
	case 0x0F: // spawn mob: only kept for its name (armor stands carry a lobby's holograms)
		eid := r.VarInt()
		kind := r.Byte()
		x, y, z := float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32
		yaw, pitch, head := float32(int8(r.Byte()))*360/256, float32(int8(r.Byte()))*360/256, float32(int8(r.Byte()))*360/256
		r.Bytes(6) // velocity
		meta, _ := readMeta(r)
		if r.Err() != nil {
			return nil
		}
		e := &netEntity{eid: eid, kind: kind, pos: vec3{x, y, z}, prev: vec3{x, y, z}, target: vec3{x, y, z}}
		e.mob = newNetMob(kind, e.pos, yaw, pitch, head)
		if kind == mobArmorStand {
			e.stand = newStand(yaw)
		}
		e.apply(meta)
		return func(g *game) { g.net.addEntity(e) }
	case 0x0E: // spawn object: a fishing hook, an arrow, an item
		eid := r.VarInt()
		kind := r.Byte()
		x, y, z := float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32
		r.Bytes(2) // pitch, yaw
		data := r.Int()
		var vel vec3
		if data > 0 {
			vel = vec3{float64(r.Short()) / 8000, float64(r.Short()) / 8000, float64(r.Short()) / 8000}
		}
		if r.Err() != nil {
			return nil
		}
		if kind == objItem {
			return func(g *game) { g.netSpawnItem(eid, vec3{x, y, z}, vel, data > 0) }
		}
		if kind == objLeashKnot { // the block it is tied to; a knot sits in its middle
			at := vec3{math.Floor(x) + 0.5, math.Floor(y) + 0.5, math.Floor(z) + 0.5}
			e := &netEntity{eid: eid, obj: kind, pos: at, prev: at, target: at}
			return func(g *game) { g.net.addEntity(e) }
		}
		if kind == objItemFrame { // the packet carries the block it hangs in, and the facing as its data
			f := &frameState{hang: [3]int32{int32(math.Floor(x)), int32(math.Floor(y)), int32(math.Floor(z))}, facing: int(data) & 3}
			c := f.center()
			e := &netEntity{eid: eid, obj: kind, frame: f, pos: c, prev: c, target: c}
			return func(g *game) { g.net.addEntity(e) }
		}
		e := &netEntity{eid: eid, obj: kind, owner: data, pos: vec3{x, y, z}, prev: vec3{x, y, z}, target: vec3{x, y, z}, vel: vel}
		return func(g *game) { g.net.addEntity(e) }
	case 0x10: // spawn painting
		eid, title, at, facing, ok := parseSpawnPainting(r)
		if !ok {
			return nil
		}
		art, known := paintingArtByTitle(title)
		if !known { // the game builds the entity anyway, with an art it picked; one it has no title for is not drawn
			return nil
		}
		p := &paintingState{art: art, facing: facing, hang: at}
		c := p.center()
		e := &netEntity{eid: eid, painting: p, pos: c, prev: c, target: c}
		return func(g *game) { g.net.addEntity(e) }
	case 0x2C: // spawn global entity: a lightning bolt
		eid := r.VarInt()
		kind := r.Byte()
		x, y, z := float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32
		if r.Err() != nil || kind != 1 {
			return nil
		}
		e := &netEntity{eid: eid, obj: objLightning, bolt: newBolt(), pos: vec3{x, y, z}, prev: vec3{x, y, z}, target: vec3{x, y, z}}
		return func(g *game) { g.net.addEntity(e) }
	case 0x11: // spawn experience orb
		eid := r.VarInt()
		x, y, z := float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32
		count := int(r.Short())
		if r.Err() != nil {
			return nil
		}
		e := &netEntity{eid: eid, obj: objOrb, orb: count, pos: vec3{x, y, z}, prev: vec3{x, y, z}, target: vec3{x, y, z}}
		return func(g *game) { g.net.addEntity(e) }
	case 0x1C: // entity metadata
		eid := r.VarInt()
		meta, _ := readMeta(r)
		if r.Err() != nil || len(meta) == 0 {
			return nil
		}
		return func(g *game) {
			if eid == g.net.eid {
				if v, ok := meta[18].(int); ok {
					g.net.score = int32(v)
				}
				if v, ok := meta[17].(float32); ok { // golden hearts
					g.absorption = v
				}
				if v, ok := meta[1].(int); ok { // the air supply
					g.air = v
				}
			}
			if p := g.net.remote[eid]; p != nil {
				p.applyMeta(meta)
			} else if e := g.net.entity(eid); e != nil {
				e.apply(meta)
			} else if it := g.net.netItem(eid); it != nil {
				if s, ok := meta[10].(slot); ok { // what the item is
					it.stack = s
				}
			}
		}
	case 0x2D: // open window
		win := int8(r.Byte())
		kind := r.String(32)
		title := r.String(32767)
		n := int(r.Byte())
		if kind == "EntityHorse" {
			r.Int()
		}
		if r.Err() != nil {
			return nil
		}
		return func(g *game) { g.openServerWindow(win, kind, title, n) }
	case 0x2E: // close window
		win := int8(r.Byte())
		if r.Err() != nil {
			return nil
		}
		return func(g *game) {
			if g.swin != nil && g.swin.id == win {
				g.closeServerWindow(false)
			}
		}
	case 0x40: // disconnect
		reason := chatText(r.String(32767))
		return func(g *game) { g.netLost(reason) }
	}
	return nil
}

// ---- game side ----

// connect joins a server and starts playing in its world.
func (g *game) connect(addr, name string) error {
	la := g.onlineLogin() // nil unless the player signed in and chose to play with the account
	if la != nil {
		name = la.Session.Name // an online-mode server knows the player by this name
	}
	c, err := dial(addr, name, la)
	if err != nil {
		return err
	}
	g.closeWorld()
	g.swin = nil
	g.net = &netState{c: c, addr: addr, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}, sentHeld: -1}
	g.world = world.NewFrom(func(world.ChunkPos) *world.Chunk { return nil })
	g.world.SetLightRules(g.res.light)
	g.mobs, g.arrows, g.items = nil, nil, nil
	g.inv, g.armor, g.cursor = [36]slot{}, [4]slot{}, slot{} // a server says what the player carries
	g.health, g.food, g.saturation = maxHealth, maxFood, 5
	g.creative, g.flying, g.onGround, g.allowFly = true, false, false, false
	g.effects = nil
	g.walkCap, g.flyCap = 0, 0
	g.abilFlags = 0
	g.speedsSet, g.sprintTicks = false, 0
	g.pvp.use = itemUse{}
	g.titles = titleState{}
	g.maxHP, g.absorption, g.xpBar, g.xpLevel, g.air = 0, 0, 0, 0, 300
	g.pos = vec3{0.5, world.SpawnY + eyeHeight, 0.5}
	g.feet = g.pos.sub(vec3{0, eyeHeight, 0})
	g.prevFeet, g.spawn, g.vel = g.feet, g.feet, vec3{}
	g.yaw, g.pitch, g.sel = 0, 0, 0
	g.chat = nil
	g.inGame = true
	if g.win != nil && g.opt.Screenshot == "" {
		g.closeScreen()
	}
	logf("connected to %s as %s", addr, name)
	return nil
}

// clientBrand is what the client calls itself in MC|Brand: gocraft, unless GOCRAFT_BRAND says otherwise
// (some networks only let a "vanilla" client through their verification).
func clientBrand() string {
	if b := os.Getenv("GOCRAFT_BRAND"); b != "" {
		return b
	}
	return "gocraft"
}

// sendClientInfo is what every 1.8 client says right after joining: its brand and its settings.
// The brand is gocraft's own unless GOCRAFT_BRAND is set.
func (g *game) sendClientInfo() {
	set := newPacket(0x15)
	set.String("es_ES")
	set.Byte(byte(g.opts.RenderDistance))
	set.Byte(0)    // chat shown
	set.Bool(true) // chat colours
	set.Byte(0x7F) // every skin layer
	g.net.c.send(set)
	// handleJoinGame sends the settings first and the brand second.
	brand := newPacket(0x17)
	brand.String("MC|Brand")
	brand.String(clientBrand())
	g.net.c.send(brand)
}

// netDimension is the dimension the server says the player is in; only the sky and the fog look at it.
func (g *game) netDimension(d int) {
	if d != dimNether && d != dimEnd {
		d = dimOverworld
	}
	g.dimn.id = d
}

// netGameMode is what a game mode from the server does to the player (WorldSettings.GameType.
// configurePlayerCapabilities): creative may fly and takes no damage, survival and adventure stop flying,
// a spectator flies. Abilities from the server may change that afterwards.
func (g *game) netGameMode(mode byte) {
	switch mode & 7 { // 8 is hardcore
	case 1:
		g.creative, g.allowFly, g.abilFlags = true, true, 1|8
	case 3:
		g.creative, g.allowFly, g.flying, g.abilFlags = false, true, true, 1
	default:
		g.creative, g.allowFly, g.flying, g.abilFlags = false, false, false, 0
	}
}

// netRespawn handles a Respawn: the world the player was in is gone. A proxy sends one (twice, with a
// dummy dimension in between) when it moves the player to another server, and the player's own death
// is answered by one, so the columns, the other players and the entities all go, and the server then
// sends the new ones and a position.
func (g *game) netRespawn(d int, mode byte) {
	n := g.net
	n.transaction0 = 0                    // a new player has a new window
	n.border = nil                        // and a new world has the default border until the server says otherwise
	g.speedsSet, g.sprintTicks = false, 0 // a new player entity
	g.pvp.use = itemUse{}                 // with nothing in use
	for cp := range n.received {
		g.forgetMesh(cp)
	}
	n.received = map[world.ChunkPos]bool{}
	n.remote = map[int32]*remotePlayer{}
	n.ents, n.items, n.pickups = nil, nil, nil
	g.damage, g.signs, g.skulls, g.spawners = nil, nil, nil, nil
	g.closeServerWindow(false)
	g.world = world.NewFrom(func(world.ChunkPos) *world.Chunk { return nil })
	g.world.SetLightRules(g.res.light)
	g.mobs, g.arrows, g.items = nil, nil, nil
	g.netDimension(d)
	g.flying, g.vel, g.fall = false, vec3{}, 0
	g.netGameMode(mode)
	g.health, g.food = maxHealth, maxFood
	if g.screen == screenDead {
		g.closeScreen()
	}
	n.terrainDone = false
}

// netLost leaves the server, back to the title screen with the reason.
func (g *game) netLost(reason string) {
	if g.net == nil {
		return
	}
	g.net.c.close()
	g.net = nil
	g.closeWorld()
	g.world = world.New()
	g.worldList = nil
	g.screen = screenTitle
	g.netMsg = reason
	logf("left the server: %s", reason)
	if g.win != nil {
		g.openScreen(screenTitle)
	}
}

// netChunk installs a received column and has it and its neighbours meshed again.
func (g *game) netChunk(cp world.ChunkPos, c *world.Chunk) {
	if c == nil {
		g.world.Remove(cp)
		delete(g.net.received, cp)
		g.forgetMesh(cp)
		return
	}
	g.world.Put(cp, c)
	g.net.received[cp] = true
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			g.forgetMesh(world.ChunkPos{X: cp.X + dx, Z: cp.Z + dz})
		}
	}
}

// forgetMesh drops a column's mesh and bumps its version, so the streamer builds it again.
func (g *game) forgetMesh(cp world.ChunkPos) {
	if g.stream != nil {
		g.stream.version[cp]++
	}
	if m := g.meshes[cp]; m != nil {
		m.free()
		delete(g.meshes, cp)
	}
}

// pumpNet applies everything the network thread has queued: the cheap packets at once, the columns within
// their budget. The game loop uses pumpLight before each tick and pumpHeavy once a frame.
func (g *game) pumpNet() {
	g.pumpLight()
	g.pumpHeavy(4 * time.Millisecond)
}

// pumpLight applies the cheap packets, all of them and in order. Run right before a tick, it is what the game
// does too: other players' moves are taken in, and the same tick takes the first step towards them.
func (g *game) pumpLight() {
	if g.net != nil {
		g.net.pumped = time.Now()
	}
	for g.net != nil {
		select {
		case f := <-g.net.c.inbox:
			f(g)
		default:
			return
		}
	}
}

// pumpHeavy applies columns and blocks until the budget is spent. A column of a world the game has not begun
// yet (its Respawn has not been applied) waits; one of a world that is gone is dropped.
func (g *game) pumpHeavy(budget time.Duration) {
	n := g.net
	if n == nil {
		return
	}
	for { // take in what has arrived
		select {
		case m := <-n.c.heavy:
			n.heavyQ = append(n.heavyQ, m)
			continue
		default:
		}
		break
	}
	end := time.Now().Add(budget)
	done := 0
	for done < len(n.heavyQ) {
		m := n.heavyQ[done]
		if m.epoch > n.epochApplied {
			break
		}
		done++
		if m.epoch == n.epochApplied {
			m.f(g)
			if g.net == nil {
				return
			}
		}
		if time.Now().After(end) {
			break
		}
	}
	if done > 0 {
		n.heavyQ = append(n.heavyQ[:0], n.heavyQ[done:]...)
	}
}

// tickNet runs once a tick: other players move on, and what we did is told to the server.
func (g *game) tickNet() {
	n := g.net
	g.tickRemotes()
	g.syncHotbar() // PlayerControllerMP.updateController syncs the held slot at the start of the tick
	// EntityPlayerSP.onUpdateWalkingPlayer: the sprint and sneak changes go before the movement.
	g.sendActions()
	// 1.8 then sends exactly one movement packet every tick, the position when it moved more than
	// 0.03 blocks since the last one sent or when 20 ticks have passed since it (the count is read
	// before it is raised), the look when it changed, and just the ground flag otherwise.
	yaw, pitch := g.yawDeg(), g.pitch*180/math.Pi
	d := g.feet.sub(n.sentPos)
	moved := d.dot(d) > 9.0e-4 || n.sinceFull >= 20
	n.sinceFull++
	turned := yaw != n.sentYaw || pitch != n.sentPt
	var w *proto.Writer
	switch {
	case moved && turned:
		w = newPacket(0x06)
	case moved:
		w = newPacket(0x04)
	case turned:
		w = newPacket(0x05)
	default:
		w = newPacket(0x03)
	}
	if moved {
		w.Double(float64(g.feet[0]))
		w.Double(float64(g.feet[1]))
		w.Double(float64(g.feet[2]))
		n.sentPos, n.sinceFull = g.feet, 0
	}
	if turned {
		w.Float(yaw)
		w.Float(pitch)
		n.sentYaw, n.sentPt = yaw, pitch
	}
	w.Bool(g.onGround)
	n.c.send(w)
	n.sentGround = g.onGround
}

// tickRemotes moves the other players of the server one tick on.
func (g *game) tickRemotes() {
	n := g.net
	g.tickEnts()
	g.tickPickups()
	for _, p := range n.remote {
		p.prev = p.pos
		p.prevBody = p.body
		p.prevYaw, p.prevHead, p.prevPitch = p.yaw, p.headYaw, p.pitch
		if p.inc > 0 { // EntityOtherPlayerMP.onLivingUpdate: a third of the way each tick, then half, then there
			k := float64(p.inc)
			p.pos = p.pos.add(p.target.sub(p.pos).scale(1 / k))
			p.yaw += wrapDeg(p.tyaw-p.yaw) / float32(p.inc)
			p.headYaw += wrapDeg(p.thead-p.headYaw) / float32(p.inc)
			p.pitch += (p.tpitch - p.pitch) / float32(p.inc)
			p.inc--
		}
		d := float32(math.Hypot(float64(p.pos[0]-p.prev[0]), float64(p.pos[2]-p.prev[2])))
		p.lsa += (min(d*4, 1) - p.lsa) * 0.4
		p.ls += p.lsa
		p.body = stepBodyYaw(p.body, p.yaw, d > 0.0025)
		if p.using {
			p.useTicks++
		} else {
			p.useTicks = 0
		}
		if p.swing > 0 {
			p.swing--
		}
		if p.hurt > 0 {
			p.hurt--
		}
		if p.dead {
			p.deathTime++
		}
	}
}

// stepBodyYaw is the body turning after the head, as the player's own does: towards the view while moving,
// and never more than 75 degrees away from it.
func stepBodyYaw(body, yaw float32, moving bool) float32 {
	if moving {
		body = turnToward(body, yaw, 25)
	}
	if diff := float32(math.Mod(float64(yaw-body)+540, 360)) - 180; diff > 75 {
		body = yaw - 75
	} else if diff < -75 {
		body = yaw + 75
	}
	return body
}

// moveTo is the server putting a player somewhere, turned some way: they get there in three ticks.
func (p *remotePlayer) moveTo(t vec3, yaw, pitch float32) {
	p.target, p.tyaw, p.tpitch, p.inc = t, yaw, pitch, 3
}

// applyMeta takes the flags of a player's metadata: fire, sneaking, sprinting, using an item, invisible.
func (p *remotePlayer) applyMeta(m map[int]any) {
	if v, ok := m[0].(int); ok {
		p.onFire, p.sneaking, p.sprinting = v&1 != 0, v&2 != 0, v&8 != 0
		p.using, p.invisible = v&16 != 0, v&32 != 0
	}
}

// syncHotbar tells the server what the hotbar and the armour hold and which slot is in hand. A
// vanilla client only ever sends its inventory in creative mode (Creative Inventory Action), and a
// server may throw out one that does so otherwise, so: in creative that packet; in survival only a
// server that said it is gocraft's own is told, through the plugin channel gocraft|inv (it works
// out weapon damage and armour from it); any other server hears nothing of it.
func (g *game) syncHotbar() {
	n := g.net
	if !n.joined {
		return // the game mode is not known yet
	}
	via := 0
	switch {
	case g.creative && n.gocraft:
		via = 1
	case n.gocraft:
		via = 2
	}
	if via != n.syncVia { // the way changed (or it is the first time): everything goes again
		n.syncVia, n.hotbarKnown = via, false
	}
	if via != 0 {
		send := func(window int, s slot) {
			var w *proto.Writer
			if via == 1 {
				w = newPacket(0x10)
			} else {
				w = newPacket(0x17)
				w.String("gocraft|inv")
			}
			w.UShort(uint16(window))
			if s.empty() {
				w.UShort(0xFFFF)
				if via == 2 {
					w.Byte(0)
					w.UShort(0)
				}
			} else {
				w.UShort(s.ID)
				w.Byte(byte(s.N))
				w.UShort(s.Dmg)
				if via == 1 {
					w.Byte(0) // no NBT
				}
			}
			n.c.send(w)
		}
		for i := 0; i < 9; i++ {
			if s := g.inv[i]; !n.hotbarKnown || s != n.sentHotbar[i] {
				send(36+i, s)
				n.sentHotbar[i] = s
			}
		}
		for i := 0; i < 4; i++ { // helmet, chestplate, leggings, boots: window slots 5 to 8
			if s := g.armor[i]; !n.hotbarKnown || s != n.sentArmor[i] {
				send(5+i, s)
				n.sentArmor[i] = s
			}
		}
		n.hotbarKnown = true
	}
	g.syncHeld()
}

// sendDig tells the server a block was broken (creative breaks at once).
func (g *game) sendDig(x, y, z int32, face int) {
	if g.net == nil {
		return
	}
	w := newPacket(0x07)
	w.Byte(0)
	w.Long(packPos(x, y, z))
	w.Byte(byte(face))
	g.net.c.send(w)
}

// sendPlace tells the server a block was placed against the face of the block at x, y, z.
func (g *game) sendPlace(x, y, z int32, face int) {
	if g.net == nil {
		return
	}
	w := newPacket(0x08)
	w.Long(packPos(x, y, z))
	w.Byte(byte(face))
	w.UShort(0xFFFF)
	w.Byte(8)
	w.Byte(8)
	w.Byte(8)
	g.net.c.send(w)
}

// sendSwing tells the server the arm swung.
func (g *game) sendSwing() {
	if g.net != nil {
		g.net.c.send(newPacket(0x0A))
	}
}

// sendChat sends a chat line.
func (g *game) sendChat(s string) {
	if g.net == nil || s == "" {
		return
	}
	w := newPacket(0x01)
	w.String(s)
	g.net.c.send(w)
}

// leaveServer disconnects on purpose.
func (g *game) leaveServer() {
	if g.net == nil {
		return
	}
	g.net.c.close()
	g.net = nil
}

// short names for the protocol helpers the multiplayer code uses a lot
var (
	newPacket = proto.NewPacket
	packPos   = proto.PackPos
)

// terrainLoaded is EntityPlayerSP.onUpdate's first test: until the column under the player has
// arrived the game does not simulate the player or send a movement packet at all.
func (g *game) terrainLoaded() bool {
	return g.net.received[world.ChunkPos{X: int32(math.Floor(float64(g.feet[0]))) >> 4, Z: int32(math.Floor(float64(g.feet[2]))) >> 4}]
}

// tickWaiting is a tick spent waiting for the terrain. Until the first position arrives the game
// shows its "Downloading terrain" screen, which sends a keep-alive with key 0 every 20 ticks.
func (g *game) tickWaiting() {
	n := g.net
	g.tickRemotes()
	if !n.terrainDone {
		if n.waitTicks++; n.waitTicks%20 == 0 {
			w := newPacket(0x00)
			w.VarInt(0)
			n.c.send(w)
		}
	}
}

// readSlotData reads an item stack as the protocol sends it: id (-1 for none), count, damage and a
// named binary tag (enchantments, names) that is skipped.
func readSlotData(r *proto.Reader) (slot, bool) {
	s, _, ok := readSlotX(r)
	return s, ok
}

// skipNBT steps over one tag of type t (and its name when it is a root tag).
func skipNBT(r *proto.Reader, t byte, named bool) bool {
	if named {
		r.Bytes(int(r.UShort()))
	}
	return skipPayload(r, t, 0)
}

func skipPayload(r *proto.Reader, t byte, depth int) bool {
	if depth > 32 || r.Err() != nil {
		return false
	}
	switch t {
	case 1:
		r.Bytes(1)
	case 2:
		r.Bytes(2)
	case 3, 5:
		r.Bytes(4)
	case 4, 6:
		r.Bytes(8)
	case 7:
		r.Bytes(int(r.Int()))
	case 8:
		r.Bytes(int(r.UShort()))
	case 9:
		et, n := r.Byte(), int(r.Int())
		for i := 0; i < n && r.Err() == nil; i++ {
			if !skipPayload(r, et, depth+1) {
				return false
			}
		}
	case 10:
		for r.Err() == nil {
			ct := r.Byte()
			if ct == 0 {
				return true
			}
			r.Bytes(int(r.UShort()))
			if !skipPayload(r, ct, depth+1) {
				return false
			}
		}
	case 11:
		r.Bytes(4 * int(r.Int()))
	default:
		return false
	}
	return r.Err() == nil
}

// setWindowSlot puts a stack in the player's own inventory window: 5 to 8 are the armour, 9 to 35
// the main inventory and 36 to 44 the hotbar.
func (g *game) setWindowSlot(i int, s slot) {
	if i == 36+g.sel { // a new stack in hand: the game's item in use is no longer the one held (itemuse.go)
		g.pvp.use.replaced = true
	}
	switch {
	case i >= 36 && i <= 44:
		g.inv[i-36] = s
	case i >= 9 && i <= 35:
		g.inv[i] = s
	case i >= 5 && i <= 8:
		g.armor[i-5] = s
	}
}

// answerEncryption is the client's half of the online-mode handshake. It tells the session server this
// player is joining (the hash of the server ID, a fresh shared secret and the server's public key),
// answers the server with the secret and its verify token encrypted to its public key, and returns the
// connection with encryption on in both directions.
func answerEncryption(conn net.Conn, r *proto.Reader, la *loginAuth) (io.Reader, io.Writer, error) {
	serverID := r.String(20)
	pub := append([]byte(nil), r.Bytes(int(r.VarInt()))...)
	token := append([]byte(nil), r.Bytes(int(r.VarInt()))...)
	if r.Err() != nil {
		return nil, nil, fmt.Errorf("bad encryption request: %w", r.Err())
	}
	if la == nil || la.Session == nil {
		return nil, nil, fmt.Errorf("this server needs a Minecraft account (online mode): sign in with Microsoft on the Multiplayer screen")
	}
	key, err := x509.ParsePKIXPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("the server's key is not valid: %w", err)
	}
	pk, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("the server's key is not RSA")
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := la.Client.Join(ctx, la.Session, auth.ServerHash(serverID, secret, pub)); err != nil {
		return nil, nil, err
	}
	encSecret, err := rsa.EncryptPKCS1v15(rand.Reader, pk, secret)
	if err != nil {
		return nil, nil, err
	}
	encToken, err := rsa.EncryptPKCS1v15(rand.Reader, pk, token)
	if err != nil {
		return nil, nil, err
	}
	w := newPacket(0x01)
	w.VarInt(int32(len(encSecret)))
	w.Bytes(encSecret)
	w.VarInt(int32(len(encToken)))
	w.Bytes(encToken)
	if _, err := conn.Write(w.Frame()); err != nil { // the last packet in the clear
		return nil, nil, err
	}
	return proto.Encrypted(conn, secret)
}
