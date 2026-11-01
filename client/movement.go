package client

import "math"

// The player's movement, written after 1.8.9's EntityPlayerSP.onLivingUpdate, EntityLivingBase
// .moveEntityWithHeading and Entity.moveEntity, with the same order of steps, the same constants
// and the game's own sine table, so that speeds, jumps and stops agree tick for tick.

// sinTable is MathHelper's lookup table: the game turns yaw into sin and cos through it, which is
// not the same, to the last digits, as computing them.
var sinTable = func() (t [65536]float32) {
	for i := range t {
		t[i] = float32(math.Sin(float64(i) * math.Pi * 2 / 65536))
	}
	return
}()

func mhSin(f float32) float32 { return sinTable[int32(f*10430.378)&65535] }
func mhCos(f float32) float32 { return sinTable[int32(f*10430.378+16384)&65535] }

// Constants the game writes as floats and then widens to double, or as double literals of such a value.
var (
	jumpMotion  = float64(float32(0.42)) // getJumpUpwardsMotion() returns 0.42F
	dragY       = float64(float32(0.98)) // motionY *= 0.9800000190734863D
	webY        = float64(float32(0.05)) // y *= 0.05000000074505806D in a cobweb
	waterY      = float64(float32(0.8))  // motionY *= 0.800000011920929D in water
	ladderLimit = float64(float32(0.15)) // float f6 = 0.15F, widened by clamp_double
	stepUp      = float64(float32(0.6))  // stepHeight is the float 0.6F
	ledgeLift   = float64(float32(0.6))  // isOffsetPositionInLiquid(..., motionY + 0.6000000238418579D ...)
	ledgeHopY   = float64(float32(0.3))  // motionY = 0.30000001192092896D
	swimUp      = float64(float32(0.04)) // motionY += 0.03999999910593033D, holding jump in a fluid
	knockPower  = float64(float32(0.4))  // knockBack: float f1 = 0.4F, and the cap 0.4000000059604645D
)

const (
	landSpeed     = float32(0.1)  // movement speed attribute
	sprintBonus   = 0.3           // the 0.3D that sprinting adds to the air factor (the speed attribute has its own, see sprintAmount)
	sprintLasts   = 600           // ticks a sprint runs out after (EntityPlayerSP.setSprinting)
	airFactor     = float32(0.02) // acceleration in the air
	flySpeed      = float32(0.05)
	jumpDelay     = 10 // ticks before the next jump may start
	sprintTapTime = 7
)

// slipperiness of the block under the feet; ground friction is this times 0.91.
func slipperiness(id uint16) float32 {
	switch id {
	case 79, 174: // ice, packed ice
		return 0.98
	case 165: // slime
		return 0.8
	}
	return 0.6
}

// blockUnderFeet is the id of the block the game reads friction from: the one below the feet.
func (g *game) blockUnderFeet() uint16 {
	return g.world.Block(int32(math.Floor(g.feet[0])), int32(math.Floor(g.feet[1]))-1, int32(math.Floor(g.feet[2]))) >> 4
}

// fluidIn reports whether any cell of the given kind overlaps the player's box shrunk as the game
// does: for water 0.4 off the bottom and top, for lava also 0.1 off the sides.
func (g *game) fluidIn(lava bool) bool {
	b := playerBox(g.feet)
	shrinkX := 0.001
	if lava {
		shrinkX = 0.1
	}
	b.min = b.min.add(vec3{shrinkX, 0.4 + 0.001, shrinkX})
	b.max = b.max.sub(vec3{shrinkX, 0.4 + 0.001, shrinkX})
	return g.anyCell(b, func(id uint16) bool {
		if lava {
			return id == 10 || id == 11
		}
		return id == 8 || id == 9
	})
}

// anyCell reports whether any block cell overlapping the box satisfies the test.
func (g *game) anyCell(b aabb, f func(id uint16) bool) bool {
	for x := int32(math.Floor(b.min[0])); x <= int32(math.Floor(b.max[0])); x++ {
		for y := int32(math.Floor(b.min[1])); y <= int32(math.Floor(b.max[1])); y++ {
			for z := int32(math.Floor(b.min[2])); z <= int32(math.Floor(b.max[2])); z++ {
				if f(g.world.Block(x, y, z) >> 4) {
					return true
				}
			}
		}
	}
	return false
}

