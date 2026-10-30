package client

import (
	"gocraft/world"
)

// vertex is what the GPU reads: a position, atlas coordinates and a colour (tint x shade x light).
type vertex struct {
	X, Y, Z, U, V float32
	C             [4]uint8
}

const vertexBytes = 24

var white = [3]float32{1, 1, 1}

// Blocks whose faces are hidden by a neighbour of the same kind, even though they are not opaque.
func joinsSame(id uint16) bool { return id == 20 || id == 95 || id == 79 || id == 165 }

// meshData is the quads of one column, four vertices each, in world coordinates. The index buffer
// is shared: quad i uses vertices 4i..4i+3.
type meshData struct {
	solid []vertex
	water []vertex // drawn after everything else, blended
	// minY and maxY bound the geometry, for culling.
	minY, maxY float32
	// solidSec[s] and waterSec[s] are the first vertex of section s (16 blocks high) in solid and
	// water; the quads of section s are solidSec[s]..solidSec[s+1]. The mesher walks the sections
	// from the bottom, so each section's quads are together.
	solidSec, waterSec [17]int32
	// vis[s] is the section's face-to-face connectivity (see sectionVis), for cave culling.
	vis [16]uint16
	// Once meshed, solid and water are sorted by the direction the quads face and then by section
	// (see faces.go); solidF and waterF say where each group is. solidSec and waterSec then describe
	// the order the mesher produced them in, before the sort.
	solidF, waterF     faceIndex
	tmpSolid, tmpWater []vertex // the other half of the sort, swapped in and out
}

// sortAll sorts the finished solid and water quads by facing direction.
func (m *meshData) sortAll() {
	d := sortFaces(m.solid, &m.solidSec, m.tmpSolid, &m.solidF)
	m.solid, m.tmpSolid = d, m.solid[:0]
	d = sortFaces(m.water, &m.waterSec, m.tmpWater, &m.waterF)
	m.water, m.tmpWater = d, m.water[:0]
}

func (m *meshData) reset() {
	m.solid, m.water = m.solid[:0], m.water[:0]
	m.solidSec, m.waterSec = [17]int32{}, [17]int32{}
	m.vis = [16]uint16{}
}

func toByte(f float32) uint8 {
	v := int(f*255 + 0.5)
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return uint8(v)
}

// padded index offsets of the six neighbours, in the order of dDown..dEast, and their y steps.
var (
	padOff = [6]int{-18 * 18, 18 * 18, -18, 18, -1, 1}
	dirDY  = [6]int{-1, 1, 0, 0, 0, 0}
)

// faceLight is the light a face is drawn with, as 1.8's flat lighting takes it: a face on the edge
// of its block gets the light of the block it faces, any other face the block's own light.
func faceLight(p *world.Padded, idx, y int, q *quad, own uint8) uint8 {
	d := q.cull
	if d < 0 {
		d = q.dir
		if y2 := y + dirDY[d]; y2 < 0 || y2 > 255 {
			return own
		}
		return maxLight(own, p.Light[idx+padOff[d]])
	}
	if y2 := y + dirDY[d]; y2 < 0 || y2 > 255 {
		return world.FullLight
	}
	return p.Light[idx+padOff[d]]
}

// mesher turns columns into meshes. One per worker: it owns the copy of the column it works on.
type mesher struct {
	pad   world.Padded
	seen  [4096]bool // sectionVis: cells already in a region
	stack []uint16
}

// maxLight is the brighter of two light bytes, sky and block light taken apart.
func maxLight(a, b uint8) uint8 { return max(a&0xF0, b&0xF0) | max(a&15, b&15) }

// emitQuad adds a quad's four vertices. The colour is tint x shade; the light byte (sky<<4 | block)
// rides in the alpha, and the GPU turns it into a colour through the lightmap.
func emitQuad(out []vertex, a *quad, wx, wy, wz float32, tint [3]float32, light uint8) []vertex {
	c := [4]uint8{toByte(tint[0] * a.shade), toByte(tint[1] * a.shade), toByte(tint[2] * a.shade), light}
	for i := 0; i < 4; i++ {
		out = append(out, vertex{wx + a.apos[i][0], wy + a.apos[i][1], wz + a.apos[i][2], a.auv[i][0], a.auv[i][1], c})
	}
	return out
}

