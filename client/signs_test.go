package client

import (
	"testing"
)

// Update Sign gives a sign its lines; the board, the stick (standing only) and the text are laid out in front
// of the board, facing the way the meta says, and the text goes when the sign does.
func TestSignsFromTheServer(t *testing.T) {
	g, _, apply := mobGame(t)
	g.camPos, g.camDir = vec3{0.5, 5.6, -2}, vec3{0, 0, 1}
	g.world.SetBlock(0, 4, 2, idStandingSign<<4|8) // facing north, towards the camera
	w := newPacket(0x33)
	w.Long(packPos(0, 4, 2))
	for _, l := range []string{`{"text":"[SkyWars]","color":"dark_green","bold":true}`, `"Solo"`, `""`, `{"text":"12/12"}`} {
		w.String(l)
	}
	apply(w)
	text := g.signs[[3]int32{0, 4, 2}]
	if text == nil || plainRuns(text[0]) != "[SkyWars]" || !text[0][0].st.bold || text[1][0].st.col != ([3]float32{0, 0, 0}) {
		t.Fatalf("sign text %+v", text)
	}
	groups := map[string][]vertex{}
	g.emitSigns(groups)
	if n := len(groups["sign"]); n != 2*6*4 {
		t.Fatalf("%d board vertices, want the board and the stick (48)", n)
	}
	font := groups["!font"]
	if len(font) == 0 || len(font)%4 != 0 {
		t.Fatalf("%d text vertices", len(font))
	}
	for _, v := range font { // the text sits in front of the board, on the camera's side (north, -z)
		if v.Z >= 2.5 {
			t.Fatalf("a letter at z %v: behind the board", v.Z)
		}
	}
	g.world.SetBlock(1, 5, 3, idWallSign<<4|2)
	g.netUpdateSign([3]int32{1, 5, 3}, [4]string{`"a"`, `""`, `""`, `""`})
	groups = map[string][]vertex{}
	g.emitSigns(groups)
	if n := len(groups["sign"]); n != 2*6*4+6*4 {
		t.Fatalf("%d board vertices with a wall sign, want 72 (it has no stick)", n)
	}
	g.changeBlock(0, 4, 2, 0, false)
	if g.signs[[3]int32{0, 4, 2}] != nil {
		t.Fatal("a broken sign kept its text")
	}
}
