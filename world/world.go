// Package world is the block world shared by the server and the client: chunks of block states,
// generated flat for now. Nothing here knows about the network or the screen.
package world

import "sync"

type ChunkPos struct{ X, Z int32 }

// Chunk holds 16 sections of 16x16x16 blocks as vanilla stores them: id<<4 | meta, indexed y<<8 | z<<4 | x.
type Chunk struct {
	Blocks [65536]uint16
	// Light is sky light in the high nibble and block light in the low one, indexed like Blocks.
	// It is nil for a column that has none (generated flat, say), which draws fully lit.
	Light []uint8
	// Biomes is a biome id per column (z<<4 | x); nil means plains everywhere.
	Biomes []uint8
}

// BiomeAt is the biome id of the column at world coordinates x, z (1 is plains).
func (c *Chunk) BiomeAt(x, z int32) uint8 {
	if c.Biomes == nil {
		return 1
	}
	return c.Biomes[int(z&15)<<4|int(x&15)]
}

// FullLight is daylight in the open: sky 15, block 0.
const FullLight = 0xF0

// World is the one Overworld, generated per chunk on first use. It is safe for concurrent use:
// the game thread edits it while workers copy columns to mesh them.
type World struct {
	mu     sync.Mutex
	chunks map[ChunkPos]*Chunk
	gen    func(ChunkPos) *Chunk
	dirty  map[ChunkPos]bool // columns changed since the last TakeDirty
	rules  *LightRules
}

func New() *World { return &World{chunks: map[ChunkPos]*Chunk{}} }

// NewFrom is a world whose columns come from gen (a save on disk, say) the first time they are
// used. A nil result is an empty column. gen may be called from several goroutines at once.
func NewFrom(gen func(ChunkPos) *Chunk) *World {
	return &World{chunks: map[ChunkPos]*Chunk{}, gen: gen}
}

// Flat layers, bottom up: bedrock, 2 dirt, grass. The surface is y=3, so the first free block is y=4.
var flatLayers = []uint16{7 << 4, 3 << 4, 3 << 4, 2 << 4}

const SpawnY = 4

func fillFlat(c *Chunk) {
	for y, id := range flatLayers {
		for i := 0; i < 256; i++ {
			c.Blocks[y<<8|i] = id
		}
	}
}

// get returns the column, generating or loading it first if needed. Loading (disk, decompression)
// happens without the lock, so workers loading different columns do not wait for each other.
func (w *World) get(cp ChunkPos) *Chunk {
	w.mu.Lock()
	c := w.chunks[cp]
	w.mu.Unlock()
	if c != nil {
		return c
	}
	var fresh *Chunk
	if w.gen != nil {
		fresh = w.gen(cp)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if c := w.chunks[cp]; c != nil { // another goroutine got there first
		return c
	}
	if fresh == nil {
		fresh = &Chunk{}
		if w.gen == nil {
			fillFlat(fresh)
		}
	}
	w.chunks[cp] = fresh
	return fresh
}

func InRange(y int32) bool { return y >= 0 && y < 256 }

func index(x, y, z int32) int { return int(y)<<8 | int(z&15)<<4 | int(x&15) }

// Block returns the block state (id<<4 | meta) at the position; outside the world's height it is air.
func (w *World) Block(x, y, z int32) uint16 {
	if !InRange(y) {
		return 0
	}
	c := w.get(ChunkPos{X: x >> 4, Z: z >> 4})
	w.mu.Lock()
	defer w.mu.Unlock()
	return c.Blocks[index(x, y, z)]
}

func (w *World) SetBlock(x, y, z int32, state uint16) bool {
	if !InRange(y) {
		return false
	}
	cp := ChunkPos{X: x >> 4, Z: z >> 4}
	c := w.get(cp)
	w.mu.Lock()
	defer w.mu.Unlock()
	c.Blocks[index(x, y, z)] = state
	if w.dirty == nil {
		w.dirty = map[ChunkPos]bool{}
	}
	w.dirty[cp] = true
	return true
}

// Put installs a column received from elsewhere (a server), replacing any there. It is not an
// edit: the column is not marked dirty.
func (w *World) Put(cp ChunkPos, c *Chunk) {
	w.mu.Lock()
	w.chunks[cp] = c
	w.mu.Unlock()
}

// Remove forgets a column; with a source that cannot reload it, it is empty afterwards.
func (w *World) Remove(cp ChunkPos) {
	w.mu.Lock()
	delete(w.chunks, cp)
	w.mu.Unlock()
}

// TakeDirty returns the columns changed since the last call, and forgets them.
func (w *World) TakeDirty() []ChunkPos {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]ChunkPos, 0, len(w.dirty))
	for cp := range w.dirty {
		out = append(out, cp)
	}
	w.dirty = nil
	return out
}

