package anvil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"gocraft/world"
)

// Level is the part of level.dat the game needs to open a world.
type Level struct {
	Name           string
	SpawnX, SpawnY int32
	SpawnZ         int32
	HasPlayer      bool
	PlayerPos      [3]float64
	PlayerYaw      float32
	PlayerPitch    float32
	DayTime        int64 // the time of day, in ticks, as the world was saved
	GameType       int32 // 0 survival, 1 creative, 2 adventure, 3 spectator
	Health         float32
	Food           int32
	Inventory      []Stack // everything the player carries: slots 0-35, armour 100-103
	Seed           int64   // RandomSeed: what the world was generated from
	Generator      string  // generatorName: "default", "flat", "largeBiomes"...
	MapFeatures    bool    // villages, strongholds and the like were generated
}

// Stack is an item stack of the player's inventory.
type Stack struct {
	Slot    int
	ID, Dmg uint16
	Count   int8
}

// Save is an opened world folder.
type Save struct {
	Dir   string
	Level Level

	mu    sync.Mutex
	files map[[2]int32]*os.File // region files, by region coordinates; a nil entry is a missing file
}

func Open(dir string) (*Save, error) {
	f, err := os.Open(filepath.Join(dir, "level.dat"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("level.dat: %w", err)
	}
	root, err := ReadNBT(zr)
	if err != nil {
		return nil, fmt.Errorf("level.dat: %w", err)
	}
	data, _ := root["Data"].(Compound)
	s := &Save{Dir: dir, files: map[[2]int32]*os.File{}}
	s.Level.Name, _ = data["LevelName"].(string)
	if seed, ok := data["RandomSeed"].(int64); ok {
		s.Level.Seed = seed
	}
	s.Level.Generator, _ = data["generatorName"].(string)
	if n, ok := Number(data["MapFeatures"]); ok {
		s.Level.MapFeatures = n != 0
	}
	num := func(c Compound, k string) int32 { n, _ := Number(c[k]); return int32(n) }
	if n, ok := Number(data["DayTime"]); ok {
		s.Level.DayTime = int64(n)
	} else if n, ok := Number(data["Time"]); ok {
		s.Level.DayTime = int64(n)
	}
	if n, ok := Number(data["GameType"]); ok {
		s.Level.GameType = int32(n)
	}
	s.Level.SpawnX, s.Level.SpawnY, s.Level.SpawnZ = num(data, "SpawnX"), num(data, "SpawnY"), num(data, "SpawnZ")
	p, ok := data["Player"].(Compound)
	if !ok {
		p = s.newestPlayer() // a world with a server's layout keeps its players beside level.dat
	}
	if p != nil {
		if n, ok := Number(p["Health"]); ok {
			s.Level.Health = float32(n)
		}
		if n, ok := Number(p["foodLevel"]); ok {
			s.Level.Food = int32(n)
		}
		inv, _ := p["Inventory"].([]any)
		for _, v := range inv {
			it, _ := v.(Compound)
			slot, _ := Number(it["Slot"])
			id, _ := Number(it["id"])
			count, _ := Number(it["Count"])
			dmg, _ := Number(it["Damage"])
			if (slot >= 0 && slot < 36 || slot >= 100 && slot < 104) && id > 0 && id < 4096 && count > 0 {
				s.Level.Inventory = append(s.Level.Inventory, Stack{Slot: int(slot), ID: uint16(id), Dmg: uint16(dmg), Count: int8(count)})
			}
		}
		if n, ok := Number(p["playerGameType"]); ok { // the player's own mode wins over the world's, as in the game
			s.Level.GameType = int32(n)
		}
		if pos, ok := p["Pos"].([]any); ok && len(pos) == 3 {
			for i := range pos {
				s.Level.PlayerPos[i], _ = Number(pos[i])
			}
			s.Level.HasPlayer = true
		}
		if rot, ok := p["Rotation"].([]any); ok && len(rot) == 2 {
			y, _ := Number(rot[0])
			p, _ := Number(rot[1])
			s.Level.PlayerYaw, s.Level.PlayerPitch = float32(y), float32(p)
		}
	}
	return s, nil
}

// newestPlayer is the most recently saved file of playerdata/, or nil.
func (s *Save) newestPlayer() Compound {
	files, _ := filepath.Glob(filepath.Join(s.Dir, "playerdata", "*.dat"))
	var best string
	var bestTime int64
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && st.ModTime().UnixNano() > bestTime {
			best, bestTime = f, st.ModTime().UnixNano()
		}
	}
	if best == "" {
		return nil
	}
	f, err := os.Open(best)
	if err != nil {
		return nil
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	root, err := ReadNBT(zr)
	if err != nil {
		return nil
	}
	return root
}

func (s *Save) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.files {
		if f != nil {
			f.Close()
		}
	}
	s.files = map[[2]int32]*os.File{}
}

