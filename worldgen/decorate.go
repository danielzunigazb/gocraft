package worldgen

import "math"

// populate: ChunkProviderGenerate.populate and BiomeDecorator.decorate for one chunk, run on a zone.

type popCtx struct {
	g   *Generator
	z   *zone
	rnd *Random
	px  int // block position of the chunk being populated
	pz  int
	bio *Biome
}

func (g *Generator) populate(z *zone, pcx, pcz int32) {
	px, pz := int(pcx)*16, int(pcz)*16
	// The biome that decorates is the one at the far corner: (+16, +16) from the chunk's own corner.
	bio := z.biome(px+16, pz+16)
	rnd := NewRandom(g.seed)
	k := rnd.Long()/2*2 + 1
	l := rnd.Long()/2*2 + 1
	rnd.SetSeed(int64(pcx)*k + int64(pcz)*l ^ g.seed)
	c := &popCtx{g: g, z: z, rnd: rnd, px: px, pz: pz, bio: bio}

	if bio.ID != 2 && bio.ID != 17 && rnd.IntN(4) == 0 { // water lakes
		x := px + int(rnd.IntN(16)) + 8
		y := int(rnd.IntN(256))
		zz := pz + int(rnd.IntN(16)) + 8
		c.lake(x, y, zz, stWater)
	}
	if rnd.IntN(8) == 0 { // lava lakes
		x := px + int(rnd.IntN(16)) + 8
		y := int(rnd.IntN(rnd.IntN(248) + 8))
		zz := pz + int(rnd.IntN(16)) + 8
		if y < seaLevel || rnd.IntN(10) == 0 {
			c.lake(x, y, zz, stLava)
		}
	}
	for i := 0; i < dungeonAttempts; i++ { // dungeons
		x := px + int(rnd.IntN(16)) + 8
		y := int(rnd.IntN(256))
		zz := pz + int(rnd.IntN(16)) + 8
		c.dungeon(x, y, zz)
	}
	c.decorate()
	c.freezeAndSnow()
}

// dungeonAttempts is how many dungeon sites a population tries. The saved worlds this was checked
// against hold one spawner in 150 chunks and their ore veins line up with no attempts drawn, so
// the default is none; see doc.go.
var dungeonAttempts = 0

// beforeOres is a test hook.
var beforeOres func(c *popCtx)

// ---- decorate ----

func (c *popCtx) pos(rx, ry, rz int) (int, int, int) { return c.px + rx, ry, c.pz + rz }

