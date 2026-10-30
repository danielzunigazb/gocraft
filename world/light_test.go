package world

import "testing"

func rules() *LightRules {
	r := &LightRules{}
	for id := 1; id < 256; id++ {
		r.Opacity[id] = 15 // solid unless said otherwise
	}
	r.Opacity[0], r.Opacity[20], r.Opacity[50] = 0, 0, 0 // air, glass, torches
	r.Opacity[8], r.Opacity[9] = 3, 3
	r.Luminance[50], r.Luminance[89] = 14, 15
	return r
}

// lit is a world of stone up to y=59 with open air above, lit by the sky from y=60 up.
func lit() *World {
	w := NewFrom(func(cp ChunkPos) *Chunk {
		c := &Chunk{Light: make([]uint8, 65536)}
		for y := 0; y < 256; y++ {
			for i := 0; i < 256; i++ {
				if y < 60 {
					c.Blocks[y<<8|i] = 1 << 4
				} else {
					c.Light[y<<8|i] = FullLight
				}
			}
		}
		return c
	})
	w.SetLightRules(rules())
	return w
}

func blockLight(w *World, x, y, z int32) uint8 { return w.Light(x, y, z) & 15 }
func skyLight(w *World, x, y, z int32) uint8   { return w.Light(x, y, z) >> 4 }

func TestTorchLightsAndFadesOneStepPerBlock(t *testing.T) {
	w := lit()
	w.SetBlockLit(5, 60, 5, 50<<4)
	for d, want := range map[int32]uint8{0: 14, 1: 13, 2: 12, 7: 7, 13: 1, 14: 0, 20: 0} {
		if got := blockLight(w, 5+d, 60, 5); got != want {
			t.Errorf("%d blocks from a torch: %d, want %d", d, got, want)
		}
	}
	// Diagonals count as steps along the axes: Manhattan distance.
	if got := blockLight(w, 7, 62, 8); got != 14-7 {
		t.Errorf("7 steps away: %d", got)
	}
	// Taking the torch away puts the dark back, everywhere it reached.
	w.SetBlockLit(5, 60, 5, 0)
	for _, c := range [][3]int32{{5, 60, 5}, {6, 60, 5}, {12, 60, 5}, {5, 63, 9}} {
		if got := blockLight(w, c[0], c[1], c[2]); got != 0 {
			t.Errorf("after removing the torch: %d at %v", got, c)
		}
	}
}

func TestWallsStopBlockLight(t *testing.T) {
	w := lit()
	for y := int32(60); y < 70; y++ {
		for z := int32(-5); z < 15; z++ {
			w.SetBlockLit(8, y, z, 1<<4) // a wall of stone west to east at x=8
		}
	}
	w.SetBlockLit(5, 61, 5, 50<<4)
	if got := blockLight(w, 7, 61, 5); got != 12 {
		t.Errorf("beside the wall: %d", got)
	}
	if got := blockLight(w, 9, 61, 5); got != 0 {
		t.Errorf("behind a tall wall: %d, want none", got)
	}
	// Over the top of the wall light gets round: 9 steps up the near side and 3 across is 12 steps.
	if got := blockLight(w, 8, 70, 5); got != 2 {
		t.Errorf("over the wall: %d, want 2", got)
	}
	// Open a window and light passes: glass is transparent, so the pane itself is 3 steps away
	// and the block behind it 4.
	w.SetBlockLit(8, 61, 5, 20<<4)
	if got := blockLight(w, 8, 61, 5); got != 11 {
		t.Errorf("in the glass: %d, want 11", got)
	}
	if got := blockLight(w, 9, 61, 5); got != 10 {
		t.Errorf("through glass: %d, want 10", got)
	}
}

func TestARoofDarkensTheSkyLightBelowIt(t *testing.T) {
	w := lit()
	// A hole in the ground: sky light falls into it undimmed.
	for y := int32(55); y < 60; y++ {
		w.SetBlockLit(10, y, 10, 0)
	}
	if got := skyLight(w, 10, 55, 10); got != 15 {
		t.Fatalf("bottom of an open shaft: %d, want 15", got)
	}
	w.SetBlockLit(10, 59, 10, 1<<4) // cap it
	for y := int32(55); y < 59; y++ {
		if got := skyLight(w, 10, y, 10); got != 0 {
			t.Fatalf("under the cap at y=%d: %d, want 0", y, got)
		}
	}
	// Uncap it and the light comes back.
	w.SetBlockLit(10, 59, 10, 0)
	if got := skyLight(w, 10, 55, 10); got != 15 {
		t.Fatalf("after opening it again: %d", got)
	}
}