// Light returns the light at a position (see Chunk.Light); outside the world's height it is sky.
func (w *World) Light(x, y, z int32) uint8 {
	if !InRange(y) {
		return FullLight
	}
	c := w.get(ChunkPos{X: x >> 4, Z: z >> 4})
	w.mu.Lock()
	defer w.mu.Unlock()
	return c.LightAt(x, y, z)
}

// LightAt is the light inside this column at world coordinates x, y, z.
func (c *Chunk) LightAt(x, y, z int32) uint8 {
	if c.Light == nil {
		return FullLight
	}
	return c.Light[index(x, y, z)]
}

// WithChunk runs fn on the column under the world's lock, so the caller sees a consistent chunk.
func (w *World) WithChunk(cp ChunkPos, fn func(*Chunk)) {
	c := w.get(cp)
	w.mu.Lock()
	defer w.mu.Unlock()
	fn(c)
}

// Snapshot copies a column, for work that must not hold the lock.
func (w *World) Snapshot(cp ChunkPos) *Chunk {
	c := &Chunk{}
	w.WithChunk(cp, func(src *Chunk) { *c = *src })
	return c
}

// LoadedPositions lists the columns in memory.
func (w *World) LoadedPositions() []ChunkPos {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]ChunkPos, 0, len(w.chunks))
	for cp := range w.chunks {
		out = append(out, cp)
	}
	return out
}

// Loaded is how many columns are in memory.
func (w *World) Loaded() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.chunks)
}

// Evict drops columns that keep returns false for, except ones with edits not yet taken by
// TakeDirty: they would be lost. A dropped column loads again from its source when needed.
func (w *World) Evict(keep func(ChunkPos) bool) (dropped int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for cp := range w.chunks {
		if !keep(cp) && !w.dirty[cp] {
			delete(w.chunks, cp)
			dropped++
		}
	}
	return dropped
}

// Padded is a column with one block of its four neighbours around it, copied so a worker can mesh
// it without touching the world again. Index: (y*18 + z+1)*18 + x+1 for x, z in -1..16.
type Padded struct {
	Blocks [18 * 18 * 256]uint16
	Light  [18 * 18 * 256]uint8
	Biomes [256]uint8
	// Solid counts the non-air blocks of each 16-high section of the column itself.
	Solid [16]int32
}

// PadIndex is the position of block x, y, z (relative to the column, x and z in -1..16) in Padded.
func PadIndex(x, y, z int) int { return (y*18+z+1)*18 + x + 1 }

// FillPadded copies the column at cp and the edges of its neighbours into p. Columns not loaded
// yet are loaded first, outside the lock.
func (w *World) FillPadded(cp ChunkPos, p *Padded) {
	centre := w.get(cp)
	nb := [4]*Chunk{
		w.get(ChunkPos{X: cp.X - 1, Z: cp.Z}), w.get(ChunkPos{X: cp.X + 1, Z: cp.Z}),
		w.get(ChunkPos{X: cp.X, Z: cp.Z - 1}), w.get(ChunkPos{X: cp.X, Z: cp.Z + 1}),
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := range p.Solid {
		p.Solid[i] = 0
	}
	light := func(c *Chunk, i int) uint8 {
		if c.Light == nil {
			return FullLight
		}
		return c.Light[i]
	}
	for y := 0; y < 256; y++ {
		for z := 0; z < 16; z++ {
			src := y<<8 | z<<4
			dst := PadIndex(0, y, z)
			copy(p.Blocks[dst:dst+16], centre.Blocks[src:src+16])
			if centre.Light == nil {
				for x := 0; x < 16; x++ {
					p.Light[dst+x] = FullLight
				}
			} else {
				copy(p.Light[dst:dst+16], centre.Light[src:src+16])
			}
			for _, b := range centre.Blocks[src : src+16] {
				if b != 0 {
					p.Solid[y>>4]++
				}
			}
			// West and east neighbours: their column x=15 and x=0 beside this row.
			p.Blocks[PadIndex(-1, y, z)], p.Light[PadIndex(-1, y, z)] = nb[0].Blocks[src+15], light(nb[0], src+15)
			p.Blocks[PadIndex(16, y, z)], p.Light[PadIndex(16, y, z)] = nb[1].Blocks[src], light(nb[1], src)
		}
		for x := 0; x < 16; x++ {
			// North (z=-1) and south (z=16): the row of the neighbour beside this column.
			n, s := y<<8|15<<4|x, y<<8|x
			p.Blocks[PadIndex(x, y, -1)], p.Light[PadIndex(x, y, -1)] = nb[2].Blocks[n], light(nb[2], n)
			p.Blocks[PadIndex(x, y, 16)], p.Light[PadIndex(x, y, 16)] = nb[3].Blocks[s], light(nb[3], s)
		}
	}
	for i := 0; i < 256; i++ {
		p.Biomes[i] = 1
	}
	if centre.Biomes != nil {
		copy(p.Biomes[:], centre.Biomes)
	}
}