func (c *popCtx) decorate() {
	r := c.rnd
	b := c.bio
	c.biomeExtrasBefore()
	if beforeOres != nil {
		beforeOres(c)
	}
	c.ores()
	d := b.Decor
	for i := 0; i < d.Sand2; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		c.sandDisk(x, c.z.topSolidOrLiquid(x, zz), zz, stSand, 7)
	}
	for i := 0; i < d.Clay; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		c.clayDisk(x, c.z.topSolidOrLiquid(x, zz), zz)
	}
	for i := 0; i < d.Sand; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		c.sandDisk(x, c.z.topSolidOrLiquid(x, zz), zz, stGravel, 6)
	}
	trees := d.Trees
	if r.IntN(10) == 0 {
		trees++
	}
	for i := 0; i < trees; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		c.tree(x, c.z.height(x, zz), zz)
	}
	for i := 0; i < d.BigMushrooms; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		c.bigMushroom(x, c.z.height(x, zz), zz)
	}
	for i := 0; i < d.Flowers; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		h := c.z.height(x, zz) + 32
		if h > 0 {
			y := int(r.IntN(int32(h)))
			c.flowerPatch(x, y, zz, c.pickFlower(x, y, zz))
		}
	}
	for i := 0; i < d.Grass; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		h := c.z.height(x, zz) * 2
		if h > 0 {
			y := int(r.IntN(int32(h)))
			c.tallGrass(x, y, zz, c.grassKind())
		}
	}
	for i := 0; i < d.DeadBush; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		h := c.z.height(x, zz) * 2
		if h > 0 {
			c.deadBush(x, int(r.IntN(int32(h))), zz)
		}
	}
	for i := 0; i < d.Lilies; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		h := c.z.height(x, zz) * 2
		if h > 0 {
			y := int(r.IntN(int32(h)))
			for y > 0 && c.z.isAir(x, y-1, zz) {
				y--
			}
			c.waterlily(x, y, zz)
		}
	}
	for i := 0; i < d.Mushrooms; i++ {
		if r.IntN(4) == 0 {
			x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
			c.flowerPatchOf(x, c.z.height(x, zz), zz, stBrownMush)
		}
		if r.IntN(8) == 0 {
			x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
			if h := c.z.height(x, zz) * 2; h > 0 {
				c.flowerPatchOf(x, int(r.IntN(int32(h))), zz, stRedMush)
			}
		}
	}
	if r.IntN(4) == 0 {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		if h := c.z.height(x, zz) * 2; h > 0 {
			c.flowerPatchOf(x, int(r.IntN(int32(h))), zz, stBrownMush)
		}
	}
	if r.IntN(8) == 0 {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		if h := c.z.height(x, zz) * 2; h > 0 {
			c.flowerPatchOf(x, int(r.IntN(int32(h))), zz, stRedMush)
		}
	}
	for i := 0; i < d.Reeds; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		if h := c.z.height(x, zz) * 2; h > 0 {
			c.reeds(x, int(r.IntN(int32(h))), zz)
		}
	}
	for i := 0; i < 10; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		if h := c.z.height(x, zz) * 2; h > 0 {
			c.reeds(x, int(r.IntN(int32(h))), zz)
		}
	}
	if r.IntN(32) == 0 {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		if h := c.z.height(x, zz) * 2; h > 0 {
			c.pumpkins(x, int(r.IntN(int32(h))), zz)
		}
	}
	for i := 0; i < d.Cacti; i++ {
		x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
		if h := c.z.height(x, zz) * 2; h > 0 {
			c.cactus(x, int(r.IntN(int32(h))), zz)
		}
	}
	for i := 0; i < 50; i++ { // water springs
		x := c.px + int(r.IntN(16)) + 8
		y := int(r.IntN(r.IntN(248) + 8))
		zz := c.pz + int(r.IntN(16)) + 8
		c.spring(x, y, zz, stFlowWater)
	}
	for i := 0; i < 20; i++ { // lava springs
		x := c.px + int(r.IntN(16)) + 8
		y := int(r.IntN(r.IntN(r.IntN(240)+8) + 8))
		zz := c.pz + int(r.IntN(16)) + 8
		c.spring(x, y, zz, stFlowLava)
	}
	c.biomeExtrasAfter()
}

// ---- ores ----

// ChunkProviderSettings defaults: how many veins of dirt and gravel a chunk gets, and the order of
// the stone variants.
var (
	oreDirt         = 10
	oreGravel       = 8
	oreGraniteFirst = false
)

