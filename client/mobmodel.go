package client

import "math"

// Mob models, after 1.8's ModelBase classes: a mob is a handful of boxes, each hung from a pivot
// and turned by a pose (walking swing, head look, arms raised). The boxes are laid out on the
// mob's texture the way ModelBox does it, so the game's own textures fit.

type mbox struct {
	u, v    float32 // where the box's unfolded faces start on the texture
	x, y, z float32 // the corner nearest the origin, in 1/16 blocks, y pointing down
	w, h, d float32
	expand  float32
	mirror  bool
}

type mpart struct {
	boxes      []mbox
	px, py, pz float32 // the pivot the part turns about
	rx, ry, rz float32 // its rest rotation, radians
}

type mvert struct{ x, y, z, u, v float32 }

type mquad [4]mvert

// boxQuads are the six faces of a box with ModelBox's corner order and texture rectangles.
func boxQuads(b mbox, texW, texH float32) [6]mquad {
	x1, y1, z1 := b.x-b.expand, b.y-b.expand, b.z-b.expand
	x2, y2, z2 := b.x+b.w+b.expand, b.y+b.h+b.expand, b.z+b.d+b.expand
	if b.mirror {
		x1, x2 = x2, x1
	}
	p := [8][3]float32{
		{x1, y1, z1}, {x2, y1, z1}, {x2, y2, z1}, {x1, y2, z1}, {x1, y1, z2}, {x2, y1, z2}, {x2, y2, z2}, {x1, y2, z2},
	} // v7, v0, v1, v2, v3, v4, v5, v6 in the game's naming
	const v7, v0, v1, v2, v3, v4, v5, v6 = 0, 1, 2, 3, 4, 5, 6, 7
	u, v, w, h, d := b.u, b.v, b.w, b.h, b.d
	type face struct {
		c              [4]int
		u1, v1, u2, v2 float32
	}
	faces := [6]face{
		{[4]int{v4, v0, v1, v5}, u + d + w, v + d, u + d + w + d, v + d + h},
		{[4]int{v7, v3, v6, v2}, u, v + d, u + d, v + d + h},
		{[4]int{v4, v3, v7, v0}, u + d, v, u + d + w, v + d},
		{[4]int{v1, v2, v6, v5}, u + d + w, v + d, u + d + w + w, v},
		{[4]int{v0, v7, v2, v1}, u + d, v + d, u + d + w, v + d + h},
		{[4]int{v3, v4, v5, v6}, u + d + w + d, v + d, u + d + w + d + w, v + d + h},
	}
	var out [6]mquad
	for i, f := range faces {
		uv := [4][2]float32{{f.u2, f.v1}, {f.u1, f.v1}, {f.u1, f.v2}, {f.u2, f.v2}}
		for k := 0; k < 4; k++ {
			c := p[f.c[k]]
			out[i][k] = mvert{c[0], c[1], c[2], uv[k][0] / texW, uv[k][1] / texH}
		}
		if b.mirror { // a mirrored box is turned inside out and so its faces are reversed
			out[i][0], out[i][1], out[i][2], out[i][3] = out[i][3], out[i][2], out[i][1], out[i][0]
		}
	}
	return out
}

type mmodel struct {
	parts      []mpart
	texW, texH float32
	quads      [][][6]mquad // per part, per box
}

func newModel(texW, texH float32, parts ...mpart) *mmodel {
	m := &mmodel{parts: parts, texW: texW, texH: texH}
	for _, p := range parts {
		var q [][6]mquad
		for _, b := range p.boxes {
			q = append(q, boxQuads(b, texW, texH))
		}
		m.quads = append(m.quads, q)
	}
	return m
}

