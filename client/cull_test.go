package client

import (
	"testing"

	"gocraft/world"
)

// allSeen is a frustum that contains everything.
var allSeen = frustum{{0, 0, 0, 1e9}, {0, 0, 0, 1e9}, {0, 0, 0, 1e9}, {0, 0, 0, 1e9}, {0, 0, 0, 1e9}, {0, 0, 0, 1e9}}

func TestSectionVisibility(t *testing.T) {
	g := testGame(t)
	// Section 5 of the column at 0,0 (y 80..95): solid stone, then carve.
	fill := func(x0, y0, z0, x1, y1, z1 int32, st uint16) {
		for x := x0; x <= x1; x++ {
			for y := y0; y <= y1; y++ {
				for z := z0; z <= z1; z++ {
					g.world.SetBlock(x, y, z, st)
				}
			}
		}
	}
	fill(0, 80, 0, 15, 95, 15, 1<<4)
	fill(6, 86, 6, 9, 89, 9, 0) // a sealed cavity in the middle
	var m mesher
	var md meshData
	m.mesh(g.world, world.ChunkPos{}, g.res, true, &md)
	if md.vis[5] != 0 {
		t.Fatalf("a sealed cave joins no faces, got %015b", md.vis[5])
	}
	if md.vis[9] != visAll {
		t.Fatalf("an empty section joins all faces, got %015b", md.vis[9])
	}
	// A tunnel from the west face to the east face of the section.
	fill(0, 86, 7, 15, 87, 8, 0)
	m.mesh(g.world, world.ChunkPos{}, g.res, true, &md)
	if want := pairBit[dWest][dEast]; md.vis[5] != want {
		t.Fatalf("a tunnel west to east joins only those faces: got %015b, want %015b", md.vis[5], want)
	}
	// The flat world's section 0 is solid below y=4 and open above: every face but the bottom connects.
	var flat meshData
	m.mesh(world.New(), world.ChunkPos{X: 40, Z: 40}, g.res, true, &flat)
	if flat.vis[0]&pairBit[dDown][dUp] != 0 || flat.vis[0]&pairBit[dWest][dEast] == 0 || flat.vis[0]&pairBit[dUp][dNorth] == 0 {
		t.Fatalf("the open top of the flat world joins the sides and the top, not the bottom: %015b", flat.vis[0])
	}
}

// Quads are laid out by section, so a section's range is exactly its geometry.
func TestSectionRangesCoverTheMesh(t *testing.T) {
	g := testGame(t)
	g.world.SetBlock(3, 70, 3, 1<<4)
	g.world.SetBlock(4, 20, 4, 1<<4)
	var m mesher
	var md meshData
	m.mesh(g.world, world.ChunkPos{}, g.res, true, &md)
	// The quads are sorted by facing direction, then by section: every group is one section's.
	end := int32(0)
	for k := 0; k < faceClasses; k++ {
		for s := 0; s < 16; s++ {
			if md.solidF.start[k][s] != end {
				t.Fatalf("direction %d section %d starts at %d, the previous group ended at %d", k, s, md.solidF.start[k][s], end)
			}
			next := md.solidF.start[k][s+1]
			if s < 15 {
				next = md.solidF.start[k][s+1]
			} else {
				next = md.solidF.start[k][16]
			}
			for i := md.solidF.start[k][s]; i < next; i++ {
				if y := int(md.solid[i].Y); y>>4 != s {
					t.Fatalf("vertex %d at y %d is in the group of section %d", i, y, s)
				}
			}
			end = next
		}
	}
	if end != int32(len(md.solid)) {
		t.Fatalf("the groups end at %d of %d vertices", end, len(md.solid))
	}
}

func TestSectionRuns(t *testing.T) {
	// Sections 0 to 3 hold 10 quads each; 4 holds none; 5 and 6 hold 10 each.
	var sec [17]int32
	for s := 0; s < 16; s++ {
		n := int32(10)
		if s == 4 || s > 6 {
			n = 0
		}
		sec[s+1] = sec[s] + n
	}
	run := func(mask uint16) [][2]int32 {
		var r [8][2]int32
		n := sectionRuns(&sec, mask, &r)
		return r[:n]
	}
	if got := run(0b0001111); len(got) != 1 || got[0] != [2]int32{0, 40} {
		t.Fatalf("four neighbouring sections are one range, got %v", got)
	}
	if got := run(0b1101111); len(got) != 1 || got[0] != [2]int32{0, 60} {
		t.Fatalf("an empty hidden section does not break a run: %v", got)
	}
	if got := run(0b0000101); len(got) != 2 || got[0] != [2]int32{0, 10} || got[1] != [2]int32{20, 30} {
		t.Fatalf("a hidden section with quads does: %v", got)
	}
	if got := run(0); len(got) != 0 {
		t.Fatalf("nothing is drawn for no sections: %v", got)
	}
}

// A walk from the camera's section: it enters a section that blocks its way, and goes no further.
func TestCaveCullingTraversal(t *testing.T) {
	g := testGame(t)
	g.meshes = map[world.ChunkPos]*chunkMesh{}
	col := &chunkMesh{}
	for s := range col.vis {
		col.vis[s] = visAll
	}
	col.vis[3] = 0 // a section of solid rock under the camera's section: it joins no faces
	g.meshes[world.ChunkPos{}] = col
	g.cullSections(&allSeen, 0, 0, 4*16+8, 1) // the camera is in section 4
	m := g.vis.masks[(1*3)+1]                 // the centre column of a 3x3 grid
	if m&(1<<4) == 0 || m&(1<<3) == 0 {
		t.Fatalf("the camera's section and the rock under it are reached: %016b", m)
	}
	if m&(1<<2) != 0 || m&(1<<1) != 0 || m&1 != 0 {
		t.Fatalf("what lies behind the rock cannot be seen: %016b", m)
	}
	if m&(1<<5) == 0 || m&(1<<15) == 0 {
		t.Fatalf("the open sky above is reached: %016b", m)
	}
}