func (s *Save) region(rx, rz int32) *os.File {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := [2]int32{rx, rz}
	if f, ok := s.files[k]; ok {
		return f
	}
	f, err := os.Open(filepath.Join(s.Dir, "region", fmt.Sprintf("r.%d.%d.mca", rx, rz)))
	if err != nil {
		f = nil
	}
	s.files[k] = f
	return f
}

// RawChunk returns the NBT root of a column as it is stored (its entities, tile entities and all),
// or nil if the world never generated it.
func (s *Save) RawChunk(cp world.ChunkPos) (Compound, error) {
	root, err := s.readChunk(cp)
	return root, err
}

// Chunk loads one column, or returns nil if the world never generated it.
func (s *Save) Chunk(cp world.ChunkPos) (*world.Chunk, error) {
	root, err := s.readChunk(cp)
	if root == nil || err != nil {
		return nil, err
	}
	level, _ := root["Level"].(Compound)
	return decodeChunk(level), nil
}

func (s *Save) readChunk(cp world.ChunkPos) (Compound, error) {
	f := s.region(cp.X>>5, cp.Z>>5)
	if f == nil {
		return nil, nil
	}
	var loc [4]byte
	if _, err := f.ReadAt(loc[:], int64((cp.X&31)+(cp.Z&31)*32)*4); err != nil {
		return nil, nil // a truncated header: no such chunk
	}
	offset := int64(loc[0])<<16 | int64(loc[1])<<8 | int64(loc[2])
	if offset == 0 || loc[3] == 0 {
		return nil, nil
	}
	var head [5]byte
	if _, err := f.ReadAt(head[:], offset*4096); err != nil {
		return nil, err
	}
	length := int64(binary.BigEndian.Uint32(head[:4])) - 1
	if length <= 0 || length > int64(loc[3])*4096 {
		return nil, fmt.Errorf("chunk %v: bad length %d", cp, length)
	}
	raw := make([]byte, length)
	if _, err := f.ReadAt(raw, offset*4096+5); err != nil {
		return nil, err
	}
	var r io.Reader
	var err error
	switch head[4] {
	case 1:
		r, err = gzip.NewReader(bytes.NewReader(raw))
	case 2:
		r, err = zlib.NewReader(bytes.NewReader(raw))
	default:
		return nil, fmt.Errorf("chunk %v: compression %d", cp, head[4])
	}
	if err != nil {
		return nil, err
	}
	root, err := ReadNBT(r)
	if err != nil {
		return nil, fmt.Errorf("chunk %v: %w", cp, err)
	}
	return root, nil
}

// decodeChunk turns the "Sections" of a column into block states (id<<4 | meta). A section is
// 4096 block ids, optionally with 4 more high bits ("Add"), and 4096 metadata nibbles ("Data"),
// both indexed y<<8 | z<<4 | x within the section.
func decodeChunk(level Compound) *world.Chunk {
	c := &world.Chunk{Light: make([]uint8, 65536)}
	for i := range c.Light {
		c.Light[i] = world.FullLight // sections the file leaves out are open air: lit by the sky
	}
	if b, ok := level["Biomes"].([]byte); ok && len(b) == 256 {
		c.Biomes = b
	}
	secs, _ := level["Sections"].([]any)
	for _, sv := range secs {
		sec, _ := sv.(Compound)
		yv, _ := Number(sec["Y"])
		y := int(yv)
		blocks, _ := sec["Blocks"].([]byte)
		data, _ := sec["Data"].([]byte)
		add, _ := sec["Add"].([]byte)
		if y < 0 || y > 15 || len(blocks) != 4096 {
			continue
		}
		sky, _ := sec["SkyLight"].([]byte)
		blk, _ := sec["BlockLight"].([]byte)
		for i := 0; i < 4096; i++ {
			var l uint8
			if len(sky) == 2048 {
				l = nibble(sky, i) << 4
			}
			if len(blk) == 2048 {
				l |= nibble(blk, i)
			}
			c.Light[y*4096+i] = l
		}
		for i := 0; i < 4096; i++ {
			id := uint16(blocks[i])
			if len(add) == 2048 {
				id |= uint16(nibble(add, i)) << 8
			}
			var meta uint16
			if len(data) == 2048 {
				meta = uint16(nibble(data, i))
			}
			c.Blocks[y*4096+i] = id<<4 | meta
		}
	}
	return c
}

func nibble(a []byte, i int) byte {
	if i&1 == 0 {
		return a[i>>1] & 15
	}
	return a[i>>1] >> 4
}
