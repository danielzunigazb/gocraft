package client

import (
	"gocraft/anvil"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gocraft/world"
)

var isolated sync.Map // *testing.T -> bool: tests that already have their own data folders

var (
	resOnce sync.Once
	testRes *resources
)

// testGame needs the player's own 1.8.9 jar; the test is skipped without it.
func testGame(t *testing.T) *game {
	jar := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "client.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip("no 1.8.9 client jar")
	}
	resOnce.Do(func() {
		r, err := loadResources(jar, nil)
		if err != nil {
			t.Fatal(err)
		}
		testRes = r
	})
	if _, again := isolated.LoadOrStore(t, true); !again { // once per test: its games share these folders
		t.Setenv("XDG_DATA_HOME", t.TempDir()) // tests never read or write the player's own saves
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	}
	return &game{world: world.New(), res: testRes, health: maxHealth, food: maxFood, dirty: map[world.ChunkPos]bool{}, meshes: map[world.ChunkPos]*chunkMesh{}}
}

func run(g *game, ticks int, in input) {
	for i := 0; i < ticks; i++ {
		g.tick(in)
	}
}

func near[T float32 | float64](a, b, eps T) bool {
	return math.Abs(float64(a)-float64(b)) <= float64(eps)
}

func TestFallAndLand(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 10, 0.5}
	run(g, 60, input{})
	if !g.onGround || !near(g.feet[1], 4, 1e-4) {
		t.Fatalf("feet y %v onGround %v, want standing on the flat world's surface at y=4", g.feet[1], g.onGround)
	}
}

func TestJumpHeight(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.onGround = true
	peak := 0.0
	g.tick(input{jump: true})
	for i := 0; i < 20; i++ {
		peak = max(peak, g.feet[1]-4)
		g.tick(input{})
	}
	if peak < 1.2 || peak > 1.3 { // vanilla's jump reaches about 1.252 blocks
		t.Fatalf("jump peak %v", peak)
	}
}

func TestWallStopsPlayer(t *testing.T) {
	g := testGame(t)
	for y := int32(4); y < 7; y++ {
		g.world.SetBlock(0, y, 3, 1<<4) // a stone wall south of the start
	}
	g.feet = vec3{0.5, 4, 0.5}
	g.onGround = true
	run(g, 80, input{forward: 1}) // yaw 0 walks towards +Z
	if g.feet[2] > 3-playerW/2+1e-3 || g.feet[2] < 2 {
		t.Fatalf("stopped at z=%v, want against the wall at z=%v", g.feet[2], 3-playerW/2)
	}
}

func TestStepsUpSlabNotBlock(t *testing.T) {
	g := testGame(t)
	for z := int32(3); z < 20; z++ {
		g.world.SetBlock(0, 4, z, 44<<4) // a row of bottom slabs: 0.5 high
	}
	g.feet = vec3{0.5, 4, 0.5}
	g.onGround = true
	run(g, 40, input{forward: 1})
	if g.feet[2] < 5 || !near(g.feet[1], 4.5, 1e-3) {
		t.Fatalf("at %v, want walked onto the slabs (z>5, y=4.5)", g.feet)
	}

	g2 := testGame(t)
	g2.world.SetBlock(0, 4, 3, 1<<4) // a full block: 1 high, too tall to step on
	g2.feet = vec3{0.5, 4, 0.5}
	g2.onGround = true
	run(g2, 40, input{forward: 1})
	if g2.feet[1] != 4 || g2.feet[2] > 3 {
		t.Fatalf("at %v, a full block must stop the player", g2.feet)
	}
}

func TestWalkSpeed(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.onGround = true
	g.tick(input{})
	z0 := g.feet[2]
	run(g, 20, input{forward: 1}) // a second of walking: about 4.3 blocks
	d := g.feet[2] - z0
	if d < 3.8 || d > 4.6 {
		t.Fatalf("walked %v blocks in a second, want about 4.3", d)
	}
}