// mesh builds the geometry of the column at cp into out, reusing out's slices.
func (m *mesher) mesh(w *world.World, cp world.ChunkPos, res *resources, fancy bool, out *meshData) {
	out.reset()
	p := &m.pad
	w.FillPadded(cp, p)
	bx, bz := float32(cp.X*16), float32(cp.Z*16)

	var cx, cy, cz int // the block being meshed, for the neighbour lookups of fences, panes and doors
	nb := func(dx, dy, dz int32) uint16 {
		y := cy + int(dy)
		if y < 0 || y > 255 {
			return 0
		}
		return p.Blocks[world.PadIndex(cx+int(dx), y, cz+int(dz))]
	}

	lo, hi := 16, -1
	for sy := 0; sy < 16; sy++ {
		out.solidSec[sy], out.waterSec[sy] = int32(len(out.solid)), int32(len(out.water))
		if p.Solid[sy] == 0 {
			out.vis[sy] = visAll
			continue // a section of air: nothing to draw
		}
		out.vis[sy] = m.sectionVis(res, sy)
		lo, hi = min(lo, sy), sy
		for y := sy * 16; y < sy*16+16; y++ {
			for z := 0; z < 16; z++ {
				base := world.PadIndex(0, y, z)
				for x := 0; x < 16; x++ {
					idx := base + x
					st := p.Blocks[idx]
					if st == 0 || st >= 4096 {
						continue
					}
					id := st >> 4
					wx, wz := bx+float32(x), bz+float32(z)
					if isFluid(id) {
						m.fluid(out, res, id, idx, y, wx, wz)
						continue
					}
					e := &res.fast[st]
					vs := e.variants
					if e.dep {
						cx, cy, cz = x, y, z
						file, props := res.stateOf(id, int(st&15), nb)
						vs = res.pick(file, props)
					}
					if len(vs) == 0 {
						continue
					}
					v := vs[int(uint32(int32(wx))*3129871^uint32(int32(wz))*116129781^uint32(y))%len(vs)]
					own := p.Light[idx]
					biome := p.Biomes[z<<4|x]
					tinted := false
					var tint [3]float32
					leaf := !fancy && (id == 18 || id == 161)
					for i := range v.quads {
						q := &v.quads[i]
						if leaf { // fast graphics treat leaves as solid: no face is drawn against more leaves
							var n uint16
							if y2 := y + dirDY[q.dir]; y2 >= 0 && y2 <= 255 {
								n = p.Blocks[idx+padOff[q.dir]] >> 4
							}
							if n == 18 || n == 161 {
								continue
							}
						}
						if q.cull >= 0 {
							var n uint16
							if y2 := y + dirDY[q.cull]; y2 >= 0 && y2 <= 255 {
								n = p.Blocks[idx+padOff[q.cull]] >> 4
							}
							if res.opaque[n&255] || (n == id && joinsSame(id)) {
								continue
							}
						}
						t := white
						if q.tint >= 0 {
							if !tinted {
								tint, tinted = res.tintFor(id, int(st&15), biome), true
							}
							t = tint
						}
						out.solid = emitQuad(out.solid, q, wx, float32(y), wz, t, faceLight(p, idx, y, q, own))
					}
				}
			}
		}
	}
	out.solidSec[16], out.waterSec[16] = int32(len(out.solid)), int32(len(out.water))
	out.sortAll()
	if hi < 0 {
		out.minY, out.maxY = 0, 0
		return
	}
	out.minY, out.maxY = float32(lo*16), float32(hi*16+16)
}

func (m *mesher) fluid(out *meshData, res *resources, id uint16, idx, y int, wx, wz float32) {
	p := &m.pad
	water := id == 8 || id == 9
	class := func(n uint16) bool { return isFluid(n) && (n == 8 || n == 9) == water }
	tile, tint := res.lava, white
	if water {
		tile, tint = res.water, tintWater
	}
	top := float32(14)
	if y < 255 && class(p.Blocks[idx+padOff[dUp]]>>4) {
		top = 16
	}
	for d := 0; d < 6; d++ {
		var n uint16
		l := uint8(world.FullLight)
		if y2 := y + dirDY[d]; y2 >= 0 && y2 <= 255 {
			n = p.Blocks[idx+padOff[d]] >> 4
			l = p.Light[idx+padOff[d]]
		}
		if class(n) || res.opaque[n&255] {
			continue
		}
		q := fluidQuad(d, tile, top)
		if d >= dNorth {
			q.uv = [4][2]float32{{0, 0}, {0, top}, {16, top}, {16, 0}}
		}
		for k := 0; k < 4; k++ {
			q.apos[k] = [3]float32{q.pos[k][0] / 16, q.pos[k][1] / 16, q.pos[k][2] / 16}
			q.auv[k][0], q.auv[k][1] = res.atlas.coord(q.tile, q.uv[k][0], q.uv[k][1])
		}
		if water {
			out.water = emitQuad(out.water, &q, wx, float32(y), wz, tint, l)
		} else {
			out.solid = emitQuad(out.solid, &q, wx, float32(y), wz, tint, l)
		}
	}
}

