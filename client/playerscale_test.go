package client

import (
	"math"
	"testing"
)

// A player's model is 1.875 blocks tall (RenderPlayer scales it by 0.9375), and armour is scaled with it.
func TestPlayerModelHeight(t *testing.T) {
	g := testGame(t)
	g.net = &netState{remote: map[int32]*remotePlayer{}}
	pos := vec3{10, 70, 10}
	p := &remotePlayer{eid: 1, pos: pos, prev: pos, target: pos, armor: [4]slot{{}, {}, {}, {ID: 310, N: 1}}}
	groups := map[string][]vertex{}
	g.emitRemote(groups, p)
	top := func(vs []vertex) float64 {
		m := math.Inf(-1)
		for _, v := range vs {
			m = math.Max(m, float64(v.Y))
		}
		return m - pos[1]
	}
	if h := top(groups["look/skin"]); math.Abs(h-1.875-0.05) > 0.02 { // the hat layer stands a little over the head
		t.Fatalf("the player's skin reaches %.4f above the feet, want 1.875 (hat 0.05 more)", h)
	}
	if h := top(groups["armor/diamond_layer_1"]); h < 1.9 || h > 2.0 {
		t.Fatalf("a helmet reaches %.4f", h)
	}
}
