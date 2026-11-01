package client

import (
	"testing"

	"gocraft/proto"
)

// metaPacket is Entity Metadata with the entries a function writes.
func metaPacket(eid int32, entries func(w *proto.Writer)) *proto.Writer {
	w := newPacket(0x1C)
	w.VarInt(eid)
	entries(w)
	w.Byte(0x7F)
	return w
}

// A magma cube is eight slices of the body and a core, a pixel high each, on the slime's rules for its size.
func TestMagmaCubeModel(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(60, 62, 3.5, 4, 3.5, 0, 0, metaByte(16, 2)))
	e := g.net.entity(60)
	if e == nil || e.mob == nil || e.mob.m.kind.id != "LavaSlime" || e.mob.m.size != 2 {
		t.Fatalf("magma cube %+v", e)
	}
	e.mob.m.prev = e.mob.m.pos
	groups := map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	vs := groups["slime/magmacube"]
	if len(vs) != 9*24 {
		t.Fatalf("%d vertices, want %d", len(vs), 9*24)
	}
	lo, hi := standBounds(vs)
	// the slices run from 16 to 24 pixels down the model: half a block, and twice that at size 2
	if !near(float64(hi[1]-lo[1]), 2*0.5, 1e-3) || !near(float64(hi[0]-lo[0]), 2*0.5, 1e-3) {
		t.Fatalf("size 2 spans %v wide and %v high, want 1 and 1", hi[0]-lo[0], hi[1]-lo[1])
	}
	if w, h := e.mob.dims(); w != 1.2 || h != 1.2 {
		t.Fatalf("hit box %v x %v", w, h)
	}
}

// A ghast has a cube of 16 pixels and nine tentacles hung in a grid, at 4.5 times its model, and opens its mouth
// when the server says it is about to shoot.
func TestGhastModel(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(61, 56, 3.5, 4, 3.5, 0, 0, nil))
	e := g.net.entity(61)
	if e == nil || e.mob == nil || e.mob.m.kind.id != "Ghast" {
		t.Fatalf("ghast %+v", e)
	}
	e.mob.m.prev = e.mob.m.pos
	groups := map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	vs := groups["ghast/ghast"]
	if len(vs) != 10*24 {
		t.Fatalf("%d vertices, want %d", len(vs), 10*24)
	}
	lo, hi := standBounds(vs)
	// the body: 16 pixels across at 4.5 times, from 9.6 + 8 - 8 pixels below the top of the model
	if !near(float64(hi[0]-lo[0]), 4.5, 0.2) { // the tentacles at the corners stay inside the body's width
		t.Fatalf("ghast is %v wide, want about 4.5", hi[0]-lo[0])
	}
	if want := 4 + 4.5*(1.5078125-(8+9.6-8)/16); !near(float64(hi[1]), want, 1e-3) {
		t.Fatalf("top at %v, want %v", hi[1], want)
	}
	if w, h := e.mob.dims(); w != 4 || h != 4 {
		t.Fatalf("hit box %v x %v", w, h)
	}
	apply(metaPacket(61, metaByte(16, 1)))
	groups = map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	if len(groups["ghast/ghast_shooting"]) == 0 || len(groups["ghast/ghast"]) != 0 {
		t.Fatal("a ghast about to shoot is not drawn with its shooting face")
	}
}

// The tentacles' lengths are Random(1660)'s: nine of 8 to 14 pixels, the same every time.
func TestGhastTentaclesAreSeeded(t *testing.T) {
	a, b := ghastModel(), ghastModel()
	lengths := map[float32]bool{}
	for i := 1; i < 10; i++ {
		la, lb := a.parts[i].boxes[0].h, b.parts[i].boxes[0].h
		if la != lb || la < 8 || la > 14 {
			t.Fatalf("tentacle %d is %v and %v", i, la, lb)
		}
		lengths[la] = true
	}
	// what java.util.Random(1660) gives for nextInt(7) + 8, nine times (computed with the game's own JDK)
	for i, want := range []float32{8, 13, 9, 11, 11, 10, 12, 9, 12} {
		if got := a.parts[1+i].boxes[0].h; got != want {
			t.Fatalf("tentacle %d is %v long, want %v", i, got, want)
		}
	}
	if len(lengths) < 3 {
		t.Fatalf("the tentacles are %d lengths, a seeded Random gives more variety", len(lengths))
	}
	// the grid ModelGhast lays them in: three columns, the middle row shifted by half a step, three rows in z
	for j := 0; j < 9; j++ {
		wx := (float32(j%3)-float32(j/3%2)*0.5+0.25)/2*2 - 1
		wz := float32(j/3)/2*2 - 1
		if p := a.parts[1+j]; p.px != wx*5 || p.pz != wz*5 {
			t.Fatalf("tentacle %d hangs at (%v, %v), want (%v, %v)", j, p.px, p.pz, wx*5, wz*5)
		}
	}
}

// The slices of the magma cube are on the sheet where ModelMagmaCube puts them (two of them elsewhere), each a pixel
// high at 16 + i, and the core is the 4-pixel cube at 18.
func TestMagmaCubeSlices(t *testing.T) {
	m := magmaModel()
	for i, uv := range [][2]float32{{0, 0}, {0, 1}, {24, 10}, {24, 19}, {0, 4}, {0, 5}, {0, 6}, {0, 7}} {
		b := m.parts[i].boxes[0]
		if b.u != uv[0] || b.v != uv[1] || b.y != float32(16+i) || b.h != 1 || b.w != 8 || b.d != 8 || b.x != -4 || b.z != -4 {
			t.Fatalf("slice %d: %+v, want uv %v at y %d", i, b, uv, 16+i)
		}
	}
	if c := m.parts[8].boxes[0]; c.u != 0 || c.v != 16 || c.x != -2 || c.y != 18 || c.z != -2 || c.w != 4 || c.h != 4 || c.d != 4 {
		t.Fatalf("core %+v", c)
	}
}

