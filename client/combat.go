package client

import "math"

// Fighting, after 1.8: how hard a weapon hits, critical hits, sprint knockback, the invulnerability
// frames after a hit, armour and blocking with a sword, explosions, and what mobs drop.

// weaponDamage is the damage a held item adds to the player's own 1.
func weaponDamage(s slot) float32 {
	class, tier := toolOf(s)
	tierBonus := [5]float32{0, 1, 2, 3, 0}[tier]
	switch class {
	case "sword":
		return 4 + tierBonus
	case "axe":
		return 3 + tierBonus
	case "pickaxe":
		return 2 + tierBonus
	case "shovel":
		return 1 + tierBonus
	}
	return 0
}

// armourPoints is what an armour piece is worth: helmet, chestplate, leggings, boots by material.
func armourPoints(id uint16) int {
	if id < 298 || id > 317 {
		return 0
	}
	mat := int(id-298) / 4 // leather, chainmail, iron, diamond, gold
	return [5][4]int{{1, 3, 2, 1}, {2, 5, 4, 1}, {2, 6, 5, 2}, {3, 8, 6, 3}, {2, 5, 3, 1}}[mat][int(id-298)%4]
}

func (g *game) armourTotal() int {
	n := 0
	for _, a := range g.armor {
		if !a.empty() {
			n += armourPoints(a.ID)
		}
	}
	return n
}

// afterArmour is EntityPlayer.applyArmorCalculations: each armour point takes 4% off.
func afterArmour(damage float32, points int) float32 { return damage * float32(25-points) / 25 }

// hurtPlayer is attackEntityFrom for the player: invulnerability, blocking, armour, knockback.
func (g *game) hurtPlayer(amount float32, from vec3, unblockable bool) bool {
	if g.creative || g.health <= 0 {
		return false
	}
	fresh := true
	applied := amount
	if g.hurtResist > 10 {
		if amount <= g.lastDamage {
			return false
		}
		applied, fresh = amount-g.lastDamage, false
		g.lastDamage = amount
	} else {
		g.lastDamage, g.hurtResist = amount, 20
	}
	if g.blocking && !unblockable && applied > 0 {
		applied = (1 + applied) * 0.5
	}
	if !unblockable {
		applied = afterArmour(applied, g.armourTotal())
		g.wearArmour(applied)
	}
	if fresh {
		dx, dz := from[0]-g.feet[0], from[2]-g.feet[2]
		for dx*dx+dz*dz < 1e-4 {
			dx, dz = float64(rng.Float32()-rng.Float32())*0.01, float64(rng.Float32()-rng.Float32())*0.01
		}
		// EntityLivingBase.knockBack: the distance is a float, the push 0.4F, the cap 0.4000000059604645.
		d := float64(float32(math.Sqrt(dx*dx + dz*dz)))
		g.vel[0], g.vel[1], g.vel[2] = g.vel[0]/2, g.vel[1]/2, g.vel[2]/2
		g.vel[0] -= dx / d * knockPower
		g.vel[1] += knockPower
		g.vel[2] -= dz / d * knockPower
		if g.vel[1] > knockPower {
			g.vel[1] = knockPower
		}
		g.onGround = false
	}
	g.hurt(applied)
	return true
}

// wearArmour is EntityPlayer.damageArmor: every piece loses a quarter of the damage, at least 1.
func (g *game) wearArmour(damage float32) {
	n := max(int(damage/4), 1)
	for i := range g.armor {
		a := &g.armor[i]
		if a.empty() {
			continue
		}
		if a.Dmg += uint16(n); int(a.Dmg) > a.durability() {
			*a = slot{}
		}
	}
}

func (g *game) onLadder2() bool { return g.onLadder() }

// attackMob is EntityPlayer.attackTargetEntityWithCurrentItem against a mob.
func (g *game) attackMob(m *mob) bool {
	held := g.held()
	f := 1 + weaponDamage(held)
	knock := 0
	if g.sprinting {
		knock++
	}
	crit := g.fall > 0 && !g.onGround && !g.onLadder() && !g.fluidIn(false)
	if crit {
		f *= 1.5
		g.critEffect(m.pos, m.kind.w, m.kind.h)
	}
	if !m.hurt(f, g.feet) {
		return false
	}
	if knock > 0 {
		s, c := yawVector(g.yawDeg())
		m.vel[0] += float64(-s * float32(knock) * 0.5)
		m.vel[1] += 0.1
		m.vel[2] += float64(c * float32(knock) * 0.5)
		g.vel[0], g.vel[2] = g.vel[0]*0.6, g.vel[2]*0.6
		g.setSprinting(false)
	}
	g.exhaust(0.3)
	if class, _ := toolOf(held); class == "sword" {
		g.useDurability(1)
	} else if class != "" {
		g.useDurability(2)
	}
	return true
}

