package worldgen

import "math"

// A zone is the 3 x 3 chunks around the one being made. Decorating chunk P changes the 2 x 2
// chunks from P on, so the four decorations that reach the middle chunk all fit in a zone.

type zone struct {
	ox, oz int // block position of the zone's corner: the first chunk of the 3 x 3
	blocks [3][3]*[65536]uint16
	biomes [3][3]*[256]uint8
}

func (z *zone) chunkOf(x, zz int) (cx, cz, lx, lz int, ok bool) {
	rx, rz := x-z.ox, zz-z.oz
	if rx < 0 || rz < 0 || rx >= 48 || rz >= 48 {
		return 0, 0, 0, 0, false
	}
	return rx >> 4, rz >> 4, rx & 15, rz & 15, true
}

func (z *zone) get(x, y, zz int) uint16 {
	if y < 0 || y > 255 {
		return 0
	}
	cx, cz, lx, lz, ok := z.chunkOf(x, zz)
	if !ok {
		return 0
	}
	return z.blocks[cx][cz][idx(lx, y, lz)]
}

func (z *zone) set(x, y, zz int, st uint16) {
	if y < 0 || y > 255 {
		return
	}
	cx, cz, lx, lz, ok := z.chunkOf(x, zz)
	if !ok {
		return
	}
	z.blocks[cx][cz][idx(lx, y, lz)] = st
}

func (z *zone) biome(x, zz int) *Biome {
	cx, cz, lx, lz, ok := z.chunkOf(x, zz)
	if !ok {
		return biomes[1]
	}
	return biomeByID(int(z.biomes[cx][cz][lx+lz*16]))
}

func (z *zone) id(x, y, zz int) uint16 { return z.get(x, y, zz) >> 4 }

func (z *zone) isAir(x, y, zz int) bool { return z.get(x, y, zz) == 0 }

// ---- what a block is, as the generators ask ----

func isLiquidID(id uint16) bool { return id >= 8 && id <= 11 }
func isWaterID(id uint16) bool  { return id == 8 || id == 9 }
func isLeavesID(id uint16) bool { return id == 18 || id == 161 }
func isLogID(id uint16) bool    { return id == 17 || id == 162 }

// plantID: blocks of Material.plants / vine / snow layer: not solid, and nothing walks into them.
func plantID(id uint16) bool {
	switch id {
	case 6, 31, 32, 37, 38, 39, 40, 50, 51, 55, 59, 63, 65, 66, 68, 69, 70, 72, 75, 76, 77, 78, 83, 104, 105, 106, 111, 115, 140, 141, 142, 143, 171, 175:
		return true
	}
	return false
}

// solidMat is Material.isSolid.
func solidMat(id uint16) bool { return id != 0 && !isLiquidID(id) && !plantID(id) }

// blocksMovement is Material.blocksMovement: what a standing player stops on.
func blocksMovement(id uint16) bool { return solidMat(id) }

var fullBlocks = func() [256]bool {
	var t [256]bool
	for _, id := range []int{1, 2, 3, 4, 5, 7, 12, 13, 14, 15, 16, 17, 18, 19, 21, 22, 23, 24, 25, 35, 41, 42, 43, 45, 46, 47, 48, 49, 56, 57, 58, 61, 62, 73, 74, 80, 82, 84, 86, 87, 88, 89, 91, 95, 97, 98, 99, 100, 110, 112, 121, 123, 124, 129, 133, 153, 155, 159, 161, 162, 168, 172, 173, 174, 179, 216} {
		t[id] = true
	}
	return t
}()

func fullBlockID(id uint16) bool { return id < 256 && fullBlocks[id] }

// lightOpacity: what a block takes from the light that passes through (0 lets it by).
func lightOpacity(id uint16) uint8 {
	switch {
	case id == 0 || plantID(id):
		return 0
	case id == 20 || id == 95 || id == 102 || id == 160 || id == 27 || id == 28 || id == 66 || id == 157 || id == 50 || id == 51 || id == 65 || id == 54 && false:
		return 0
	case id == 8 || id == 9 || id == 79:
		return 3
	case id == 10 || id == 11:
		return 0
	case id == 18 || id == 161 || id == 30:
		return 1
	}
	return 255
}

var opacityTable = func() (t [4096]uint8) {
	for id := range t {
		t[id] = lightOpacity(uint16(id))
	}
	return
}()

func opacityOf(id uint16) uint8 { return opacityTable[id&4095] }

func lightBlocked(id uint16) bool { return opacityOf(id) > 0 }

// height is the game's getHeight: one above the highest block that stops light.
func (z *zone) height(x, zz int) int {
	cx, cz, lx, lz, ok := z.chunkOf(x, zz)
	if !ok {
		return 0
	}
	c := z.blocks[cx][cz]
	for y := 255; y > 0; y-- {
		if lightBlocked(c[idx(lx, y-1, lz)] >> 4) {
			return y
		}
	}
	return 0
}

// precipitationHeight is one above the highest block that stops movement or is liquid.
func (z *zone) precipitationHeight(x, zz int) int {
	cx, cz, lx, lz, ok := z.chunkOf(x, zz)
	if !ok {
		return 0
	}
	c := z.blocks[cx][cz]
	for y := 255; y >= 0; y-- {
		id := c[idx(lx, y, lz)] >> 4
		if blocksMovement(id) || isLiquidID(id) {
			return y + 1
		}
	}
	return 0
}

// topSolidOrLiquid is getTopSolidOrLiquidBlock: the block above the highest one that holds a
// player up (leaves excluded), starting from above the highest filled section.
func (z *zone) topSolidOrLiquid(x, zz int) int {
	cx, cz, lx, lz, ok := z.chunkOf(x, zz)
	if !ok {
		return 0
	}
	c := z.blocks[cx][cz]
	top := 0
	for s := 15; s >= 0; s-- {
		filled := false
		for i := s * 4096; i < s*4096+4096; i++ {
			if c[i] != 0 {
				filled = true
				break
			}
		}
		if filled {
			top = s*16 + 16
			break
		}
	}
	y := top + 16
	for ; y >= 0; y-- {
		id := c[idx(lx, min(y-1, 255), lz)] >> 4
		if y-1 < 0 {
			break
		}
		if blocksMovement(id) && !isLeavesID(id) {
			break
		}
	}
	return y
}

// canSeeSky: nothing that stops light above the block.
func (z *zone) canSeeSky(x, y, zz int) bool { return y >= z.height(x, zz) }

// skyLight is a stand-in for the light engine during generation: 15 in the open, less under
// what stops light, 0 in a closed cave.
func (z *zone) skyLight(x, y, zz int) int {
	if y >= z.height(x, zz) {
		return 15
	}
	lost := 0
	for yy := y + 1; yy < 256; yy++ {
		o := opacityOf(z.id(x, yy, zz))
		if o == 255 {
			return 0
		}
		lost += int(o)
		if lost >= 15 {
			return 0
		}
	}
	return 15 - lost
}

// temperatureAt is BiomeGenBase.getFloatTemperature: colder as it climbs above y 64.
func (g *Generator) temperatureAt(b *Biome, x, y, z int) float32 {
	if y > 64 {
		f := float32(g.tempNoise.at(float64(x)/8, float64(z)/8) * 4)
		return b.Temp - (f+float32(y)-64)*0.05/30
	}
	return b.Temp
}

var _ = math.Pi
