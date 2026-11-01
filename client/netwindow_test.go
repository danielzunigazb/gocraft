package client

import (
	"bufio"
	"bytes"
	"math"
	"testing"

	"gocraft/proto"
	"gocraft/world"
)

func nbtString(w *proto.Writer, name, val string) {
	w.Byte(8)
	w.UShort(uint16(len(name)))
	w.Bytes([]byte(name))
	w.UShort(uint16(len(val)))
	w.Bytes([]byte(val))
}

// namedItem writes a stack with a display name and lore, as a lobby menu has them.
func namedItem(w *proto.Writer, id uint16, n byte, name string, lore ...string) []byte {
	w.UShort(id)
	w.Byte(n)
	w.UShort(0)
	start := len(w.B)
	w.Byte(10) // root compound, no name
	w.UShort(0)
	w.Byte(10) // display
	w.UShort(7)
	w.Bytes([]byte("display"))
	nbtString(w, "Name", name)
	w.Byte(9) // Lore: list of strings
	w.UShort(4)
	w.Bytes([]byte("Lore"))
	w.Byte(8)
	w.Int(int32(len(lore)))
	for _, l := range lore {
		w.UShort(uint16(len(l)))
		w.Bytes([]byte(l))
	}
	w.Byte(0) // end display
	w.Byte(0) // end root
	return append([]byte(nil), w.B[start:]...)
}

func windowGame(t *testing.T) (*game, *netClient) {
	g := testGame(t)
	c := &netClient{out: make(chan []byte, 16)}
	c.threshold.Store(-1)
	g.net = &netState{c: c, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	return g, c
}

func openChest(t *testing.T, g *game, id byte, title string, slots byte) {
	w := newPacket(0x2D)
	w.Byte(id)
	w.String("minecraft:chest")
	w.String(title)
	w.Byte(slots)
	decodeFrame(t, w, nil)(g)
}

func TestServerWindowOpensWithItemsAndNames(t *testing.T) {
	g, _ := windowGame(t)
	openChest(t, g, 3, `{"text":"§6Selector de juegos"}`, 27)
	if g.swin == nil || g.screen != screenServerWin || g.swin.n != 27 || plainRuns(g.swin.title) != "Selector de juegos" {
		t.Fatalf("window %+v screen %v", g.swin, g.screen)
	}
	// items: slot 13 is a named compass, slot 27 (the first of the player's own) a stone
	w := newPacket(0x30)
	w.Byte(3)
	w.UShort(27 + 36)
	for i := 0; i < 27+36; i++ {
		switch i {
		case 13:
			namedItem(w, 345, 1, "§a§lSkyWars", "§7Click para jugar", "§e12 jugando")
		case 27:
			w.UShort(1)
			w.Byte(64)
			w.UShort(0)
			w.Byte(0)
		case 27 + 27: // the first hotbar slot
			w.UShort(276)
			w.Byte(1)
			w.UShort(0)
			w.Byte(0)
		default:
			w.UShort(0xFFFF)
		}
	}
	decodeFrame(t, w, nil)(g)
	it, ex := g.windowItem(13)
	if it.ID != 345 || it.N != 1 || plainRuns(ex.name) != "SkyWars" || ex.name[0].st.col != chatColours[10] || !ex.name[0].st.bold {
		t.Fatalf("slot 13: %+v %+v", it, ex)
	}
	if len(ex.lore) != 2 || plainRuns(ex.lore[1]) != "12 jugando" || ex.lore[1][0].st.col != chatColours[14] {
		t.Fatalf("lore %+v", ex.lore)
	}
	if len(ex.nbt) == 0 || ex.nbt[0] != 10 {
		t.Fatalf("the tag was not kept: %v", ex.nbt)
	}
	if g.inv[9].ID != 1 || g.inv[9].N != 64 {
		t.Fatalf("the first slot of the player's own part went to %v, want the main inventory's first", g.inv[9])
	}
	if g.inv[0].ID != 276 {
		t.Fatalf("the hotbar's first slot is %v", g.inv[0])
	}
}

func TestServerWindowClickSendsTheItemAsTheServerGaveIt(t *testing.T) {
	g, c := windowGame(t)
	openChest(t, g, 5, `{"text":"Menu"}`, 9)
	w := newPacket(0x2F)
	w.Byte(5)
	w.UShort(4)
	tag := namedItem(w, 399, 1, "§bVolver", "x")
	decodeFrame(t, w, nil)(g)
	g.sendWindowClick(4, 1, 0)
	select {
	case f := <-c.out:
		id, r, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(f)))
		if err != nil || id != 0x0E {
			t.Fatalf("id 0x%02x err %v", id, err)
		}
		if win, slot, button, action, mode := r.Byte(), r.Short(), r.Byte(), r.Short(), r.Byte(); win != 5 || slot != 4 || button != 1 || action != 1 || mode != 0 {
			t.Fatalf("click %d %d %d %d %d", win, slot, button, action, mode)
		}
		if item := r.Short(); item != 399 || r.Byte() != 1 || r.Short() != 0 {
			t.Fatalf("clicked item %d", item)
		}
		if got := r.Bytes(r.Len()); !bytes.Equal(got, tag) {
			t.Fatalf("the tag went back different:\n got %v\nwant %v", got, tag)
		}
	default:
		t.Fatal("no click was sent")
	}
	g.sendWindowClick(0, 0, 1)
	<-c.out
	if g.swin.action != 2 {
		t.Fatalf("the next action number is %d, want 2", g.swin.action)
	}
}

