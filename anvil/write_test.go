package anvil

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gocraft/world"
)

// The example from the NBT specification: TAG_Compound("hello world") { TAG_String("name"): "Bananrama" }.
func TestWriteNBTSpecVector(t *testing.T) {
	var b bytes.Buffer
	if err := WriteNBT(&b, Compound{"name": "Bananrama"}, "hello world"); err != nil {
		t.Fatal(err)
	}
	want := []byte{0x0a, 0x00, 0x0b, 'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd',
		0x08, 0x00, 0x04, 'n', 'a', 'm', 'e', 0x00, 0x09, 'B', 'a', 'n', 'a', 'n', 'r', 'a', 'm', 'a', 0x00}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("got % x\nwant % x", b.Bytes(), want)
	}
}

func TestNBTRoundTrip(t *testing.T) {
	in := Compound{
		"b": int8(-3), "s": int16(1234), "i": int32(-70000), "l": int64(1 << 40), "f": float32(1.5), "d": float64(-2.25),
		"str": "héllo", "bytes": []byte{1, 2, 3}, "ints": []int32{-1, 0, 7},
		"list":  []any{int32(1), int32(2)},
		"empty": []any{},
		"nest":  Compound{"x": Compound{"y": []any{Compound{"a": int8(1)}, Compound{"a": int8(2)}}}},
	}
	var b bytes.Buffer
	if err := WriteNBT(&b, in, ""); err != nil {
		t.Fatal(err)
	}
	out, err := ReadNBT(&b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed it:\n in %#v\nout %#v", in, out)
	}
	// The same compound always gives the same bytes.
	var b2, b3 bytes.Buffer
	WriteNBT(&b2, in, "")
	WriteNBT(&b3, in, "")
	if !bytes.Equal(b2.Bytes(), b3.Bytes()) {
		t.Fatal("two writes differ")
	}
	if err := WriteNBT(&b, Compound{"bad": []any{int8(1), int16(2)}}, ""); err == nil {
		t.Fatal("a list of mixed types was written")
	}
}

// testColumn is a column with a bit of everything: low and high block ids, metadata, light and biomes.
func testColumn(seed int) *world.Chunk {
	c := &world.Chunk{Light: make([]uint8, 65536), Biomes: make([]uint8, 256)}
	for y := 0; y < 70; y++ {
		for i := 0; i < 256; i++ {
			id := uint16((y*7+i*3+seed)%250) + 1
			if (y+i)%97 == 0 {
				id = 300 + uint16(seed%50) // ids above 255 need the Add array
			}
			c.Blocks[y<<8|i] = id<<4 | uint16((y+i+seed)&15)
			c.Light[y<<8|i] = uint8((y*13 + i + seed) & 0xFF)
		}
	}
	for i := range c.Biomes {
		c.Biomes[i] = uint8(i % 40)
	}
	return c
}

func writeTestLevel(t *testing.T, dir, name string) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	root := Compound{"Data": Compound{"LevelName": name, "SpawnX": int32(1), "SpawnY": int32(70), "SpawnZ": int32(2), "GameType": int32(1), "RandomSeed": int64(42)}}
	if err := WriteNBT(zw, root, ""); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "level.dat"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sameBlocks(t *testing.T, got, want *world.Chunk, what string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: no column", what)
	}
	if got.Blocks != want.Blocks {
		for i := range got.Blocks {
			if got.Blocks[i] != want.Blocks[i] {
				t.Fatalf("%s: block %d is %#x, want %#x", what, i, got.Blocks[i], want.Blocks[i])
			}
		}
	}
	if !bytes.Equal(got.Biomes, want.Biomes) {
		t.Fatalf("%s: biomes differ", what)
	}
	for sec := 0; sec < 16; sec++ { // light is compared where there are blocks: a section of air is left out
		empty := true
		for _, b := range want.Blocks[sec*4096 : sec*4096+4096] {
			if b != 0 {
				empty = false
			}
		}
		if empty {
			continue
		}
		for i := sec * 4096; i < sec*4096+4096; i++ {
			if got.Light[i] != want.Light[i] {
				t.Fatalf("%s: light %d is %#x, want %#x", what, i, got.Light[i], want.Light[i])
			}
		}
	}
}

