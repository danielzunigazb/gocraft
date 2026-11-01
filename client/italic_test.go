package client

import "testing"

// An italic glyph leans: the game moves the top of each quad right by a pixel and the bottom left by one, a
// plain one stays square.
func TestItalicGlyphsLean(t *testing.T) {
	var h hud
	h.rect(10, 20, 8, 8, 0, 0, 8, 8, 128, 128, [4]float32{1, 1, 1, 1})
	plain := append([]float32(nil), h.verts...)
	h.verts = h.verts[:0]
	h.rectLean(10, 20, 8, 8, 0, 0, 8, 8, 128, 128, [4]float32{1, 1, 1, 1}, 0)
	if len(h.verts) != len(plain) {
		t.Fatal("no lean changed the vertex count")
	}
	for i := range plain {
		if h.verts[i] != plain[i] {
			t.Fatalf("lean 0 differs from rect at %d", i)
		}
	}
	h.verts = h.verts[:0]
	h.rectLean(10, 20, 8, 8, 0, 0, 8, 8, 128, 128, [4]float32{1, 1, 1, 1}, 2)
	// the first triangle is (top-left, bottom-left, bottom-right): x 12 at y 20, x 8 at y 28, x 16 at y 28
	x := func(v int) float32 { return h.verts[v*8] }
	y := func(v int) float32 { return h.verts[v*8+1] }
	if x(0) != 12 || y(0) != 20 || x(1) != 8 || y(1) != 28 || x(2) != 16 || y(2) != 28 {
		t.Fatalf("leaning quad %v %v, %v %v, %v %v", x(0), y(0), x(1), y(1), x(2), y(2))
	}
	if x(5) != 20 || y(5) != 20 { // the top-right corner, moved the same way as the top-left
		t.Fatalf("top right at %v %v", x(5), y(5))
	}
}
