package client

import (
	"testing"

	"gocraft/proto"
	"gocraft/world"
)

func spawnItemPacket(eid int32, x, y, z float64, vel vec3) *proto.Writer {
	w := newPacket(0x0E)
	w.VarInt(eid)
	w.Byte(objItem)
	w.Int(int32(x * 32))
	w.Int(int32(y * 32))
	w.Int(int32(z * 32))
	w.Byte(0) // pitch
	w.Byte(0) // yaw
	w.Int(1)  // an item always carries its motion
	w.UShort(uint16(int16(vel[0] * 8000)))
	w.UShort(uint16(int16(vel[1] * 8000)))
	w.UShort(uint16(int16(vel[2] * 8000)))
	return w
}

func itemMetaPacket(eid int32, id uint16, n byte, dmg uint16) *proto.Writer {
	w := newPacket(0x1C)
	w.VarInt(eid)
	w.Byte(0<<5 | 0) // flags first, as the server writes them
	w.Byte(0)
	w.Byte(5<<5 | 10) // the stack
	w.UShort(id)
	w.Byte(n)
	w.UShort(dmg)
	w.Byte(0) // no NBT
	w.Byte(0x7F)
	return w
}

func collectPacket(item, collector int32) *proto.Writer {
	w := newPacket(0x0D)
	w.VarInt(item)
	w.VarInt(collector)
	return w
}

func destroyPacket(ids ...int32) *proto.Writer {
	w := newPacket(0x13)
	w.VarInt(int32(len(ids)))
	for _, id := range ids {
		w.VarInt(id)
	}
	return w
}

// A server's item appears, learns what it is from metadata 10 and falls onto the ground on its own; it is
// neither picked up nor merged here, even next to the player and to a stack of the same item.
func TestServerItemFallsAndWaitsForTheServer(t *testing.T) {
	g, _ := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true // the player waits for its column, as in the game
	g.feet, g.prevFeet = vec3{8.5, 4, 8.5}, vec3{8.5, 4, 8.5}
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(spawnItemPacket(40, 8.5, 6, 8.5, vec3{}))
	apply(spawnItemPacket(41, 8.6, 6, 8.5, vec3{}))
	if len(g.items) != 2 || !g.items[0].net || !g.items[0].stack.empty() {
		t.Fatalf("items %d, the first net %v stack %v", len(g.items), g.items[0].net, g.items[0].stack)
	}
	apply(itemMetaPacket(40, 4, 3, 0)) // three cobblestone
	apply(itemMetaPacket(41, 4, 2, 0))
	if s := g.net.netItem(40).stack; s.ID != 4 || s.N != 3 {
		t.Fatalf("stack %+v, want 3 of id 4", s)
	}
	for i := 0; i < 80; i++ {
		g.tick(input{})
	}
	if len(g.items) != 2 {
		t.Fatalf("%d items left: the client merged or took one", len(g.items))
	}
	if e := g.net.netItem(40); !near(e.pos[1], 4, 1e-6) {
		t.Fatalf("the item is at y %v, want lying on the ground at 4", e.pos[1])
	}
	if g.inv[0].N != 0 {
		t.Fatalf("the player got %+v: pickup is the server's", g.inv[0])
	}
}

// The server's moves are kept exact in 32nds, and an item put inside a block is lifted onto it
// (Entity.setPositionAndRotation2).
func TestServerItemMovesAndIsLiftedOutOfBlocks(t *testing.T) {
	g, _ := windowGame(t)
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(spawnItemPacket(7, 3.5, 10, 3.5, vec3{}))
	e := g.net.netItem(7)
	w := newPacket(0x15) // relative move
	w.VarInt(7)
	w.Byte(32) // +1 x
	w.Byte(byte(0x100 - 16))
	w.Byte(1) // +1/32 z
	w.Bool(false)
	apply(w)
	if e.pos != (vec3{4.5, 9.5, 3.5 + 1.0/32}) || e.srv != e.pos {
		t.Fatalf("after a relative move %v (server %v)", e.pos, e.srv)
	}
	tp := newPacket(0x18)
	tp.VarInt(7)
	tp.Int(int32(3.5 * 32))
	tp.Int(int32(3.75 * 32)) // inside the flat world's top layer, which ends at y=4
	tp.Int(int32(3.5 * 32))
	tp.Byte(0)
	tp.Byte(0)
	tp.Bool(true)
	apply(tp)
	if !near(e.pos[1], 4, 1e-12) || e.srv[1] != 3.75 {
		t.Fatalf("teleported into the ground: y %v (server %v), want lifted to 4", e.pos[1], e.srv[1])
	}
	v := newPacket(0x12)
	v.VarInt(7)
	v.UShort(800)
	v.UShort(0)
	v.UShort(0)
	apply(v)
	if e.vel[0] != 0.1 {
		t.Fatalf("velocity %v", e.vel)
	}
}