func TestServerWindowCursorAndClose(t *testing.T) {
	g, c := windowGame(t)
	openChest(t, g, 2, `{"translate":"container.chest"}`, 27)
	if plainRuns(g.swin.title) != "Chest" {
		t.Fatalf("title %q", plainRuns(g.swin.title))
	}
	w := newPacket(0x2F)
	w.Byte(0xFF) // window -1
	w.UShort(0xFFFF)
	w.UShort(264)
	w.Byte(3)
	w.UShort(0)
	w.Byte(0)
	decodeFrame(t, w, nil)(g)
	if g.swin.cursor.ID != 264 || g.swin.cursor.N != 3 {
		t.Fatalf("cursor %v", g.swin.cursor)
	}
	cl := newPacket(0x2E)
	cl.Byte(2)
	decodeFrame(t, cl, nil)(g)
	if g.swin != nil || g.screen == screenServerWin {
		t.Fatal("the server closed the window and it stayed")
	}
	if len(c.out) != 0 {
		t.Fatal("closing because the server did must not answer with a close")
	}
	openChest(t, g, 4, `{"text":"x"}`, 27)
	g.closeServerWindow(true)
	f := <-c.out
	if id, r, _ := proto.ReadPacket(bufio.NewReader(bytes.NewReader(f))); id != 0x0D || r.Byte() != 4 {
		t.Fatalf("the player's close sent id 0x%02x", id)
	}
}

func TestServerWindowLayouts(t *testing.T) {
	for _, c := range []struct {
		kind    string
		n       int
		h, last int
	}{{"minecraft:chest", 27, 114 + 3*18, 27 + 36}, {"minecraft:chest", 54, 114 + 6*18, 54 + 36}, {"minecraft:hopper", 5, 133, 5 + 36},
		{"minecraft:dispenser", 9, 166, 9 + 36}, {"minecraft:crafting_table", 10, 166, 10 + 36}, {"minecraft:furnace", 3, 166, 3 + 36}} {
		w := &serverWindow{kind: c.kind, n: c.n}
		slots, h := w.layout()
		if h != c.h || len(slots) != c.last {
			t.Errorf("%s/%d: height %d slots %d, want %d %d", c.kind, c.n, h, len(slots), c.h, c.last)
		}
		for i, s := range slots {
			if s.idx != i {
				t.Fatalf("%s: slot %d has index %d", c.kind, i, s.idx)
			}
		}
	}
}

func readUse(t *testing.T, c *netClient) (pos int64, face byte, item int16, r *proto.Reader) {
	t.Helper()
	select {
	case f := <-c.out:
		id, rd, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(f)))
		if err != nil || id != 0x08 {
			t.Fatalf("sent id 0x%02x err %v", id, err)
		}
		pos, face, item = rd.Long(), rd.Byte(), rd.Short()
		return pos, face, item, rd
	default:
		t.Fatal("nothing was sent")
	}
	return
}

// A compass in the hand and a click in the air is what opens a lobby's game selector.
func TestRightClickInTheAirSendsTheHeldItem(t *testing.T) {
	g, c := windowGame(t)
	g.pos = vec3{0.5, 30, 0.5}
	g.yaw, g.pitch = 0, 0
	g.inv[0] = slot{ID: 345, N: 1}
	g.sel = 0
	g.useBlock()
	pos, face, item, _ := readUse(t, c)
	x, y, z := proto.UnpackPos(pos)
	if x != -1 || y != -1 || z != -1 || face != 255 || item != 345 {
		t.Fatalf("air click: %d %d %d face %d item %d", x, y, z, face, item)
	}
	g.inv[0] = slot{}
	g.useBlock()
	if len(c.out) != 0 {
		t.Fatal("an empty hand sent a click")
	}
}

// Clicking a block sends it to the server and changes nothing here.
func TestRightClickOnABlockOnlyAsksTheServer(t *testing.T) {
	g, c := windowGame(t)
	g.inv[0] = slot{ID: 1, N: 5}
	g.sel = 0
	g.pos = vec3{0.5, 5.4, 0.5}
	g.yaw = 0
	for _, pitch := range []float32{1.4, -1.4} {
		g.pitch = pitch
		if _, ok := g.target(); ok {
			break
		}
	}
	if _, ok := g.target(); !ok {
		t.Skip("no block in reach")
	}
	before := g.world.Block(0, 4, 0)
	g.useBlock()
	pos, face, item, _ := readUse(t, c)
	if x, _, _ := proto.UnpackPos(pos); x < -1 || item != 1 || face > 5 {
		t.Fatalf("block click: pos %d face %d item %d", pos, face, item)
	}
	if g.world.Block(0, 4, 0) != before || g.inv[0].N != 5 {
		t.Fatal("the click placed a block or used up the stack here instead of leaving it to the server")
	}
}

