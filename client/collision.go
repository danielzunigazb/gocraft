package client

import "math"

// What a player collides with and what a ray hits, block by block, after 1.8's own boxes: models
// give most of them, a handful of blocks have the boxes their code sets (soul sand is 7/8 high,
// fences stand 1.5 tall for collision and 1 for the selection).

type box6 = [6]float32

func full() box6 { return box6{0, 0, 0, 1, 1, 1} }

// box6lo and box6hi are the corners of a block-unit box as doubles: the game's block bounds are floats
// (setBlockBounds(0.375F, ...)) widened to double, which is what the conversion does.
func box6lo(b box6) vec3 { return vec3{float64(b[0]), float64(b[1]), float64(b[2])} }
func box6hi(b box6) vec3 { return vec3{float64(b[3]), float64(b[4]), float64(b[5])} }

// connections of a fence, wall or pane to its four sides, in the order north, east, south, west.
func (r *resources) connections(id uint16, nb nbFunc) (c [4]bool) {
	steps := [4][2]int32{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for i, s := range steps {
		o := nb(s[0], 0, s[1]) >> 4
		switch id {
		case 102, 160, 101: // panes and bars join panes, bars, glass and solid blocks
			c[i] = o == 102 || o == 160 || o == 101 || o == 20 || o == 95 || (o != 0 && r.opaque[o&255])
		case 139:
			c[i] = o == 139 || o == 107 || (o >= 183 && o <= 187) || (o != 0 && r.opaque[o&255])
		default: // fences
			c[i] = o == id || o == 107 || (o >= 183 && o <= 187) || (o != 0 && r.opaque[o&255])
		}
	}
	return c
}

var fenceIDs = map[uint16]bool{85: true, 188: true, 189: true, 190: true, 191: true, 192: true, 113: true}

// collisionBoxes are the boxes of a block state a player stops against, in block units.
func (r *resources) collisionBoxes(st uint16, nb nbFunc) []box6 {
	id := st >> 4
	if id == 78 { // snow layers: BlockSnow's box is (layers-1)/8 high, so one layer has none. noCollide lists the id for collision(), which the model box answers
		if h := float32(st&7) * 0.125; h > 0 {
			return []box6{{0, 0, 0, 1, h, 1}}
		}
		return nil
	}
	if id == 0 || isFluid(id) || noCollide[id] {
		return nil
	}
	switch {
	case id == 88: // soul sand
		return []box6{{0, 0, 0, 1, 0.875, 1}}
	case id == 60: // farmland collides as a whole block in 1.8
		return []box6{full()}
	case id == 81: // cactus
		return []box6{{0.0625, 0, 0.0625, 0.9375, 0.9375, 0.9375}}
	case id == 54 || id == 146 || id == 130: // chests
		return []box6{{0.0625, 0, 0.0625, 0.9375, 0.875, 0.9375}}
	case id == 26: // bed
		return []box6{{0, 0, 0, 1, 0.5625, 1}}
	case id == 107 || (id >= 183 && id <= 187): // fence gates: open ones are walked through
		if st&4 != 0 {
			return nil
		}
		if st&1 == 0 { // facing south or north: the gate runs along x
			return []box6{{0, 0, 0.375, 1, 1.5, 0.625}}
		}
		return []box6{{0.375, 0, 0, 0.625, 1.5, 1}}
	case fenceIDs[id]:
		c := r.connections(id, nb)
		out := []box6{{0.375, 0, 0.375, 0.625, 1.5, 0.625}}
		if c[0] {
			out = append(out, box6{0.375, 0, 0, 0.625, 1.5, 0.375})
		}
		if c[1] {
			out = append(out, box6{0.625, 0, 0.375, 1, 1.5, 0.625})
		}
		if c[2] {
			out = append(out, box6{0.375, 0, 0.625, 0.625, 1.5, 1})
		}
		if c[3] {
			out = append(out, box6{0, 0, 0.375, 0.375, 1.5, 0.625})
		}
		return out
	case id == 139: // cobblestone walls
		c := r.connections(id, nb)
		n, e, s, w := c[0], c[1], c[2], c[3]
		lo, hi := float32(0.25), float32(0.75)
		x0, x1, z0, z1 := lo, hi, lo, hi
		if n {
			z0 = 0
		}
		if s {
			z1 = 1
		}
		if w {
			x0 = 0
		}
		if e {
			x1 = 1
		}
		if n && s && !e && !w { // a straight wall is thinner
			x0, x1 = 0.3125, 0.6875
		} else if e && w && !n && !s {
			z0, z1 = 0.3125, 0.6875
		}
		return []box6{{x0, 0, z0, x1, 1.5, z1}}
	case id == 102 || id == 160 || id == 101: // panes and bars
		c := r.connections(id, nb)
		out := []box6{}
		if c[0] || c[2] {
			z0, z1 := float32(0.4375), float32(0.5625)
			if c[0] {
				z0 = 0
			} else {
				z0 = 0.4375
			}
			if c[2] {
				z1 = 1
			}
			out = append(out, box6{0.4375, 0, z0, 0.5625, 1, z1})
		}
		if c[1] || c[3] {
			x0, x1 := float32(0.4375), float32(0.5625)
			if c[3] {
				x0 = 0
			}
			if c[1] {
				x1 = 1
			}
			out = append(out, box6{x0, 0, 0.4375, x1, 1, 0.5625})
		}
		if len(out) == 0 {
			out = append(out, box6{0.4375, 0, 0.4375, 0.5625, 1, 0.5625})
		}
		return out
	}
	file, props := r.stateOf(id, int(st&15), nb)
	if vs := r.pick(file, props); len(vs) > 0 && len(vs[0].boxes) > 0 {
		return vs[0].boxes
	}
	return []box6{full()}
}

// blockBoxes returns the collision boxes of the block at x, y, z in world coordinates, appended to out.
func (g *game) blockBoxes(x, y, z int32, out []aabb) []aabb {
	st := g.world.Block(x, y, z)
	if st == 0 {
		return out
	}
	var nb nbFunc
	if id := st >> 4; neighbourDependent[id] {
		nb = func(dx, dy, dz int32) uint16 { return g.world.Block(x+dx, y+dy, z+dz) }
	}
	o := vec3{float64(x), float64(y), float64(z)}
	for _, b := range g.res.collisionBoxes(st, nb) {
		// The block's bounds are floats in the game too (setBlockBounds(0.375F, ...)), widened to doubles.
		out = append(out, aabb{o.add(box6lo(b)), o.add(box6hi(b))})
	}
	return out
}

// plantBounds are the selection boxes of plants, which are smaller than their models.
var plantBounds = map[uint16]box6{
	6:   {0.1, 0, 0.1, 0.9, 0.8, 0.9}, // sapling
	31:  {0.1, 0, 0.1, 0.9, 0.8, 0.9}, // tall grass
	32:  {0.1, 0, 0.1, 0.9, 0.8, 0.9}, // dead bush
	37:  {0.3, 0, 0.3, 0.7, 0.6, 0.7}, // flowers
	38:  {0.3, 0, 0.3, 0.7, 0.6, 0.7},
	39:  {0.3, 0, 0.3, 0.7, 0.4, 0.7}, // mushrooms
	40:  {0.3, 0, 0.3, 0.7, 0.4, 0.7},
	83:  {0.125, 0, 0.125, 0.875, 1, 0.875}, // sugar cane
	111: {0.0625, 0, 0.0625, 0.9375, 0.015625, 0.9375},
	175: {0.1, 0, 0.1, 0.9, 1, 0.9},
}

// selectionBoxes are what a ray can hit in a block: its own boxes for stairs and the like, the
// hull of the model for most, the smaller bounds of plants, and for a fence the hull of post and
// arms one block high.
func (r *resources) selectionBoxes(st uint16, nb nbFunc) []box6 {
	id := st >> 4
	if id == 0 || isFluid(id) {
		return nil
	}
	if b, ok := plantBounds[id]; ok {
		return []box6{b}
	}
	if stairIDs[id] || id == 44 || id == 126 || id == 182 {
		return r.collisionBoxes(st, nb)
	}
	var boxes []box6
	switch {
	case id == 60: // farmland: selection is the 15/16 box
		return []box6{{0, 0, 0, 1, 0.9375, 1}}
	case id == 88:
		return []box6{{0, 0, 0, 1, 1, 1}}
	case id == 78:
		return []box6{{0, 0, 0, 1, float32(st&7+1) * 0.125, 1}}
	case id == 107 || (id >= 183 && id <= 187):
		if st&1 == 0 {
			return []box6{{0, 0, 0.375, 1, 1, 0.625}}
		}
		return []box6{{0.375, 0, 0, 0.625, 1, 1}}
	case fenceIDs[id] || id == 139 || id == 102 || id == 160 || id == 101:
		boxes = r.collisionBoxes(st, nb)
		out := box6{1, 1, 1, 0, 0, 0}
		for _, b := range boxes {
			for k := 0; k < 3; k++ {
				out[k], out[k+3] = min(out[k], b[k]), max(out[k+3], b[k+3])
			}
		}
		if id != 139 {
			out[4] = 1 // selected one block high even where it collides 1.5
		}
		return []box6{out}
	}
	file, props := r.stateOf(id, int(st&15), nb)
	if vs := r.pick(file, props); len(vs) > 0 && len(vs[0].boxes) > 0 {
		out := box6{1, 1, 1, 0, 0, 0}
		for _, b := range vs[0].boxes {
			for k := 0; k < 3; k++ {
				out[k], out[k+3] = min(out[k], b[k]), max(out[k+3], b[k+3])
			}
		}
		return []box6{out}
	}
	return []box6{full()}
}

// rayHit is where a ray met a block: the cell, the face it entered by and the point.
type rayHit struct {
	pos  [3]int32
	side int // dDown..dEast
	at   vec3
}

// segmentBox intersects a segment with a box (the game's AxisAlignedBB.calculateIntercept): it
// returns the entry point and the face, or false.
func segmentBox(b aabb, a, c vec3) (vec3, int, bool) {
	best, side := math.Inf(1), -1
	var at vec3
	d := c.sub(a)
	for axis := 0; axis < 3; axis++ {
		if d[axis] == 0 {
			continue
		}
		for end := 0; end < 2; end++ {
			plane := b.min[axis]
			if end == 1 {
				plane = b.max[axis]
			}
			t := (plane - a[axis]) / d[axis]
			if t < 0 || t > 1 || t >= best {
				continue
			}
			p := a.add(d.scale(t))
			o1, o2 := (axis+1)%3, (axis+2)%3
			if p[o1] < b.min[o1] || p[o1] > b.max[o1] || p[o2] < b.min[o2] || p[o2] > b.max[o2] {
				continue
			}
			best, at = t, p
			// The face of the block that was hit: the low plane of an axis is its west, down or north face.
			side = [3][2]int{{dWest, dEast}, {dDown, dUp}, {dNorth, dSouth}}[axis][end]
		}
	}
	return at, side, side >= 0
}

// rayTrace walks a ray through the blocks as World.rayTraceBlocks does: cell by cell from the
// start, testing each block's own boxes, and returns the first block hit. Liquids are not hit.
func (g *game) rayTrace(start, end vec3) (rayHit, bool) {
	cell := func(v vec3) [3]int32 {
		return [3]int32{int32(math.Floor(v[0])), int32(math.Floor(v[1])), int32(math.Floor(v[2]))}
	}
	test := func(c [3]int32) (rayHit, bool) {
		st := g.world.Block(c[0], c[1], c[2])
		if st == 0 || isFluid(st>>4) {
			return rayHit{}, false
		}
		var nb nbFunc
		if neighbourDependent[st>>4] {
			nb = func(dx, dy, dz int32) uint16 { return g.world.Block(c[0]+dx, c[1]+dy, c[2]+dz) }
		}
		o := vec3{float64(c[0]), float64(c[1]), float64(c[2])}
		bestDist := math.Inf(1)
		var hit rayHit
		found := false
		for _, b := range g.res.selectionBoxes(st, nb) {
			bx := aabb{o.add(box6lo(b)), o.add(box6hi(b))}
			if at, side, ok := segmentBox(bx, start, end); ok {
				if d := at.sub(start).dot(at.sub(start)); d < bestDist {
					bestDist, hit, found = d, rayHit{c, side, at}, true
				}
			}
		}
		return hit, found
	}

	cur := cell(start)
	if h, ok := test(cur); ok {
		return h, true
	}
	endCell := cell(end)
	for i := 0; i < 200; i++ {
		if cur == endCell {
			return rayHit{}, false
		}
		// Step into the next cell the segment passes through: the nearest of the three planes.
		dir := end.sub(start)
		tMin, axis := math.Inf(1), -1
		for a := 0; a < 3; a++ {
			if dir[a] == 0 {
				continue
			}
			var next float64
			if dir[a] > 0 {
				next = float64(cur[a] + 1)
			} else {
				next = float64(cur[a])
			}
			if t := (next - start[a]) / dir[a]; t < tMin {
				tMin, axis = t, a
			}
		}
		if axis < 0 || tMin > 1 {
			return rayHit{}, false
		}
		if dir[axis] > 0 {
			cur[axis]++
		} else {
			cur[axis]--
		}
		if h, ok := test(cur); ok {
			return h, true
		}
	}
	return rayHit{}, false
}

// outlineBox is what the selection outline draws: the block's bounds, which for stairs is the whole cube.
func (r *resources) outlineBox(st uint16, nb nbFunc) box6 {
	if stairIDs[st>>4] {
		return full()
	}
	out := box6{1, 1, 1, 0, 0, 0}
	boxes := r.selectionBoxes(st, nb)
	if len(boxes) == 0 {
		return full()
	}
	for _, b := range boxes {
		for k := 0; k < 3; k++ {
			out[k], out[k+3] = min(out[k], b[k]), max(out[k+3], b[k+3])
		}
	}
	return out
}
