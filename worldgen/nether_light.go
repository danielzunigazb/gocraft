package worldgen

// The Nether has no sky, so a column's light is block light only, spread from lava, fire, glowstone
// and the rest of what shines, through everything that is not solid. The zone supplies the
// neighbours the light comes in from.

func nhLuminance(id uint16) uint8 {
	switch id {
	case 10, 11, 51, 89: // lava, fire, glowstone
		return 15
	case 50: // torch
		return 14
	case 90: // portal
		return 11
	case 39: // brown mushroom
		return 1
	}
	return 0
}

// nhOpaque reports whether light stops at the block.
func nhOpaque(id uint16) bool {
	switch id {
	case 0, 50, 51, 39, 40, 90:
		return false
	}
	return true
}

// nhLight is the light of the middle column of the zone: sky 0, block light as spread.
func nhLight(z *nhZone) []uint8 {
	const w = 48
	cell := func(x, y, zz int) int { return (y*w+zz)*w + x }
	blk := make([]uint8, nhHeight*w*w)
	var queue []int32
	for y := 0; y < nhHeight; y++ {
		for zz := 0; zz < w; zz++ {
			for x := 0; x < w; x++ {
				id := z.col[x>>4][zz>>4][nhIdx(x&15, y, zz&15)] >> 4
				if l := nhLuminance(id); l > 0 {
					i := cell(x, y, zz)
					blk[i] = l
					queue = append(queue, int32(i))
				}
			}
		}
	}
	// Label-correcting spread: a cell is looked at again whenever a brighter light reaches it.
	for head := 0; head < len(queue); head++ {
		c := int(queue[head])
		l := blk[c]
		if l <= 1 {
			continue
		}
		x, zz, y := c%w, (c/w)%w, c/(w*w)
		for _, d := range [6][3]int{{-1, 0, 0}, {1, 0, 0}, {0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}} {
			nx, ny, nz := x+d[0], y+d[1], zz+d[2]
			if nx < 0 || ny < 0 || nz < 0 || nx >= w || ny >= nhHeight || nz >= w {
				continue
			}
			if nhOpaque(z.col[nx>>4][nz>>4][nhIdx(nx&15, ny, nz&15)] >> 4) {
				continue
			}
			j := cell(nx, ny, nz)
			if blk[j] < l-1 {
				blk[j] = l - 1
				queue = append(queue, int32(j))
			}
		}
	}
	out := make([]uint8, 65536)
	for y := 0; y < nhHeight; y++ {
		for zz := 0; zz < 16; zz++ {
			for x := 0; x < 16; x++ {
				out[nhIdx(x, y, zz)] = blk[cell(x+16, y, zz+16)]
			}
		}
	}
	return out
}
