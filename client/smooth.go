package client

import (
	"sync/atomic"

	"gocraft/world"
)

// Smooth lighting, after 1.8's BlockModelRenderer.renderModelAmbientOcclusionQuad (the game's default): each
// corner of a block's face takes the light of the four cells around it (the one the face looks at, the two beside
// that along the face's edges, and the diagonal one), averaged, and is darkened by how many of them are solid
// (ambient occlusion: a solid cell counts 0.2, an open one 1.0). So the light and the shade run smoothly over a
// face and the creases and edges of terrain are darker. A cell of solid block holds no light, so it is read as
// the light of the cell in front of the face (getAoBrightness), and a diagonal cell between two solid ones is
// as solid as they are. Blocks that give light, and faces that are not the whole face of a block, keep the flat
// light of the game's "smooth lighting: off".

// smoothLighting is the setting (options.txt's ao), read by the mesh workers.
var smoothLighting atomic.Bool

func init() { smoothLighting.Store(true) }

type lightCell struct {
	light  uint8
	ao     float32 // 0.2 for a solid cell, 1 for an open one
	opaque bool
}

// cellAt reads the cell at the padded offset off from the block at idx, dy cells up from y.
func cellAt(res *resources, p *world.Padded, idx, y, off, dy int) lightCell {
	if y2 := y + dy; y2 > 255 {
		return lightCell{world.FullLight, 1, false}
	} else if y2 < 0 {
		return lightCell{0, 0.2, true}
	}
	i := idx + off
	if res.opaque[(p.Blocks[i]>>4)&255] {
		return lightCell{p.Light[i], 0.2, true}
	}
	return lightCell{p.Light[i], 1, false}
}

// wholeFace is whether a quad is the entire face of a block on the side of d: all its corners on that side and
// reaching both extremes of the other two axes.
func wholeFace(q *quad, d int) bool {
	axis := [6]int{1, 1, 2, 2, 0, 0}[d] // the axis the face is perpendicular to
	edge := [6]float32{0, 1, 0, 1, 0, 1}[d]
	var lo, hi [3]bool
	for i := 0; i < 4; i++ {
		if q.apos[i][axis] != edge {
			return false
		}
		for a := 0; a < 3; a++ {
			switch q.apos[i][a] {
			case 0:
				lo[a] = true
			case 1:
				hi[a] = true
			default:
				if a != axis {
					return false
				}
			}
		}
	}
	for a := 0; a < 3; a++ {
		if a != axis && (!lo[a] || !hi[a]) {
			return false
		}
	}
	return true
}

// smoothFace adds a whole block face with a light and a shade of its own at each corner.
func smoothFace(out []vertex, res *resources, p *world.Padded, idx, y int, q *quad, wx, wy, wz float32, tint [3]float32) []vertex {
	d := q.cull
	// The two axes along the face: padded offset, the cell step in y, and the quad's coordinate for each.
	var e1, e2, y2, c1, c2 int
	switch d {
	case dDown, dUp:
		e1, c1, e2, c2 = 1, 0, 18, 2
	case dNorth, dSouth:
		e1, c1, e2, c2, y2 = 1, 0, 18*18, 1, 1
	default:
		e1, c1, e2, c2, y2 = 18, 2, 18*18, 1, 1
	}
	front, fdy := padOff[d], dirDY[d]
	centre := cellAt(res, p, idx, y, front, fdy)
	for i := 0; i < 4; i++ {
		s1, s2 := -1, -1
		if q.apos[i][c1] > 0.5 {
			s1 = 1
		}
		if q.apos[i][c2] > 0.5 {
			s2 = 1
		}
		a := cellAt(res, p, idx, y, front+s1*e1, fdy)
		b := cellAt(res, p, idx, y, front+s2*e2, fdy+s2*y2)
		k := a // a diagonal cell between two solid ones is as solid as they are
		if !(a.opaque && b.opaque) {
			k = cellAt(res, p, idx, y, front+s1*e1+s2*e2, fdy+s2*y2)
		}
		la, lb, lk := a.light, b.light, k.light
		if la == 0 {
			la = centre.light
		}
		if lb == 0 {
			lb = centre.light
		}
		if lk == 0 {
			lk = centre.light
		}
		sky := (int(lk>>4) + int(la>>4) + int(lb>>4) + int(centre.light>>4) + 2) >> 2
		blk := (int(lk&15) + int(la&15) + int(lb&15) + int(centre.light&15) + 2) >> 2
		ao := (k.ao + a.ao + b.ao + centre.ao) * 0.25
		s := q.shade * ao
		out = append(out, vertex{wx + q.apos[i][0], wy + q.apos[i][1], wz + q.apos[i][2], q.auv[i][0], q.auv[i][1],
			[4]uint8{toByte(tint[0] * s), toByte(tint[1] * s), toByte(tint[2] * s), uint8(sky<<4 | blk)}})
	}
	return out
}
