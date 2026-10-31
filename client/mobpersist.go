package client

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"gocraft/world"
)

// Mobs and dropped items outlive a session: the mobs of a column are kept in the overlay with the
// column's edits, so a mob that was killed stays dead and one that was led away stays where it was left.
// A column's entities are read from the player's world file the first time it loads (and never again),
// and from then on from the overlay. Mobs of columns that fall behind are put away and come back with
// the column; hostile mobs far from the player simply go (EntityLiving.despawnEntity).

type mobJSON struct {
	ID      string  `json:"id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Z       float64 `json:"z"`
	Yaw     float32 `json:"yaw"`
	Health  float32 `json:"health"`
	Colour  int     `json:"colour,omitempty"`
	Sheared bool    `json:"sheared,omitempty"`
	Size    int     `json:"size,omitempty"`
	Persist bool    `json:"persist,omitempty"`
}

type itemJSON struct {
	Stack stackJSON  `json:"stack"`
	Pos   [3]float64 `json:"pos"`
	Age   int        `json:"age"`
}

type mobsFile struct {
	Resolved []string             `json:"resolved"` // columns whose entities came from the world file already
	Mobs     map[string][]mobJSON `json:"mobs"`     // mobs put away with their column
	Live     []mobJSON            `json:"live"`     // the mobs that were in the game when it was saved
	Items    []itemJSON           `json:"items"`
}

func colKey(cp world.ChunkPos) string { return fmt.Sprintf("%d,%d", cp.X, cp.Z) }

func parseCol(k string) (world.ChunkPos, bool) {
	var x, z int32
	if _, err := fmt.Sscanf(k, "%d,%d", &x, &z); err != nil {
		return world.ChunkPos{}, false
	}
	return world.ChunkPos{X: x, Z: z}, true
}

func toMobJSON(m *mob) mobJSON {
	return mobJSON{ID: m.kind.id, X: m.pos[0], Y: m.pos[1], Z: m.pos[2], Yaw: m.yaw, Health: m.health,
		Colour: m.colour, Sheared: m.sheared, Size: m.size, Persist: m.persist}
}

func (g *game) fromMobJSON(j mobJSON) *mob {
	k := kinds[j.ID]
	if k == nil {
		return nil
	}
	m := g.newMob(k, vec3{float64(j.X), float64(j.Y), float64(j.Z)}, j.Yaw)
	m.health, m.colour, m.sheared, m.size, m.persist = j.Health, j.Colour, j.Sheared, j.Size, j.Persist
	return m
}

func (o overlay) mobsPath() string { return filepath.Join(o.dir, "mobs.json") }

func mobColumn(m *mob) world.ChunkPos {
	return world.ChunkPos{X: int32(math.Floor(float64(m.pos[0]))) >> 4, Z: int32(math.Floor(float64(m.pos[2]))) >> 4}
}

// saveMobStore writes the mobs that are put away, the ones in the game and the dropped items.
func (g *game) saveMobStore(o overlay) {
	f := mobsFile{Mobs: map[string][]mobJSON{}}
	for cp := range g.be.resolved {
		f.Resolved = append(f.Resolved, colKey(cp))
	}
	for cp, ms := range g.be.mobStore {
		if len(ms) > 0 {
			f.Mobs[colKey(cp)] = ms
		}
	}
	for _, m := range g.mobs {
		if !m.dead() && m.kind.id != "PvPBot" {
			f.Live = append(f.Live, toMobJSON(m))
		}
	}
	for _, it := range g.items {
		if !it.stack.empty() && !it.net {
			f.Items = append(f.Items, itemJSON{Stack: toJSON(it.stack), Pos: it.pos, Age: it.age})
		}
	}
	b, _ := json.Marshal(f)
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		logf("saving mobs: %v", err)
		return
	}
	if err := writeAtomic(o.mobsPath(), func(w io.Writer) error { _, err := w.Write(b); return err }); err != nil {
		logf("saving mobs: %v", err)
	}
}

// loadMobStore starts a world's entities afresh and reads back what was saved.
func (g *game) loadMobStore(o overlay) {
	g.mobs, g.items, g.arrows, g.entityChunks = nil, nil, nil, nil
	g.be.resolved, g.be.mobStore = map[world.ChunkPos]bool{}, map[world.ChunkPos][]mobJSON{}
	b, err := os.ReadFile(o.mobsPath())
	if err != nil {
		return
	}
	var f mobsFile
	if json.Unmarshal(b, &f) != nil {
		return
	}
	for _, k := range f.Resolved {
		if cp, ok := parseCol(k); ok {
			g.be.resolved[cp] = true
		}
	}
	for k, ms := range f.Mobs {
		if cp, ok := parseCol(k); ok {
			g.be.mobStore[cp] = ms
		}
	}
	// The mobs that were in the game come back as they were; their columns count as loaded.
	g.entityChunks = map[world.ChunkPos]bool{}
	for _, j := range f.Live {
		if m := g.fromMobJSON(j); m != nil {
			g.mobs = append(g.mobs, m)
			g.entityChunks[mobColumn(m)] = true
		}
	}
	for _, it := range f.Items {
		p := vec3(it.Pos)
		g.items = append(g.items, &itemEntity{stack: slot{it.Stack.ID, it.Stack.Dmg, it.Stack.N}, pos: p, prev: p, age: it.Age, pickupDelay: 10})
	}
}

// spawnStored brings the mobs put away with a column back into the game.
func (g *game) spawnStored(cp world.ChunkPos) {
	for _, j := range g.be.mobStore[cp] {
		if m := g.fromMobJSON(j); m != nil {
			g.mobs = append(g.mobs, m)
		}
	}
	delete(g.be.mobStore, cp)
}

// unloadEntities puts away the mobs of a column that has fallen behind.
func (g *game) unloadEntities(cp world.ChunkPos) {
	keep := g.mobs[:0]
	for _, m := range g.mobs {
		if mobColumn(m) == cp && !m.dead() && m.kind.id != "PvPBot" {
			g.be.mobStore[cp] = append(g.be.mobStore[cp], toMobJSON(m))
			continue
		}
		keep = append(keep, m)
	}
	g.mobs = keep
	delete(g.entityChunks, cp)
}

// despawns is EntityLiving.despawnEntity for a mob: hostile mobs go when more than 128 blocks from the
// player, and after 30 seconds with the player more than 32 blocks off, at random.
func (g *game) despawns(m *mob, d2 float64) bool {
	if !m.kind.hostile || m.persist || m.kind.id == "PvPBot" {
		return false
	}
	if d2 > 128*128 {
		return true
	}
	if d2 < 32*32 {
		m.despawnAge = 0
		return false
	}
	return m.despawnAge > 600 && rng.Intn(800) == 0
}
