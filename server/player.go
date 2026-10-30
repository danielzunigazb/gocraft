package server

import (
	"bufio"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"gocraft/proto"
	"gocraft/world"
)

type slot struct {
	id     int16 // item id; blocks use their block id; -1 is empty
	count  int8
	damage int16 // block metadata for a block item
}

type Player struct {
	nc   net.Conn
	name string
	uuid string
	eid  int32

	wmu sync.Mutex // serialises writes: the play loop, the keep-alive and other players' broadcasts

	mu         sync.Mutex
	joined     bool
	x, y, z    float64
	yaw, pitch float32
	chunk      world.ChunkPos
	loaded     map[world.ChunkPos]bool
	inv        [46]slot // window slots; the armour is 5..8 and the hotbar 36..44
	held       int

	// combat (see combat.go)
	health     float32
	hurtResist int     // ticks of invulnerability left
	lastDamage float32 // the damage that started them
	dead       int     // ticks until the respawn; 0 when alive
	regen      int
	sprinting  bool
	blocking   bool
	onGround   bool
	fall       float32
	vel        [3]float64 // knockback not yet sent
}

func newPlayer(nc net.Conn, eid int32, name string) *Player {
	p := &Player{nc: nc, name: name, uuid: offlineUUID(name), eid: eid, loaded: map[world.ChunkPos]bool{}, health: maxHealth}
	for i := range p.inv {
		p.inv[i].id = -1
	}
	return p
}

func (p *Player) inWorld() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.joined
}

func (p *Player) send(frame []byte) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	p.nc.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := p.nc.Write(frame); err != nil {
		p.nc.Close() // the read loop sees it and the player leaves
	}
}

// play sends the world to a player who just logged in, then serves their packets until they leave.
func (p *Player) play(s *Server, br *bufio.Reader) {
	join := proto.NewPacket(0x01)
	join.Int(p.eid)
	if s.cfg.Survival {
		join.Byte(0)
	} else {
		join.Byte(1) // creative
	}
	join.Byte(0) // Overworld
	join.Byte(0) // peaceful
	join.Byte(byte(min(s.cfg.MaxPlayers, 255)))
	join.String("flat")
	join.Bool(false)
	p.send(join.Frame())

	sx, sy, sz, scp := s.spawnPoint()
	spawn := proto.NewPacket(0x05)
	spawn.Long(proto.PackPos(int32(sx), int32(sy), int32(sz)))
	p.send(spawn.Frame())

	brand := proto.NewPacket(0x3F) // plugin message: who this server is, as every 1.8 server says
	brand.String("MC|Brand")
	brand.String("gocraft")
	p.send(brand.Frame())

	tm := proto.NewPacket(0x03)
	tm.Long(0)
	tm.Long(-6000) // negative: the clock is stopped, at noon
	p.send(tm.Frame())

	abil := proto.NewPacket(0x39)
	if s.cfg.Survival {
		abil.Byte(0)
	} else {
		abil.Byte(0x01 | 0x04 | 0x08) // invulnerable, may fly, creative
	}
	abil.Float(0.05)
	abil.Float(0.1)
	p.send(abil.Frame())

	p.mu.Lock()
	p.x, p.y, p.z = sx, sy, sz
	p.chunk = scp
	p.mu.Unlock()
	p.streamChunks(s)
	p.teleport(sx, sy, sz, 0, 0)

	p.mu.Lock()
	p.joined = true
	p.mu.Unlock()
	s.introduce(p)
	if s.cfg.Survival {
		p.send(p.healthPacket())
	}
	s.Broadcast(chatPacket(p.name + " joined the game"))

	done := make(chan struct{})
	defer close(done)
	go p.keepAlive(done)

	for {
		p.nc.SetReadDeadline(time.Now().Add(60 * time.Second))
		id, r, err := proto.ReadPacket(br)
		if err != nil {
			return
		}
		p.handle(s, id, r)
		if r.Err() != nil {
			log.Printf("%s: bad packet 0x%02x: %v", p.name, id, r.Err())
			return
		}
	}
}

func (p *Player) keepAlive(done chan struct{}) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	var n int32
	for {
		select {
		case <-done:
			return
		case <-t.C:
			n++
			k := proto.NewPacket(0x00)
			k.VarInt(n)
			p.send(k.Frame())
		}
	}
}

func (p *Player) teleport(x, y, z float64, yaw, pitch float32) {
	t := proto.NewPacket(0x08)
	t.Double(x)
	t.Double(y)
	t.Double(z)
	t.Float(yaw)
	t.Float(pitch)
	t.Byte(0) // all absolute
	p.send(t.Frame())
}