// An NPC in front of the player gets the interact click.
func TestRightClickOnAnEntity(t *testing.T) {
	g, c := windowGame(t)
	g.pos = vec3{0.5, 30, 0.5}
	g.yaw, g.pitch = 0, 0
	d := look(g.yaw, g.pitch)
	g.net.addEntity(&netEntity{eid: 77, kind: mobArmorStand, pos: vec3{g.pos[0] + d[0]*2, g.pos[1] - 1, g.pos[2] + d[2]*2}})
	g.useBlock()
	ps := drain(t, c)
	if len(ps) != 2 || ps[0].id != 0x02 || ps[1].id != 0x02 {
		t.Fatalf("sent %d packets: an armor stand is offered the click (interact at, then interact) and keeps it", len(ps))
	}
	if eid, kind := ps[0].r.VarInt(), ps[0].r.VarInt(); eid != 77 || kind != 2 {
		t.Fatalf("first: entity %d action %d, want interact at (2)", eid, kind)
	}
	if eid, kind := ps[1].r.VarInt(), ps[1].r.VarInt(); eid != 77 || kind != 0 {
		t.Fatalf("second: entity %d action %d, want interact (0)", eid, kind)
	}
}

// With an item that is not a block in the hand and a rival in front, the game offers the click to the rival and
// then uses the item as in the air: a rod is cast at them, a sword blocks. A villager keeps the click for itself.
func TestItemIsUsedWhenPointingAtAPlayer(t *testing.T) {
	for _, c := range []struct {
		name string
		id   uint16
	}{{"fishing rod", 346}, {"sword", 267}} {
		g, ch := windowGame(t)
		g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
		g.pos = vec3{0.5, 5.62, 0.5}
		g.feet = vec3{0.5, 4, 0.5}
		g.yaw, g.pitch = 0, 0
		g.inv[0], g.sel = slot{ID: c.id, N: 1}, 0
		g.net.remote[9] = &remotePlayer{eid: 9, name: "Rival", pos: vec3{0.5, 4, 2.2}, prev: vec3{0.5, 4, 2.2}, target: vec3{0.5, 4, 2.2}}
		g.useBlock()
		var ids []int32
		var place *sent
		for _, p := range drain(t, ch) {
			p := p
			ids = append(ids, p.id)
			if p.id == 0x08 {
				place = &p
			}
		}
		if len(ids) < 3 || ids[0] != 0x02 || ids[1] != 0x02 || ids[2] != 0x08 {
			t.Fatalf("%s: sent %v, want interact at, interact, and the item used (0x08)", c.name, ids)
		}
		if pos := place.r.Long(); pos != packPos(-1, -1, -1) || place.r.Byte() != 255 || place.r.Short() != int16(c.id) {
			t.Fatalf("%s: the use was not an item click in the air", c.name)
		}
		if c.id == 346 && ids[len(ids)-1] != 0x0A { // a rod swings the arm as it casts
			t.Fatalf("%s: no swing after the cast: %v", c.name, ids)
		}
	}

	g, ch := windowGame(t)
	g.pos = vec3{0.5, 5.62, 0.5}
	g.yaw, g.pitch = 0, 0
	g.inv[0], g.sel = slot{ID: 346, N: 1}, 0
	g.net.addEntity(&netEntity{eid: 5, kind: 120, pos: vec3{0.5, 4, 2.2}})
	g.useBlock()
	if ps := drain(t, ch); len(ps) != 2 {
		t.Fatalf("a villager: %d packets, want just the interaction: it trades and the rod stays", len(ps))
	}
}

func abilitiesPacket(flags byte) *proto.Writer {
	w := newPacket(0x39)
	w.Byte(flags)
	w.Float(0.05)
	w.Float(0.1)
	return w
}

func positionPacket(x, y, z float64) *proto.Writer {
	w := newPacket(0x08)
	w.Double(x)
	w.Double(y)
	w.Double(z)
	w.Float(0)
	w.Float(0)
	w.Byte(0)
	return w
}

