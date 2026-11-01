package client

import (
	"image"

	"github.com/go-gl/gl/v3.3-core/gl"
)

const hudVS = `#version 330 core
layout(location = 0) in vec2 pos;
layout(location = 1) in vec2 uv;
layout(location = 2) in vec4 col;
uniform vec2 screen;
out vec2 vUV;
out vec4 vCol;
void main() {
	gl_Position = vec4(pos.x / screen.x * 2.0 - 1.0, 1.0 - pos.y / screen.y * 2.0, 0.0, 1.0);
	vUV = uv;
	vCol = col;
}`

const hudFS = `#version 330 core
in vec2 vUV;
in vec4 vCol;
uniform sampler2D tex;
out vec4 frag;
void main() {
	vec4 t = texture(tex, vUV);
	if (t.a < 0.05) discard;
	frag = t * vCol;
}`

const lineVS = `#version 330 core
layout(location = 0) in vec3 pos;
uniform mat4 mvp;
void main() { gl_Position = mvp * vec4(pos, 1.0); gl_Position.z -= 0.0005; }`

type hud struct {
	prog, line                                                   uint32
	vao, vbo                                                     uint32
	lvao, lvbo                                                   uint32
	uScreen, uMVP                                                int32
	uLineCol                                                     int32
	widgets, icons                                               uint32
	tabItems, white, font, dirt, logo, itemTex, invTex, tableTex uint32
	items                                                        []float32 // icon quads of the item atlas, queued beside verts
	verts                                                        []float32
}

func uploadTexture(img *image.RGBA) uint32 {
	var t uint32
	gl.GenTextures(1, &t)
	gl.BindTexture(gl.TEXTURE_2D, t)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(img.Rect.Dx()), int32(img.Rect.Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
	return t
}

func (g *game) initHUD() error {
	var err error
	h := &g.hud
	if h.prog, err = program(hudVS, hudFS); err != nil {
		return err
	}
	if h.line, err = program(lineVS, flatFS); err != nil {
		return err
	}
	h.uScreen, h.uMVP, h.uLineCol = uniform(h.prog, "screen"), uniform(h.line, "mvp"), uniform(h.line, "colour")
	h.uploadAll(g)
	px := image.NewRGBA(image.Rect(0, 0, 1, 1))
	px.Pix = []uint8{255, 255, 255, 255}
	h.white = uploadTexture(px)

	gl.GenVertexArrays(1, &h.vao)
	gl.GenBuffers(1, &h.vbo)
	gl.BindVertexArray(h.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, h.vbo)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 32, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, 32, 8)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 4, gl.FLOAT, false, 32, 16)

	gl.GenVertexArrays(1, &h.lvao)
	gl.GenBuffers(1, &h.lvbo)
	gl.BindVertexArray(h.lvao)
	gl.BindBuffer(gl.ARRAY_BUFFER, h.lvbo)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 12, 0)
	return nil
}

// rect queues a textured rectangle: destination in pixels, source in texels of a w x h texture.
func (h *hud) rect(x, y, w, hgt, sx, sy, sw, sh, texW, texH float32, col [4]float32) {
	u0, v0, u1, v1 := sx/texW, sy/texH, (sx+sw)/texW, (sy+sh)/texH
	q := [4][4]float32{{x, y, u0, v0}, {x, y + hgt, u0, v1}, {x + w, y + hgt, u1, v1}, {x + w, y, u1, v0}}
	for _, i := range [6]int{0, 1, 2, 0, 2, 3} {
		h.verts = append(h.verts, q[i][0], q[i][1], q[i][2], q[i][3], col[0], col[1], col[2], col[3])
	}
}

func (h *hud) flush(tex uint32) {
	if len(h.verts) == 0 {
		return
	}
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.BindVertexArray(h.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, h.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(h.verts)*4, gl.Ptr(h.verts), gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, int32(len(h.verts)/8))
	h.verts = h.verts[:0]
}

