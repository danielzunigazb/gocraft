package worldgen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gocraft/anvil"
	"gocraft/world"
)

// userWorlds are the worlds in the player's own game folder. They are read, never written.
func userWorlds(t testing.TB) []*anvil.Save {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("no 1.8.9 saves folder")
	}
	var out []*anvil.Save
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		s, err := anvil.Open(filepath.Join(dir, e.Name()))
		if err != nil || s.Level.Seed == 0 {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		t.Skip("no worlds with a seed")
	}
	return out
}

func savedChunks(s *anvil.Save, max int) []world.ChunkPos {
	var out []world.ChunkPos
	for cx := int32(-12); cx <= 12 && len(out) < max*8; cx++ {
		for cz := int32(-12); cz <= 12; cz++ {
			if c, err := s.Chunk(world.ChunkPos{X: cx, Z: cz}); err == nil && c != nil && c.Biomes != nil {
				out = append(out, world.ChunkPos{X: cx, Z: cz})
			}
		}
	}
	return out
}

func TestBiomesMatchSavedWorlds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: compares many chunks with the user's saved worlds")
	}
	for _, s := range userWorlds(t) {
		st := newBiomeStack(s.Level.Seed)
		cps := savedChunks(s, 40)
		same, total := 0, 0
		diff := map[[2]int]int{}
		for _, cp := range cps {
			c, _ := s.Chunk(cp)
			got := st.index.ints(int(cp.X)*16, int(cp.Z)*16, 16, 16)
			for i := 0; i < 256; i++ {
				total++
				if int32(c.Biomes[i]) == got[i] {
					same++
				} else {
					diff[[2]int{int(c.Biomes[i]), int(got[i])}]++
				}
			}
		}
		fmt.Printf("%s seed %d (%s): %d chunks, biomes equal %d/%d = %.2f%%\n", s.Level.Name, s.Level.Seed, s.Level.Generator, len(cps), same, total, 100*float64(same)/float64(max(total, 1)))
		n := 0
		for k, v := range diff {
			if n++; n > 8 {
				break
			}
			fmt.Printf("   saved %d, ours %d: %d blocks\n", k[0], k[1], v)
		}
	}
}

// class folds a block into what the base terrain decides: air, water, lava, or something solid.
func class(st uint16) int {
	switch st >> 4 {
	case 0:
		return 0
	case 8, 9:
		return 1
	case 10, 11:
		return 2
	case 18, 161, 17, 162, 31, 32, 37, 38, 39, 40, 175, 78, 83, 81, 86, 106, 111, 59, 50, 51, 66, 63, 65, 64, 54, 52:
		return 0 // what decoration puts above the ground reads as air for the base terrain
	}
	return 3
}

func TestBaseTerrainMatchesSavedWorlds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: compares many chunks with the user's saved worlds")
	}
	for _, s := range userWorlds(t) {
		if s.Level.Generator != "default" {
			continue
		}
		g := NewGenerator(s.Level.Seed)
		cps := savedChunks(s, 12)
		if len(cps) > 60 {
			cps = cps[:60]
		}
		exact, classes, total := 0, 0, 0
		perY := map[int][2]int{}
		for _, cp := range cps {
			saved, _ := s.Chunk(cp)
			base := g.base(cp.X, cp.Z)
			for i := 0; i < 65536; i++ {
				total++
				if saved.Blocks[i]>>4 == base.blocks[i]>>4 {
					exact++
				}
				y := i >> 8
				if class(saved.Blocks[i]) == class(base.blocks[i]) {
					classes++
				} else {
					e := perY[y/16]
					e[0]++
					perY[y/16] = e
				}
			}
		}
		fmt.Printf("%s: %d chunks, same block id %.2f%%, same class (air/water/lava/solid) %.2f%%\n", s.Level.Name, len(cps), 100*float64(exact)/float64(total), 100*float64(classes)/float64(total))
		for y := 0; y < 16; y++ {
			if e := perY[y]; e[0] > 0 {
				fmt.Printf("   class differs in y %d-%d: %d blocks\n", y*16, y*16+15, e[0])
			}
		}
	}
}

func worldPos(x, z int32) world.ChunkPos { return world.ChunkPos{X: x, Z: z} }

// highestStone is the top of the stone below the soil: what the density field decides.
func highestStone(get func(y int) uint16) int {
	for y := 255; y >= 0; y-- {
		if get(y)>>4 == 1 {
			return y
		}
	}
	return -1
}
