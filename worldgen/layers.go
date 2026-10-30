package worldgen

// The biome map: a stack of GenLayers, each a transformation of the one below it, from a coarse
// land/sea pattern up to one biome per block. Every layer seeds its own random numbers from the
// world seed and the position, so any area can be computed on its own.

const (
	lcgMul = 6364136223846793005
	lcgAdd = 1442695040888963407
)

type layer interface {
	ints(x, z, w, h int) []int32
	initWorldSeed(seed int64)
}

type layerBase struct {
	baseSeed, worldSeed, chunkSeed int64
	parents                        []layer // layers whose seed follows this one's
}

func newLayerBase(seed int64) layerBase {
	b := seed
	b *= b*lcgMul + lcgAdd
	b += seed
	b *= b*lcgMul + lcgAdd
	b += seed
	b *= b*lcgMul + lcgAdd
	b += seed
	return layerBase{baseSeed: b}
}

func (l *layerBase) initWorldSeed(seed int64) {
	l.worldSeed = seed
	for _, p := range l.parents {
		p.initWorldSeed(seed)
	}
	l.worldSeed *= l.worldSeed*lcgMul + lcgAdd
	l.worldSeed += l.baseSeed
	l.worldSeed *= l.worldSeed*lcgMul + lcgAdd
	l.worldSeed += l.baseSeed
	l.worldSeed *= l.worldSeed*lcgMul + lcgAdd
	l.worldSeed += l.baseSeed
}

func (l *layerBase) initChunkSeed(x, z int64) {
	l.chunkSeed = l.worldSeed
	l.chunkSeed *= l.chunkSeed*lcgMul + lcgAdd
	l.chunkSeed += x
	l.chunkSeed *= l.chunkSeed*lcgMul + lcgAdd
	l.chunkSeed += z
	l.chunkSeed *= l.chunkSeed*lcgMul + lcgAdd
	l.chunkSeed += x
	l.chunkSeed *= l.chunkSeed*lcgMul + lcgAdd
	l.chunkSeed += z
}

func (l *layerBase) nextInt(n int32) int32 {
	i := int32((l.chunkSeed >> 24) % int64(n))
	if i < 0 {
		i += n
	}
	l.chunkSeed *= l.chunkSeed*lcgMul + lcgAdd
	l.chunkSeed += l.worldSeed
	return i
}

func (l *layerBase) selectRandom(v ...int32) int32 { return v[l.nextInt(int32(len(v)))] }

func (l *layerBase) selectModeOrRandom(a, b, c, d int32) int32 {
	switch {
	case b == c && c == d:
		return b
	case a == b && a == c:
		return a
	case a == b && a == d:
		return a
	case a == c && a == d:
		return a
	case a == b && c != d:
		return a
	case a == c && b != d:
		return a
	case a == d && b != c:
		return a
	case b == c && a != d:
		return b
	case b == d && a != c:
		return b
	case c == d && a != b:
		return c
	}
	return l.selectRandom(a, b, c, d)
}

// ---- island: the first land/sea pattern ----

type layerIsland struct{ layerBase }

func newIsland(seed int64) *layerIsland { return &layerIsland{newLayerBase(seed)} }

func (l *layerIsland) ints(x, z, w, h int) []int32 {
	out := make([]int32, w*h)
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			l.initChunkSeed(int64(x+i), int64(z+j))
			if l.nextInt(10) == 0 {
				out[i+j*w] = 1
			}
		}
	}
	if x > -w && x <= 0 && z > -h && z <= 0 {
		out[-x+-z*w] = 1
	}
	return out
}

// ---- zoom ----

type layerZoom struct {
	layerBase
	parent layer
	fuzzy  bool
}

func newZoom(seed int64, parent layer, fuzzy bool) *layerZoom {
	l := &layerZoom{layerBase: newLayerBase(seed), parent: parent, fuzzy: fuzzy}
	l.parents = []layer{parent}
	return l
}