func TestSideLightUnderASmallRoof(t *testing.T) {
	w := lit()
	for y := int32(54); y < 59; y++ { // dig out a room under the surface, leaving the top layer as its ceiling
		for x := int32(4); x <= 12; x++ {
			w.SetBlockLit(x, y, 20, 0)
		}
	}
	if got := skyLight(w, 8, 57, 20); got != 0 {
		t.Fatalf("a closed room is lit: %d", got)
	}
	// Open the ceiling over one end: light falls in and spreads along, one step dimmer a block.
	w.SetBlockLit(4, 59, 20, 0)
	if got := skyLight(w, 4, 58, 20); got != 15 {
		t.Fatalf("under the hole: %d", got)
	}
	if got := skyLight(w, 6, 58, 20); got != 13 {
		t.Fatalf("two blocks along: %d, want 13", got)
	}
	if got := skyLight(w, 12, 58, 20); got != 7 {
		t.Fatalf("eight blocks along: %d, want 7", got)
	}
	// Close the hole and the room goes dark again.
	w.SetBlockLit(4, 59, 20, 1<<4)
	if got := skyLight(w, 8, 58, 20); got != 0 {
		t.Fatalf("after closing it: %d", got)
	}
}

func TestPlacingARoofOverOpenGround(t *testing.T) {
	w := lit()
	// Under a single block in the open the light comes in from the sides, one step dimmer.
	w.SetBlockLit(30, 62, 30, 1<<4)
	if got := skyLight(w, 30, 61, 30); got != 14 {
		t.Errorf("under a single block: %d, want 14", got)
	}
	// Under the middle of a 3 x 3 roof two steps in from the open air.
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			w.SetBlockLit(50+dx, 62, 50+dz, 1<<4)
		}
	}
	if got := skyLight(w, 50, 61, 50); got != 13 {
		t.Errorf("under the middle of a 3x3 roof: %d, want 13", got)
	}
	// Taking it away restores full light.
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			w.SetBlockLit(50+dx, 62, 50+dz, 0)
		}
	}
	if got := skyLight(w, 50, 61, 50); got != 15 {
		t.Errorf("after taking the roof away: %d", got)
	}
}

func TestWaterDimsSkyLightByThreePerBlock(t *testing.T) {
	w := lit()
	for y := int32(55); y < 61; y++ { // a column of water in a one-block well
		w.SetBlockLit(70, y, 70, 0)
	}
	for y := int32(55); y < 61; y++ {
		w.SetBlockLit(70, y, 70, 9<<4)
	}
	for y, want := range map[int32]uint8{60: 12, 59: 9, 58: 6, 57: 3, 56: 0, 55: 0} {
		if got := skyLight(w, 70, y, 70); got != want {
			t.Errorf("water at y=%d: sky light %d, want %d", y, got, want)
		}
	}
}

func TestLightCrossesColumnsAndRemeshesTheNeighbour(t *testing.T) {
	w := lit()
	touched := w.SetBlockLit(15, 60, 8, 50<<4) // at the east edge of column 0
	if got := blockLight(w, 16, 60, 8); got != 13 {
		t.Fatalf("one block into the next column: %d", got)
	}
	has := map[ChunkPos]bool{}
	for _, c := range touched {
		has[c] = true
	}
	if !has[ChunkPos{0, 0}] || !has[ChunkPos{1, 0}] {
		t.Fatalf("columns to remesh: %v, want both sides of the edge", touched)
	}
	if has[ChunkPos{-1, 0}] || has[ChunkPos{0, 5}] {
		t.Fatalf("remeshing columns the light never reached: %v", touched)
	}
	// Both columns are marked as changed so the edit and its light are saved together.
	dirty := map[ChunkPos]bool{}
	for _, c := range w.TakeDirty() {
		dirty[c] = true
	}
	if !dirty[ChunkPos{0, 0}] || !dirty[ChunkPos{1, 0}] {
		t.Fatalf("dirty %v", dirty)
	}
}

func TestGlowstoneIsAnOpaqueLamp(t *testing.T) {
	w := lit()
	w.SetBlockLit(40, 61, 40, 89<<4)
	if got := blockLight(w, 40, 61, 40); got != 15 {
		t.Errorf("glowstone's own cell: %d", got)
	}
	if got := blockLight(w, 41, 61, 40); got != 14 {
		t.Errorf("next to glowstone: %d", got)
	}
}

func TestLightingAColumnThatHadNoLightData(t *testing.T) {
	w := New() // the flat world: nothing stores light
	w.SetLightRules(rules())
	w.SetBlockLit(3, 4, 3, 50<<4)
	if got := blockLight(w, 3, 4, 3); got != 14 {
		t.Fatalf("a torch on flat ground: %d", got)
	}
	if got := skyLight(w, 8, 10, 8); got != 15 {
		t.Fatalf("open sky over a column that gained light data: %d", got)
	}
	if got := skyLight(w, 8, 1, 8); got != 0 {
		t.Fatalf("inside the ground: %d", got)
	}
}
