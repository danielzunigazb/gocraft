package client

import (
	"math"
)

// Particles a server sends, after 1.8's client: Particle (0x2A) names one of EnumParticleTypes and is
// spread as NetHandlerPlayClient.handleParticles spreads it; RenderGlobal.spawnEntityFX drops those more than
// 16 blocks from the camera unless the packet says they are seen from far; each type is made as its EntityFX
// subclass makes it, and moves as that class's onUpdate moves it. Explosion (0x27) is the boom, the puffs
// and the blocks it took.

// The particle ids of 1.8 (EnumParticleTypes).
const (
	ptExplode          = 0
	ptLargeExplode     = 1
	ptHugeExplosion    = 2
	ptSplash           = 5
	ptCrit             = 9
	ptMagicCrit        = 10
	ptSmoke            = 11
	ptLargeSmoke       = 12
	ptSpell            = 13
	ptInstantSpell     = 14
	ptMobSpell         = 15
	ptMobSpellAmbient  = 16
	ptWitchMagic       = 17
	ptAngryVillager    = 20
	ptHappyVillager    = 21
	ptTownAura         = 22
	ptNote             = 23
	ptPortal           = 24
	ptEnchantmentTable = 25
	ptFlame            = 26
	ptLava             = 27
	ptCloud            = 29
	ptRedDust          = 30
	ptSnowballPoof     = 31
	ptSlime            = 33
	ptHeart            = 34
	ptIconCrack        = 36
	ptBlockCrack       = 37
	ptBlockDust        = 38
)

// particleArgs is how many VarInts follow a particle of the type: an item and its damage, or a block state.
func particleArgs(id int32) int {
	switch id {
	case ptIconCrack:
		return 2
	case ptBlockCrack, ptBlockDust:
		return 1
	}
	return 0
}

// The kinds of particle that come from servers, past the ones gocraft makes itself.
const (
	pReddust      particleKind = iota + 16 // EntityReddustFX
	pSpell                                 // EntitySpellFX: an animation of eight sprites from base
	pHeart                                 // EntityHeartFX, and the angry villager
	pAura                                  // EntityAuraFX: the town aura and the happy villager
	pNote                                  // EntityNoteFX
	pPortal                                // EntityPortalFX
	pEnchant                               // EntityEnchantmentTableParticleFX
	pCloud                                 // EntityCloudFX
	pExplode                               // EntityExplodeFX
	pLava                                  // EntityLavaFX
	pSplash                                // EntitySplashFX
	pLargeExplode                          // EntityLargeExplodeFX: frames of explosion.png
	pItemCrack                             // EntityBreakingFX: a piece of an item's icon
)

// hugeEmitter is EntityHugeExplodeFX: for eight ticks it throws six big puffs a tick around where it is.
type hugeEmitter struct {
	at  vec3
	age int
}

const hugeExplodeTicks = 8

// fxShake is the motion EntityFX's constructor gives: the given speed shaken by up to 0.4 on each axis, set
// to a random length, and lifted a tenth.
func fxShake(m vec3) vec3 {
	m = vec3{m[0] + (rng.Float64()*2-1)*0.4000000059604645, m[1] + (rng.Float64()*2-1)*0.4000000059604645, m[2] + (rng.Float64()*2-1)*0.4000000059604645}
	f := float32(rng.Float64()+rng.Float64()+1) * 0.15
	n := float32(math.Sqrt(m.dot(m)))
	if n == 0 {
		return vec3{0, 0.10000000149011612, 0}
	}
	k := float64(f) / float64(n) * 0.4000000059604645
	return vec3{m[0] * k, m[1]*k + 0.10000000149011612, m[2] * k}
}

// fxScale is EntityFX's random size, (0.5..1) * 2.
func fxScale() float32 { return (rnd()*0.5 + 0.5) * 2 }

// netParticles is a Particle packet: one particle moving at speed times the offsets when the count is 0,
// else count of them spread by a normal distribution over the offsets, each with a random speed.
func (g *game) netParticles(id int32, far bool, at, off vec3, speed float32, count int32, args []int32) {
	if count == 0 {
		g.spawnFX(id, far, at, off.scale(float64(speed)), args)
		return
	}
	for i := int32(0); i < min(count, 1000); i++ {
		p := vec3{at[0] + rng.NormFloat64()*off[0], at[1] + rng.NormFloat64()*off[1], at[2] + rng.NormFloat64()*off[2]}
		v := vec3{rng.NormFloat64() * float64(speed), rng.NormFloat64() * float64(speed), rng.NormFloat64() * float64(speed)}
		g.spawnFX(id, far, p, v, args)
	}
}