// universocraft's client check, as a vanilla 1.8.9 client was seen to answer it: the player falls onto a
// platform of snow layers; the server then sets it flying and moves it up (it must hover); then ends
// the flight and moves it again (it must fall like vanilla). Each phase is judged by where the feet are.
func TestServerFlightAndFallCheck(t *testing.T) {
	g, _ := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true // the player waits for its column, as in the game
	for x := int32(6); x <= 10; x++ {
		for z := int32(6); z <= 10; z++ {
			g.world.SetBlock(x, 4, z, 78<<4|2) // three snow layers: stops the feet at y=4.25
		}
	}
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	until := func(max int) int {
		for i := 1; i <= max; i++ {
			g.tick(input{})
			if g.onGround {
				return i
			}
		}
		return -1
	}

	apply(abilitiesPacket(0))
	apply(positionPacket(8, 8, 8))
	if n := until(60); n < 0 || !near(g.feet[1], 4.25, 1e-9) {
		t.Fatalf("phase 1: after %d ticks feet y %v onGround %v, want standing at 4.25", n, g.feet[1], g.onGround)
	}

	apply(abilitiesPacket(6)) // allowed to fly and flying
	apply(positionPacket(8, 9, 8))
	for i := 0; i < 8; i++ {
		g.tick(input{})
		if !near(g.feet[1], 9, 1e-9) || !g.flying {
			t.Fatalf("phase 2, tick %d: feet y %v flying %v: a flying player does not fall", i+1, g.feet[1], g.flying)
		}
	}

	apply(abilitiesPacket(0)) // the flight ends
	apply(positionPacket(8, 9, 8))
	if g.flying || g.allowFly {
		t.Fatal("abilities 0 left the player able to fly")
	}
	g.tick(input{})
	if !near(g.feet[1], 9, 1e-9) {
		t.Fatalf("phase 3, tick 1: feet y %v, want 9 (the move uses motion 0)", g.feet[1])
	}
	g.tick(input{})
	if !near(g.feet[1], 8.9216, 1e-4) {
		t.Fatalf("phase 3, tick 2: feet y %.6f, want 8.9216", g.feet[1])
	}
	if n := until(60); n < 0 || !near(g.feet[1], 4.25, 1e-9) {
		t.Fatalf("phase 3: after %d more ticks feet y %v onGround %v, want standing at 4.25", n, g.feet[1], g.onGround)
	}
}

// A server that allows flight lets the player toggle it; one that does not, does not.
func TestServerAllowsFlight(t *testing.T) {
	g, _ := windowGame(t)
	g.creative = false
	if g.canFly() {
		t.Fatal("survival can fly")
	}
	decodeFrame(t, abilitiesPacket(4), nil)(g)
	if !g.canFly() || g.flying {
		t.Fatalf("abilities 4: canFly %v flying %v", g.canFly(), g.flying)
	}
}

// On a server that is not gocraft's own, letting go of a drawn bow shoots nothing here: the server is told.
func TestBowReleaseIsLeftToTheServer(t *testing.T) {
	g, c, held := duel(t, 0)
	g.inv[0] = slot{ID: 261, N: 1}
	g.inv[1] = slot{ID: 262, N: 64}
	held["use"] = true
	g.pressButton(g.opts.Keys["use"]) // drawn by the click, as the game's sendUseItem puts it in use
	for i := 0; i < 20; i++ {
		g.tick(input{})
	}
	if g.bowTicks != 20 {
		t.Fatalf("bowTicks %d after 20 ticks drawn", g.bowTicks)
	}
	drain(t, c)
	held["use"] = false // let go: the next tick sends the release and the server shoots
	g.tick(input{})
	if len(g.arrows) != 0 {
		t.Fatal("an arrow was shot here on a server that shoots its own")
	}
	if g.bowTicks != 0 {
		t.Fatalf("bowTicks %d after the release", g.bowTicks)
	}
	if n, sent := releases(t, c); n != 1 {
		t.Fatalf("sent %#x, want one Release Use Item", sent)
	}
	g.tick(input{})
	if n, _ := releases(t, c); n != 0 {
		t.Fatal("the release is sent again every tick")
	}
	// and on gocraft's own server the arrow is the client's
	g2, _ := windowGame(t)
	g2.net.gocraft = true
	g2.inv[0], g2.inv[1] = slot{ID: 261, N: 1}, slot{ID: 262, N: 64}
	g2.sel = 0
	g2.pos = vec3{0.5, 5.6, 0.5}
	g2.bowTicks = 20
	g2.tickBow()
	if len(g2.arrows) != 1 {
		t.Fatalf("gocraft's own server: %d arrows", len(g2.arrows))
	}
}

func healthPacket(hp float32) *proto.Writer {
	w := newPacket(0x06)
	w.Float(hp)
	w.VarInt(20)
	w.Float(5)
	return w
}

