package client

import (
	"math"
	"math/rand"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Dropped items, after 1.8's EntityItem: a 0.25 box that falls with gravity 0.04, bounces a little,
// merges with its neighbours, is picked up by a player within one block after a short delay and
// disappears after five minutes.

const (
	itemLifespan    = 6000
	dropPickupDelay = 10
)

var rng = rand.New(rand.NewSource(1))

type itemEntity struct {
	stack       slot
	pos         vec3 // centre of the base of the box
	prev, vel   vec3
	age         int
	pickupDelay int
	onGround    bool
	hover       float32 // a random phase for the bobbing
}

func itemBox(p vec3) aabb {
	return aabb{vec3{p[0] - 0.125, p[1], p[2] - 0.125}, vec3{p[0] + 0.125, p[1] + 0.25, p[2] + 0.125}}
}

// spawnDrop makes an item entity where a block broke: somewhere in its middle half, thrown up a
// little and sideways a little, as Block.spawnAsEntity does.
func (g *game) spawnDrop(bx, by, bz float64, s slot) {
	if s.empty() {
		return
	}
	e := &itemEntity{stack: s, pickupDelay: dropPickupDelay, hover: rng.Float32() * math.Pi * 2}
	e.pos = vec3{bx + float64(rng.Float32()*0.5) + 0.25, by + float64(rng.Float32()*0.5) + 0.25, bz + float64(rng.Float32()*0.5) + 0.25}
	e.vel = vec3{float64(rng.Float32())*0.2 - 0.1, 0.2, float64(rng.Float32())*0.2 - 0.1}
	e.prev = e.pos
	g.items = append(g.items, e)
}

// throwItem drops a stack from the player, in front of them, with the longer delay of a toss.
func (g *game) throwItem(s slot) {
	if s.empty() {
		return
	}
	look := look(g.yaw, g.pitch)
	e := &itemEntity{stack: s, pickupDelay: 40, hover: rng.Float32() * math.Pi * 2}
	e.pos = vec3{g.feet[0], g.feet[1] + eyeHeight - 0.3, g.feet[2]}
	e.vel = look.scale(0.3)
	e.vel[1] += 0.1
	e.prev = e.pos
	g.items = append(g.items, e)
}

func (g *game) opaqueAt(x, y, z int32) bool {
	id := g.world.Block(x, y, z) >> 4
	return id != 0 && g.res.opaque[id&255]
}

// pushOut is Entity.pushOutOfBlocks: an item whose middle is inside a solid block drifts out by the
// nearest free side. It reports whether it was inside.
func (g *game) pushOut(e *itemEntity) bool {
	cx, cy, cz := e.pos[0], e.pos[1]+0.125, e.pos[2]
	x, y, z := int32(math.Floor(cx)), int32(math.Floor(cy)), int32(math.Floor(cz))
	if !g.opaqueAt(x, y, z) {
		return false
	}
	d0, d1, d2 := cx-float64(x), cy-float64(y), cz-float64(z)
	best, dir := 9999.0, 1
	try := func(free bool, d float64, which int) {
		if free && d < best {
			best, dir = d, which
		}
	}
	try(!g.opaqueAt(x-1, y, z), d0, 0)
	try(!g.opaqueAt(x+1, y, z), 1-d0, 1)
	try(!g.opaqueAt(x, y-1, z), d1, 2)
	try(!g.opaqueAt(x, y+1, z), 1-d1, 3)
	try(!g.opaqueAt(x, y, z-1), d2, 4)
	try(!g.opaqueAt(x, y, z+1), 1-d2, 5)
	const f = 0.1
	switch dir {
	case 0:
		e.vel[0] = -f
	case 1:
		e.vel[0] = f
	case 2:
		e.vel[1] = -f
	case 3:
		e.vel[1] = f
	case 4:
		e.vel[2] = -f
	case 5:
		e.vel[2] = f
	}
	return true
}

// tickItem advances one item by a tick; it reports whether it still exists.
func (g *game) tickItem(e *itemEntity) bool {
	if e.stack.empty() {
		return false
	}
	if e.pickupDelay > 0 {
		e.pickupDelay--
	}
	e.prev = e.pos
	e.vel[1] -= 0.04
	if g.pushOut(e) {
		e.pos = e.pos.add(e.vel) // inside a block it moves without colliding
		e.onGround = false
	} else {
		box, moved := g.slide(itemBox(e.pos), e.vel)
		e.pos = vec3{(box.min[0] + box.max[0]) / 2, box.min[1], (box.min[2] + box.max[2]) / 2}
		e.onGround = moved[1] != e.vel[1] && e.vel[1] < 0
		if moved[0] != e.vel[0] {
			e.vel[0] = 0
		}
		if moved[1] != e.vel[1] {
			e.vel[1] = 0
		}
		if moved[2] != e.vel[2] {
			e.vel[2] = 0
		}
	}
	f := float32(0.98)
	if e.onGround {
		under := g.world.Block(int32(math.Floor(float64(e.pos[0]))), int32(math.Floor(float64(e.pos[1])))-1, int32(math.Floor(float64(e.pos[2])))) >> 4
		f = slipperiness(under) * 0.98
	}
	e.vel[0], e.vel[1], e.vel[2] = e.vel[0]*float64(f), e.vel[1]*dragY, e.vel[2]*float64(f)
	if e.onGround {
		e.vel[1] *= -0.5
	}
	e.age++
	return e.age < itemLifespan
}

// mergeItems joins stacks of the same item lying within half a block of each other.
func (g *game) mergeItems(e *itemEntity) {
	box := itemBox(e.pos)
	box.min = box.min.sub(vec3{0.5, 0, 0.5})
	box.max = box.max.add(vec3{0.5, 0, 0.5})
	for _, o := range g.items {
		if o == e || o.stack.empty() || e.stack.empty() || !o.stack.same(e.stack) || !overlaps(box, itemBox(o.pos)) {
			continue
		}
		if int(o.stack.N)+int(e.stack.N) > int(o.stack.maxStack()) {
			continue
		}
		o.stack.N += e.stack.N
		o.pickupDelay = max(o.pickupDelay, e.pickupDelay)
		o.age = min(o.age, e.age)
		e.stack = slot{}
		return
	}
}

// tickItems moves every dropped item and lets the player pick up those within reach.
func (g *game) tickItems() {
	for _, e := range g.items {
		cell := [3]int32{int32(e.pos[0]), int32(e.pos[1]), int32(e.pos[2])}
		alive := g.tickItem(e)
		moved := cell != [3]int32{int32(e.pos[0]), int32(e.pos[1]), int32(e.pos[2])}
		every := 40
		if moved {
			every = 2
		}
		if alive && e.age%every == 0 {
			g.mergeItems(e)
		}
		if !alive {
			e.stack = slot{}
		}
	}
	reach := playerBox(g.feet)
	reach.min = reach.min.sub(vec3{1, 0.5, 1})
	reach.max = reach.max.add(vec3{1, 0.5, 1})
	live := g.items[:0]
	for _, e := range g.items {
		if !e.stack.empty() && e.pickupDelay == 0 && overlaps(reach, itemBox(e.pos)) {
			before := e.stack.N
			e.stack = g.giveSlot(e.stack)
			if e.stack.N != before {
				g.pickupSound()
			}
		}
		if !e.stack.empty() {
			live = append(live, e)
		}
	}
	g.items = live
}

// entityMesh is a vertex buffer rewritten every frame, for things that move.
type entityMesh struct {
	vao, vbo uint32
	capacity int
}

func (m *entityMesh) draw(verts []vertex) {
	if len(verts) == 0 {
		return
	}
	if m.vao == 0 {
		gl.GenVertexArrays(1, &m.vao)
		gl.GenBuffers(1, &m.vbo)
		gl.BindVertexArray(m.vao)
		gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, quadEBO)
		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, vertexBytes, 0)
		gl.EnableVertexAttribArray(1)
		gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, vertexBytes, 12)
		gl.EnableVertexAttribArray(2)
		gl.VertexAttribPointerWithOffset(2, 4, gl.UNSIGNED_BYTE, true, vertexBytes, 20)
	}
	gl.BindVertexArray(m.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(verts)*vertexBytes, gl.Ptr(verts), gl.STREAM_DRAW)
	rc.entBytes += len(verts) * vertexBytes
	quads := int32(len(verts) / 4)
	for off := int32(0); off < quads; off += maxQuadsPerDraw {
		n := min(maxQuadsPerDraw, quads-off)
		gl.DrawElementsBaseVertex(gl.TRIANGLES, n*6, gl.UNSIGNED_SHORT, nil, off*4) // the shared quad indices are 16-bit
		rc.entDraws++
	}
}