func (g *game) fluidAt(feet vec3) bool {
	saved := g.feet
	g.feet = feet
	defer func() { g.feet = saved }()
	return g.fluidIn(false)
}

func (g *game) onLadder() bool {
	id := g.world.Block(int32(math.Floor(g.feet[0])), int32(math.Floor(g.feet[1])), int32(math.Floor(g.feet[2]))) >> 4
	return id == 65 || id == 106
}

// moveFlying adds the input to the motion, in the direction the player faces.
func (g *game) moveFlying(strafe, forward, friction float32) {
	f := strafe*strafe + forward*forward
	if f < 1e-4 {
		return
	}
	f = float32(math.Sqrt(float64(f)))
	if f < 1 {
		f = 1
	}
	f = friction / f
	strafe, forward = strafe*f, forward*f
	yaw := g.yawDeg() * math.Pi / 180
	sin, cos := mhSin(yaw), mhCos(yaw)
	// Entity.moveFlying works in floats and widens the sums to the double motion; the explicit
	// conversions keep the compiler from fusing the products.
	g.vel[0] += float64(float32(strafe*cos) - float32(forward*sin))
	g.vel[2] += float64(float32(forward*cos) + float32(strafe*sin))
}

// yawDeg is the facing in degrees, as the game keeps it.
func (g *game) yawDeg() float32 { return g.yaw * 180 / math.Pi }

// jump is EntityLivingBase.jump: 0.42 up, and a push forward when sprinting.
func (g *game) jump() {
	g.vel[1] = jumpMotion + g.jumpBoost()
	if g.sprinting {
		g.exhaust(0.8)
	} else {
		g.exhaust(0.2)
	}
	if g.sprinting {
		f := g.yawDeg() * 0.017453292
		g.vel[0] -= float64(mhSin(f) * 0.2)
		g.vel[2] += float64(mhCos(f) * 0.2)
	}
}

