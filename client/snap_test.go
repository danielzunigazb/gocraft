package client

import (
	"math"
	"testing"
)

// A server that moves the player a little leaves the view where it was and eases it over; a move of two blocks or
// more is a jump; corrections that come every tick add up but never push the view further than 2 blocks.
func TestViewEasesAfterASmallCorrection(t *testing.T) {
	g, _, apply := mobGame(t)
	g.net.terrainDone = true
	g.feet = vec3{10, 5, 10}
	g.snapView(vec3{10, 5, 10}, vec3{10.4, 5, 10})
	if !near(g.snap[0], -0.4, 1e-9) || g.snap[1] != 0 || g.snap[2] != 0 {
		t.Fatalf("snap %v: the view starts where it was, 0.4 behind the new place", g.snap)
	}
	prev := math.Inf(1)
	for i := 0; i < 8; i++ { // frames of 16 ms
		v := g.easeSnap(0.016)
		if n := math.Abs(v[0]); n >= prev || n > 0.4 {
			t.Fatalf("frame %d: %v does not shrink from %v", i, n, prev)
		} else {
			prev = n
		}
	}
	if math.Abs(g.snap[0]) > 0.4*math.Exp(-0.128/snapTime)*1.01 {
		t.Fatalf("after 128 ms %v is left", g.snap)
	}
	for i := 0; i < 200; i++ {
		g.easeSnap(0.016)
	}
	if g.snap != (vec3{}) {
		t.Fatalf("never ends: %v", g.snap)
	}

	g.snapView(vec3{0, 0, 0}, vec3{5, 0, 0}) // a spawn or an arena: the view goes with the player
	if g.snap != (vec3{}) {
		t.Fatalf("a long move was eased: %v", g.snap)
	}
	for i := 0; i < 12; i++ { // the same correction every tick
		g.snapView(vec3{0, 0, 0}, vec3{0.5, 0, 0})
	}
	if n := math.Sqrt(g.snap.dot(g.snap)); n > snapMax+1e-9 {
		t.Fatalf("the view is %v behind: it may be no more than %v", n, snapMax)
	}

	// It is wired to the packet: Player Position And Look leaves the physics at the new place and the view behind.
	g.snap = vec3{}
	g.feet, g.vel = vec3{20, 5, 20}, vec3{}
	w := newPacket(0x08)
	w.Double(20.3)
	w.Double(5)
	w.Double(20)
	w.Float(0)
	w.Float(0)
	w.Byte(0)
	apply(w)
	if g.feet != (vec3{20.3, 5, 20}) || !near(g.snap[0], -0.3, 1e-9) {
		t.Fatalf("feet %v snap %v", g.feet, g.snap)
	}
}