// spawnFX makes one particle of a type, as its factory does, unless it is more than 16 blocks from the
// camera and not one to be seen from far (RenderGlobal.spawnEntityFX).
func (g *game) spawnFX(id int32, far bool, at, v vec3, args []int32) {
	if d := at.sub(g.feet); !far && d.dot(d) > 256 {
		return
	}
	ps := g.particles()
	switch id {
	case ptExplode:
		g2 := rnd()*0.3 + 0.7
		ps.add(particle{pos: at, prev: at, kind: pExplode, vel: v.add(vec3{(rng.Float64()*2 - 1) * 0.05, (rng.Float64()*2 - 1) * 0.05, (rng.Float64()*2 - 1) * 0.05}),
			scale: rnd()*rnd()*6 + 1, max: int32(16/(rnd()*0.8+0.2)) + 2, tile: 7, rgb: [3]float32{g2, g2, g2}})
	case ptLargeExplode:
		g2 := rnd()*0.6 + 0.4
		ps.add(particle{pos: at, prev: at, kind: pLargeExplode, scale: 1 - float32(v[0])*0.5, max: 6 + int32(rng.Intn(4)), rgb: [3]float32{g2, g2, g2}})
	case ptHugeExplosion:
		if len(ps.huge) < cap(ps.huge) {
			ps.huge = append(ps.huge, hugeEmitter{at: at})
		}
	case ptSplash:
		p := particle{pos: at, prev: at, kind: pSplash, vel: fxShake(vec3{}).scale(0.3), scale: fxScale(), gravity: 0.04,
			tile: 20 + int32(rng.Intn(4)), max: int32(8 / (rnd()*0.8 + 0.2)), rgb: [3]float32{1, 1, 1}}
		p.vel[1] = float64(rnd()*0.2 + 0.1)
		if v[1] == 0 && (v[0] != 0 || v[2] != 0) {
			p.vel = vec3{v[0], v[1] + 0.1, v[2]}
		}
		ps.add(p)
	case ptCrit, ptMagicCrit:
		ps.add(critFX(at, v, id == ptMagicCrit))
	case ptSmoke, ptLargeSmoke:
		k := float32(1)
		if id == ptLargeSmoke {
			k = 2.5
		}
		grey := rnd() * 0.3
		ps.add(particle{pos: at, prev: at, kind: pSmoke, vel: fxShake(vec3{}).scale(0.1).add(v), scale: fxScale() * 0.75 * k, tile: 7,
			max: int32(float32(int32(8/(rnd()*0.8+0.2))) * k), rgb: [3]float32{grey, grey, grey}})
	case ptSpell, ptInstantSpell, ptMobSpell, ptMobSpellAmbient, ptWitchMagic:
		if id == ptMobSpellAmbient {
			return // drawn at 15% opacity in the game; gocraft's particles have no opacity, and a solid one would mislead
		}
		p := particle{pos: at, prev: at, kind: pSpell, vel: fxShake(vec3{0.5 - rng.Float64(), v[1], 0.5 - rng.Float64()}), scale: fxScale() * 0.75,
			base: 128, max: int32(8 / (rnd()*0.8 + 0.2)), rgb: [3]float32{1, 1, 1}}
		p.vel[1] *= 0.2
		if v[0] == 0 && v[2] == 0 {
			p.vel[0], p.vel[2] = p.vel[0]*0.1, p.vel[2]*0.1
		}
		switch id {
		case ptInstantSpell:
			p.base = 144
		case ptMobSpell:
			p.rgb = [3]float32{float32(v[0]), float32(v[1]), float32(v[2])}
		case ptWitchMagic:
			f := rnd()*0.5 + 0.35
			p.rgb = [3]float32{f, 0, f}
		}
		p.tile = p.base + 7
		ps.add(p)
	case ptAngryVillager, ptHeart:
		p := particle{pos: at, prev: at, kind: pHeart, vel: fxShake(vec3{}).scale(0.01), scale: fxScale() * 0.75 * 2, tile: 80, max: 16, rgb: [3]float32{1, 1, 1}}
		p.vel[1] += 0.1
		if id == ptAngryVillager {
			p.pos[1] += 0.5
			p.prev, p.tile = p.pos, 81
		}
		ps.add(p)
	case ptHappyVillager, ptTownAura:
		f := rnd()*0.1 + 0.2
		p := particle{pos: at, prev: at, kind: pAura, vel: fxShake(v).scale(0.019999999552965164), scale: fxScale() * (rnd()*0.6 + 0.5),
			max: int32(20 / (rng.Float64()*0.8 + 0.2)), rgb: [3]float32{f, f, f}}
		if id == ptHappyVillager {
			p.tile, p.rgb = 82, [3]float32{1, 1, 1}
		}
		ps.add(p)
	case ptNote:
		n := float32(v[0])
		col := func(o float32) float32 { return float32(math.Sin(float64((n+o)*math.Pi*2)))*0.65 + 0.35 }
		p := particle{pos: at, prev: at, kind: pNote, vel: fxShake(vec3{}).scale(0.01), scale: fxScale() * 0.75 * 2, tile: 64, max: 6,
			rgb: [3]float32{col(0), col(1.0 / 3), col(2.0 / 3)}}
		p.vel[1] += 0.2
		ps.add(p)
	case ptPortal:
		f := rnd()*0.6 + 0.4
		ps.add(particle{pos: at, prev: at, origin: at, kind: pPortal, vel: v, scale: rnd()*0.2 + 0.5, tile: int32(rng.Float64() * 8),
			max: int32(rng.Float64()*10) + 40, rgb: [3]float32{f * 0.9, f * 0.3, f}})
	case ptEnchantmentTable:
		f := rnd()*0.6 + 0.4
		ps.add(particle{pos: at, prev: at, origin: at, kind: pEnchant, vel: v, scale: rnd()*0.5 + 0.2, tile: int32(rng.Float64()*26 + 1 + 224),
			max: int32(rng.Float64()*10) + 30, rgb: [3]float32{f * 0.9, f * 0.9, f}})
	case ptFlame:
		p := particle{pos: at, prev: at, kind: pFlame, vel: fxShake(v).scale(0.01).add(v), scale: fxScale(), tile: 48,
			max: int32(8/(rng.Float64()*0.8+0.2)) + 4, rgb: [3]float32{1, 1, 1}}
		p.pos = p.pos.add(vec3{float64(rnd()-rnd()) * 0.05, float64(rnd()-rnd()) * 0.05, float64(rnd()-rnd()) * 0.05})
		p.prev = p.pos
		ps.add(p)
	case ptLava:
		p := particle{pos: at, prev: at, kind: pLava, vel: fxShake(vec3{}).scale(0.8), scale: fxScale() * (rnd()*2 + 0.2), tile: 49,
			max: int32(16 / (rng.Float64()*0.8 + 0.2)), rgb: [3]float32{1, 1, 1}}
		p.vel[1] = float64(rnd()*0.4 + 0.05)
		ps.add(p)
	case ptCloud:
		f := 1 - rnd()*0.3
		ps.add(particle{pos: at, prev: at, kind: pCloud, vel: fxShake(vec3{}).scale(0.1).add(v), scale: fxScale() * 0.75 * 2.5, tile: 7,
			max: int32(float32(int32(8/(rnd()*0.8+0.3))) * 2.5), rgb: [3]float32{f, f, f}})
	case ptRedDust:
		r, gr, b := float32(v[0]), float32(v[1]), float32(v[2])
		if r == 0 {
			r = 1
		}
		f := float32(rng.Float64())*0.4 + 0.6
		ps.add(particle{pos: at, prev: at, kind: pReddust, vel: fxShake(vec3{}).scale(0.1), scale: fxScale() * 0.75, tile: 7,
			max: int32(8 / (rng.Float64()*0.8 + 0.2)),
			rgb: [3]float32{(float32(rng.Float64()*0.2) + 0.8) * r * f, (float32(rng.Float64()*0.2) + 0.8) * gr * f, (float32(rng.Float64()*0.2) + 0.8) * b * f}})
	case ptSnowballPoof, ptSlime, ptIconCrack:
		item := slot{ID: 332, N: 1} // a snowball
		switch {
		case id == ptSlime:
			item.ID = 341
		case id == ptIconCrack && len(args) == 2:
			item = slot{ID: uint16(args[0]), Dmg: uint16(args[1]), N: 1}
		}
		tile, ok := g.res.itemTile(item)
		if !ok {
			return
		}
		vel := fxShake(vec3{})
		if id == ptIconCrack { // the factory with a speed: shaken a tenth, plus the speed
			vel = vel.scale(0.1).add(v)
		}
		ps.add(particle{pos: at, prev: at, kind: pItemCrack, vel: vel, scale: fxScale() / 2, tile: int32(tile), ju: rnd() * 3, jv: rnd() * 3,
			gravity: 1, max: int32(4 / (rnd()*0.9 + 0.1)), rgb: [3]float32{1, 1, 1}})
	case ptBlockCrack, ptBlockDust:
		if len(args) != 1 {
			return
		}
		st := uint16(args[0]&4095)<<4 | uint16(args[0]>>12&15) // the game's state id is the block id plus the meta times 4096
		tile, tinted, ok := g.res.blockParticleTile(st)
		if !ok || st>>4 == 0 {
			return
		}
		rgb := [3]float32{0.6, 0.6, 0.6}
		if tinted && st>>4 != 2 { // EntityDiggingFX tints all but grass by the biome
			t := g.res.tintFor(st>>4, int(st&15), 1)
			rgb = [3]float32{0.6 * t[0], 0.6 * t[1], 0.6 * t[2]}
		}
		vel := fxShake(v)
		if id == ptBlockDust { // EntityBlockDustFX keeps the given speed as it is
			vel = v
		}
		ps.add(particle{pos: at, prev: at, kind: pDig, vel: vel, scale: fxScale() / 2, tile: int32(tile), ju: rnd() * 3, jv: rnd() * 3,
			gravity: 1, max: int32(4 / (rnd()*0.9 + 0.1)), rgb: rgb})
	}
}

