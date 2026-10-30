package client

import (
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Particles, after 1.8's EffectRenderer: the bits that fly off a broken block (EntityDiggingFX), the flame
// and smoke of a torch (EntityFlameFX, EntitySmokeFX) and the sparks of a critical hit (EntityCrit2FX).
// They live in a fixed pool (no allocation while playing) and are drawn as camera-facing quads in one
// batch per texture: the block atlas for the pieces of a block, the particle sheet for the rest.

const (
	maxParticles = 2048
	sheetCells   = 16 // particles.png is a 16 x 16 grid of sprites
)

type particleKind uint8

const (
	pDig particleKind = iota
	pFlame
	pSmoke
	pCrit
)

type particle struct {
	pos, prev, vel vec3
	age, max       int32
	kind           particleKind
	onGround       bool
	scale          float32 // the base size; half the width of the quad is 0.1 of it
	tile           int32   // dig: the atlas tile; the others: the sprite's index in the sheet
	ju, jv         float32 // dig: which quarter of the tile, 0..3
	gravity        float32
	rgb            [3]float32
}

// critEmitter is EntityParticleEmitter: a few ticks of sparks around a creature that was hit hard.
type critEmitter struct {
	at    vec3
	w, h  float32
	ticks int
}

type particleSystem struct {
	list     []particle // the live ones; its capacity is the pool
	next     int        // where the next one goes when the pool is full: the oldest are replaced first
	emitters []critEmitter
	scratch  []aabb // the boxes of the block a particle is in, kept between calls
	// vertices rebuilt each frame into storage that is kept: one batch for the block atlas, one for the sheet
	atlasVerts, sheetVerts []vertex
	mesh                   [2]entityMesh
}

// particles is the pool, made on first use.
func (g *game) particles() *particleSystem {
	if g.parts == nil {
		g.parts = newParticleSystem()
	}
	return g.parts
}

func newParticleSystem() *particleSystem {
	return &particleSystem{list: make([]particle, 0, maxParticles), emitters: make([]critEmitter, 0, 8),
		atlasVerts: make([]vertex, 0, 4*maxParticles), sheetVerts: make([]vertex, 0, 4*maxParticles)}
}

func (ps *particleSystem) add(p particle) {
	if len(ps.list) < cap(ps.list) {
		ps.list = append(ps.list, p)
		return
	}
	ps.list[ps.next%len(ps.list)] = p
	ps.next++
}

func rnd() float32 { return rng.Float32() }

// speedFrom is EntityFX's constructor: the given motion plus a random shake, scaled to a random speed.
func speedFrom(m vec3) vec3 {
	m = vec3{m[0] + float64((rnd()*2-1)*0.4), m[1] + float64((rnd()*2-1)*0.4), m[2] + float64((rnd()*2-1)*0.4)}
	f := (rnd() + rnd() + 1) * 0.15
	n := float32(math.Sqrt(float64(m.dot(m))))
	if n < 1e-6 {
		return vec3{}
	}
	return m.scale(float64(f / n * 0.4))
}

// blockParticleTile is the atlas tile whose pieces fly off a broken block, and whether it takes the
// biome tint: the side texture of the block, as its model has it.
func (r *resources) blockParticleTile(st uint16) (tile int, tinted bool, ok bool) {
	if st == 0 || st >= 4096 {
		return 0, false, false
	}
	e := &r.fast[st]
	vs := e.variants
	if e.dep {
		file, props := r.stateOf(st>>4, int(st&15), func(int32, int32, int32) uint16 { return 0 })
		vs = r.pick(file, props)
	}
	if len(vs) == 0 || len(vs[0].quads) == 0 {
		return 0, false, false
	}
	q := &vs[0].quads[0]
	for i := range vs[0].quads {
		if vs[0].quads[i].dir == dNorth {
			q = &vs[0].quads[i]
			break
		}
	}
	return q.tile, q.tint >= 0, true
}

// blockBreakEffects is EffectRenderer.addBlockDestroyEffects: a 4 x 4 x 4 cloud of pieces of the block.
func (g *game) blockBreakEffects(x, y, z int32, st uint16) {
	ps := g.particles()
	tile, tinted, ok := g.res.blockParticleTile(st)
	if !ok || isFluid(st>>4) {
		return
	}
	rgb := [3]float32{0.6, 0.6, 0.6}
	if tinted {
		t := g.res.tintFor(st>>4, int(st&15), 1)
		rgb = [3]float32{0.6 * t[0], 0.6 * t[1], 0.6 * t[2]}
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			for k := 0; k < 4; k++ {
				c := vec3{float64((float32(i) + 0.5) / 4), float64((float32(j) + 0.5) / 4), float64((float32(k) + 0.5) / 4)}
				p := vec3{float64(x) + c[0], float64(y) + c[1], float64(z) + c[2]}
				ps.add(particle{pos: p, prev: p, vel: speedFrom(c.sub(vec3{0.5, 0.5, 0.5})), kind: pDig,
					scale: (rnd()*0.5 + 0.5), tile: int32(tile), ju: rnd() * 3, jv: rnd() * 3, gravity: 1,
					max: int32(4 / (rnd()*0.9 + 0.1)), rgb: rgb})
			}
		}
	}
}