func (l *layerZoom) pick(a, b, c, d int32) int32 {
	if l.fuzzy {
		return l.selectRandom(a, b, c, d)
	}
	return l.selectModeOrRandom(a, b, c, d)
}

func (l *layerZoom) ints(x, z, w, h int) []int32 {
	i := x >> 1
	j := z >> 1
	k := (w >> 1) + 2
	ll := (h >> 1) + 2
	p := l.parent.ints(i, j, k, ll)
	i1 := (k - 1) << 1
	j1 := (ll - 1) << 1
	tmp := make([]int32, i1*j1)
	for k1 := 0; k1 < ll-1; k1++ {
		l1 := (k1 << 1) * i1
		i2 := 0
		j2 := p[i2+0+(k1+0)*k]
		for k2 := p[i2+0+(k1+1)*k]; i2 < k-1; i2++ {
			l.initChunkSeed(int64((i2+i)<<1), int64((k1+j)<<1))
			l2 := p[i2+1+(k1+0)*k]
			i3 := p[i2+1+(k1+1)*k]
			tmp[l1] = j2
			tmp[l1+i1] = l.selectRandom(j2, k2)
			l1++
			tmp[l1] = l.selectRandom(j2, l2)
			tmp[l1+i1] = l.pick(j2, l2, k2, i3)
			l1++
			j2 = l2
			k2 = i3
		}
	}
	out := make([]int32, w*h)
	for j3 := 0; j3 < h; j3++ {
		copy(out[j3*w:(j3+1)*w], tmp[(j3+(z&1))*i1+(x&1):])
	}
	return out
}

func magnify(seed int64, l layer, n int) layer {
	for i := 0; i < n; i++ {
		l = newZoom(seed+int64(i), l, false)
	}
	return l
}

// ---- simple 3x3 layers ----

// stage3 is the shape of most layers: a function of one cell and its neighbours, with the cell's own seed.
type stage3 struct {
	layerBase
	parent layer
	fn     func(l *stage3, c cell3) int32
	seeded bool // the function draws random numbers, so the cell's seed is set first
}

type cell3 struct {
	n, e, s, w, c  int32 // up (z-1), right (x+1), down (z+1), left (x-1) and the centre
	nw, ne, sw, se int32 // the corners
	x, z           int
}

func newStage3(seed int64, parent layer, seeded bool, fn func(l *stage3, c cell3) int32) *stage3 {
	l := &stage3{layerBase: newLayerBase(seed), parent: parent, fn: fn, seeded: seeded}
	l.parents = []layer{parent}
	return l
}

func (l *stage3) ints(x, z, w, h int) []int32 {
	k := w + 2
	p := l.parent.ints(x-1, z-1, k, h+2)
	out := make([]int32, w*h)
	for i := 0; i < h; i++ {
		for j := 0; j < w; j++ {
			c := cell3{
				nw: p[j+0+(i+0)*k], n: p[j+1+(i+0)*k], ne: p[j+2+(i+0)*k],
				w: p[j+0+(i+1)*k], c: p[j+1+(i+1)*k], e: p[j+2+(i+1)*k],
				sw: p[j+0+(i+2)*k], s: p[j+1+(i+2)*k], se: p[j+2+(i+2)*k],
				x: j + x, z: i + z,
			}
			if l.seeded {
				l.initChunkSeed(int64(c.x), int64(c.z))
			}
			out[j+i*w] = l.fn(l, c)
		}
	}
	return out
}

func newAddIsland(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		k2 := c.c
		if k2 == 0 && (c.nw != 0 || c.ne != 0 || c.sw != 0 || c.se != 0) {
			l2 := int32(1)
			i3 := int32(1)
			if c.nw != 0 {
				if l.nextInt(l2) == 0 {
					i3 = c.nw
				}
				l2++
			}
			if c.ne != 0 {
				if l.nextInt(l2) == 0 {
					i3 = c.ne
				}
				l2++
			}
			if c.sw != 0 {
				if l.nextInt(l2) == 0 {
					i3 = c.sw
				}
				l2++
			}
			if c.se != 0 {
				if l.nextInt(l2) == 0 {
					i3 = c.se
				}
				l2++
			}
			if l.nextInt(3) == 0 {
				return i3
			}
			if i3 == 4 {
				return 4
			}
			return 0
		} else if k2 > 0 && (c.nw == 0 || c.ne == 0 || c.sw == 0 || c.se == 0) {
			if l.nextInt(5) == 0 {
				if k2 == 4 {
					return 4
				}
				return 0
			}
			return k2
		}
		return k2
	})
}