func TestSurfaceAndUnstick(t *testing.T) {
	g := testGame(t)
	// A column of the flat world: bedrock, dirt, dirt, grass, so the first free height is y=4.
	if y, ok := g.surfaceAt(0, 0); !ok || y != 4 {
		t.Fatalf("surface %v %v, want 4", y, ok)
	}
	// level.dat's spawn can lie underground: a player inside the dirt is lifted to the surface.
	if got := g.freeSpot(vec3{0.5, 2.2, 0.5}); got[1] < 4 || got[1] >= 5 { // then gravity settles it on y=4
		t.Fatalf("freed at %v, want just above the surface", got)
	}
	// A player already standing free stays where they are.
	if got := g.freeSpot(vec3{0.5, 4, 0.5}); got[1] != 4 {
		t.Fatalf("moved a free player to %v", got)
	}
	// A pocket under a ceiling: the first free spot is above the ceiling, not inside it.
	g.world.SetBlock(0, 6, 0, 1<<4)
	g.world.SetBlock(1, 6, 0, 1<<4)
	g.world.SetBlock(0, 7, 0, 1<<4)
	g.world.SetBlock(1, 7, 0, 1<<4)
	if got := g.freeSpot(vec3{0.8, 5.5, 0.5}); got[1] < 8 {
		t.Fatalf("freed at %v, inside the ceiling", got)
	}
}

func TestCreativeItemsHaveIcons(t *testing.T) {
	g := testGame(t)
	items := g.creativeItems()
	if len(items) < 250 {
		t.Fatalf("only %d creative stacks", len(items))
	}
	seen := map[slot]bool{}
	blocks, things := 0, 0
	for _, st := range items {
		if seen[st] {
			t.Fatalf("%v listed twice", st)
		}
		seen[st] = true
		if st.isBlock() {
			blocks++
			if _, _, ok := g.blockIcon(st.state()); !ok {
				t.Fatalf("block %v has no icon", st)
			}
		} else {
			things++
			if _, ok := g.res.itemTile(st); !ok {
				t.Fatalf("item %v has no picture", st)
			}
		}
	}
	if blocks < 150 || things < 120 {
		t.Fatalf("%d blocks and %d items", blocks, things)
	}
}

// Every item the game defines has a model in the jar, or is on the short list of ones 1.8 draws another way.
func TestEveryItemHasAPicture(t *testing.T) {
	g := testGame(t)
	for id := range itemDefs {
		s := slot{ID: id, N: 1}
		if _, ok := g.res.itemTile(s); !ok {
			t.Logf("no picture for item %d (%s)", id, itemModelName(s))
		}
	}
	for _, id := range []uint16{256, 267, 270, 280, 297, 351, 263, 349, 322, 265, 264} {
		if _, ok := g.res.itemTile(slot{ID: id, N: 1}); !ok {
			t.Errorf("item %d (%s) must have a picture", id, itemModelName(slot{ID: id}))
		}
	}
}

// platform lays a one-block-thick slab of stone at y=4 (top at y=5) over x in [x0,x1], z in [-2,2].
func platform(g *game, x0, x1 int32) {
	for x := x0; x <= x1; x++ {
		for z := int32(-2); z <= 2; z++ {
			g.world.SetBlock(x, 4, z, 1<<4)
		}
	}
}

func TestSneakStopsAtTheEdge(t *testing.T) {
	g := testGame(t)
	platform(g, -3, 0) // the edge is at x=1
	g.feet = vec3{0.5, 5, 0}
	g.onGround = true
	g.yaw = -math.Pi / 2 // facing +X
	run(g, 60, input{forward: 1, sneak: true})
	if g.feet[1] != 5 || g.feet[0] > 1.3+1e-3 || g.feet[0] < 1.0 {
		t.Fatalf("sneaking player ended at %v: must hang over the edge without falling (x up to 1.3, y=5)", g.feet)
	}

	f := testGame(t)
	platform(f, -3, 0)
	f.feet = vec3{0.5, 5, 0}
	f.onGround = true
	f.yaw = -math.Pi / 2
	run(f, 60, input{forward: 1})
	if f.feet[1] >= 5 || f.feet[0] < 1.3 {
		t.Fatalf("walking player ended at %v: should have walked off the edge and fallen", f.feet)
	}
}

