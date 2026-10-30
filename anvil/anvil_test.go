package anvil

import (
	"os"
	"path/filepath"
	"testing"

	"gocraft/world"
)

// The player's own 1.8.9 worlds are the fixture; the test is skipped without them.
func TestOpenRealWorld(t *testing.T) {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "New World")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no 1.8.9 world")
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Level.GameType != 1 { // this world was made in creative mode
		t.Fatalf("game type %d", s.Level.GameType)
	}
	if s.Level.Name != "New World" {
		t.Fatalf("level %+v", s.Level)
	}
	// The chunk at the world's spawn has bedrock at the bottom and something solid under their feet.
	cp := world.ChunkPos{X: s.Level.SpawnX >> 4, Z: s.Level.SpawnZ >> 4}
	c, err := s.Chunk(cp)
	if err != nil || c == nil {
		t.Fatalf("chunk %v: %v %v", cp, c, err)
	}
	bedrock := 0
	for i := 0; i < 256; i++ {
		if c.Blocks[i]>>4 == 7 {
			bedrock++
		}
	}
	if bedrock == 0 {
		t.Fatal("no bedrock on y=0")
	}
	if far, err := s.Chunk(world.ChunkPos{X: 1 << 20, Z: 1 << 20}); far != nil || err != nil {
		t.Fatalf("a chunk nobody generated: %v %v", far, err)
	}
}

// A world saved by singleplayer keeps its player in playerdata/, not in level.dat.
func TestPlayerFromPlayerdata(t *testing.T) {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "bridge")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no 1.8.9 world")
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Level.GameType != 0 { // its player is in survival, per playerdata
		t.Fatalf("game type %d", s.Level.GameType)
	}
	if !s.Level.HasPlayer || s.Level.PlayerPos == [3]float64{} {
		t.Fatalf("no player position: %+v", s.Level)
	}
}
