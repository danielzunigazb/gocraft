package client

import "math"

// When a server puts the player somewhere a little different from where they are (a movement check that does not
// agree with them), the player is there at once for the game's physics and for the packets, as in the game; but
// the view does not jump. It starts from where it was and comes to the player over a tenth of a second, so
// a run of such corrections (a server that puts the player back every tick for a few ticks after a blow)
// is a few soft nudges and not a stutter. A move of two blocks or more (a spawn, an arena) is a jump as it is.

const (
	snapMax  = 2.0  // blocks: up to this far the view eases; farther it jumps
	snapTime = 0.06 // seconds: the time constant of the easing
)

// snapView is called when the server moves the player from one place to another.
func (g *game) snapView(from, to vec3) {
	d := from.sub(to)
	if n := math.Sqrt(d.dot(d)); n == 0 || n >= snapMax || g.net == nil || !g.net.terrainDone {
		g.snap = vec3{}
		return
	}
	g.snap = g.snap.add(d)
	if n := math.Sqrt(g.snap.dot(g.snap)); n > snapMax {
		g.snap = g.snap.scale(snapMax / n)
	}
}

// easeSnap is what is still to be added to the view for a frame that took dt seconds, and takes a step.
func (g *game) easeSnap(dt float64) vec3 {
	if g.snap == (vec3{}) {
		return vec3{}
	}
	g.snap = g.snap.scale(math.Exp(-dt / snapTime))
	if g.snap.dot(g.snap) < 1e-8 {
		g.snap = vec3{}
	}
	return g.snap
}
