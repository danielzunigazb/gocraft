package client

import (
	"testing"
)

// A flat world: ground up to y=3, so mobs stand with their feet at y=4.
func wallAcross(g *game, z int32, gapX int32) {
	for x := int32(-30); x <= 30; x++ {
		if x == gapX {
			continue
		}
		g.setBlock(x, 4, z, 1<<4)
		g.setBlock(x, 5, z, 1<<4)
	}
}

func TestPathGoesThroughTheGapInAWall(t *testing.T) {
	g := testGame(t)
	wallAcross(g, 5, 3)
	path := g.findPath(node{0, 4, 0}, node{0, 4, 10}, 0.6, 1.95)
	if len(path) == 0 || path[len(path)-1] != (node{0, 4, 10}) {
		t.Fatalf("no path to the other side: %v", path)
	}
	through := false
	for _, n := range path {
		if n.z == 5 {
			through = true
			if n.x != 3 {
				t.Fatalf("the path crosses the wall at x=%d, not through the gap at 3", n.x)
			}
		}
		if n.y != 4 {
			t.Fatalf("level ground but the path leaves y=4: %v", n)
		}
	}
	if !through {
		t.Fatal("the path never crossed the wall")
	}
}

func TestPathStopsAtAWallWithNoGap(t *testing.T) {
	g := testGame(t)
	wallAcross(g, 5, 99)
	path := g.findPath(node{0, 4, 0}, node{0, 4, 10}, 0.6, 1.95)
	for _, n := range path {
		if n.z >= 5 {
			t.Fatalf("the path went through a solid wall: %v", n)
		}
	}
}

func TestPathStepsUpAndDownAndAvoidsWater(t *testing.T) {
	g := testGame(t)
	// A staircase: one block high at z=3, two at z=4.
	for x := int32(-1); x <= 1; x++ {
		g.setBlock(x, 4, 3, 1<<4)
		g.setBlock(x, 4, 4, 1<<4)
		g.setBlock(x, 5, 4, 1<<4)
	}
	path := g.findPath(node{0, 4, 0}, node{0, 6, 4}, 0.6, 1.95)
	if len(path) == 0 || path[len(path)-1] != (node{0, 6, 4}) {
		t.Fatalf("the path up the steps: %v", path)
	}
	// A pond across the way: water the whole row, so the search goes around it.
	h := testGame(t)
	for x := int32(-3); x <= 3; x++ {
		h.setBlock(x, 3, 4, 9<<4)
	}
	for _, n := range h.findPath(node{0, 4, 0}, node{0, 4, 8}, 0.6, 1.95) {
		if n.z == 4 && n.y == 4 && n.x >= -3 && n.x <= 3 {
			t.Fatalf("the path walks over water at %v", n)
		}
	}
}

func TestHunterWalksAroundAWall(t *testing.T) {
	g := testGame(t)
	wallAcross(g, 5, 3)
	g.feet = vec3{0.5, 4, 10.5}
	g.creative = false
	z := g.newMob(kinds["Zombie"], vec3{0.5, 4, 0.5}, 0)
	g.mobs = []*mob{z}
	crossed := false
	for i := 0; i < 400 && !crossed; i++ {
		g.tickMobs()
		g.feet = vec3{0.5, 4, 10.5} // the player stands still, out of reach
		if z.pos[2] > 6 {
			crossed = true
		}
	}
	if !crossed {
		t.Fatalf("the zombie never got round the wall: at %v", z.pos)
	}
}

func TestMobsKeepAwayFromWaterAndDrops(t *testing.T) {
	g := testGame(t)
	m := g.newMob(kinds["Sheep"], vec3{0.5, 4, 0.5}, 0) // facing south: +z
	g.setBlock(0, 3, 1, 9<<4)                           // water where it would step
	if !g.hazardAhead(m, 0) {
		t.Fatal("water ahead is a hazard")
	}
	if g.hazardAhead(m, 90) {
		t.Fatal("solid ground to the side is not")
	}
	for y := int32(0); y <= 3; y++ { // a hole four deep
		g.setBlock(1, y, 0, 0)
	}
	if !g.hazardAhead(m, 270) {
		t.Fatal("a drop of four is a hazard")
	}
}

func TestZombiesBurnInTheSunNotAtNight(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{200, 4, 200}
	day := g.newMob(kinds["Zombie"], vec3{0.5, 4, 0.5}, 0)
	g.worldTime = 6000
	for i := 0; i < 400 && day.burning == 0; i++ {
		g.tickMobExtras(day)
	}
	if day.burning == 0 {
		t.Fatal("a zombie in the noon sun never caught fire")
	}
	h0 := day.health
	for i := 0; i < 60; i++ {
		g.tickMobExtras(day)
	}
	if h0-day.health < 2 || h0-day.health > 4 {
		t.Fatalf("fire should cost a heart a second: lost %v in 3 seconds", h0-day.health)
	}
	night := g.newMob(kinds["Zombie"], vec3{0.5, 4, 0.5}, 0)
	g.worldTime = 18000
	for i := 0; i < 2000; i++ {
		g.tickMobExtras(night)
	}
	if night.burning != 0 {
		t.Fatal("a zombie burned at night")
	}
	// Water puts it out.
	g.worldTime = 6000
	g.setBlock(5, 4, 5, 9<<4)
	wet := g.newMob(kinds["Zombie"], vec3{5.5, 4, 5.5}, 0)
	wet.burning = 100
	g.tickMobExtras(wet)
	if wet.burning != 0 {
		t.Fatal("water should put the fire out")
	}
}

