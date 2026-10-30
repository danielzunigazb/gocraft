package client

import (
	"math"
	"sort"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// updateLighting sets the sky, the fog, the lightmap and the dynamic lights for this frame.
func (g *game) updateLighting() {
	angle := g.dimAngle()
	sun := sunBrightness(angle)
	fog := g.dimFogColour(angle)
	gl.ClearColor(fog[0], fog[1], fog[2], 1)
	gl.Uniform3f(g.uFogC, fog[0], fog[1], fog[2])

	// The lightmap changes with the sun, the brightness setting and the flicker: a 768 byte upload.
	var lm [768]uint8
	g.buildLightmapDim(sun, &lm)
	if lm != g.lightmap {
		g.lightmap = lm
		gl.ActiveTexture(gl.TEXTURE1)
		gl.BindTexture(gl.TEXTURE_2D, g.lightTex)
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, 16, 16, gl.RGB, gl.UNSIGNED_BYTE, gl.Ptr(&lm[0]))
		gl.ActiveTexture(gl.TEXTURE0)
	}

	lights := g.dynamicLights()
	gl.Uniform1i(g.uDynCount, int32(len(lights)))
	if len(lights) > 0 {
		buf := lights // the slice's own memory holds only floats
		gl.Uniform4fv(g.uDyn, int32(len(buf)), &buf[0][0])
	}
}

// dynamicLights are the light sources that move: what the player holds and light-giving items
// lying about, the nearest sixteen to the camera. A light spreads as far as its level, dimming by
// a step per block, and does not stop at walls, as in the dynamic lights mods of Java Edition.
func (g *game) dynamicLights() [][4]float32 {
	if !g.opts.DynamicLights {
		return nil
	}
	var out [][4]float32
	if l := lightOf(g.held()); l > 0 {
		out = append(out, [4]float32{float32(g.feet[0]), float32(g.feet[1] + 1.1), float32(g.feet[2]), float32(l)})
	}
	for _, e := range g.items {
		if l := lightOf(e.stack); l > 0 {
			p := e.prev.add(e.pos.sub(e.prev).scale(float64(g.partial)))
			out = append(out, [4]float32{float32(p[0]), float32(p[1] + 0.2), float32(p[2]), float32(l)})
		}
	}
	if len(out) > 16 {
		eye := g.pos
		sort.Slice(out, func(i, j int) bool {
			di := math.Hypot(float64(out[i][0])-eye[0], float64(out[i][2])-eye[2])
			dj := math.Hypot(float64(out[j][0])-eye[0], float64(out[j][2])-eye[2])
			return di < dj
		})
		out = out[:16]
	}
	return out
}

// drawSky fills the window with the sky before anything else is drawn.
func (g *game) drawSky(fovY float32, w, h int, f vec3) {
	if g.skyProg == 0 {
		var err error
		if g.skyProg, err = program(skyVS, skyFS); err != nil {
			logf("sky shader: %v", err)
			return
		}
		gl.GenVertexArrays(1, &g.skyVAO)
	}
	angle := g.dimAngle()
	top, horizon := g.dimSkyColours(angle)
	right := f.cross(vec3{0, 1, 0}).norm()
	up := right.cross(f)
	th := float32(math.Tan(float64(fovY) / 2))
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.UseProgram(g.skyProg)
	set3 := func(name string, v [3]float32) { gl.Uniform3f(uniform(g.skyProg, name), v[0], v[1], v[2]) }
	set3("forward", f.f32())
	set3("right", right.f32())
	set3("up", up.f32())
	set3("skyTop", top)
	set3("skyHorizon", horizon)
	gl.Uniform2f(uniform(g.skyProg, "tanFov"), th*float32(w)/float32(h), th)
	gl.BindVertexArray(g.skyVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	if g.dimn.id == dimEnd {
		g.drawEndSky(fovY, w, h, f)
	}
	gl.Enable(gl.DEPTH_TEST)
	gl.Enable(gl.CULL_FACE)
}