// tickHuge runs the explosions' emitters a tick.
func (g *game) tickHuge() {
	ps := g.parts
	for i := 0; i < len(ps.huge); {
		h := &ps.huge[i]
		for k := 0; k < 6; k++ {
			at := h.at.add(vec3{(rng.Float64() - rng.Float64()) * 4, (rng.Float64() - rng.Float64()) * 4, (rng.Float64() - rng.Float64()) * 4})
			g.spawnFX(ptLargeExplode, true, at, vec3{float64(float32(h.age) / hugeExplodeTicks), 0, 0}, nil)
		}
		if h.age++; h.age == hugeExplodeTicks {
			ps.huge[i] = ps.huge[len(ps.huge)-1]
			ps.huge = ps.huge[:len(ps.huge)-1]
			continue
		}
		i++
	}
}

// stepFX is onUpdate for the server's kinds of particle; false when it is gone.
func (g *game) stepFX(p *particle) bool {
	p.prev = p.pos
	switch p.kind {
	case pAura: // moves through blocks, then ages
		p.pos = p.pos.add(p.vel)
		p.vel = p.vel.scale(0.99)
		p.age++
		return p.age <= p.max
	case pPortal:
		f := float64(p.age) / float64(p.max)
		f1 := f
		f = 1 - (-f + f*f*2)
		p.pos = vec3{p.origin[0] + p.vel[0]*f, p.origin[1] + p.vel[1]*f + (1 - f1), p.origin[2] + p.vel[2]*f}
		p.age++
		return p.age <= p.max
	case pEnchant:
		f := 1 - float64(p.age)/float64(p.max)
		f1 := 1 - f
		f1 *= f1
		f1 *= f1
		p.pos = vec3{p.origin[0] + p.vel[0]*f, p.origin[1] + p.vel[1]*f - f1*1.2, p.origin[2] + p.vel[2]*f}
		p.age++
		return p.age <= p.max
	case pLargeExplode:
		p.age++
		return p.age < p.max
	case pSplash: // EntityRainFX.onUpdate counts its life down, and may end when it lands
		p.vel[1] -= float64(p.gravity)
		g.moveParticle(p)
		p.vel = p.vel.scale(0.98)
		if p.max <= 0 {
			return false
		}
		p.max--
		if p.onGround {
			if rnd() < 0.5 {
				return false
			}
			p.vel[0], p.vel[2] = p.vel[0]*0.7, p.vel[2]*0.7
		}
		return true
	}
	if p.age >= p.max {
		return false
	}
	p.age++
	drag, slide := 0.96, false
	switch p.kind {
	case pReddust:
		p.tile, slide = 7-p.age*8/p.max, true
	case pCloud:
		p.tile = 7 - p.age*8/p.max
	case pSpell:
		p.tile = p.base + 7 - p.age*8/p.max
		p.vel[1] += 0.004
		slide = true
	case pHeart:
		drag, slide = 0.86, true
	case pNote:
		drag, slide = 0.66, true
	case pExplode:
		p.tile = 7 - p.age*8/p.max
		p.vel[1] += 0.004
		drag = 0.8999999761581421
	case pLava:
		if rnd() > float32(p.age)/float32(p.max) {
			grey := rnd() * 0.3
			g.parts.add(particle{pos: p.pos, prev: p.pos, kind: pSmoke, vel: fxShake(vec3{}).scale(0.1).add(p.vel), scale: fxScale() * 0.75, tile: 7,
				max: int32(8 / (rnd()*0.8 + 0.2)), rgb: [3]float32{grey, grey, grey}})
		}
		p.vel[1] -= 0.03
		drag = 0.999
	case pItemCrack:
		p.vel[1] -= 0.04 * float64(p.gravity)
		drag = 0.98
	}
	y0 := p.pos[1]
	g.moveParticle(p)
	if slide && p.pos[1] == y0 { // stopped going up or down: it spreads sideways
		p.vel[0], p.vel[2] = p.vel[0]*1.1, p.vel[2]*1.1
	}
	p.vel = p.vel.scale(drag)
	if p.onGround {
		p.vel[0], p.vel[2] = p.vel[0]*0.7, p.vel[2]*0.7
	}
	return true
}

