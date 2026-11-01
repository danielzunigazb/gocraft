package client

import (
	"testing"

	"gocraft/proto"
)

// mapPacket is Map Data: an id, a scale, icons, and a rectangle of colours (none when cols is 0).
func mapPacket(id int32, cols, rows, x0, z0 int, data []byte) *proto.Writer {
	w := newPacket(0x34)
	w.VarInt(id)
	w.Byte(1)
	w.VarInt(1) // one icon: a player marker, turned 3, at (5, 6)
	w.Byte(1<<4 | 3)
	w.Byte(5)
	w.Byte(6)
	w.Byte(byte(cols))
	if cols > 0 {
		w.Byte(byte(rows))
		w.Byte(byte(x0))
		w.Byte(byte(z0))
		w.VarInt(int32(len(data)))
		w.Bytes(data)
	}
	return w
}

// A colour index is a base colour times a shade of 180, 220, 255 or 135 over 255; index 0 is a dark, see-through
// checker whose two squares are told apart by the alpha.
func TestMapPixels(t *testing.T) {
	for _, d := range []struct {
		c       byte
		r, g, b uint8
	}{
		{4*1 + 2, 127, 178, 56}, // grass at full
		{4*1 + 1, 127 * 220 / 255, 178 * 220 / 255, 56 * 220 / 255},
		{4*1 + 0, 127 * 180 / 255, 178 * 180 / 255, 56 * 180 / 255},
		{4*1 + 3, 127 * 135 / 255, 178 * 135 / 255, 56 * 135 / 255},
		{4*4 + 2, 255, 0, 0}, // TNT red
		{4*8 + 2, 255, 255, 255},
	} {
		if p := mapPixel(0, d.c); p.R != d.r || p.G != d.g || p.B != d.b || p.A != 255 {
			t.Fatalf("colour %d: %v, want %d %d %d", d.c, p, d.r, d.g, d.b)
		}
	}
	for _, d := range []struct {
		i int
		a uint8
	}{{0, 16}, {1, 24}, {2, 16}, {127, 24}, {128, 24}, {129, 16}} { // (i + i/128 & 1) * 8 + 16
		if p := mapPixel(d.i, 0); p.A != d.a || p.R != 0 || p.G != 0 || p.B != 0 {
			t.Fatalf("pixel %d of colour 0: %v, want alpha %d", d.i, p, d.a)
		}
	}
	if mapBase[1] != 0x7FB238 || mapBase[35] != 7340544 || len(mapBase) != 36 {
		t.Fatal("the base colours are not MapColor's")
	}
}

// Map Data puts a rectangle of colours into the map, row by row, and leaves the rest.
func TestMapDataRectangles(t *testing.T) {
	g, _, apply := mobGame(t)
	data := []byte{10, 11, 12, 20, 21, 22} // 3 columns, 2 rows
	apply(mapPacket(7, 3, 2, 100, 50, data))
	m := g.net.maps[7]
	if m == nil || m.scale != 1 || !m.dirty || len(m.icons) != 1 || m.icons[0] != (mapIcon{1, 3, 5, 6}) {
		t.Fatalf("map %+v", m)
	}
	for i, want := range data {
		x, z := 100+i%3, 50+i/3
		if got := m.colors[x+z*mapSize]; got != want {
			t.Fatalf("(%d, %d) is %d, want %d", x, z, got, want)
		}
	}
	if m.colors[99+50*mapSize] != 0 || m.colors[103+50*mapSize] != 0 || m.colors[100+52*mapSize] != 0 {
		t.Fatal("a pixel outside the rectangle changed")
	}
	m.dirty = false
	apply(mapPacket(7, 0, 0, 0, 0, nil)) // icons only
	if m.dirty || len(m.icons) != 1 {
		t.Fatal("a packet with no colours marked the picture changed")
	}
	apply(mapPacket(7, 1, 1, 100, 50, []byte{99})) // a later rectangle changes one pixel
	if m.colors[100+50*mapSize] != 99 || m.colors[101+50*mapSize] != 11 || !m.dirty {
		t.Fatal("an update did not merge")
	}
	// a rectangle that does not fit, or a length that is not columns times rows, is not applied
	if f := decodeFrame(t, mapPacket(8, 100, 2, 50, 0, make([]byte, 200)), nil); f != nil {
		t.Fatal("a rectangle past the edge was applied")
	}
	if f := decodeFrame(t, mapPacket(8, 3, 2, 0, 0, make([]byte, 5)), nil); f != nil {
		t.Fatal("a short rectangle was applied")
	}
	if g.net.maps[8] != nil {
		t.Fatal("a refused map was kept")
	}
}