func (c *popCtx) ores() {
	r := c.rnd
	one := func(count int, size int, st uint16, lo, hi int) {
		if hi < lo {
			lo, hi = hi, lo
		} else if hi == lo {
			if lo < 255 {
				hi++
			} else {
				lo--
			}
		}
		for i := 0; i < count; i++ {
			x := c.px + int(r.IntN(16))
			y := int(r.IntN(int32(hi-lo))) + lo
			zz := c.pz + int(r.IntN(16))
			c.minable(x, y, zz, size, st)
		}
	}
	one(oreDirt, 33, stDirt, 0, 256)
	one(oreGravel, 33, stGravel, 0, 256)
	if oreGraniteFirst {
		one(10, 33, stGranite, 0, 80)
		one(10, 33, stDiorite, 0, 80)
	} else {
		one(10, 33, stDiorite, 0, 80)
		one(10, 33, stGranite, 0, 80)
	}
	one(10, 33, stAndesite, 0, 80)
	one(20, 17, stCoalOre, 0, 128)
	one(20, 9, stIronOre, 0, 64)
	one(2, 9, stGoldOre, 0, 32)
	one(8, 8, stRedstoneOr, 0, 16)
	one(1, 8, stDiamondOre, 0, 16)
	for i := 0; i < 1; i++ { // lapis: centred on y 16, spread 16
		x := c.px + int(r.IntN(16))
		y := int(r.IntN(16)) + int(r.IntN(16)) + 16 - 16
		zz := c.pz + int(r.IntN(16))
		c.minable(x, y, zz, 7, stLapisOre)
	}
}

// minable is WorldGenMinable: a blob of ore strung along a short line, replacing stone.
func (c *popCtx) minable(px, py, pz, n int, st uint16) {
	r := c.rnd
	f := r.Float() * piF
	fn := float32(n)
	d0 := float64(float32(px+8) + mhSin(f)*fn/8)
	d1 := float64(float32(px+8) - mhSin(f)*fn/8)
	d2 := float64(float32(pz+8) + mhCos(f)*fn/8)
	d3 := float64(float32(pz+8) - mhCos(f)*fn/8)
	d4 := float64(py + int(r.IntN(3)) - 2)
	d5 := float64(py + int(r.IntN(3)) - 2)
	for i := 0; i < n; i++ {
		f1 := float32(i) / fn
		d6 := d0 + (d1-d0)*float64(f1)
		d7 := d4 + (d5-d4)*float64(f1)
		d8 := d2 + (d3-d2)*float64(f1)
		d9 := r.Double() * float64(n) / 16
		d10 := float64(mhSin(piF*f1)+1)*d9 + 1
		d11 := d10
		j := int(floorD(d6 - d10/2))
		k := int(floorD(d7 - d11/2))
		l := int(floorD(d8 - d10/2))
		i1 := int(floorD(d6 + d10/2))
		j1 := int(floorD(d7 + d11/2))
		k1 := int(floorD(d8 + d10/2))
		// Only the middle chunk of the zone is kept: a blob that misses it changes nothing we use.
		if i1 < c.z.ox+16 || j > c.z.ox+31 || k1 < c.z.oz+16 || l > c.z.oz+31 {
			continue
		}
		for l1 := j; l1 <= i1; l1++ {
			d12 := (float64(l1) + 0.5 - d6) / (d10 / 2)
			if d12*d12 < 1 {
				for i2 := k; i2 <= j1; i2++ {
					d13 := (float64(i2) + 0.5 - d7) / (d11 / 2)
					if d12*d12+d13*d13 < 1 {
						for j2 := l; j2 <= k1; j2++ {
							d14 := (float64(j2) + 0.5 - d8) / (d10 / 2)
							if d12*d12+d13*d13+d14*d14 < 1 && c.z.id(l1, i2, j2) == 1 {
								c.z.set(l1, i2, j2, st)
							}
						}
					}
				}
			}
		}
	}
}

// ---- sand, clay and gravel disks ----

func (c *popCtx) sandDisk(x, y, z int, st uint16, radius int) {
	if !isWaterID(c.z.id(x, y, z)) {
		return
	}
	i := int(c.rnd.IntN(int32(radius-2))) + 2
	for k := x - i; k <= x+i; k++ {
		for l := z - i; l <= z+i; l++ {
			if (k-x)*(k-x)+(l-z)*(l-z) <= i*i {
				for yy := y - 2; yy <= y+2; yy++ {
					if id := c.z.id(k, yy, l); id == 3 || id == 2 {
						c.z.set(k, yy, l, st)
					}
				}
			}
		}
	}
}