// buildFX lays out one of the server's kinds of particle: its size for its age, its light and its batch.
func (ps *particleSystem) buildFX(res *resources, p *particle, pos, right, up vec3, partial float32, light func(vec3) uint8) {
	size := 0.1 * p.scale
	life := (float32(p.age) + partial) / float32(p.max)
	l := light(pos)
	switch p.kind {
	case pReddust, pSpell, pHeart, pNote, pCloud: // they grow to their size in the first thirty-second of their life
		size *= min(max(life*32, 0), 1)
	case pPortal, pEnchant:
		f := 1 - life
		size *= 1 - f*f
	case pLava:
		size *= 1 - life*life
		l = 0xFF
	case pLargeExplode:
		frame := int((float32(p.age) + partial) * 15 / float32(p.max))
		if frame > 15 {
			return
		}
		size = 2 * p.scale
		u0, v0 := float32(frame%4)/4, float32(frame/4)/4
		ps.boomVerts = appendQuad(ps.boomVerts, pos, right, up, size, u0, v0, u0+0.24975, v0+0.24975, p.rgb, 0xF0)
		return
	case pItemCrack:
		tile := int(p.tile)
		u0, v0 := res.items.coord(tile, float32(int(p.ju))*4, float32(int(p.jv))*4)
		u1, v1 := res.items.coord(tile, float32(int(p.ju))*4+4, float32(int(p.jv))*4+4)
		ps.itemVerts = appendQuad(ps.itemVerts, pos, right, up, size, u0, v0, u1, v1, p.rgb, l)
		return
	}
	cell := float32(1) / sheetCells
	u0, v0 := float32(int(p.tile)%sheetCells)*cell, float32(int(p.tile)/sheetCells)*cell
	ps.sheetVerts = appendQuad(ps.sheetVerts, pos, right, up, size, u0, v0, u0+cell, v0+cell, p.rgb, l)
}