// blockHitEffect is addBlockHitEffects: one piece off the face being mined, smaller and slower.
func (g *game) blockHitEffect(pos [3]int32, side int, st uint16) {
	ps := g.particles()
	tile, tinted, ok := g.res.blockParticleTile(st)
	if !ok {
		return
	}
	rgb := [3]float32{0.6, 0.6, 0.6}
	if tinted {
		t := g.res.tintFor(st>>4, int(st&15), 1)
		rgb = [3]float32{0.6 * t[0], 0.6 * t[1], 0.6 * t[2]}
	}
	p := vec3{float64(pos[0]) + float64(rnd()*0.8) + 0.1, float64(pos[1]) + float64(rnd()*0.8) + 0.1, float64(pos[2]) + float64(rnd()*0.8) + 0.1}
	d := dirVec[side]
	for a := 0; a < 3; a++ { // on the face, a tenth outside it
		switch d[a] {
		case 1:
			p[a] = float64(pos[a]) + 1.1
		case -1:
			p[a] = float64(pos[a]) - 0.1
		}
	}
	ps.add(particle{pos: p, prev: p, vel: speedFrom(vec3{}).scale(0.2), kind: pDig, scale: (rnd()*0.5 + 0.5) * 0.6,
		tile: int32(tile), ju: rnd() * 3, jv: rnd() * 3, gravity: 1, max: int32(4 / (rnd()*0.9 + 0.1)), rgb: rgb})
}

// critEffect starts the sparks of a critical hit on a creature whose box is w wide and h high at feet.
func (g *game) critEffect(feet vec3, w, h float32) {
	ps := g.particles()
	if len(ps.emitters) == cap(ps.emitters) {
		return
	}
	ps.emitters = append(ps.emitters, critEmitter{at: feet, w: w, h: h, ticks: 3})
}

func (ps *particleSystem) spawnCrit(e critEmitter) {
	for i := 0; i < 16; i++ {
		d := vec3{float64(rnd()*2 - 1), float64(rnd()*2 - 1), float64(rnd()*2 - 1)}
		if d.dot(d) > 1 {
			continue
		}
		at := vec3{e.at[0] + d[0]*float64(e.w)/4, e.at[1] + float64(e.h)/2 + d[1]*float64(e.h)/4, e.at[2] + d[2]*float64(e.w)/4}
		// EntityCrit2FX: the random shake of EntityFX times 0.1, then the given speed times 0.4.
		v := speedFrom(vec3{}).scale(0.1).add(vec3{d[0], d[1] + 0.2, d[2]}.scale(0.4))
		g := rnd()*0.3 + 0.6
		ps.add(particle{pos: at, prev: at, vel: v, kind: pCrit, scale: 0.75 * (rnd()*0.5 + 0.5) * 2, tile: 65,
			max: int32(6 / (float64(rnd())*0.8 + 0.2)), rgb: [3]float32{g, g, g}})
	}
}

// torchEffect is BlockTorch.randomDisplayTick: smoke and a flame at the top of the torch.
func (g *game) torchEffect(x, y, z int32, meta int) {
	ps := g.particles()
	px, py, pz := float32(x)+0.5, float32(y)+0.7, float32(z)+0.5
	switch meta {
	case 1:
		px, py = px-0.27, py+0.22
	case 2:
		px, py = px+0.27, py+0.22
	case 3:
		pz, py = pz-0.27, py+0.22
	case 4:
		pz, py = pz+0.27, py+0.22
	}
	p := vec3{float64(px), float64(py), float64(pz)}
	grey := rnd() * 0.3
	ps.add(particle{pos: p, prev: p, vel: speedFrom(vec3{}).scale(0.1), kind: pSmoke, scale: 0.75 * (rnd()*0.5 + 0.5) * 2, tile: 7,
		max: int32(8 / (float64(rnd())*0.8 + 0.2)), rgb: [3]float32{grey, grey, grey}})
	ps.add(particle{pos: p, prev: p, vel: speedFrom(vec3{}).scale(0.01), kind: pFlame, scale: (rnd()*0.5 + 0.5) * 2, tile: 48,
		max: int32(8/(float64(rnd())*0.8+0.2)) + 4, rgb: [3]float32{1, 1, 1}})
}