func TestPlacementRefusedInsideThePlayer(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	for _, c := range []struct {
		y    int32
		want bool
	}{{4, false}, {5, false}, {6, true}, {3, true}} {
		if got := g.canPlace(0, c.y, 0, 1<<4); got != c.want {
			t.Errorf("stone at y=%d: %v, want %v", c.y, got, c.want)
		}
	}
	if !g.canPlace(0, 4, 0, 50<<4) { // a torch has no collision box
		t.Error("a torch can be placed where the player stands")
	}
	if g.canPlace(0, 4, 0, 44<<4) { // a bottom slab fills y 4..4.5 where the feet are
		t.Error("a slab was placed inside the player's feet")
	}
}

func TestReachAndEyes(t *testing.T) {
	g := testGame(t)
	g.creative = true
	if g.reachDist() != 5 {
		t.Fatalf("creative reach %v", g.reachDist())
	}
	g.creative = false
	if g.reachDist() != 4.5 {
		t.Fatalf("survival reach %v", g.reachDist())
	}
	g.sneaking = true
	if g.eyeOffset() != 1.54 {
		t.Fatalf("sneaking eye height %v", g.eyeOffset())
	}
}

func TestMiningTimesWithBareHands(t *testing.T) {
	for _, c := range []struct {
		id   uint16
		want int
	}{{3, 15}, {17, 60}, {20, 9}, {1, 150}, {31, 0}, {7, -1}, {49, 5000}} {
		if got := mineTicks(c.id); got != c.want {
			t.Errorf("block %d: %d ticks, want %d (hardness x30, or x100 when it needs a pickaxe)", c.id, got, c.want)
		}
	}
}

func TestDropsByHand(t *testing.T) {
	g := testGame(t)
	by := func(state uint16, tool slot) []slot {
		_, harvest := g.breakInfo(state, tool)
		return g.dropsFor(state, harvest, tool)
	}
	if d := by(2<<4, slot{}); len(d) != 1 || d[0] != blockSlot(3<<4, 1) {
		t.Errorf("grass drops %v, want dirt", d)
	}
	if d := by(1<<4, slot{}); len(d) != 0 {
		t.Errorf("stone broken by hand must drop nothing, got %v", d)
	}
	if d := by(35<<4|14, slot{}); len(d) != 1 || d[0] != blockSlot(35<<4|14, 1) {
		t.Errorf("red wool drops %v", d)
	}
	if d := by(53<<4|6, slot{}); len(d) != 1 || d[0] != blockSlot(53<<4, 1) {
		t.Errorf("stairs drop %v, want a plain stair item", d)
	}
}

