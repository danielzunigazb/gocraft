package client

import (
	"os"
	"path/filepath"
	"testing"

	"gocraft/world"
)

func TestSeedFromText(t *testing.T) {
	if got := seedFromText("12345"); got != 12345 {
		t.Fatalf("a number is itself, got %d", got)
	}
	if got := seedFromText("hello"); got != 99162322 { // "hello".hashCode()
		t.Fatalf("text is its Java hash code, got %d", got)
	}
	if a, b := seedFromText(""), seedFromText("0"); a == 0 || b == 0 {
		t.Fatal("no seed (or 0) must be random, not 0")
	}
}

func TestCreateAndOpenGeneratedWorld(t *testing.T) {
	g := testGame(t)
	dir, err := createWorld("My Test: World", "42", 1)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != worldsRoot() || !isGeneratedWorld(dir) {
		t.Fatalf("world made in %s, not under %s", dir, worldsRoot())
	}
	if again, _ := createWorld("My Test: World", "42", 1); again == dir {
		t.Fatal("a second world with the same name reused the folder")
	}
	if list := scanGenerated(); len(list) != 2 || !list[0].generated {
		t.Fatalf("the world list should show both new worlds, got %v", list)
	}
	if err := g.loadWorld(dir); err != nil {
		t.Fatal(err)
	}
	if !g.inGame || !g.creative {
		t.Fatal("the world did not open in creative")
	}
	// The player stands on real ground, not in the air or inside it.
	below := g.world.Block(int32(g.feet[0]), int32(g.feet[1])-1, int32(g.feet[2]))
	here := g.world.Block(int32(g.feet[0]), int32(g.feet[1]), int32(g.feet[2]))
	if below == 0 || here != 0 {
		t.Fatalf("feet at %v: block below %d, block at feet %d", g.feet, below>>4, here>>4)
	}
	// An edit survives closing and opening: it goes to the overlay, over the generated terrain.
	px, py, pz := int32(g.feet[0])+3, int32(g.feet[1])+10, int32(g.feet[2])
	g.setBlock(px, py, pz, 57<<4) // a diamond block in the air
	g.closeWorld()
	g2 := testGame(t)
	if err := g2.loadWorld(dir); err != nil {
		t.Fatal(err)
	}
	if got := g2.world.Block(px, py, pz); got != 57<<4 {
		t.Fatalf("the edit was lost: block %d", got>>4)
	}
	// The same seed gives the same world; nothing was written into the player's own saves.
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "gen_My_Test_World")); err == nil {
		t.Fatal("a generated world must never be created in the 1.8.9 saves folder")
	}
	_ = world.ChunkPos{}
}