func newRemoveTooMuchOcean(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		if c.c == 0 && c.n == 0 && c.e == 0 && c.w == 0 && c.s == 0 && l.nextInt(2) == 0 {
			return 1
		}
		return c.c
	})
}

func newAddSnow(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		if c.c == 0 {
			return 0
		}
		r := l.nextInt(6)
		switch {
		case r == 0:
			return 4
		case r <= 1:
			return 3
		}
		return 1
	})
}

const (
	edgeCoolWarm = iota
	edgeHeatIce
	edgeSpecial
)

func newEdge(seed int64, parent layer, mode int) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		k := c.c
		switch mode {
		case edgeCoolWarm:
			if k == 1 {
				if c.n == 3 || c.e == 3 || c.w == 3 || c.s == 3 || c.n == 4 || c.e == 4 || c.w == 4 || c.s == 4 {
					k = 2
				}
			}
		case edgeHeatIce:
			if k == 4 {
				if c.n == 1 || c.e == 1 || c.w == 1 || c.s == 1 || c.n == 2 || c.e == 2 || c.w == 2 || c.s == 2 {
					k = 3
				}
			}
		case edgeSpecial:
			if k != 0 && l.nextInt(13) == 0 {
				k |= (1 + l.nextInt(15)) << 8 & 3840
			}
		}
		return k
	})
}

func newAddMushroomIsland(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		if c.c == 0 && c.nw == 0 && c.ne == 0 && c.sw == 0 && c.se == 0 && l.nextInt(100) == 0 {
			return 14
		}
		return c.c
	})
}

func newDeepOcean(seed int64, parent layer) layer {
	return newStage3(seed, parent, false, func(l *stage3, c cell3) int32 {
		n := 0
		if c.n == 0 {
			n++
		}
		if c.e == 0 {
			n++
		}
		if c.w == 0 {
			n++
		}
		if c.s == 0 {
			n++
		}
		if c.c == 0 && n > 3 {
			return 24
		}
		return c.c
	})
}

func newSmooth(seed int64, parent layer) layer {
	return newStage3(seed, parent, false, func(l *stage3, c cell3) int32 {
		k2 := c.c
		if c.w == c.e && c.n == c.s {
			l.initChunkSeed(int64(c.x), int64(c.z))
			if l.nextInt(2) == 0 {
				k2 = c.w
			} else {
				k2 = c.n
			}
		} else {
			if c.w == c.e {
				k2 = c.w
			}
			if c.n == c.s {
				k2 = c.n
			}
		}
		return k2
	})
}

func riverClass(v int32) int32 {
	if v >= 2 {
		return 2 + (v & 1)
	}
	return v
}

func newRiver(seed int64, parent layer) layer {
	return newStage3(seed, parent, false, func(l *stage3, c cell3) int32 {
		k2 := riverClass(c.c)
		if k2 == riverClass(c.w) && k2 == riverClass(c.n) && k2 == riverClass(c.e) && k2 == riverClass(c.s) {
			return -1
		}
		return 7
	})
}

func newRareBiome(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		if l.nextInt(57) == 0 {
			if c.c == 1 {
				return 129
			}
			return c.c
		}
		return c.c
	})
}

// ---- river init, biome choice ----

type layerRiverInit struct {
	layerBase
	parent layer
}

func newRiverInit(seed int64, parent layer) *layerRiverInit {
	l := &layerRiverInit{layerBase: newLayerBase(seed), parent: parent}
	l.parents = []layer{parent}
	return l
}