func TestSpiderIsNeutralByDayUntilHurt(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{2.5, 4, 0.5}
	g.creative = false
	s := g.newMob(kinds["Spider"], vec3{0.5, 4, 0.5}, 0)
	g.worldTime = 6000
	if g.spiderTarget(s) {
		t.Fatal("a spider in daylight leaves the player alone")
	}
	s.hurt(1, g.feet)
	if !g.spiderTarget(s) {
		t.Fatal("a hurt spider goes for its attacker")
	}
	n := g.newMob(kinds["Spider"], vec3{0.5, 4, 0.5}, 0)
	g.worldTime = 18000
	if !g.spiderTarget(n) {
		t.Fatal("at night a spider hunts")
	}
}

func TestSpiderClimbsWalls(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{200, 4, 200}
	for y := int32(4); y <= 8; y++ { // a wall to the south
		for x := int32(-1); x <= 1; x++ {
			g.setBlock(x, y, 3, 1<<4)
		}
	}
	s := g.newMob(kinds["Spider"], vec3{0.5, 4, 2.2}, 0)
	g.mobs = []*mob{s}
	for i := 0; i < 60; i++ {
		s.wanderTicks, s.wanderYaw = 30, 0 // walking south, into the wall
		g.tickMob(s)
	}
	if s.pos[1] < 6 {
		t.Fatalf("the spider should be well up the wall, y=%v", s.pos[1])
	}
}

func TestSlimeSplitsAndLeavesSlimeballsOnlySmall(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{40, 4, 40}
	big := g.newSlime(vec3{0.5, 4, 0.5}, 4)
	if big.health != 16 {
		t.Fatalf("a size 4 slime has %v health, want 16", big.health)
	}
	w, h := big.dims()
	if w != 2.4 || h != 2.4 {
		t.Fatalf("a size 4 slime's box is %vx%v, want 2.4", w, h)
	}
	g.mobs = []*mob{big}
	big.health = 0
	g.tickMobs()
	g.tickMobs()
	kids := 0
	for _, m := range g.mobs {
		if m.kind.slime && m != big {
			kids++
			if m.size != 2 || m.health != 4 {
				t.Fatalf("a child has size %d health %v, want 2 and 4", m.size, m.health)
			}
		}
	}
	if kids < 2 || kids > 4 {
		t.Fatalf("%d children, want 2 to 4", kids)
	}
	if len(g.mobDrops(big)) != 0 {
		t.Fatal("a big slime leaves no slimeballs")
	}
	small := g.newSlime(vec3{}, 1)
	if sm := g.mobDrops(small); len(sm) > 0 && sm[0].ID != 341 {
		t.Fatalf("a small slime drops %v", sm)
	}
	g.mobs = []*mob{small}
	small.health = 0
	g.tickMobs()
	g.tickMobs()
	if len(g.mobs) > 1 {
		t.Fatal("a size 1 slime does not split")
	}
}

func TestSlimesHop(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 8.5}
	g.creative = false
	s := g.newSlime(vec3{0.5, 4, 0.5}, 2)
	g.mobs = []*mob{s}
	maxY := 0.0
	for i := 0; i < 120; i++ {
		g.tickMob(s)
		maxY = max(maxY, s.pos[1])
	}
	if maxY < 4.5 {
		t.Fatalf("the slime never left the ground: %v", maxY)
	}
	if s.pos[2] < 1.5 {
		t.Fatalf("it should hop towards the player: z=%v", s.pos[2])
	}
}

func facingMob(t *testing.T, kind string) (*game, *mob) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.pos = vec3{0.5, 4 + eyeHeight, 0.5}
	g.onGround = true
	g.pitch = 0.3 // looking a little down, at the animal
	m := g.newMob(kinds[kind], vec3{0.5, 4, 2}, 0)
	g.mobs = []*mob{m}
	return g, m
}