// Dying on another server shows the death screen, Respawn asks the server, and the server's Respawn ends it.
func TestDeathScreenAndRespawnRequest(t *testing.T) {
	g, c := windowGame(t)
	openChest(t, g, 2, `{"text":"x"}`, 27)
	decodeFrame(t, healthPacket(0), nil)(g)
	if g.screen != screenDead || g.swin != nil {
		t.Fatalf("screen %v window %v: dying should close a menu and show the death screen", g.screen, g.swin)
	}
	for len(c.out) > 0 { // the close of the menu
		<-c.out
	}
	if in := g.input(); in != (input{}) {
		t.Fatalf("a dead player moved: %+v", in)
	}
	g.sendRespawn()
	f := <-c.out
	id, r, _ := proto.ReadPacket(bufio.NewReader(bytes.NewReader(f)))
	if id != 0x16 || r.VarInt() != 0 {
		t.Fatalf("sent id 0x%02x, want Client Status: perform respawn", id)
	}
	decodeFrame(t, respawn(0, 0), nil)(g)
	if g.screen == screenDead || g.health <= 0 {
		t.Fatalf("after the Respawn: screen %v health %v", g.screen, g.health)
	}
	// the score comes from the player's own metadata
	g.net.eid = 4
	md := newPacket(0x1C)
	md.VarInt(4)
	md.Byte(2<<5 | 18)
	md.Int(1234)
	md.Byte(0x7F)
	decodeFrame(t, md, nil)(g)
	if g.net.score != 1234 {
		t.Fatalf("score %d", g.net.score)
	}
	// gocraft's own server brings the player back by itself: no screen there
	g2, _ := windowGame(t)
	g2.net.gocraft = true
	decodeFrame(t, healthPacket(0), nil)(g2)
	if g2.screen == screenDead {
		t.Fatal("the death screen on gocraft's own server")
	}
}

func statusPacket(eid int32, st byte) *proto.Writer {
	w := newPacket(0x1A)
	w.Int(eid)
	w.Byte(st)
	return w
}

// A blow shakes the view as hurtCameraEffect has it: nothing at the instant of the blow, most at about
// two ticks on, then back to level; a dead player's view falls over.
func TestHurtShakesTheView(t *testing.T) {
	g, _ := windowGame(t)
	g.net.eid = 5
	decodeFrame(t, statusPacket(5, 2), nil)(g)
	if g.hurtTime != 10 {
		t.Fatalf("hurtTime %d", g.hurtTime)
	}
	if r := g.cameraRoll(0); r != 0 && math.Abs(float64(r)) > 1e-3 {
		t.Fatalf("roll at the instant of the blow %v, want 0", r)
	}
	g.hurtTime = 8
	r := g.cameraRoll(0)
	if math.Abs(float64(r)+14*math.Sin(0.8*0.8*0.8*0.8*math.Pi)) > 1e-3 || r > -13 {
		t.Fatalf("roll two ticks on %v, want about -13.5", r)
	}
	g.hurtTime = 0
	if r := g.cameraRoll(0); r != 0 {
		t.Fatalf("roll after the shake %v", r)
	}
	if up := g.rolledUp(vec3{0, 0, 1}); up != (vec3{0, 1, 0}) {
		t.Fatalf("a level view has up %v", up)
	}
	g.hurtTime = 8
	if up := g.rolledUp(vec3{0, 0, 1}); math.Abs(up[0]) < 0.1 || up[1] > 0.99 {
		t.Fatalf("a shaken view's up %v should lean", up)
	}
	// another player's blow does not shake ours
	g.hurtTime = 0
	g.net.remote[9] = &remotePlayer{eid: 9}
	decodeFrame(t, statusPacket(9, 2), nil)(g)
	if g.hurtTime != 0 || g.net.remote[9].hurt != 10 {
		t.Fatalf("hurtTime %d, their tint %d", g.hurtTime, g.net.remote[9].hurt)
	}
	g.health, g.deathTime = 0, 20
	if r := g.cameraRoll(0); math.Abs(float64(r)-(40-8000.0/220)) > 1e-3 {
		t.Fatalf("a dead player's view rolls %v", r)
	}
}

func TestSoundEffectPacketIsHarmlessWithoutAudio(t *testing.T) {
	g, _ := windowGame(t)
	w := newPacket(0x29)
	w.String("random.bow")
	w.Int(80)
	w.Int(512)
	w.Int(-8)
	w.Float(1)
	w.Byte(63)
	f := decodeFrame(t, w, nil)
	if f == nil {
		t.Fatal("a sound effect was not understood")
	}
	f(g) // no sound system in a test: nothing happens, and nothing crashes
}

func effectPacket(eid int32, id, amp byte, dur int32) *proto.Writer {
	w := newPacket(0x1D)
	w.VarInt(eid)
	w.Byte(id)
	w.Byte(amp)
	w.VarInt(dur)
	w.Bool(false)
	return w
}

