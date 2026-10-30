package worldgen

import (
	"sync"
	"testing"
	"time"

	"gocraft/world"
)

func TestChunkIsDeterministicAndInfinite(t *testing.T) {
	g := NewGenerator(12345)
	a := g.Chunk(world.ChunkPos{X: 3, Z: -7})
	b := NewGenerator(12345).Chunk(world.ChunkPos{X: 3, Z: -7})
	if a.Blocks != b.Blocks {
		t.Fatal("the same seed and position gave different columns")
	}
	// Far from the origin the world goes on: ground, water and sky as anywhere else.
	for _, cp := range []world.ChunkPos{{X: 100000, Z: 100000}, {X: -250000, Z: 7}, {X: 0, Z: 1500000}, {X: -1875000, Z: -1875000}} {
		c := g.Chunk(cp)
		solid := 0
		for i := 0; i < 65536; i++ {
			if id := c.Blocks[i] >> 4; id != 0 && id != 8 && id != 9 {
				solid++
			}
		}
		if solid < 4000 {
			t.Fatalf("chunk %v has only %d solid blocks", cp, solid)
		}
		if c.Blocks[idx(0, 0, 0)]>>4 != 7 && c.Blocks[idx(1, 0, 1)]>>4 != 7 && c.Blocks[idx(2, 0, 2)]>>4 != 7 {
			t.Fatalf("chunk %v has no bedrock at the bottom", cp)
		}
	}
}

func TestChunkConcurrent(t *testing.T) {
	g := NewGenerator(-4711)
	var wg sync.WaitGroup
	results := make([]*world.Chunk, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = g.Chunk(world.ChunkPos{X: int32(i % 3), Z: int32(i / 3)})
		}()
	}
	wg.Wait()
	for i, c := range results {
		want := NewGenerator(-4711).Chunk(world.ChunkPos{X: int32(i % 3), Z: int32(i / 3)})
		if c.Blocks != want.Blocks {
			t.Fatalf("chunk %d differs between a shared generator used from many goroutines and a fresh one", i)
		}
	}
}

func TestChunkSpeed(t *testing.T) {
	g := NewGenerator(987654321)
	// A walk of fresh chunks, as streaming asks for them: each needs its neighbours' base terrain,
	// which the cache keeps from the one before.
	start := time.Now()
	n := 0
	for x := int32(0); x < 12; x++ {
		for z := int32(0); z < 12; z++ {
			g.Chunk(world.ChunkPos{X: x, Z: z})
			n++
		}
	}
	per := time.Since(start) / time.Duration(n)
	t.Logf("%d chunks, %v per chunk (decoration and light included)", n, per)
	if per > 60*time.Millisecond && !testing.Short() {
		t.Fatalf("too slow: %v per chunk", per)
	}
}

func TestSpawn(t *testing.T) {
	g := NewGenerator(42)
	x, z := g.Spawn()
	if !g.canSpawnAt(int(x), int(z)) {
		t.Fatalf("spawn %d,%d is not on grass", x, z)
	}
	if x < -400 || x > 400 || z < -400 || z > 400 {
		t.Fatalf("spawn %d,%d is far from the origin", x, z)
	}
}