// blockIcon is the atlas tile and tint a hotbar slot shows for a block: its top face.
// flushItems draws the queued item pictures with the item atlas.
func (h *hud) flushItems() {
	swap := h.verts
	h.verts = h.items
	h.flush(h.itemTex)
	h.items = h.verts[:0]
	h.verts = swap
}

// drawSlotIcon queues the picture of a stack: a block's top face from the block atlas, an item's
// picture from the item atlas.
func (g *game) drawSlotIcon(s slot, x, y, size float32) {
	if s.empty() {
		return
	}
	if s.isBlock() {
		g.iconQuads(s.state(), x, y, size)
		return
	}
	tile, ok := g.res.itemTile(s)
	if !ok {
		return
	}
	hd := &g.hud
	u0, v0 := g.res.items.coord(tile, 0, 0)
	u1, v1 := g.res.items.coord(tile, 16, 16)
	q := [4][4]float32{{x, y, u0, v0}, {x, y + size, u0, v1}, {x + size, y + size, u1, v1}, {x + size, y, u1, v0}}
	for _, k := range [6]int{0, 1, 2, 0, 2, 3} {
		hd.items = append(hd.items, q[k][0], q[k][1], q[k][2], q[k][3], 1, 1, 1, 1)
	}
}

func (g *game) blockIcon(state uint16) (tile int, tint [3]float32, ok bool) {
	id := state >> 4
	file, props := g.res.stateOf(id, int(state&15), func(int32, int32, int32) uint16 { return 0 })
	vs := g.res.pick(file, props)
	if len(vs) == 0 || len(vs[0].quads) == 0 {
		if t, ok := g.res.iconFallback[id]; ok { // a block drawn by code in the game: a stand-in picture
			return t, white, true
		}
		return 0, white, false
	}
	q := vs[0].quads[0]
	for _, c := range vs[0].quads {
		if c.dir == dUp {
			q = c
			break
		}
	}
	tint = white
	if q.tint >= 0 {
		tint = g.res.tintFor(id, int(state&15), 1)
	}
	return q.tile, tint, true
}

// guiScale is 1.8's automatic scale: the largest whole factor that leaves 320 x 240 of room.
func autoGuiScale(w, h int) int { return max(1, min(w/320, h/240)) }

func (g *game) drawHUD(w, h int) {
	hd := &g.hud
	s := float32(g.scaleFor(w, h))
	gl.Disable(gl.DEPTH_TEST)
	gl.UseProgram(hd.prog)
	gl.Uniform2f(hd.uScreen, float32(w), float32(h))
	gl.ActiveTexture(gl.TEXTURE0)
	one := [4]float32{1, 1, 1, 1}

	// Crosshair, inverted against what is behind it as the game does.
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(gl.ONE_MINUS_DST_COLOR, gl.ONE_MINUS_SRC_COLOR, gl.ONE, gl.ZERO)
	hd.rect(float32(w)/2-8*s, float32(h)/2-8*s, 16*s, 16*s, 0, 0, 16, 16, 256, 256, one)
	hd.flush(hd.icons)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Hotbar: a 182 x 22 bar, the selector 24 x 24 over the chosen slot.
	x0, y0 := float32(w)/2-91*s, float32(h)-22*s
	hd.rect(x0, y0, 182*s, 22*s, 0, 0, 182, 22, 256, 256, one)
	hd.rect(x0-s+float32(g.sel)*20*s, y0-s, 24*s, 24*s, 0, 22, 24, 24, 256, 256, one)
	hd.flush(hd.widgets)
	if !g.creative {
		g.drawStatus(s, x0, y0)
	}

	for i := 0; i < 9; i++ {
		g.drawSlotIcon(g.inv[i], x0+(3+float32(i)*20)*s, y0+3*s, 16*s)
	}
	hd.flush(g.tex)
	hd.flushItems()
	for i := 0; i < 9; i++ {
		g.drawCount(int(g.inv[i].N), x0+(3+float32(i)*20)*s, y0+3*s, s)
	}
	gl.Disable(gl.BLEND)
	if g.screen == screenInventory {
		g.drawInventory(w, h)
	}
	if g.screen == screenContainer {
		g.drawContainer(w, h)
	}
	if g.debug {
		g.drawDebug(w, h)
	}
	gl.Enable(gl.DEPTH_TEST)
}