// Speed multiplies the movement speed attribute, Slowness takes from it, Jump Boost lifts the jump: the
// server's effects on the player become the numbers movement uses.
func TestPotionEffectsChangeMovement(t *testing.T) {
	g, _ := windowGame(t)
	g.net.eid = 5
	if v := g.groundSpeed(); !near(float64(v), 0.1, 1e-7) {
		t.Fatalf("base speed %v", v)
	}
	g.sprinting = true
	if v := g.groundSpeed(); !near(float64(v), 0.13, 1e-7) {
		t.Fatalf("sprint speed %v", v)
	}
	g.sprinting = false
	decodeFrame(t, effectPacket(5, effSpeed, 1, 600), nil)(g) // Speed II
	if v := g.groundSpeed(); !near(float64(v), 0.14, 1e-7) {
		t.Fatalf("Speed II %v, want 0.14", v)
	}
	g.sprinting = true
	if v := g.groundSpeed(); !near(float64(v), 0.182, 1e-6) {
		t.Fatalf("Speed II sprinting %v, want 0.182", v)
	}
	g.sprinting = false
	decodeFrame(t, effectPacket(5, effSlowness, 0, 100), nil)(g) // Slowness I on top
	if v := g.groundSpeed(); !near(float64(v), 0.14*0.85, 1e-6) {
		t.Fatalf("Speed II and Slowness I %v", v)
	}
	decodeFrame(t, effectPacket(5, effJump, 1, 100), nil)(g) // Jump Boost II
	g.jump()
	if !near(g.vel[1], 0.42+0.2, 1e-6) {
		t.Fatalf("jump %v, want 0.62", g.vel[1])
	}
	// other players' effects are not ours
	decodeFrame(t, effectPacket(9, effSpeed, 4, 100), nil)(g)
	if e := g.effects[effSpeed]; e.amp != 1 {
		t.Fatalf("someone else's effect reached us: %+v", e)
	}
	rm := newPacket(0x1E)
	rm.VarInt(5)
	rm.Byte(effSpeed)
	decodeFrame(t, rm, nil)(g)
	if _, ok := g.effects[effSpeed]; ok {
		t.Fatal("a removed effect stayed")
	}
	g.effects[effJump] = potionEffect{1, 2}
	g.tickEffects()
	g.tickEffects()
	if _, ok := g.effects[effJump]; ok {
		t.Fatal("an effect that ran out stayed")
	}
}

// Experience, the player's own attributes and golden hearts arrive as packets and reach the status bars.
func TestStatusBarPackets(t *testing.T) {
	g, _ := windowGame(t)
	g.net.eid = 3
	xp := newPacket(0x1F)
	xp.Float(0.6)
	xp.VarInt(12)
	xp.VarInt(300)
	decodeFrame(t, xp, nil)(g)
	if g.xpBar != 0.6 || g.xpLevel != 12 {
		t.Fatalf("xp %v level %d", g.xpBar, g.xpLevel)
	}
	// 20 base, a Health Boost modifier of +8 (operation 0): 28 hearts' worth
	pr := newPacket(0x20)
	pr.VarInt(3)
	pr.Int(2)
	pr.String("generic.maxHealth")
	pr.Double(20)
	pr.VarInt(1)
	pr.Bytes(make([]byte, 16))
	pr.Double(8)
	pr.Byte(0)
	pr.String("generic.movementSpeed")
	pr.Double(0.1)
	pr.VarInt(0)
	decodeFrame(t, pr, nil)(g)
	if g.maxHP != 28 {
		t.Fatalf("max health %v, want 28", g.maxHP)
	}
	// another entity's attributes are not ours
	pr2 := newPacket(0x20)
	pr2.VarInt(9)
	pr2.Int(1)
	pr2.String("generic.maxHealth")
	pr2.Double(100)
	pr2.VarInt(0)
	decodeFrame(t, pr2, nil)(g)
	if g.maxHP != 28 {
		t.Fatalf("someone else's max health reached us: %v", g.maxHP)
	}
	md := newPacket(0x1C)
	md.VarInt(3)
	md.Byte(3<<5 | 17) // absorption, a float
	md.Float(6)
	md.Byte(1<<5 | 1) // air, a short
	md.UShort(120)
	md.Byte(0x7F)
	decodeFrame(t, md, nil)(g)
	if g.absorption != 6 || g.air != 120 {
		t.Fatalf("absorption %v air %d", g.absorption, g.air)
	}
}

// sentAbilities is the flags of every Player Abilities packet the client sent, in order.
func sentAbilities(t *testing.T, c *netClient) []byte {
	t.Helper()
	var out []byte
	for {
		select {
		case f := <-c.out:
			id, rd, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(f)))
			if err != nil {
				t.Fatal(err)
			}
			if id == 0x13 {
				out = append(out, rd.Byte())
			}
		default:
			return out
		}
	}
}

