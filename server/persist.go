package server

import (
	"log"
	"math"
	"sync"
	"time"

	"gocraft/anvil"
	"gocraft/proto"
	"gocraft/world"
)

// Keeping the world. With Config.SaveDir the server writes what players changed as a 1.8.9 world folder: the
// edited columns in region/ and a level.dat (so the game itself can open it), and each player's position, health
// and inventory as playerdata/<uuid>.dat. A column nobody edited is not written: it comes again from the
// generator, which is the seed's own. On start the saved columns are read before the generator is asked.

// saveInterval is how many ticks pass between saves: a minute.
const saveInterval = 1200

type store struct {
	dir  string
	mu   sync.Mutex
	save *anvil.Save // the opened folder, while there is a level.dat to read
}

// load is the saved column at cp, or nil when it was never saved.
func (st *store) load(cp world.ChunkPos) *world.Chunk {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.save == nil {
		sv, err := anvil.Open(st.dir)
		if err != nil {
			return nil // no level.dat yet: nothing was saved
		}
		st.save = sv
	}
	c, err := st.save.Chunk(cp)
	if err != nil {
		log.Printf("save: column %d,%d: %v", cp.X, cp.Z, err)
		return nil
	}
	return c
}

// write saves columns, and forgets the opened folder: its region files have changed under it.
func (st *store) write(level anvil.Compound, chunks map[world.ChunkPos]*world.Chunk) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.save != nil {
		st.save.Close()
		st.save = nil
	}
	return anvil.WriteChunks(st.dir, level, chunks)
}

// markEdited remembers that a column changed, so the next save writes it.
func (s *Server) markEdited(cp world.ChunkPos) {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	if s.edited == nil {
		s.edited = map[world.ChunkPos]bool{}
	}
	s.edited[cp] = true
	s.mu.Unlock()
}

// levelData is the Data compound of a new level.dat.
func (s *Server) levelData() anvil.Compound {
	gen := "flat"
	if s.cfg.Generator != nil {
		gen = "default"
	}
	sx, sy, sz, _ := s.spawnPoint()
	mode := int32(1)
	if s.cfg.Survival {
		mode = 0
	}
	return anvil.Compound{
		"LevelName": "gocraft", "RandomSeed": s.cfg.Seed, "generatorName": gen, "version": int32(19133),
		"GameType": mode, "MapFeatures": int8(0), "SpawnX": int32(math.Floor(sx)), "SpawnY": int32(math.Floor(sy)), "SpawnZ": int32(math.Floor(sz)),
		"Time": int64(0), "DayTime": int64(6000), "raining": int8(0), "thundering": int8(0),
	}
}

