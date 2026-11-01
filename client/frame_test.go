package client

import (
	"testing"

	"gocraft/proto"
)

// spawnFrame is Spawn Object (71): the block it hangs in times 32, and the facing as the data.
func spawnFrame(eid int32, x, y, z int32, facing int32) *proto.Writer {
	return spawnObject(eid, objItemFrame, facing, float64(x), float64(y), float64(z), 0, 0, 0)
}

// frameMeta is Entity Metadata with the item (entry 8) and its rotation (entry 9).
func frameMeta(eid int32, item uint16, rot byte) *proto.Writer {
	w := newPacket(0x1C)
	w.VarInt(eid)
	w.Byte(5<<5 | 8)
	w.UShort(item)
	w.Byte(1)
	w.UShort(0)
	w.Byte(0) // no NBT
	w.Byte(0<<5 | 9)
	w.Byte(rot)
	w.Byte(0x7F)
	return w
}

// The frame hangs in the block the packet names, faces where the data says, and keeps its item and turn.
func TestItemFrameSpawnAndMetadata(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnFrame(10, 10, 5, 20, 3)) // east: data 3 is more than 0, so velocity follows it
	e := g.net.entity(10)
	if e == nil || e.frame == nil {
		t.Fatal("no frame")
	}
	if e.frame.hang != [3]int32{10, 5, 20} || e.frame.facing != 3 {
		t.Fatalf("frame %+v", e.frame)
	}
	// 12 pixels is no whole block: the middle is the block's, 0.46875 towards the wall
	if !near(e.pos[0], 10.5-0.46875, 1e-9) || !near(e.pos[1], 5.5, 1e-9) || !near(e.pos[2], 20.5, 1e-9) {
		t.Fatalf("frame at %v", e.pos)
	}
	apply(frameMeta(10, 276, 5))
	if e.frame.item.ID != 276 || e.frame.rot != 5 {
		t.Fatalf("item %v rot %d", e.frame.item, e.frame.rot)
	}
	b := e.box()
	if !near(b.max[1]-b.min[1], 12.0/16, 1e-9) || !near(b.max[2]-b.min[2], 12.0/16, 1e-9) || !near(b.max[0]-b.min[0], 1.0/16, 1e-9) {
		t.Fatalf("box %v..%v", b.min, b.max)
	}
}

// The frame is the model of the game: its slab lies against the wall, whichever way it faces.
func TestItemFrameModelAgainstTheWall(t *testing.T) {
	g, _, apply := mobGame(t)
	for _, d := range []struct {
		facing int32
		axis   int
		lo, hi float64 // the slab's span along its thin axis
	}{
		{0, 2, 20.0, 20.0625}, // south: the wall is north, the slab on the block's north side
		{2, 2, 20.9375, 21},   // north
		{1, 0, 10.9375, 11},   // west: the wall is east
		{3, 0, 10.0, 10.0625}, // east
	} {
		eid := 20 + d.facing
		apply(spawnFrame(eid, 10, 5, 20, d.facing))
		groups := map[string][]vertex{}
		g.emitFrame(groups, g.net.entity(eid))
		vs := groups["!blocks"]
		if len(vs) == 0 {
			t.Fatalf("facing %d: nothing drawn", d.facing)
		}
		lo, hi := standBounds(vs)
		if !near(float64(lo[d.axis]), d.lo, 1e-4) || !near(float64(hi[d.axis]), d.hi, 1e-4) {
			t.Fatalf("facing %d: slab spans %v..%v on axis %d, want %v..%v", d.facing, lo[d.axis], hi[d.axis], d.axis, d.lo, d.hi)
		}
		if !near(float64(hi[1]-lo[1]), 12.0/16, 1e-4) { // the frame is 12 pixels tall
			t.Fatalf("facing %d: %v tall", d.facing, hi[1]-lo[1])
		}
	}
}

// What is in the frame stands at 0.4375 in front of the block's middle, a quarter of a block across, and turns
// by eighths of a turn.
func TestItemFrameShowsItsItem(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnFrame(10, 10, 5, 20, 0)) // south
	groups := map[string][]vertex{}
	g.emitFrame(groups, g.net.entity(10))
	if len(groups["!items"]) != 0 {
		t.Fatal("an empty frame shows an item")
	}
	apply(frameMeta(10, 264, 0)) // a diamond
	groups = map[string][]vertex{}
	g.emitFrame(groups, g.net.entity(10))
	vs := groups["!items"]
	if len(vs) == 0 {
		t.Fatal("the diamond is not drawn")
	}
	lo, hi := standBounds(vs)
	mid := (lo[2] + hi[2]) / 2
	if !near(float64(mid), 20.0625, 0.01) {
		t.Fatalf("the item is at z %v, want the slab's front face at 20.0625", mid)
	}
	w0 := hi[0] - lo[0]
	if !near(float64(w0), 0.25, 0.01) || !near(float64(hi[1]-lo[1]), 0.25, 0.01) {
		t.Fatalf("item is %v by %v, want a quarter of a block", w0, hi[1]-lo[1])
	}
	apply(frameMeta(10, 264, 1)) // an eighth of a turn: the square's corners reach out
	groups = map[string][]vertex{}
	g.emitFrame(groups, g.net.entity(10))
	lo, hi = standBounds(groups["!items"])
	if !near(float64(hi[1]-lo[1]), 0.25*1.41421356, 0.01) {
		t.Fatalf("turned by 45 degrees it spans %v, want %v", hi[1]-lo[1], 0.25*1.41421356)
	}
	apply(frameMeta(10, 358, 0)) // a map takes the other model and shows no item (maps are not drawn yet)
	groups = map[string][]vertex{}
	g.emitFrame(groups, g.net.entity(10))
	if len(groups["!items"]) != 0 || len(groups["!blocks"]) == 0 {
		t.Fatalf("map frame: %d item vertices, %d frame vertices", len(groups["!items"]), len(groups["!blocks"]))
	}
}