// tick advances the player by one game tick.
func (g *game) tick(in input) {
	if g.net != nil && !g.terrainLoaded() { // the player does not exist for the game until its column has arrived
		g.tickWaiting()
		return
	}
	g.ticks++
	g.applyPendingHit()
	g.takeClicks() // Minecraft.runTick takes the mouse in before the world, and so the player, moves on
	g.updateFov()
	g.tickDamage()
	y0 := g.feet[1]
	defer g.tickSurvival(y0)
	g.prevFeet = g.feet
	g.tickActions()

	// EntityPlayerSP.onLivingUpdate: what the keys ask for.
	if g.sprintTicks > 0 {
		if g.sprintTicks--; g.sprintTicks == 0 { // a sprint that was never ended runs out
			g.setSprinting(false)
		}
	}
	if g.sprintToggle > 0 {
		g.sprintToggle--
	}
	if g.flyToggle > 0 {
		g.flyToggle--
	}
	if g.canFly() && in.jump && !g.wasJump { // two quick jumps start or stop flying
		if g.flyToggle == 0 {
			g.flyToggle = 7
		} else {
			g.flying, g.flyToggle = !g.flying, 0
			g.sendAbilities()
		}
	}
	g.wasJump = in.jump

	forward, strafe := in.forward, in.strafe
	if in.sneak {
		forward, strafe = forward*0.3, strafe*0.3
	}
	using := g.usingItem()
	if using { // using an item slows walking to a fifth, and forgets a first tap of forward
		forward, strafe = forward*0.2, strafe*0.2
		g.sprintToggle = 0
	}
	wasSneak, wasForward := g.wasSneak, g.wasForward
	g.wasSneak, g.wasForward = in.sneak, forward >= 0.8
	_, blind := g.hasEffect(effBlindness)
	canSprint := g.food > 6 || g.canFly()
	if g.onGround && !wasSneak && !wasForward && forward >= 0.8 && !g.sprinting && canSprint && !using && !blind {
		if g.sprintToggle <= 0 && !in.sprint {
			g.sprintToggle = sprintTapTime
		} else {
			g.setSprinting(true)
		}
	}
	if !g.sprinting && forward >= 0.8 && canSprint && !using && !blind && in.sprint {
		g.setSprinting(true)
	}
	if g.sprinting && (forward < 0.8 || g.collidedH || !canSprint) {
		g.setSprinting(false)
	}

	if g.flying {
		if in.sneak {
			g.vel[1] -= float64(g.flySpeedNow() * 3)
		}
		if in.jump {
			g.vel[1] += float64(g.flySpeedNow() * 3)
		}
	}
	g.sneaking = in.sneak

	// EntityLivingBase.onLivingUpdate.
	if g.jumpTicks > 0 {
		g.jumpTicks--
	}
	for i := range g.vel {
		if g.vel[i] < 0.005 && g.vel[i] > -0.005 {
			g.vel[i] = 0
		}
	}
	inWater := g.fluidIn(false)
	inLava := g.fluidIn(true)
	if in.jump {
		if inWater || inLava {
			g.vel[1] += swimUp
		} else if g.onGround && g.jumpTicks == 0 {
			g.jump()
			g.jumpTicks = jumpDelay
		}
	} else {
		g.jumpTicks = 0
	}
	strafe, forward = strafe*0.98, forward*0.98
	g.moveWithHeading(strafe, forward, inWater, inLava)
	g.setMoveSpeeds()           // EntityPlayer.onLivingUpdate: after the move, for the next tick's
	if g.onGround && g.flying { // landing ends a flight, once the player has moved
		g.flying = false
		g.sendAbilities()
	}

	// EntityPlayer.addMovementStat: every metre walked costs a little, running costs ten times more.
	if d := float32(math.Hypot(g.feet[0]-g.prevFeet[0], g.feet[2]-g.prevFeet[2])); d > 0 {
		switch {
		case inWater:
			g.exhaust(0.015 * d)
		case g.onGround && g.sprinting:
			g.exhaust(0.1 * d)
		case g.onGround:
			g.exhaust(0.01 * d)
		}
	}
	g.worldTime++
	g.loadNearbyEntities()
	// Torch light flickers a little: a damped random walk (EntityRenderer.updateTorchFlicker).
	g.flicker = (g.flicker + (rng.Float32()-rng.Float32())*rng.Float32()*rng.Float32()) * 0.9
	g.tickBody()
	g.tickHand()
	g.tickPortal()
	g.tickParticles()
	g.tickSound()
	g.tickItems()
	g.tickBlockEntities()
	if g.net != nil {
		g.tickNet()
	} else {
		g.tickMobs()
		g.tickArrows()
		g.spawnHostiles()
		g.spawnAmbient()
	}
	if g.hurtResist > 0 {
		g.hurtResist--
	}
	if g.hurtTime > 0 {
		g.hurtTime--
	}
	g.tickEffects()
	g.statusBars.ticks++
	g.tickTitles()
	if g.health <= 0 && g.net != nil {
		g.deathTime = min(g.deathTime+1, 20)
	} else {
		g.deathTime = 0
	}
	if g.feet[1] < -64 { // fell out of the world: back to the start
		g.fellOut()
	}
}

// moveWithHeading is EntityPlayer.moveEntityWithHeading together with EntityLivingBase's.
func (g *game) moveWithHeading(strafe, forward float32, inWater, inLava bool) {
	if g.flying {
		keep := g.vel[1]
		factor := g.flySpeedNow()
		if g.sprinting {
			factor *= 2
		}
		g.livingMove(strafe, forward, factor, false, false)
		g.vel[1] = keep * 0.6
		return
	}
	factor := airFactor // a player that has not ended a tick yet has the default
	if g.speedsSet {
		factor = g.jumpFactor
	}
	g.livingMove(strafe, forward, factor, inWater, inLava)
}

// flySpeedNow is how fast the player flies: 0.05, or what the server's abilities say.
func (g *game) flySpeedNow() float32 {
	if g.flyCap > 0 {
		return g.flyCap
	}
	return flySpeed
}

// setSprinting is EntityPlayerSP.setSprinting: a sprint is given 600 ticks, and none when it ends.
func (g *game) setSprinting(on bool) {
	g.sprinting = on
	g.sprintTicks = 0
	if on {
		g.sprintTicks = sprintLasts
	}
}

// setMoveSpeeds is what EntityPlayer.onLivingUpdate does after the player has moved: it keeps the air
// factor (sprinting adds 30% of it) and the movement speed attribute for the next tick to move with. So a
// sprint starts to count one tick after it begins, and one tick after it ends.
func (g *game) setMoveSpeeds() {
	g.speedsSet = true
	g.jumpFactor = airFactor
	if g.sprinting {
		g.jumpFactor = float32(float64(g.jumpFactor) + float64(airFactor)*sprintBonus)
	}
	g.aiSpeed = g.groundSpeed()
}