// fluidQuad builds one face of a fluid block with the given top height (in 1/16).
func fluidQuad(d int, tile int, top float32) quad {
	q := quad{tile: tile, tint: -1, cull: -1, dir: d, shade: shadeOf[d]}
	ext := [6]float64{0, 0, 0, 16, float64(top), 16}
	for i := 0; i < 4; i++ {
		c := faceCorners[d][i]
		q.pos[i] = [3]float32{float32(ext[c[0]]), float32(ext[c[1]]), float32(ext[c[2]])}
	}
	q.uv = [4][2]float32{{0, 0}, {0, 16}, {16, 16}, {16, 0}}
	return q
}

// The faces of a section, in the order of dDown..dEast.
const visAll = 1<<15 - 1 // every pair of faces connected

// pairBit[a][b] is the bit for "a path through the section joins face a and face b".
var pairBit = func() (t [6][6]uint16) {
	n := uint(0)
	for a := 0; a < 6; a++ {
		for b := a + 1; b < 6; b++ {
			t[a][b], t[b][a] = 1<<n, 1<<n
			n++
		}
	}
	return
}()

// sectionVis is 1.8's VisGraph for one 16x16x16 section of the column in m.pad: every region of
// connected see-through cells is followed, the faces of the section it touches are noted, and each
// pair of those faces is marked as connected. Cave culling then never looks through a section from
// one face to another unless a path of see-through blocks joins them.
func (m *mesher) sectionVis(res *resources, sy int) (vis uint16) {
	p := &m.pad
	m.seen = [4096]bool{}
	y0 := sy * 16
	at := func(x, y, z int) bool { // is the cell see-through
		return !res.opaque[(p.Blocks[world.PadIndex(x, y0+y, z)]>>4)&255]
	}
	for start := 0; start < 4096; start++ {
		if m.seen[start] {
			continue
		}
		if x, y, z := start&15, start>>8, start>>4&15; !at(x, y, z) {
			m.seen[start] = true
			continue
		}
		var faces uint8
		m.stack = append(m.stack[:0], uint16(start))
		m.seen[start] = true
		for len(m.stack) > 0 {
			c := int(m.stack[len(m.stack)-1])
			m.stack = m.stack[:len(m.stack)-1]
			x, y, z := c&15, c>>8, c>>4&15
			if y == 0 {
				faces |= 1 << dDown
			} else if y == 15 {
				faces |= 1 << dUp
			}
			if z == 0 {
				faces |= 1 << dNorth
			} else if z == 15 {
				faces |= 1 << dSouth
			}
			if x == 0 {
				faces |= 1 << dWest
			} else if x == 15 {
				faces |= 1 << dEast
			}
			for d := 0; d < 6; d++ {
				nx, ny, nz := x, y, z
				switch d {
				case dDown:
					ny--
				case dUp:
					ny++
				case dNorth:
					nz--
				case dSouth:
					nz++
				case dWest:
					nx--
				case dEast:
					nx++
				}
				if nx < 0 || nx > 15 || ny < 0 || ny > 15 || nz < 0 || nz > 15 {
					continue
				}
				n := nx | nz<<4 | ny<<8
				if m.seen[n] {
					continue
				}
				m.seen[n] = true
				if at(nx, ny, nz) {
					m.stack = append(m.stack, uint16(n))
				}
			}
		}
		for a := 0; a < 6; a++ {
			if faces&(1<<a) == 0 {
				continue
			}
			for b := a + 1; b < 6; b++ {
				if faces&(1<<b) != 0 {
					vis |= pairBit[a][b]
				}
			}
		}
		if vis == visAll {
			return vis
		}
	}
	return vis
}
