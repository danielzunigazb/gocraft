package client

import (
	"math"

	"gocraft/anvil"
	"gocraft/world"
)

// Mobs: living things that move, think and can be hurt. Movement follows EntityLivingBase (the
// same friction and gravity the player has), hurting follows attackEntityFrom (invulnerability
// frames, knockback) and the kinds below are the ones that live in the player's worlds.

type mobKind struct {
	id          string  // the id the game's save files give it
	w, h        float32 // the size of its box
	health      float32
	speed       float32 // the movement speed attribute
	hostile     bool
	tex, fur    string // paths under textures/entity
	model, fm   *mmodel
	pose        func(m *mob, ls, lsa, age, headYaw, headPitch float32) []rot
	attack      float32  // damage of one blow on Normal
	follow      float32  // how far it notices the player
	interval    int      // ticks between its blows; 0 is a second
	flee        float32  // the speed multiplier of EntityAIPanic when hurt; 0 is 1.25
	scale       float32  // how big its model is drawn; 0 is 1
	climbs      bool     // a spider goes up walls
	leaps       bool     // and jumps at its target
	day         bool     // neutral in daylight (a spider)
	slime       bool     // a slime: hops, comes in sizes and splits
	flies       bool     // a bat: no gravity
	texVariants []string // textures by the mob's colour: villager professions, rabbit coats
	swims       bool     // a squid: lives in water
	hops        bool     // a rabbit: moves only by hopping
	yoff        float32  // its model is drawn this much higher
	dropsFn     func() []slot
}

type mob struct {
	kind                  *mobKind
	pos, prev, vel        vec3
	yaw, prevYaw          float32 // body, degrees, 0 faces south
	headYaw, pitch        float32 // relative to the body
	health                float32
	hurtTime, hurtResist  int
	lastDamage            float32
	deathTime             int
	onGround, collidedH   bool
	limbSwing, limbAmount float32
	age                   int
	colour                int     // sheep: the fleece colour
	tex                   string  // a texture other than its kind's (a zombie pigman is drawn as a zombie)
	drawScale             float32 // how much bigger it is drawn than its model; 0 is 1 (a server's baby, a wither skeleton)

	// what it is doing
	wanderTicks int
	wanderYaw   float32
	panic       int
	fuse        int
	attackTick  int
	burning     int
	dropsGiven  bool
	persist     bool // it does not despawn
	despawnAge  int

	sheared            bool     // a sheep with no wool
	size               int      // a slime's size: 1, 2 or 4
	eatTicks           int      // a sheep eating grass
	eggTimer           int      // ticks until a chicken lays an egg
	lookTicks          int      // how long it keeps looking at the player
	jumpDelay          int      // ticks until a slime hops again
	angry              bool     // a spider that has turned on the player in daylight
	path               []node   // where an A* search says to go
	pathAt, pathStep   int      // the tick it was made and the waypoint it is heading for
	pathGoal           [3]int32 // the block the path leads to
	headYawT, headPitT float32  // where the head is turning to
	target             vec3     // where a bat is flying to
	swimVec            vec3     // where a squid is swimming
}

var kinds = map[string]*mobKind{}

// dims are the width and height of its box: a slime's grow with its size.
func (m *mob) dims() (w, h float32) {
	if m.kind.slime {
		s := float32(max(m.size, 1))
		return 0.6 * s, 0.6 * s
	}
	return m.kind.w, m.kind.h
}

func (m *mob) box() aabb {
	w, h := m.dims()
	hw, hh := float64(w/2), float64(h) // widths and heights are floats in the game
	return aabb{vec3{m.pos[0] - hw, m.pos[1], m.pos[2] - hw}, vec3{m.pos[0] + hw, m.pos[1] + hh, m.pos[2] + hw}}
}

func (m *mob) dead() bool { return m.health <= 0 }

func (g *game) newMob(k *mobKind, pos vec3, yaw float32) *mob {
	m := &mob{kind: k, pos: pos, prev: pos, yaw: yaw, prevYaw: yaw, health: k.health, onGround: true}
	return m
}

// ---- hurting, after EntityLivingBase.attackEntityFrom and knockBack ----