// tickParticles runs once a tick: the torches near the player drip their fire, and everything moves on.
func (g *game) tickParticles() {
	ps := g.particles()
	g.torchSample()
	for i := 0; i < len(ps.emitters); {
		ps.spawnCrit(ps.emitters[i])
		if ps.emitters[i].ticks--; ps.emitters[i].ticks <= 0 {
			ps.emitters[i] = ps.emitters[len(ps.emitters)-1]
			ps.emitters = ps.emitters[:len(ps.emitters)-1]
			continue
		}
		i++
	}
	for i := 0; i < len(ps.list); {
		p := &ps.list[i]
		if g.stepParticle(p) {
			i++
			continue
		}
		ps.list[i] = ps.list[len(ps.list)-1]
		ps.list = ps.list[:len(ps.list)-1]
	}
}

// torchSample picks 1000 spots within 16 blocks of the player, as World.doVoidFogParticles does, and
// lets any torch there show itself: a torch makes a flame and some smoke about twice a second.
func (g *game) torchSample() {
	if g.opt.Bench > 0 {
		return
	}
	cx, cy, cz := int32(math.Floor(float64(g.pos[0]))), int32(math.Floor(float64(g.pos[1]))), int32(math.Floor(float64(g.pos[2])))
	for i := 0; i < 1000; i++ {
		x, y, z := cx+int32(rng.Intn(33))-16, cy+int32(rng.Intn(33))-16, cz+int32(rng.Intn(33))-16
		if y < 0 || y > 255 {
			continue
		}
		if st := g.world.Block(x, y, z); st>>4 == 50 {
			g.torchEffect(x, y, z, int(st&15))
		}
	}
}

// stepParticle ages and moves one particle (EntityFX.onUpdate and its subclasses); false when it is gone.
func (g *game) stepParticle(p *particle) bool {
	p.prev = p.pos
	if p.age++; p.age >= p.max {
		return false
	}
	switch p.kind {
	case pDig:
		p.vel[1] -= 0.04 * float64(p.gravity)
	case pSmoke:
		p.tile = int32(7 - p.age*8/p.max)
		p.vel[1] += 0.004
	case pCrit:
		p.rgb[1] *= 0.96
		p.rgb[2] *= 0.9
		p.vel[1] -= 0.02
	}
	// A particle is stopped by opaque blocks, axis by axis (y first), without any allocation.
	var moved vec3
	for _, axis := range [3]int{1, 0, 2} {
		next := p.pos
		next[axis] += p.vel[axis]
		if g.particleSolid(next) {
			moved[axis] = 0
			continue
		}
		moved[axis] = p.vel[axis]
		p.pos = next
	}
	p.onGround = moved[1] != p.vel[1] && p.vel[1] < 0
	for a := 0; a < 3; a++ {
		if moved[a] != p.vel[a] {
			p.vel[a] = 0
		}
	}
	drag := float32(0.98)
	switch p.kind {
	case pFlame, pSmoke:
		drag = 0.96
	case pCrit:
		drag = 0.7
	}
	if p.kind == pSmoke && p.pos[1] == p.prev[1] { // blocked above: it spreads along the ceiling
		p.vel[0], p.vel[2] = p.vel[0]*1.1, p.vel[2]*1.1
	}
	p.vel = p.vel.scale(float64(drag))
	if p.onGround {
		p.vel[0], p.vel[2] = p.vel[0]*0.7, p.vel[2]*0.7
	}
	return true
}

