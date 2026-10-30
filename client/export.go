package client

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gocraft/anvil"
	"gocraft/world"
	"gocraft/worldgen"
)

// Exporting a world. The game never writes into the worlds of the player's own 1.8.9 folder: the edits
// live in an overlay (see overlay.go). "Export World" in the pause menu writes a COPY of the world, with
// those edits applied, into a new folder, as a world 1.8.9 itself opens. The original stays as it was.

var exporting struct {
	mu   sync.Mutex
	busy bool
	msg  string
}

func exportStatus() (msg string, busy bool) {
	exporting.mu.Lock()
	defer exporting.mu.Unlock()
	return exporting.msg, exporting.busy
}

func setExport(busy bool, msg string) {
	exporting.mu.Lock()
	exporting.busy, exporting.msg = busy, msg
	exporting.mu.Unlock()
}

// exportRoot is where exports go by default: $XDG_DATA_HOME/gocraft/exports.
func exportRoot() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "gocraft", "exports")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "gocraft", "exports")
}

// exportDir is the folder for an export of the named world made at the given time.
func exportDir(name string, at time.Time) string {
	clean := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" || clean == "." || clean == ".." {
		clean = "world"
	}
	return filepath.Join(exportRoot(), clean+"-"+at.Format("20060102-150405"))
}

// canExport is whether the open world can be exported: one from a save folder, not a server's.
func (g *game) canExport() bool { return g.save != nil && g.net == nil && g.inGame }

// startExport flushes the edits to the overlay (on the game thread, which owns the world) and copies the
// world in the background. The pause menu shows how it went.
func (g *game) startExport() {
	if !g.canExport() {
		return
	}
	if _, busy := exportStatus(); busy {
		return
	}
	g.saveWorld()
	srcDir, name := g.save.Dir, g.save.Level.Name
	if name == "" {
		name = filepath.Base(srcDir)
	}
	dst := exportDir(filepath.Base(srcDir), time.Now())
	setExport(true, "Exporting...")
	centre := world.ChunkPos{X: int32(math.Floor(float64(g.feet[0]))) >> 4, Z: int32(math.Floor(float64(g.feet[2]))) >> 4}
	generated := isGeneratedWorld(srcDir)
	go func() {
		var n int
		var err error
		if generated {
			n, err = exportGenerated(srcDir, dst, name, centre, exportRadius, func(done, total int) {
				setExport(true, fmt.Sprintf("Exporting... %d / %d columns", done, total))
			})
		} else {
			n, err = exportWorld(srcDir, dst, name)
		}
		if err != nil {
			setExport(false, "Export failed: "+err.Error())
			return
		}
		setExport(false, fmt.Sprintf("Exported %d columns to %s", n, dst))
	}()
}

// exportWorld copies the world in srcDir to dst with its saved edits applied, and says how many columns it wrote.
func exportWorld(srcDir, dst, name string) (int, error) {
	o := overlayFor(srcDir)
	chunks, err := overlayChunks(o)
	if err != nil {
		return 0, err
	}
	p, havePlayer := o.loadPlayer()
	return anvil.Export(srcDir, dst, anvil.Edits{Chunks: chunks, Level: func(d anvil.Compound) {
		levelEdits(d, name, p, havePlayer, time.Now())
	}})
}

// levelEdits brings level.dat's "Data" up to date: the name, when it was last played, the time of day and
// the player as the game left them.
func levelEdits(d anvil.Compound, name string, p playerState, havePlayer bool, now time.Time) {
	d["LevelName"] = name + " (gocraft export)"
	d["LastPlayed"] = now.UnixMilli()
	if !havePlayer {
		return
	}
	if p.Time > 0 {
		d["DayTime"] = p.Time
	}
	pl, ok := d["Player"].(anvil.Compound)
	if !ok {
		return // a world that keeps its player in playerdata/: left as it is
	}
	fillPlayer(pl, p)
}

// fillPlayer writes the player as the game left them into a player compound of level.dat.
func fillPlayer(pl anvil.Compound, p playerState) {
	pl["Pos"] = []any{float64(p.Pos[0]), float64(p.Pos[1]), float64(p.Pos[2])}
	pl["Rotation"] = []any{float32(p.Yaw * 180 / math.Pi), float32(p.Pitch * 180 / math.Pi)}
	pl["Health"] = p.Health
	pl["foodLevel"] = int32(p.Food)
	pl["SelectedItemSlot"] = int32(p.Selected)
	items := []any{}
	add := func(slot int8, s stackJSON) {
		if s.ID != 0 && s.N > 0 {
			items = append(items, anvil.Compound{"Count": s.N, "Damage": int16(s.Dmg), "Slot": slot, "id": int16(s.ID)})
		}
	}
	for i, s := range p.Inv {
		add(int8(i), s)
	}
	for i, s := range p.Armor { // our helmet is the first; the game numbers armour 103 (helmet) down to 100 (boots)
		add(int8(103-i), s)
	}
	pl["Inventory"] = items
}

