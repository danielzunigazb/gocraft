package client

import (
	"math"
	"testing"

	"gocraft/proto"
)

func metaRot(idx byte, x, y, z float32) func(w *proto.Writer) {
	return func(w *proto.Writer) {
		w.Byte(7<<5 | idx)
		w.Float(x)
		w.Float(y)
		w.Float(z)
	}
}

func metaAll(fs ...func(w *proto.Writer)) func(w *proto.Writer) {
	return func(w *proto.Writer) {
		for _, f := range fs {
			f(w)
		}
	}
}

func equipPacket(eid int32, sl int16, id uint16) *proto.Writer {
	w := newPacket(0x04)
	w.VarInt(eid)
	w.UShort(uint16(sl))
	w.UShort(id)
	w.Byte(1)
	w.UShort(0)
	w.Byte(0) // no NBT
	return w
}

// standBounds are the lowest and highest corners of what a stand adds to a group.
func standBounds(vs []vertex) (lo, hi [3]float32) {
	lo, hi = [3]float32{1e9, 1e9, 1e9}, [3]float32{-1e9, -1e9, -1e9}
	for _, v := range vs {
		for i, c := range [3]float32{v.X, v.Y, v.Z} {
			lo[i], hi[i] = min(lo[i], c), max(hi[i], c)
		}
	}
	return
}

// A stand spawns with the default pose and takes its flags and its six rotations from the metadata.
func TestArmorStandPoseFromMetadata(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(40, 30, 3.5, 4, 3.5, 64, 64, nil))
	s := g.net.entity(40).stand
	if s == nil || s.pose[2] != [3]float32{-10, 0, -10} || s.pose[3] != [3]float32{-15, 0, 10} || s.pose[4] != [3]float32{-1, 0, -1} || s.pose[5] != [3]float32{1, 0, 1} {
		t.Fatalf("default pose %+v", s)
	}
	if s.arms || s.noBase || !near(s.yaw, 90, 1e-4) {
		t.Fatalf("a bare stand has arms %v, no base %v, yaw %v", s.arms, s.noBase, s.yaw)
	}
	apply(spawnMobPacket(41, 30, 5.5, 4, 3.5, 0, 0, metaAll(metaByte(10, 1|4|8), metaRot(11, 10, 20, 30), metaRot(14, 1, 2, 3))))
	e := g.net.entity(41)
	if !e.small || !e.stand.arms || !e.stand.noBase || e.stand.pose[0] != [3]float32{10, 20, 30} || e.stand.pose[3] != [3]float32{1, 2, 3} {
		t.Fatalf("stand %+v small %v", e.stand, e.small)
	}
	if e.stand.pose[2] != [3]float32{-10, 0, -10} { // what the metadata does not say stays
		t.Fatalf("left arm %v", e.stand.pose[2])
	}
}

// The server dresses a stand with Entity Equipment: what it holds and the four pieces of armour.
func TestArmorStandEquipment(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(40, 30, 3.5, 4, 3.5, 0, 0, nil))
	apply(equipPacket(40, 0, 276))
	apply(equipPacket(40, 4, 306))
	apply(equipPacket(40, 1, 309))
	s := g.net.entity(40).stand
	if s.held.ID != 276 || s.armor[3].ID != 306 || s.armor[0].ID != 309 {
		t.Fatalf("held %v armour %v", s.held, s.armor)
	}
	apply(equipPacket(40, 4, 0xFFFF)) // empty
	if !s.armor[3].empty() {
		t.Fatalf("the helmet is still %v", s.armor[3])
	}
}

