package client

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"gocraft/proto"
)

// The sign editor, after 1.8's GuiEditSign. Open Sign Editor (0x36) names the sign. The player types on four lines: up
// and down (or Enter) move between them, backspace deletes, a line takes characters while it is at most 90 pixels
// wide; Done or Escape sends Update Sign (0x12) with the four lines as chat components. The game draws the board in
// 3D; here it is the board's front face from the sign's texture, flat.

type signEditor struct {
	pos   [3]int32
	lines [4]string
	line  int
}

// openSignEditor is the editor for the sign at pos, with the sign's own text in it.
func (g *game) openSignEditor(pos [3]int32) {
	ed := &signEditor{pos: pos}
	if t := g.signs[pos]; t != nil {
		for i, runs := range t {
			ed.lines[i] = plainRuns(runs)
		}
	}
	g.sedit = ed
	g.openScreen(screenSignEdit)
}

// signChar is a character typed in the editor.
func (g *game) signChar(r rune) {
	ed := g.sedit
	if ed == nil || r < 32 || r == 127 {
		return
	}
	if s := ed.lines[ed.line] + string(r); g.textWidth(s) <= signLineWidth {
		ed.lines[ed.line] = s
	}
}

// signKey is a key pressed in the editor (GuiEditSign.keyTyped and its Done button).
func (g *game) signKey(key glfw.Key) {
	ed := g.sedit
	if ed == nil {
		return
	}
	switch key {
	case glfw.KeyUp:
		ed.line = (ed.line - 1) & 3
	case glfw.KeyDown, glfw.KeyEnter, glfw.KeyKPEnter:
		ed.line = (ed.line + 1) & 3
	case glfw.KeyBackspace:
		if rs := []rune(ed.lines[ed.line]); len(rs) > 0 {
			ed.lines[ed.line] = string(rs[:len(rs)-1])
		}
	case glfw.KeyEscape:
		g.closeSignEditor()
	}
}

// closeSignEditor is Done: the lines go to the server, and the sign shows them at once.
func (g *game) closeSignEditor() {
	ed := g.sedit
	g.sedit = nil
	if ed == nil {
		return
	}
	if g.net != nil {
		p := newPacket(0x12)
		p.Long(proto.PackPos(ed.pos[0], ed.pos[1], ed.pos[2]))
		for _, l := range ed.lines {
			p.String(textComponent(l))
		}
		g.net.c.send(p)
	}
	var runs [4][]styledRun
	for i, l := range ed.lines {
		runs[i] = []styledRun{{l, signStyle}}
	}
	g.setSignText(ed.pos, runs)
	if g.screen == screenSignEdit {
		g.closeScreen()
	}
}

// textComponent is a plain chat component as the game writes it: a JSON string.
func textComponent(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return string(bytes.TrimSpace(b.Bytes()))
}

func (g *game) drawSignEditor(w, h int) {
	ed := g.sedit
	if g.screen != screenSignEdit || ed == nil {
		return
	}
	g.enableUI()
	hd := &g.hud
	s := float32(g.scaleFor(w, h))
	cx := float32(w) / 2
	hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0.06, 0.06, 0.6})
	hd.flush(hd.white)
	title := "Edit sign message:"
	g.text(title, cx-float32(g.textWidth(title))*s/2, 40*s, s, [4]float32{1, 1, 1, 1}, true)
	// the board: 24 x 12 pixels of the sign's texture, shown at 140 x 70
	bw, bh := float32(140)*s, float32(70)*s
	bx, by := cx-bw/2, 62*s
	if tex, ok := g.mobTex["sign"]; ok {
		hd.rect(bx, by, bw, bh, 2, 2, 24, 12, 64, 32, [4]float32{1, 1, 1, 1})
		hd.flush(tex)
	} else {
		hd.rect(bx, by, bw, bh, 0, 0, 1, 1, 1, 1, [4]float32{0.65, 0.5, 0.3, 1})
		hd.flush(hd.white)
	}
	black := [4]float32{0, 0, 0, 1}
	for i, l := range ed.lines {
		if i == ed.line && time.Now().UnixMilli()/300%2 == 0 {
			l = "> " + l + " <" // the line being edited
		}
		g.text(l, cx-float32(g.textWidth(l))*s/2, by+(10+float32(i)*12)*s, s, black, false)
	}
	if g.button("Done", cx/s-100, float32(h)/s/4+120, 200, true) {
		g.closeSignEditor()
	}
	g.uiClick = false
}
