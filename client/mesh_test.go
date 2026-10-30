package client

import (
	"fmt"
	"sort"
	"testing"

	"gocraft/world"
)

// quadKey is a quad reduced to comparable text: its four corners, atlas coordinates and colour.
func quadKey(vs [4]vertex) string {
	s := ""
	for _, v := range vs {
		s += fmt.Sprintf("%.4f,%.4f,%.4f/%.5f,%.5f ", v.X, v.Y, v.Z, v.U, v.V)
	}
	return s
}

func newQuads(vs []vertex) (keys []string, cols [][4]uint8) {
	for i := 0; i+4 <= len(vs); i += 4 {
		keys = append(keys, quadKey([4]vertex{vs[i], vs[i+1], vs[i+2], vs[i+3]}))
		cols = append(cols, vs[i].C)
	}
	return
}

// refQuads reads the reference mesher's output (6 vertices per quad, float colours) the same way.
func refQuads(f []float32) (keys []string, cols [][4]uint8) {
	for i := 0; i+6*vertexFloats <= len(f); i += 6 * vertexFloats {
		var vs [4]vertex
		for k, src := range [4]int{0, 1, 2, 5} { // the two triangles (0,1,2) and (0,2,3)
			o := i + src*vertexFloats
			vs[k] = vertex{X: f[o], Y: f[o+1], Z: f[o+2], U: f[o+3], V: f[o+4]}
		}
		keys = append(keys, quadKey(vs))
		cols = append(cols, [4]uint8{toByte(f[i+5]), toByte(f[i+6]), toByte(f[i+7]), 255})
	}
	return
}

func sortedWithColours(keys []string, cols [][4]uint8) []string {
	out := make([]string, len(keys))
	for i := range keys {
		out[i] = fmt.Sprint(keys[i], cols[i])
	}
	sort.Strings(out)
	return out
}

// The fast mesher must draw exactly what the earlier, simpler one drew: same faces, same texture
// coordinates, same light and tint (within one colour step of rounding). Leaves differ on purpose
// in fast graphics, so this compares the fancy setting.
func TestFastMesherMatchesTheReference(t *testing.T) {
	g := testWorldGame(t, "New World")
	m := &mesher{}
	var md meshData
	for _, cp := range []world.ChunkPos{{X: -14, Z: 11}, {X: -13, Z: 11}, {X: -14, Z: 12}, {X: -15, Z: 10}} {
		m.mesh(g.world, cp, g.res, true, &md)
		ref := refMeshChunk(g.world, cp, g.res)

		gotKeys, gotCols := newQuads(md.solid)
		wantKeys, wantCols := refQuads(ref.solid)
		if len(gotKeys) != len(wantKeys) {
			t.Fatalf("column %v: %d quads, the reference has %d", cp, len(gotKeys), len(wantKeys))
		}
		// Same quads (geometry), then colours within one step.
		type kc struct {
			k string
			c [4]uint8
		}
		build := func(keys []string, cols [][4]uint8) map[string][][4]uint8 {
			m := map[string][][4]uint8{}
			for i, k := range keys {
				m[k] = append(m[k], cols[i])
			}
			return m
		}
		gm, wm := build(gotKeys, gotCols), build(wantKeys, wantCols)
		if len(gm) != len(wm) {
			t.Fatalf("column %v: %d distinct quads, want %d", cp, len(gm), len(wm))
		}
		for k, wc := range wm {
			gc := gm[k]
			if len(gc) != len(wc) {
				t.Fatalf("column %v: quad %s appears %d times, want %d", cp, k, len(gc), len(wc))
			}
			sort.Slice(gc, func(i, j int) bool { return fmt.Sprint(gc[i]) < fmt.Sprint(gc[j]) })
			sort.Slice(wc, func(i, j int) bool { return fmt.Sprint(wc[i]) < fmt.Sprint(wc[j]) })
			for i := range wc {
				for ch := 0; ch < 3; ch++ {
					if d := int(gc[i][ch]) - int(wc[i][ch]); d < -1 || d > 1 {
						t.Fatalf("column %v: quad %s colour %v, want %v", cp, k, gc[i], wc[i])
					}
				}
			}
		}
		gw, ww := len(md.water)/4, len(ref.water)/(6*vertexFloats)
		if gw != ww {
			t.Fatalf("column %v: %d water quads, want %d", cp, gw, ww)
		}
		_ = kc{}
	}
}