// A region written here is read back by the reader the game uses, columns identical, including the
// negative coordinates (a column's slot in the file wraps modulo 32).
func TestRegionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeTestLevel(t, dir, "rt")
	cols := map[world.ChunkPos]*world.Chunk{{X: 0, Z: 0}: testColumn(1), {X: 31, Z: 31}: testColumn(2), {X: 5, Z: 17}: testColumn(3)}
	reg := &Region{}
	for cp, c := range cols {
		if err := reg.Set(cp, ChunkNBT(cp, c, nil), 1234); err != nil {
			t.Fatal(err)
		}
	}
	b, err := reg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(b)%sector != 0 {
		t.Fatalf("the file is %d bytes: not whole sectors", len(b))
	}
	os.MkdirAll(filepath.Join(dir, "region"), 0o755)
	os.WriteFile(filepath.Join(dir, "region", "r.0.0.mca"), b, 0o644)

	neg := &Region{}
	ncp := world.ChunkPos{X: -1, Z: -33} // region -1,-2, slot 31,31
	neg.Set(ncp, ChunkNBT(ncp, testColumn(4), nil), 1)
	nb, _ := neg.Bytes()
	os.WriteFile(filepath.Join(dir, "region", "r.-1.-2.mca"), nb, 0o644)

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for cp, want := range cols {
		got, err := s.Chunk(cp)
		if err != nil {
			t.Fatal(err)
		}
		sameBlocks(t, got, want, "column")
	}
	got, err := s.Chunk(ncp)
	if err != nil {
		t.Fatal(err)
	}
	sameBlocks(t, got, testColumn(4), "negative column")
	if c, _ := s.Chunk(world.ChunkPos{X: 9, Z: 9}); c != nil {
		t.Fatal("a column that was never written exists")
	}
	// And the file survives being read as a Region and written again unchanged.
	again, err := ReadRegion(b)
	if err != nil {
		t.Fatal(err)
	}
	ab, _ := again.Bytes()
	if !bytes.Equal(ab, b) {
		t.Fatal("a region read and written again changed")
	}
}

func TestChunkNBTKeepsWhatTheGameDoesNotEdit(t *testing.T) {
	base := Compound{"Level": Compound{
		"Entities":      []any{Compound{"id": "Pig", "Pos": []any{1.5, 4.0, 2.5}}},
		"TileEntities":  []any{Compound{"id": "Chest", "x": int32(1)}},
		"InhabitedTime": int64(999),
	}}
	cp := world.ChunkPos{X: 2, Z: 3}
	got := ChunkNBT(cp, testColumn(5), base)["Level"].(Compound)
	if len(got["Entities"].([]any)) != 1 || len(got["TileEntities"].([]any)) != 1 || got["InhabitedTime"] != int64(999) {
		t.Fatalf("entities, tile entities or inhabited time were lost: %v", got)
	}
	if got["xPos"] != int32(2) || got["zPos"] != int32(3) || got["TerrainPopulated"] != int8(1) {
		t.Fatalf("position or flags: %v", got)
	}
	if h := got["HeightMap"].([]int32); len(h) != 256 || h[0] != 70 {
		t.Fatalf("height map %v", h[:4])
	}
}

func fileHash(t *testing.T, p string) [20]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha1.Sum(b)
}

func TestExportCopiesTheWorldAndAppliesEdits(t *testing.T) {
	src := filepath.Join(t.TempDir(), "World")
	writeTestLevel(t, src, "Original")
	os.MkdirAll(filepath.Join(src, "region"), 0o755)
	os.MkdirAll(filepath.Join(src, "playerdata"), 0o755)
	os.WriteFile(filepath.Join(src, "session.lock"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(src, "playerdata", "p.dat"), []byte("player"), 0o644)

	keep, edit := testColumn(1), testColumn(2)
	reg := &Region{}
	keepCP, editCP := world.ChunkPos{X: 1, Z: 1}, world.ChunkPos{X: 2, Z: 2}
	reg.Set(keepCP, ChunkNBT(keepCP, keep, Compound{"Level": Compound{"Entities": []any{Compound{"id": "Cow"}}}}), 1)
	reg.Set(editCP, ChunkNBT(editCP, edit, Compound{"Level": Compound{"Entities": []any{Compound{"id": "Sheep"}}}}), 1)
	b, _ := reg.Bytes()
	regPath := filepath.Join(src, "region", "r.0.0.mca")
	os.WriteFile(regPath, b, 0o644)
	levelBefore, regBefore := fileHash(t, filepath.Join(src, "level.dat")), fileHash(t, regPath)

	edited := testColumn(2)
	edited.Blocks[5<<8|3<<4|4] = 57 << 4 // a diamond block
	far := testColumn(9)                 // a column in a region that did not exist
	farCP := world.ChunkPos{X: 40, Z: -3}
	dst := filepath.Join(t.TempDir(), "exports", "World-1")
	n, err := Export(src, dst, Edits{
		Chunks: map[world.ChunkPos]*world.Chunk{editCP: edited, farCP: far},
		Level:  func(d Compound) { d["LevelName"] = "Original (export)" },
	})
	if err != nil || n != 2 {
		t.Fatalf("export: %d columns, %v", n, err)
	}
	// The original is exactly as it was.
	if fileHash(t, filepath.Join(src, "level.dat")) != levelBefore || fileHash(t, regPath) != regBefore {
		t.Fatal("the export changed the original world")
	}
	if _, err := os.Stat(filepath.Join(dst, "session.lock")); err == nil {
		t.Fatal("the session lock was copied")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "playerdata", "p.dat")); string(b) != "player" {
		t.Fatal("playerdata was not copied")
	}
	s, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Level.Name != "Original (export)" || s.Level.GameType != 1 || s.Level.Seed != 42 {
		t.Fatalf("level %+v", s.Level)
	}
	got, _ := s.Chunk(editCP)
	sameBlocks(t, got, edited, "edited column")
	if got.Blocks[5<<8|3<<4|4]>>4 != 57 {
		t.Fatal("the edit is not in the export")
	}
	got, _ = s.Chunk(keepCP)
	sameBlocks(t, got, keep, "untouched column")
	got, _ = s.Chunk(farCP)
	sameBlocks(t, got, far, "column in a new region")
	// The entities the world had in an edited column survive.
	raw, _ := s.RawChunk(editCP)
	if ents := raw["Level"].(Compound)["Entities"].([]any); len(ents) != 1 || ents[0].(Compound)["id"] != "Sheep" {
		t.Fatalf("entities of the edited column: %v", ents)
	}

	// It refuses to write over the world, inside it, or into a folder that has things in it.
	for name, d := range map[string]string{"over": src, "inside": filepath.Join(src, "sub"), "non-empty": dst} {
		if _, err := Export(src, d, Edits{Chunks: map[world.ChunkPos]*world.Chunk{editCP: edited}}); err == nil {
			t.Errorf("an export %s was accepted", name)
		}
	}
	if fileHash(t, regPath) != regBefore {
		t.Fatal("a refused export changed the original")
	}
}

