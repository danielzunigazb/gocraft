package worldgen

// The End, after ChunkProviderEnd (class aob in the 1.8.9 client jar, read from its bytecode): one
// island of end stone around the origin, whose density falls off with the distance from it, and the
// obsidian pillars of BiomeEndDecorator on top.
//
// What is exact: the density field and the cell interpolation, with java.util.Random in the
// game's order. The end stone "surface" pass of the game never matches anything (it looks for stone
// in a column of end stone), so there is none here either. What is approximate: the pillars come
// from memory of WorldGenSpikes (their height 6 to 37 and radius 1 to 4, a bedrock cap), and the game
// places them with the world's own unseeded Random, so no seed can reproduce them: here they are
// seeded from the world seed and the chunk. The ender crystals on top are entities and are not made,
// and neither is the dragon.

import (
	"math"
	"sync"

	"gocraft/world"
)

const (
	enEndStone uint16 = 121 << 4
	enObsidian uint16 = 49 << 4
	enBiome           = 9
)

// End makes the End's columns from a world seed. It is safe for concurrent use.
type End struct {
	seed             int64
	gen1, gen2, gen3 *octaves

	mu    sync.Mutex
	cache map[world.ChunkPos]*nhColumn
	order []world.ChunkPos
}

// NewEnd returns the End of a world.
func NewEnd(seed int64) *End {
	r := NewRandom(seed)
	e := &End{seed: seed, cache: map[world.ChunkPos]*nhColumn{}}
	// The game builds five octave generators from one Random: 16, 16, 8, 10 and 16 octaves. The
	// last two feed nothing, so they are not built.
	e.gen1 = newOctaves(r, 16)
	e.gen2 = newOctaves(r, 16)
	e.gen3 = newOctaves(r, 8)
	return e
}

// Func is Chunk as the plain function the world takes.
func (e *End) Func() func(world.ChunkPos) *world.Chunk { return e.Chunk }

// density is initializeNoiseField: 3 x 33 x 3 samples for the 2 x 32 x 2 cells of a chunk.
func (e *End) density(xo, zo int) []float64 {
	const xs, ys, zs = 3, 33, 3
	d0, d1 := 684.412*2, 684.412
	c := e.gen3.generate(nil, xo, 0, zo, xs, ys, zs, d0/80, d1/160, d0/80)
	d := e.gen1.generate(nil, xo, 0, zo, xs, ys, zs, d0, d1, d0)
	ee := e.gen2.generate(nil, xo, 0, zo, xs, ys, zs, d0, d1, d0)
	out := make([]float64, xs*ys*zs)
	i := 0
	for i2 := 0; i2 < xs; i2++ {
		for j2 := 0; j2 < zs; j2++ {
			// How far this cell is from the middle of the island, in the game's float arithmetic.
			f15 := float32(i2+xo) / 1
			f16 := float32(j2+zo) / 1
			f17 := 100 - float32(math.Sqrt(float64(f15*f15+f16*f16)))*8
			f17 = min(f17, 80)
			f17 = max(f17, -100)
			for k := 0; k < ys; k++ {
				d5 := d[i] / 512
				d6 := ee[i] / 512
				d7 := (c[i]/10 + 1) / 2
				var d4 float64
				switch {
				case d7 < 0:
					d4 = d5
				case d7 > 1:
					d4 = d6
				default:
					d4 = d5 + (d6-d5)*d7
				}
				d4 -= 8
				d4 += float64(f17)
				if k > ys/2-2 { // the roof of the island: pulled down hard above the middle
					d8 := float64(float32(k-(ys/2-2)) / 64)
					d8 = math.Max(0, math.Min(1, d8))
					d4 = d4*(1-d8) + -3000*d8
				}
				if k < 8 { // and the underside tapers
					d9 := float64(float32(8-k) / float32(7))
					d4 = d4*(1-d9) + -30*d9
				}
				out[i] = d4
				i++
			}
		}
	}
	return out
}

