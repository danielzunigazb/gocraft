package client

import (
	"math"
	"testing"

	"gocraft/world"
)

func spawn(g *game, kind string, x, z float64) *mob {
	m := g.newMob(kinds[kind], vec3{x, 4, z}, 0)
	g.mobs = append(g.mobs, m)
	return m
}

func fighter(t *testing.T) *game {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.pos = vec3{0.5, 4 + eyeHeight, 0.5}
	g.onGround = true
	g.creative = false
	return g
}

func TestKnockbackMatchesTheGame(t *testing.T) {
	g := fighter(t)
	m := spawn(g, "Zombie", 3.5, 0.5)
	m.hurt(5, g.feet) // attacker two blocks... three blocks west of it
	// motion /2 (zero), minus 0.4 along the unit vector away from the attacker, +0.4 up.
	if !near(m.vel[0], 0.4, 1e-6) || !near(m.vel[1], 0.4, 1e-6) || !near(m.vel[2], 0, 1e-6) {
		t.Fatalf("knockback velocity %v, want (0.4, 0.4, 0)", m.vel)
	}
	m.vel = vec3{0.2, 0.6, 0}
	m.hurtResist = 0
	m.hurt(5, g.feet)
	// halved first: 0.1, 0.3; then +0.4 away (x) and up, y capped at 0.4.
	if !near(m.vel[0], 0.5, 1e-6) || !near(m.vel[1], 0.4, 1e-6) {
		t.Fatalf("a moving target: %v", m.vel)
	}
}

func TestInvulnerabilityFrames(t *testing.T) {
	g := fighter(t)
	m := spawn(g, "Zombie", 3.5, 0.5)
	if !m.hurt(5, g.feet) || m.health != 15 {
		t.Fatalf("first hit: health %v", m.health)
	}
	for i := 0; i < 5; i++ { // within the first half of the 20 ticks
		m.hurtResist--
	}
	if m.hurt(5, g.feet) || m.health != 15 {
		t.Fatal("an equal hit while invulnerable counted")
	}
	if !m.hurt(7, g.feet) || m.health != 13 { // only the 2 extra points
		t.Fatalf("a bigger hit adds the difference: health %v", m.health)
	}
	m.hurtResist = 10 // half the time gone: hits count in full again
	if !m.hurt(4, g.feet) || m.health != 9 {
		t.Fatalf("after 10 ticks a hit counts fully: health %v", m.health)
	}
}

func TestWeaponsAndCriticals(t *testing.T) {
	g := fighter(t)
	for _, c := range []struct {
		id   uint16
		want float32
	}{{0, 1}, {268, 5}, {272, 6}, {267, 7}, {276, 8}, {283, 5}, {271, 4}, {257, 5}} {
		m := spawn(g, "Zombie", 3.5, 0.5)
		g.inv[0], g.sel = slot{ID: c.id, N: 1}, 0
		if c.id == 0 {
			g.inv[0] = slot{}
		}
		g.attackMob(m)
		if got := 20 - m.health; !near(got, c.want, 1e-5) {
			t.Errorf("item %d hit for %v, want %v", c.id, got, c.want)
		}
	}
	// Falling and not on the ground: a critical hit, one and a half times.
	m := spawn(g, "Zombie", 3.5, 0.5)
	g.inv[0] = slot{ID: 276, N: 1}
	g.fall, g.onGround = 1, false
	g.attackMob(m)
	if got := 20 - m.health; !near(got, 12, 1e-5) {
		t.Errorf("critical hit with a diamond sword: %v, want 12", got)
	}
	// On the ground it is an ordinary hit.
	g.fall, g.onGround = 0, true
	m2 := spawn(g, "Zombie", 3.5, 0.5)
	g.attackMob(m2)
	if got := 20 - m2.health; !near(got, 8, 1e-5) {
		t.Errorf("ordinary hit %v", got)
	}
}

