package client

import (
	"bufio"
	"bytes"
	"math"
	"testing"

	"gocraft/proto"
)

func spawnHook(eid int32, owner int32, x, y, z float64) *proto.Writer {
	w := newPacket(0x0E)
	w.VarInt(eid)
	w.Byte(objFishHook)
	w.Int(int32(x * 32))
	w.Int(int32(y * 32))
	w.Int(int32(z * 32))
	w.Byte(0)
	w.Byte(0)
	w.Int(owner)
	if owner > 0 {
		w.UShort(0)
		w.UShort(0)
		w.UShort(0)
	}
	return w
}

func TestFishingHookSpawnsAndFollowsTheServer(t *testing.T) {
	g, _ := windowGame(t)
	g.net.eid = 7
	decodeFrame(t, spawnHook(50, 7, 10, 64, 10), nil)(g)
	e := g.net.hookOf(7)
	if e == nil || e.eid != 50 || e.owner != 7 || e.pos != (vec3{10, 64, 10}) {
		t.Fatalf("hook %+v", e)
	}
	if g.net.hookOf(8) != nil {
		t.Fatal("another player's hook was taken for ours")
	}
	// the server teleports it two blocks east: the game takes five ticks (three, and the hook's two more)
	tp := newPacket(0x18)
	tp.VarInt(50)
	tp.Int(12 * 32)
	tp.Int(64 * 32)
	tp.Int(10 * 32)
	tp.Byte(0)
	tp.Byte(0)
	decodeFrame(t, tp, nil)(g)
	if e.inc != 5 || e.target != (vec3{12, 64, 10}) {
		t.Fatalf("inc %d target %v", e.inc, e.target)
	}
	prev := e.pos[0]
	for i := 1; i <= 5; i++ {
		g.tickEnts()
		if e.pos[0] <= prev {
			t.Fatalf("tick %d: x %v did not advance", i, e.pos[0])
		}
		prev = e.pos[0]
	}
	if e.pos[0] != 12 || e.inc != 0 {
		t.Fatalf("after five ticks x %v inc %d", e.pos[0], e.inc)
	}
	// a mob gets there in three
	decodeFrame(t, spawnStand(9, 0, 0, 0, "x", 0), nil)(g)
	m := newPacket(0x15)
	m.VarInt(9)
	m.Byte(32)
	m.Byte(0)
	m.Byte(0)
	m.Bool(true)
	decodeFrame(t, m, nil)(g)
	if g.net.ents[9].inc != 3 {
		t.Fatalf("a mob's inc is %d, want 3", g.net.ents[9].inc)
	}
}

// RenderFish: where the line leaves the rod, in first and third person.
func TestAnglerTip(t *testing.T) {
	feet := vec3{100, 64, 200}
	tip, extra := anglerTip(feet, 0, 0, 0, 0, 1.62, false, false)
	if !near(tip[0], 100-0.36, 1e-6) || !near(tip[1], 64.03, 1e-6) || !near(tip[2], 200.35, 1e-6) || !near(extra, 1.62, 1e-6) {
		t.Fatalf("first person, facing south: %v %v", tip, extra)
	}
	// turned a quarter (yaw 90, facing west): the right hand, with the rod, is to the north
	tip, _ = anglerTip(feet, 90, 0, 0, 0, 1.62, false, false)
	if !near(tip[0], 100-0.35, 1e-6) || !near(tip[2], 200-0.36, 1e-6) {
		t.Fatalf("first person, yaw 90: %v", tip)
	}
	// seen from outside the line leaves from in front of the body, lower, and a sneak lowers it again
	tip, extra = anglerTip(feet, 0, 0, 0, 0, 1.62, false, true)
	if !near(tip[0], 100-0.35, 1e-6) || !near(tip[1], 64+1.62-0.45, 1e-6) || !near(tip[2], 200+0.8, 1e-6) || extra != 0 {
		t.Fatalf("third person: %v %v", tip, extra)
	}
	if _, extra = anglerTip(feet, 0, 0, 0, 0, 1.54, true, true); !near(extra, -0.1875, 1e-9) {
		t.Fatalf("sneaking extra %v", extra)
	}
	// the swing of the arm moves the first-person tip
	still, _ := anglerTip(feet, 0, 0, 0, 0, 1.62, false, false)
	swung, _ := anglerTip(feet, 0, 0, 0, 0.5, 1.62, false, false)
	if swung == still {
		t.Fatal("a swing did not move the tip")
	}
}

func TestFishingLineHangsBetweenHookAndRod(t *testing.T) {
	hook := vec3{10, 64, 10}
	tip := vec3{10, 65.62, 4}
	pts := linePoints(hook, tip, 1.62)
	if pts[0] != (vec3{10, 64.25, 10}) {
		t.Fatalf("the line starts at %v, want on top of the hook", pts[0])
	}
	if end := pts[16]; !near(end[0], 10, 1e-4) || !near(end[1], 65.62+1.62, 1e-4) || !near(end[2], 4, 1e-4) {
		t.Fatalf("the line ends at %v", end)
	}
	for i := 1; i <= 16; i++ {
		if pts[i][2] >= pts[i-1][2] {
			t.Fatalf("the line goes backwards at %d", i)
		}
	}
	// halfway the arc is above the straight line: it sags the other way, as RenderFish's f*f+f has it
	mid := pts[8]
	straight := (pts[0][1] + pts[16][1]) / 2
	if math.Abs(mid[1]-straight) < 1e-6 {
		t.Fatal("the line is straight")
	}
}