// buildItemGeometry lays out the dropped items for this frame: blocks as small cubes, other items
// as flat pictures, spinning and bobbing as 1.8's do.
func (g *game) buildItemGeometry(partial float32) (blocks, flat []vertex) {
	for _, e := range g.items {
		pos := e.prev.add(e.pos.sub(e.prev).scale(float64(partial)))
		t := float32(e.age) + partial
		bob := float32(math.Sin(float64(t/10+e.hover)))*0.1 + 0.1
		spin := t/20 + e.hover
		sin, cos := float32(math.Sin(float64(spin))), float32(math.Cos(float64(spin)))
		l := g.world.Light(int32(math.Floor(float64(pos[0]))), int32(math.Floor(float64(pos[1]+0.2))), int32(math.Floor(float64(pos[2]))))
		centre := vec3{pos[0], pos[1] + float64(bob) + 0.25, pos[2]}
		rot := func(x, y, z float32) (float32, float32, float32) { // turn about the vertical axis through the centre
			return float32(centre[0] + float64(x*cos-z*sin)), float32(centre[1] + float64(y)), float32(centre[2] + float64(x*sin+z*cos))
		}
		copies := min(int(e.stack.N+15)/16, 4) // a bigger stack is drawn as a few items
		for c := 0; c < max(copies, 1); c++ {
			off := float32(c) * 0.08
			if e.stack.isBlock() {
				vs := g.res.fast[int(e.stack.state())&4095].variants
				if len(vs) == 0 {
					continue
				}
				tint := white
				for i := range vs[0].quads {
					q := &vs[0].quads[i]
					if q.tint >= 0 {
						tint = g.res.tintFor(e.stack.ID, int(e.stack.Dmg), 1)
					}
					col := [4]uint8{toByte(tint[0] * q.shade), toByte(tint[1] * q.shade), toByte(tint[2] * q.shade), l}
					for v := 0; v < 4; v++ {
						x, y, z := rot((q.apos[v][0]-0.5)*0.25+off, (q.apos[v][1]-0.5)*0.25, (q.apos[v][2]-0.5)*0.25+off)
						blocks = append(blocks, vertex{x, y, z, q.auv[v][0], q.auv[v][1], col})
					}
				}
				continue
			}
			tile, ok := g.res.itemTile(e.stack)
			if !ok {
				continue
			}
			u0, v0 := g.res.items.coord(tile, 0, 0)
			u1, v1 := g.res.items.coord(tile, 16, 16)
			col := [4]uint8{255, 255, 255, l}
			const half = 0.25
			corner := func(x, y, u, v float32) vertex {
				px, py, pz := rot(x+off, y, off)
				return vertex{px, py, pz, u, v, col}
			}
			// Both sides, so the picture shows from behind as well.
			flat = append(flat, corner(-half, -half, u0, v1), corner(half, -half, u1, v1), corner(half, half, u1, v0), corner(-half, half, u0, v0))
			flat = append(flat, corner(half, -half, u1, v1), corner(-half, -half, u0, v1), corner(-half, half, u0, v0), corner(half, half, u1, v0))
		}
	}
	return blocks, flat
}
