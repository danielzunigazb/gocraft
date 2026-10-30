package worldgen

import (
	"math"
	"sync"
)

// The base terrain of one chunk, after ChunkProviderGenerate.provideChunk: the 3D density field,
// stone and water, the biome's surface layers and bedrock, then caves and ravines carved out.

const seaLevel = 63

var surfXMajor = false

// idx is the position of a block in a column: y, then z, then x, as world.Chunk lays it out.
func idx(x, y, z int) int { return y<<8 | z<<4 | x }

type baseChunk struct {
	blocks [65536]uint16
	biomes [256]uint8 // z*16 + x
}

// Generator makes 1.8.9 worlds from a seed. It is safe for concurrent use.
type Generator struct {
	seed int64

	minLimit, maxLimit, main *octaves
	surface                  *perlin
	depth                    *octaves
	parabolic                [25]float32
	swampNoise               *perlin
	tempNoise, grassNoise    *perlin // BiomeGenBase.temperatureNoise (Random 1234) and the plains flower noise (2345)

	stacks sync.Pool // *biomeStack: layers keep random state, so each goroutine takes its own

	mu    sync.Mutex
	cache map[[2]int32]*baseChunk
	order [][2]int32
}

// NewGenerator is ChunkProviderGenerate's constructor for a seed: the noise generators are built
// from one Random in the game's order, which fixes their permutation tables.
func NewGenerator(seed int64) *Generator {
	g := &Generator{seed: seed, cache: map[[2]int32]*baseChunk{}}
	r := NewRandom(seed)
	g.minLimit = newOctaves(r, 16)
	g.maxLimit = newOctaves(r, 16)
	g.main = newOctaves(r, 8)
	g.surface = newPerlin(r, 4)
	newOctaves(r, 10) // scale noise: built, never used in 1.8.9
	g.depth = newOctaves(r, 16)
	newOctaves(r, 8) // forest noise
	for j := -2; j <= 2; j++ {
		for k := -2; k <= 2; k++ {
			f := float32(10) / float32(math.Sqrt(float64(float32(j*j+k*k)+0.2)))
			g.parabolic[j+2+(k+2)*5] = f
		}
	}
	g.swampNoise = newPerlin(NewRandom(701), 1)
	g.tempNoise = newPerlin(NewRandom(1234), 1)
	g.grassNoise = newPerlin(NewRandom(2345), 1)
	g.stacks.New = func() any { return newBiomeStack(seed) }
	return g
}

func (g *Generator) Seed() int64 { return g.seed }

// BiomeAt is the biome of the block column at world coordinates x, z.
func (g *Generator) BiomeAt(x, z int) uint8 {
	st := g.stacks.Get().(*biomeStack)
	defer g.stacks.Put(st)
	return uint8(st.index.ints(x, z, 1, 1)[0])
}

var (
	coordScale = float64(float32(684.412))
	mainXZ     = float64(float32(684.412) / float32(80.0))
	mainY      = float64(float32(684.412) / float32(160.0))
)

// densities is generateHeightmap: the 5 x 33 x 5 field, x-major then z then y.
func (g *Generator) densities(cx, cz int32, biomes []int32) []float64 {
	x4, z4 := int(cx)*4, int(cz)*4
	depth := g.depth.generate2D(nil, x4, z4, 5, 5, 200, 200)
	main := g.main.generate(nil, x4, 0, z4, 5, 33, 5, mainXZ, mainY, mainXZ)
	lower := g.minLimit.generate(nil, x4, 0, z4, 5, 33, 5, coordScale, coordScale, coordScale)
	upper := g.maxLimit.generate(nil, x4, 0, z4, 5, 33, 5, coordScale, coordScale, coordScale)
	out := make([]float64, 5*5*33)
	l, i1 := 0, 0
	for j1 := 0; j1 < 5; j1++ {
		for k1 := 0; k1 < 5; k1++ {
			var f, f1, f2 float32
			bc := biomeByID(int(biomes[j1+2+(k1+2)*10]))
			for l1 := -2; l1 <= 2; l1++ {
				for i2 := -2; i2 <= 2; i2++ {
					b := biomeByID(int(biomes[j1+l1+2+(k1+i2+2)*10]))
					f3 := b.MinH
					f4 := b.MaxH
					f5 := g.parabolic[l1+2+(i2+2)*5] / (f3 + 2)
					if b.MinH > bc.MinH {
						f5 /= 2
					}
					f += f4 * f5
					f1 += f3 * f5
					f2 += f5
				}
			}
			f /= f2
			f1 /= f2
			f = f*0.9 + 0.1
			f1 = (f1*4 - 1) / 8
			d12 := depth[i1] / 8000
			if d12 < 0 {
				d12 = -d12 * 0.3
			}
			d12 = d12*3 - 2
			if d12 < 0 {
				d12 /= 2
				if d12 < -1 {
					d12 = -1
				}
				d12 /= 1.4
				d12 /= 2
			} else {
				if d12 > 1 {
					d12 = 1
				}
				d12 /= 8
			}
			i1++
			d13 := float64(f1)
			d14 := float64(f)
			d13 += d12 * 0.2
			d13 = d13 * 8.5 / 8
			d5 := 8.5 + d13*4
			for j2 := 0; j2 < 33; j2++ {
				d6 := (float64(j2) - d5) * 12 * 128 / 256 / d14
				if d6 < 0 {
					d6 *= 4
				}
				d7 := lower[l] / 512
				d8 := upper[l] / 512
				d9 := (main[l]/10 + 1) / 2
				var d10 float64
				switch {
				case d9 < 0:
					d10 = d7
				case d9 > 1:
					d10 = d8
				default:
					d10 = d7 + (d8-d7)*d9
				}
				d10 -= d6
				if j2 > 29 {
					d11 := float64(float32(j2-29) / 3)
					d10 = d10*(1-d11) + -10*d11
				}
				out[l] = d10
				l++
			}
		}
	}
	return out
}

