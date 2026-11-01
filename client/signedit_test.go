package client

import (
	"strings"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"

	"gocraft/proto"
)

func openSignPacket(x, y, z int32) *proto.Writer {
	w := newPacket(0x36)
	w.Long(proto.PackPos(x, y, z))
	return w
}

func typeInto(g *game, s string) {
	for _, r := range s {
		g.signChar(r)
	}
}

// Open Sign Editor puts the sign's own text in the editor and gives it the keyboard.
func TestSignEditorOpensWithTheSignsText(t *testing.T) {
	g, _, apply := mobGame(t)
	g.setSignText([3]int32{4, 5, 6}, [4][]styledRun{{{"uno", signStyle}}, nil, {{"tres", signStyle}}, nil})
	apply(openSignPacket(4, 5, 6))
	if g.screen != screenSignEdit || g.sedit == nil || g.sedit.pos != [3]int32{4, 5, 6} || g.sedit.lines != [4]string{"uno", "", "tres", ""} {
		t.Fatalf("screen %v editor %+v", g.screen, g.sedit)
	}
	if !g.typing() {
		t.Fatal("the editor does not own the keyboard")
	}
}

// Typing, the arrow keys, Enter, Backspace and the width of a line, as GuiEditSign has them.
func TestSignEditorKeys(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(openSignPacket(1, 2, 3))
	typeInto(g, "Hola")
	g.signKey(glfw.KeyDown)
	typeInto(g, "mundo")
	g.signKey(glfw.KeyEnter)
	g.signKey(glfw.KeyKPEnter)
	g.signKey(glfw.KeyEnter) // 1 + 3 = line 4, which is line 0 again
	if g.sedit.line != 0 {
		t.Fatalf("line %d after four downs from 1", g.sedit.line)
	}
	g.signKey(glfw.KeyUp) // up from the first goes to the last
	if g.sedit.line != 3 {
		t.Fatalf("line %d after up from 0", g.sedit.line)
	}
	g.signKey(glfw.KeyUp)
	g.signKey(glfw.KeyUp)
	g.signKey(glfw.KeyUp) // back at 0
	g.signKey(glfw.KeyBackspace)
	if g.sedit.lines[0] != "Hol" {
		t.Fatalf("line 0 is %q after Backspace", g.sedit.lines[0])
	}
	g.signKey(glfw.KeyBackspace)
	g.signKey(glfw.KeyBackspace)
	g.signKey(glfw.KeyBackspace)
	g.signKey(glfw.KeyBackspace) // nothing left: no harm
	if g.sedit.lines[0] != "" || g.sedit.lines[1] != "mundo" {
		t.Fatalf("lines %q", g.sedit.lines)
	}
	typeInto(g, strings.Repeat("W", 400)) // too wide: it stops where one more would pass 90 pixels
	line := g.sedit.lines[0]
	if g.textWidth(line) > signLineWidth || g.textWidth(line+"W") <= signLineWidth || len(line) >= 400 {
		t.Fatalf("the line is %d pixels wide with %d characters; the limit is %d", g.textWidth(line), len(line), signLineWidth)
	}
	g.signChar('\n') // control characters are not text
	g.signChar(127)
	if strings.ContainsAny(g.sedit.lines[0], "\n\x7f") {
		t.Fatal("a control character went in")
	}
}

// Done and Escape send Update Sign with the position and the four lines as JSON chat components, and the sign shows
// the new text at once.
func TestSignEditorSendsUpdateSign(t *testing.T) {
	for _, closeIt := range []func(g *game){func(g *game) { g.signKey(glfw.KeyEscape) }, func(g *game) { g.closeSignEditor() }} {
		g, c, apply := mobGame(t)
		g.screen = screenNone
		apply(openSignPacket(-7, 64, 12))
		typeInto(g, `Di "hola" & <hasta>`)
		g.signKey(glfw.KeyDown)
		g.signKey(glfw.KeyDown)
		typeInto(g, "ñandú")
		closeIt(g)
		if g.screen == screenSignEdit || g.sedit != nil {
			t.Fatal("the editor stayed open")
		}
		var got []string
		var x, y, z int32
		for _, p := range drain(t, c) {
			if p.id != 0x12 {
				continue
			}
			x, y, z = proto.UnpackPos(p.r.Long())
			for i := 0; i < 4; i++ {
				got = append(got, p.r.String(32767))
			}
		}
		if x != -7 || y != 64 || z != 12 || len(got) != 4 {
			t.Fatalf("Update Sign at %d %d %d with %v", x, y, z, got)
		}
		want := []string{`"Di \"hola\" & <hasta>"`, `""`, `"ñandú"`, `""`}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("line %d is %s, want %s", i, got[i], want[i])
			}
		}
		if txt := g.signs[[3]int32{-7, 64, 12}]; txt == nil || plainRuns(txt[2]) != "ñandú" {
			t.Fatalf("the sign shows %v", txt)
		}
	}
}