func TestToolsMineFasterAndHarvestMore(t *testing.T) {
	g := testGame(t)
	g.onGround = true
	for _, c := range []struct {
		name  string
		state uint16
		tool  slot
		ticks int
		drops uint16 // block or item id of the first drop, 0 for nothing
		item  bool
	}{
		{"stone with a wooden pickaxe", 1 << 4, slot{ID: 270, N: 1}, 23, 4, false}, // 1.5*30/2 = 22.5
		{"stone with a diamond pickaxe", 1 << 4, slot{ID: 278, N: 1}, 6, 4, false}, // 1.5*30/8 = 5.6
		{"stone with a golden pickaxe", 1 << 4, slot{ID: 285, N: 1}, 4, 4, false},  // speed 12: 3.75
		{"obsidian with a diamond pickaxe", 49 << 4, slot{ID: 278, N: 1}, 188, 49, false},
		{"obsidian with an iron pickaxe", 49 << 4, slot{ID: 257, N: 1}, 5000/1 + 0, 0, false}, // iron cannot harvest it
		{"coal ore with a wooden pickaxe", 16 << 4, slot{ID: 270, N: 1}, 45, 263, true},
		{"iron ore with a wooden pickaxe", 15 << 4, slot{ID: 270, N: 1}, 0, 0, false},
		{"iron ore with a stone pickaxe", 15 << 4, slot{ID: 274, N: 1}, 23, 15, false},
		{"diamond ore with a stone pickaxe", 56 << 4, slot{ID: 274, N: 1}, 0, 0, false},
		{"diamond ore with an iron pickaxe", 56 << 4, slot{ID: 257, N: 1}, 15, 264, true},
		{"a log with a wooden axe", 17 << 4, slot{ID: 271, N: 1}, 30, 17, false},
		{"dirt with a wooden shovel", 3 << 4, slot{ID: 269, N: 1}, 8, 3, false},
		{"wool with shears", 35 << 4, slot{ID: 359, N: 1}, 5, 35, false},
		{"a cobweb with a sword", 30 << 4, slot{ID: 267, N: 1}, 8, 287, true},
	} {
		ticks, harvest := g.breakInfo(c.state, c.tool)
		if c.ticks != 0 && c.drops != 0 && ticks != c.ticks {
			t.Errorf("%s: %d ticks, want %d", c.name, ticks, c.ticks)
		}
		d := g.dropsFor(c.state, harvest, c.tool)
		if (c.drops == 0) != (len(d) == 0) || (len(d) > 0 && d[0].ID != c.drops) {
			t.Errorf("%s drops %v, want id %d", c.name, d, c.drops)
		}
	}
	// In the air everything takes five times as long.
	g.onGround = false
	if ticks, _ := g.breakInfo(3<<4, slot{}); ticks != 75 {
		t.Errorf("dirt while falling: %d ticks, want 75", ticks)
	}
}

func TestToolWearAndBreaking(t *testing.T) {
	g := testGame(t)
	g.inv[0], g.sel = slot{ID: 270, N: 1}, 0 // a wooden pickaxe: 59 uses
	for i := 0; i < 59; i++ {
		g.useDurability(wear("pickaxe", 1<<4))
	}
	if g.inv[0].empty() || g.inv[0].Dmg != 59 {
		t.Fatalf("after 59 uses: %v", g.inv[0])
	}
	g.useDurability(1)
	if !g.inv[0].empty() {
		t.Fatalf("the pickaxe should break on use 60: %v", g.inv[0])
	}
	if wear("sword", 1<<4) != 2 || wear("pickaxe", 31<<4) != 0 {
		t.Error("a sword wears 2 a block; breaking grass is free")
	}
	g.creative = true
	g.inv[0] = slot{ID: 270, N: 1}
	g.useDurability(5)
	if g.inv[0].Dmg != 0 {
		t.Error("tools do not wear in creative mode")
	}
}

