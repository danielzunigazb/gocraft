package client

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gocraft/anvil"
	"gocraft/world"
	"gocraft/worldgen"
)

// The world's edits (overlay) and the player's state come out as a 1.8.9 world: the original is not
// touched, the edit is in the column, and level.dat has the player where the game left them.
func TestExportWorldFromTheOverlay(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // overlays and exports of a test go in a temporary folder
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	src := filepath.Join(t.TempDir(), "My World")
	if err := os.MkdirAll(filepath.Join(src, "region"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	level := anvil.Compound{"Data": anvil.Compound{
		"LevelName": "My World", "SpawnX": int32(0), "SpawnY": int32(70), "SpawnZ": int32(0), "GameType": int32(0), "RandomSeed": int64(7),
		"Player": anvil.Compound{"Pos": []any{0.5, 70.0, 0.5}, "Rotation": []any{float32(0), float32(0)}, "Health": float32(20), "foodLevel": int32(20), "Inventory": []any{}},
	}}
	if err := anvil.WriteNBT(zw, level, ""); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	os.WriteFile(filepath.Join(src, "level.dat"), buf.Bytes(), 0o644)

	cp := world.ChunkPos{X: 0, Z: 0}
	orig := &world.Chunk{Light: make([]uint8, 65536)}
	for i := 0; i < 256; i++ {
		orig.Blocks[i] = 7 << 4 // bedrock floor
	}
	reg := &anvil.Region{}
	reg.Set(cp, anvil.ChunkNBT(cp, orig, nil), 1)
	rb, _ := reg.Bytes()
	os.WriteFile(filepath.Join(src, "region", "r.0.0.mca"), rb, 0o644)

	// What the player did: a block placed, a sword in the hand, walked somewhere.
	edited := *orig
	edited.Blocks[1<<8|2<<4|3] = 41 << 4 // a gold block
	o := overlayFor(src)
	if err := o.saveChunk(cp, &edited); err != nil {
		t.Fatal(err)
	}
	st := playerState{Pos: [3]float64{10.5, 71, -3.5}, Yaw: 1.5707964, Health: 14, Food: 17, Selected: 2, Time: 6000}
	st.Inv[2] = stackJSON{ID: 267, N: 1}
	st.Armor[0] = stackJSON{ID: 310, N: 1} // a diamond helmet
	if err := o.savePlayer(st); err != nil {
		t.Fatal(err)
	}

	dst := exportDir("My World", time.Date(2026, 10, 29, 12, 30, 0, 0, time.UTC))
	if !strings.HasPrefix(dst, exportRoot()) || !strings.HasSuffix(dst, "My World-20261029-123000") {
		t.Fatalf("default export folder %s", dst)
	}
	n, err := exportWorld(src, dst, "My World")
	if err != nil || n != 1 {
		t.Fatalf("%d columns, %v", n, err)
	}
	s, err := anvil.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Level.Name != "My World (gocraft export)" || s.Level.DayTime != 6000 || s.Level.Seed != 7 {
		t.Fatalf("level %+v", s.Level)
	}
	if got, _ := s.Chunk(cp); got == nil || got.Blocks[1<<8|2<<4|3]>>4 != 41 || got.Blocks[0]>>4 != 7 {
		t.Fatal("the exported column lacks the edit or lost the floor")
	}
	if !s.Level.HasPlayer || s.Level.PlayerPos != [3]float64{10.5, 71, -3.5} || s.Level.Health != 14 || s.Level.Food != 17 {
		t.Fatalf("player %+v", s.Level)
	}
	inv := map[int]uint16{}
	for _, it := range s.Level.Inventory {
		inv[it.Slot] = it.ID
	}
	if inv[2] != 267 || inv[103] != 310 {
		t.Fatalf("inventory %v: want the sword in slot 2 and the helmet in 103", inv)
	}
	// The original world is as it was: no edit in its column.
	orig2, _ := anvil.Open(src)
	defer orig2.Close()
	if got, _ := orig2.Chunk(cp); got.Blocks[1<<8|2<<4|3] != 0 {
		t.Fatal("the export reached the original world")
	}
	// A second export to the same folder is refused.
	if _, err := exportWorld(src, dst, "My World"); err == nil {
		t.Fatal("an export over an export was accepted")
	}
}

func TestExportDirIsSafe(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for in, want := range map[string]string{
		"New World": "/data/gocraft/exports/New World-20260102-030405",
		"../../etc": "/data/gocraft/exports/.._.._etc-20260102-030405",
		"":          "/data/gocraft/exports/world-20260102-030405",
	} {
		if got := exportDir(in, at); got != want {
			t.Errorf("%q -> %s, want %s", in, got, want)
		}
	}
}

// A generated world has no folder of 1.8.9 files, so its export is built from nothing: a level.dat with the
// seed and the player, the columns around the player as generated, and every edited column.
func TestExportOfAGeneratedWorld(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := createWorld("Island", "12345", 1)
	if err != nil {
		t.Fatal(err)
	}
	lv, _ := readLevel(dir)
	gen := worldgen.NewGenerator(lv.Seed)
	lv.SpawnX, lv.SpawnZ = gen.Spawn()
	lv.HasSpwn = true
	if err := writeLevel(dir, lv); err != nil {
		t.Fatal(err)
	}
	// An edit far from the player (a column the radius does not reach) and the player's own state.
	far := world.ChunkPos{X: 40, Z: -25}
	edited := gen.Chunk(far)
	edited.Blocks[200<<8|1<<4|1] = 57 << 4
	o := overlayFor(dir)
	if err := o.saveChunk(far, edited); err != nil {
		t.Fatal(err)
	}
	st := playerState{Pos: [3]float64{4.5, 80, 6.5}, Health: 20, Food: 20, Selected: 1, Time: 4000}
	st.Inv[0] = stackJSON{ID: 276, N: 1}
	if err := o.savePlayer(st); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "island-export")
	n, err := exportGenerated(dir, dst, "Island", world.ChunkPos{X: 0, Z: 0}, 1, nil)
	if err != nil || n != 10 {
		t.Fatalf("%d columns, %v", n, err)
	}
	s, err := anvil.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Level.Seed != lv.Seed || s.Level.Generator != "default" || s.Level.GameType != 1 || s.Level.Name != "Island (gocraft export)" || s.Level.DayTime != 4000 {
		t.Fatalf("level %+v", s.Level)
	}
	if s.Level.SpawnX != lv.SpawnX || s.Level.SpawnZ != lv.SpawnZ || s.Level.SpawnY < 1 {
		t.Fatalf("spawn %d %d %d, want %d,_,%d", s.Level.SpawnX, s.Level.SpawnY, s.Level.SpawnZ, lv.SpawnX, lv.SpawnZ)
	}
	if !s.Level.HasPlayer || s.Level.PlayerPos != [3]float64{4.5, 80, 6.5} {
		t.Fatalf("player %+v", s.Level)
	}
	if len(s.Level.Inventory) != 1 || s.Level.Inventory[0].ID != 276 {
		t.Fatalf("inventory %v", s.Level.Inventory)
	}
	// The columns around the centre are what the generator makes, block for block.
	for dz := int32(-1); dz <= 1; dz++ {
		for dx := int32(-1); dx <= 1; dx++ {
			cp := world.ChunkPos{X: dx, Z: dz}
			got, err := s.Chunk(cp)
			if err != nil || got == nil {
				t.Fatalf("column %v: %v", cp, err)
			}
			if want := gen.Chunk(cp); got.Blocks != want.Blocks {
				t.Fatalf("column %v differs from the generator's", cp)
			}
		}
	}
	// The far one is the edited one, with the edit.
	got, _ := s.Chunk(far)
	if got == nil || got.Blocks[200<<8|1<<4|1]>>4 != 57 {
		t.Fatal("the far edited column or its edit is missing")
	}
	if c, _ := s.Chunk(world.ChunkPos{X: 5, Z: 5}); c != nil {
		t.Fatal("a column outside the radius was written")
	}
	// Nothing was written into the world's own folder besides what the game keeps there.
	if _, err := os.Stat(filepath.Join(dir, "level.dat")); err == nil {
		t.Fatal("the export put a level.dat into the generated world's folder")
	}
}