// appendQuad adds a square facing the camera, size from its middle to each side.
func appendQuad(dst []vertex, pos, right, up vec3, size, u0, v0, u1, v1 float32, rgb [3]float32, l uint8) []vertex {
	c := [4]uint8{toByte(rgb[0]), toByte(rgb[1]), toByte(rgb[2]), l}
	r, u := right.scale(float64(size)), up.scale(float64(size))
	a, b, cc, d := pos.sub(r).sub(u), pos.sub(r).add(u), pos.add(r).add(u), pos.add(r).sub(u)
	return append(dst,
		vertex{float32(a[0]), float32(a[1]), float32(a[2]), u0, v1, c}, vertex{float32(b[0]), float32(b[1]), float32(b[2]), u0, v0, c},
		vertex{float32(cc[0]), float32(cc[1]), float32(cc[2]), u1, v0, c}, vertex{float32(d[0]), float32(d[1]), float32(d[2]), u1, v1, c})
}

// netExplosion is Explosion (Explosion.doExplosionB on the client): the boom, a huge or a large puff, the
// blocks it took gone (each throwing a puff and smoke out from the middle), and the push it gave the player.
func (g *game) netExplosion(at vec3, strength float32, blocks [][3]int32, push vec3) {
	g.sound("random.explode", at, 4, (1+(rnd()-rnd())*0.2)*0.7)
	if strength >= 2 {
		g.spawnFX(ptHugeExplosion, true, at, vec3{1, 0, 0}, nil)
	} else {
		g.spawnFX(ptLargeExplode, true, at, vec3{1, 0, 0}, nil)
	}
	for _, b := range blocks {
		p := vec3{float64(float32(b[0]) + rnd()), float64(float32(b[1]) + rnd()), float64(float32(b[2]) + rnd())}
		d := p.sub(at)
		n := math.Sqrt(d.dot(d))
		if n > 0 {
			d = d.scale(1 / n)
		}
		k := 0.5 / (n/float64(strength) + 0.1)
		k *= float64(rnd()*rnd() + 0.3)
		d = d.scale(k)
		g.spawnFX(ptExplode, true, p.add(at).scale(0.5), d, nil) // explosions are seen from any distance
		g.spawnFX(ptSmoke, false, p, d, nil)
		if g.world.Block(b[0], b[1], b[2]) != 0 {
			g.changeBlock(b[0], b[1], b[2], 0, false)
		}
	}
	g.vel = g.vel.add(push)
}