func (l *layerRiverInit) ints(x, z, w, h int) []int32 {
	p := l.parent.ints(x, z, w, h)
	out := make([]int32, w*h)
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			l.initChunkSeed(int64(i+x), int64(j+z))
			if p[i+j*w] > 0 {
				out[i+j*w] = l.nextInt(299999) + 2
			}
		}
	}
	return out
}

func isOceanic(id int32) bool { return id == 0 || id == 24 || id == 10 }

var (
	warmBiomes = []int32{2, 2, 2, 35, 35, 1}
	mediumBiom = []int32{4, 29, 3, 1, 27, 6}
	coolBiomes = []int32{4, 3, 5, 1}
	icyBiomes  = []int32{12, 12, 12, 30}
)

func newBiomeLayer(seed int64, parent layer) layer {
	return newStage3Single(seed, parent, func(l *layerBase, v int32, x, z int) int32 {
		l.initChunkSeed(int64(x), int64(z))
		special := (v & 3840) >> 8
		v &^= 3840
		switch {
		case isOceanic(v):
			return v
		case v == 14:
			return v
		case v == 1:
			if special > 0 {
				if l.nextInt(3) == 0 {
					return 39
				}
				return 38
			}
			return warmBiomes[l.nextInt(int32(len(warmBiomes)))]
		case v == 2:
			if special > 0 {
				return 21
			}
			return mediumBiom[l.nextInt(int32(len(mediumBiom)))]
		case v == 3:
			if special > 0 {
				return 32
			}
			return coolBiomes[l.nextInt(int32(len(coolBiomes)))]
		case v == 4:
			return icyBiomes[l.nextInt(int32(len(icyBiomes)))]
		}
		return 14
	})
}

// stage1 maps each cell on its own (no neighbours).
type stage1 struct {
	layerBase
	parent layer
	fn     func(l *layerBase, v int32, x, z int) int32
}

func newStage3Single(seed int64, parent layer, fn func(l *layerBase, v int32, x, z int) int32) layer {
	l := &stage1{layerBase: newLayerBase(seed), parent: parent, fn: fn}
	l.parents = []layer{parent}
	return l
}

func (l *stage1) ints(x, z, w, h int) []int32 {
	p := l.parent.ints(x, z, w, h)
	out := make([]int32, w*h)
	for i := 0; i < h; i++ {
		for j := 0; j < w; j++ {
			out[j+i*w] = l.fn(&l.layerBase, p[j+i*w], j+x, i+z)
		}
	}
	return out
}

// ---- biome edge ----

func biomesAlike(a, b int32) bool {
	if a == b {
		return true
	}
	if a != 38 && a != 39 {
		x, y := biomeByID(int(a)), biomeByID(int(b))
		return x != nil && y != nil && x.Class == y.Class
	}
	return b == 38 || b == 39
}

func canBeNeighbors(a, b int32) bool {
	if biomesAlike(a, b) {
		return true
	}
	x, y := biomeByID(int(a)), biomeByID(int(b))
	if x == nil || y == nil {
		return false
	}
	c1, c2 := x.tempCategory(), y.tempCategory()
	return c1 == c2 || c1 == catMedium || c2 == catMedium
}