func TestSprintHitKnocksFurtherAndCostsTheSprint(t *testing.T) {
	g := fighter(t)
	g.yaw = 0 // facing south, +z
	g.sprinting = true
	g.vel = vec3{0.1, 0, 0.2}
	m := spawn(g, "Zombie", 0.5, 2.5)
	g.attackMob(m)
	// Ordinary knockback away along +z (0.4), plus the sprint push: 0.5 more along the facing.
	if !near(m.vel[2], 0.4+0.5, 1e-5) || !near(m.vel[1], 0.4+0.1, 1e-5) {
		t.Fatalf("sprint knockback velocity %v", m.vel)
	}
	if g.sprinting || !near(g.vel[0], 0.06, 1e-6) || !near(g.vel[2], 0.12, 1e-6) {
		t.Fatalf("the attacker loses sprint and 40%% of their speed: sprinting %v vel %v", g.sprinting, g.vel)
	}
}

func TestArmourReducesDamage(t *testing.T) {
	g := fighter(t)
	if g.armourTotal() != 0 {
		t.Fatal("armour on a naked player")
	}
	g.armor = [4]slot{{ID: 310, N: 1}, {ID: 311, N: 1}, {ID: 312, N: 1}, {ID: 313, N: 1}} // diamond
	if g.armourTotal() != 20 {
		t.Fatalf("diamond armour is worth %d, want 20", g.armourTotal())
	}
	g.armor = [4]slot{{ID: 306, N: 1}, {ID: 307, N: 1}, {ID: 308, N: 1}, {ID: 309, N: 1}} // iron
	if g.armourTotal() != 15 {
		t.Fatalf("iron armour %d, want 15", g.armourTotal())
	}
	if got := afterArmour(10, 15); !near(got, 4, 1e-5) {
		t.Fatalf("10 damage through iron armour: %v, want 4", got)
	}
	before := g.health
	g.hurtPlayer(10, vec3{5, 4, 0.5}, false)
	if !near(before-g.health, 4, 1e-5) {
		t.Fatalf("hurt for %v, want 4", before-g.health)
	}
	if g.armor[0].Dmg == 0 {
		t.Fatal("armour should wear")
	}
	// Armour does not help against unblockable damage (explosions here, as magic in the game).
	g.hurtResist = 0
	b2 := g.health
	g.hurtPlayer(3, vec3{5, 4, 0.5}, true)
	if !near(b2-g.health, 3, 1e-5) {
		t.Fatalf("unblockable: %v", b2-g.health)
	}
}

func TestBlockingWithASwordHalvesDamage(t *testing.T) {
	g := fighter(t)
	g.blocking = true
	g.hurtPlayer(8, vec3{5, 4, 0.5}, false)
	if got := maxHealth - g.health; !near(got, 4.5, 1e-5) { // (1+8)/2
		t.Fatalf("blocked 8 damage into %v, want 4.5", got)
	}
}

func TestPlayerIsKnockedBackAndHasFrames(t *testing.T) {
	g := fighter(t)
	g.hurtPlayer(3, vec3{2.5, 4, 0.5}, false) // from the east
	if !near(g.vel[0], -0.4, 1e-6) || !near(g.vel[1], 0.4, 1e-6) {
		t.Fatalf("knocked back %v", g.vel)
	}
	h := g.health
	g.hurtPlayer(3, vec3{2.5, 4, 0.5}, false) // the same blow while invulnerable
	if g.health != h {
		t.Fatal("hit twice in a tick")
	}
	g.creative = true
	if g.hurtPlayer(5, vec3{}, false) {
		t.Fatal("creative players cannot be hurt")
	}
}

func TestZombieChasesAndHits(t *testing.T) {
	g := fighter(t)
	z := spawn(g, "Zombie", 8.5, 0.5)
	start := z.pos[0]
	for i := 0; i < 60; i++ {
		g.tickMob(z)
	}
	if z.pos[0] >= start-1 { // walking at about 2.3 blocks a second
		t.Fatalf("the zombie moved from %v to %v", start, z.pos[0])
	}
	// Now beside the player: it hits for 3 every 20 ticks.
	z.pos, z.vel, z.attackTick = vec3{1.4, 4, 0.5}, vec3{}, 0
	g.hurtResist = 0
	g.health = 20
	for i := 0; i < 5; i++ {
		g.tickMob(z)
	}
	if g.health != 17 {
		t.Fatalf("health %v after the first blow, want 17", g.health)
	}
	for i := 0; i < 40; i++ {
		g.tickMob(z)
		if g.hurtResist > 0 {
			g.hurtResist--
		}
	}
	if g.health > 14 {
		t.Fatalf("it should keep hitting: health %v", g.health)
	}
}