// sendAbilities tells a server that the player started or stopped flying (serverbound Player Abilities).
func (g *game) sendAbilities() {
	if g.net == nil {
		return
	}
	flags := g.abilFlags & (1 | 8) // invulnerable and creative, as the server last set them (C13PacketPlayerAbilities)
	if g.flying {
		flags |= 2
	}
	if g.canFly() {
		flags |= 4
	}
	w := newPacket(0x13)
	w.Byte(flags)
	w.Float(g.flySpeedNow())
	w.Float(g.walkSpeedNow())
	g.net.c.send(w)
}

// walkSpeedNow is the walking speed of the abilities: 0.1, or what the server set.
func (g *game) walkSpeedNow() float32 {
	if g.walkCap > 0 {
		return g.walkCap
	}
	return landSpeed
}

func (g *game) livingMove(strafe, forward, airMove float32, inWater, inLava bool) {
	switch {
	case inWater && !g.flying:
		y0 := g.feet[1]
		g.moveFlying(strafe, forward, 0.02)
		g.moveEntity(g.vel[0], g.vel[1], g.vel[2])
		g.vel[0], g.vel[2] = g.vel[0]*waterY, g.vel[2]*waterY // 0.8F, sprinting or not (depth strider aside)
		g.vel[1] = g.vel[1]*waterY - 0.02
		g.ledgeHop(y0)
	case inLava && !g.flying:
		y0 := g.feet[1]
		g.moveFlying(strafe, forward, 0.02)
		g.moveEntity(g.vel[0], g.vel[1], g.vel[2])
		g.vel[0], g.vel[1], g.vel[2] = g.vel[0]*0.5, g.vel[1]*0.5-0.02, g.vel[2]*0.5
		g.ledgeHop(y0)
	default:
		slip := float32(0.91)
		if g.onGround {
			slip = slipperiness(g.blockUnderFeet()) * 0.91
		}
		f := 0.16277136 / (slip * slip * slip)
		var friction float32
		if g.onGround {
			friction = g.aiMoveSpeed() * f
		} else {
			friction = airMove
		}
		g.moveFlying(strafe, forward, friction)
		slip = 0.91
		if g.onGround {
			slip = slipperiness(g.blockUnderFeet()) * 0.91
		}
		ladder := g.onLadder()
		if ladder {
			g.vel[0] = max(-ladderLimit, min(ladderLimit, g.vel[0]))
			g.vel[2] = max(-ladderLimit, min(ladderLimit, g.vel[2]))
			g.fall = 0
			if g.vel[1] < -0.15 {
				g.vel[1] = -0.15
			}
			if g.sneaking && g.vel[1] < 0 {
				g.vel[1] = 0
			}
		}
		g.moveEntity(g.vel[0], g.vel[1], g.vel[2])
		if g.collidedH && g.onLadder() { // asked again: the move may have taken the player off the ladder
			g.vel[1] = 0.2
		}
		g.vel[1] = (g.vel[1] - 0.08) * dragY
		g.vel[0], g.vel[2] = g.vel[0]*float64(slip), g.vel[2]*float64(slip)
	}
}

// aiMoveSpeed is the movement speed the player walks with this tick: the one the last tick ended with.
func (g *game) aiMoveSpeed() float32 {
	if !g.speedsSet {
		return 0
	}
	return g.aiSpeed
}

// ledgeHop is the small jump that lifts a swimmer out of the water onto a ledge.
func (g *game) ledgeHop(y0 float64) {
	if !g.collidedH {
		return
	}
	at := playerBox(g.feet).offset(vec3{g.vel[0], g.vel[1] + ledgeLift - g.feet[1] + y0, g.vel[2]})
	if g.anyBlock(at) {
		return
	}
	if g.anyCell(at, func(id uint16) bool { return isFluid(id) }) {
		return
	}
	g.vel[1] = ledgeHopY
}