// drawExportButton is the pause menu's Export World button and, under the menu, how the last one went.
func (g *game) drawExportButton(left, y, s float32) {
	if !g.canExport() {
		return
	}
	msg, busy := exportStatus()
	if g.button("Export World", left, y, 200, !busy) {
		g.startExport()
	}
	if msg != "" {
		g.text(trimName(msg, 60), 4*s, float32(g.fb[1])-12*s, s, [4]float32{0.8, 0.8, 0.8, 1}, true)
	}
}

// overlayChunks are the columns the player edited, as the overlay has them.
func overlayChunks(o overlay) (map[world.ChunkPos]*world.Chunk, error) {
	files, _ := filepath.Glob(filepath.Join(o.dir, "c.*.bin"))
	chunks := map[world.ChunkPos]*world.Chunk{}
	for _, f := range files {
		var cp world.ChunkPos
		if _, err := fmt.Sscanf(filepath.Base(f), "c.%d.%d.bin", &cp.X, &cp.Z); err != nil {
			continue
		}
		c, err := o.loadChunk(cp)
		if err != nil {
			return nil, fmt.Errorf("column %d,%d: %w", cp.X, cp.Z, err)
		}
		if c != nil {
			chunks[cp] = c
		}
	}
	return chunks, nil
}

// exportRadius is how many columns around the player a generated world's export holds, besides every
// column that was edited. Further away the 1.8.9 generates its own terrain from the seed.
const exportRadius = 10

// exportGenerated writes a generated world (one with a level.json and no folder of 1.8.9 files) as a 1.8.9
// world: a level.dat with its seed and the player, the columns within radius of the centre as the game
// generated them and every column the player edited. progress, if set, is told how far it got.
func exportGenerated(dir, dst, name string, center world.ChunkPos, radius int, progress func(done, total int)) (int, error) {
	lv, err := readLevel(dir)
	if err != nil {
		return 0, err
	}
	o := overlayFor(dir)
	chunks, err := overlayChunks(o)
	if err != nil {
		return 0, err
	}
	gen := worldgen.NewGenerator(lv.Seed)
	total := (2*radius+1)*(2*radius+1) + len(chunks)
	done := 0
	for dz := -radius; dz <= radius; dz++ {
		for dx := -radius; dx <= radius; dx++ {
			cp := world.ChunkPos{X: center.X + int32(dx), Z: center.Z + int32(dz)}
			if chunks[cp] == nil { // an edited column is kept as it was edited
				chunks[cp] = gen.Chunk(cp)
			}
			if done++; progress != nil && done%16 == 0 {
				progress(done, total)
			}
		}
	}
	p, havePlayer := o.loadPlayer()
	now := time.Now()
	t := lv.Time
	if havePlayer && p.Time > 0 {
		t = p.Time
	}
	spawnY := int32(64)
	if c := chunks[world.ChunkPos{X: lv.SpawnX >> 4, Z: lv.SpawnZ >> 4}]; c != nil {
		for y := 255; y >= 0; y-- {
			if c.Blocks[y<<8|int(lv.SpawnZ&15)<<4|int(lv.SpawnX&15)] != 0 {
				spawnY = int32(y + 1)
				break
			}
		}
	}
	data := anvil.Compound{
		"version": int32(19133), "initialized": int8(1), "LevelName": name + " (gocraft export)",
		"generatorName": "default", "generatorVersion": int32(1), "generatorOptions": "", "RandomSeed": lv.Seed,
		"MapFeatures": int8(1), "GameType": lv.Mode, "hardcore": int8(0), "allowCommands": int8(boolInt(lv.Mode == 1)),
		"Time": t, "DayTime": t, "SpawnX": lv.SpawnX, "SpawnY": spawnY, "SpawnZ": lv.SpawnZ,
		"LastPlayed": now.UnixMilli(), "SizeOnDisk": int64(0), "Difficulty": int8(2), "DifficultyLocked": int8(0),
		"raining": int8(0), "rainTime": int32(0), "thundering": int8(0), "thunderTime": int32(0),
		"GameRules": anvil.Compound{"commandBlockOutput": "true", "doDaylightCycle": "true", "doFireTick": "true", "doMobLoot": "true",
			"doMobSpawning": "true", "doTileDrops": "true", "keepInventory": "false", "mobGriefing": "true", "naturalRegeneration": "true",
			"randomTickSpeed": "3", "reducedDebugInfo": "false", "sendCommandFeedback": "true", "showDeathMessages": "true"},
	}
	if havePlayer {
		pl := anvil.Compound{"Dimension": int32(0), "OnGround": int8(1), "Air": int16(300), "Fire": int16(-20), "FallDistance": float32(0),
			"playerGameType": lv.Mode}
		fillPlayer(pl, p)
		data["Player"] = pl
	}
	return anvil.Create(dst, data, chunks)
}