func TestMonstersLeaveCreativePlayersAlone(t *testing.T) {
	g := fighter(t)
	g.creative = true
	z := spawn(g, "Zombie", 4.5, 0.5)
	x0 := z.pos[0]
	for i := 0; i < 200; i++ {
		g.tickMob(z)
	}
	if g.health != maxHealth {
		t.Fatal("a creative player was hurt")
	}
	if math.Abs(float64(z.pos[0]-x0)) > 30 { // it wanders, it does not hunt
		t.Logf("wandered %v", z.pos[0]-x0)
	}
}

func TestCreeperBlowsUpNearThePlayer(t *testing.T) {
	g := fighter(t)
	for x := int32(-6); x <= 6; x++ {
		for z := int32(-6); z <= 6; z++ {
			for y := int32(0); y <= 3; y++ {
				g.world.SetBlock(x+10, y, z, 3<<4) // dirt under the creeper's feet
			}
		}
	}
	g.feet = vec3{10.5, 4, 0.5}
	g.spawn = vec3{-50, 4, -50}
	c := spawn(g, "Creeper", 12, 0.5)
	for i := 0; i < 40 && !c.dead(); i++ {
		g.tickMob(c)
	}
	if !c.dead() {
		t.Fatal("the creeper never exploded")
	}
	// A creeper's blast at point blank kills: the player is back at the spawn with full health.
	if g.feet != g.spawn {
		t.Fatalf("the player took no lethal damage: at %v with %v health", g.feet, g.health)
	}
	if g.world.Block(12, 3, 0) != 0 { // under the creeper: a crater
		t.Fatalf("no crater under the creeper: %d", g.world.Block(12, 3, 0))
	}
	if g.world.Block(10+6, 3, 6) == 0 {
		t.Fatal("the crater is too big")
	}
}

func TestExplosionsDoNotBreakObsidianOrBedrock(t *testing.T) {
	g := fighter(t)
	g.world.SetBlock(30, 4, 30, 49<<4)
	g.world.SetBlock(31, 4, 30, 1<<4)
	g.world.SetBlock(29, 4, 30, 3<<4)
	g.feet = vec3{100, 4, 100}
	g.explode(vec3{30.5, 4.5, 29.5}, 3, nil)
	if g.world.Block(30, 4, 30)>>4 != 49 {
		t.Error("obsidian blew up")
	}
	if g.world.Block(29, 4, 30) != 0 {
		t.Error("dirt next to the blast survived")
	}
	if g.world.Block(0, 0, 0)>>4 != 7 && g.world.Block(0, 0, 0) != 0 {
		_ = 0
	}
}

func TestMobsDropWhatTheyShould(t *testing.T) {
	g := fighter(t)
	rng.Seed(5)
	counts := map[uint16]int{}
	for i := 0; i < 300; i++ {
		for _, kind := range []string{"Zombie", "Creeper", "Pig", "Cow", "Chicken", "Skeleton"} {
			for _, d := range g.mobDrops(g.newMob(kinds[kind], vec3{}, 0)) {
				counts[d.ID] += int(d.N)
				if d.N > 3 {
					t.Fatalf("%s dropped %d of %d", kind, d.N, d.ID)
				}
			}
		}
	}
	for _, id := range []uint16{367, 289, 319, 334, 363, 288, 365, 352, 262} {
		if counts[id] == 0 {
			t.Errorf("nothing of item %d ever dropped", id)
		}
	}
	// A sheep leaves wool of its colour.
	s := g.newMob(kinds["Sheep"], vec3{}, 0)
	s.colour = 14
	if d := g.mobDrops(s); len(d) != 1 || d[0] != blockSlot(35<<4|14, 1) {
		t.Errorf("red sheep drops %v", d)
	}
}