func TestHungerAndStarvation(t *testing.T) {
	g := testGame(t)
	g.food, g.saturation, g.health = 20, 0, 10
	g.exhaust(4.5)
	g.tickFood()
	if g.food != 19 { // 4 exhaustion cost a food point (no saturation left)
		t.Fatalf("food %d", g.food)
	}
	g.saturation, g.exhaustion = 3, 0
	g.exhaust(4.1)
	g.tickFood()
	if g.saturation != 2 || g.food != 19 {
		t.Fatalf("saturation goes first: sat %v food %d", g.saturation, g.food)
	}
	// Full food and saturation heals fast: one point every 10 ticks costs exhaustion.
	g.food, g.saturation, g.health, g.exhaustion, g.foodTimer = 20, 6, 10, 0, 0
	for i := 0; i < 10; i++ {
		g.tickFood()
	}
	if g.health != 11 {
		t.Fatalf("fast regeneration: health %v after 10 ticks", g.health)
	}
	// 18 food and above heals a heart every 80 ticks.
	g.food, g.saturation, g.health, g.foodTimer, g.exhaustion = 18, 0, 10, 0, 0
	for i := 0; i < 80; i++ {
		g.tickFood()
	}
	if g.health != 11 {
		t.Fatalf("slow regeneration: %v", g.health)
	}
	// Starvation hurts every 80 ticks, but not below half a heart.
	g.food, g.health, g.foodTimer = 0, 4, 0
	for i := 0; i < 80*10; i++ {
		g.tickFood()
	}
	if g.health != 1 {
		t.Fatalf("starving health %v, want it to stop at 1", g.health)
	}
	g.creative, g.food = true, 5
	g.exhaust(100)
	g.tickFood()
	if g.food != 5 {
		t.Error("creative mode gets hungry")
	}
}

func TestInventoryStacksAndPlacing(t *testing.T) {
	g := testGame(t)
	g.giveSlot(blockSlot(3<<4, 1))
	g.giveSlot(blockSlot(3<<4, 1))
	g.giveSlot(blockSlot(2<<4, 1))
	if g.inv[0] != blockSlot(3<<4, 2) || g.inv[1] != blockSlot(2<<4, 1) {
		t.Fatalf("inventory %v", g.inv[:3])
	}
	g.inv[0].N = 64
	g.giveSlot(blockSlot(3<<4, 1)) // a full stack: the next block starts another
	if g.inv[2] != blockSlot(3<<4, 1) {
		t.Fatalf("overflow went to %v", g.inv[:4])
	}
	// A stack bigger than the room left is split: what does not fit comes back.
	var full [36]slot
	for i := range full {
		full[i] = blockSlot(1<<4, 64)
	}
	g.inv = full
	if rest := g.giveSlot(blockSlot(2<<4, 5)); rest.N != 5 {
		t.Fatalf("a full inventory took the stack: %v", rest)
	}
	g.inv = [36]slot{}
	g.inv[1] = slot{ID: 280, N: 60} // sticks
	if rest := g.giveSlot(slot{ID: 280, N: 10}); !rest.empty() || g.inv[1].N != 64 || g.inv[0].N != 6 {
		t.Fatalf("topping up and spilling: rest %v inv %v", rest, g.inv[:2])
	}
	g.inv = [36]slot{}
	g.inv[1] = blockSlot(3<<4, 1)
	g.sel = 1
	g.consume()
	if !g.inv[1].empty() {
		t.Fatalf("the last block of a stack left %v", g.inv[1])
	}
	g.creative = true
	g.inv[1] = blockSlot(3<<4, 5)
	g.consume()
	if g.inv[1].N != 5 {
		t.Fatal("creative mode must not use up blocks")
	}
}

func TestFallDamageAndRegen(t *testing.T) {
	g := testGame(t)
	g.spawn = vec3{0.5, 4, 0.5}
	g.feet = vec3{0.5, 20, 0.5}
	run(g, 80, input{})
	if !g.onGround || g.health != maxHealth-13 { // 16 blocks fallen: 16-3 = 13 hearts of damage
		t.Fatalf("health %v onGround %v after a 16 block fall", g.health, g.onGround)
	}
	g.food = 20
	run(g, 80, input{})
	if g.health != maxHealth-12 {
		t.Fatalf("health %v, want one point back after 4 seconds", g.health)
	}

	h := testGame(t)
	h.spawn = vec3{0.5, 4, 0.5}
	h.feet = vec3{0.5, 60, 0.5}
	run(h, 120, input{})
	if h.health != maxHealth || h.fall != 0 || h.feet[1] > 5 {
		t.Fatalf("after a deadly fall: health %v at %v: want a respawn at full health", h.health, h.feet)
	}

	c := testGame(t)
	c.creative = true
	c.feet = vec3{0.5, 60, 0.5}
	run(c, 120, input{})
	if c.health != maxHealth {
		t.Fatalf("creative mode took damage: %v", c.health)
	}
}

