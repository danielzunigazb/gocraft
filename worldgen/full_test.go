package worldgen

import (
	"fmt"
	"testing"
)

// TestFullChunksMatchSavedWorlds generates whole chunks (base, decoration, light not compared) and
// compares them with what the game saved. Decoration depends on the order the game populated its
// chunks in, which a saved world does not record, so this is an honest measure, not a goal of 100%.
func TestFullChunksMatchSavedWorlds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: compares many chunks with the user's saved worlds")
	}
	for _, s := range userWorlds(t) {
		if s.Level.Generator != "default" {
			continue
		}
		g := NewGenerator(s.Level.Seed)
		cps := savedChunks(s, 8)
		if len(cps) > 80 {
			cps = cps[:80]
		}
		exact, total := 0, 0
		count := func(blocks *[65536]uint16, ids ...uint16) int {
			n := 0
			for _, st := range blocks {
				for _, id := range ids {
					if st>>4 == id {
						n++
					}
				}
			}
			return n
		}
		var coalSame, ironSame, logSame, diffSame, lakeSame int
		for _, cp := range cps {
			saved, _ := s.Chunk(cp)
			ours := g.Chunk(cp)
			for i := 0; i < 65536; i++ {
				total++
				if saved.Blocks[i]>>4 == ours.Blocks[i]>>4 {
					exact++
				}
			}
			if count(&saved.Blocks, 16) == count(&ours.Blocks, 16) {
				coalSame++
			}
			if count(&saved.Blocks, 15) == count(&ours.Blocks, 15) {
				ironSame++
			}
			if count(&saved.Blocks, 17, 162) == count(&ours.Blocks, 17, 162) {
				logSame++
			}
			if count(&saved.Blocks, 3, 13) == count(&ours.Blocks, 3, 13) {
				diffSame++
			}
			if count(&saved.Blocks, 8, 9) == count(&ours.Blocks, 8, 9) {
				lakeSame++
			}
		}
		n := len(cps)
		fmt.Printf("%s: %d chunks, block ids equal %.2f%%; chunks whose count is identical: coal %d/%d, iron %d/%d, logs %d/%d, dirt+gravel %d/%d, water %d/%d\n",
			s.Level.Name, n, 100*float64(exact)/float64(total), coalSame, n, ironSame, n, logSame, n, diffSame, n, lakeSame, n)
	}
}

// TestOresMatchSavedWorlds: how much of the saved coal our veins land on. Veins are the first thing
// decoration draws, so this measures whether the random sequence of a population is the game's.
func TestOresMatchSavedWorlds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	for _, s := range userWorlds(t) {
		if s.Level.Generator != "default" {
			continue
		}
		g := NewGenerator(s.Level.Seed)
		cps := savedChunks(s, 8)
		if len(cps) > 40 {
			cps = cps[:40]
		}
		hit, tot, ours, perfect := 0, 0, 0, 0
		for _, cp := range cps {
			saved, _ := s.Chunk(cp)
			c := g.Chunk(cp)
			h, n := 0, 0
			for i := 0; i < 65536; i++ {
				a, b := saved.Blocks[i]>>4 == 16, c.Blocks[i]>>4 == 16
				if a {
					n++
				}
				if b {
					ours++
				}
				if a && b {
					h++
				}
			}
			hit += h
			tot += n
			if n > 0 && h == n {
				perfect++
			}
		}
		fmt.Printf("%s: saved coal %d blocks, ours %d, shared %d (%.1f%%); chunks with every coal block shared: %d/%d\n", s.Level.Name, tot, ours, hit, 100*float64(hit)/float64(max(tot, 1)), perfect, len(cps))
	}
}