func newBiomeEdge(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		k := c.c
		// replaceBiomeEdgeIfNecessary: hills edge by temperature
		if biomesAlike(k, 3) {
			if canBeNeighbors(c.n, 3) && canBeNeighbors(c.e, 3) && canBeNeighbors(c.w, 3) && canBeNeighbors(c.s, 3) {
				return k
			}
			return 20
		}
		edge := func(biome, to int32) (int32, bool) {
			if k != biome {
				return 0, false
			}
			if biomesAlike(c.n, biome) && biomesAlike(c.e, biome) && biomesAlike(c.w, biome) && biomesAlike(c.s, biome) {
				return k, true
			}
			return to, true
		}
		if v, ok := edge(38, 37); ok {
			return v
		}
		if v, ok := edge(39, 37); ok {
			return v
		}
		if v, ok := edge(32, 5); ok {
			return v
		}
		switch k {
		case 2: // desert next to ice becomes extreme hills+
			if c.n != 12 && c.e != 12 && c.w != 12 && c.s != 12 {
				return k
			}
			return 34
		case 6: // swamp
			if c.n != 2 && c.e != 2 && c.w != 2 && c.s != 2 && c.n != 30 && c.e != 30 && c.w != 30 && c.s != 30 && c.n != 12 && c.e != 12 && c.w != 12 && c.s != 12 {
				if c.n != 21 && c.e != 21 && c.w != 21 && c.s != 21 {
					return k
				}
				return 23
			}
			return 1
		}
		return k
	})
}

// ---- hills ----

type layerHills struct {
	layerBase
	parent, river layer
}

func newHills(seed int64, parent, river layer, seedRiver bool) *layerHills {
	l := &layerHills{layerBase: newLayerBase(seed), parent: parent, river: river}
	l.parents = []layer{parent}
	if seedRiver {
		l.parents = append(l.parents, river)
	}
	return l
}

func (l *layerHills) ints(x, z, w, h int) []int32 {
	k := w + 2
	p := l.parent.ints(x-1, z-1, k, h+2)
	r := l.river.ints(x-1, z-1, k, h+2)
	out := make([]int32, w*h)
	for i := 0; i < h; i++ {
		for j := 0; j < w; j++ {
			l.initChunkSeed(int64(j+x), int64(i+z))
			c := p[j+1+(i+1)*k]
			rv := r[j+1+(i+1)*k]
			flag := (rv-2)%29 == 0
			if c != 0 && rv >= 2 && (rv-2)%29 == 1 && c < 128 {
				if hasMutation(int(c)) {
					out[j+i*w] = c + 128
				} else {
					out[j+i*w] = c
				}
				continue
			}
			if l.nextInt(3) != 0 && !flag {
				out[j+i*w] = c
				continue
			}
			i1 := c
			switch {
			case c == 2:
				i1 = 17
			case c == 4:
				i1 = 18
			case c == 27:
				i1 = 28
			case c == 29:
				i1 = 1
			case c == 5:
				i1 = 19
			case c == 32:
				i1 = 33
			case c == 30:
				i1 = 31
			case c == 1:
				if l.nextInt(3) == 0 {
					i1 = 18
				} else {
					i1 = 4
				}
			case c == 12:
				i1 = 13
			case c == 21:
				i1 = 22
			case c == 0:
				i1 = 24
			case c == 3:
				i1 = 34
			case c == 35:
				i1 = 36
			case biomesAlike(c, 38):
				i1 = 37
			case c == 24 && l.nextInt(3) == 0:
				if l.nextInt(2) == 0 {
					i1 = 1
				} else {
					i1 = 4
				}
			}
			if flag && i1 != c {
				if hasMutation(int(i1)) {
					i1 += 128
				} else {
					i1 = c
				}
			}
			if i1 == c {
				out[j+i*w] = c
				continue
			}
			n := 0
			for _, nb := range [4]int32{p[j+1+(i+0)*k], p[j+2+(i+1)*k], p[j+0+(i+1)*k], p[j+1+(i+2)*k]} {
				if biomesAlike(nb, c) {
					n++
				}
			}
			if n >= 3 {
				out[j+i*w] = i1
			} else {
				out[j+i*w] = c
			}
		}
	}
	return out
}

// ---- shore ----

func isJungleLike(id int32) bool {
	if b := biomeByID(int(id)); b != nil && b.Class == clsJungle {
		return true
	}
	return id == 23 || id == 21 || id == 22 || id == 4 || id == 5 || isOceanic(id)
}

func isMesaBiome(id int32) bool {
	b := biomeByID(int(id))
	return b != nil && b.Class == clsMesa
}

