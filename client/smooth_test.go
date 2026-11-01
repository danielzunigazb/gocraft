package client

import (
	"testing"

	"gocraft/world"
)

// padWorld is a padded column of open air, lit by the sky at 15, with a floor of stone at y=3 (x and z of 0 to 15).
func padWorld(t *testing.T) (*resources, *world.Padded) {
	g := testGame(t)
	p := new(world.Padded)
	for y := 0; y < 256; y++ {
		for z := -1; z <= 16; z++ {
			for x := -1; x <= 16; x++ {
				i := world.PadIndex(x, y, z)
				p.Light[i] = 0xF0
				if y <= 3 {
					p.Blocks[i], p.Light[i] = 1<<4, 0
				}
			}
		}
	}
	return g.res, p
}

func upFace() *quad {
	return &quad{cull: dUp, dir: dUp, shade: 1, apos: [4][3]float32{{0, 1, 0}, {0, 1, 1}, {1, 1, 1}, {1, 1, 0}}}
}

func solidAt(p *world.Padded, x, y, z int) {
	i := world.PadIndex(x, y, z)
	p.Blocks[i], p.Light[i] = 1<<4, 0
}

// On open ground a face is lit by what is above it and is not darkened; next to a wall the corners on the wall's
// side get 0.2 for the wall in their average of four (0.8), and in a concave corner a vertex between two solid
// cells counts the diagonal one as solid too (0.4).
func TestSmoothFaceShadesTheCornersNextToSolids(t *testing.T) {
	res, p := padWorld(t)
	idx := world.PadIndex(5, 3, 5)
	face := func() []vertex { return smoothFace(nil, res, p, idx, 3, upFace(), 5, 3, 5, white) }
	for i, v := range face() {
		if v.C != [4]uint8{255, 255, 255, 0xF0} {
			t.Fatalf("open ground, vertex %d: %v", i, v.C)
		}
	}
	solidAt(p, 6, 4, 5) // a wall on the east side of the cell above the block
	vs := face()
	for i, v := range vs { // the quad's x=1 vertices are 2 and 3
		want := uint8(255)
		if i >= 2 {
			want = toByte(0.8)
		}
		if v.C[0] != want {
			t.Fatalf("beside a wall, vertex %d is %d, want %d", i, v.C[0], want)
		}
		if v.C[3] != 0xF0 { // the wall holds no light: it is read as the light in front of the face
			t.Fatalf("beside a wall, vertex %d light %#x", i, v.C[3])
		}
	}
	solidAt(p, 5, 4, 6) // and one on the south side: the vertex at x=1, z=1 is between the two
	vs = face()
	if got := vs[2].C[0]; got != toByte(0.4) {
		t.Fatalf("a concave corner: %d, want %d", got, toByte(0.4))
	}
	if got := vs[0].C[0]; got != 255 {
		t.Fatalf("the far corner is %d, it should be untouched", got)
	}
}

// A corner's light is the average of the four cells around it (sky and block apart); a face of a solid block next to
// a dim cell is smoothed down to it.
func TestSmoothFaceAveragesTheLight(t *testing.T) {
	res, p := padWorld(t)
	idx := world.PadIndex(5, 3, 5)
	set := func(x, z int, l uint8) { p.Light[world.PadIndex(x, 4, z)] = l }
	set(5, 5, 0xF3) // in front of the face: sky 15, block 3
	set(6, 5, 0x75) // an edge cell: sky 7, block 5
	set(5, 6, 0x75) // the other edge cell of the vertex at x=1, z=1
	set(6, 6, 0x75) // and its diagonal
	vs := smoothFace(nil, res, p, idx, 3, upFace(), 5, 3, 5, white)
	// (7+7+7+15+2)>>2 = 9 sky; (5+5+5+3+2)>>2 = 5 block
	if got := vs[2].C[3]; got != 0x95 {
		t.Fatalf("the corner's light %#x, want 0x95", got)
	}
}

// Only the whole face of a block is smoothed: a half-height side or an inset face is not.
func TestWholeFace(t *testing.T) {
	if !wholeFace(upFace(), dUp) {
		t.Fatal("the top of a block is a whole face")
	}
	slab := &quad{cull: dNorth, apos: [4][3]float32{{1, 0.5, 0}, {1, 0, 0}, {0, 0, 0}, {0, 0.5, 0}}}
	if wholeFace(slab, dNorth) {
		t.Fatal("the side of a slab is half a face")
	}
	inset := &quad{cull: dUp, apos: [4][3]float32{{0.25, 1, 0.25}, {0.25, 1, 0.75}, {0.75, 1, 0.75}, {0.75, 1, 0.25}}}
	if wholeFace(inset, dUp) {
		t.Fatal("a small face on the top is not a whole face")
	}
	lower := &quad{cull: dUp, apos: [4][3]float32{{0, 0.9, 0}, {0, 0.9, 1}, {1, 0.9, 1}, {1, 0.9, 0}}}
	if wholeFace(lower, dUp) {
		t.Fatal("a face below the top of the block is not on its boundary")
	}
}

// The padding has its corners, from the four diagonal columns, which the shading of faces at the edge of a column
// reads.
func TestPaddedCornersComeFromTheDiagonalColumns(t *testing.T) {
	g := testGame(t)
	g.world.SetBlock(-1, 5, -1, 4<<4) // the diagonal column at x-1, z-1: its last row and column
	g.world.SetBlock(16, 5, -1, 5<<4)
	g.world.SetBlock(-1, 5, 16, 6<<4)
	g.world.SetBlock(16, 5, 16, 7<<4)
	p := new(world.Padded)
	g.world.FillPadded(world.ChunkPos{X: 0, Z: 0}, p)
	for _, c := range []struct {
		x, z int
		id   uint16
	}{{-1, -1, 4}, {16, -1, 5}, {-1, 16, 6}, {16, 16, 7}} {
		if got := p.Blocks[world.PadIndex(c.x, 5, c.z)] >> 4; got != c.id {
			t.Errorf("corner %d,%d holds %d, want %d", c.x, c.z, got, c.id)
		}
	}
}