// A rod aimed at the ground: the game sends the click on the block and then the use in the air.
func TestRodAtTheGroundSendsBothClicks(t *testing.T) {
	g, c := windowGame(t)
	g.inv[0] = slot{ID: 346, N: 1}
	g.sel = 0
	g.pos = vec3{0.5, 5.4, 0.5}
	for _, pitch := range []float32{1.4, -1.4} {
		g.pitch = pitch
		if _, ok := g.target(); ok {
			break
		}
	}
	if _, ok := g.target(); !ok {
		t.Skip("no block in reach")
	}
	g.useBlock()
	var ids []int32
	for len(c.out) > 0 {
		f := <-c.out
		id, _, _ := proto.ReadPacket(bufioReader(f))
		ids = append(ids, id)
	}
	// block placement, swing, use in the air, swing
	if len(ids) != 4 || ids[0] != 0x08 || ids[1] != 0x0A || ids[2] != 0x08 || ids[3] != 0x0A {
		t.Fatalf("packets %v, want 8, 10, 8, 10", ids)
	}
	if g.swingTicks != 6 {
		t.Fatal("casting a rod did not swing the arm")
	}
}

func bufioReader(b []byte) *bufio.Reader { return bufio.NewReader(bytes.NewReader(b)) }

func spawnObject(eid int32, kind byte, data int32, x, y, z float64, vx, vy, vz int16) *proto.Writer {
	w := newPacket(0x0E)
	w.VarInt(eid)
	w.Byte(kind)
	w.Int(int32(x * 32))
	w.Int(int32(y * 32))
	w.Int(int32(z * 32))
	w.Byte(0)
	w.Byte(0)
	w.Int(data)
	if data > 0 {
		w.UShort(uint16(vx))
		w.UShort(uint16(vy))
		w.UShort(uint16(vz))
	}
	return w
}

// An arrow flies on its own between the server's updates, falls as arrows do, and stops in a block; the
// server's position replaces its own at once.
func TestServerArrowFliesAndSticks(t *testing.T) {
	g, _ := windowGame(t)
	g.net.eid = 7
	// 2.4 blocks a tick along +z (19200/8000), a little up
	decodeFrame(t, spawnObject(80, objArrow, 7, 0.5, 8, 0.5, 0, 800, 19200), nil)(g)
	e := g.net.ents[80]
	if e == nil || e.obj != objArrow || !near(e.vel[2], 2.4, 1e-9) || !near(e.vel[1], 0.1, 1e-9) {
		t.Fatalf("arrow %+v", e)
	}
	prevZ := e.pos[2]
	g.tickEnts()
	if !near(e.pos[2]-prevZ, 2.4, 1e-9) || !near(e.pos[1], 8.1, 1e-9) {
		t.Fatalf("after one tick at %v", e.pos)
	}
	if !near(e.vel[2], 2.4*0.99, 1e-9) || !near(e.vel[1], 0.1*0.99-arrowGravity, 1e-9) {
		t.Fatalf("drag and gravity: vel %v", e.vel)
	}
	// a wall ahead: the flat test world has none, so put one up
	for y := int32(0); y < 20; y++ {
		for x := int32(-2); x <= 3; x++ {
			g.world.SetBlock(x, y, 12, 1<<4)
		}
	}
	for i := 0; i < 20 && !e.stuck; i++ {
		g.tickEnts()
	}
	if !e.stuck || e.pos[2] < 11.5 || e.pos[2] > 12.1 {
		t.Fatalf("stuck %v at %v, want stopped in the wall at z=12", e.stuck, e.pos)
	}
	at := e.pos
	g.tickEnts()
	if e.pos != at {
		t.Fatal("an arrow in a block moved")
	}
	// the server's update puts it where it says, at once
	tp := newPacket(0x18)
	tp.VarInt(80)
	tp.Int(1 * 32)
	tp.Int(9 * 32)
	tp.Int(2 * 32)
	tp.Byte(0)
	tp.Byte(0)
	decodeFrame(t, tp, nil)(g)
	if e.pos != (vec3{1, 9, 2}) || e.inc != 0 {
		t.Fatalf("after the server's update %v inc %d", e.pos, e.inc)
	}
}

func TestThrownItemsAreShown(t *testing.T) {
	g, _ := windowGame(t)
	decodeFrame(t, spawnObject(81, objEnderPearl, 7, 0.5, 8, 0.5, 100, 200, 100), nil)(g)
	decodeFrame(t, spawnObject(82, objPotion, 16421, 3, 8, 3, 0, 0, 0), nil)(g)
	if it, ok := thrownItem(objPotion, 16421); !ok || it.ID != 373 || it.Dmg != 16421 {
		t.Fatalf("potion item %v", it)
	}
	g.yaw, g.pitch = 0, 0
	groups := map[string][]vertex{}
	g.emitProjectiles(groups)
	if n := len(groups["!items"]); n != 8 {
		t.Fatalf("%d vertices for a pearl and a potion, want 8", n)
	}
	// velocity packets move what flies
	v := newPacket(0x12)
	v.VarInt(81)
	v.UShort(0)
	v.UShort(0)
	v.UShort(uint16(8000))
	decodeFrame(t, v, nil)(g)
	if e := g.net.ents[81]; !near(e.vel[2], 1, 1e-9) {
		t.Fatalf("velocity %v", e.vel)
	}
}
