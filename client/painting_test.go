package client

import (
	"testing"

	"gocraft/proto"
)

func paintingPacket(eid int32, title string, x, y, z int32, facing byte) *proto.Writer {
	w := newPacket(0x10)
	w.VarInt(eid)
	w.String(title)
	w.Long(proto.PackPos(x, y, z))
	w.Byte(facing)
	return w
}

// EnumArt has 26 pictures, each whole squares of 16 pixels that lie inside the 256 x 256 sheet.
func TestPaintingArtTable(t *testing.T) {
	if len(paintingArts) != 26 {
		t.Fatalf("%d arts, want 26", len(paintingArts))
	}
	seen := map[string]bool{}
	for _, a := range paintingArts {
		if seen[a.title] || a.w%16 != 0 || a.h%16 != 0 || a.u%16 != 0 || a.v%16 != 0 || a.u+a.w > 256 || a.v+a.h > 256 {
			t.Fatalf("bad art %+v", a)
		}
		seen[a.title] = true
	}
}

// The painting's entity is at the middle of the picture: 0.46875 off the block's middle towards the wall, and
// a picture of whole blocks is moved half a block to the left (rotateYCCW) and up.
func TestPaintingPosition(t *testing.T) {
	g, _, apply := mobGame(t)
	for i, d := range []struct {
		title   string
		facing  byte
		x, y, z float64
	}{
		{"Kebab", 0, 10.5, 5.5, 20.5 - 0.46875},     // south: the wall is to the north
		{"Pool", 0, 11.0, 5.5, 20.5 - 0.46875},      // 32 wide: half a block to the east, which is its left seen from the front
		{"Match", 2, 10.0, 6.0, 20.5 + 0.46875},     // north, 32 x 32: half a block west and up
		{"Kebab", 3, 10.5 - 0.46875 + 1, 5.5, 20.5}, // east: x moves towards the wall (west)
		{"Wanderer", 1, 10.5 + 0.46875, 6.0, 20.5},  // west, 16 x 32: up half a block
	} {
		eid := int32(10 + i)
		apply(paintingPacket(eid, d.title, 10, 5, 20, d.facing))
		e := g.net.entity(eid)
		if e == nil || e.painting == nil {
			t.Fatalf("%s: no painting", d.title)
		}
		want := vec3{d.x, d.y, d.z}
		if d.title == "Kebab" && d.facing == 3 {
			want[0] = 10.5 - 0.46875
		}
		if !near(e.pos[0], want[0], 1e-9) || !near(e.pos[1], want[1], 1e-9) || !near(e.pos[2], want[2], 1e-9) {
			t.Fatalf("%s facing %d: at %v, want %v", d.title, d.facing, e.pos, want)
		}
	}
	if f := decodeFrame(t, paintingPacket(40, "NoSuchArt", 1, 2, 3, 0), nil); f != nil {
		t.Fatal("a painting with an unknown title is applied")
	}
	if g.net.entity(40) != nil {
		t.Fatal("a painting with an unknown title was kept")
	}
}

// Each 16 x 16 square is six faces; the picture is the art's rectangle of the sheet, seen from the front.
func TestPaintingGeometry(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(paintingPacket(10, "Kebab", 10, 5, 20, 0))
	groups := map[string][]vertex{}
	g.emitPainting(groups, g.net.entity(10))
	vs := groups[paintingSheet]
	if len(vs) != 24 {
		t.Fatalf("a 16 x 16 painting has %d vertices, want 24", len(vs))
	}
	lo, hi := standBounds(vs)
	if !near(float64(lo[0]), 10, 1e-4) || !near(float64(hi[0]), 11, 1e-4) || !near(float64(lo[1]), 5, 1e-4) || !near(float64(hi[1]), 6, 1e-4) ||
		!near(float64(lo[2]), 20.0, 1e-4) || !near(float64(hi[2]), 20.0625, 1e-4) {
		t.Fatalf("Kebab spans %v..%v", lo, hi)
	}
	// 64 x 64 is sixteen squares, and Pigscene is at (64,192): its picture covers u 0.25-0.5, v 0.75-1
	apply(paintingPacket(11, "Pigscene", 30, 5, 20, 0))
	groups = map[string][]vertex{}
	g.emitPainting(groups, g.net.entity(11))
	vs = groups[paintingSheet]
	if len(vs) != 16*24 {
		t.Fatalf("Pigscene has %d vertices, want %d", len(vs), 16*24)
	}
	umin, umax, vmin, vmax := float32(9), float32(-9), float32(9), float32(-9)
	for i := 0; i < len(vs); i += 24 { // the front is the first quad of each square's six
		for _, v := range vs[i : i+4] {
			umin, umax, vmin, vmax = min(umin, v.U), max(umax, v.U), min(vmin, v.V), max(vmax, v.V)
		}
	}
	if umin != 0.25 || umax != 0.5 || vmin != 0.75 || vmax != 1 {
		t.Fatalf("Pigscene shows u %v..%v v %v..%v of the sheet", umin, umax, vmin, vmax)
	}
	// seen from the front (south of a south-facing painting) the picture's left is the sheet's left
	apply(paintingPacket(12, "Pool", 50, 5, 20, 0)) // 32 x 16: two squares
	groups = map[string][]vertex{}
	g.emitPainting(groups, g.net.entity(12))
	vs = groups[paintingSheet]
	var east, west vertex
	west.X, east.X = 1e9, -1e9
	for i := 0; i < len(vs); i += 24 {
		for _, v := range vs[i : i+4] {
			if v.X > east.X {
				east = v
			}
			if v.X < west.X {
				west = v
			}
		}
	}
	// the viewer stands south, looking north: east is on their right, so the right edge has the picture's right u
	if !(east.U > west.U) || west.U != 0 || east.U != 0.125 {
		t.Fatalf("east edge u %v, west edge u %v: the picture is mirrored for a viewer in front of it", east.U, west.U)
	}
}

// A click on a painting meets its own thin box, not a player-sized one.
func TestPaintingBox(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(paintingPacket(10, "Pointer", 10, 5, 20, 2)) // north, 64 x 64
	b := g.net.entity(10).box()
	if !near(b.max[0]-b.min[0], 4, 1e-9) || !near(b.max[1]-b.min[1], 4, 1e-9) || !near(b.max[2]-b.min[2], 1.0/16, 1e-9) { // 64 pixels are four blocks
		t.Fatalf("box %v..%v", b.min, b.max)
	}
	apply(paintingPacket(11, "Pointer", 30, 5, 20, 1)) // west: the width runs along z
	b = g.net.entity(11).box()
	if !near(b.max[2]-b.min[2], 4, 1e-9) || !near(b.max[0]-b.min[0], 1.0/16, 1e-9) {
		t.Fatalf("west box %v..%v", b.min, b.max)
	}
}