// Starting or ending a flight tells the server, with the invulnerable and creative bits as the server gave
// them (C13PacketPlayerAbilities is built from the client's capabilities, which the game mode and S39 set).
func TestFlightToggleSendsAbilitiesAsTheGame(t *testing.T) {
	doubleJump := func(g *game) {
		g.tick(input{jump: true})
		g.tick(input{})
		g.tick(input{jump: true})
		g.tick(input{})
	}
	g, c := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true // the player waits for its column, as in the game
	decodeFrame(t, joinGame(5, 1, 0), nil)(g)         // creative: may fly, invulnerable
	sentAbilities(t, c)
	doubleJump(g)
	if got := sentAbilities(t, c); len(got) == 0 || got[0] != 1|2|4|8 || !g.flying {
		t.Fatalf("creative flight: sent %v flying %v, want first 15", got, g.flying)
	}

	g, c = windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	decodeFrame(t, joinGame(5, 0, 0), nil)(g)
	if g.creative || g.canFly() || g.abilFlags != 0 {
		t.Fatalf("survival join: creative %v canFly %v flags %d", g.creative, g.canFly(), g.abilFlags)
	}
	decodeFrame(t, abilitiesPacket(1|4), nil)(g) // a lobby: invulnerable and allowed to fly
	sentAbilities(t, c)
	doubleJump(g)
	if got := sentAbilities(t, c); len(got) == 0 || got[0] != 1|2|4 {
		t.Fatalf("lobby flight: sent %v, want first 7", got)
	}

	w := newPacket(0x2B) // back to survival: the flight and the permission go, as setGameType does
	w.Byte(3)
	w.Float(0)
	decodeFrame(t, w, nil)(g)
	if g.flying || g.canFly() || g.abilFlags != 0 {
		t.Fatalf("survival again: flying %v canFly %v flags %d", g.flying, g.canFly(), g.abilFlags)
	}
}

// Held Item Change moves the hand to the slot the server picks, and the client answers with the same slot,
// as the game's syncCurrentPlayItem does; a slot outside the hotbar is ignored.
func TestServerPicksTheHeldSlot(t *testing.T) {
	g, c := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	decodeFrame(t, joinGame(5, 0, 0), nil)(g)
	g.tick(input{})
	for len(c.out) > 0 {
		<-c.out
	}
	w := newPacket(0x09)
	w.Byte(4)
	decodeFrame(t, w, nil)(g)
	if g.sel != 4 {
		t.Fatalf("held slot %d, want 4", g.sel)
	}
	g.tick(input{})
	got := -1
	for len(c.out) > 0 {
		id, r, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(<-c.out)))
		if err == nil && id == 0x09 {
			got = int(r.Short())
		}
	}
	if got != 4 {
		t.Fatalf("the client answered slot %d, want 4", got)
	}
	w = newPacket(0x09)
	w.Byte(9)
	if f := decodeFrame(t, w, nil); f != nil {
		t.Fatal("slot 9 is not in the hotbar and should be ignored")
	}
}

// Hitting another player on a server sends the attack and, as the game's client does (EntityOtherPlayerMP says the
// blow landed), ends the attacker's sprint and slows them to 0.6; the server does the same to its copy and a
// movement check expects it. A mob is not like that: the client's world never lets a blow on it land.
func TestHittingAPlayerEndsTheSprintAsTheGameDoes(t *testing.T) {
	g, c := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	g.feet, g.prevFeet = vec3{0.5, 4, 0.5}, vec3{0.5, 4, 0.5}
	g.pos = g.feet.add(vec3{0, eyeHeight, 0})
	g.yaw, g.pitch = 0, 0
	g.onGround = true
	g.inv[0] = slot{ID: 276, N: 1, Dmg: 0} // a diamond sword
	g.sel = 0
	g.net.remote[9] = &remotePlayer{eid: 9, name: "Rival", pos: vec3{0.5, 4, 2.2}, prev: vec3{0.5, 4, 2.2}, target: vec3{0.5, 4, 2.2}}
	g.sprinting, g.vel = true, vec3{0, 0, 0.28}
	g.net.sentSprint = true
	hunger := g.exhaustion
	drain(t, c)
	if !g.attackClick() {
		t.Fatal("the click did not reach the player in front")
	}
	if !g.sprinting || g.vel[2] != 0.28 {
		t.Fatalf("the click itself changed the motion (%v, %v): it is paid at the start of the next tick", g.sprinting, g.vel)
	}
	g.applyPendingHit()
	if g.sprinting || !near(g.vel[2], 0.28*0.6, 1e-9) {
		t.Fatalf("after the blow: sprinting %v vel %v, want the sprint ended and the motion at 0.6", g.sprinting, g.vel)
	}
	if g.inv[0].Dmg == 0 || g.exhaustion == hunger {
		t.Fatalf("the blow cost nothing: wear %d hunger %v", g.inv[0].Dmg, g.exhaustion)
	}
	g.sendActions() // the end of the sprint is told to the server before the next movement, as the game does
	stopped := false
	for _, p := range drain(t, c) {
		if p.id == 0x0B {
			p.r.VarInt()
			stopped = p.r.VarInt() == 4
		}
	}
	if !stopped {
		t.Fatal("the end of the sprint was not told to the server")
	}
}