func TestFastGraphicsDropsInnerLeafFaces(t *testing.T) {
	g := testWorldGame(t, "New World")
	m := &mesher{}
	var fast, fancy meshData
	cp := world.ChunkPos{X: -14, Z: 11}
	m.mesh(g.world, cp, g.res, false, &fast)
	m.mesh(g.world, cp, g.res, true, &fancy)
	if len(fast.solid) >= len(fancy.solid) {
		t.Fatalf("fast %d vertices, fancy %d: leaves should lose their inner faces", len(fast.solid), len(fancy.solid))
	}
	t.Logf("forest column: %d vertices fancy, %d fast (%.0f%% fewer)", len(fancy.solid), len(fast.solid), 100*(1-float64(len(fast.solid))/float64(len(fancy.solid))))
}

func TestFrustumCulling(t *testing.T) {
	proj := perspective(70*3.14159265/180, 16.0/9.0, 0.05, 200)
	view := lookAt(vec3{0, 0, 0}, vec3{0, 0, 1}, vec3{0, 1, 0}) // looking towards +Z
	mvp := mul(proj, view)
	f := newFrustum(&mvp)
	if !f.sees(-1, -1, 10, 1, 1, 12) {
		t.Error("a box straight ahead is hidden")
	}
	if f.sees(-1, -1, -12, 1, 1, -10) {
		t.Error("a box behind the camera is drawn")
	}
	if f.sees(300, -1, 10, 302, 1, 12) {
		t.Error("a box far to the side is drawn")
	}
	if f.sees(-1, -1, 300, 1, 1, 302) {
		t.Error("a box beyond the far plane is drawn")
	}
	if !f.sees(-1, -1, -1, 1, 1, 1) {
		t.Error("a box around the camera is hidden")
	}
}

func TestPaddedColumnMatchesTheWorld(t *testing.T) {
	g := testWorldGame(t, "New World")
	cp := world.ChunkPos{X: -14, Z: 11}
	var p world.Padded
	g.world.FillPadded(cp, &p)
	for _, c := range [][3]int{{0, 70, 0}, {15, 65, 15}, {-1, 66, 3}, {16, 66, 3}, {4, 66, -1}, {4, 66, 16}, {7, 100, 7}} {
		wx, wy, wz := cp.X*16+int32(c[0]), int32(c[1]), cp.Z*16+int32(c[2])
		if got, want := p.Blocks[world.PadIndex(c[0], c[1], c[2])], g.world.Block(wx, wy, wz); got != want {
			t.Errorf("padded %v = %d, world says %d", c, got, want)
		}
		if got, want := p.Light[world.PadIndex(c[0], c[1], c[2])], g.world.Light(wx, wy, wz); got != want {
			t.Errorf("padded light %v = %x, world says %x", c, got, want)
		}
	}
}

func TestFacesTakeTheLightOfTheBlockTheyFace(t *testing.T) {
	g := testGame(t)
	// A stone block on the flat world's surface, with its own light made distinct from its neighbours.
	g.world.SetBlock(2, 4, 2, 1<<4)
	c := g.world.Snapshot(world.ChunkPos{})
	c.Light = make([]uint8, 65536)
	for i := range c.Light {
		c.Light[i] = 0xF0 // full sky
	}
	idx := func(x, y, z int) int { return y<<8 | z<<4 | x }
	c.Light[idx(2, 5, 2)] = 0x3A // above the block: sky 3, block 10
	c.Light[idx(3, 4, 2)] = 0x70 // east of it: sky 7, block 0
	g.world = world.NewFrom(func(cp world.ChunkPos) *world.Chunk {
		if cp == (world.ChunkPos{}) {
			return c
		}
		return nil
	})
	m := &mesher{}
	var md meshData
	m.mesh(g.world, world.ChunkPos{}, g.res, true, &md)
	found := map[float32]uint8{} // light byte of a face, keyed by the face's height or x
	for i := 0; i+4 <= len(md.solid); i += 4 {
		v := md.solid[i]
		if v.X >= 2 && v.X <= 3 && v.Z >= 2 && v.Z <= 3 && v.Y >= 4 && v.Y <= 5 {
			found[v.Y*10+v.X] = v.C[3]
		}
	}
	top, east := uint8(0), uint8(0)
	for i := 0; i+4 <= len(md.solid); i += 4 {
		q := md.solid[i : i+4]
		if q[0].X >= 2 && q[2].X <= 3 && q[0].Z >= 2 && q[2].Z <= 3 {
			if q[0].Y == 5 && q[1].Y == 5 && q[2].Y == 5 { // a top face of the block
				top = q[0].C[3]
			}
			if q[0].X == 3 && q[1].X == 3 && q[2].X == 3 && q[0].Y <= 5 && q[0].Y >= 4 { // an east face
				east = q[0].C[3]
			}
		}
	}
	if top != 0x3A {
		t.Errorf("top face light %#x, want the light above it (0x3a)", top)
	}
	if east != 0x70 {
		t.Errorf("east face light %#x, want the light beside it (0x70)", east)
	}
	if maxLight(0x3A, 0x70) != 0x7A {
		t.Error("maxLight takes sky and block light apart")
	}
}