func (c *popCtx) clayDisk(x, y, z int) {
	if !isWaterID(c.z.id(x, y, z)) {
		return
	}
	i := int(c.rnd.IntN(2)) + 2
	for k := x - i; k <= x+i; k++ {
		for l := z - i; l <= z+i; l++ {
			if (k-x)*(k-x)+(l-z)*(l-z) <= i*i {
				for yy := y - 1; yy <= y+1; yy++ {
					if id := c.z.id(k, yy, l); id == 3 || id == 82 {
						c.z.set(k, yy, l, stClay)
					}
				}
			}
		}
	}
}

// ---- lakes ----

func (c *popCtx) lake(x, y, z int, liquid uint16) {
	r := c.rnd
	x -= 8
	z -= 8
	for y > 5 && c.z.isAir(x, y, z) {
		y--
	}
	if y <= 4 {
		return
	}
	y -= 4
	var lk [2048]bool
	n := int(r.IntN(4)) + 4
	for j := 0; j < n; j++ {
		d0 := r.Double()*6 + 3
		d1 := r.Double()*4 + 2
		d2 := r.Double()*6 + 3
		d3 := r.Double()*(16-d0-2) + 1 + d0/2
		d4 := r.Double()*(8-d1-4) + 2 + d1/2
		d5 := r.Double()*(16-d2-2) + 1 + d2/2
		for l := 1; l < 15; l++ {
			for i1 := 1; i1 < 15; i1++ {
				for j1 := 1; j1 < 7; j1++ {
					d6 := (float64(l) - d3) / (d0 / 2)
					d7 := (float64(j1) - d4) / (d1 / 2)
					d8 := (float64(i1) - d5) / (d2 / 2)
					if d6*d6+d7*d7+d8*d8 < 1 {
						lk[(l*16+i1)*8+j1] = true
					}
				}
			}
		}
	}
	edge := func(a, b, k int) bool {
		return a < 15 && lk[((a+1)*16+b)*8+k] || a > 0 && lk[((a-1)*16+b)*8+k] ||
			b < 15 && lk[(a*16+b+1)*8+k] || b > 0 && lk[(a*16+b-1)*8+k] ||
			k < 7 && lk[(a*16+b)*8+k+1] || k > 0 && lk[(a*16+b)*8+k-1]
	}
	for a := 0; a < 16; a++ {
		for b := 0; b < 16; b++ {
			for k := 0; k < 8; k++ {
				if !lk[(a*16+b)*8+k] && edge(a, b, k) {
					id := c.z.id(x+a, y+k, z+b)
					if k >= 4 && isLiquidID(id) {
						return
					}
					if k < 4 && !solidMat(id) && c.z.get(x+a, y+k, z+b) != liquid {
						return
					}
				}
			}
		}
	}
	for a := 0; a < 16; a++ {
		for b := 0; b < 16; b++ {
			for k := 0; k < 8; k++ {
				if lk[(a*16+b)*8+k] {
					if k >= 4 {
						c.z.set(x+a, y+k, z+b, stAir)
					} else {
						c.z.set(x+a, y+k, z+b, liquid)
					}
				}
			}
		}
	}
	for a := 0; a < 16; a++ {
		for b := 0; b < 16; b++ {
			for k := 4; k < 8; k++ {
				if lk[(a*16+b)*8+k] {
					if c.z.id(x+a, y+k-1, z+b) == 3 && c.z.skyLight(x+a, y+k, z+b) > 0 {
						if c.z.biome(x+a, z+b).Top == stMycelium {
							c.z.set(x+a, y+k-1, z+b, stMycelium)
						} else {
							c.z.set(x+a, y+k-1, z+b, stGrass)
						}
					}
				}
			}
		}
	}
	if liquid == stLava {
		for a := 0; a < 16; a++ {
			for b := 0; b < 16; b++ {
				for k := 0; k < 8; k++ {
					if !lk[(a*16+b)*8+k] && edge(a, b, k) && (k < 4 || r.IntN(2) != 0) && solidMat(c.z.id(x+a, y+k, z+b)) {
						c.z.set(x+a, y+k, z+b, stStone)
					}
				}
			}
		}
	}
	if liquid == stWater {
		for a := 0; a < 16; a++ {
			for b := 0; b < 16; b++ {
				if c.canFreeze(x+a, y+4, z+b) {
					c.z.set(x+a, y+4, z+b, stIce)
				}
			}
		}
	}
}