// With the player's own worlds (read-only): an export to a temporary folder opens, keeps every column
// the same, and shows the edit. Skipped without the folder.
func TestExportOfARealWorld(t *testing.T) {
	if testing.Short() {
		t.Skip("copies a whole world")
	}
	src := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "New World")
	orig, err := Open(src)
	if err != nil {
		t.Skip("no 1.8.9 world to export")
	}
	defer orig.Close()
	files, _ := filepath.Glob(filepath.Join(src, "region", "r.*.mca"))
	before := map[string][20]byte{}
	for _, f := range files {
		before[f] = fileHash(t, f)
	}
	// Find a column that exists, and edit one block in it.
	var cp world.ChunkPos
	var col *world.Chunk
	for z := int32(-4); z < 4 && col == nil; z++ {
		for x := int32(-4); x < 4 && col == nil; x++ {
			if c, _ := orig.Chunk(world.ChunkPos{X: x, Z: z}); c != nil {
				cp, col = world.ChunkPos{X: x, Z: z}, c
			}
		}
	}
	if col == nil {
		t.Skip("no columns near the origin")
	}
	edited := *col
	edited.Blocks[100<<8] = 57 << 4
	dst := filepath.Join(t.TempDir(), "real-export")
	if n, err := Export(src, dst, Edits{Chunks: map[world.ChunkPos]*world.Chunk{cp: &edited}}); err != nil || n != 1 {
		t.Fatalf("%d, %v", n, err)
	}
	for f, h := range before {
		if fileHash(t, f) != h {
			t.Fatalf("%s changed", f)
		}
	}
	out, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if out.Level.Seed != orig.Level.Seed || out.Level.Name != orig.Level.Name {
		t.Fatalf("level %+v vs %+v", out.Level, orig.Level)
	}
	got, _ := out.Chunk(cp)
	if got.Blocks[100<<8]>>4 != 57 {
		t.Fatal("the edit is missing")
	}
	// Every other column that exists in the original is the same.
	same := 0
	for z := int32(-8); z < 8; z++ {
		for x := int32(-8); x < 8; x++ {
			p := world.ChunkPos{X: x, Z: z}
			if p == cp {
				continue
			}
			a, _ := orig.Chunk(p)
			b, _ := out.Chunk(p)
			if (a == nil) != (b == nil) || a != nil && a.Blocks != b.Blocks {
				t.Fatalf("column %v differs", p)
			}
			if a != nil {
				same++
			}
		}
	}
	t.Logf("%d untouched columns identical, 1 edited", same)
}

func TestCreateWritesAWorldFromNothing(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "new")
	cols := map[world.ChunkPos]*world.Chunk{{X: 0, Z: 0}: testColumn(1), {X: -1, Z: 33}: testColumn(2)}
	data := Compound{"LevelName": "Made", "RandomSeed": int64(99), "generatorName": "default", "GameType": int32(1),
		"SpawnX": int32(3), "SpawnY": int32(70), "SpawnZ": int32(4), "DayTime": int64(1234)}
	n, err := Create(dst, data, cols)
	if err != nil || n != 2 {
		t.Fatalf("%d, %v", n, err)
	}
	s, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Level.Name != "Made" || s.Level.Seed != 99 || s.Level.Generator != "default" || s.Level.GameType != 1 || s.Level.SpawnZ != 4 || s.Level.DayTime != 1234 {
		t.Fatalf("level %+v", s.Level)
	}
	for cp, want := range cols {
		got, _ := s.Chunk(cp)
		sameBlocks(t, got, want, "column")
	}
	if _, err := Create(dst, data, cols); err == nil {
		t.Fatal("a world was created over a world")
	}
}