// The model is what ModelArmorStand's bytecode says: a bare stand has eight parts, arms add two, a missing
// plate takes one away, and it is as tall as its head's top (pivot 1, box -7..0) puts it.
func TestArmorStandModelParts(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	count := func(eid int32, meta func(w *proto.Writer)) (int, [3]float32, [3]float32) {
		apply(spawnMobPacket(eid, 30, 3.5, 4, 3.5, 0, 0, meta))
		e := g.net.entity(eid)
		e.prev = e.pos
		groups := map[string][]vertex{}
		g.emitStand(groups, e, e.small)
		vs := groups["armorstand/wood"]
		lo, hi := standBounds(vs)
		return len(vs) / 24, lo, hi
	}
	if n, lo, hi := count(50, nil); n != 8 {
		t.Fatalf("a bare stand has %d parts, want 8 (head, body, legs, three rods... and the plate)", n)
	} else {
		// the plate is 12 wide, on the ground (feet at 4 + 0.0078125), the head tops out 6 pixels above y=1.5078125
		if !near(float64(hi[1]-4), 1.5078125+6.0/16, 1e-4) || !near(float64(lo[1]-4), 1.5078125-24.0/16, 1e-4) {
			t.Fatalf("stand spans y %v..%v", lo[1]-4, hi[1]-4)
		}
		if !near(float64(hi[0]-lo[0]), 12.0/16, 1e-4) || !near(float64(hi[2]-lo[2]), 12.0/16, 1e-4) { // the body's rack and the plate
			t.Fatalf("stand is %v wide and %v deep, want 0.75 each", hi[0]-lo[0], hi[2]-lo[2])
		}
	}
	if n, _, _ := count(51, metaByte(10, 4)); n != 10 {
		t.Fatalf("a stand with arms has %d parts, want 10", n)
	}
	if n, _, _ := count(52, metaByte(10, 4|8)); n != 9 {
		t.Fatalf("a stand with arms and no plate has %d parts, want 9", n)
	}
	apply(spawnMobPacket(53, 30, 3.5, 4, 3.5, 0, 0, metaByte(0, 0x20))) // invisible
	groups := map[string][]vertex{}
	e := g.net.entity(53)
	e.prev = e.pos
	g.emitStand(groups, e, false)
	if len(groups["armorstand/wood"]) != 0 {
		t.Fatalf("an invisible stand draws %d vertices", len(groups["armorstand/wood"]))
	}
}

// A small stand is drawn as a child model: half the height, the head three quarters of its size.
func TestSmallArmorStandIsAChild(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(60, 30, 3.5, 4, 3.5, 0, 0, metaByte(10, 1)))
	e := g.net.entity(60)
	e.prev = e.pos
	groups := map[string][]vertex{}
	g.emitStand(groups, e, true)
	lo, hi := standBounds(groups["armorstand/wood"])
	// the head's top: 0.7578125 - 0.75 * (-6) / 16; the plate's underside on the ground, a hair above it
	if !near(float64(hi[1]-4), 0.7578125+0.75*6/16, 1e-4) {
		t.Fatalf("small stand tops out at %v", hi[1]-4)
	}
	if !near(float64(lo[1]-4), 0.00390625+0.5*(1.5078125-24.0/16), 1e-4) {
		t.Fatalf("small stand bottoms at %v", lo[1]-4)
	}
}

// The server turns a stand with a look, and the stand gets there over three ticks.
func TestArmorStandTurns(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(40, 30, 3.5, 4, 3.5, 0, 0, nil))
	w := newPacket(0x16)
	w.VarInt(40)
	w.Byte(64) // 90 degrees
	w.Byte(0)
	w.Bool(true)
	apply(w)
	s := g.net.entity(40).stand
	for i, want := range []float64{30, 60, 90} {
		g.tick(input{})
		if !near(float64(s.yaw), want, 1e-3) {
			t.Fatalf("tick %d: yaw %v, want %v", i+1, s.yaw, want)
		}
	}
}

// The plate stays put as the stand turns: it turns against the yaw. The rods follow the body's rotation.
func TestArmorStandPlateAndRods(t *testing.T) {
	s := newStand(90)
	s.pose[1] = [3]float32{10, 20, 30}
	r := s.rots()
	if !near(float64(r[sBase].y), -90*degToRad, 1e-6) {
		t.Fatalf("plate turn %v", r[sBase].y)
	}
	for _, i := range []int{sRodR, sRodL, sWaist} {
		if r[i] != r[sBody] {
			t.Fatalf("part %d does not follow the body: %v vs %v", i, r[i], r[sBody])
		}
	}
	if math.Abs(float64(r[sBody].x)-10*degToRad) > 1e-6 {
		t.Fatalf("body x %v", r[sBody].x)
	}
}
