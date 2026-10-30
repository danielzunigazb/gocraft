package client

import (
	"strconv"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// The screen of a player who died on a server (GuiGameOver): a red veil, "You died!", the score and the
// button that asks the server to respawn them. The world goes on behind it.

// sendRespawn is Client Status: perform respawn.
func (g *game) sendRespawn() {
	if g.net == nil {
		return
	}
	w := newPacket(0x16)
	w.VarInt(0)
	g.net.c.send(w)
}

func (g *game) drawDeathScreen(w, h int) {
	if g.screen != screenDead || g.net == nil {
		return
	}
	g.enableUI()
	hd := &g.hud
	s := float32(g.scaleFor(w, h))
	cx := float32(w) / 2
	// drawGradientRect(0, 0, width, height, 0x60500000, 0xA0803030): from the top colour to the bottom one
	const strips = 48
	for i := 0; i < strips; i++ {
		f := float32(i) / (strips - 1)
		c := [4]float32{(0x50 + (0x80-0x50)*f) / 255, 0x30 * f / 255, 0x30 * f / 255, (0x60 + (0xA0-0x60)*f) / 255}
		y0, y1 := float32(h)*float32(i)/strips, float32(h)*float32(i+1)/strips
		hd.rect(0, y0, float32(w), y1-y0+1, 0, 0, 1, 1, 1, 1, c)
	}
	hd.flush(hd.white)
	title := "You died!"
	g.text(title, cx-float32(g.textWidth(title))*s, 30*2*s, 2*s, [4]float32{1, 1, 1, 1}, true)
	score := legacyRuns("Score: §e"+strconv.Itoa(int(g.net.score)), defaultStyle)
	g.drawRuns(score, cx-float32(g.runsWidth(score))*s/2, 100*s, s, 1, true)
	left := cx/s - 100
	top := float32(h)/s/4 + 72
	if g.button("Respawn", left, top, 200, true) {
		g.sendRespawn()
	}
	if g.button("Title Screen", left, top+24, 200, true) {
		g.closeWorld()
		g.screen = screenTitle
		if g.win != nil {
			g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
		}
	}
	g.uiClick = false
}
