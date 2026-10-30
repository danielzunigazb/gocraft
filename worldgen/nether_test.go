package worldgen

import (
	"testing"
	"time"

	"gocraft/world"
)

func countBlocks(c *world.Chunk) map[uint16]int {
	m := map[uint16]int{}
	for _, b := range c.Blocks[:nhVolume] {
		m[b>>4]++
	}
	return m
}

// The same seed gives the same column, from a new generator or an old one, in any order.
func TestNetherIsDeterministic(t *testing.T) {
	a, b := NewNether(1234), NewNether(1234)
	cps := []world.ChunkPos{{X: 0, Z: 0}, {X: 3, Z: -2}, {X: -5, Z: 7}}
	first := make([]*world.Chunk, len(cps))
	for i, cp := range cps {
		first[i] = a.Chunk(cp)
	}
	for i := len(cps) - 1; i >= 0; i-- { // the other way round, on a generator with a cold cache
		got := b.Chunk(cps[i])
		if got.Blocks != first[i].Blocks || string(got.Light) != string(first[i].Light) {
			t.Fatalf("chunk %v differs between two generators of the same seed", cps[i])
		}
	}
	if NewNether(99).Chunk(cps[0]).Blocks == first[0].Blocks {
		t.Fatal("different seeds made the same column")
	}
}

// What a Nether column looks like: ragged bedrock at the floor and roof, lava below y=32 in the
// open, netherrack for the most part, and glowstone, fire and quartz somewhere in a few columns.
func TestNetherShape(t *testing.T) {
	g := NewNether(42)
	total := map[uint16]int{}
	for xi := int32(0); xi < 7; xi++ { // a spread of columns: one area can be nearly all rock or all cavern
		for zi := int32(0); zi < 7; zi++ {
			x, z := xi*6-18, zi*6-18
			c := g.Chunk(world.ChunkPos{X: x, Z: z})
			for i := 0; i < 256; i++ { // y=0 and y=127 are always bedrock
				if c.Blocks[nhIdx(i&15, 0, i>>4)]>>4 != 7 || c.Blocks[nhIdx(i&15, 127, i>>4)]>>4 != 7 {
					t.Fatalf("chunk %d,%d: no bedrock floor/roof at column %d", x, z, i)
				}
			}
			for y := nhHeight; y < 256; y++ { // nothing above the Nether's 128
				if c.Blocks[nhIdx(0, y, 0)] != 0 {
					t.Fatal("blocks above the roof")
				}
			}
			for id, n := range countBlocks(c) {
				total[id] += n
			}
			if c.Biomes[0] != 8 {
				t.Fatal("not the Hell biome")
			}
		}
	}
	vol := 49 * nhVolume
	solid := total[87]
	if solid < vol/5 || solid > vol*7/10 {
		t.Fatalf("netherrack is %d of %d blocks", solid, vol)
	}
	for id, name := range map[uint16]string{11: "lava", 89: "glowstone", 51: "fire", 153: "quartz ore", 88: "soul sand", 13: "gravel"} {
		if total[id] == 0 {
			t.Errorf("no %s in 49 columns (%v)", name, total)
		}
	}
	// Lava only below the lake level in the open, apart from the odd pocket and spring in the walls.
	lavaAbove := 0
	c := g.Chunk(world.ChunkPos{})
	for y := 40; y < 127; y++ {
		for i := 0; i < 256; i++ {
			if c.Blocks[nhIdx(i&15, y, i>>4)]>>4 == 11 {
				lavaAbove++
			}
		}
	}
	if lavaAbove > 200 {
		t.Fatalf("%d lava blocks above y=40: the lake level is 32", lavaAbove)
	}
}

// Light has no sky in the Nether: the sky half of every light byte is 0, and lava and glowstone shine.
func TestNetherLightIsBlockLightOnly(t *testing.T) {
	g := NewNether(7)
	lit := 0
	for x := int32(0); x < 3; x++ {
		c := g.Chunk(world.ChunkPos{X: x})
		for i, l := range c.Light {
			if l>>4 != 0 {
				t.Fatal("sky light in the Nether")
			}
			if l == 15 && (c.Blocks[i]>>4 == 11 || c.Blocks[i]>>4 == 89 || c.Blocks[i]>>4 == 51) {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Fatal("no light at the sources")
	}
}

func TestNetherChunkTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	g := NewNether(5)
	g.Chunk(world.ChunkPos{X: 100, Z: 100}) // warm up
	start := time.Now()
	const n = 16
	for i := int32(0); i < n; i++ {
		g.Chunk(world.ChunkPos{X: i, Z: 0})
	}
	t.Logf("Nether: %v a chunk (a row of %d, each reusing its neighbours' terrain)", time.Since(start)/n, n)
	cold := time.Now()
	NewNether(6).Chunk(world.ChunkPos{X: 40, Z: 40}) // nothing cached: nine terrains, caves and four decorations
	t.Logf("Nether: %v for one chunk with nothing cached", time.Since(cold))
}