// netCritSparks is Animation 4 or 5: the sparks of a critical hit around a player or a mob, for three
// ticks (EffectRenderer.emitParticleAtEntity).
func (g *game) netCritSparks(eid int32, magic bool) {
	n := g.net
	var at vec3
	w, h := float32(playerW), float32(1.8)
	switch {
	case eid == n.eid:
		at = g.feet
	case n.remote[eid] != nil:
		at = n.remote[eid].pos
	case n.entity(eid) != nil:
		e := n.entity(eid)
		at = e.pos
		if e.mob != nil {
			w, h = e.mob.dims()
		}
	default:
		return
	}
	ps := g.particles()
	if len(ps.emitters) < cap(ps.emitters) {
		ps.emitters = append(ps.emitters, critEmitter{at: at, w: w, h: h, ticks: 3, magic: magic})
	}
}

// netEffect is Effect (RenderGlobal.playAuxSFX): a sound at the block, or the particles of something
// that happened there.
func (g *game) netEffect(effect int32, x, y, z int32, data int32) {
	at := centre(x, y, z)
	pitch := func() float32 { return (rnd()-rnd())*0.2 + 1 }
	switch effect {
	case 1000:
		g.sound("random.click", at, 1, 1)
	case 1001:
		g.sound("random.click", at, 1, 1.2)
	case 1002:
		g.sound("random.bow", at, 1, 1.2)
	case 1003:
		if rng.Float64() < 0.5 {
			g.sound("random.door_open", at, 1, rnd()*0.1+0.9)
		} else {
			g.sound("random.door_close", at, 1, rnd()*0.1+0.9)
		}
	case 1004:
		g.sound("random.fizz", at, 0.5, 2.6+(rnd()-rnd())*0.8)
	case 1007:
		g.sound("mob.ghast.charge", at, 10, pitch())
	case 1008:
		g.sound("mob.ghast.fireball", at, 10, pitch())
	case 1009:
		g.sound("mob.ghast.fireball", at, 2, pitch())
	case 1010:
		g.sound("mob.zombie.wood", at, 2, pitch())
	case 1011:
		g.sound("mob.zombie.metal", at, 2, pitch())
	case 1012:
		g.sound("mob.zombie.woodbreak", at, 2, pitch())
	case 1014:
		g.sound("mob.wither.shoot", at, 2, pitch())
	case 1015:
		g.sound("mob.bat.takeoff", at, 0.05, pitch())
	case 1016:
		g.sound("mob.zombie.infect", at, 2, pitch())
	case 1017:
		g.sound("mob.zombie.unfect", at, 2, pitch())
	case 1020:
		g.sound("random.anvil_break", at, 1, rnd()*0.1+0.9)
	case 1021:
		g.sound("random.anvil_use", at, 1, rnd()*0.1+0.9)
	case 1022:
		g.sound("random.anvil_land", at, 0.3, rnd()*0.1+0.9)
	case 2000: // a dispenser's smoke, out of the face it looks through
		dx, dz := float64(data%3-1), float64(data/3%3-1)
		ox, oy, oz := float64(x)+dx*0.6+0.5, float64(y)+0.5, float64(z)+dz*0.6+0.5
		for i := 0; i < 10; i++ {
			s := rng.Float64()*0.2 + 0.01
			p := vec3{ox + dx*0.01 + (rng.Float64()-0.5)*dz*0.5, oy + (rng.Float64()-0.5)*0.5, oz + dz*0.01 + (rng.Float64()-0.5)*dx*0.5}
			g.spawnFX(ptSmoke, false, p, vec3{dx*s + rng.NormFloat64()*0.01, -0.03 + rng.NormFloat64()*0.01, dz*s + rng.NormFloat64()*0.01}, nil)
		}
	case 2001: // a block broken by someone else: its pieces and its sound
		st := uint16(data&4095)<<4 | uint16(data>>12&15)
		if st>>4 == 0 {
			return
		}
		if k := soundOfBlock(st); k != bsNone {
			g.sound(k.dig(), at, 1, 0.8)
		}
		g.blockBreakEffects(x, y, z, st)
	case 2002: // a splash potion breaking: the bottle's pieces and the smash (the coloured swirl is not drawn)
		for i := 0; i < 8; i++ {
			g.spawnFX(ptIconCrack, false, vec3{float64(x), float64(y), float64(z)}, vec3{rng.NormFloat64() * 0.15, rng.Float64() * 0.2, rng.NormFloat64() * 0.15}, []int32{373, data})
		}
		g.sound("game.potion.smash", at, 1, rnd()*0.1+0.9)
	case 2004: // a spawner bringing a mob out: smoke and flames in its block
		for i := 0; i < 20; i++ {
			p := vec3{float64(x) + 0.5 + float64(rnd()-0.5)*2, float64(y) + 0.5 + float64(rnd()-0.5)*2, float64(z) + 0.5 + float64(rnd()-0.5)*2}
			g.spawnFX(ptSmoke, false, p, vec3{}, nil)
			g.spawnFX(ptFlame, false, p, vec3{}, nil)
		}
	case 2005: // bone meal: green sparkles over the plant (ItemDye.spawnBonemealParticles)
		if g.world.Block(x, y, z) == 0 {
			return
		}
		n := data
		if n == 0 {
			n = 15
		}
		for i := int32(0); i < min(n, 100); i++ {
			v := vec3{rng.NormFloat64() * 0.02, rng.NormFloat64() * 0.02, rng.NormFloat64() * 0.02}
			g.spawnFX(ptHappyVillager, false, vec3{float64(x) + float64(rnd()), float64(y) + float64(rnd()), float64(z) + float64(rnd())}, v, nil)
		}
	}
}

// noteInstruments are the sounds of a note block, by the instrument the block action names.
var noteInstruments = [5]string{"harp", "bd", "snare", "hat", "bassattack"}

// netBlockAction is Block Action (WorldClient.addBlockEvent): the block says what it does. A chest has its lid
// lifted for as long as players have it open (action 1, the number of them), a note block plays its note with
// a puff of a note over it. Pistons and the rest need no more than the block changes that come with them.
func (g *game) netBlockAction(p [3]int32, id, param byte, blk int32) {
	switch blk {
	case idChest, idTrappedChest:
		if id == 1 {
			if be := g.beAt(p, true); be != nil {
				be.viewers = int(param)
			}
		}
	case 25: // note block
		if int(id) >= len(noteInstruments) {
			return
		}
		pitch := float32(math.Pow(2, float64(int(param)-12)/12))
		g.sound("note."+noteInstruments[id], centre(p[0], p[1], p[2]), 3, pitch)
		g.spawnFX(ptNote, false, vec3{float64(p[0]) + 0.5, float64(p[1]) + 1.2, float64(p[2]) + 0.5}, vec3{float64(param) / 24, 0, 0}, nil)
	}
}
