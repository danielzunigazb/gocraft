package worldgen

// The Nether, after ChunkProviderHell (class anw in the 1.8.9 client jar, read from its bytecode):
// a 3D density field, netherrack above lava at y=32, a surface band of soul sand and gravel near 64,
// bedrock above and below, caves, then the decoration: lava, fire, glowstone, mushrooms, quartz.
//
// What is exact: the density field, the cell interpolation, the surface rules and the cave carver,
// all with java.util.Random in the order the game draws from it. What is approximate: vanilla draws
// the decoration from the same Random the last generated chunk left behind, so it depends on the
// order chunks were made; here each chunk's decoration is seeded from the world seed and its
// position, as the Overworld's is. Nether fortresses are not generated. See nether_populate.go.

import (
	"math"
	"sync"

	"gocraft/world"
)

const (
	nhHeight = 128
	nhVolume = nhHeight * 256 // blocks in a Nether column; the rest of the 256 above is air
	// nhSea is the sea level the world provider reports (63). The lava lake level is half of it plus 1,
	// and the soul sand and gravel band sits around it plus 1.
	nhSea = 63

	nhAir        uint16 = 0
	nhBedrock    uint16 = 7 << 4
	nhNetherrack uint16 = 87 << 4
	nhSoulSand   uint16 = 88 << 4
	nhGlowstone  uint16 = 89 << 4
	nhLava       uint16 = 11 << 4
	nhGravel     uint16 = 13 << 4
	nhFire       uint16 = 51 << 4
	nhQuartzOre  uint16 = 153 << 4
	nhBrownMush  uint16 = 39 << 4
	nhRedMush    uint16 = 40 << 4
	nhDirt       uint16 = 3 << 4
	nhGrassBlock uint16 = 2 << 4
	nhHellBiome         = 8
)

type nhColumn [nhVolume]uint16

func nhIdx(x, y, z int) int { return y<<8 | z<<4 | x }

// Nether makes Nether columns from a world seed. It is safe for concurrent use.
type Nether struct {
	seed             int64
	gen1, gen2, gen3 *octaves // the density noises: 16, 16 and 8 octaves
	sand, ex         *octaves // soul sand and gravel, and the netherrack thickness: 4 octaves each

	mu    sync.Mutex
	cache map[world.ChunkPos]*nhColumn // terrain only, kept for the neighbours that decorate over it
	order []world.ChunkPos
}

// NewNether returns the Nether of a world.
func NewNether(seed int64) *Nether {
	r := NewRandom(seed)
	n := &Nether{seed: seed, cache: map[world.ChunkPos]*nhColumn{}}
	// The game builds seven octave generators from one Random, in this order; the last two (10 and 16
	// octaves) feed nothing, so they are not built.
	n.gen1 = newOctaves(r, 16)
	n.gen2 = newOctaves(r, 16)
	n.gen3 = newOctaves(r, 8)
	n.sand = newOctaves(r, 4)
	n.ex = newOctaves(r, 4)
	return n
}

// Func is Chunk as the plain function the world takes.
func (n *Nether) Func() func(world.ChunkPos) *world.Chunk { return n.Chunk }

// chunkRandom is the Random the game seeds for the terrain of a chunk.
func nhChunkRandom(cx, cz int32) *Random {
	return NewRandom(int64(cx)*341873128712 + int64(cz)*132897987541)
}

// density is initializeNoiseField: 5 x 17 x 5 samples for the 4 x 16 x 4 cells of a chunk.
func (n *Nether) density(xo, yo, zo, xs, ys, zs int) []float64 {
	const d0, d1 = 684.412, 2053.236
	pnr := n.gen3.generate(nil, xo, yo, zo, xs, ys, zs, d0/80, d1/60, d0/80)
	ar := n.gen1.generate(nil, xo, yo, zo, xs, ys, zs, d0, d1, d0)
	br := n.gen2.generate(nil, xo, yo, zo, xs, ys, zs, d0, d1, d0)
	out := make([]float64, xs*ys*zs)
	// The shape along y: the cosine that makes floor and ceiling, and the thick base and roof.
	prof := make([]float64, ys)
	for j := 0; j < ys; j++ {
		prof[j] = math.Cos(float64(j)*math.Pi*6/float64(ys)) * 2
		d2 := float64(j)
		if j > ys/2 {
			d2 = float64(ys - 1 - j)
		}
		if d2 < 4 {
			d2 = 4 - d2
			prof[j] -= d2 * d2 * d2 * 10
		}
	}
	i := 0
	for l := 0; l < xs; l++ {
		for i1 := 0; i1 < zs; i1++ {
			for k := 0; k < ys; k++ {
				d6 := ar[i] / 512
				d7 := br[i] / 512
				d8 := (pnr[i]/10 + 1) / 2
				var d4 float64
				switch {
				case d8 < 0:
					d4 = d6
				case d8 > 1:
					d4 = d7
				default:
					d4 = d6 + (d7-d6)*d8
				}
				d4 -= prof[k]
				if k > ys-4 {
					d9 := float64(float32(k-(ys-4)) / 3)
					d4 = d4*(1-d9) + -10*d9
				}
				// (the game also pulls the field down below a "d3" that is always 0: no effect)
				out[i] = d4
				i++
			}
		}
	}
	return out
}

