package worldgen

import (
	"testing"
	"time"

	"gocraft/world"
)

// TestChunkCostBreakdown reports where a chunk's time goes (run it by hand: go test -run Breakdown -v).
func TestChunkCostBreakdown(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement, not a check")
	}
	g := NewGenerator(31337)
	const n = 40
	var base, rest time.Duration
	for i := 0; i < n; i++ {
		cx, cz := int32(i%8), int32(i/8)
		t0 := time.Now()
		for dx := int32(-1); dx <= 1; dx++ {
			for dz := int32(-1); dz <= 1; dz++ {
				g.base(cx+dx, cz+dz)
			}
		}
		base += time.Since(t0)
		t1 := time.Now()
		g.Chunk(world.ChunkPos{X: cx, Z: cz})
		rest += time.Since(t1)
	}
	t.Logf("per chunk: base terrain of its 3x3 (mostly cached) %v, then copy + decoration + light %v", base/n, rest/n)
}

func BenchmarkChunk(b *testing.B) {
	g := NewGenerator(31337)
	for i := 0; i < b.N; i++ {
		g.Chunk(world.ChunkPos{X: int32(i % 20), Z: int32(i / 20 % 20)})
	}
}