func box(u, v, x, y, z, w, h, d float32) mbox {
	return mbox{u: u, v: v, x: x, y: y, z: z, w: w, h: h, d: d}
}
func boxX(u, v, x, y, z, w, h, d, e float32) mbox {
	return mbox{u: u, v: v, x: x, y: y, z: z, w: w, h: h, d: d, expand: e}
}
func mirrored(b mbox) mbox { b.mirror = true; return b }
func part(px, py, pz float32, boxes ...mbox) mpart {
	return mpart{boxes: boxes, px: px, py: py, pz: pz}
}
func turned(p mpart, rx, ry, rz float32) mpart { p.rx, p.ry, p.rz = rx, ry, rz; return p }

// rot is a part's current angles.
type rot struct {
	x, y, z    float32
	dx, dy, dz float32 // moves the pivot, as a sneaking player's legs and head do
	ox         float32 // a further turn about x applied last, about the pivot: how a cape swings
}

// place turns a model-space point by a part's angles about its pivot, as ModelRenderer.render does:
// translate to the pivot, then z, y and x rotations.
func place(p *mpart, r rot, v mvert) (float32, float32, float32) {
	return placeAt(p, r, v)
}

func placeAt(p *mpart, r rot, v mvert) (float32, float32, float32) {
	x, y, z := v.x, v.y, v.z
	if r.x != 0 {
		c, s := float32(math.Cos(float64(r.x))), float32(math.Sin(float64(r.x)))
		y, z = y*c-z*s, y*s+z*c
	}
	if r.y != 0 {
		c, s := float32(math.Cos(float64(r.y))), float32(math.Sin(float64(r.y)))
		x, z = x*c+z*s, -x*s+z*c
	}
	if r.z != 0 {
		c, s := float32(math.Cos(float64(r.z))), float32(math.Sin(float64(r.z)))
		x, y = x*c-y*s, x*s+y*c
	}
	if r.ox != 0 {
		c, s := float32(math.Cos(float64(r.ox))), float32(math.Sin(float64(r.ox)))
		y, z = y*c-z*s, y*s+z*c
	}
	return x + p.px + r.dx, y + p.py + r.dy, z + p.pz + r.dz
}

// toWorld turns a model-space point into a point in the world around an entity at pos facing yaw
// (degrees): the game's scale of 1/16, y flipped and up from the feet, then the turn about y.
func toWorld(x, y, z, yawDeg float32, pos vec3, tilt float32) vec3 {
	wx, wy, wz := -x/16, 1.5078125-y/16, z/16
	if tilt != 0 { // lying down as a dying mob does: a turn about the front-to-back axis
		c, s := float32(math.Cos(float64(tilt))), float32(math.Sin(float64(tilt)))
		wx, wy = wx*c-wy*s, wx*s+wy*c
	}
	a := float64(180-yawDeg) * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	fx, fy, fz := float64(wx), float64(wy), float64(wz)
	return vec3{pos[0] + fx*c + fz*s, pos[1] + fy, pos[2] - fx*s + fz*c}
}

// emitModel adds a posed model's quads, lit and tinted, to out.
func emitModel(out []vertex, m *mmodel, rots []rot, pos vec3, yawDeg, tilt float32, tint [3]uint8, light uint8) []vertex {
	return emitModelScaled(out, m, rots, nil, pos, yawDeg, tilt, tint, light, 1)
}

// emitModelParts is emitModel for some of the parts: those the mask lists (nil: all of them).
func emitModelParts(out []vertex, m *mmodel, rots []rot, mask []bool, pos vec3, yawDeg, tilt float32, tint [3]uint8, light uint8) []vertex {
	return emitModelScaled(out, m, rots, mask, pos, yawDeg, tilt, tint, light, 1)
}

// m3 is a 3 x 3 turn, row by row.
type m3 [9]float32

