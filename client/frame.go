package client

import "math"

// Item frames, after 1.8's EntityItemFrame and RenderItemFrame. Spawn Object (71) gives the block the frame
// hangs in (the packet's position over 32, floored) and which way it faces; its metadata gives the item (entry 8)
// and how far it is turned, in eighths of a turn (entry 9). The frame is the block model item_frame (or
// item_frame_map for a map), turned about the block's middle; the item stands 0.4375 in front of that middle,
// at half size, turned about z by the rotation, and then drawn at half size again by RenderItem.

const (
	objItemFrame = 71
	itemMap      = 358 // filled_map: the frame's other model, with a map where the item is
)

type frameState struct {
	hang   [3]int32
	facing int // EnumFacing's horizontal index
	item   slot
	rot    int
}

func (f *frameState) center() vec3 { return hangCenter(f.hang, f.facing, 12, 12) }

func (f *frameState) box(c vec3) aabb { return hangBox(c, f.facing, 12, 12) }

// apply takes the item and its turn from the metadata.
func (f *frameState) apply(m map[int]any) {
	if s, ok := m[8].(slot); ok {
		f.item = s
	}
	if v, ok := m[9].(int); ok {
		f.rot = v
	}
}

// emitFrame adds an item frame and what is in it.
func (g *game) emitFrame(groups map[string][]vertex, e *netEntity) {
	f := e.frame
	variant := "normal"
	if f.item.ID == itemMap {
		variant = "map"
	}
	vs := g.res.pick("item_frame", variant)
	if len(vs) == 0 || len(vs[0].quads) == 0 {
		return
	}
	cx, cy, cz := float64(f.hang[0])+0.5, float64(f.hang[1])+0.5, float64(f.hang[2])+0.5
	a := float64(180-f.facing*90) * math.Pi / 180
	ac, as := math.Cos(a), math.Sin(a)
	l := g.world.Light(f.hang[0], f.hang[1], f.hang[2])
	for i := range vs[0].quads {
		q := &vs[0].quads[i]
		n := dirVec[q.dir]
		s := entityShade(float64(n[0])*ac+float64(n[2])*as, float64(n[1]), -float64(n[0])*as+float64(n[2])*ac)
		col := [4]uint8{uint8(255 * s), uint8(255 * s), uint8(255 * s), l}
		for k := 0; k < 4; k++ { // the model is the block's: turned about its middle
			px, py, pz := float64(q.apos[k][0])-0.5, float64(q.apos[k][1])-0.5, float64(q.apos[k][2])-0.5
			groups["!blocks"] = append(groups["!blocks"], vertex{
				float32(cx + px*ac + pz*as), float32(cy + py), float32(cz - px*as + pz*ac), q.auv[k][0], q.auv[k][1], col})
		}
	}
	it := f.item
	if it.ID == itemMap { // a map is drawn flat over the frame
		g.emitMapInFrame(groups, f, l)
		return
	}
	if it.empty() {
		return
	}
	flat := !it.isBlock() || flatBlocks[it.ID] // RenderItemFrame turns what is not drawn as a block about y
	place := func(display mat4) mat4 {
		m := rotYM(float32(180-f.facing*90)).then(translateM(0, 0, 0.4375), rotZM(float32(f.rot)*360/8), scaleM(0.5, 0.5, 0.5))
		if flat {
			m = m.then(rotYM(180))
		}
		return m.then(display, scaleM(0.5, 0.5, 0.5))
	}
	g.emitHeldWith(groups, it, place, vec3{cx, cy, cz}, false, 0, false, l, false, true)
}
