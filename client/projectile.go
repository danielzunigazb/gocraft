package client

import "math"

// Arrows, after 1.8's EntityArrow and ItemBow: drawn for up to a second, shot with a speed that
// grows with the draw, pulled down by 0.05 a tick, slowed by 1% (40% in water), stuck in what
// they hit, and hurting for the ceiling of their speed times two.

type arrow struct {
	pos, prev, vel vec3
	shooter        *mob // nil: shot by the player
	damage         float32
	crit           bool
	inGround       bool
	groundTicks    int
	airTicks       int
	shake          int
	age            int
	pickable       bool
}

const (
	arrowGravity = 0.05
	bowFullDraw  = 20 // ticks for a full draw
)

func gauss() float32 {
	// Box-Muller: the standard normal the game's nextGaussian gives.
	u1, u2 := float64(rng.Float32()), float64(rng.Float32())
	if u1 < 1e-9 {
		u1 = 1e-9
	}
	return float32(math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2))
}

// heading is setThrowableHeading: aim along a direction at a speed, with random scatter.
func heading(dir vec3, speed, inaccuracy float32) vec3 {
	d := dir.norm()
	d[0] += float64(gauss() * 0.0075 * inaccuracy)
	d[1] += float64(gauss() * 0.0075 * inaccuracy)
	d[2] += float64(gauss() * 0.0075 * inaccuracy)
	return d.scale(float64(speed))
}

// drawPower is how strong a bow drawn for n ticks is, 0..1, as ItemBow.onPlayerStoppedUsing has it.
func drawPower(n int) float32 {
	f := float32(n) / bowFullDraw
	f = (f*f + f*2) / 3
	return min(f, 1)
}

func (g *game) hasArrow() bool {
	if g.creative {
		return true
	}
	for _, s := range g.inv {
		if s.ID == 262 && !s.empty() {
			return true
		}
	}
	return false
}

func (g *game) takeArrow() {
	if g.creative {
		return
	}
	for i := range g.inv {
		if g.inv[i].ID == 262 && !g.inv[i].empty() {
			if g.inv[i].N--; g.inv[i].N <= 0 {
				g.inv[i] = slot{}
			}
			return
		}
	}
}

// shootArrow looses an arrow from the player at the given draw power.
func (g *game) shootArrow(f float32) {
	yaw, pitch := g.yawDeg()*deg, g.pitch
	g.bowSound(f)
	a := &arrow{damage: 2, crit: f >= 1, pickable: !g.creative}
	a.pos = vec3{g.pos[0] - float64(mhCos(yaw)*0.16), g.pos[1] - 0.1, g.pos[2] - float64(mhSin(yaw)*0.16)}
	dir := vec3{float64(-mhSin(yaw) * mhCos(pitch)), float64(-mhSin(pitch)), float64(mhCos(yaw) * mhCos(pitch))}
	a.vel = heading(dir, f*2*1.5, 1)
	a.vel[0] += g.vel[0]
	a.vel[2] += g.vel[2]
	if !g.onGround {
		a.vel[1] += g.vel[1]
	}
	a.prev = a.pos
	g.arrows = append(g.arrows, a)
	g.takeArrow()
	g.useDurability(1)
	g.exhaust(0)
}

// shootFromMob is a skeleton's shot at the player (EntitySkeleton.attackEntityWithRangedAttack).
func (g *game) shootFromMob(m *mob, distFactor float32) {
	a := &arrow{shooter: m, pickable: true}
	g.skeletonBowSound(m.pos)
	a.pos = vec3{m.pos[0], m.pos[1] + float64(m.kind.h)*0.85 - 0.1, m.pos[2]}
	a.damage = distFactor*2 + gauss()*0.25 + 2*0.11 // Normal difficulty
	d := vec3{g.feet[0] - a.pos[0], g.feet[1] + playerH/3 - a.pos[1], g.feet[2] - a.pos[2]}
	d[1] += math.Hypot(d[0], d[2]) * 0.2
	a.vel = heading(d, 1.6, 10)
	a.prev = a.pos
	g.arrows = append(g.arrows, a)
}

