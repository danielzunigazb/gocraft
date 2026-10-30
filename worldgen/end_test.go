package worldgen

import (
	"testing"
	"time"

	"gocraft/world"
)

func TestEndIsDeterministic(t *testing.T) {
	a, b := NewEnd(77), NewEnd(77)
	for _, cp := range []world.ChunkPos{{X: 0, Z: 0}, {X: 2, Z: -1}, {X: -4, Z: 3}} {
		if a.Chunk(cp).Blocks != b.Chunk(cp).Blocks {
			t.Fatalf("chunk %v differs between generators of one seed", cp)
		}
	}
}

// The End is one island: end stone around the origin, a void far away, and pillars of obsidian.
func TestEndIsAnIsland(t *testing.T) {
	g := NewEnd(2024)
	mid := countBlocks(g.Chunk(world.ChunkPos{}))
	if mid[121] < 3000 {
		t.Fatalf("only %d end stone in the middle chunk", mid[121])
	}
	if far := countBlocks(g.Chunk(world.ChunkPos{X: 30, Z: 30})); len(far) != 1 || far[0] == 0 {
		t.Fatalf("something far from the island: %v", far)
	}
	var obsidian, bedrock int
	for x := int32(-6); x <= 6; x++ {
		for z := int32(-6); z <= 6; z++ {
			m := countBlocks(g.Chunk(world.ChunkPos{X: x, Z: z}))
			obsidian += m[49]
			bedrock += m[7]
		}
	}
	if obsidian == 0 || bedrock == 0 {
		t.Fatalf("no pillar in 169 columns: obsidian %d, bedrock %d", obsidian, bedrock)
	}
	if c := g.Chunk(world.ChunkPos{}); c.Biomes[0] != 9 {
		t.Fatal("not the End biome")
	}
}

func TestEndChunkTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	g := NewEnd(5)
	g.Chunk(world.ChunkPos{X: 100, Z: 100})
	start := time.Now()
	const n = 16
	for i := int32(0); i < n; i++ {
		g.Chunk(world.ChunkPos{X: i - 8, Z: 0})
	}
	t.Logf("End: %v a chunk", time.Since(start)/n)
	cold := time.Now()
	NewEnd(6).Chunk(world.ChunkPos{X: 1, Z: 1})
	t.Logf("End: %v for one chunk with nothing cached", time.Since(cold))
}
