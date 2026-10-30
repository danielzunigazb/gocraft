package client

import "math"

// What mobs do besides walking and hitting, after 1.8's entity AI: burning in the sun, keeping away
// from water, lava and drops, looking at the player, sheep eating grass and being sheared, cows milked,
// chickens laying eggs, spiders going for the player only in the dark, and slimes hopping and splitting.

func (g *game) cellAt(x, y, z float64) (int32, int32, int32) {
	return int32(math.Floor(x)), int32(math.Floor(y)), int32(math.Floor(z))
}

// lightBrightness is Block.lightBrightnessTable: how bright a light level looks, 0 to 1.
func lightBrightness(level int) float32 {
	f := 1 - float32(level)/15
	return (1 - f) / (f*3 + 1)
}

// sunlit is whether the sky shines on a mob's place brightly enough to set a zombie or skeleton alight
// (EntityZombie.onLivingUpdate: brightness above a half and the sky in sight).
func (g *game) sunlit(m *mob) bool {
	x, y, z := g.cellAt(m.pos[0], m.pos[1]+0.1, m.pos[2])
	light := g.world.Light(x, y, z)
	sky := int(light >> 4)
	if sky < 15 {
		return false
	}
	level := max(int(light&15), sky-skylightSubtracted(celestialAngle(g.worldTime)))
	return lightBrightness(level) > 0.5
}

// hazardAhead is whether the next step along a heading would take a mob into water, lava, fire or a
// cactus, or off a drop of more than three blocks.
func (g *game) hazardAhead(m *mob, yawDeg float32) bool {
	s, c := yawVector(yawDeg)
	x, y, z := g.cellAt(m.pos[0]-float64(s*0.9), m.pos[1], m.pos[2]+float64(c*0.9))
	for _, dy := range []int32{0, 1} {
		id := g.world.Block(x, y+dy, z) >> 4
		if isFluid(id) || id == 51 || id == 81 || id == 30 { // fire, cactus, web
			return true
		}
	}
	for d := int32(1); d <= 4; d++ { // is there ground within three blocks below?
		id := g.world.Block(x, y-d, z) >> 4
		if g.opaqueAt(x, y-d, z) {
			return isFluid(g.world.Block(x, y-d+1, z) >> 4)
		}
		if isFluid(id) {
			return true
		}
	}
	return true
}

// tickMobExtras runs once a tick for every living mob before it decides how to move.
func (g *game) tickMobExtras(m *mob) {
	k := m.kind
	// Burning: zombies and skeletons catch fire in daylight and lose a heart a second; water puts it out.
	if k.id == "Zombie" || k.id == "Skeleton" {
		if m.burning == 0 && g.sunlit(m) && rng.Float32()*30 < (lightBrightness(15)-0.4)*2 {
			m.burning = 160
		}
	}
	if m.burning > 0 {
		x, y, z := g.cellAt(m.pos[0], m.pos[1]+0.4, m.pos[2])
		if id := g.world.Block(x, y, z) >> 4; id == 8 || id == 9 {
			m.burning = 0
		} else {
			if m.burning--; m.burning%20 == 0 {
				m.health -= 1
				m.hurtTime = 10
			}
		}
	}
	switch {
	case k.slime:
		g.tickSlime(m)
	case k.flies:
		g.tickBat(m)
	case k.swims:
		g.tickSquid(m)
	case k.hops:
		g.tickRabbit(m)
	case !k.hostile:
		g.tickAnimal(m)
	}
	// Looking at the player: the head turns towards them when they are near.
	if dx, dz := g.feet[0]-m.pos[0], g.feet[2]-m.pos[2]; !k.slime && dx*dx+dz*dz < 36 && m.lookTicks > 0 {
		m.lookTicks--
		want := float32(math.Mod(float64(faceYaw(dx, dz)-m.yaw)+540, 360) - 180)
		m.headYawT = max(-40, min(40, want))
		dy := g.feet[1] + 1.6 - (m.pos[1] + 1)
		m.headPitT = -float32(math.Atan2(float64(dy), math.Hypot(float64(dx), float64(dz)))) / deg
	} else {
		m.headYawT, m.headPitT = 0, 0
		if !k.hostile && !k.slime && g.distToPlayer(m) < 6 && rng.Float32() < 0.02 {
			m.lookTicks = 40 + rng.Intn(40)
		}
	}
	m.headYaw += (m.headYawT - m.headYaw) * 0.3
	m.pitch += (m.headPitT - m.pitch) * 0.3
}

