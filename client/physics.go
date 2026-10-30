package client

import (
	"math"
)

// The player's movement follows 1.8's: 20 ticks a second, a 0.6 x 1.8 box, gravity 0.08 with 0.98
// drag, ground friction 0.546, a jump of 0.42, steps up to 0.6 high.

const (
	tickRate   = 20
	playerW    = 0.6
	playerH    = 1.8
	eyeHeight  = 1.62
	stepHeight = 0.6
)

type aabb struct{ min, max vec3 }

func (b aabb) offset(d vec3) aabb { return aabb{b.min.add(d), b.max.add(d)} }

func (b aabb) expand(d vec3) aabb {
	for i := 0; i < 3; i++ {
		if d[i] < 0 {
			b.min[i] += d[i]
		} else {
			b.max[i] += d[i]
		}
	}
	return b
}

// The box is built as Entity.setPosition does: the width and height are floats (0.6F, 1.8F), so the
// half width is 0.3F, which as a double is 0.30000001192..., not 0.3.
var (
	playerHalfW = float64(float32(playerW) / 2)
	playerBoxH  = float64(float32(playerH))
)

func playerBox(feet vec3) aabb {
	return aabb{vec3{feet[0] - playerHalfW, feet[1], feet[2] - playerHalfW}, vec3{feet[0] + playerHalfW, feet[1] + playerBoxH, feet[2] + playerHalfW}}
}

// Blocks a player walks through.
var noCollide = map[uint16]bool{6: true, 27: true, 28: true, 31: true, 32: true, 37: true, 38: true, 39: true, 40: true, 50: true, 51: true,
	55: true, 59: true, 63: true, 66: true, 68: true, 69: true, 70: true, 72: true, 75: true, 76: true, 77: true, 83: true, 90: true, 104: true,
	105: true, 106: true, 111: true, 115: true, 131: true, 132: true, 141: true, 142: true, 143: true, 157: true, 175: true, 8: true, 9: true,
	10: true, 11: true, 78: true}

// collision is the box a block fills, from its model, in block units; ok is false for blocks to walk through.
func (r *resources) collision(state uint16) (b [6]float32, ok bool) {
	if id := state >> 4; id == 0 || noCollide[id] {
		return b, false
	}
	return r.modelBox(state)
}

// modelBox is the bounds of a block's model, in block units; blocks without a model are full cubes.
func (r *resources) modelBox(state uint16) (b [6]float32, ok bool) {
	id := state >> 4
	if id == 0 {
		return b, false
	}
	if v, hit := r.boxes[state]; hit {
		return v.box, v.ok
	}
	none := func(int32, int32, int32) uint16 { return 0 }
	file, props := r.stateOf(id, int(state&15), none)
	cached := cachedBox{box: [6]float32{0, 0, 0, 1, 1, 1}, ok: true} // blocks without a model are full cubes
	if vs := r.pick(file, props); len(vs) > 0 && len(vs[0].quads) > 0 {
		lo, hi := [3]float32{16, 16, 16}, [3]float32{}
		for _, q := range vs[0].quads {
			for _, v := range q.pos {
				for k := 0; k < 3; k++ {
					lo[k], hi[k] = min(lo[k], v[k]), max(hi[k], v[k])
				}
			}
		}
		cached.box = [6]float32{lo[0] / 16, lo[1] / 16, lo[2] / 16, hi[0] / 16, min(hi[1], 16) / 16, hi[2] / 16}
	}
	r.boxes[state] = cached
	return cached.box, cached.ok
}

type cachedBox struct {
	box [6]float32
	ok  bool
}

// boxesIn lists the block boxes touching a region.
func (g *game) boxesIn(region aabb) []aabb {
	var out []aabb
	x0, x1 := int32(math.Floor(region.min[0])), int32(math.Floor(region.max[0]))
	// One cell lower than the box: fences and walls are 1.5 high, so their boxes reach up into the cell above.
	y0, y1 := int32(math.Floor(region.min[1]))-1, int32(math.Floor(region.max[1]))
	z0, z1 := int32(math.Floor(region.min[2])), int32(math.Floor(region.max[2]))
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			for y := y0; y <= y1; y++ {
				out = g.blockBoxes(x, y, z, out)
			}
		}
	}
	return out
}