// Collect Item takes the item away and flies it to the collector in three ticks, squared; Destroy Entities
// and a respawn take items away too.
func TestServerItemCollectAndDestroy(t *testing.T) {
	g, _ := windowGame(t)
	g.net.eid = 1
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	g.feet, g.prevFeet = vec3{0.5, 4, 0.5}, vec3{0.5, 4, 0.5}
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(spawnItemPacket(9, 2.5, 4, 0.5, vec3{}))
	apply(itemMetaPacket(9, 264, 1, 0)) // a diamond
	apply(collectPacket(9, 1))
	if g.net.netItem(9) != nil || len(g.net.pickups) != 1 {
		t.Fatalf("after collect: item %v, %d pickups", g.net.netItem(9), len(g.net.pickups))
	}
	p := g.net.pickups[0]
	if at := g.pickupAt(p, 0); at != p.item.pos {
		t.Fatalf("the pickup starts at %v, want where the item lay %v", at, p.item.pos)
	}
	p.age = 1
	want := p.item.pos.add(vec3{0.5, 4.5, 0.5}.sub(p.item.pos).scale(4.0 / 9))
	if at := g.pickupAt(p, 1); at.sub(want).dot(at.sub(want)) > 1e-18 {
		t.Fatalf("two thirds of the way: %v, want %v ((2/3)^2 of it)", at, want)
	}
	p.age = 0
	g.tick(input{})
	if len(g.items) != 0 {
		t.Fatalf("%d items after the tick: a collected item stays in the world", len(g.items))
	}
	g.tick(input{})
	g.tick(input{})
	if len(g.net.pickups) != 0 {
		t.Fatalf("%d pickups after three ticks", len(g.net.pickups))
	}

	apply(spawnItemPacket(10, 2.5, 4, 0.5, vec3{}))
	apply(spawnItemPacket(11, 2.5, 4, 0.5, vec3{}))
	apply(destroyPacket(10))
	g.tick(input{})
	if g.net.netItem(10) != nil || len(g.items) != 1 {
		t.Fatalf("destroy: %d items left", len(g.items))
	}
	apply(respawn(0, 0))
	if len(g.items) != 0 || g.net.netItem(11) != nil {
		t.Fatal("a respawn left the server's items")
	}
}

// An experience orb is spawned where the server says, is not a click target, draws as the sprite of its size
// with a colour that pulses, and is gone when it is collected.
func TestExperienceOrbs(t *testing.T) {
	g, _ := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	w := newPacket(0x11)
	w.VarInt(80)
	w.Int(int32(2.5 * 32))
	w.Int(int32(5 * 32))
	w.Int(int32(3.5 * 32))
	w.UShort(37)
	decodeFrame(t, w, nil)(g)
	e := g.net.entity(80)
	if e == nil || e.obj != objOrb || e.orb != 37 || e.pos != (vec3{2.5, 5, 3.5}) {
		t.Fatalf("orb %+v", e)
	}
	for v, want := range map[int]int{0: 0, 2: 0, 3: 1, 6: 1, 7: 2, 36: 3, 37: 4, 148: 5, 149: 6, 616: 7, 617: 8, 1236: 8, 1237: 9, 2476: 9, 2477: 10, 99999: 10} {
		if got := orbIcon(v); got != want {
			t.Errorf("an orb of %d is icon %d, want %d", v, got, want)
		}
	}
	vs := orbVertices(nil, e.pos, vec3{1, 0, 0}, vec3{0, 1, 0}, 37, 0, 0, 0xF0)
	if len(vs) != 4 || vs[0].U != 0 || vs[2].U != 16.0/64 || vs[0].V != 32.0/64 { // icon 4: first column, second row
		t.Fatalf("quad %+v", vs)
	}
	if h := vs[2].Y - vs[0].Y; !near(h, 0.3, 1e-6) {
		t.Fatalf("an orb is %v high, want 0.3", h)
	}
	pulse := map[uint8]bool{}
	for age := 0; age < 40; age++ {
		pulse[orbVertices(nil, e.pos, vec3{1, 0, 0}, vec3{0, 1, 0}, 37, age, 0, 0xF0)[0].C[0]] = true
	}
	if len(pulse) < 10 {
		t.Fatalf("the colour takes only %d values over its life", len(pulse))
	}
	g.tick(input{})
	if e.age != 1 {
		t.Fatalf("age %d", e.age)
	}
	if h := g.net.entity(80); h.height() == 0 {
		t.Log("height is not used for an orb")
	}
	if _, ok := g.entityTarget(); ok {
		t.Fatal("an orb is a click target")
	}
	decodeFrame(t, collectPacket(80, 1), nil)(g)
	if g.net.entity(80) != nil {
		t.Fatal("a collected orb stayed")
	}
}