// A snowman is five boxes shrunk by half a pixel, on ModelSnowMan's pivots.
func TestSnowmanModelParts(t *testing.T) {
	m := snowmanModel()
	for i, want := range []struct {
		u, v, x, y, z, w, h, d float32
		py                     float32
	}{
		{0, 0, -4, -8, -4, 8, 8, 8, 4}, {32, 0, -1, 0, -1, 12, 2, 2, 6}, {32, 0, -1, 0, -1, 12, 2, 2, 6},
		{0, 16, -5, -10, -5, 10, 10, 10, 13}, {0, 36, -6, -12, -6, 12, 12, 12, 24},
	} {
		b := m.parts[i].boxes[0]
		if b.u != want.u || b.v != want.v || b.x != want.x || b.y != want.y || b.z != want.z || b.w != want.w || b.h != want.h || b.d != want.d ||
			b.expand != -0.5 || m.parts[i].py != want.py || m.parts[i].px != 0 || m.parts[i].pz != 0 {
			t.Fatalf("part %d: %+v pivot (%v, %v, %v)", i, b, m.parts[i].px, m.parts[i].py, m.parts[i].pz)
		}
	}
}

// The head follows the look, the body a quarter of the way, and the arms are laid out from the body's turn: one to each
// side, 5 pixels from the middle, turned a half turn apart.
func TestSnowmanPose(t *testing.T) {
	r := snowmanPose(nil, 0, 0, 0, 40, 10)
	if !near(float64(r[0].y), float64(40*deg), 1e-6) || !near(float64(r[0].x), float64(10*deg), 1e-6) {
		t.Fatalf("head %+v", r[0])
	}
	by := 40 * deg * 0.25
	if !near(float64(r[3].y), float64(by), 1e-6) || r[4] != (rot{}) {
		t.Fatalf("body %+v bottom %+v", r[3], r[4])
	}
	f, f1 := sin32(by), cos32(by)
	if r[1].z != 1 || r[2].z != -1 || !near(float64(r[1].y), float64(by), 1e-6) || !near(float64(r[2].y), float64(pi+by), 1e-5) {
		t.Fatalf("arms turn %+v %+v", r[1], r[2])
	}
	if !near(float64(r[1].dx), float64(f1*5), 1e-5) || !near(float64(r[1].dz), float64(-f*5), 1e-5) ||
		!near(float64(r[2].dx), float64(-f1*5), 1e-5) || !near(float64(r[2].dz), float64(f*5), 1e-5) {
		t.Fatalf("arm pivots %+v %+v", r[1], r[2])
	}
}

// The pumpkin is on its head unless metadata says it was taken off; it is 0.625 of a block across and sits 0.34375
// above the head's pivot.
func TestSnowmanPumpkin(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(70, 97, 3.5, 4, 3.5, 0, 0, nil))
	e := g.net.entity(70)
	if e == nil || e.mob == nil || e.mob.m.kind.id != "SnowMan" || e.mob.m.sheared {
		t.Fatalf("snowman %+v", e)
	}
	e.mob.m.prev = e.mob.m.pos
	groups := map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	if n := len(groups["snowman"]); n != 5*24 {
		t.Fatalf("%d vertices for the body, want %d", n, 5*24)
	}
	vs := groups["!blocks"]
	if len(vs) != 24 {
		t.Fatalf("%d vertices for the pumpkin, want 24", len(vs))
	}
	lo, hi := standBounds(vs)
	centre := 4 + 1.5078125 - 4.0/16 + 0.34375 // the head's pivot, and the pumpkin's middle above it
	for i, c := range [3]float64{3.5, centre, 3.5} {
		if mid := float64(lo[i]+hi[i]) / 2; !near(mid, c, 1e-3) || !near(float64(hi[i]-lo[i]), 0.625, 1e-3) {
			t.Fatalf("axis %d: pumpkin %v..%v, want 0.625 across around %v", i, lo[i], hi[i], c)
		}
	}
	// the carved face is the one side with a texture of its own (the other three share one): it is on the front, the
	// side the snowman faces (south, +z, for a mob at yaw 0), so its four corners are on that side of the middle
	count := map[int]int{}
	qs := g.res.fast[86<<4|2].variants[0].quads
	for i := range qs {
		if qs[i].dir != dUp && qs[i].dir != dDown {
			count[qs[i].tile]++
		}
	}
	carved := -1
	for i := range qs {
		if qs[i].dir != dUp && qs[i].dir != dDown && count[qs[i].tile] == 1 {
			carved = i
		}
	}
	if carved < 0 {
		t.Fatal("the pumpkin has no carved face")
	}
	// find that face among what was drawn by its texture corners, whichever way the model was turned
	var sumZ float32
	n := 0
	for i := 0; i+4 <= len(vs); i += 4 {
		if vs[i].U == qs[carved].auv[0][0] && vs[i].V == qs[carved].auv[0][1] {
			for _, v := range vs[i : i+4] {
				sumZ += v.Z
			}
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the carved face was drawn %d times", n)
	}
	if mid := float64(sumZ / 4); mid < 3.5+0.25 {
		t.Fatalf("the carved face is at z %v: not on the front (+z) side of a snowman at z 3.5", mid)
	}
	apply(metaPacket(70, metaByte(16, 0))) // the pumpkin taken off
	groups = map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	if len(groups["!blocks"]) != 0 || len(groups["snowman"]) != 5*24 {
		t.Fatalf("without the pumpkin: %d block vertices", len(groups["!blocks"]))
	}
}
