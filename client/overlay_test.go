package client

import (
	"os"
	"path/filepath"
	"testing"

	"gocraft/anvil"
	"gocraft/world"
)

func TestOverlayRoundTrip(t *testing.T) {
	o := overlay{t.TempDir()}
	cp := world.ChunkPos{X: -3, Z: 7}
	if c, err := o.loadChunk(cp); c != nil || err != nil {
		t.Fatalf("an unsaved column loaded as %v %v", c, err)
	}
	c := &world.Chunk{Light: make([]uint8, 65536), Biomes: make([]uint8, 256)}
	c.Blocks[0], c.Blocks[65535], c.Blocks[4097] = 7<<4, 35<<4|14, 1<<4
	c.Light[12345], c.Biomes[255] = 0xF3, 21
	if err := o.saveChunk(cp, c); err != nil {
		t.Fatal(err)
	}
	got, err := o.loadChunk(cp)
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	if got.Blocks != c.Blocks || got.Light[12345] != 0xF3 || got.Biomes[255] != 21 {
		t.Fatal("a saved column came back different")
	}
	// A column saved without light or biomes keeps them absent.
	plain := &world.Chunk{}
	plain.Blocks[9] = 3 << 4
	if err := o.saveChunk(world.ChunkPos{}, plain); err != nil {
		t.Fatal(err)
	}
	if got, _ := o.loadChunk(world.ChunkPos{}); got == nil || got.Light != nil || got.Biomes != nil || got.Blocks[9] != 3<<4 {
		t.Fatalf("plain column: %+v", got)
	}
	// Garbage is an error, not a world.
	if err := os.WriteFile(o.chunkPath(world.ChunkPos{X: 1}), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := o.loadChunk(world.ChunkPos{X: 1}); got != nil || err == nil {
		t.Fatalf("garbage loaded as %v %v", got, err)
	}
}

func TestDirtyTracking(t *testing.T) {
	w := world.New()
	if len(w.TakeDirty()) != 0 {
		t.Fatal("a new world has edits")
	}
	w.SetBlock(1, 5, 1, 1<<4)
	w.SetBlock(2, 5, 2, 1<<4)  // the same column
	w.SetBlock(-1, 5, 0, 1<<4) // the column to the west
	if d := w.TakeDirty(); len(d) != 2 {
		t.Fatalf("dirty columns %v, want 2", d)
	}
	if len(w.TakeDirty()) != 0 {
		t.Fatal("TakeDirty must forget what it returned")
	}
}

// Edits survive a restart, and the player's own world files are never written.
func TestEditsPersistAndOriginalIsUntouched(t *testing.T) {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "bridge")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no bridge world")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	fingerprint := func() map[string]int64 {
		out := map[string]int64{}
		filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				out[p] = info.Size()*1000003 + info.ModTime().UnixNano()
			}
			return nil
		})
		return out
	}
	before := fingerprint()

	open := func() (*game, *anvil.Save) {
		save, err := anvil.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		g := testGame(t)
		g.applySave(save)
		return g, save
	}
	g, save := open()
	x, z := int32(g.feet[0]), int32(g.feet[2])
	y := int32(g.feet[1]) + 5
	g.world.SetBlock(x, y, z, 35<<4|11) // blue wool in the air above the player
	g.inv[3], g.sel = blockSlot(45<<4, 12), 3
	g.inv[10] = slot{ID: 278, Dmg: 5, N: 1} // a diamond pickaxe, used five times
	g.armor[0] = slot{ID: 310, N: 1}
	g.saveWorld()
	feet := g.feet
	save.Close()

	g2, save2 := open()
	defer save2.Close()
	if got := g2.world.Block(x, y, z); got != 35<<4|11 {
		t.Fatalf("the edit came back as %d", got)
	}
	if g2.inv[3] != blockSlot(45<<4, 12) || g2.sel != 3 || g2.inv[10] != (slot{ID: 278, Dmg: 5, N: 1}) || g2.armor[0].ID != 310 {
		t.Fatalf("inventory %v sel %d armour %v", g2.inv[:11], g2.sel, g2.armor)
	}
	if g2.feet[0] != feet[0] || g2.feet[2] != feet[2] {
		t.Fatalf("player at %v, saved at %v", g2.feet, feet)
	}
	after := fingerprint()
	if len(after) != len(before) {
		t.Fatal("files appeared or vanished in the original world")
	}
	for p, f := range before {
		if after[p] != f {
			t.Fatalf("the original world file %s changed", p)
		}
	}
}