// tickArrow advances an arrow; it reports whether it still exists.
func (g *game) tickArrow(a *arrow) bool {
	a.age++
	a.prev = a.pos
	if a.shake > 0 {
		a.shake--
	}
	if a.inGround {
		if a.groundTicks++; a.groundTicks >= 1200 {
			return false
		}
		// A player standing next to it takes it back.
		if a.pickable && a.shake <= 0 && g.hasRoomFor(262) {
			reach := playerBox(g.feet)
			reach.min = reach.min.sub(vec3{1, 0.5, 1})
			reach.max = reach.max.add(vec3{1, 0.5, 1})
			if overlaps(reach, aabb{a.pos.sub(vec3{0.1, 0.1, 0.1}), a.pos.add(vec3{0.1, 0.1, 0.1})}) {
				g.giveSlot(slot{ID: 262, N: 1})
				return false
			}
		}
		return true
	}
	a.airTicks++
	start, end := a.pos, a.pos.add(a.vel)
	blockHit, hitBlock := g.rayTrace(start, end)
	if hitBlock {
		end = blockHit.at
	}
	// The nearest creature along the way: boxes grown by 0.3.
	var target *mob
	hitPlayer := false
	best := math.Inf(1)
	for _, m := range g.mobs {
		if m.dead() || (a.shooter == m && a.airTicks < 5) {
			continue
		}
		b := m.box()
		b.min, b.max = b.min.sub(vec3{0.3, 0.3, 0.3}), b.max.add(vec3{0.3, 0.3, 0.3})
		if at, _, ok := segmentBox(b, start, end); ok {
			if d := at.sub(start).dot(at.sub(start)); d < best && a.shooter != m {
				best, target = d, m
			}
		}
	}
	if a.shooter != nil && g.playerTargetable() {
		b := playerBox(g.feet)
		b.min, b.max = b.min.sub(vec3{0.3, 0.3, 0.3}), b.max.add(vec3{0.3, 0.3, 0.3})
		if at, _, ok := segmentBox(b, start, end); ok {
			if d := at.sub(start).dot(at.sub(start)); d < best {
				best, target, hitPlayer = d, nil, true
			}
		}
	}
	if target != nil || hitPlayer {
		speed := float32(math.Sqrt(float64(a.vel.dot(a.vel))))
		dmg := float32(math.Ceil(float64(speed * a.damage)))
		if a.crit {
			dmg += float32(rng.Intn(int(dmg)/2 + 2))
		}
		from := g.feet
		if a.shooter != nil {
			from = a.shooter.pos
		}
		var ok bool
		if hitPlayer {
			ok = g.hurtPlayer(dmg, from, false)
		} else {
			ok = target.hurt(dmg, from)
		}
		if ok {
			g.arrowHitSound(a.pos)
			return false
		}
		a.vel = a.vel.scale(-0.1) // it bounces off one that cannot be hurt
		a.airTicks = 0
		return true
	}
	if hitBlock {
		a.vel = blockHit.at.sub(a.pos)
		n := a.vel.norm()
		a.pos = blockHit.at.sub(n.scale(0.05))
		a.inGround, a.shake, a.crit = true, 7, false
		g.arrowHitSound(a.pos)
		return true
	}
	a.pos = a.pos.add(a.vel)
	drag := float32(0.99)
	if g.world.Block(int32(math.Floor(float64(a.pos[0]))), int32(math.Floor(float64(a.pos[1]))), int32(math.Floor(float64(a.pos[2]))))>>4 == 9 {
		drag = 0.6
	}
	a.vel = a.vel.scale(float64(drag))
	a.vel[1] -= arrowGravity
	return a.pos[1] > -64
}

func (g *game) hasRoomFor(id uint16) bool {
	return g.capacity(slot{ID: id, N: 1}) > 0
}