// A knockback replaces the motion and nothing more: the player is still on the ground for the tick it moves
// in, so the ground's friction slows it, and it does not slide as far as it would from the air.
func TestKnockbackKeepsTheGroundForItsFirstTick(t *testing.T) {
	g := standing(t)
	g.netVelocity(4000, 2880, 0) // 0.5 along x, 0.36 up
	if !g.onGround {
		t.Fatal("a velocity packet took the player off the ground")
	}
	if g.vel[0] != 0.5 || g.vel[1] != 0.36 {
		t.Fatalf("velocity %v", g.vel)
	}
	g.tick(input{})
	ground := g.vel[0]
	air := standing(t)
	air.netVelocity(4000, 2880, 0)
	air.onGround = false
	air.tick(input{})
	if ground >= air.vel[0] {
		t.Fatalf("after one tick %v on the ground against %v from the air: the ground must slow it more", ground, air.vel[0])
	}
}

// Player Position And Look puts the player where the server says: an axis it gives relative is added to where the
// player is and keeps its motion, one it gives absolute loses it, and a relative angle is added to the one the player
// looks along.
func TestServerTeleportKeepsTheMotionOfRelativeAxes(t *testing.T) {
	g, c := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	g.feet = vec3{10, 5, 20}
	g.vel = vec3{0.3, 0.2, 0.1}
	g.yaw, g.pitch = 0.5, 0.25
	drain(t, c)
	pkt := func(flags byte, x, y, z float64, yaw, pitch float32) {
		w := newPacket(0x08)
		w.Double(x)
		w.Double(y)
		w.Double(z)
		w.Float(yaw)
		w.Float(pitch)
		w.Byte(flags)
		decodeFrame(t, w, nil)(g)
	}
	pkt(0x01|0x10, 2, 70, 30, 90, 40) // x and the yaw relative; y, z and the pitch absolute
	if g.feet != (vec3{12, 70, 30}) {
		t.Fatalf("feet %v, want 12 70 30", g.feet)
	}
	if g.vel != (vec3{0.3, 0, 0}) {
		t.Fatalf("motion %v: the relative axis keeps its motion, the others lose it", g.vel)
	}
	if !near(g.yaw, 0.5+float32(math.Pi/2), 1e-5) || !near(g.pitch, float32(40*math.Pi/180), 1e-5) {
		t.Fatalf("yaw %v pitch %v", g.yaw, g.pitch)
	}
	ps := drain(t, c)
	if len(ps) != 1 || ps[0].id != 0x06 { // it answers at once with where it is
		t.Fatalf("answered %v", ps)
	}
	pkt(0, 1, 2, 3, 0, 0)
	if g.feet != (vec3{1, 2, 3}) || g.vel != (vec3{}) {
		t.Fatalf("absolute: feet %v motion %v", g.feet, g.vel)
	}
}

// A knockback that came just before the player's own blow is slowed by it: the packet is taken in at the start of
// the frame and the blow is paid at the start of the next tick, so the slowdown falls on the knockback. If the
// click slowed the old motion and the packet then replaced it, the player would go on as far as an unhit one.
func TestKnockbackThenBlowIsSlowed(t *testing.T) {
	run := func(hit, packetFirst bool) float64 {
		g, c := windowGame(t)
		c.inbox = make(chan func(*game), 16)
		g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
		g.feet, g.prevFeet = vec3{0.5, 4, 0.5}, vec3{0.5, 4, 0.5}
		g.pos = g.feet.add(vec3{0, eyeHeight, 0})
		g.yaw, g.pitch = 0, 0
		g.onGround = true
		g.net.remote[9] = &remotePlayer{eid: 9, name: "Rival", pos: vec3{0.5, 4, 2.2}, prev: vec3{0.5, 4, 2.2}, target: vec3{0.5, 4, 2.2}}
		g.sprinting = true
		g.net.sentSprint = true
		v := newPacket(0x12)
		v.VarInt(g.net.eid)
		v.UShort(uint16(int16(4000))) // 0.5 along x
		v.UShort(0)
		v.UShort(0)
		c.inbox <- decodeFrame(t, v, nil)
		if packetFirst {
			g.pumpLight() // the packet is taken in at the start of the frame, before the click
		}
		if hit && !g.attackClick() {
			t.Fatal("no blow")
		}
		g.pumpLight() // what the loop does before each tick: a packet that was still queued lands now
		if g.vel[0] != 0.5 {
			t.Fatalf("the knockback is %v", g.vel)
		}
		x0 := g.feet[0]
		g.tick(input{})
		return g.feet[0] - x0
	}
	for _, packetFirst := range []bool{true, false} { // the packet before the click, or still queued when it comes
		free, struck := run(false, packetFirst), run(true, packetFirst)
		if free < 0.3 || !near(struck/free, 0.6, 0.02) {
			t.Fatalf("packet first %v: an unhit player moved %.4f and one that struck %.4f, want 0.6 of it (%.3f)", packetFirst, free, struck, struck/free)
		}
	}
}