// knockBack pushes the mob away from a point: its motion is halved, then thrown back and up.
func (m *mob) knockBack(fromX, fromZ float64) {
	d := float64(float32(math.Sqrt(fromX*fromX + fromZ*fromZ))) // MathHelper.sqrt_double returns a float
	if d == 0 {
		return
	}
	m.vel[0], m.vel[1], m.vel[2] = m.vel[0]/2, m.vel[1]/2, m.vel[2]/2
	m.vel[0] -= fromX / d * knockPower
	m.vel[1] += knockPower
	m.vel[2] -= fromZ / d * knockPower
	if m.vel[1] > knockPower {
		m.vel[1] = knockPower
	}
}

// hurt applies a blow from a source at attackerPos; it reports whether it counted. A second blow
// within half the invulnerability time only counts for what it adds to the first.
func (m *mob) hurt(amount float32, attackerPos vec3) bool {
	if m.dead() {
		return false
	}
	m.limbAmount = 1.5
	fresh := true
	if m.hurtResist > 10 {
		if amount <= m.lastDamage {
			return false
		}
		m.health -= amount - m.lastDamage
		m.lastDamage = amount
		fresh = false
	} else {
		m.lastDamage = amount
		m.hurtResist = 20
		m.health -= amount
		m.hurtTime = 10
	}
	if fresh {
		dx, dz := attackerPos[0]-m.pos[0], attackerPos[2]-m.pos[2]
		for dx*dx+dz*dz < 1e-4 {
			dx, dz = float64(rng.Float32()-rng.Float32())*0.01, float64(rng.Float32()-rng.Float32())*0.01
		}
		m.knockBack(dx, dz)
		if !m.kind.hostile {
			m.panic = 60
		}
		if m.kind.day { // a spider hurt in daylight turns on its attacker
			m.angry = true
		}
	}
	return true
}

// ---- movement ----

func yawVector(yawDeg float32) (sin, cos float32) {
	a := yawDeg * math.Pi / 180
	return mhSin(a), mhCos(a)
}

// tickMob advances a mob one tick: thinking, then moving as EntityLivingBase does.
func (g *game) tickMob(m *mob) {
	m.prev, m.prevYaw = m.pos, m.yaw
	m.age++
	if m.hurtResist > 0 {
		m.hurtResist--
	}
	if m.hurtTime > 0 {
		m.hurtTime--
	}
	if m.dead() {
		m.deathTime++
		m.vel[0], m.vel[2] = m.vel[0]*0.9, m.vel[2]*0.9
	}
	forward, jump := float32(0), false
	if !m.dead() {
		g.tickMobExtras(m)
		forward, jump = g.think(m)
	}
	if m.kind.climbs && m.collidedH && !m.dead() { // a spider climbs what it walks into
		m.vel[1] = 0.2
	}
	for i := range m.vel {
		if m.vel[i] < 0.005 && m.vel[i] > -0.005 {
			m.vel[i] = 0
		}
	}
	inWater := g.world.Block(int32(math.Floor(float64(m.pos[0]))), int32(math.Floor(float64(m.pos[1]+0.4))), int32(math.Floor(float64(m.pos[2]))))>>4 == 9 ||
		g.world.Block(int32(math.Floor(float64(m.pos[0]))), int32(math.Floor(float64(m.pos[1]+0.4))), int32(math.Floor(float64(m.pos[2]))))>>4 == 8
	if jump {
		if inWater {
			m.vel[1] += 0.04
		} else if m.onGround {
			m.vel[1] = jumpMotion
		}
	}
	if forward != 0 {
		f := forward * 0.98
		// moveFlying: the speed multiplies the input twice, as the game's move helper sets both.
		slip := float32(0.91)
		if m.onGround {
			slip = slipperiness(g.world.Block(int32(math.Floor(float64(m.pos[0]))), int32(math.Floor(float64(m.pos[1])))-1, int32(math.Floor(float64(m.pos[2]))))>>4) * 0.91
		}
		friction := forward
		if !m.onGround {
			friction = 0.02
		} else {
			friction = forward * (0.16277136 / (slip * slip * slip))
		}
		n := f * f
		if n >= 1e-4 {
			n = float32(math.Sqrt(float64(n)))
			if n < 1 {
				n = 1
			}
			k := friction / n
			s, c := yawVector(m.yaw)
			m.vel[0] += float64(-f * k * s)
			m.vel[2] += float64(f * k * c)
		}
	}
	slip := float32(0.91)
	if m.onGround {
		slip = slipperiness(g.world.Block(int32(math.Floor(float64(m.pos[0]))), int32(math.Floor(float64(m.pos[1])))-1, int32(math.Floor(float64(m.pos[2]))))>>4) * 0.91
	}
	box, moved := g.slide(m.box(), m.vel)
	m.pos = vec3{(box.min[0] + box.max[0]) / 2, box.min[1], (box.min[2] + box.max[2]) / 2}
	m.collidedH = moved[0] != m.vel[0] || moved[2] != m.vel[2]
	m.onGround = moved[1] != m.vel[1] && m.vel[1] < 0
	if moved[0] != m.vel[0] {
		m.vel[0] = 0
	}
	if moved[2] != m.vel[2] {
		m.vel[2] = 0
	}
	if moved[1] != m.vel[1] {
		m.vel[1] = 0
	}
	if inWater {
		m.vel[0], m.vel[1], m.vel[2] = m.vel[0]*waterY, m.vel[1]*waterY-0.02, m.vel[2]*waterY
	} else {
		if m.kind.flies {
			m.vel[1] *= 0.6
		} else {
			m.vel[1] = (m.vel[1] - 0.08) * dragY
		}
		m.vel[0], m.vel[2] = m.vel[0]*float64(slip), m.vel[2]*float64(slip)
	}
	// Limb swing follows how far it moved this tick.
	d := float32(math.Hypot(float64(m.pos[0]-m.prev[0]), float64(m.pos[2]-m.prev[2]))) * 4
	if d > 1 {
		d = 1
	}
	m.limbAmount += (d - m.limbAmount) * 0.4
	m.limbSwing += m.limbAmount
	g.mobSounds(m)
	if m.pos[1] < -64 {
		m.health = 0
		m.deathTime = 20
	}
}