func newShore(seed int64, parent layer) layer {
	return newStage3(seed, parent, true, func(l *stage3, c cell3) int32 {
		k := c.c
		b := biomeByID(int(k))
		nb := [4]int32{c.n, c.e, c.w, c.s}
		anyOcean := func() bool {
			return isOceanic(nb[0]) || isOceanic(nb[1]) || isOceanic(nb[2]) || isOceanic(nb[3])
		}
		beachOf := func(beach int32) int32 {
			if isOceanic(k) {
				return k
			}
			if !anyOcean() {
				return k
			}
			return beach
		}
		switch {
		case k == 14:
			if nb[0] != 0 && nb[1] != 0 && nb[2] != 0 && nb[3] != 0 {
				return k
			}
			return 15
		case b != nil && b.Class == clsJungle:
			if !isJungleLike(nb[0]) || !isJungleLike(nb[1]) || !isJungleLike(nb[2]) || !isJungleLike(nb[3]) {
				return 23
			}
			if anyOcean() {
				return 16
			}
			return k
		case k != 3 && k != 34 && k != 20:
			switch {
			case b != nil && b.Snow:
				return beachOf(26)
			case k != 37 && k != 38:
				if k != 0 && k != 24 && k != 7 && k != 6 {
					if !anyOcean() {
						return k
					}
					return 16
				}
				return k
			default:
				if !anyOcean() {
					if isMesaBiome(nb[0]) && isMesaBiome(nb[1]) && isMesaBiome(nb[2]) && isMesaBiome(nb[3]) {
						return k
					}
					return 2
				}
				return k
			}
		default:
			return beachOf(25)
		}
	})
}

// ---- river mix, voronoi ----

type layerRiverMix struct {
	layerBase
	biome, river layer
}

func newRiverMix(seed int64, biome, river layer) *layerRiverMix {
	l := &layerRiverMix{layerBase: newLayerBase(seed), biome: biome, river: river}
	l.parents = []layer{biome, river}
	return l
}

func (l *layerRiverMix) ints(x, z, w, h int) []int32 {
	a := l.biome.ints(x, z, w, h)
	b := l.river.ints(x, z, w, h)
	out := make([]int32, w*h)
	for i := range out {
		switch {
		case a[i] == 0 || a[i] == 24:
			out[i] = a[i]
		case b[i] == 7:
			switch {
			case a[i] == 12:
				out[i] = 11
			case a[i] != 14 && a[i] != 15:
				out[i] = b[i] & 255
			default:
				out[i] = a[i]
			}
		default:
			out[i] = a[i]
		}
	}
	return out
}

type layerVoronoi struct {
	layerBase
	parent layer
}

func newVoronoi(seed int64, parent layer) *layerVoronoi {
	l := &layerVoronoi{layerBase: newLayerBase(seed), parent: parent}
	l.parents = []layer{parent}
	return l
}