// canFreeze is canBlockFreezeWater: a still-water block in a biome cold enough.
func (c *popCtx) canFreeze(x, y, z int) bool {
	if c.g.temperatureAt(c.z.biome(x, z), x, y, z) > 0.15 {
		return false
	}
	return y >= 0 && y < 256 && c.z.get(x, y, z) == stWater
}

// ---- dungeons ----

func (c *popCtx) dungeon(px, py, pz int) {
	r := c.rnd
	j := int(r.IntN(2)) + 2
	k := -j - 1
	l := j + 1
	k1 := int(r.IntN(2)) + 2
	l1 := -k1 - 1
	i2 := k1 + 1
	j2 := 0
	for a := k; a <= l; a++ {
		for b := -1; b <= 4; b++ {
			for d := l1; d <= i2; d++ {
				solid := solidMat(c.z.id(px+a, py+b, pz+d))
				if b == -1 && !solid || b == 4 && !solid {
					return
				}
				if (a == k || a == l || d == l1 || d == i2) && b == 0 && c.z.isAir(px+a, py+b, pz+d) && c.z.isAir(px+a, py+b+1, pz+d) {
					j2++
				}
			}
		}
	}
	if j2 < 1 || j2 > 5 {
		return
	}
	for a := k; a <= l; a++ {
		for b := 3; b >= -1; b-- {
			for d := l1; d <= i2; d++ {
				x, y, z := px+a, py+b, pz+d
				if a != k && b != -1 && d != l1 && a != l && b != 4 && d != i2 {
					if c.z.id(x, y, z) != 54 {
						c.z.set(x, y, z, stAir)
					}
				} else if y >= 0 && !solidMat(c.z.id(x, y-1, z)) {
					c.z.set(x, y, z, stAir)
				} else if solidMat(c.z.id(x, y, z)) && c.z.id(x, y, z) != 54 {
					if b == -1 && r.IntN(4) != 0 {
						c.z.set(x, y, z, stMossy)
					} else {
						c.z.set(x, y, z, stCobble)
					}
				}
			}
		}
	}
	for a := 0; a < 2; a++ {
		for b := 0; b < 3; b++ {
			x := px + int(r.IntN(int32(j*2+1))) - j
			y := py
			z := pz + int(r.IntN(int32(k1*2+1))) - k1
			if c.z.isAir(x, y, z) {
				n := 0
				for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					if solidMat(c.z.id(x+d[0], y, z+d[1])) {
						n++
					}
				}
				if n == 1 {
					c.z.set(x, y, z, stChest)
					c.chestLoot()
					goto spawner
				}
			}
		}
	}
spawner:
	c.z.set(px, py, pz, stSpawner)
	r.IntN(4) // which mob: the spawner's entity is not stored in the column
}

type lootEntry struct {
	weight, min, max int
	stack            int // max stack size of the item
}

// The 1.8.9 dungeon chest: saddle, iron, bread, wheat, gunpowder, string, bucket, golden apple,
// redstone, two records, name tag and the three horse armours (item stack sizes decide the draws).
var dungeonLoot = []lootEntry{
	{10, 1, 1, 1}, {10, 1, 4, 64}, {10, 1, 1, 64}, {10, 1, 4, 64}, {10, 1, 4, 64}, {10, 1, 4, 64},
	{10, 1, 1, 16}, {1, 1, 1, 64}, {10, 1, 4, 64}, {10, 1, 1, 1}, {10, 1, 1, 1}, {10, 1, 1, 64},
	{2, 1, 1, 1}, {5, 1, 1, 1}, {1, 1, 1, 1},
}