// buildParticles lays every live particle out as a quad facing the camera, into the two batches. light
// gives the light byte at a position. partial is how far into the tick this frame is.
func (ps *particleSystem) build(res *resources, right, up vec3, partial float32, light func(vec3) uint8) {
	ps.atlasVerts, ps.sheetVerts = ps.atlasVerts[:0], ps.sheetVerts[:0]
	for i := range ps.list {
		p := &ps.list[i]
		pos := p.prev.add(p.pos.sub(p.prev).scale(float64(partial)))
		size := 0.1 * p.scale
		rgb := p.rgb
		l := uint8(0xFF) // fire does not need light
		switch p.kind {
		case pDig:
			l = light(pos)
		case pSmoke:
			f := (float32(p.age) + partial) / float32(p.max) * 32
			size *= min(max(f, 0), 1)
			l = light(pos)
		case pFlame:
			f := (float32(p.age) + partial) / float32(p.max)
			size *= 1 - f*f*0.5
		}
		var u0, v0, u1, v1 float32
		if p.kind == pDig {
			tile := int(p.tile)
			u0, v0 = res.atlas.coord(tile, float32(int(p.ju))*4, float32(int(p.jv))*4)
			u1, v1 = res.atlas.coord(tile, float32(int(p.ju))*4+4, float32(int(p.jv))*4+4)
		} else {
			cell := float32(1) / sheetCells
			u0, v0 = float32(int(p.tile)%sheetCells)*cell, float32(int(p.tile)/sheetCells)*cell
			u1, v1 = u0+cell, v0+cell
		}
		c := [4]uint8{toByte(rgb[0]), toByte(rgb[1]), toByte(rgb[2]), l}
		r, u := right.scale(float64(size)), up.scale(float64(size))
		a, b, cc, d := pos.sub(r).sub(u), pos.sub(r).add(u), pos.add(r).add(u), pos.add(r).sub(u)
		dst := &ps.sheetVerts
		if p.kind == pDig {
			dst = &ps.atlasVerts
		}
		*dst = append(*dst,
			vertex{float32(a[0]), float32(a[1]), float32(a[2]), u0, v1, c}, vertex{float32(b[0]), float32(b[1]), float32(b[2]), u0, v0, c},
			vertex{float32(cc[0]), float32(cc[1]), float32(cc[2]), u1, v0, c}, vertex{float32(d[0]), float32(d[1]), float32(d[2]), u1, v1, c})
	}
}

// drawParticles draws the particles over the world, as the mobs are: no culling, the same program.
func (g *game) drawParticles() {
	ps := g.parts
	if ps == nil || len(ps.list) == 0 {
		return
	}
	dir := look(g.yaw, g.pitch)
	right := dir.cross(vec3{0, 1, 0}).norm()
	up := right.cross(dir).norm()
	ps.build(g.res, right, up, g.partial, func(p vec3) uint8 {
		return g.world.Light(int32(math.Floor(float64(p[0]))), int32(math.Floor(float64(p[1]))), int32(math.Floor(float64(p[2]))))
	})
	gl.Disable(gl.CULL_FACE)
	if len(ps.atlasVerts) > 0 {
		gl.BindTexture(gl.TEXTURE_2D, g.tex)
		ps.mesh[0].draw(ps.atlasVerts)
	}
	if tex, ok := g.mobTex["particles"]; ok && len(ps.sheetVerts) > 0 {
		gl.BindTexture(gl.TEXTURE_2D, tex)
		ps.mesh[1].draw(ps.sheetVerts)
		gl.BindTexture(gl.TEXTURE_2D, g.tex)
	}
	gl.Enable(gl.CULL_FACE)
}

// blockEffects is what a changed block shows: the pieces of one that was broken. (Sound joins it in sound.go.)
func (g *game) blockEffects(x, y, z int32, old, state uint16) {
	if g.opt.Screenshot != "" || g.opt.Bench > 0 {
		return
	}
	if old != 0 && state == 0 {
		g.blockBreakEffects(x, y, z, old)
	}
	g.blockSounds(x, y, z, old, state)
}

// demoParticles fills the scene of a screenshot (-screen particles): a stone block breaking, a torch's
// fire and a critical hit, a few ticks old.
func (g *game) demoParticles() {
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	at := g.pos.add(f.scale(3))
	bx, by, bz := int32(math.Floor(float64(at[0]-right[0]*1.2))), int32(math.Floor(float64(at[1]-1.4))), int32(math.Floor(float64(at[2]-right[2]*1.2)))
	g.blockBreakEffects(bx, by, bz, 1<<4) // stone
	g.blockBreakEffects(bx, by, bz, 2<<4) // grass: its side
	for i := 0; i < 40; i++ {
		g.torchEffect(int32(math.Floor(float64(at[0]+right[0]*0.5))), int32(math.Floor(float64(at[1]-1.3))), int32(math.Floor(float64(at[2]+right[2]*0.5))), 5)
	}
	g.critEffect(at.add(right.scale(1.6)).sub(vec3{0, 1.6, 0}), 0.6, 1.8)
	for i := 0; i < 4; i++ {
		g.tickParticles()
	}
}

// particleSolid is whether a point lies inside the collision box of a block.
func (g *game) particleSolid(p vec3) bool {
	y := int32(math.Floor(float64(p[1])))
	if y < 0 || y > 255 {
		return false
	}
	ps := g.parts
	ps.scratch = g.blockBoxes(int32(math.Floor(float64(p[0]))), y, int32(math.Floor(float64(p[2]))), ps.scratch[:0])
	for _, b := range ps.scratch {
		if p[0] >= b.min[0] && p[0] < b.max[0] && p[1] >= b.min[1] && p[1] < b.max[1] && p[2] >= b.min[2] && p[2] < b.max[2] {
			return true
		}
	}
	return false
}