// partTurn is the turn place() gives a point for a part's angles: about x, then y, then z, then the extra turn
// about x. Worked out once for a part, it is what every point of the part is multiplied by.
func partTurn(r rot) m3 {
	turn := func(x, y, z float32) (float32, float32, float32) {
		if r.x != 0 {
			c, s := float32(math.Cos(float64(r.x))), float32(math.Sin(float64(r.x)))
			y, z = y*c-z*s, y*s+z*c
		}
		if r.y != 0 {
			c, s := float32(math.Cos(float64(r.y))), float32(math.Sin(float64(r.y)))
			x, z = x*c+z*s, -x*s+z*c
		}
		if r.z != 0 {
			c, s := float32(math.Cos(float64(r.z))), float32(math.Sin(float64(r.z)))
			x, y = x*c-y*s, x*s+y*c
		}
		if r.ox != 0 {
			c, s := float32(math.Cos(float64(r.ox))), float32(math.Sin(float64(r.ox)))
			y, z = y*c-z*s, y*s+z*c
		}
		return x, y, z
	}
	a0, a1, a2 := turn(1, 0, 0)
	b0, b1, b2 := turn(0, 1, 0)
	c0, c1, c2 := turn(0, 0, 1)
	return m3{a0, b0, c0, a1, b1, c1, a2, b2, c2}
}

// emitModelScaled is emitModelParts with the whole model scaled about the entity's feet (a player is
// 0.9375 of the boxes it is made of). The trigonometry is done once per part and once per model, not per point.
func emitModelScaled(out []vertex, m *mmodel, rots []rot, mask []bool, pos vec3, yawDeg, tilt float32, tint [3]uint8, light uint8, scale float64) []vertex {
	ya := float64(180-yawDeg) * math.Pi / 180
	yc, ys := math.Cos(ya), math.Sin(ya)
	tc, ts := float32(1), float32(0)
	if tilt != 0 { // lying down as a dying mob does: a turn about the front-to-back axis
		tc, ts = float32(math.Cos(float64(tilt))), float32(math.Sin(float64(tilt)))
	}
	world := func(x, y, z float32) vec3 {
		wx, wy, wz := -x/16, 1.5078125-y/16, z/16
		if tilt != 0 {
			wx, wy = wx*tc-wy*ts, wx*ts+wy*tc
		}
		fx, fy, fz := float64(wx), float64(wy), float64(wz)
		return vec3{pos[0] + (fx*yc+fz*ys)*scale, pos[1] + fy*scale, pos[2] + (-fx*ys+fz*yc)*scale}
	}
	for pi := range m.parts {
		if mask != nil && !mask[pi] {
			continue
		}
		p := &m.parts[pi]
		r := rot{x: p.rx, y: p.ry, z: p.rz}
		if pi < len(rots) {
			o := rots[pi]
			r.x, r.y, r.z, r.dx, r.dy, r.dz, r.ox = r.x+o.x, r.y+o.y, r.z+o.z, o.dx, o.dy, o.dz, o.ox
		}
		turn := partTurn(r)
		px, py, pz := p.px+r.dx, p.py+r.dy, p.pz+r.dz
		for _, bq := range m.quads[pi] {
			for _, q := range bq {
				var w [4]vec3
				for k := 0; k < 4; k++ {
					v := q[k]
					w[k] = world(turn[0]*v.x+turn[1]*v.y+turn[2]*v.z+px, turn[3]*v.x+turn[4]*v.y+turn[5]*v.z+py, turn[6]*v.x+turn[7]*v.y+turn[8]*v.z+pz)
				}
				n := w[1].sub(w[0]).cross(w[3].sub(w[0])) // the face's normal, for a little shading: squares, so no root
				shade := float32(0.8)
				switch l2 := n[0]*n[0] + n[1]*n[1] + n[2]*n[2]; {
				case n[1] > 0 && n[1]*n[1] > 0.25*l2:
					shade = 1
				case n[1] < 0 && n[1]*n[1] > 0.25*l2:
					shade = 0.5
				case math.Abs(n[0]) > math.Abs(n[2]):
					shade = 0.6
				}
				c := [4]uint8{uint8(float32(tint[0]) * shade), uint8(float32(tint[1]) * shade), uint8(float32(tint[2]) * shade), light}
				for k := 0; k < 4; k++ {
					out = append(out, vertex{float32(w[k][0]), float32(w[k][1]), float32(w[k][2]), q[k].u, q[k].v, c})
				}
			}
		}
	}
	return out
}