// fillDensity turns the density field into stone and water, interpolating between its points.
func fillDensity(b *baseChunk, f []float64) {
	for i := 0; i < 4; i++ {
		j := i * 5
		k := (i + 1) * 5
		for l := 0; l < 4; l++ {
			i1 := (j + l) * 33
			j1 := (j + l + 1) * 33
			k1 := (k + l) * 33
			l1 := (k + l + 1) * 33
			for i2 := 0; i2 < 32; i2++ {
				const d0 = 0.125
				d1, d2, d3, d4 := f[i1+i2], f[j1+i2], f[k1+i2], f[l1+i2]
				d5 := (f[i1+i2+1] - d1) * d0
				d6 := (f[j1+i2+1] - d2) * d0
				d7 := (f[k1+i2+1] - d3) * d0
				d8 := (f[l1+i2+1] - d4) * d0
				for j2 := 0; j2 < 8; j2++ {
					const d9 = 0.25
					d10, d11 := d1, d2
					d12 := (d3 - d1) * d9
					d13 := (d4 - d2) * d9
					for k2 := 0; k2 < 4; k2++ {
						const d14 = 0.25
						d16 := (d11 - d10) * d14
						d15 := d10 - d16
						for l2 := 0; l2 < 4; l2++ {
							d15 += d16
							y := i2*8 + j2
							if d15 > 0 {
								b.blocks[idx(i*4+k2, y, l*4+l2)] = stStone
							} else if y < seaLevel {
								b.blocks[idx(i*4+k2, y, l*4+l2)] = stWater
							}
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

// surfaceBlocks is replaceBlocksForBiome: each column gets its biome's top and filler blocks,
// bedrock at the bottom, and ice or water where the sea level cuts the stone.
func (g *Generator) surfaceBlocks(b *baseChunk, cx, cz int32, biomes []int32, rnd *Random) {
	const d0 = 0.03125
	noise := g.surface.area(nil, float64(cx*16), float64(cz*16), 16, 16, d0*2, d0*2, 1)
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			bio := biomeByID(int(biomes[lx+lz*16]))
			ni := lx + lz*16
			if surfXMajor {
				ni = lz + lx*16
			}
			g.columnTerrain(b, bio, rnd, lx, lz, int(cx)*16+lz, int(cz)*16+lx, noise[ni])
		}
	}
}

// columnTerrain is BiomeGenBase.genTerrainBlocks. The game calls it with its two loop counters
// in the opposite order to the block's position (and passes them on as x and z), which only
// shows in the swamp's water patches; wx and wz here are what it passes.
func (g *Generator) columnTerrain(b *baseChunk, bio *Biome, rnd *Random, lx, lz, wx, wz int, noiseVal float64) {
	top, fill := bio.Top, bio.Fill
	switch {
	case bio.Class == clsHills:
		switch {
		case (noiseVal < -1 || noiseVal > 2) && bio.Hills == 2:
			top, fill = stGravel, stGravel
		case noiseVal > 1 && bio.Hills != 1:
			top, fill = stStone, stStone
		default:
			top, fill = stGrass, stDirt
		}
	case bio.Class == clsTaiga && (bio.TaigaType == 1 || bio.TaigaType == 2):
		top, fill = stGrass, stDirt
		if noiseVal > 1.75 {
			top = stCoarseDirt
		} else if noiseVal > -0.95 {
			top = stPodzol
		}
	case bio.Class == clsSavanna && bio.Mutated:
		top, fill = stGrass, stDirt
		if noiseVal > 1.75 {
			top, fill = stStone, stStone
		} else if noiseVal > -0.5 {
			top = stCoarseDirt
		}
	case bio.Class == clsSwamp:
		d0 := g.swampNoise.at(float64(wx)*0.25, float64(wz)*0.25)
		if d0 > 0 {
			for y := 255; y >= 0; y-- {
				if b.blocks[idx(lx, y, lz)] != 0 {
					if y == 62 && b.blocks[idx(lx, y, lz)] != stWater {
						b.blocks[idx(lx, y, lz)] = stWater
						if d0 < 0.12 {
							b.blocks[idx(lx, y+1, lz)] = stWaterlily
						}
					}
					break
				}
			}
		}
	case bio.Class == clsMesa:
		top, fill = bio.Top, bio.Fill
	}
	g.generateBiomeTerrain(b, bio, rnd, lx, lz, noiseVal, top, fill)
}

func (g *Generator) generateBiomeTerrain(b *baseChunk, bio *Biome, rnd *Random, lx, lz int, noiseVal float64, top, fill uint16) {
	curTop, curFill := top, fill
	k := int32(-1)
	l := int32(noiseVal/3 + 3 + rnd.Double()*0.25)
	for y := 255; y >= 0; y-- {
		if int32(y) <= rnd.IntN(5) {
			b.blocks[idx(lx, y, lz)] = stBedrock
			continue
		}
		st := b.blocks[idx(lx, y, lz)]
		switch {
		case st == stAir:
			k = -1
		case st == stStone:
			if k == -1 {
				if l <= 0 {
					curTop, curFill = stAir, stStone
				} else if y >= seaLevel-4 && y <= seaLevel+1 {
					curTop, curFill = top, fill
				}
				if y < seaLevel && curTop == stAir {
					if bio.Temp < 0.15 {
						curTop = stIce
					} else {
						curTop = stWater
					}
				}
				k = l
				switch {
				case y >= seaLevel-1:
					b.blocks[idx(lx, y, lz)] = curTop
				case int32(y) < seaLevel-7-l:
					curTop, curFill = stAir, stStone
					b.blocks[idx(lx, y, lz)] = stGravel
				default:
					b.blocks[idx(lx, y, lz)] = curFill
				}
			} else if k > 0 {
				k--
				b.blocks[idx(lx, y, lz)] = curFill
				if k == 0 && (curFill == stSand || curFill == stRedSand) {
					k = rnd.IntN(4) + max(0, int32(y)-63)
					if curFill == stRedSand {
						curFill = stRedSandst
					} else {
						curFill = stSandstone
					}
				}
			}
		}
	}
}

// base builds (or recalls) the base terrain of a chunk.
func (g *Generator) base(cx, cz int32) *baseChunk {
	key := [2]int32{cx, cz}
	g.mu.Lock()
	if b := g.cache[key]; b != nil {
		g.mu.Unlock()
		return b
	}
	g.mu.Unlock()
	b := g.makeBase(cx, cz)
	g.mu.Lock()
	defer g.mu.Unlock()
	if old := g.cache[key]; old != nil {
		return old
	}
	g.cache[key] = b
	g.order = append(g.order, key)
	if len(g.order) > 160 {
		for _, k := range g.order[:32] {
			delete(g.cache, k)
		}
		g.order = append([][2]int32(nil), g.order[32:]...)
	}
	return b
}

func (g *Generator) makeBase(cx, cz int32) *baseChunk {
	b := &baseChunk{}
	rnd := NewRandom(int64(cx)*341873128712 + int64(cz)*132897987541)
	st := g.stacks.Get().(*biomeStack)
	defer g.stacks.Put(st)
	gen := st.gen.ints(int(cx)*4-2, int(cz)*4-2, 10, 10)
	fillDensity(b, g.densities(cx, cz, gen))
	biomes := st.index.ints(int(cx)*16, int(cz)*16, 16, 16)
	g.surfaceBlocks(b, cx, cz, biomes, rnd)
	for i, v := range biomes {
		b.biomes[i] = uint8(v)
	}
	g.carveCaves(b, cx, cz)
	g.carveRavines(b, cx, cz)
	return b
}