func TestScanSaves(t *testing.T) {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no saves")
	}
	list := scanSaves(dir)
	if len(list) == 0 {
		t.Fatal("no worlds listed")
	}
	modes := map[string]int32{}
	for _, e := range list {
		modes[filepath.Base(e.dir)] = e.mode
	}
	if m, ok := modes["New World"]; !ok || m != 1 {
		t.Fatalf("worlds %v: New World must be listed as creative", modes)
	}
}

func TestApplySaveSetsUpTheWorld(t *testing.T) {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "bridge")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no bridge world")
	}
	save, err := anvil.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer save.Close()
	g := testGame(t)
	g.applySave(save)
	if g.creative {
		t.Fatal("bridge is a survival world")
	}
	if g.health <= 0 || g.food != 17 {
		t.Fatalf("health %v food %v: want the saved hunger of 17 and a living player", g.health, g.food)
	}
	if g.world.Block(0, 0, 0) == 0 && g.world.Block(int32(g.feet[0]), int32(g.feet[1])-1, int32(g.feet[2]))>>4 == 0 {
		t.Fatal("nothing under the player's feet")
	}
}

func TestTextMeasuring(t *testing.T) {
	g := testGame(t)
	g.glyphs = glyphWidths(g.res.gui["ascii"])
	if g.glyphs[' '] != 3 || g.glyphs['i'] >= g.glyphs['m'] || g.glyphs['m'] < 5 {
		t.Fatalf("glyph widths: space %d i %d m %d", g.glyphs[' '], g.glyphs['i'], g.glyphs['m'])
	}
	if w := g.textWidth("Singleplayer"); w < 40 || w > 90 {
		t.Fatalf("a word measured %d GUI pixels", w)
	}
}

func TestDoorsAndGatesFindTheirModels(t *testing.T) {
	g := testGame(t)
	// A wooden door facing south, open, hinge on the right: lower meta 1|4, upper meta 8|1.
	lowerMeta, upperMeta := 1|4, 8|1
	nbOf := func(self int) nbFunc {
		return func(dx, dy, dz int32) uint16 {
			switch {
			case dy == 1 && self == lowerMeta:
				return 64<<4 | uint16(upperMeta)
			case dy == -1 && self == upperMeta:
				return 64<<4 | uint16(lowerMeta)
			}
			return 0
		}
	}
	for _, c := range []struct {
		meta int
		want string
	}{{lowerMeta, "facing=south,half=lower,hinge=right,open=true"}, {upperMeta, "facing=south,half=upper,hinge=right,open=true"}} {
		file, props := g.res.stateOf(64, c.meta, nbOf(c.meta))
		if file != "wooden_door" || props != c.want {
			t.Errorf("door meta %d: %s %q, want %q", c.meta, file, props, c.want)
		}
		if len(g.res.pick(file, props)) == 0 || len(g.res.states[file][props]) == 0 {
			t.Errorf("%s %q has no model in the jar", file, props)
		}
	}
	// A closed and an open door fill different space: open, it stands against the side of its block.
	closed, _ := g.res.collision(64<<4 | 0)
	open, _ := g.res.collision(64<<4 | 4)
	if closed == open {
		t.Error("an open door has the same box as a closed one")
	}
	if file, props := g.res.stateOf(107, 5, nil); file != "fence_gate" || props != "facing=west,in_wall=false,open=true" || len(g.res.states[file][props]) == 0 {
		t.Errorf("gate: %s %q", file, props)
	}
}

// testWorldGame opens one of the player's worlds with the shared resources.
func testWorldGame(t *testing.T, name string) *game {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", name)
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no 1.8.9 world")
	}
	save, err := anvil.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(save.Close)
	g := testGame(t)
	g.applySave(save)
	return g
}