// chestLoot spends the random numbers that filling the chest takes (8 stacks, 27 slots).
func (c *popCtx) chestLoot() {
	total := 0
	for _, e := range dungeonLoot {
		total += e.weight
	}
	r := c.rnd
	for i := 0; i < 8; i++ {
		w := int(r.IntN(int32(total)))
		var e lootEntry
		for _, e = range dungeonLoot {
			if w -= e.weight; w < 0 {
				break
			}
		}
		n := e.min + int(r.IntN(int32(e.max-e.min+1)))
		if e.stack >= n {
			r.IntN(27)
		} else {
			for j := 0; j < n; j++ {
				r.IntN(27)
			}
		}
	}
}

// ---- springs ----

func (c *popCtx) spring(x, y, z int, liquid uint16) {
	if c.z.id(x, y+1, z) != 1 || c.z.id(x, y-1, z) != 1 {
		return
	}
	if id := c.z.get(x, y, z); id != 0 && id>>4 != 1 {
		return
	}
	stone, air := 0, 0
	for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		if c.z.id(x+d[0], y, z+d[1]) == 1 {
			stone++
		}
		if c.z.isAir(x+d[0], y, z+d[1]) {
			air++
		}
	}
	if stone == 3 && air == 1 {
		c.z.set(x, y, z, liquid)
	}
}

// ---- plants ----

func (c *popCtx) soilForPlant(id uint16) bool { return id == 2 || id == 3 || id == 60 }

// canStay is BlockBush.canBlockStay: light, and the right ground.
func (c *popCtx) canStayBush(x, y, z int) bool {
	if !(c.z.skyLight(x, y, z) >= 8 || c.z.canSeeSky(x, y, z)) {
		return false
	}
	return c.soilForPlant(c.z.id(x, y-1, z))
}

func (c *popCtx) flowerPatch(x, y, z int, st uint16) {
	c.flowerPatchOf(x, y, z, st)
}

func (c *popCtx) flowerPatchOf(x, y, z int, st uint16) {
	r := c.rnd
	mush := st == stBrownMush || st == stRedMush
	for i := 0; i < 64; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if !c.z.isAir(bx, by, bz) || by >= 255 {
			continue
		}
		ok := false
		if mush {
			below := c.z.get(bx, by-1, bz)
			switch {
			case below>>4 == 110 || below == stPodzol:
				ok = true
			default:
				ok = by >= 0 && c.z.skyLight(bx, by, bz) < 13 && fullBlockID(below>>4)
			}
		} else {
			ok = c.canStayBush(bx, by, bz)
		}
		if ok {
			c.z.set(bx, by, bz, st)
		}
	}
}

func (c *popCtx) pickFlower(x, y, z int) uint16 {
	r := c.rnd
	b := c.bio
	if b.ID == 1 || b.ID == 129 { // plains: tulips where a slow noise is low, otherwise a mix
		d0 := c.g.grassNoise.at(float64(x)/200, float64(z)/200)
		if d0 < -0.8 {
			return stRedFlower | uint16(4+r.IntN(4))
		}
		if r.IntN(3) > 0 {
			switch r.IntN(3) {
			case 0:
				return stRedFlower
			case 1:
				return stRedFlower | 3
			}
			return stRedFlower | 8
		}
		return stYellowFl
	}
	if r.IntN(3) > 0 {
		return stYellowFl
	}
	return stRedFlower
}

func (c *popCtx) grassKind() uint16 {
	switch c.bio.Class {
	case clsTaiga:
		if c.rnd.IntN(5) > 0 {
			return 31<<4 | 2
		}
	}
	return 31<<4 | 1
}

