package client

import (
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Fishing: the server spawns the hook as an object (type 90) whose data is the angler's entity id, and moves
// it. The client draws the bobber (a sprite of particles.png that faces the camera) and the line from the
// rod's tip to it, as RenderFish does; with its own hook out, the player's rod shows as cast.

const objFishHook = 90

// hookOf is the fishing hook an entity has out, if any.
func (n *netState) hookOf(eid int32) *netEntity {
	for _, e := range n.ents {
		if e.obj == objFishHook && e.owner == eid {
			return e
		}
	}
	return nil
}

// rotatePitch and rotateYaw are Vec3's: the angle in radians.
func rotatePitch(v [3]float32, a float32) [3]float32 {
	c, s := cos32f(a), sin32f(a)
	return [3]float32{v[0], v[1]*c + v[2]*s, v[2]*c - v[1]*s}
}

func rotateYaw(v [3]float32, a float32) [3]float32 {
	c, s := cos32f(a), sin32f(a)
	return [3]float32{v[0]*c + v[2]*s, v[1], v[2]*c - v[0]*s}
}

// anglerTip is where the line leaves the rod, and the height that goes on top of it: RenderFish's d0 to d3.
// feet is the angler's position, yaw and pitch (degrees) their view, body their body's yaw, swing how far
// the arm is through a swing (0 to 1), thirdPerson whether the player is seen from outside.
func anglerTip(feet vec3, yaw, pitch, body, swing float32, eye float32, sneaking, thirdPerson bool) (tip vec3, extra float64) {
	f8 := sin32f(sqrt32f(swing) * math.Pi)
	v := [3]float32{-0.36, 0.03, 0.35}
	v = rotatePitch(v, -pitch*math.Pi/180)
	v = rotateYaw(v, -yaw*math.Pi/180)
	v = rotateYaw(v, f8*0.5)
	v = rotatePitch(v, -f8*0.7)
	tip = feet.add(vec3{float64(v[0]), float64(v[1]), float64(v[2])})
	extra = float64(eye)
	if thirdPerson {
		f9 := float64(body) * math.Pi / 180
		s, c := math.Sin(f9), math.Cos(f9)
		tip = vec3{feet[0] - c*0.35 - s*0.8, feet[1] + float64(eye) - 0.45, feet[2] - s*0.35 + c*0.8}
		extra = 0
		if sneaking {
			extra = -0.1875
		}
	}
	return tip, extra
}

// linePoints are the 17 points of the line from the hook to the angler, hanging in an arc (RenderFish).
func linePoints(hook, tip vec3, extra float64) [17]vec3 {
	dx := float64(float32(tip[0] - hook[0]))
	dy := float64(float32(tip[1]-(hook[1]+0.25))) + extra
	dz := float64(float32(tip[2] - hook[2]))
	var out [17]vec3
	for l := range out {
		f := float64(l) / 16
		out[l] = vec3{hook[0] + dx*f, hook[1] + dy*(f*f+f)*0.5 + 0.25, hook[2] + dz*f}
	}
	return out
}

// hookPos is where a hook is drawn, between its last tick and this one.
func (e *netEntity) at(partial float32) vec3 {
	return e.prev.add(e.pos.sub(e.prev).scale(float64(partial)))
}

// emitBillboard adds a square that faces the camera: half the side s, the cell (u0,v0)-(u1,v1) of a sheet.
func emitBillboard(out []vertex, c, right, up vec3, s float64, u0, v0, u1, v1 float32, light uint8) []vertex {
	col := [4]uint8{255, 255, 255, light}
	p := func(a, b float64, u, v float32) vertex {
		q := c.add(right.scale(a * s)).add(up.scale(b * s))
		return vertex{float32(q[0]), float32(q[1]), float32(q[2]), u, v, col}
	}
	return append(out, p(-1, -1, u0, v1), p(1, -1, u1, v1), p(1, 1, u1, v0), p(-1, 1, u0, v0))
}

// emitHooks lays out the bobbers: the sprite at column 1, row 2 of particles.png, half a block across.
func (g *game) emitHooks(groups map[string][]vertex) {
	n := g.net
	if n == nil {
		return
	}
	f := look(g.yaw, g.pitch)
	right := f.cross(vec3{0, 1, 0}).norm()
	up := right.cross(f)
	for _, e := range n.ents {
		if e.obj != objFishHook {
			continue
		}
		p := e.at(g.partial)
		l := g.world.Light(int32(math.Floor(p[0])), int32(math.Floor(p[1])), int32(math.Floor(p[2])))
		groups["particles"] = emitBillboard(groups["particles"], p.add(vec3{0, 0.25, 0}), right, up, 0.25, 8.0/128, 16.0/128, 16.0/128, 24.0/128, l)
	}
}

// drawFishLines draws each hook's line to its angler: thin and black.
func (g *game) drawFishLines(mvp *mat4) {
	n := g.net
	if n == nil || len(n.ents) == 0 {
		return
	}
	var v []float32
	for _, e := range n.ents {
		if e.obj != objFishHook {
			continue
		}
		var tip vec3
		var extra float64
		switch {
		case e.owner == n.eid:
			partial := float64(g.partial)
			feet := g.prevFeet.add(g.feet.sub(g.prevFeet).scale(partial))
			swing := float32(0)
			if g.swingTicks > 0 {
				swing = (6 - float32(g.swingTicks) + g.partial) / 6
			}
			eye := float32(eyeHeight)
			if g.sneaking {
				eye -= 0.08
			}
			body := g.prevBodyYaw + float32(math.Mod(float64(g.bodyYaw-g.prevBodyYaw)+540, 360)-180)*g.partial
			tip, extra = anglerTip(feet, g.yawDeg(), g.pitch*180/math.Pi, body, swing, eye, g.sneaking, g.perspective > 0)
		default:
			p := n.remote[e.owner]
			if p == nil {
				continue
			}
			swing := float32(0)
			if p.swing > 0 {
				swing = (6 - float32(p.swing) + g.partial) / 6
			}
			feet := p.prev.add(p.pos.sub(p.prev).scale(float64(g.partial)))
			eye := float32(eyeHeight)
			if p.sneaking {
				eye -= 0.08
			}
			tip, extra = anglerTip(feet, p.yaw, p.pitch, p.yaw, swing, eye, p.sneaking, true)
		}
		pts := linePoints(e.at(g.partial), tip, extra)
		for i := 0; i < 16; i++ {
			a, b := pts[i], pts[i+1]
			v = append(v, float32(a[0]), float32(a[1]), float32(a[2]), float32(b[0]), float32(b[1]), float32(b[2]))
		}
	}
	if len(v) == 0 {
		return
	}
	hd := &g.hud
	gl.UseProgram(hd.line)
	gl.UniformMatrix4fv(hd.uMVP, 1, false, &mvp[0])
	gl.Uniform4f(hd.uLineCol, 0, 0, 0, 1)
	gl.BindVertexArray(hd.lvao)
	gl.BindBuffer(gl.ARRAY_BUFFER, hd.lvbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(v)*4, gl.Ptr(v), gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.LINES, 0, int32(len(v)/3))
}