// terrain is the first step: netherrack where the density is positive, lava below y=32.
func (n *Nether) terrain(cx, cz int32, b *nhColumn) {
	const cells = 4
	sea := nhSea/2 + 1 // 32
	field := n.density(int(cx)*cells, 0, int(cz)*cells, cells+1, 17, cells+1)
	at := func(x, z, y int) float64 { return field[(x*(cells+1)+z)*17+y] }
	for l := 0; l < cells; l++ {
		for i1 := 0; i1 < cells; i1++ {
			for j1 := 0; j1 < 16; j1++ {
				d1, d2 := at(l, i1, j1), at(l, i1+1, j1)
				d3, d4 := at(l+1, i1, j1), at(l+1, i1+1, j1)
				d5 := (at(l, i1, j1+1) - d1) * 0.125
				d6 := (at(l, i1+1, j1+1) - d2) * 0.125
				d7 := (at(l+1, i1, j1+1) - d3) * 0.125
				d8 := (at(l+1, i1+1, j1+1) - d4) * 0.125
				for k1 := 0; k1 < 8; k1++ {
					d10, d11 := d1, d2
					d12, d13 := (d3-d1)*0.25, (d4-d2)*0.25
					for l1 := 0; l1 < 4; l1++ {
						d15 := d10
						d16 := (d11 - d10) * 0.25
						for i2 := 0; i2 < 4; i2++ {
							st := nhAir
							if j1*8+k1 < sea {
								st = nhLava
							}
							if d15 > 0 {
								st = nhNetherrack
							}
							b[nhIdx(l1+l*4, k1+j1*8, i2+i1*4)] = st
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

// surface is replaceBlocksForBiome: the soul sand and gravel band, the thin crust of netherrack over
// lava, and the ragged bedrock floor and roof.
func (n *Nether) surface(cx, cz int32, b *nhColumn, r *Random) {
	sea := nhSea + 1 // 64
	const d0 = 0.03125
	sand := n.sand.generate(nil, int(cx)*16, int(cz)*16, 0, 16, 16, 1, d0, d0, 1)
	gravel := n.sand.generate(nil, int(cx)*16, 109, int(cz)*16, 16, 1, 16, d0, 1, d0)
	ex := n.ex.generate(nil, int(cx)*16, int(cz)*16, 0, 16, 16, 1, d0*2, d0*2, d0*2)
	for j := 0; j < 16; j++ { // j is z here and k is x, as in the game
		for k := 0; k < 16; k++ {
			flag := sand[j+k*16]+r.Double()*0.2 > 0
			flag1 := gravel[j+k*16]+r.Double()*0.2 > 0
			l := int(ex[j+k*16]/3 + 3 + r.Double()*0.25)
			i1 := -1
			top, under := nhNetherrack, nhNetherrack
			for j1 := 127; j1 >= 0; j1-- {
				if j1 < 127-int(r.IntN(5)) && j1 > int(r.IntN(5)) {
					cur := b[nhIdx(k, j1, j)]
					if cur == nhAir {
						i1 = -1
					} else if cur == nhNetherrack {
						if i1 == -1 {
							if l <= 0 {
								top, under = nhAir, nhNetherrack
							} else if j1 >= sea-4 && j1 <= sea+1 {
								top, under = nhNetherrack, nhNetherrack
								if flag1 {
									top, under = nhGravel, nhNetherrack
								}
								if flag {
									top, under = nhSoulSand, nhSoulSand
								}
							}
							if j1 < sea && top == nhAir {
								top = nhLava
							}
							i1 = l
							if j1 >= sea-1 {
								b[nhIdx(k, j1, j)] = top
							} else {
								b[nhIdx(k, j1, j)] = under
							}
						} else if i1 > 0 {
							i1--
							b[nhIdx(k, j1, j)] = under
						}
					}
				} else {
					b[nhIdx(k, j1, j)] = nhBedrock
				}
			}
		}
	}
}

// base is the terrain of one column: noise, surface and caves.
func (n *Nether) base(cx, cz int32) *nhColumn {
	cp := world.ChunkPos{X: cx, Z: cz}
	n.mu.Lock()
	if c := n.cache[cp]; c != nil {
		n.mu.Unlock()
		return c
	}
	n.mu.Unlock()
	c := new(nhColumn)
	r := nhChunkRandom(cx, cz)
	n.terrain(cx, cz, c)
	n.surface(cx, cz, c, r)
	n.carve(cx, cz, c)
	n.mu.Lock()
	if old := n.cache[cp]; old != nil {
		c = old
	} else {
		n.cache[cp] = c
		n.order = append(n.order, cp)
		if len(n.order) > 192 { // enough for the zones of the chunks being made, not for ever
			for _, drop := range n.order[:64] {
				delete(n.cache, drop)
			}
			n.order = append([]world.ChunkPos(nil), n.order[64:]...)
		}
	}
	n.mu.Unlock()
	return c
}

// Chunk is the finished column at cp: terrain, the decoration of the four chunks whose population
// reaches it, and block light (the Nether has no sky). It can be called for any coordinates from any
// goroutine.
func (n *Nether) Chunk(cp world.ChunkPos) *world.Chunk {
	z := &nhZone{ox: int(cp.X-1) * 16, oz: int(cp.Z-1) * 16}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			col := new(nhColumn)
			*col = *n.base(cp.X-1+int32(i), cp.Z-1+int32(j))
			z.col[i][j] = col
		}
	}
	for _, d := range [4][2]int32{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
		n.populate(z, cp.X+d[0], cp.Z+d[1])
	}
	c := &world.Chunk{Biomes: make([]uint8, 256)}
	for i := range c.Biomes {
		c.Biomes[i] = nhHellBiome
	}
	copy(c.Blocks[:nhVolume], z.col[1][1][:])
	c.Light = nhLight(z)
	return c
}