func TestShearingASheep(t *testing.T) {
	g, sheep := facingMob(t, "Sheep")
	sheep.colour = 14
	g.inv[0], g.sel = slot{ID: 359, N: 1}, 0
	if !g.useOnMob() || !sheep.sheared {
		t.Fatal("shears should take the wool")
	}
	if len(g.items) < 1 || len(g.items) > 3 || g.items[0].stack.ID != 35 || g.items[0].stack.Dmg != 14 {
		t.Fatalf("wool drops: %d stacks, first %v", len(g.items), g.items)
	}
	if g.useOnMob() && len(g.items) > 3 {
		t.Fatal("a sheared sheep gives no more")
	}
	if len(g.mobDrops(sheep)) != 0 {
		t.Fatal("a sheared sheep drops no wool when it dies")
	}
}

func TestMilkingACow(t *testing.T) {
	g, _ := facingMob(t, "Cow")
	g.creative = false
	g.inv[0], g.sel = slot{ID: 325, N: 1}, 0
	if !g.useOnMob() || g.inv[0].ID != 335 {
		t.Fatalf("a bucket on a cow gives milk: %v", g.inv[0])
	}
	g.inv[0], g.inv[1] = slot{ID: 325, N: 2}, slot{}
	g.useOnMob()
	if g.inv[0].ID != 325 || g.inv[0].N != 1 || g.inv[1].ID != 335 {
		t.Fatalf("with two buckets one is filled: %v %v", g.inv[0], g.inv[1])
	}
}

func TestChickensLayEggsAndSheepEatGrass(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{200, 4, 200}
	c := g.newMob(kinds["Chicken"], vec3{0.5, 4, 0.5}, 0)
	c.eggTimer = 2
	g.tickAnimal(c)
	g.tickAnimal(c)
	if len(g.items) != 1 || g.items[0].stack.ID != 344 {
		t.Fatalf("an egg should drop: %v", g.items)
	}
	if c.eggTimer < 6000 || c.eggTimer >= 12000 {
		t.Fatalf("the next egg in %d ticks, want 6000 to 12000", c.eggTimer)
	}
	s := g.newMob(kinds["Sheep"], vec3{3.5, 4, 3.5}, 0)
	s.sheared, s.eatTicks = true, 5
	for i := 0; i < 5; i++ {
		g.tickAnimal(s)
	}
	if g.world.Block(3, 3, 3)>>4 != 3 || s.sheared {
		t.Fatalf("the sheep ate the grass: block %d, sheared %v", g.world.Block(3, 3, 3)>>4, s.sheared)
	}
}

func TestBatsFlyWithoutFalling(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{30, 4, 30}
	b := g.newMob(kinds["Bat"], vec3{0.5, 10, 0.5}, 0)
	b.onGround = false
	g.mobs = []*mob{b}
	moved := float32(0)
	for i := 0; i < 200; i++ {
		g.tickMob(b)
		if b.pos[1] < 4.5 {
			t.Fatalf("the bat fell to the ground at tick %d", i)
		}
		moved = max(moved, float32(abs(int32(b.pos[0]*10)))+float32(abs(int32(b.pos[2]*10))))
	}
	if moved < 10 {
		t.Fatalf("the bat hardly moved: %v", moved)
	}
}

func TestSquidSwimsInsideTheWater(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{30, 4, 30}
	for x := int32(-8); x <= 8; x++ { // a pool, 6 deep, dug into the ground
		for z := int32(-8); z <= 8; z++ {
			for y := int32(0); y <= 3; y++ {
				g.setBlock(x, y, z, 9<<4)
			}
			g.setBlock(x, 4, z, 9<<4)
			g.setBlock(x, 5, z, 9<<4)
		}
	}
	s := g.newMob(kinds["Squid"], vec3{0.5, 3, 0.5}, 0)
	g.mobs = []*mob{s}
	for i := 0; i < 600; i++ {
		g.tickMob(s)
		if !g.inWater(s) {
			t.Fatalf("the squid left the water at tick %d: %v", i, s.pos)
		}
	}
}

func TestRabbitsHopAwayFromThePlayer(t *testing.T) {
	g := testGame(t)
	g.creative = false
	g.feet = vec3{0.5, 4, 0.5}
	r := g.newMob(kinds["Rabbit"], vec3{0.5, 4, 3}, 0)
	g.mobs = []*mob{r}
	up := false
	for i := 0; i < 80; i++ {
		g.tickMob(r)
		up = up || r.pos[1] > 4.3
	}
	if !up {
		t.Fatal("a rabbit hops")
	}
	if r.pos[2] < 6 {
		t.Fatalf("it should run from the player: z=%v", r.pos[2])
	}
}

func TestNewMobsHaveModelsAndTextures(t *testing.T) {
	g := testGame(t)
	for _, id := range []string{"Villager", "Witch", "Rabbit", "Squid", "Bat", "Spider", "CaveSpider", "Slime"} {
		k := kinds[id]
		if k == nil || k.model == nil || k.pose == nil {
			t.Fatalf("%s is missing", id)
		}
		for _, tex := range append([]string{k.tex}, k.texVariants...) {
			if g.res.entity[tex] == nil {
				t.Errorf("%s: no texture %q in the jar", id, tex)
			}
		}
	}
}