// tickAnimal is what the peaceful animals do on the side.
func (g *game) tickAnimal(m *mob) {
	switch m.kind.id {
	case "Chicken":
		if m.eggTimer == 0 {
			m.eggTimer = 6000 + rng.Intn(6000)
		}
		if m.eggTimer--; m.eggTimer <= 0 {
			g.spawnDrop(m.pos[0]-0.5, m.pos[1], m.pos[2]-0.5, slot{ID: 344, N: 1})
			m.eggTimer = 6000 + rng.Intn(6000)
		}
	case "Sheep": // EntityAIEatGrass
		if m.eatTicks > 0 {
			if m.eatTicks--; m.eatTicks == 4 {
				x, y, z := g.cellAt(m.pos[0], m.pos[1], m.pos[2])
				switch st := g.world.Block(x, y, z); {
				case st>>4 == 31 && st&15 == 1: // tall grass
					g.setBlock(x, y, z, 0)
					m.sheared = false
				case g.world.Block(x, y-1, z)>>4 == 2:
					g.setBlock(x, y-1, z, 3<<4)
					m.sheared = false
				}
			}
		} else if m.panic == 0 && rng.Intn(1000) == 0 {
			x, y, z := g.cellAt(m.pos[0], m.pos[1], m.pos[2])
			if st := g.world.Block(x, y, z); st>>4 == 31 && st&15 == 1 || g.world.Block(x, y-1, z)>>4 == 2 {
				m.eatTicks, m.wanderTicks = 40, 0
			}
		}
	}
}

// ---- spiders ----

// spiderTarget is whether a spider goes for the player: always in the dark, in daylight only once
// it has been angered (EntitySpider.getEntityToAttack).
func (g *game) spiderTarget(m *mob) bool {
	if !m.kind.day {
		return true
	}
	if m.angry {
		return true
	}
	x, y, z := g.cellAt(m.pos[0], m.pos[1]+0.1, m.pos[2])
	light := g.world.Light(x, y, z)
	level := max(int(light&15), int(light>>4)-skylightSubtracted(celestialAngle(g.worldTime)))
	return lightBrightness(level) < 0.5
}

// leapAtPlayer is EntityAILeapAtTarget(0.4): a spider within 4 to 16 blocks, on the ground, now and
// then jumps towards its target.
func (g *game) leapAtPlayer(m *mob) {
	d := g.distToPlayer(m)
	if !m.kind.leaps || !m.onGround || d < 4 || d > 16 || rng.Intn(5) != 0 {
		return
	}
	dx, dz := g.feet[0]-m.pos[0], g.feet[2]-m.pos[2]
	h := math.Hypot(dx, dz)
	m.vel[0] += dx/h*0.4 + m.vel[0]*0.2
	m.vel[2] += dz/h*0.4 + m.vel[2]*0.2
	m.vel[1] = 0.4
}

// ---- slimes ----

func (g *game) newSlime(pos vec3, size int) *mob {
	m := g.newMob(kinds["Slime"], pos, rng.Float32()*360)
	m.size = size
	m.health = float32(size * size)
	return m
}

func slimeSpeed(size int) float32 { return 0.2 + 0.1*float32(size) }

// tickSlime is the slime's hopping: every 10 to 30 ticks it jumps, towards the player when it can see
// one (a third of the wait then), otherwise in a random direction.
func (g *game) tickSlime(m *mob) {
	if m.size == 0 {
		m.size = 1
	}
	if m.onGround {
		if m.jumpDelay--; m.jumpDelay <= 0 {
			target := g.playerTargetable() && g.distToPlayer(m) < 16
			m.jumpDelay = 10 + rng.Intn(20)
			if target {
				m.jumpDelay /= 3
				m.yaw = turnToward(m.yaw, faceYaw(g.feet[0]-m.pos[0], g.feet[2]-m.pos[2]), 90)
			} else if rng.Intn(3) == 0 {
				m.yaw = rng.Float32() * 360
			}
			s, c := yawVector(m.yaw)
			v := 0.18 + 0.05*float32(m.size)
			m.vel[0], m.vel[1], m.vel[2] = float64(-s*v), jumpMotion, float64(c*v)
			m.onGround = false
		}
	}
	// A slime of size 2 or more hurts what it lands on (EntitySlime.onCollideWithPlayer).
	if m.size > 1 && g.playerTargetable() {
		w, _ := m.dims()
		if g.distToPlayer(m) < 0.6*float32(m.size)+w/2+0.3 && math.Abs(float64(g.feet[1]-m.pos[1])) < 1.2 {
			g.hurtPlayer(float32(m.size), m.pos, false)
		}
	}
}

// splitSlime is what a dying slime of size 2 or more leaves: two to four of half the size.
func (g *game) splitSlime(m *mob) {
	if m.size <= 1 {
		return
	}
	n := 2 + rng.Intn(3)
	for i := 0; i < n; i++ {
		ox := (float32(i%2) - 0.5) * float32(m.size) / 4
		oz := (float32(i/2) - 0.5) * float32(m.size) / 4
		g.be.newMobs = append(g.be.newMobs, g.newSlime(vec3{m.pos[0] + float64(ox), m.pos[1] + 0.5, m.pos[2] + float64(oz)}, m.size/2))
	}
}

// ---- using an item on a mob ----

