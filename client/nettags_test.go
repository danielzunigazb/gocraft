package client

import (
	"math"
	"testing"
)

// The objective shown under names puts "score name-of-objective" under a player's name when they are within 10
// blocks, and the name goes up one line; a hearts objective is known as such; farther away, only the name.
func TestScoreUnderNames(t *testing.T) {
	n := &netState{names: map[string]string{}}
	sb := n.board()
	sb.objective("hp", 0, "§c❤", false)
	sb.slots[2] = "hp"
	sb.setScore("Steve", "hp", 14, false)
	p := &remotePlayer{name: "Steve", pos: vec3{3, 4, 4}}
	ls := n.playerLabels(p, vec3{0, 5.6, 0}) // 5 blocks away
	if len(ls) != 2 || plainRuns(ls[0].runs) != "14 ❤" || plainRuns(ls[1].runs) != "Steve" {
		t.Fatalf("labels %v", ls)
	}
	if d := ls[1].at[1] - ls[0].at[1]; math.Abs(d-nameLine) > 1e-9 || math.Abs(nameLine-0.276) > 0.001 {
		t.Fatalf("the name is %v above the score, want %v", d, nameLine)
	}
	if ls[0].at[1] != 4+1.8+0.5 {
		t.Fatalf("the score at %v", ls[0].at)
	}
	if far := n.playerLabels(p, vec3{20, 5.6, 0}); len(far) != 1 || plainRuns(far[0].runs) != "Steve" {
		t.Fatalf("far labels %v", far)
	}
	sb.slots[2] = ""
	if none := n.playerLabels(p, vec3{0, 5.6, 0}); len(none) != 1 {
		t.Fatal("no objective under names, and a score was drawn")
	}
	sb.team["Steve"] = &scoreTeam{prefix: "§a[V] "}
	if l := n.playerLabels(p, vec3{0, 5.6, 0}); plainRuns(l[0].runs) != "[V] Steve" {
		t.Fatalf("team prefix: %q", plainRuns(l[0].runs))
	}
}

func TestObjectiveTypeHearts(t *testing.T) {
	g, _, apply := mobGame(t)
	for _, c := range []struct {
		typ    string
		hearts bool
	}{{"integer", false}, {"hearts", true}} {
		w := newPacket(0x3B)
		w.String("hp")
		w.Byte(2) // renamed or made
		w.String("Health")
		w.String(c.typ)
		apply(w)
		if o := g.net.board().objectives["hp"]; o == nil || o.hearts != c.hearts {
			t.Fatalf("%s: %+v", c.typ, o)
		}
	}
}
