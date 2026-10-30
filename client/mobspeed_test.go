package client

import (
	"math"
	"testing"
)

// steadySpeed is how far a mob walks or flees, in blocks a second, once it has got up to speed.
func steadySpeed(g *game, kind string, flee bool) float64 {
	m := g.newMob(kinds[kind], vec3{0.5, 4, 0.5}, 0)
	m.onGround = true
	var start vec3
	for i := 0; i < 100; i++ {
		if flee {
			m.panic = 50
		} else {
			m.wanderTicks, m.wanderYaw = 50, 0
		}
		g.tickMob(m)
		if i == 39 {
			start = m.pos
		}
	}
	return math.Hypot(float64(m.pos[0]-start[0]), float64(m.pos[2]-start[2])) / 60 * 20
}

// The move helper sets both the speed and the forward input, so a mob's speed goes with the square of
// its attribute: a sheep (0.23) walks at 2.28 blocks a second and flees (EntityAIPanic 1.25) at 3.57.
func TestMobSpeeds(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{200, 4, 200}
	for _, c := range []struct {
		kind  string
		flee  bool
		speed float64
	}{{"Sheep", false, 2.28}, {"Sheep", true, 3.57}, {"Cow", true, 6.91}, {"Chicken", true, 5.29}} {
		if got := steadySpeed(g, c.kind, c.flee); math.Abs(got-c.speed) > 0.1 {
			t.Errorf("%s (flee %v): %.2f blocks/s, want %.2f", c.kind, c.flee, got, c.speed)
		}
	}
}