// ---- thinking ----

func (g *game) playerTargetable() bool { return !g.creative && g.health > 0 }

func (g *game) distToPlayer(m *mob) float32 {
	dx, dz := g.feet[0]-m.pos[0], g.feet[2]-m.pos[2]
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

func turnToward(from, to, maxStep float32) float32 {
	d := float32(math.Mod(float64(to-from)+540, 360)) - 180
	if d > maxStep {
		d = maxStep
	}
	if d < -maxStep {
		d = -maxStep
	}
	return from + d
}

func faceYaw(dx, dz float64) float32 {
	return float32(math.Atan2(float64(dz), float64(dx)))*180/math.Pi - 90
}

// think decides what a mob does this tick: how hard it walks and whether it jumps.
func (g *game) think(m *mob) (forward float32, jump bool) {
	k := m.kind
	if k.slime || k.hops || k.flies || k.swims { // they move by their own rules (tickMobExtras)
		return 0, false
	}
	if k.hostile {
		if g.playerTargetable() && g.distToPlayer(m) < k.follow && (!k.day || g.spiderTarget(m)) {
			return g.chase(m)
		}
		m.fuse = max(m.fuse-1, 0)
		return g.wander(m)
	}
	if m.panic > 0 { // run away from where it was hit
		m.panic--
		m.wanderTicks = 0
		sx, sz := m.pos[0]-g.feet[0], m.pos[2]-g.feet[2]
		m.yaw = turnToward(m.yaw, faceYaw(sx, sz), 30) // face away from where it was hit
		if g.hazardAhead(m, m.yaw) {
			m.yaw += 90 // not into the water, off a cliff
		}
		flee := k.flee
		if flee == 0 {
			flee = 1.25
		}
		return k.speed * flee, m.collidedH && m.onGround
	}
	return g.wander(m)
}

// wander walks in a random direction for a while, then stands, as EntityAIWander and
// EntityAILookIdle make an animal do.
func (g *game) wander(m *mob) (float32, bool) {
	if m.wanderTicks <= 0 {
		if rng.Intn(120) == 0 { // a new idea about once in six seconds
			m.wanderYaw = rng.Float32() * 360
			m.wanderTicks = 40 + rng.Intn(60)
		}
		return 0, false
	}
	m.wanderTicks--
	m.yaw = turnToward(m.yaw, m.wanderYaw, 20)
	if g.hazardAhead(m, m.yaw) { // water, lava or a drop ahead: stop and think again
		m.wanderTicks = 0
		return 0, false
	}
	return m.kind.speed, m.collidedH && m.onGround
}

// chase goes for the player and hits when close; a creeper swells instead.
func (g *game) chase(m *mob) (float32, bool) {
	k := m.kind
	yaw, stepJump := g.pathSteer(m, g.feet)
	m.yaw = turnToward(m.yaw, yaw, 30)
	m.headYaw = 0
	g.leapAtPlayer(m)
	dist := g.distToPlayer(m)
	if m.attackTick > 0 {
		m.attackTick--
	}
	if k.id == "Creeper" {
		if dist < 3 {
			if m.fuse++; m.fuse >= 30 {
				g.explode(m.pos, 3, m)
				m.health, m.deathTime = 0, 20
				m.dropsGiven = true
			}
			return 0, false
		}
		m.fuse = max(m.fuse-1, 0)
	} else if k.id == "Skeleton" {
		// EntityAIArrowAttack(1.0, 20, 60, 15): stop and shoot inside 15 blocks, with a view of the player.
		eye := vec3{m.pos[0], m.pos[1] + float64(k.h)*0.85, m.pos[2]}
		_, blocked := g.rayTrace(eye, vec3{g.feet[0], g.feet[1] + playerH*0.9, g.feet[2]})
		if dist < 15 && !blocked {
			m.headYaw = 0
			if m.attackTick <= 0 {
				f := min(max(dist/15, 0.1), 1)
				g.shootFromMob(m, f)
				m.attackTick = int(f*(60-20) + 20)
			}
			return 0, false
		}
	} else if dist < k.w*2+g.playerWidth() && math.Abs(float64(g.feet[1]-m.pos[1])) < 1.5 && m.attackTick <= 0 {
		m.attackTick = cmpOr(k.interval, 20)
		g.hurtPlayer(k.attack, m.pos, k.id == "PvPBot")
	}
	return k.speed, stepJump || m.collidedH && m.onGround
}

func (g *game) playerWidth() float32 { return playerW }

// ---- the world's mobs ----

func (g *game) tickMobs() {
	g.be.pathBudget = 2
	live := g.mobs[:0]
	for _, m := range g.mobs {
		dx, dz := m.pos[0]-g.feet[0], m.pos[2]-g.feet[2]
		d2 := dx*dx + dz*dz
		if g.despawns(m, d2) { // hostile mobs go when the player is far from them
			continue
		}
		m.despawnAge++
		if d2 > 128*128 { // a far mob is not simulated, as in the game
			live = append(live, m)
			continue
		}
		g.tickMob(m)
		if m.dead() && m.deathTime == 1 && !m.dropsGiven {
			m.dropsGiven = true
			if m.kind.slime {
				g.splitSlime(m)
			}
			for _, d := range g.mobDrops(m) {
				g.spawnDrop(m.pos[0]-0.5, m.pos[1], m.pos[2]-0.5, d)
			}
		}
		if m.deathTime < 20 {
			live = append(live, m)
		}
	}
	g.mobs = append(live, g.be.newMobs...)
	g.be.newMobs = nil
}

// ---- reading mobs from a saved world ----

func nbtFloats(v any, n int) []float64 {
	l, _ := v.([]any)
	out := make([]float64, n)
	for i := 0; i < n && i < len(l); i++ {
		out[i], _ = anvil.Number(l[i])
	}
	return out
}

// loadEntities brings the saved animals, monsters and dropped items of a column into the game.
func (g *game) loadEntities(cp world.ChunkPos) {
	if g.save == nil {
		return
	}
	g.loadSigns(cp)        // signs are not put away with the mobs: read their text every time
	if g.be.resolved[cp] { // read from the world file once; from then on the overlay has them
		g.spawnStored(cp)
		return
	}
	if g.be.resolved == nil {
		g.be.resolved = map[world.ChunkPos]bool{}
	}
	g.be.resolved[cp] = true
	root, err := g.save.RawChunk(cp)
	if root == nil || err != nil {
		return
	}
	level, _ := root["Level"].(anvil.Compound)
	ents, _ := level["Entities"].([]any)
	for _, e := range ents {
		c, _ := e.(anvil.Compound)
		id, _ := c["id"].(string)
		p := nbtFloats(c["Pos"], 3)
		pos := vec3{p[0], p[1], p[2]}
		if id == "Item" {
			it, _ := c["Item"].(anvil.Compound)
			iid, _ := anvil.Number(it["id"])
			n, _ := anvil.Number(it["Count"])
			dmg, _ := anvil.Number(it["Damage"])
			if iid > 0 && n > 0 {
				g.items = append(g.items, &itemEntity{stack: slot{uint16(iid), uint16(dmg), int8(n)}, pos: pos, prev: pos, pickupDelay: 10})
			}
			continue
		}
		k := kinds[id]
		if k == nil {
			continue
		}
		rot := nbtFloats(c["Rotation"], 2)
		m := g.newMob(k, pos, float32(rot[0]))
		if h, ok := anvil.Number(c["HealF"]); ok && h > 0 {
			m.health = float32(h)
		} else if h, ok := anvil.Number(c["Health"]); ok && h > 0 {
			m.health = float32(h)
		}
		if col, ok := anvil.Number(c["Color"]); ok {
			m.colour = int(col) & 15
		}
		if rt, ok := anvil.Number(c["RabbitType"]); ok && k.id == "Rabbit" { // the coat
			m.colour = [...]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5}[min(max(int(rt), 0), 5)]
		}
		if pr, ok := anvil.Number(c["Profession"]); ok && k.id == "Villager" {
			m.colour = int(pr)
		}
		if sh, ok := anvil.Number(c["Sheared"]); ok {
			m.sheared = sh != 0
		}
		if p, ok := anvil.Number(c["PersistenceRequired"]); ok {
			m.persist = p != 0
		}
		if k.slime {
			sz, _ := anvil.Number(c["Size"]) // the file keeps the size less one
			m.size = int(sz) + 1
			if m.health = float32(m.size * m.size); m.health <= 0 {
				m.health = 1
			}
		}
		g.mobs = append(g.mobs, m)
	}
}

