package client

import (
	"math"
	"testing"
)

func particleGame(t *testing.T) *game {
	g := testGame(t)
	g.pos = vec3{0.5, 5.62, 0.5}
	g.feet = vec3{0.5, 4, 0.5}
	return g
}

// A broken block throws 4 x 4 x 4 pieces; they fall, rest on the ground and are gone within 40 ticks.
func TestBreakingABlockThrowsSixtyFourPieces(t *testing.T) {
	g := particleGame(t)
	g.blockBreakEffects(3, 4, 3, 1<<4) // stone, standing on the flat world's surface
	ps := g.particles()
	if len(ps.list) != 64 {
		t.Fatalf("%d pieces, want 64", len(ps.list))
	}
	low := 100.0
	for i := 0; i < 12; i++ {
		g.tickParticles()
		for _, p := range ps.list {
			low = min(low, p.pos[1])
		}
	}
	if low < 3.99 { // the ground is the top of the block layer at y=3, which is height 4
		t.Fatalf("a piece sank to y=%v, through the ground", low)
	}
	for i := 0; i < 40; i++ {
		g.tickParticles()
	}
	if len(ps.list) != 0 {
		t.Fatalf("%d pieces still alive after 52 ticks", len(ps.list))
	}
}

// The pool is fixed: past its size the oldest are replaced, it never grows.
func TestParticlePoolIsFixed(t *testing.T) {
	g := particleGame(t)
	for i := 0; i < 5000; i++ {
		g.torchEffect(2, 5, 2, 5)
	}
	ps := g.particles()
	if len(ps.list) != maxParticles || cap(ps.list) != maxParticles {
		t.Fatalf("pool %d (cap %d), want %d", len(ps.list), cap(ps.list), maxParticles)
	}
}

// Building the quads and moving the particles on do not allocate: the batches are kept between frames.
func TestParticlesDoNotAllocate(t *testing.T) {
	g := particleGame(t)
	for i := 0; i < 20; i++ {
		g.blockBreakEffects(int32(i), 4, 0, 1<<4)
	}
	for i := 0; i < 200; i++ {
		g.torchEffect(2, 5, 2, 5)
	}
	ps := g.particles()
	right, up := vec3{1, 0, 0}, vec3{0, 1, 0}
	light := func(vec3) uint8 { return 0xF0 }
	ps.build(g.res, right, up, 0.5, light) // the first build sizes the batches
	if n := testing.AllocsPerRun(20, func() { ps.build(g.res, right, up, 0.5, light) }); n != 0 {
		t.Fatalf("building the quads allocated %v times a frame", n)
	}
	if len(ps.atlasVerts) == 0 || len(ps.sheetVerts) == 0 {
		t.Fatalf("batches: %d atlas vertices, %d sheet vertices", len(ps.atlasVerts), len(ps.sheetVerts))
	}
	if len(ps.atlasVerts)%4 != 0 || len(ps.sheetVerts)%4 != 0 {
		t.Fatal("a batch is not whole quads")
	}
	if n := testing.AllocsPerRun(20, g.tickParticles); n != 0 {
		t.Fatalf("a tick of particles allocated %v times", n)
	}
}

// A piece of a block shows a quarter of its tile, the sprite of the sheet a whole cell.
func TestParticleTextureCoordinates(t *testing.T) {
	g := particleGame(t)
	ps := g.particles()
	ps.add(particle{pos: vec3{1, 1, 1}, prev: vec3{1, 1, 1}, kind: pDig, scale: 1, tile: 5, ju: 2.7, jv: 0.2, max: 10, rgb: [3]float32{1, 1, 1}})
	ps.add(particle{pos: vec3{2, 1, 1}, prev: vec3{2, 1, 1}, kind: pCrit, scale: 1, tile: 65, max: 10, rgb: [3]float32{1, 1, 1}})
	ps.build(g.res, vec3{1, 0, 0}, vec3{0, 1, 0}, 0, func(vec3) uint8 { return 0 })
	a := ps.atlasVerts
	cols := float32(g.res.atlas.cols)
	if w := (a[2].U - a[0].U) * cols * 16; math.Abs(float64(w)-4*(16-0.04)/16) > 0.01 { // 4 of 16 sixteenths of the tile, less the inset
		t.Fatalf("a block piece is %v pixels wide, want 4", w)
	}
	s := ps.sheetVerts
	if s[0].U != 1.0/16 || s[0].V != 5.0/16 || s[2].U != 2.0/16 || s[2].V != 4.0/16 { // sprite 65: column 1, row 4 (the first corner is bottom left)
		t.Fatalf("sprite 65 at %v,%v to %v,%v", s[0].U, s[0].V, s[2].U, s[2].V)
	}
}

// A torch shows itself about as often as the game does: 1000 spots in 33^3 each tick, so a torch within
// range makes a flame about once every 36 ticks.
func TestTorchesShowTheirFire(t *testing.T) {
	g := particleGame(t)
	g.world.SetBlock(2, 5, 2, 50<<4|5)
	g.pos = vec3{2.5, 5.6, 2.5}
	ps := g.particles()
	flames := 0
	for i := 0; i < 3600; i++ { // three minutes
		g.torchSample()
		for _, p := range ps.list {
			if p.kind == pFlame {
				flames++
			}
		}
		ps.list = ps.list[:0]
	}
	rate := float64(flames) / 3600 * 20 // per second
	if rate < 0.35 || rate > 0.85 {
		t.Fatalf("%.2f flames a second from one torch, want about 0.56", rate)
	}
}