// A map in an item frame is a quad of a block in front of the frame, the picture's top left at the viewer's top left.
func TestMapInAnItemFrame(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(mapPacket(0, 1, 1, 0, 0, []byte{4*4 + 2}))
	apply(spawnFrame(10, 10, 5, 20, 0)) // south: seen from the south, with the wall behind
	apply(frameMeta(10, itemMap, 0))    // map id 0
	groups := map[string][]vertex{}
	g.emitFrame(groups, g.net.entity(10))
	vs := groups[mapKey(0)]
	if len(vs) != 4 {
		t.Fatalf("%d vertices for the map, want 4", len(vs))
	}
	lo, hi := standBounds(vs)
	if !near(float64(lo[0]), 10, 1e-3) || !near(float64(hi[0]), 11, 1e-3) || !near(float64(lo[1]), 5, 1e-3) || !near(float64(hi[1]), 6, 1e-3) {
		t.Fatalf("the map spans %v..%v, want the block 10..11, 5..6", lo, hi)
	}
	// in front of the frame's slab (which ends at 20.0625) by 0.4375 - 1.01/128 from the block's middle
	for _, v := range vs {
		if !near(float64(v.Z), 20.5-(0.4375-1.01/128), 1e-4) {
			t.Fatalf("a corner is at z %v", v.Z)
		}
	}
	// the viewer stands south looking north: the picture's top left (u 0, v 0) is on their left (west, x small) and up
	for _, v := range vs {
		if v.U == 0 && v.V == 0 && !(v.X < 10.5 && v.Y > 5.5) {
			t.Fatalf("the top left is at (%v, %v), want the west, upper corner", v.X, v.Y)
		}
		if v.U == 1 && v.V == 1 && !(v.X > 10.5 && v.Y < 5.5) {
			t.Fatalf("the bottom right is at (%v, %v)", v.X, v.Y)
		}
	}
	// the frame's turn counts in quarters for a map: 4 is the same as 0
	apply(frameMeta(10, itemMap, 4))
	again := map[string][]vertex{}
	g.emitFrame(again, g.net.entity(10))
	for i, v := range again[mapKey(0)] {
		if !near(float64(v.X), float64(vs[i].X), 1e-4) || !near(float64(v.Y), float64(vs[i].Y), 1e-4) {
			t.Fatalf("rotation 4 moved a corner: %v vs %v", v, vs[i])
		}
	}
	apply(frameMeta(10, itemMap, 1)) // a quarter turn
	turned := map[string][]vertex{}
	g.emitFrame(turned, g.net.entity(10))
	var tl vertex
	for _, v := range turned[mapKey(0)] {
		if v.U == 0 && v.V == 0 {
			tl = v
		}
	}
	if !(tl.X > 10.5 && tl.Y > 5.5) { // 90 degrees: the top left is now at the east, upper corner
		t.Fatalf("after a quarter turn the top left is at (%v, %v)", tl.X, tl.Y)
	}
	// a map nobody told about is not drawn
	apply(frameMeta(10, itemMap, 0))
	g.net.maps = nil
	empty := map[string][]vertex{}
	g.emitFrame(empty, g.net.entity(10))
	if len(empty[mapKey(0)]) != 0 {
		t.Fatal("an unknown map was drawn")
	}
}