// useOnMob is a right click on a mob with something in the hand: shears take a sheep's wool and a bucket
// takes a cow's milk. It says whether the click was used.
func (g *game) useOnMob() bool {
	held := g.held()
	if held.empty() {
		return false
	}
	m, ok := g.mobTarget()
	if !ok {
		return false
	}
	switch {
	case held.ID == 359 && m.kind.id == "Sheep" && !m.sheared: // shears
		m.sheared = true
		for n := 1 + rng.Intn(3); n > 0; n-- {
			g.spawnDrop(m.pos[0]-0.5, m.pos[1]+1, m.pos[2]-0.5, blockSlot(35<<4|uint16(m.colour&15), 1))
		}
		g.useDurability(1)
		return true
	case held.ID == 325 && m.kind.id == "Cow": // an empty bucket
		if g.creative {
			return true
		}
		if g.inv[g.sel].N == 1 {
			g.inv[g.sel] = slot{ID: 335, N: 1}
		} else {
			g.inv[g.sel].N--
			if rest := g.giveSlot(slot{ID: 335, N: 1}); !rest.empty() {
				g.throwItem(rest)
			}
		}
		return true
	}
	return false
}

// ---- bats, squid and rabbits ----

func sgn(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// tickBat is EntityBat's flight: it picks a point near where it is, and its motion is pulled towards the
// direction of that point a tenth of the way each tick.
func (g *game) tickBat(m *mob) {
	if m.jumpDelay--; m.target == (vec3{}) || m.jumpDelay <= 0 {
		m.target = vec3{m.pos[0] + float64(rng.Intn(7)-rng.Intn(7)), m.pos[1] + float64(rng.Intn(6)) - 2, m.pos[2] + float64(rng.Intn(7)-rng.Intn(7))}
		m.jumpDelay = 20 + rng.Intn(60)
		if x, y, z := g.cellAt(m.target[0], m.target[1], m.target[2]); y < 1 || g.world.Block(x, y, z) != 0 {
			m.target = vec3{m.pos[0], m.pos[1] + 1, m.pos[2]} // not into a wall
		}
	}
	d := m.target.sub(m.pos)
	m.vel[0] += (sgn(d[0])*0.5 - m.vel[0]) * 0.1
	m.vel[1] += (sgn(d[1])*0.7 - m.vel[1]) * 0.1
	m.vel[2] += (sgn(d[2])*0.5 - m.vel[2]) * 0.1
	m.yaw = faceYaw(m.vel[0], m.vel[2])
	m.onGround = false
}

func (g *game) inWater(m *mob) bool {
	x, y, z := g.cellAt(m.pos[0], m.pos[1]+float64(m.kind.h)/2, m.pos[2])
	id := g.world.Block(x, y, z) >> 4
	return id == 8 || id == 9
}

// tickSquid swims about in the water in a direction that changes every few seconds, and does not
// leave it; out of the water it only falls.
func (g *game) tickSquid(m *mob) {
	if !g.inWater(m) {
		return
	}
	if m.jumpDelay--; m.jumpDelay <= 0 {
		a := rng.Float32() * 2 * pi
		m.swimVec = vec3{float64(cos32(a)), float64((rng.Float32() - 0.5) * 0.6), float64(sin32(a))}
		m.jumpDelay = 40 + rng.Intn(60)
	}
	sp := float32(0.12)
	next := m.pos.add(m.swimVec.scale(float64(sp * 6)))
	if x, y, z := g.cellAt(next[0], next[1]+float64(m.kind.h)/2, next[2]); g.world.Block(x, y, z)>>4 != 9 && g.world.Block(x, y, z)>>4 != 8 {
		m.swimVec = m.swimVec.scale(-1) // the shore: turn round
		m.jumpDelay = 20
	}
	m.vel = m.swimVec.scale(float64(sp))
	m.yaw = faceYaw(m.vel[0], m.vel[2])
}

// tickRabbit hops: a rabbit rests and then jumps, away from the player when they come near.
func (g *game) tickRabbit(m *mob) {
	if !m.onGround {
		return
	}
	scared := m.panic > 0 || g.distToPlayer(m) < 6 && !g.creative
	if m.jumpDelay--; m.jumpDelay > 0 {
		return
	}
	m.jumpDelay = 10 + rng.Intn(40)
	speed := float32(0.18)
	switch {
	case scared:
		m.jumpDelay = 3 + rng.Intn(4)
		speed = 0.3
		m.yaw = faceYaw(m.pos[0]-g.feet[0], m.pos[2]-g.feet[2])
	case rng.Intn(3) == 0:
		m.yaw = rng.Float32() * 360
	default:
		return
	}
	for try := 0; try < 4 && g.hazardAhead(m, m.yaw); try++ { // not into the water or off a cliff
		m.yaw += 90
	}
	s, c := yawVector(m.yaw)
	m.vel[0], m.vel[1], m.vel[2] = float64(-s*speed), 0.41, float64(c*speed)
	m.onGround = false
}
