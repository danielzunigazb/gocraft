package world

// Light propagation, after 1.8's World.checkLightFor: when a block changes, the light it carried
// is taken away and then spread again from what remains, so lamps, roofs and shafts light and
// darken the places around them. Light levels are 0..15, sky light in the high nibble of a
// column's Light array and block light in the low one.

// LightRules says what blocks do to light.
type LightRules struct {
	Opacity   [256]uint8 // how much a block dims light passing through it; 15 stops it
	Luminance [256]uint8 // how much light a block gives out
}

// SetLightRules turns on light updates for block changes made through SetBlockLit.
func (w *World) SetLightRules(r *LightRules) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rules = r
}

type lcell struct{ x, y, z int32 }

var lightDirs = [6][3]int32{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}

// ensureLight gives a column without light data some: lit by the sky where there is no solid block.
func (w *World) ensureLight(c *Chunk) {
	if c.Light != nil {
		return
	}
	c.Light = make([]uint8, 65536)
	for i, st := range c.Blocks {
		if w.opacity(st>>4) < 15 {
			c.Light[i] = FullLight
		}
	}
}

func (w *World) opacity(id uint16) uint8 {
	if id > 255 {
		return 15
	}
	return w.rules.Opacity[id]
}

// SetBlockLit changes a block and updates the light around it. It returns the columns that need
// meshing again: the one that changed, and any whose light or edge light changed.
func (w *World) SetBlockLit(x, y, z int32, state uint16) []ChunkPos {
	if !InRange(y) {
		return nil
	}
	cp := ChunkPos{X: x >> 4, Z: z >> 4}
	// Light can spread 15 blocks: make sure the surrounding columns are loaded before the lock.
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			w.get(ChunkPos{X: cp.X + dx, Z: cp.Z + dz})
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.chunks[cp]
	old := c.Blocks[index(x, y, z)]
	c.Blocks[index(x, y, z)] = state
	if w.dirty == nil {
		w.dirty = map[ChunkPos]bool{}
	}
	w.dirty[cp] = true
	touched := map[ChunkPos]bool{cp: true}
	if w.rules != nil && old != state {
		w.ensureLight(c)
		for _, sky := range [2]bool{true, false} {
			w.relight(lcell{x, y, z}, old>>4, state>>4, sky, touched)
		}
	}
	out := make([]ChunkPos, 0, len(touched))
	for t := range touched {
		out = append(out, t)
		w.dirty[t] = true // changed light is worth saving with the edit
	}
	return out
}

// relight updates one channel (sky or block light) after the block at p went from oldID to newID.
func (w *World) relight(p lcell, oldID, newID uint16, sky bool, touched map[ChunkPos]bool) {
	shift := uint(0)
	if sky {
		shift = 4
	}
	at := func(c lcell) (*Chunk, int, bool) {
		if !InRange(c.y) {
			return nil, 0, false
		}
		ch := w.chunks[ChunkPos{X: c.x >> 4, Z: c.z >> 4}]
		if ch == nil {
			return nil, 0, false
		}
		w.ensureLight(ch)
		return ch, index(c.x, c.y, c.z), true
	}
	get := func(c lcell) uint8 {
		ch, i, ok := at(c)
		if !ok {
			return 0
		}
		return ch.Light[i] >> shift & 15
	}
	set := func(c lcell, v uint8) {
		ch, i, ok := at(c)
		if !ok {
			return
		}
		ch.Light[i] = ch.Light[i]&^(15<<shift) | v<<shift
		touched[ChunkPos{X: c.x >> 4, Z: c.z >> 4}] = true
		// A cell on the edge of its column changes what the next column draws against it.
		if c.x&15 == 0 {
			touched[ChunkPos{X: c.x>>4 - 1, Z: c.z >> 4}] = true
		}
		if c.x&15 == 15 {
			touched[ChunkPos{X: c.x>>4 + 1, Z: c.z >> 4}] = true
		}
		if c.z&15 == 0 {
			touched[ChunkPos{X: c.x >> 4, Z: c.z>>4 - 1}] = true
		}
		if c.z&15 == 15 {
			touched[ChunkPos{X: c.x >> 4, Z: c.z>>4 + 1}] = true
		}
	}
	idOf := func(c lcell) uint16 {
		ch, i, ok := at(c)
		if !ok {
			return 255 // beyond what is loaded counts as solid
		}
		return ch.Blocks[i] >> 4
	}
	neighbour := func(c lcell, d int) lcell {
		return lcell{c.x + lightDirs[d][0], c.y + lightDirs[d][1], c.z + lightDirs[d][2]}
	}
	type entry struct {
		c lcell
		l uint8
	}

	// 1. Take away the light the old state carried.
	var refill []lcell
	if l := get(p); l > 0 {
		set(p, 0)
		queue := []entry{{p, l}}
		for len(queue) > 0 {
			e := queue[0]
			queue = queue[1:]
			for d := 0; d < 6; d++ {
				n := neighbour(e.c, d)
				nl := get(n)
				down := sky && d == 0 && e.l == 15 && nl == 15 // sky light falls straight down without fading
				if nl != 0 && (nl < e.l || down) {
					set(n, 0)
					queue = append(queue, entry{n, nl})
				} else if nl >= e.l && nl != 0 {
					refill = append(refill, n)
				}
			}
		}
	}

	// 2. Seed the new light: what the block gives out, what flows into a place that opened, and
	// what is left standing where the removal stopped.
	var seeds []lcell
	if !sky && w.rules.Luminance[newID&255] > 0 {
		set(p, w.rules.Luminance[newID&255])
		seeds = append(seeds, p)
	}
	if sky && w.opacity(newID) < 15 {
		above := lcell{p.x, p.y + 1, p.z}
		if p.y == 255 || (get(above) == 15 && w.opacity(newID) == 0) {
			for c := p; InRange(c.y) && w.opacity(idOf(c)) == 0 && get(c) != 15; c.y-- { // straight down the shaft
				set(c, 15)
				seeds = append(seeds, c)
			}
		}
	}
	for d := 0; d < 6; d++ {
		seeds = append(seeds, neighbour(p, d))
	}
	seeds = append(seeds, refill...)

	// 3. Spread: each cell gives its neighbours one less, or the same straight down for sky light.
	queue := seeds
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		l := get(c)
		if l == 0 {
			continue
		}
		for d := 0; d < 6; d++ {
			n := neighbour(c, d)
			op := w.opacity(idOf(n))
			if op >= 15 {
				continue
			}
			var nl uint8
			switch {
			case sky && d == 0 && l == 15 && op == 0:
				nl = 15
			case l > max(op, 1):
				nl = l - max(op, 1)
			}
			if nl > get(n) {
				set(n, nl)
				queue = append(queue, n)
			}
		}
	}
}