// mobTarget is the mob the player points at within reach 3 (a block in front of it hides it).
func (g *game) mobTarget() (*mob, bool) {
	dir := look(g.yaw, g.pitch)
	end := g.pos.add(dir.scale(3))
	var best *mob
	bestD := math.Inf(1)
	for _, m := range g.mobs {
		if m.dead() {
			continue
		}
		b := m.box()
		b.min = b.min.sub(vec3{0.1, 0.1, 0.1}) // the game grows the hitbox by a tenth
		b.max = b.max.add(vec3{0.1, 0.1, 0.1})
		if at, _, ok := segmentBox(b, g.pos, end); ok {
			if d := at.sub(g.pos).dot(at.sub(g.pos)); d < bestD {
				best, bestD = m, d
			}
		} else if g.pos[0] >= b.min[0] && g.pos[0] <= b.max[0] && g.pos[1] >= b.min[1] && g.pos[1] <= b.max[1] && g.pos[2] >= b.min[2] && g.pos[2] <= b.max[2] {
			best, bestD = m, 0
		}
	}
	if best == nil {
		return nil, false
	}
	if h, ok := g.rayTrace(g.pos, end); ok && h.at.sub(g.pos).dot(h.at.sub(g.pos)) < bestD {
		return nil, false // a block is in the way
	}
	return best, true
}

// attackClick is a left click: it hits a mob if one is pointed at, and says whether it did.
func (g *game) attackClick() bool {
	if g.net != nil { // the server's players and mobs: the server decides what the blow does
		eid, ok := g.entityTarget()
		g.noteReach(eid, ok)
		if ok {
			g.attackEntity(eid)
		}
		return ok
	}
	m, mobOK := g.mobTarget()
	if !mobOK {
		return false
	}
	g.attackMob(m)
	return true
}

func (g *game) mobDrops(m *mob) []slot {
	var out []slot
	if m.kind.slime { // only the smallest leave slimeballs
		if m.size <= 1 {
			out = append(out, slot{ID: 341, N: int8(rng.Intn(3))})
		}
	} else if m.kind.id == "Sheep" {
		if !m.sheared {
			out = append(out, blockSlot(35<<4|uint16(m.colour&15), 1))
		}
	} else if m.kind.dropsFn != nil {
		out = m.kind.dropsFn()
	}
	res := out[:0]
	for _, s := range out {
		if s.N > 0 {
			res = append(res, s)
		}
	}
	return res
}

// blastResistance is how much of an explosion's strength a block soaks up.
func blastResistance(id uint16) float32 {
	switch id {
	case 0:
		return 0
	case 7, 90, 119:
		return 18000000
	case 49:
		return 6000
	case 8, 9, 10, 11:
		return 500
	case 1, 4, 14, 15, 16, 21, 22, 24, 41, 42, 45, 48, 56, 57, 61, 62, 98, 109, 121, 129, 133, 153, 159, 172, 173:
		return 30
	case 3, 12, 13, 2, 82, 110, 88:
		return 2.5
	case 18, 161, 31, 32, 37, 38, 39, 40, 50, 51, 59, 83, 106:
		return 0
	case 20, 102, 95, 160, 79:
		return 1.5
	}
	return 10
}

// explode is World.createExplosion: rays from the centre carve out blocks until their strength is
// spent, and everything near is hurt and thrown.
func (g *game) explode(at vec3, power float32, src *mob) {
	g.explosionSound(at)
	destroyed := map[[3]int32]bool{}
	for x := 0; x < 16; x++ {
		for y := 0; y < 16; y++ {
			for z := 0; z < 16; z++ {
				if x != 0 && x != 15 && y != 0 && y != 15 && z != 0 && z != 15 {
					continue
				}
				dx, dy, dz := float32(x)/15*2-1, float32(y)/15*2-1, float32(z)/15*2-1
				n := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
				dx, dy, dz = dx/n*0.3, dy/n*0.3, dz/n*0.3
				strength := power * (0.7 + rng.Float32()*0.6)
				px, py, pz := at[0], at[1]+0.5, at[2]
				for strength > 0 {
					bx, by, bz := int32(math.Floor(px)), int32(math.Floor(py)), int32(math.Floor(pz))
					if !world_InRange(by) {
						break
					}
					st := g.world.Block(bx, by, bz)
					if st != 0 {
						strength -= (blastResistance(st>>4) + 0.3) * 0.3
					}
					if strength > 0 && st != 0 {
						destroyed[[3]int32{bx, by, bz}] = true
					}
					px, py, pz = px+float64(dx), py+float64(dy), pz+float64(dz)
					strength -= 0.225
				}
			}
		}
	}
	for c := range destroyed {
		st := g.world.Block(c[0], c[1], c[2])
		if rng.Float32() < 1/power {
			for _, d := range g.dropsFor(st, true, slot{}) {
				g.spawnDrop(float64(c[0]), float64(c[1]), float64(c[2]), d)
			}
		}
		g.setBlock(c[0], c[1], c[2], 0)
	}
	r := power * 2
	hit := func(pos vec3, hurt func(dmg float32), push func(v vec3)) {
		d := float32(math.Sqrt(float64(pos.sub(at).dot(pos.sub(at))))) / r
		if d > 1 {
			return
		}
		impact := 1 - d
		hurt(float32(int((impact*impact+impact)/2*8*r + 1)))
		dir := pos.sub(at).norm()
		push(dir.scale(float64(impact)))
	}
	hit(vec3{g.feet[0], g.feet[1] + 0.9, g.feet[2]}, func(d float32) { g.hurtPlayer(d, at, true) }, func(v vec3) { g.vel = g.vel.add(v) })
	for _, m := range g.mobs {
		if m == src {
			continue
		}
		m := m
		hit(vec3{m.pos[0], m.pos[1] + float64(m.kind.h)/2, m.pos[2]}, func(d float32) { m.health -= d; m.hurtTime = 10 }, func(v vec3) { m.vel = m.vel.add(v) })
	}
}

func world_InRange(y int32) bool { return y >= 0 && y < 256 }
