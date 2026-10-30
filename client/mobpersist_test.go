package client

import (
	"testing"

	"gocraft/world"
)

func TestMobsAndItemsSurviveTheOverlay(t *testing.T) {
	g := testGame(t)
	sheep := g.newMob(kinds["Sheep"], vec3{3.5, 4, 5.5}, 90)
	sheep.colour, sheep.sheared, sheep.health = 5, true, 3
	slime := g.newSlime(vec3{-8.5, 4, 2.5}, 2)
	slime.health = 3
	z := g.newMob(kinds["Zombie"], vec3{20.5, 4, 20.5}, 10)
	dead := g.newMob(kinds["Cow"], vec3{1, 4, 1}, 0)
	dead.health = 0
	bot := g.newMob(kinds["PvPBot"], vec3{2, 4, 2}, 0)
	g.mobs = []*mob{sheep, slime, z, dead, bot}
	g.items = []*itemEntity{{stack: slot{ID: 264, N: 5}, pos: vec3{1.5, 4, 1.5}, age: 100}}
	g.be.mobStore = map[world.ChunkPos][]mobJSON{{X: 50, Z: 50}: {{ID: "Pig", X: 800, Y: 4, Z: 800, Health: 10}}}
	g.be.resolved = map[world.ChunkPos]bool{{X: 0, Z: 0}: true, {X: 50, Z: 50}: true}
	o := overlay{dir: t.TempDir()}
	g.saveMobStore(o)

	h := testGame(t)
	h.mobs = []*mob{h.newMob(kinds["Chicken"], vec3{}, 0)} // what a previous world left behind goes
	h.loadMobStore(o)
	if len(h.mobs) != 3 {
		t.Fatalf("%d mobs came back, want the sheep, the slime and the zombie (not the dead cow or the bot)", len(h.mobs))
	}
	by := map[string]*mob{}
	for _, m := range h.mobs {
		by[m.kind.id] = m
	}
	s := by["Sheep"]
	if s == nil || s.colour != 5 || !s.sheared || s.health != 3 || s.yaw != 90 || s.pos != (vec3{3.5, 4, 5.5}) {
		t.Fatalf("the sheep came back as %+v", s)
	}
	if sl := by["Slime"]; sl == nil || sl.size != 2 || sl.health != 3 {
		t.Fatalf("the slime came back as %+v", sl)
	}
	if len(h.items) != 1 || h.items[0].stack.ID != 264 || h.items[0].stack.N != 5 || h.items[0].age != 100 {
		t.Fatalf("the item came back as %+v", h.items)
	}
	if !h.be.resolved[world.ChunkPos{X: 0, Z: 0}] || len(h.be.mobStore[world.ChunkPos{X: 50, Z: 50}]) != 1 {
		t.Fatal("the resolved columns and the mobs put away should be kept")
	}
	// A mob put away comes back with its column, once.
	h.spawnStored(world.ChunkPos{X: 50, Z: 50})
	if len(h.mobs) != 4 || len(h.be.mobStore[world.ChunkPos{X: 50, Z: 50}]) != 0 {
		t.Fatalf("spawnStored: %d mobs, store %v", len(h.mobs), h.be.mobStore)
	}
}

func TestLeavingAColumnPutsItsMobsAway(t *testing.T) {
	g := testGame(t)
	g.be.mobStore = map[world.ChunkPos][]mobJSON{}
	a := g.newMob(kinds["Cow"], vec3{200.5, 4, 3.5}, 0) // column 12
	b := g.newMob(kinds["Pig"], vec3{1.5, 4, 3.5}, 0)   // column 0
	g.mobs = []*mob{a, b}
	g.entityChunks = map[world.ChunkPos]bool{{X: 12, Z: 0}: true, {X: 0, Z: 0}: true}
	g.unloadEntities(world.ChunkPos{X: 12, Z: 0})
	if len(g.mobs) != 1 || g.mobs[0] != b || g.entityChunks[world.ChunkPos{X: 12, Z: 0}] {
		t.Fatalf("mobs %v, columns %v", len(g.mobs), g.entityChunks)
	}
	if st := g.be.mobStore[world.ChunkPos{X: 12, Z: 0}]; len(st) != 1 || st[0].ID != "Cow" {
		t.Fatalf("the cow should be put away: %v", st)
	}
}

func TestHostileMobsDespawnFarAnimalsStay(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0, 4, 0}
	z := g.newMob(kinds["Zombie"], vec3{130, 4, 0}, 0)
	cow := g.newMob(kinds["Cow"], vec3{130, 4, 0}, 0)
	near := g.newMob(kinds["Zombie"], vec3{20, 4, 0}, 0)
	near.despawnAge = 10000
	g.mobs = []*mob{z, cow, near}
	g.tickMobs()
	ids := map[string]bool{}
	for _, m := range g.mobs {
		ids[m.kind.id] = true
	}
	if ids["Zombie"] && z != near && len(g.mobs) != 2 {
		t.Fatalf("mobs %d", len(g.mobs))
	}
	if !ids["Cow"] {
		t.Fatal("a cow 130 blocks away stays")
	}
	for _, m := range g.mobs {
		if m == z {
			t.Fatal("a zombie 130 blocks away despawns")
		}
	}
	// Between 32 and 128 blocks an old hostile mob goes at random; within 32 never.
	far := g.newMob(kinds["Zombie"], vec3{60, 4, 0}, 0)
	gone := false
	for i := 0; i < 20000 && !gone; i++ {
		far.despawnAge = 601
		gone = g.despawns(far, 3600)
	}
	if !gone {
		t.Fatal("an old zombie 60 blocks away should despawn sometime")
	}
	for i := 0; i < 5000; i++ {
		near.despawnAge = 100000
		if g.despawns(near, 400) {
			t.Fatal("a zombie within 32 blocks never despawns")
		}
	}
	if g.despawns(g.newMob(kinds["PvPBot"], vec3{}, 0), 1e9) {
		t.Fatal("the practice bot stays")
	}
}
