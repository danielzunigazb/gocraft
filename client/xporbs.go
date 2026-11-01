package client

import (
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Experience orbs, after 1.8's RenderXPOrb: a sprite of experience_orb.png (a four by four sheet, bigger orbs
// for bigger amounts) that faces the camera, 0.3 blocks wide, whose green pulses with its age. A server spawns
// one with Spawn Experience Orb and moves it with the usual entity packets; when a player takes it, Collect Item
// plays a soft chime.

const objOrb = 0xFE // netEntity.obj for an experience orb: not a mob, not clickable, put where the server says

// orbIcon is EntityXPOrb.getTextureByXP: which of the sheet's ten sprites an orb of that value is.
func orbIcon(v int) int {
	for i, from := range []int{2477, 1237, 617, 307, 149, 73, 37, 17, 7, 3} {
		if v >= from {
			return 10 - i
		}
	}
	return 0
}

// orbVertices lays one orb out facing the camera: the quad of RenderXPOrb (half a block wide, from a quarter below
// the middle to three quarters above, scaled by 0.3) with the colour of its age.
func orbVertices(dst []vertex, pos, right, up vec3, count, age int, partial float32, light uint8) []vertex {
	i := orbIcon(count)
	u0, v0 := float32(i%4*16)/64, float32(i/4*16)/64
	u1, v1 := u0+16.0/64, v0+16.0/64
	f := (float32(age) + partial) / 2
	r := uint8((math.Sin(float64(f))+1)*0.5*255 + 0.5)
	b := uint8((math.Sin(float64(f)+4.1887903)+1)*0.1*255 + 0.5)
	c := [4]uint8{r, 255, b, light}
	at := func(x, y float64, u, v float32) vertex {
		p := pos.add(right.scale(x * 0.3)).add(up.scale(y * 0.3))
		return vertex{float32(p[0]), float32(p[1]), float32(p[2]), u, v, c}
	}
	return append(dst, at(-0.5, -0.25, u0, v1), at(0.5, -0.25, u1, v1), at(0.5, 0.75, u1, v0), at(-0.5, 0.75, u0, v0))
}

// drawOrbs draws every orb the server has spawned.
func (g *game) drawOrbs() {
	if g.net == nil {
		return
	}
	tex, ok := g.mobTex["experience_orb"]
	if !ok {
		return
	}
	dir := look(g.yaw, g.pitch)
	right := dir.cross(vec3{0, 1, 0}).norm()
	up := right.cross(dir).norm()
	g.orbVerts = g.orbVerts[:0]
	for _, e := range g.net.ents {
		if e.obj != objOrb || g.offScreen(e.pos, 1) {
			continue
		}
		p := e.at(g.partial)
		l := g.world.Light(int32(math.Floor(p[0])), int32(math.Floor(p[1])), int32(math.Floor(p[2])))
		g.orbVerts = orbVertices(g.orbVerts, p, right, up, e.orb, e.age, g.partial, l)
	}
	if len(g.orbVerts) == 0 {
		return
	}
	gl.Disable(gl.CULL_FACE)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	g.orbMesh.draw(g.orbVerts)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
	gl.Enable(gl.CULL_FACE)
}
