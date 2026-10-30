package client

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"gocraft/anvil"
	"gocraft/world"
	"gocraft/worldgen"
)

// Worlds that are generated rather than read from the player's 1.8.9 folder. Each one is a folder
// under $XDG_DATA_HOME/gocraft/worlds with a level.json (name, seed, mode); the terrain comes from
// the generator and whatever the player edits is kept by the same overlay the loaded worlds use.
// The generator has no edge: columns are made as the player reaches them, at any distance.

const screenNewWorld screenKind = 100 // a menu screen (any value from screenTitle up is)

type generatedLevel struct {
	Name    string `json:"name"`
	Seed    int64  `json:"seed"`
	Mode    int32  `json:"mode"` // 0 survival, 1 creative
	Time    int64  `json:"time"`
	SpawnX  int32  `json:"spawnX"`
	SpawnZ  int32  `json:"spawnZ"`
	HasSpwn bool   `json:"hasSpawn"`
	Created int64  `json:"created"`
}

// worldsRoot is where generated worlds live.
func worldsRoot() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "gocraft", "worlds")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "gocraft", "worlds")
}

func levelPath(dir string) string { return filepath.Join(dir, "level.json") }

// isGeneratedWorld is whether dir is one of ours (a level.json, no level.dat).
func isGeneratedWorld(dir string) bool {
	_, err := os.Stat(levelPath(dir))
	return err == nil
}

func readLevel(dir string) (generatedLevel, error) {
	var lv generatedLevel
	b, err := os.ReadFile(levelPath(dir))
	if err != nil {
		return lv, err
	}
	return lv, json.Unmarshal(b, &lv)
}

func writeLevel(dir string, lv generatedLevel) error {
	b, _ := json.MarshalIndent(lv, "", "  ")
	return writeAtomicString(levelPath(dir), string(b)+"\n")
}

// scanGenerated lists the generated worlds, newest first.
func scanGenerated() []worldEntry {
	root := worldsRoot()
	ents, _ := os.ReadDir(root)
	type timed struct {
		e worldEntry
		t int64
	}
	var found []timed
	for _, d := range ents {
		dir := filepath.Join(root, d.Name())
		lv, err := readLevel(dir)
		if err != nil {
			continue
		}
		name := lv.Name
		if name == "" {
			name = d.Name()
		}
		found = append(found, timed{worldEntry{dir: dir, name: name, mode: lv.Mode, generated: true}, lv.Created})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].t > found[j].t })
	out := make([]worldEntry, len(found))
	for i, f := range found {
		out[i] = f.e
	}
	return out
}

// scanWorlds is what the world list shows: the generated worlds, then the ones in the saves folder.
func scanWorlds(saves string) []worldEntry {
	return append(scanGenerated(), scanSaves(saves)...)
}

// seedFromText is what the game does with the seed box: nothing typed (or 0) is random, a number
// is that number, any other text is its Java hash code.
func seedFromText(s string) int64 {
	s = strings.TrimSpace(s)
	if s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if n != 0 {
				return n
			}
		} else {
			return int64(worldgen.JavaHash(s))
		}
	}
	return time.Now().UnixNano() ^ int64(os.Getpid())<<32
}

var folderUnsafe = regexp.MustCompile(`[^A-Za-z0-9_\-]+`)

// createWorld makes the folder and level.json of a new world and returns the folder.
func createWorld(name, seedText string, mode int32) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "New World"
	}
	slug := strings.Trim(folderUnsafe.ReplaceAllString(name, "_"), "_")
	if slug == "" {
		slug = "world"
	}
	// A prefix keeps the overlay (named after the folder) apart from any save with the same name.
	base := "gen_" + slug
	dir := filepath.Join(worldsRoot(), base)
	for i := 2; ; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
		dir = filepath.Join(worldsRoot(), fmt.Sprintf("%s_%d", base, i))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	lv := generatedLevel{Name: name, Seed: seedFromText(seedText), Mode: mode, Created: time.Now().UnixNano()}
	return dir, writeLevel(dir, lv)
}

