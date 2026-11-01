package client

import "testing"

// Sprinting eases the view out to 15% wider, half of the way each tick, and back in when it stops; Speed I makes
// it 10% wider, flying a tenth more, and a drawn bow narrows it.
func TestFieldOfViewFollowsTheSpeed(t *testing.T) {
	g := standing(t)
	g.opts.FOV = 70
	g.tick(input{})
	if f := g.fovMod; f != 1 {
		t.Fatalf("standing: %v", f)
	}
	g.sprinting = true
	g.updateFov()
	if want := float32(1.075); !near(g.fovMod, want, 1e-4) { // halfway from 1 to 1.15
		t.Fatalf("one tick into a sprint: %v, want %v", g.fovMod, want)
	}
	for i := 0; i < 20; i++ {
		g.updateFov()
	}
	if !near(g.fovMod, 1.15, 1e-4) {
		t.Fatalf("a sprint settles at %v, want 1.15", g.fovMod)
	}
	g.partial = 0.5
	g.fovModPrev, g.fovMod = 1.0, 1.1
	if got := g.worldFOV(); !near(got, 70*1.05, 1e-3) {
		t.Fatalf("between ticks %v, want %v", got, 70*1.05)
	}
	g.sprinting = false
	for i := 0; i < 20; i++ {
		g.updateFov()
	}
	if !near(g.fovMod, 1, 1e-4) {
		t.Fatalf("after the sprint %v", g.fovMod)
	}
	g.effects = map[int]potionEffect{effSpeed: {amp: 0}}
	if f := g.fovTarget(); !near(f, 1.1, 1e-4) {
		t.Fatalf("Speed I %v, want 1.1", f)
	}
	g.effects = nil
	g.flying = true
	if f := g.fovTarget(); !near(f, 1.1, 1e-4) {
		t.Fatalf("flying %v, want 1.1", f)
	}
	g.flying = false
	g.bowTicks = 20
	if f := g.fovTarget(); !near(f, 0.85, 1e-4) {
		t.Fatalf("a bow fully drawn %v, want 0.85", f)
	}
	g.bowTicks = 10
	if f := g.fovTarget(); !near(f, 1-0.25*0.15, 1e-4) {
		t.Fatalf("a bow half drawn %v", f)
	}
	g.walkCap = 0.2 // a server that makes the player walk at twice the speed: the ratio is what counts
	g.bowTicks = 0
	g.sprinting = true
	if f := g.fovTarget(); !near(f, 1.15, 1e-4) {
		t.Fatalf("with /walkspeed: %v", f)
	}
}
