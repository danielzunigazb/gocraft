package client

import (
	"image"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Text and buttons for the menus, drawn with the game's own font and widget textures.

// glyphWidths measures each character of font/ascii.png: the rightmost opaque column plus one, in
// the 8-pixel cells of the game's font, whatever the resolution of the picture is.
func glyphWidths(img *image.RGBA) (w [256]int) {
	cell := max(img.Bounds().Dx()/16, 1)
	for c := 0; c < 256; c++ {
		ox, oy := c%16*cell, c/16*cell
		for x := 0; x < cell; x++ {
			for y := 0; y < cell; y++ {
				if img.RGBAAt(ox+x, oy+y).A > 0 {
					w[c] = max(w[c], (x+1)*8/cell+boolInt((x+1)*8%cell != 0))
				}
			}
		}
	}
	w[' '] = 3
	return w
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// textWidth is the width of a string in GUI pixels: each glyph and one pixel after it.
func (g *game) textWidth(s string) int { return g.runsWidth([]styledRun{{s, defaultStyle}}) }

// text queues a string at x, y (framebuffer pixels) at the given GUI scale, with the game's shadow.
func (g *game) text(s string, x, y, scale float32, col [4]float32, shadow bool) {
	g.drawRuns([]styledRun{{s, textStyle{col: [3]float32{col[0], col[1], col[2]}}}}, x, y, scale, col[3], shadow)
}

func (g *game) centredText(s string, cx, y, scale float32, col [4]float32) {
	g.text(s, cx-float32(g.textWidth(s))*scale/2, y, scale, col, true)
}

// mouse is the cursor in framebuffer pixels.
func (g *game) mouse() (x, y float32) {
	f := float64(g.fb[0]) / float64(max(g.winSize(), 1))
	return float32(g.cx * f), float32(g.cy * f)
}

// button draws a 200 x 20 (or narrower) button at a position in GUI pixels from the top left of the
// centred 320-wide column, and reports whether it was clicked this frame.
func (g *game) button(label string, x, y, w float32, enabled bool) bool {
	hd := &g.hud
	s := float32(g.scaleFor(g.fb[0], g.fb[1]))
	px, py := x*s, y*s
	mx, my := g.mouse()
	hover := enabled && mx >= px && mx < px+w*s && my >= py && my < py+20*s
	v := float32(66) // normal
	switch {
	case !enabled:
		v = 46
	case hover:
		v = 86
	}
	one := [4]float32{1, 1, 1, 1}
	hd.rect(px, py, w/2*s, 20*s, 0, v, w/2, 20, 256, 256, one)
	hd.rect(px+w/2*s, py, w/2*s, 20*s, 200-w/2, v, w/2, 20, 256, 256, one)
	hd.flush(hd.widgets)
	col := [4]float32{1, 1, 1, 1}
	switch {
	case !enabled:
		col = [4]float32{0.63, 0.63, 0.63, 1}
	case hover:
		col = [4]float32{1, 1, 0.63, 1}
	}
	g.centredText(label, px+w/2*s, py+6*s, s, col)
	if hover && g.uiClick {
		g.uiSound()
		return true
	}
	return false
}

// dirtBackground tiles the options background, darkened, over the whole window.
func (g *game) dirtBackground(shade float32) {
	hd := &g.hud
	if hd.dirt == 0 {
		hd.rect(0, 0, float32(g.fb[0]), float32(g.fb[1]), 0, 0, 1, 1, 1, 1, [4]float32{0.1, 0.1, 0.1, 1})
		hd.flush(hd.white)
		return
	}
	s := float32(g.scaleFor(g.fb[0], g.fb[1]))
	tile := 32 * s
	c := [4]float32{shade, shade, shade, 1}
	for y := float32(0); y < float32(g.fb[1]); y += tile {
		for x := float32(0); x < float32(g.fb[0]); x += tile {
			hd.rect(x, y, tile, tile, 0, 0, 16, 16, 16, 16, c)
		}
	}
	hd.flush(hd.dirt)
}

func (g *game) enableUI() {
	hd := &g.hud
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.UseProgram(hd.prog)
	gl.Uniform2f(hd.uScreen, float32(g.fb[0]), float32(g.fb[1]))
	gl.ActiveTexture(gl.TEXTURE0)
}

func trimName(s string, max int) string {
	if len(s) > max {
		return strings.TrimSpace(s[:max]) // the game's font has no ellipsis
	}
	return s
}