func (l *layerVoronoi) ints(x, z, w, h int) []int32 {
	x -= 2
	z -= 2
	i := x >> 2
	j := z >> 2
	k := (w >> 2) + 2
	ll := (h >> 2) + 2
	p := l.parent.ints(i, j, k, ll)
	i1 := (k - 1) << 2
	j1 := (ll - 1) << 2
	tmp := make([]int32, i1*j1)
	const d0 = 3.6
	rnd := func() float64 { return (float64(l.nextInt(1024))/1024 - 0.5) * d0 }
	for k1 := 0; k1 < ll-1; k1++ {
		l1 := 0
		i2 := p[l1+0+(k1+0)*k]
		for j2 := p[l1+0+(k1+1)*k]; l1 < k-1; l1++ {
			l.initChunkSeed(int64((l1+i)<<2), int64((k1+j)<<2))
			d1 := rnd()
			d2 := rnd()
			l.initChunkSeed(int64((l1+i+1)<<2), int64((k1+j)<<2))
			d3 := rnd() + 4
			d4 := rnd()
			l.initChunkSeed(int64((l1+i)<<2), int64((k1+j+1)<<2))
			d5 := rnd()
			d6 := rnd() + 4
			l.initChunkSeed(int64((l1+i+1)<<2), int64((k1+j+1)<<2))
			d7 := rnd() + 4
			d8 := rnd() + 4
			k2 := p[l1+1+(k1+0)*k] & 255
			l2 := p[l1+1+(k1+1)*k] & 255
			for i3 := 0; i3 < 4; i3++ {
				j3 := ((k1<<2)+i3)*i1 + (l1 << 2)
				for k3 := 0; k3 < 4; k3++ {
					fi, fk := float64(i3), float64(k3)
					d9 := (fi-d2)*(fi-d2) + (fk-d1)*(fk-d1)
					d10 := (fi-d4)*(fi-d4) + (fk-d3)*(fk-d3)
					d11 := (fi-d6)*(fi-d6) + (fk-d5)*(fk-d5)
					d12 := (fi-d8)*(fi-d8) + (fk-d7)*(fk-d7)
					switch {
					case d9 < d10 && d9 < d11 && d9 < d12:
						tmp[j3] = i2
					case d10 < d9 && d10 < d11 && d10 < d12:
						tmp[j3] = k2
					case d11 < d9 && d11 < d10 && d11 < d12:
						tmp[j3] = j2
					default:
						tmp[j3] = l2
					}
					j3++
				}
			}
			i2 = k2
			j2 = l2
		}
	}
	out := make([]int32, w*h)
	for i4 := 0; i4 < h; i4++ {
		copy(out[i4*w:(i4+1)*w], tmp[(i4+(z&3))*i1+(x&3):])
	}
	return out
}

// biomeStack is the whole stack for one seed. Layers keep their own random state, so a stack is
// used by one goroutine at a time.
type biomeStack struct {
	gen   layer // 1:4, for the terrain shape (genBiomes)
	index layer // 1:1, for each block (biomeIndexLayer)
}

// hillsSeedsRiver: whether the hills layer passes the world seed on to its river layer. The
// game's GenLayerHills does not override initWorldGenSeed; see doc.go.
var hillsSeedsRiver = false

func newBiomeStack(seed int64) *biomeStack {
	var l layer = newIsland(1)
	l = newZoom(2000, l, true)
	l = newAddIsland(1, l)
	l = newZoom(2001, l, false)
	l = newAddIsland(2, l)
	l = newAddIsland(50, l)
	l = newAddIsland(70, l)
	l = newRemoveTooMuchOcean(2, l)
	l = newAddSnow(2, l)
	l = newAddIsland(3, l)
	l = newEdge(2, l, edgeCoolWarm)
	l = newEdge(2, l, edgeHeatIce)
	l = newEdge(3, l, edgeSpecial)
	l = newZoom(2002, l, false)
	l = newZoom(2003, l, false)
	l = newAddIsland(4, l)
	l = newAddMushroomIsland(5, l)
	l = newDeepOcean(4, l)
	g4 := l
	riverInit := newRiverInit(100, g4)
	var bl layer = newBiomeLayer(200, g4)
	bl = magnify(1000, bl, 2)
	bl = newBiomeEdge(1000, bl)
	hills := newHills(1000, bl, magnify(1000, riverInit, 2), hillsSeedsRiver)
	var rv layer = magnify(1000, riverInit, 2)
	rv = magnify(1000, rv, 4)
	rv = newRiver(1, rv)
	rv = newSmooth(1000, rv)
	var b layer = newRareBiome(1001, hills)
	for k := 0; k < 4; k++ {
		b = newZoom(int64(1000+k), b, false)
		if k == 0 {
			b = newAddIsland(3, b)
		}
		if k == 1 {
			b = newShore(1000, b)
		}
	}
	b = newSmooth(1000, b)
	mix := newRiverMix(100, b, rv)
	vor := newVoronoi(10, mix)
	mix.initWorldSeed(seed)
	vor.initWorldSeed(seed)
	return &biomeStack{gen: mix, index: vor}
}