// drawOutline draws the wireframe of the block the player points at.
func (g *game) drawOutline(mvp *mat4) {
	h, ok := g.target()
	if !ok {
		return
	}
	hit := h.pos
	var nb nbFunc
	st := g.world.Block(hit[0], hit[1], hit[2])
	if neighbourDependent[st>>4] {
		nb = func(dx, dy, dz int32) uint16 { return g.world.Block(hit[0]+dx, hit[1]+dy, hit[2]+dz) }
	}
	b := g.res.outlineBox(st, nb)
	const e = 0.002
	o := vec3{float64(hit[0]), float64(hit[1]), float64(hit[2])}
	lo, hi := o.add(vec3{float64(b[0] - e), float64(b[1] - e), float64(b[2] - e)}), o.add(vec3{float64(b[3] + e), float64(b[4] + e), float64(b[5] + e)})
	c := [8]vec3{
		{lo[0], lo[1], lo[2]}, {hi[0], lo[1], lo[2]}, {hi[0], lo[1], hi[2]}, {lo[0], lo[1], hi[2]},
		{lo[0], hi[1], lo[2]}, {hi[0], hi[1], lo[2]}, {hi[0], hi[1], hi[2]}, {lo[0], hi[1], hi[2]},
	}
	var v []float32
	for _, e := range [12][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {4, 5}, {5, 6}, {6, 7}, {7, 4}, {0, 4}, {1, 5}, {2, 6}, {3, 7}} {
		v = append(v, float32(c[e[0]][0]), float32(c[e[0]][1]), float32(c[e[0]][2]), float32(c[e[1]][0]), float32(c[e[1]][1]), float32(c[e[1]][2]))
	}
	hd := &g.hud
	gl.UseProgram(hd.line)
	gl.UniformMatrix4fv(hd.uMVP, 1, false, &mvp[0])
	gl.Uniform4f(hd.uLineCol, 0, 0, 0, 0.6)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.BindVertexArray(hd.lvao)
	gl.BindBuffer(gl.ARRAY_BUFFER, hd.lvbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(v)*4, gl.Ptr(v), gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.LINES, 0, int32(len(v)/3))
	gl.Disable(gl.BLEND)
}

// uploadAll sends the interface pictures to the GPU, from the current resources.
func (h *hud) uploadAll(g *game) {
	h.widgets, h.icons = uploadTexture(g.res.gui["widgets"]), uploadTexture(g.res.gui["icons"])
	h.tabItems = uploadTexture(g.res.gui["tab_items"])
	h.font = uploadTexture(g.res.gui["ascii"])
	h.itemTex = uploadTexture(g.res.items.img)
	h.invTex, h.tableTex = uploadTexture(g.res.gui["inventory"]), uploadTexture(g.res.gui["crafting_table"])
	g.glyphs = glyphWidths(g.res.gui["ascii"])
	for _, t := range g.mobTex {
		gl.DeleteTextures(1, &t)
	}
	g.mobTex = map[string]uint32{}
	for path, img := range g.res.entity {
		g.mobTex[path] = uploadTexture(img)
	}
	if t, ok := g.mobTex["misc/forcefield"]; ok { // the border's wall repeats across its length and height
		gl.BindTexture(gl.TEXTURE_2D, t)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	}
	skin, cape, wings := loadLook()
	g.mobTex["look/skin"], g.mobTex["look/cape"], g.mobTex["look/wings"] = uploadTexture(skin), uploadTexture(cape), uploadTexture(wings)
	h.dirt, h.logo = 0, 0
	if img := g.res.gui["options_background"]; img != nil {
		h.dirt = uploadTexture(img)
	}
	if img := g.res.gui["minecraft"]; img != nil {
		h.logo = uploadTexture(img)
	}
}