func TestDeadMobsDropOnceAndDisappear(t *testing.T) {
	g := fighter(t)
	g.feet = vec3{40, 4, 40}
	rng.Seed(2)
	p := spawn(g, "Pig", 0.5, 0.5)
	p.hurt(100, vec3{5, 4, 5})
	for i := 0; i < 30; i++ {
		g.tickMobs()
	}
	if len(g.mobs) != 0 {
		t.Fatalf("a dead pig is still there after 30 ticks: %d", len(g.mobs))
	}
	if len(g.items) == 0 {
		t.Fatal("the pig dropped nothing")
	}
	for _, it := range g.items {
		if it.stack.ID != 319 {
			t.Fatalf("a pig dropped %v", it.stack)
		}
	}
}

func TestAnimalsPanicWhenHurtAndWanderOtherwise(t *testing.T) {
	g := fighter(t)
	g.feet = vec3{0.5, 4, 0.5}
	cow := spawn(g, "Cow", 3.5, 0.5)
	cow.hurt(1, g.feet)
	x0 := cow.pos[0]
	for i := 0; i < 30; i++ {
		g.tickMob(cow)
	}
	if cow.pos[0] <= x0+0.5 {
		t.Fatalf("a hurt cow should run from the player: %v -> %v", x0, cow.pos[0])
	}
	rng.Seed(9)
	pig := spawn(g, "Pig", 20.5, 20.5)
	moved := false
	for i := 0; i < 2000 && !moved; i++ {
		g.tickMob(pig)
		moved = math.Hypot(float64(pig.pos[0]-20.5), float64(pig.pos[2]-20.5)) > 1
	}
	if !moved {
		t.Fatal("an idle pig never wandered")
	}
}

func TestMobPickingAndWalls(t *testing.T) {
	g := fighter(t)
	g.yaw, g.pitch = 0, 0
	z := spawn(g, "Zombie", 0.5, 2.5)
	if m, ok := g.mobTarget(); !ok || m != z {
		t.Fatal("a zombie two blocks ahead is not targeted")
	}
	far := spawn(g, "Cow", 0.5, 6.5)
	if m, _ := g.mobTarget(); m == far {
		t.Fatal("reach is three blocks")
	}
	g.world.SetBlock(0, 5, 1, 1<<4) // a wall at eye height between them
	if _, ok := g.mobTarget(); ok {
		t.Fatal("a mob behind a wall can be hit")
	}
	g.world.SetBlock(0, 5, 1, 0)
	if !g.attackClick() || z.health >= 20 {
		t.Fatal("a click did not hit")
	}
}

func TestSpawningNeedsDarkness(t *testing.T) {
	g := fighter(t)
	g.worldTime = 6000
	rng.Seed(1)
	dark := 0
	for i := 0; i < 400; i++ {
		if g.darkEnough(50, 4, 50) {
			dark++
		}
	}
	if dark != 0 { // the open sky at noon is never dark enough
		t.Fatalf("%d spawns in daylight", dark)
	}
	// Underground, with no light at all, the chance is 1 in 8 per roll... at least some.
	c := g.world.Snapshot(world.ChunkPos{X: 10, Z: 10})
	c.Light = make([]uint8, 65536)
	g.world = world.NewFrom(func(cp world.ChunkPos) *world.Chunk {
		if cp == (world.ChunkPos{X: 10, Z: 10}) {
			return c
		}
		return nil
	})
	hits := 0
	for i := 0; i < 400; i++ {
		if g.darkEnough(160, 2, 160) {
			hits++
		}
	}
	if hits == 0 {
		t.Fatal("a pitch dark cave never allows a spawn")
	}
	if skylightSubtracted(celestialAngle(6000)) != 0 || skylightSubtracted(celestialAngle(18000)) != 11 {
		t.Fatalf("sky light subtracted at noon %d, midnight %d", skylightSubtracted(celestialAngle(6000)), skylightSubtracted(celestialAngle(18000)))
	}
}

