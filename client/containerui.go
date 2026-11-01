package client

import (
	"github.com/go-gl/gl/v3.3-core/gl"
)

const (
	contW, contH = 176, 166
)

func (g *game) contPanel() (s, left, top float32) {
	w, h := g.fb[0], g.fb[1]
	s = float32(g.scaleFor(w, h))
	return s, (float32(w) - contW*s) / 2, (float32(h) - float32(g.contHeight())*s) / 2
}

// slotUnderMouse is the slot of the open window the mouse is over.
func (g *game) slotUnderMouse() (placedSlot, bool) {
	s, left, top := g.contPanel()
	mx, my := g.mouse()
	x, y := (mx-left)/s, (my-top)/s
	for _, ps := range g.containerSlots() {
		if x >= float32(ps.x) && x < float32(ps.x+16) && y >= float32(ps.y) && y < float32(ps.y+16) {
			return ps, true
		}
	}
	return placedSlot{}, false
}

func (g *game) drawContainer(w, h int) {
	hd := &g.hud
	s, left, top := g.contPanel()
	one := [4]float32{1, 1, 1, 1}
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.UseProgram(hd.prog)
	gl.Uniform2f(hd.uScreen, float32(w), float32(h))
	gl.ActiveTexture(gl.TEXTURE0)
	hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0.06, 0.06, 0.6})
	hd.flush(hd.white)
	switch g.cont.kind {
	case contChest, contFurnace:
		g.drawBEBackground(left, top, s)
	case contTable:
		hd.rect(left, top, contW*s, contH*s, 0, 0, contW, contH, 256, 256, one)
		hd.flush(hd.tableTex)
	default:
		hd.rect(left, top, contW*s, contH*s, 0, 0, contW, contH, 256, 256, one)
		hd.flush(hd.invTex)
	}

	slots := g.containerSlots()
	lm := g.localMap()
	prev, remnant, painting := g.dragPreview(lm) // a stack painted across slots shows what each would hold
	shown := func(ps placedSlot) slot {
		if c, ok := prev[lm.indexOf(ps.ref.p)]; painting && ok {
			return slot{g.cursor.ID, g.cursor.Dmg, int8(c)}
		}
		return *ps.ref.p
	}
	for _, ps := range slots {
		g.drawSlotIcon(shown(ps), left+float32(ps.x)*s, top+float32(ps.y)*s, 16*s)
	}
	hd.flush(g.tex)
	hd.flushItems()
	for _, ps := range slots {
		g.drawCount(int(shown(ps).N), left+float32(ps.x)*s, top+float32(ps.y)*s, s)
		if _, ok := prev[lm.indexOf(ps.ref.p)]; painting && ok {
			hd.rect(left+float32(ps.x)*s, top+float32(ps.y)*s, 16*s, 16*s, 0, 0, 1, 1, 1, 1, [4]float32{1, 1, 1, 0.5})
			hd.flush(hd.white)
		}
	}
	if g.cont.kind == contPlayer { // the player, turned to the mouse
		g.drawPlayerPreview(left, top, s)
	}
	if ps, ok := g.slotUnderMouse(); ok {
		hd.rect(left+float32(ps.x)*s, top+float32(ps.y)*s, 16*s, 16*s, 0, 0, 1, 1, 1, 1, [4]float32{1, 1, 1, 0.4})
		hd.flush(hd.white)
		if g.cursor.empty() && !ps.ref.p.empty() { // and named
			mx, my := g.mouse()
			g.drawTooltip(g.displayName(*ps.ref.p), mx, my, s)
		}
	}
	if !g.cursor.empty() { // the stack on the mouse
		mx, my := g.mouse()
		g.drawSlotIcon(g.cursor, mx-8*s, my-8*s, 16*s)
		hd.flush(g.tex)
		hd.flushItems()
		n := int(g.cursor.N)
		if painting {
			n = remnant
		}
		g.drawCount(n, mx-8*s, my-8*s, s)
	}
	gl.Disable(gl.BLEND)
}