// clip shortens a move along one axis so the box stops against the others: AxisAlignedBB.calculateXOffset
// and its Y and Z twins, without any tolerance, as the game has it. Positions are doubles, so a box that
// touches a block touches it exactly and never slips into it by a hair.
func clip(box aabb, obstacles []aabb, axis int, d float64) float64 {
	a, b := (axis+1)%3, (axis+2)%3
	for _, o := range obstacles {
		if !(box.max[a] > o.min[a] && box.min[a] < o.max[a] && box.max[b] > o.min[b] && box.min[b] < o.max[b]) {
			continue
		}
		if d > 0 && box.max[axis] <= o.min[axis] {
			if d1 := o.min[axis] - box.max[axis]; d1 < d {
				d = d1
			}
		} else if d < 0 && box.min[axis] >= o.max[axis] {
			if d0 := o.max[axis] - box.min[axis]; d0 > d {
				d = d0
			}
		}
	}
	return d
}

// slide moves a box by d, stopping against blocks: y first, then x, then z, as the game does.
func (g *game) slide(box aabb, d vec3) (aabb, vec3) {
	obs := g.boxesIn(box.expand(d))
	moved := d
	for _, axis := range [3]int{1, 0, 2} {
		moved[axis] = clip(box, obs, axis, d[axis])
		var step vec3
		step[axis] = moved[axis]
		box = box.offset(step)
	}
	return box, moved
}

type input struct {
	forward, strafe float32
	jump, sneak     bool
	sprint          bool
}

func (g *game) readInput() input {
	var in input
	if g.keyDown("forward") {
		in.forward++
	}
	if g.keyDown("back") {
		in.forward--
	}
	if g.keyDown("left") {
		in.strafe++
	}
	if g.keyDown("right") {
		in.strafe--
	}
	in.jump, in.sneak, in.sprint = g.keyDown("jump"), g.keyDown("sneak"), g.keyDown("sprint")
	return in
}

// surfaceAt is the feet height standing on the highest block that stops a player in a column, as a
// new world's spawn does: level.dat's spawn height may lie underground.
func (g *game) surfaceAt(x, z int32) (float64, bool) {
	for y := int32(255); y >= 0; y-- {
		st := g.world.Block(x, y, z)
		if _, ok := g.res.collision(st); ok || isFluid(st>>4) {
			return float64(y + 1), true
		}
	}
	return 0, false
}

// unstickTolerance is how far into a block a position given from outside may be and still count as
// standing on it: a typed -pos or a saved file gives feet as eye height minus 1.62, which can come out
// a hair under the surface. It is a rule of finding a free spot, not of the game's movement, which has none.
const unstickTolerance = 1e-6

// freeSpot moves a player standing inside blocks up to the first place where the body fits.
func (g *game) freeSpot(feet vec3) vec3 {
	t := vec3{unstickTolerance, unstickTolerance, unstickTolerance}
	for ; feet[1] < 255; feet[1]++ {
		b := playerBox(feet)
		b.min, b.max = b.min.add(t), b.max.sub(t)
		if !g.anyBlock(b) {
			return g.snapOut(feet)
		}
	}
	return feet
}

// snapOut lifts or pushes a position that sits a hair inside a block (by no more than unstickTolerance)
// out to the surface. The movement code stops a box that touches a block, and has nothing to say about
// one that is already a hair inside it: it would fall through the floor or walk through the wall. A
// horizontal push leaves a nanometre of air so that rebuilding the box from the position cannot put it
// back in by an ulp.
func (g *game) snapOut(feet vec3) vec3 {
	for round := 0; round < 4; round++ {
		b := playerBox(feet)
		moved := false
		for _, o := range g.boxesIn(b) {
			if !overlaps(b, o) {
				continue
			}
			// How far the box is in, along the way out on each of the six sides.
			up, dx0, dx1, dz0, dz1 := o.max[1]-b.min[1], o.max[0]-b.min[0], b.max[0]-o.min[0], o.max[2]-b.min[2], b.max[2]-o.min[2]
			best := min(up, dx0, dx1, dz0, dz1)
			if best > unstickTolerance {
				continue
			}
			switch best {
			case up:
				feet[1] = o.max[1]
			case dx0:
				feet[0] = o.max[0] + playerHalfW + 1e-9
			case dx1:
				feet[0] = o.min[0] - playerHalfW - 1e-9
			case dz0:
				feet[2] = o.max[2] + playerHalfW + 1e-9
			default:
				feet[2] = o.min[2] - playerHalfW - 1e-9
			}
			moved = true
			break
		}
		if !moved {
			break
		}
	}
	return feet
}