// loadNearbyEntities reads each column near the player once.
func (g *game) loadNearbyEntities() {
	if g.save == nil || g.dimn.id != dimOverworld { // the saved entities are the Overworld's
		return
	}
	cx, cz := int32(math.Floor(float64(g.feet[0])))>>4, int32(math.Floor(float64(g.feet[2])))>>4
	if g.entityChunks == nil {
		g.entityChunks = map[world.ChunkPos]bool{}
	}
	for dx := int32(-4); dx <= 4; dx++ {
		for dz := int32(-4); dz <= 4; dz++ {
			cp := world.ChunkPos{X: cx + dx, Z: cz + dz}
			if !g.entityChunks[cp] {
				g.entityChunks[cp] = true
				g.loadEntities(cp)
			}
		}
	}
	for cp := range g.entityChunks { // columns left behind put their mobs away
		if abs(cp.X-cx) > 6 || abs(cp.Z-cz) > 6 {
			g.unloadEntities(cp)
		}
	}
}

func cmpOr(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// botPose poses the player model for a bot: walking limbs, and the sword arm swinging after a blow.
func botPose(m *mob, ls, lsa, age, headYaw, headPitch float32) []rot {
	st := poseState{ls: ls, lsa: lsa, age: age, headYaw: headYaw, headPitch: headPitch, swing: -1}
	if m != nil {
		st.airborne = !m.onGround
		if iv := cmpOr(m.kind.interval, 20); m.attackTick > iv-6 {
			st.swing = float32(iv-m.attackTick) / 6
		}
	}
	return playerRots(st)
}

// spawnBot puts a practice opponent a few blocks ahead of the player, facing them.
func (g *game) spawnBot() {
	yaw := g.yawDeg() * deg
	p := vec3{g.feet[0] - float64(mhSin(yaw)*6), g.feet[1], g.feet[2] + float64(mhCos(yaw)*6)}
	if y, ok := g.surfaceAt(int32(math.Floor(float64(p[0]))), int32(math.Floor(float64(p[2])))); ok {
		p[1] = y
	}
	m := g.newMob(kinds["PvPBot"], p, 0)
	g.mobs = append(g.mobs, m)
}