// loadGenerated opens a generated world and starts playing it.
func (g *game) loadGenerated(dir string) error {
	lv, err := readLevel(dir)
	if err != nil {
		return err
	}
	gen := worldgen.NewGenerator(lv.Seed)
	if !lv.HasSpwn {
		lv.SpawnX, lv.SpawnZ = gen.Spawn()
		lv.HasSpwn = true
		_ = writeLevel(dir, lv) // the spawn is chosen once; the next opening finds it here
	}
	g.closeWorld()
	g.save = anvil.Virtual(dir, anvil.Level{Name: lv.Name, Seed: lv.Seed, Generator: "default", GameType: lv.Mode})
	ov := overlayFor(dir)
	g.world = world.NewFrom(func(cp world.ChunkPos) *world.Chunk {
		if c, err := ov.loadChunk(cp); err != nil {
			logf("column %v of the saved edits: %v", cp, err)
		} else if c != nil {
			return c // what the player changed wins over the generated terrain
		}
		return gen.Chunk(cp)
	})
	g.world.SetLightRules(g.res.light)
	g.creative = lv.Mode == 1
	g.inv, g.armor, g.cursor = [36]slot{}, [4]slot{}, slot{}
	if g.creative {
		for i, st := range defaultHotbar {
			g.inv[i] = blockSlot(st, 1)
		}
	}
	g.health, g.food = maxHealth, maxFood
	g.yaw, g.pitch, g.sel = 0, 0, 0
	g.worldTime = lv.Time
	g.vel, g.flying, g.onGround, g.fall = vec3{}, false, false, 0
	g.pos = vec3{float64(lv.SpawnX) + 0.5, world.SpawnY + eyeHeight, float64(lv.SpawnZ) + 0.5}
	if y, ok := g.surfaceAt(lv.SpawnX, lv.SpawnZ); ok {
		g.pos[1] = y + eyeHeight
	}
	g.feet = g.freeSpot(g.pos.sub(vec3{0, eyeHeight, 0}))
	g.spawn = g.feet
	if p, ok := ov.loadPlayer(); ok && g.opt.Bench == 0 { // pick up where the player left off
		g.feet, g.yaw, g.pitch, g.sel = vec3(p.Pos), p.Yaw, p.Pitch, p.Selected
		g.health, g.food, g.flying = p.Health, p.Food, p.Flying && g.creative
		if p.Time > 0 {
			g.worldTime = p.Time
		}
		g.inv, g.armor = p.inventory()
		g.restoreDim(p.Dim)
		g.feet = g.freeSpot(g.feet)
		g.pos = g.feet.add(vec3{0, eyeHeight, 0})
	}
	g.prevFeet = g.feet
	g.inGame = true
	if g.win != nil && g.opt.Screenshot == "" {
		g.closeScreen()
	}
	logf("opened generated world %q (seed %d) at %.1f %.1f %.1f", lv.Name, lv.Seed, g.feet[0], g.feet[1], g.feet[2])
	return nil
}

// ---- the Create New World screen ----

var nw struct {
	name, seed string
	mode       int32
	msg        string
}

func (g *game) openNewWorld() {
	nw.name, nw.seed, nw.mode, nw.msg = "New World", "", 0, ""
	g.multiFocus = 0
	g.screen = screenNewWorld
}

// onCharAny is the window's character callback: the new-world form takes the text typed into it,
// every other screen leaves it to the chat and multiplayer code.
func (g *game) onCharAny(w *glfw.Window, r rune) {
	if g.screen != screenNewWorld {
		g.onChar(w, r)
		return
	}
	if r < 32 || r > 255 || r == 127 {
		return
	}
	f, limit := &nw.name, 32
	if g.multiFocus == 1 {
		f, limit = &nw.seed, 32
	}
	if len(*f) < limit {
		*f += string(r)
	}
}

// newWorldKey handles the keys of the form; it reports whether it took the key.
func (g *game) newWorldKey(key glfw.Key, action glfw.Action) bool {
	if g.screen != screenNewWorld || action == glfw.Release {
		return false
	}
	f := &nw.name
	if g.multiFocus == 1 {
		f = &nw.seed
	}
	switch key {
	case glfw.KeyBackspace:
		if n := len(*f); n > 0 {
			*f = (*f)[:n-1]
		}
	case glfw.KeyTab:
		g.multiFocus = 1 - g.multiFocus
	case glfw.KeyEnter, glfw.KeyKPEnter:
		g.createAndPlay()
	case glfw.KeyEscape:
		g.worldList, g.screen = scanWorlds(g.saves), screenWorlds
	}
	return true
}

func (g *game) createAndPlay() {
	dir, err := createWorld(nw.name, nw.seed, nw.mode)
	if err == nil {
		err = g.loadWorld(dir)
	}
	if err != nil {
		nw.msg = err.Error()
	}
}

func (g *game) drawNewWorld(cx, s float32) {
	g.dirtBackground(0.45)
	left := cx/s - 100
	g.centredText("Create New World", cx, 20*s, s, [4]float32{1, 1, 1, 1})
	top := float32(g.fb[1])/s/4 - 8
	g.textField("World name", nw.name, left, top, 200, g.multiFocus == 0, 0)
	g.textField("Seed for the world generator", nw.seed, left, top+36, 200, g.multiFocus == 1, 1)
	g.text("Leave blank for a random seed", (left)*s, (top+58)*s, s, [4]float32{0.5, 0.5, 0.5, 1}, true)
	mode := "Game Mode: Survival"
	if nw.mode == 1 {
		mode = "Game Mode: Creative"
	}
	if g.button(mode, left, top+74, 200, true) {
		nw.mode = 1 - nw.mode
	}
	if nw.msg != "" {
		g.centredText(trimName(nw.msg, 52), cx, (top+100)*s, s, [4]float32{1, 0.4, 0.4, 1})
	}
	if g.button("Create New World", left, top+112, 200, true) {
		g.createAndPlay()
	}
	if g.button("Cancel", left, top+136, 200, true) {
		g.worldList, g.screen = scanWorlds(g.saves), screenWorlds
	}
}

var _ = math.Pi
