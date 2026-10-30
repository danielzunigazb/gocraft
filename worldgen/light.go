package worldgen

// The light of a generated chunk: sky light falls straight down from the open sky and spreads
// sideways into caves and under overhangs; block light spreads from lava and fire. The zone gives
// the neighbours that light crosses in from, so chunk edges do not show seams.

const lightSpan = 48

func lumOf(id uint16) uint8 {
	switch id {
	case 10, 11, 51, 89:
		return 15
	case 50:
		return 14
	}
	return 0
}

type lightCell struct{ x, z, y int16 }

func (g *Generator) light(z *zone) []uint8 {
	// Only the part of the zone below the highest terrain needs spreading: above it is all open sky.
	top := 1
	var heights [lightSpan * lightSpan]int16
	for x := 0; x < lightSpan; x++ {
		for zz := 0; zz < lightSpan; zz++ {
			h := z.height(z.ox+x, z.oz+zz)
			heights[x+zz*lightSpan] = int16(h)
			top = max(top, h)
		}
	}
	top = min(top+1, 255)
	ny := top + 1
	cell := func(x, y, zz int) int { return (y*lightSpan+zz)*lightSpan + x }
	opa := make([]uint8, lightSpan*lightSpan*ny)
	sky := make([]uint8, len(opa))
	blk := make([]uint8, len(opa))
	var blkSeeds []lightCell
	for zz := 0; zz < lightSpan; zz++ {
		for x := 0; x < lightSpan; x++ {
			c := z.blocks[x>>4][zz>>4]
			lx, lz := x&15, zz&15
			l := 15
			for y := top; y >= 0; y-- {
				id := c[idx(lx, y, lz)] >> 4
				o := opacityOf(id)
				i := cell(x, y, zz)
				opa[i] = o
				if o == 255 {
					l = 0
				} else if o > 0 {
					l = max(l-int(o), 0)
				}
				sky[i] = uint8(l)
				if lum := lumOf(id); lum > 0 {
					blk[i] = lum
					blkSeeds = append(blkSeeds, lightCell{int16(x), int16(zz), int16(y)})
				}
			}
		}
	}
	spread := func(light []uint8, seeds []lightCell) {
		queue := seeds
		for len(queue) > 0 {
			c := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			l := light[cell(int(c.x), int(c.y), int(c.z))]
			if l <= 1 {
				continue
			}
			for _, d := range [6][3]int{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}, {0, -1, 0}, {0, 1, 0}} {
				x, y, zz := int(c.x)+d[0], int(c.y)+d[1], int(c.z)+d[2]
				if x < 0 || x >= lightSpan || zz < 0 || zz >= lightSpan || y < 0 || y >= ny {
					continue
				}
				i := cell(x, y, zz)
				o := opa[i]
				if o == 255 {
					continue
				}
				nl := int(l) - int(max(o, 1))
				if nl > int(light[i]) {
					light[i] = uint8(nl)
					queue = append(queue, lightCell{int16(x), int16(zz), int16(y)})
				}
			}
		}
	}
	// Sky light seeds: lit cells next to darker open cells.
	var seeds []lightCell
	for y := 0; y < ny; y++ {
		for zz := 0; zz < lightSpan; zz++ {
			for x := 0; x < lightSpan; x++ {
				i := cell(x, y, zz)
				l := sky[i]
				if l <= 1 {
					continue
				}
				edge := func(j int) bool { return opa[j] != 255 && int(sky[j]) < int(l)-1 }
				if x > 0 && edge(i-1) || x < lightSpan-1 && edge(i+1) || zz > 0 && edge(i-lightSpan) || zz < lightSpan-1 && edge(i+lightSpan) || y > 0 && edge(i-lightSpan*lightSpan) {
					seeds = append(seeds, lightCell{int16(x), int16(zz), int16(y)})
				}
			}
		}
	}
	spread(sky, seeds)
	spread(blk, blkSeeds)
	out := make([]uint8, 65536)
	for y := 0; y < 256; y++ {
		for zz := 0; zz < 16; zz++ {
			for x := 0; x < 16; x++ {
				s := uint8(15)
				var b uint8
				if y < ny {
					i := cell(x+16, y, zz+16)
					s, b = sky[i], blk[i]
				}
				out[idx(x, y, zz)] = s<<4 | b
			}
		}
	}
	return out
}