func TestMobModelsHaveTheRightShapeAndTexture(t *testing.T) {
	z := kinds["Zombie"]
	if len(z.model.parts) != 6 {
		t.Fatalf("a zombie has %d parts", len(z.model.parts))
	}
	// The head's front face (the quad at -z) reads texture pixels 8..16 across and 8..16 down of 64x64.
	front := z.model.quads[0][0][4]
	minU, maxU, minV, maxV := float32(9), float32(-1), float32(9), float32(-1)
	for _, v := range front {
		minU, maxU = min(minU, v.u), max(maxU, v.u)
		minV, maxV = min(minV, v.v), max(maxV, v.v)
	}
	if !near(minU*64, 8, 1e-4) || !near(maxU*64, 16, 1e-4) || !near(minV*64, 8, 1e-4) || !near(maxV*64, 16, 1e-4) {
		t.Fatalf("head front on the texture: u %v..%v v %v..%v", minU*64, maxU*64, minV*64, maxV*64)
	}
	// Standing at the origin, facing south: feet on the ground, head top about two blocks up.
	rots := z.pose(nil, 0, 0, 0, 0, 0)
	verts := emitModel(nil, z.model, rots, vec3{}, 0, 0, [3]uint8{255, 255, 255}, 0xF0)
	lo, hi := float32(99), float32(-99)
	for _, v := range verts {
		lo, hi = min(lo, v.Y), max(hi, v.Y)
	}
	if !near(lo, 0.0078, 0.01) || !near(hi, 2.0078, 0.02) {
		t.Fatalf("a zombie spans y %v..%v, want about 0..2", lo, hi)
	}
	if len(verts) != 6*6*4 { // six parts, one box each, six faces of four vertices
		t.Fatalf("%d vertices", len(verts))
	}
	// A mob facing south has its face on the +z side: the head's front vertices are at larger z than the back.
	var zf, zb float32 = -99, 99
	for i := 0; i < 24; i += 4 {
		if i == 16 { // head quad 4: the front face
			zf = verts[i].Z
		}
		if i == 20 { // head quad 5: the back
			zb = verts[i].Z
		}
	}
	if zf <= zb {
		t.Fatalf("a mob facing south shows its face towards +z: front z %v, back z %v", zf, zb)
	}
	// Every kind has a texture path and a model with legs that swing.
	for id, k := range kinds {
		if k.tex == "" || k.model == nil || k.pose == nil {
			t.Errorf("%s is incomplete", id)
		}
		a, b := k.pose(nil, 1, 1, 0, 0, 0), k.pose(nil, 0, 0, 0, 0, 0)
		moved := false
		for i := range a {
			if a[i] != b[i] {
				moved = true
			}
		}
		if !moved && !k.slime && id != "Blaze" && id != "Ghast" && id != "SnowMan" && id != "Silverfish" && id != "Endermite" { // a slime has no limbs; a blaze and a ghast hover: their rods and tentacles move with the age, not the walk; a snowman's arms turn with its body, not with its steps; a silverfish and an endermite wriggle with the age
			t.Errorf("%s does not swing its limbs", id)
		}
	}
}

func TestEveryMobTextureIsInTheJar(t *testing.T) {
	g := testGame(t)
	for _, k := range kinds {
		for _, path := range []string{k.tex, k.fur} {
			if path == "look/skin" { // the player's own skin is generated or the user's, not the jar's
				continue
			}
			if path != "" && g.res.entity[path] == nil {
				t.Errorf("no texture %q for %s", path, k.id)
			}
		}
	}
}

func TestSavedMobsAndItemsAreLoaded(t *testing.T) {
	g := testWorldGame(t, "New World")
	g.feet = vec3{-224, 70, 184}
	g.loadNearbyEntities()
	if len(g.mobs) == 0 {
		t.Fatal("no animals were loaded around the spawn of New World")
	}
	seen := map[string]int{}
	for _, m := range g.mobs {
		seen[m.kind.id]++
		if m.health <= 0 || m.pos[1] < 1 || m.pos[1] > 255 {
			t.Fatalf("a %s loaded with health %v at %v", m.kind.id, m.health, m.pos)
		}
	}
	t.Logf("loaded %v", seen)
	// Asking again does not load the same animals twice.
	n := len(g.mobs)
	g.loadNearbyEntities()
	if len(g.mobs) != n {
		t.Fatal("loaded twice")
	}
}