func (e *End) terrain(cx, cz int32, b *nhColumn) {
	const cells = 2
	field := e.density(int(cx)*cells, int(cz)*cells)
	at := func(x, z, y int) float64 { return field[(x*(cells+1)+z)*33+y] }
	for i1 := 0; i1 < cells; i1++ {
		for j1 := 0; j1 < cells; j1++ {
			for k1 := 0; k1 < 32; k1++ {
				d1, d2 := at(i1, j1, k1), at(i1, j1+1, k1)
				d3, d4 := at(i1+1, j1, k1), at(i1+1, j1+1, k1)
				d5 := (at(i1, j1, k1+1) - d1) * 0.25
				d6 := (at(i1, j1+1, k1+1) - d2) * 0.25
				d7 := (at(i1+1, j1, k1+1) - d3) * 0.25
				d8 := (at(i1+1, j1+1, k1+1) - d4) * 0.25
				for l1 := 0; l1 < 4; l1++ {
					d10, d11 := d1, d2
					d12, d13 := (d3-d1)*0.125, (d4-d2)*0.125
					for i2 := 0; i2 < 8; i2++ {
						d15 := d10
						d16 := (d11 - d10) * 0.125
						for j2 := 0; j2 < 8; j2++ {
							st := nhAir
							if d15 > 0 {
								st = enEndStone
							}
							b[nhIdx(i2+i1*8, l1+k1*4, j2+j1*8)] = st
							d15 += d16
						}
						d10 += d12
						d11 += d13
					}
					d1 += d5
					d2 += d6
					d3 += d7
					d4 += d8
				}
			}
		}
	}
}

func (e *End) base(cx, cz int32) *nhColumn {
	cp := world.ChunkPos{X: cx, Z: cz}
	e.mu.Lock()
	if c := e.cache[cp]; c != nil {
		e.mu.Unlock()
		return c
	}
	e.mu.Unlock()
	c := new(nhColumn)
	e.terrain(cx, cz, c)
	e.mu.Lock()
	if old := e.cache[cp]; old != nil {
		c = old
	} else {
		e.cache[cp] = c
		e.order = append(e.order, cp)
		if len(e.order) > 192 {
			for _, drop := range e.order[:64] {
				delete(e.cache, drop)
			}
			e.order = append([]world.ChunkPos(nil), e.order[64:]...)
		}
	}
	e.mu.Unlock()
	return c
}

// popRandom is the Random a chunk's pillars draw from (see the package note on why it is seeded).
func (e *End) popRandom(px, pz int32) *Random {
	r := NewRandom(e.seed)
	k := r.Long()/2*2 + 1
	l := r.Long()/2*2 + 1
	r.SetSeed((int64(px)*k + int64(pz)*l) ^ e.seed)
	return r
}

// populate is BiomeEndDecorator.genDecorations: one chance in five of an obsidian pillar.
func (e *End) populate(z *nhZone, px, pz int32) {
	r := e.popRandom(px, pz)
	if r.IntN(5) != 0 {
		return
	}
	x := int(px)*16 + int(r.IntN(16)) + 8
	zz := int(pz)*16 + int(r.IntN(16)) + 8
	// getTopSolidOrLiquidBlock: the first free block above the highest solid one.
	y := nhHeight
	for y > 0 && z.get(x, y-1, zz) == 0 {
		y--
	}
	e.spike(z, r, x, y, zz)
}

// spike is WorldGenSpikes with end stone as its base: a pillar of obsidian capped with bedrock.
func (e *End) spike(z *nhZone, r *Random, x, y, zz int) {
	if z.get(x, y, zz) != 0 || z.get(x, y-1, zz) != enEndStone {
		return
	}
	i := int(r.IntN(32)) + 6
	j := int(r.IntN(4)) + 1
	for k := x - j; k <= x+j; k++ {
		for l := zz - j; l <= zz+j; l++ {
			i1, j1 := k-x, l-zz
			if i1*i1+j1*j1 <= j*j+1 && z.get(k, y-1, l) != enEndStone {
				return
			}
		}
	}
	for l1 := y; l1 < y+i && l1 < nhHeight; l1++ {
		for i2 := x - j; i2 <= x+j; i2++ {
			for j2 := zz - j; j2 <= zz+j; j2++ {
				k2, k1 := i2-x, j2-zz
				if k2*k2+k1*k1 <= j*j+1 {
					z.set(i2, l1, j2, enObsidian)
				}
			}
		}
	}
	r.Float() // the crystal's facing; the crystal itself is an entity
	z.set(x, y+i, zz, nhBedrock)
}

// Chunk is the finished column at cp.
func (e *End) Chunk(cp world.ChunkPos) *world.Chunk {
	z := &nhZone{ox: int(cp.X-1) * 16, oz: int(cp.Z-1) * 16}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			col := new(nhColumn)
			*col = *e.base(cp.X-1+int32(i), cp.Z-1+int32(j))
			z.col[i][j] = col
		}
	}
	for _, d := range [4][2]int32{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
		e.populate(z, cp.X+d[0], cp.Z+d[1])
	}
	c := &world.Chunk{Biomes: make([]uint8, 256), Light: make([]uint8, 65536)}
	for i := range c.Biomes {
		c.Biomes[i] = enBiome
	}
	copy(c.Blocks[:nhVolume], z.col[1][1][:])
	return c
}