// moveEntity is Entity.moveEntity: it moves the box and stops it against blocks, y first, then x,
// then z, with the step up onto low ledges and a sneaking player held back from edges.
func (g *game) moveEntity(x, y, z float64) {
	if g.inWeb {
		g.inWeb = false
		x, y, z = x*0.25, y*webY, z*0.25
		g.vel = vec3{}
	}
	start := playerBox(g.feet)
	mv := vec3{x, y, z}
	if g.onGround && g.sneaking {
		mv = g.edgeClamp(start, mv)
	}
	ox, oy, oz := mv[0], mv[1], mv[2]
	obs := g.boxesIn(start.expand(mv))

	box := start
	box, _ = pushAxis(box, obs, 1, &mv[1])
	box, _ = pushAxis(box, obs, 0, &mv[0])
	box, _ = pushAxis(box, obs, 2, &mv[2])
	nx, ny, nz := mv[0], mv[1], mv[2]

	if (g.onGround || (oy != ny && oy < 0)) && (nx != ox || nz != oz) {
		after, ax, ay, az := box, nx, ny, nz
		list := g.boxesIn(start.expand(vec3{ox, stepUp, oz}))
		wide := start.expand(vec3{ox, 0, oz}) // for the rise: the ground ahead counts too
		rise := clip(wide, list, 1, stepUp)
		b1 := start.offset(vec3{0, rise, 0})
		d15 := clip(b1, list, 0, ox)
		b1 = b1.offset(vec3{d15, 0, 0})
		d16 := clip(b1, list, 2, oz)
		b1 = b1.offset(vec3{0, 0, d16})

		rise2 := clip(start, list, 1, stepUp)
		b2 := start.offset(vec3{0, rise2, 0})
		d18 := clip(b2, list, 0, ox)
		b2 = b2.offset(vec3{d18, 0, 0})
		d19 := clip(b2, list, 2, oz)
		b2 = b2.offset(vec3{0, 0, d19})

		var sx, sy, sz float64
		var sb aabb
		if d15*d15+d16*d16 > d18*d18+d19*d19 {
			sx, sz, sy, sb = d15, d16, -rise, b1
		} else {
			sx, sz, sy, sb = d18, d19, -rise2, b2
		}
		sy = clip(sb, list, 1, sy)
		sb = sb.offset(vec3{0, sy, 0})
		if ax*ax+az*az >= sx*sx+sz*sz { // no better than the plain move: keep that
			sb, sx, sy, sz = after, ax, ay, az
		}
		box, nx, ny, nz = sb, sx, sy, sz
	}
	g.feet = vec3{(box.min[0] + box.max[0]) / 2, box.min[1], (box.min[2] + box.max[2]) / 2}

	g.collidedH = ox != nx || oz != nz
	vertical := oy != ny
	g.onGround = vertical && oy < 0
	g.updateFall(ny)
	if ox != nx {
		g.vel[0] = 0
	}
	if oz != nz {
		g.vel[2] = 0
	}
	if vertical {
		g.vel[1] = 0
	}
	g.blockCollisions()
}

// pushAxis moves a box along one axis by *d, clipped by the obstacles, and stores the move back.
func pushAxis(box aabb, obs []aabb, axis int, d *float64) (aabb, float64) {
	*d = clip(box, obs, axis, *d)
	var step vec3
	step[axis] = *d
	return box.offset(step), *d
}

// updateFall is Entity.updateFallState: distance fallen builds up in the air and is spent on landing.
func (g *game) updateFall(dy float64) {
	if g.fluidIn(false) || g.flying || g.onLadder() {
		g.fall = 0
		return
	}
	if g.onGround {
		if g.fall > 3 {
			g.hurt(float32(math.Ceil(float64(g.fall - 3))))
		}
		g.fall = 0
	} else if dy < 0 {
		g.fall = float32(float64(g.fall) - dy) // fallDistance is a float
	}
}

// blockCollisions is doBlockCollisions: what the blocks the body is inside do to it. Soul sand
// slows it by 60% and a cobweb catches it for the next move.
func (g *game) blockCollisions() {
	b := playerBox(g.feet)
	b.min = b.min.add(vec3{0.001, 0.001, 0.001})
	b.max = b.max.sub(vec3{0.001, 0.001, 0.001})
	if g.anyCell(b, func(id uint16) bool { return id == 88 }) {
		g.vel[0], g.vel[2] = g.vel[0]*0.4, g.vel[2]*0.4
	}
	if g.anyCell(b, func(id uint16) bool { return id == 30 }) {
		g.inWeb = true
	}
}

// canFly is whether the player may fly: in creative mode, or when a server's abilities say so.
func (g *game) canFly() bool { return g.creative || g.allowFly }
