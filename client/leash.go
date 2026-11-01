package client

import (
	"math"
)

// Leashes, after 1.8's RenderLiving (the rope) and RenderLeashKnot. Attach Entity (0x1B) with leash 1 ties a mob to a
// holder (a player, or a leash knot on a fence); the rope is two strips of 25 segments that sag between the holder's
// hand and the mob's side, drawn flat in brown. A knot (Spawn Object 77) is a small block of the lead_knot texture.

const objLeashKnot = 77

// leashHolder is what the rope needs to know about the one that holds it: where it is, which way it looks, how high
// its eyes are, and whether it hangs on a wall (a knot: the rope then leaves straight from it).
type leashHolder struct {
	prev, pos      vec3
	yawA, yawB     float64 // rotationYaw a tick ago and now, degrees
	pitchA, pitchB float64
	eye            float64
	hanging        bool
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// leashColour is a segment's colour: 0.5, 0.4, 0.3, and 0.7 of that for the even ones.
func leashColour(j int) [4]uint8 {
	f, f1, f2 := float32(0.5), float32(0.4), float32(0.3)
	if j%2 == 0 {
		f, f1, f2 = f*0.7, f1*0.7, f2*0.7
	}
	return [4]uint8{uint8(f * 255), uint8(f1 * 255), uint8(f2 * 255), 255}
}

// leashQuads adds the rope between a mob and its holder: the mob at (mx, my, mz) with its width, height and body
// yaw (degrees), the holder as above, partial the fraction of a tick.
func leashQuads(out []vertex, mx, my, mz, width, height, bodyYaw float64, h leashHolder, partial float64) []vertex {
	y := my - (1.6-height)*0.5
	d0 := lerp(h.yawA, h.yawB, partial*0.5) * 0.01745329238474369
	d1 := lerp(h.pitchA, h.pitchB, partial*0.5) * 0.01745329238474369
	d2, d3, d4 := math.Cos(d0), math.Sin(d0), math.Sin(d1)
	if h.hanging {
		d2, d3, d4 = 0, 0, -1
	}
	d5 := math.Cos(d1)
	d6 := lerp(h.prev[0], h.pos[0], partial) - d2*0.7 - d3*0.5*d5
	d7 := lerp(h.prev[1]+h.eye*0.7, h.pos[1]+h.eye*0.7, partial) - d4*0.5 - 0.25
	d8 := lerp(h.prev[2], h.pos[2], partial) - d3*0.7 + d2*0.5*d5
	d9 := bodyYaw*0.01745329238474369 + 1.5707963267948966
	d2 = math.Cos(d9) * width * 0.4
	d3 = math.Sin(d9) * width * 0.4
	x, z := mx+d2, mz+d3 // the rope leaves the mob's side
	d13 := float64(float32(d6 - x))
	d14 := float64(float32(d7 - my))
	d15 := float64(float32(d8 - z))
	at := func(j int, dx, dy, dz float64) (float32, float32, float32) {
		f3 := float64(float32(j) / 24)
		return float32(x + d13*f3 + dx),
			float32(y + d14*(f3*f3+f3)*0.5 + float64((24-float32(j))/18+0.125) + dy),
			float32(z + d15*f3 + dz)
	}
	for pass := 0; pass < 2; pass++ { // two strips, offset so that the rope has some thickness from every side
		var lo, hi [25][3]float32
		for j := 0; j <= 24; j++ {
			if pass == 0 {
				lo[j][0], lo[j][1], lo[j][2] = at(j, 0, 0, 0)
				hi[j][0], hi[j][1], hi[j][2] = at(j, 0.025, 0.025, 0)
			} else {
				lo[j][0], lo[j][1], lo[j][2] = at(j, 0, 0.025, 0)
				hi[j][0], hi[j][1], hi[j][2] = at(j, 0.025, 0, 0.025)
			}
		}
		for j := 0; j < 24; j++ { // a triangle strip of pairs, as quads: each pair has its own segment's colour
			c0, c1 := leashColour(j), leashColour(j+1)
			for k, p := range [4][3]float32{lo[j], hi[j], hi[j+1], lo[j+1]} {
				c := c0
				if k >= 2 {
					c = c1
				}
				out = append(out, vertex{p[0], p[1], p[2], 0.5, 0.5, c})
			}
		}
	}
	return out
}

// leashHolderOf is what holds the rope of a mob: the player themself, another player, or a knot.
func (g *game) leashHolderOf(eid int32) (leashHolder, bool) {
	n := g.net
	switch {
	case eid == n.eid:
		eye := 1.62
		return leashHolder{prev: g.prevFeet, pos: g.feet, yawA: float64(g.yawDeg()), yawB: float64(g.yawDeg()),
			pitchA: float64(g.pitch) * 180 / math.Pi, pitchB: float64(g.pitch) * 180 / math.Pi, eye: eye}, true
	case n.remote[eid] != nil:
		p := n.remote[eid]
		eye := 1.62
		if p.sneaking {
			eye = 1.54
		}
		return leashHolder{prev: p.prev, pos: p.pos, yawA: float64(p.prevYaw), yawB: float64(p.yaw),
			pitchA: float64(p.prevPitch), pitchB: float64(p.pitch), eye: eye}, true
	}
	if e := n.entity(eid); e != nil && e.obj == objLeashKnot {
		return leashHolder{prev: e.pos, pos: e.pos, eye: -0.0625, hanging: true}, true
	}
	return leashHolder{}, false
}

// emitLeashes adds the ropes of the mobs a server leashed, and the knots.
func (g *game) emitLeashes(groups map[string][]vertex) {
	if g.net == nil {
		return
	}
	partial := float64(g.partial)
	for _, e := range g.net.ents {
		switch {
		case e.mob != nil && e.leash != 0:
			h, ok := g.leashHolderOf(e.leash)
			if !ok || g.offScreen(e.pos, 12) {
				continue
			}
			m := e.mob.m
			w, ht := e.mob.dims()
			bodyYaw := float64(m.prevYaw) + float64(wrapDeg(m.yaw-m.prevYaw))*partial
			groups["!leash"] = leashQuads(groups["!leash"], lerp(m.prev[0], m.pos[0], partial), lerp(m.prev[1], m.pos[1], partial),
				lerp(m.prev[2], m.pos[2], partial), float64(w), float64(ht), bodyYaw, h, partial)
		case e.obj == objLeashKnot && !g.offScreen(e.pos, 2):
			l := g.world.Light(int32(math.Floor(e.pos[0])), int32(math.Floor(e.pos[1])), int32(math.Floor(e.pos[2])))
			at := vec3{e.pos[0], e.pos[1] - 1.5078125, e.pos[2]}
			groups["lead_knot"] = emitModelScaled(groups["lead_knot"], knotModel, []rot{{}}, nil, at, 180, 0, [3]uint8{255, 255, 255}, l, 1)
		}
	}
}

// knotModel is ModelLeashKnot: one box of 6 x 8 x 6 on a 32 x 32 sheet, drawn with the renderer's (-1, -1, 1) scale.
var knotModel = newModel(32, 32, part(0, 0, 0, box(0, 0, -3, -6, -3, 6, 8, 6)))