func (g *game) tickArrows() {
	live := g.arrows[:0]
	for _, a := range g.arrows {
		if g.tickArrow(a) {
			live = append(live, a)
		}
	}
	g.arrows = live
}

// tickBow draws and looses the bow while the use button is held with one in hand.
func (g *game) tickBow() {
	held := g.held()
	using := held.ID == 261 && !held.isBlock() && g.screen == screenNone && g.keyDown("use") && g.hasArrow()
	switch {
	case using:
		g.bowTicks++
	case g.bowTicks > 0:
		if f := drawPower(g.bowTicks); f >= 0.1 && held.ID == 261 && !g.thirdParty() { // a server shoots its own arrow
			g.shootArrow(f)
		}
		g.bowTicks = 0
	}
}

// arrowGeometry lays an arrow out as 1.8's RenderArrow does: four strips of the 16 x 4 shaft picture
// (u 0..0.5, v 0..0.15625 of arrow.png) round the flight axis, two crossed planes each seen from
// both sides, and the 4 x 4 end cap (u 0..0.15625, v 0.15625..0.3125) of the fletching at the tail,
// all at a scale of 0.05625 and with the tail 11 units behind the arrow's position.
func arrowGeometry(out []vertex, a *arrow, partial float32, light uint8) []vertex {
	pos := a.prev.add(a.pos.sub(a.prev).scale(float64(partial)))
	f := a.vel
	if f.dot(f) < 1e-6 {
		f = vec3{0, 0, 1}
	}
	f = f.norm()
	up0 := vec3{0, 1, 0}
	if math.Abs(float64(f[1])) > 0.999 {
		up0 = vec3{1, 0, 0}
	}
	h := f.cross(up0).norm() // the game's z once yaw and pitch are in: sideways
	up := h.cross(f)         // and its y: up, in the vertical plane of the flight
	const s = 0.05625
	const eps = 0.0015 // coplanar partners stand a hair apart so the nearer one wins the depth test
	col := [4]uint8{255, 255, 255, light}
	at := func(x float32, across vec3) vec3 { return pos.add(f.scale(float64((x - 4) * s))).add(across) }

	// The shaft: after rotate(45) about the axis, each strip is turned a further 90 degrees.
	for j := 0; j < 4; j++ {
		r := float64(45+90*(j+1)) * math.Pi / 180
		w := up.scale(float64(float32(math.Cos(r)))).add(h.scale(float64(float32(math.Sin(r)))))
		front := f.cross(w).scale(eps) // the side the picture reads the right way round from
		corner := func(x, y, u, v float32) vertex {
			p := at(x, w.scale(float64(y*s))).add(front)
			return vertex{float32(p[0]), float32(p[1]), float32(p[2]), u, v, col}
		}
		out = append(out,
			corner(-8, -2, 0, 0), corner(8, -2, 0.5, 0), corner(8, 2, 0.5, 0.15625), corner(-8, 2, 0, 0.15625))
	}

	// The end cap, once facing along the flight and once the other way.
	cr, sr := float32(math.Cos(math.Pi/4)), float32(math.Sin(math.Pi/4))
	yAx := up.scale(float64(cr)).add(h.scale(float64(sr)))
	zAx := up.scale(float64(-sr)).add(h.scale(float64(cr)))
	endcap := func(y, z, u, v float32, side float32) vertex {
		p := at(-7, yAx.scale(float64(y*s)).add(zAx.scale(float64(z*s)))).add(f.scale(float64(side * eps)))
		return vertex{float32(p[0]), float32(p[1]), float32(p[2]), u, v, col}
	}
	out = append(out,
		endcap(-2, -2, 0, 0.15625, 1), endcap(-2, 2, 0.15625, 0.15625, 1), endcap(2, 2, 0.15625, 0.3125, 1), endcap(2, -2, 0, 0.3125, 1),
		endcap(2, -2, 0, 0.15625, -1), endcap(2, 2, 0.15625, 0.15625, -1), endcap(-2, 2, 0.15625, 0.3125, -1), endcap(-2, -2, 0, 0.3125, -1))
	return out
}