func (c *popCtx) tallGrass(x, y, z int, st uint16) {
	r := c.rnd
	for y > 0 {
		id := c.z.id(x, y, z)
		if id != 0 && !isLeavesID(id) {
			break
		}
		y--
	}
	for i := 0; i < 128; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if c.z.isAir(bx, by, bz) && c.canStayBush(bx, by, bz) {
			c.z.set(bx, by, bz, st)
		}
	}
}

func (c *popCtx) deadBush(x, y, z int) {
	r := c.rnd
	for y > 0 {
		id := c.z.id(x, y, z)
		if id != 0 && !isLeavesID(id) {
			break
		}
		y--
	}
	for i := 0; i < 4; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if c.z.isAir(bx, by, bz) {
			if below := c.z.id(bx, by-1, bz); below == 12 || below == 172 || below == 159 || below == 3 {
				c.z.set(bx, by, bz, stDeadBush)
			}
		}
	}
}

func (c *popCtx) waterlily(x, y, z int) {
	r := c.rnd
	for i := 0; i < 10; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if c.z.isAir(bx, by, bz) && c.z.get(bx, by-1, bz) == stWater && c.z.isAir(bx, by+1, bz) == c.z.isAir(bx, by+1, bz) {
			c.z.set(bx, by, bz, stWaterlily)
		}
	}
}

func (c *popCtx) reeds(x, y, z int) {
	r := c.rnd
	for i := 0; i < 20; i++ {
		bx := x + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(4)) - int(r.IntN(4))
		if !c.z.isAir(bx, y, bz) {
			continue
		}
		w := false
		for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			if isWaterID(c.z.id(bx+d[0], y-1, bz+d[1])) {
				w = true
			}
		}
		if !w {
			continue
		}
		n := 2 + int(r.IntN(r.IntN(3)+1))
		for k := 0; k < n; k++ {
			below := c.z.id(bx, y-1, bz)
			if below == 83 || (below == 2 || below == 3 || below == 12) && c.reedWater(bx, y-1, bz) {
				c.z.set(bx, y+k, bz, stReeds)
			}
		}
	}
}

func (c *popCtx) reedWater(x, y, z int) bool {
	for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		if isWaterID(c.z.id(x+d[0], y, z+d[1])) {
			return true
		}
	}
	return false
}

func (c *popCtx) cactus(x, y, z int) {
	r := c.rnd
	for i := 0; i < 10; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if !c.z.isAir(bx, by, bz) {
			continue
		}
		n := 1 + int(r.IntN(r.IntN(3)+1))
		for k := 0; k < n; k++ {
			ok := true
			for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if solidMat(c.z.id(bx+d[0], by, bz+d[1])) {
					ok = false
				}
			}
			if below := c.z.id(bx, by-1, bz); ok && (below == 81 || below == 12) {
				c.z.set(bx, by+k, bz, stCactus)
			}
		}
	}
}

func (c *popCtx) pumpkins(x, y, z int) {
	r := c.rnd
	for i := 0; i < 64; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if c.z.isAir(bx, by, bz) && c.z.id(bx, by-1, bz) == 2 {
			facing := [4]uint16{2, 3, 0, 1}[r.IntN(4)] // north, east, south, west
			c.z.set(bx, by, bz, stPumpkin|facing)
		}
	}
}

func (c *popCtx) bigMushroom(x, y, z int) {
	// Huge mushrooms (mushroom islands): the cap and stem take many draws; approximated by
	// spending the same first draws so the rest of the chunk stays in step.
	c.rnd.IntN(10)
}

// ---- snow and ice, last ----