// Save writes the edited columns and every player's data. It reports the first failure and keeps what could not
// be written for the next try.
func (s *Server) Save() error {
	if s.store == nil {
		return nil
	}
	s.mu.Lock()
	cps := make([]world.ChunkPos, 0, len(s.edited))
	for cp := range s.edited {
		cps = append(cps, cp)
	}
	s.edited = nil
	players := make([]*Player, 0, len(s.players))
	for _, p := range s.players {
		players = append(players, p)
	}
	s.mu.Unlock()
	var first error
	if len(cps) > 0 {
		chunks := make(map[world.ChunkPos]*world.Chunk, len(cps))
		for _, cp := range cps {
			chunks[cp] = s.world.Snapshot(cp)
		}
		if err := s.store.write(s.levelData(), chunks); err != nil {
			first = err
			s.mu.Lock()
			if s.edited == nil {
				s.edited = map[world.ChunkPos]bool{}
			}
			for _, cp := range cps {
				s.edited[cp] = true
			}
			s.mu.Unlock()
		}
	}
	for _, p := range players {
		if !p.inWorld() {
			continue
		}
		if err := s.savePlayer(p); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ---- players ----

// vanillaSlot is the slot number the game's inventory NBT uses for a window slot of the player's own window: the
// hotbar 0-8 (window 36-44), the main inventory 9-35, the armour 100-103 (boots to helmet; window 8 down to 5).
func vanillaSlot(w int) (int, bool) {
	switch {
	case w >= 36 && w <= 44:
		return w - 36, true
	case w >= 9 && w <= 35:
		return w, true
	case w >= 5 && w <= 8:
		return 100 + (8 - w), true
	}
	return 0, false
}

// windowSlot is the inverse of vanillaSlot.
func windowSlot(v int) (int, bool) {
	switch {
	case v >= 0 && v <= 8:
		return 36 + v, true
	case v >= 9 && v <= 35:
		return v, true
	case v >= 100 && v <= 103:
		return 8 - (v - 100), true
	}
	return 0, false
}

func (s *Server) savePlayer(p *Player) error {
	p.mu.Lock()
	var inv []any
	for w, it := range p.inv {
		if v, ok := vanillaSlot(w); ok && it.id > 0 && it.count > 0 {
			inv = append(inv, anvil.Compound{"Slot": int8(v), "id": int16(it.id), "Count": it.count, "Damage": it.damage})
		}
	}
	mode := int32(1)
	if s.cfg.Survival {
		mode = 0
	}
	data := anvil.Compound{
		"Pos":      []any{p.x, p.y, p.z},
		"Rotation": []any{p.yaw, p.pitch},
		"Health":   p.health, "foodLevel": int32(20), "SelectedItemSlot": int32(p.held), "playerGameType": mode,
		"Dimension": int32(0), "Inventory": inv,
	}
	p.mu.Unlock()
	if inv == nil {
		data["Inventory"] = []any{}
	}
	return anvil.WritePlayer(s.store.dir, p.uuid, data)
}

// savedPlayer is what the saved data of a player says; ok is false for a player who was never here.
type savedPlayer struct {
	pos        [3]float64
	yaw, pitch float32
	health     float32
	held       int
	inv        [46]slot
}

func (s *Server) loadPlayer(uuid string) (sp savedPlayer, ok bool) {
	if s.store == nil {
		return sp, false
	}
	c := anvil.ReadPlayer(s.store.dir, uuid)
	if c == nil {
		return sp, false
	}
	pos, _ := c["Pos"].([]any)
	if len(pos) != 3 {
		return sp, false
	}
	for i := range pos {
		n, _ := anvil.Number(pos[i])
		sp.pos[i] = n
	}
	if !finite(sp.pos[0], sp.pos[1], sp.pos[2]) {
		return sp, false
	}
	if rot, _ := c["Rotation"].([]any); len(rot) == 2 {
		y, _ := anvil.Number(rot[0])
		pt, _ := anvil.Number(rot[1])
		sp.yaw, sp.pitch = float32(y), float32(pt)
	}
	sp.health = maxHealth
	if n, ok := anvil.Number(c["Health"]); ok && n > 0 {
		sp.health = min(float32(n), maxHealth)
	}
	if n, ok := anvil.Number(c["SelectedItemSlot"]); ok && n >= 0 && n < 9 {
		sp.held = int(n)
	}
	for i := range sp.inv {
		sp.inv[i].id = -1
	}
	items, _ := c["Inventory"].([]any)
	for _, v := range items {
		it, _ := v.(anvil.Compound)
		slotN, _ := anvil.Number(it["Slot"])
		id, _ := anvil.Number(it["id"])
		count, _ := anvil.Number(it["Count"])
		dmg, _ := anvil.Number(it["Damage"])
		if w, ok := windowSlot(int(slotN)); ok && id > 0 && id < 4096 && count > 0 {
			sp.inv[w] = slot{id: int16(id), count: int8(count), damage: int16(dmg)}
		}
	}
	return sp, true
}

// windowItemsPacket is Window Items for the player's own window: what a returning player carries.
func (p *Player) windowItemsPacket() []byte {
	w := proto.NewPacket(0x30)
	w.Byte(0)
	w.UShort(uint16(len(p.inv)))
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, it := range p.inv {
		if it.id <= 0 || it.count <= 0 {
			w.UShort(0xFFFF)
			continue
		}
		w.UShort(uint16(it.id))
		w.Byte(byte(it.count))
		w.UShort(uint16(it.damage))
		w.Byte(0) // no tag
	}
	return w.Frame()
}

// autosave runs from the tick: a save every saveInterval ticks, with the time it took if it was slow.
func (s *Server) autosave() {
	if s.store == nil {
		return
	}
	if s.ticks++; s.ticks%saveInterval != 0 {
		return
	}
	start := time.Now()
	if err := s.Save(); err != nil {
		log.Printf("save: %v", err)
	} else if d := time.Since(start); d > time.Second {
		log.Printf("save took %v", d)
	}
}