// streamChunks sends the columns within the view distance that the client lacks and drops the far ones.
func (p *Player) streamChunks(s *Server) {
	p.mu.Lock()
	centre := p.chunk
	p.mu.Unlock()
	d := int32(s.cfg.ViewDistance)

	for dx := -d; dx <= d; dx++ {
		for dz := -d; dz <= d; dz++ {
			cp := world.ChunkPos{X: centre.X + dx, Z: centre.Z + dz}
			p.mu.Lock()
			have := p.loaded[cp]
			p.loaded[cp] = true
			p.mu.Unlock()
			if !have {
				p.send(ChunkPacket(s.world, cp))
			}
		}
	}
	p.mu.Lock()
	var drop []world.ChunkPos
	for cp := range p.loaded {
		if abs32(cp.X-centre.X) > d+1 || abs32(cp.Z-centre.Z) > d+1 {
			drop = append(drop, cp)
			delete(p.loaded, cp)
		}
	}
	p.mu.Unlock()
	for _, cp := range drop {
		p.send(UnloadPacket(cp))
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (p *Player) hasChunk(x, z int32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loaded[world.ChunkPos{X: x >> 4, Z: z >> 4}]
}

// setBlock changes the world and tells every player who has that column.
func (s *Server) setBlock(x, y, z int32, state uint16) {
	if !s.world.SetBlock(x, y, z, state) {
		return
	}
	frame := BlockChangePacket(x, y, z, state)
	s.mu.Lock()
	ps := make([]*Player, 0, len(s.players))
	for _, o := range s.players {
		ps = append(ps, o)
	}
	s.mu.Unlock()
	for _, o := range ps {
		if o.hasChunk(x, z) {
			o.send(frame)
		}
	}
}

func readSlot(r *proto.Reader) slot {
	id := r.Short()
	if id < 0 {
		return slot{id: -1}
	}
	// The NBT tag that may follow is not parsed: nothing here needs it.
	return slot{id: id, count: int8(r.Byte()), damage: r.Short()}
}

// face offsets by face index: -Y, +Y, -Z, +Z, -X, +X.
var faceOffset = [6][3]int32{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}

func (p *Player) handle(s *Server, id int32, r *proto.Reader) {
	switch id {
	case 0x01: // chat
		msg := r.String(100)
		if r.Err() != nil {
			return
		}
		if strings.HasPrefix(msg, "/") {
			p.send(chatPacket("Unknown command: " + msg))
			return
		}
		s.Broadcast(chatPacket("<" + p.name + "> " + msg))

	case 0x04, 0x05, 0x06: // position, look, both
		var x, y, z float64
		var yaw, pitch float32
		if id != 0x05 {
			x, y, z = r.Double(), r.Double(), r.Double()
		}
		if id != 0x04 {
			yaw, pitch = r.Float(), r.Float()
		}
		onGround := r.Bool()
		if r.Err() != nil {
			return
		}
		p.mu.Lock()
		oldY := p.y
		if id != 0x05 && finite(x, y, z) {
			p.x, p.y, p.z = x, y, z
		}
		if id != 0x04 {
			p.yaw, p.pitch = yaw, pitch
		}
		cp := world.ChunkPos{X: int32(math.Floor(p.x)) >> 4, Z: int32(math.Floor(p.z)) >> 4}
		moved := cp != p.chunk
		p.chunk = cp
		px, py, pz := p.x, p.y, p.z
		p.mu.Unlock()
		p.landing(s, oldY, px, py, pz, onGround)
		if moved {
			p.streamChunks(s)
		}
		s.sendOthers(p, p.teleportPacket(onGround), p.headPacket())

	case 0x03: // only whether the player is on the ground
		onGround := r.Bool()
		if r.Err() != nil {
			return
		}
		p.mu.Lock()
		x, y, z := p.x, p.y, p.z
		p.mu.Unlock()
		p.landing(s, y, x, y, z, onGround)

	case 0x02: // use entity: attacking another player
		target := r.VarInt()
		kind := r.VarInt()
		if r.Err() != nil || kind != 1 {
			return
		}
		s.mu.Lock()
		v := s.players[target]
		s.mu.Unlock()
		if v != nil && v.inWorld() {
			s.attack(p, v)
		}

	case 0x0B: // entity action: sprinting
		r.VarInt()
		action := r.VarInt()
		if r.Err() != nil {
			return
		}
		p.mu.Lock()
		switch action {
		case 3:
			p.sprinting = true
		case 4:
			p.sprinting = false
		}
		p.mu.Unlock()

	case 0x0A: // swing the arm
		s.sendOthers(p, p.swingPacket())

	case 0x07: // dig; creative breaks at once, survival when the client says it finished
		status := r.Byte()
		x, y, z := proto.UnpackPos(r.Long())
		r.Byte()
		if r.Err() != nil {
			return
		}
		if status == 5 { // release the use item: the sword stops blocking
			p.mu.Lock()
			p.blocking = false
			p.mu.Unlock()
			return
		}
		if s.cfg.Survival {
			p.mu.Lock()
			far := reachSq(p.x, p.y+eyeHeight, p.z, float64(x)+0.5, float64(y), float64(z)+0.5) > 8*8
			p.mu.Unlock()
			if status != 2 || far {
				return
			}
		} else if status != 0 {
			return
		}
		if s.world.Block(x, y, z)>>4 == 7 { // bedrock stays
			p.send(BlockChangePacket(x, y, z, s.world.Block(x, y, z)))
			return
		}
		s.setBlock(x, y, z, 0)

	case 0x08: // block placement
		x, y, z := proto.UnpackPos(r.Long())
		face := r.Byte()
		if r.Err() != nil {
			return
		}
		if face == 255 { // using the held item: a sword starts blocking
			p.mu.Lock()
			p.blocking = isSword(p.inv[36+p.held].id)
			p.mu.Unlock()
			return
		}
		if face > 5 {
			return
		}
		// The held stack and cursor that follow are not read: the server's own inventory says what is held.
		if c := s.world.Block(x, y, z) >> 4; !(c == 0 || (c >= 8 && c <= 11) || c == 31 || c == 32 || c == 106 || c == 51) {
			o := faceOffset[face] // the block clicked is not replaceable: the new one goes beside it
			x, y, z = x+o[0], y+o[1], z+o[2]
		}
		p.mu.Lock()
		it := p.inv[36+p.held]
		p.mu.Unlock()
		cur := s.world.Block(x, y, z) >> 4
		replaceable := cur == 0 || (cur >= 8 && cur <= 11)
		if !replaceable || it.id < 1 || it.id > 255 || !world.InRange(y) {
			p.send(BlockChangePacket(x, y, z, s.world.Block(x, y, z))) // undo what the client predicted
			return
		}
		s.setBlock(x, y, z, uint16(it.id)<<4|uint16(it.damage&15))

	case 0x09: // held item
		n := int(r.Short())
		if r.Err() == nil && n >= 0 && n < 9 {
			p.mu.Lock()
			p.held = n
			p.blocking = false // changing slot ends the use
			p.mu.Unlock()
		}

	case 0x10: // creative inventory: only a creative player may set slots this way
		n := int(r.Short())
		it := readSlot(r)
		if r.Err() == nil && !s.cfg.Survival {
			p.setSlot(n, it)
		}

	case 0x17: // plugin message
		channel := r.String(20)
		if channel != invChannel || r.Err() != nil {
			return
		}
		// What gocraft's own client says it carries, since the server does not run the inventory
		// itself: window slot, item (-1 none), count, damage. Only where combat needs it, in survival.
		n := int(r.UShort())
		it := slot{id: r.Short()}
		it.count, it.damage = int8(r.Byte()), r.Short()
		if r.Err() == nil && s.cfg.Survival {
			p.setSlot(n, it)
		}
	}
}

// invChannel is the plugin channel gocraft clients use to tell a gocraft server their inventory.
const invChannel = "gocraft|inv"

func (p *Player) setSlot(n int, it slot) {
	if n < 0 || n >= len(p.inv) {
		return
	}
	p.mu.Lock()
	p.inv[n] = it
	if !isSword(p.inv[36+p.held].id) {
		p.blocking = false
	}
	p.mu.Unlock()
}

func finite(v ...float64) bool {
	for _, f := range v {
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 3e7 {
			return false
		}
	}
	return true
}

// landing tracks the fall from a reported position and pays for it when the player lands.
func (p *Player) landing(s *Server, oldY, x, y, z float64, onGround bool) {
	inWater := false
	if c := s.world.Block(int32(math.Floor(x)), int32(math.Floor(y)), int32(math.Floor(z))) >> 4; c >= 8 && c <= 11 {
		inWater = true
	}
	p.mu.Lock()
	dmg := p.trackFall(oldY, onGround, inWater)
	p.mu.Unlock()
	if dmg > 0 {
		s.hurt(p, dmg, nil, true, "hit the ground too hard", [3]float64{})
	}
}