func (c *popCtx) freezeAndSnow() {
	for k := 0; k < 16; k++ {
		for j := 0; j < 16; j++ {
			x, z := c.px+8+k, c.pz+8+j
			y := c.z.precipitationHeight(x, z)
			b := c.z.biome(x, z)
			if c.g.temperatureAt(b, x, y-1, z) <= 0.15 && c.z.get(x, y-1, z) == stWater {
				c.z.set(x, y-1, z, stIce)
			}
			if c.g.temperatureAt(b, x, y, z) <= 0.15 && y >= 0 && y < 256 && c.z.isAir(x, y, z) {
				below := c.z.id(x, y-1, z)
				if below != 79 && below != 174 && (isLeavesID(below) || fullBlockID(below)) {
					c.z.set(x, y, z, stSnowLayer)
				}
			}
		}
	}
}

// ---- biome decorations around the common ones ----

func (c *popCtx) biomeExtrasBefore() {
	r := c.rnd
	b := c.bio
	if b.Class == clsForest && b.Forest != 3 { // large flowers
		n := int(r.IntN(5)) - 3
		if b.Forest == 1 {
			n += 2
		}
		for j := 0; j < n; j++ {
			kind := uint16([3]uint16{1, 4, 5}[r.IntN(3)])
			for l := 0; l < 5; l++ {
				x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
				y := int(r.IntN(int32(c.z.height(x, zz) + 32)))
				if c.doublePlant(x, y, zz, kind) {
					break
				}
			}
		}
	}
	if b.Class == clsForest && b.Forest == 3 { // roofed forest: a 4 x 4 lattice of canopy trees
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				x := c.px + i*4 + 1 + 8 + int(r.IntN(3))
				zz := c.pz + j*4 + 1 + 8 + int(r.IntN(3))
				y := c.z.height(x, zz)
				if r.IntN(20) == 0 {
					c.bigMushroom(x, y, zz)
				} else {
					c.canopyTree(x, y, zz)
				}
			}
		}
	}
	if b.Class == clsPlains && b.ID == 129 { // sunflowers
		for i := 0; i < 10; i++ {
			for l := 0; l < 5; l++ {
				x, zz := c.px+int(r.IntN(16))+8, c.pz+int(r.IntN(16))+8
				y := int(r.IntN(int32(c.z.height(x, zz) + 32)))
				if c.doublePlant(x, y, zz, 0) {
					break
				}
			}
		}
	}
}

func (c *popCtx) doublePlant(x, y, z int, kind uint16) bool {
	r := c.rnd
	any := false
	for i := 0; i < 64; i++ {
		bx := x + int(r.IntN(8)) - int(r.IntN(8))
		by := y + int(r.IntN(4)) - int(r.IntN(4))
		bz := z + int(r.IntN(8)) - int(r.IntN(8))
		if c.z.isAir(bx, by, bz) && by < 254 && c.z.isAir(bx, by+1, bz) && c.soilForPlant(c.z.id(bx, by-1, bz)) {
			c.z.set(bx, by, bz, stDoublePlnt|kind)
			c.z.set(bx, by+1, bz, stDoublePlnt|8)
			any = true
		}
	}
	return any
}

func (c *popCtx) biomeExtrasAfter() {
	r := c.rnd
	b := c.bio
	switch {
	case b.Class == clsHills:
		n := 3 + int(r.IntN(6))
		for j := 0; j < n; j++ {
			x, y, zz := c.px+int(r.IntN(16)), int(r.IntN(28))+4, c.pz+int(r.IntN(16))
			if c.z.id(x, y, zz) == 1 {
				c.z.set(x, y, zz, stEmeraldOre)
			}
		}
		for j := 0; j < 7; j++ {
			x, y, zz := c.px+int(r.IntN(16)), int(r.IntN(64)), c.pz+int(r.IntN(16))
			c.silverfish(x, y, zz)
		}
	case b.Class == clsDesert:
		r.IntN(1000)
		r.IntN(64)
	}
}

// silverfish blocks are WorldGenMinable of monster eggs, size 9, replacing stone.
func (c *popCtx) silverfish(x, y, z int) { c.minable(x, y, z, 9, stMonsterEgg) }

var _ = math.Pi
