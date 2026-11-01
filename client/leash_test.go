package client

import (
	"math"
	"testing"

	"gocraft/proto"
)

func attachPacket(eid, holder int32, leash byte) *proto.Writer {
	w := newPacket(0x1B)
	w.Int(eid)
	w.Int(holder)
	w.Byte(leash)
	return w
}

// Attach Entity with leash 1 ties a mob to a holder; a holder of -1 unties it; a rider (0) is not a leash.
func TestLeashAttachAndDetach(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(20, 90, 3.5, 4, 3.5, 0, 0, nil))
	e := g.net.entity(20)
	apply(attachPacket(20, 77, 1))
	if e.leash != 77 {
		t.Fatalf("leash %d", e.leash)
	}
	if f := decodeFrame(t, attachPacket(20, 5, 0), nil); f != nil {
		t.Fatal("a rider was applied as a leash")
	}
	apply(attachPacket(20, -1, 1))
	if e.leash != 0 {
		t.Fatalf("leash %d after untying", e.leash)
	}
}

// The rope is two strips of 24 quads; it starts at the mob's side at its height and ends at the holder's hand.
func TestLeashRopeShape(t *testing.T) {
	h := leashHolder{prev: vec3{10, 4, 0}, pos: vec3{10, 4, 0}, eye: 1.62}
	vs := leashQuads(nil, 0, 4, 0, 0.9, 0.9, 0, h, 1)
	if len(vs) != 2*24*4 {
		t.Fatalf("%d vertices, want %d", len(vs), 2*24*4)
	}
	// the first vertex of the first strip is at the mob: x = width * 0.4 * cos(yaw + 90 degrees) = 0 (its side at yaw 0
	// faces +/-z), z = 0.36 away, and at y = 4 - (1.6 - 0.9) / 2 + 24/18 + 0.125
	v := vs[0]
	if !near(float64(v.X), 0, 1e-5) || !near(float64(v.Z), 0.9*0.4, 1e-5) {
		t.Fatalf("the rope starts at (%v, %v)", v.X, v.Z)
	}
	if want := 4 - (1.6-0.9)*0.5 + 24.0/18 + 0.125; !near(float64(v.Y), want, 1e-5) {
		t.Fatalf("the rope starts at y %v, want %v", v.Y, want)
	}
	// the last vertex of the strip is the end, at the holder's hand: 0.7 in front of it, 0.5 * cos(pitch) to the side
	// and at the eye's 0.7 less a quarter, plus the sag's last term 0.125 (the (24-24)/18 is 0)
	last := vs[23*4+3] // the far end of the last quad of the first strip, on the strip's base line
	if !near(float64(last.X), 10-0.7*math.Cos(0)-0, 1e-5) || !near(float64(last.Z), 0.5*1, 1e-5) {
		t.Fatalf("the rope ends at (%v, %v)", last.X, last.Z)
	}
	if want := 4 - (1.6-0.9)*0.5 + (4+1.62*0.7-0.25-4)*(1+1)*0.5 + 0.125; !near(float64(last.Y), want, 1e-4) {
		t.Fatalf("the rope ends at y %v, want %v", last.Y, want)
	}
	// it sags: the middle is lower than the straight line between the ends
	mid := vs[12*4]
	straight := (float64(vs[0].Y) + float64(last.Y)) / 2
	if float64(mid.Y) >= straight {
		t.Fatalf("the middle at %v does not sag below %v", mid.Y, straight)
	}
	// colours alternate between segments: the even ones are 0.7 of the others
	if a, b := leashColour(2), leashColour(3); a[0] >= b[0] || b != [4]uint8{127, 102, 76, 255} {
		t.Fatalf("colours %v %v", a, b)
	}
}

// A holder that hangs on a wall (a knot) lets the rope leave straight from it, whichever way it was turned.
func TestLeashFromAKnot(t *testing.T) {
	a := leashHolder{prev: vec3{5.5, 5.5, 5.5}, pos: vec3{5.5, 5.5, 5.5}, eye: -0.0625, hanging: true, yawA: 123, yawB: 123}
	b := a
	b.yawA, b.yawB = -40, -40
	va, vb := leashQuads(nil, 0, 4, 0, 0.9, 0.9, 0, a, 1), leashQuads(nil, 0, 4, 0, 0.9, 0.9, 0, b, 1)
	for i := range va {
		if va[i] != vb[i] {
			t.Fatal("a knot's rope depends on its yaw")
		}
	}
	end := va[23*4+3]
	// d2 = d3 = 0, d4 = -1: the rope ends at the knot's x and z and 0.5 + eye * 0.7 - 0.25 above its y
	if !near(float64(end.X), 5.5, 1e-5) || !near(float64(end.Z), 5.5, 1e-5) {
		t.Fatalf("the rope ends at (%v, %v)", end.X, end.Z)
	}
	want := 5.5 - 0.0625*0.7 + 0.5 - 0.25 - (1.6-0.9)*0.5 + 0.125 // the knot's y, its eye * 0.7, the half block up, the sag's last term
	if !near(float64(end.Y), want, 1e-4) {
		t.Fatalf("the rope ends at y %v, want %v", end.Y, want)
	}
}

// A knot is a block of 6 x 8 x 6 pixels at the middle of the fence's block, with the lead_knot texture.
func TestLeashKnotIsDrawn(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnObject(30, objLeashKnot, 0, 10.5, 5.5, 20.5, 0, 0, 0))
	e := g.net.entity(30)
	if e == nil || e.pos != (vec3{10.5, 5.5, 20.5}) {
		t.Fatalf("knot %+v", e)
	}
	groups := map[string][]vertex{}
	g.emitLeashes(groups)
	vs := groups["lead_knot"]
	if len(vs) != 24 {
		t.Fatalf("%d vertices, want 24", len(vs))
	}
	lo, hi := standBounds(vs)
	if !near(float64(hi[0]-lo[0]), 6.0/16, 1e-4) || !near(float64(hi[1]-lo[1]), 8.0/16, 1e-4) || !near(float64(hi[2]-lo[2]), 6.0/16, 1e-4) {
		t.Fatalf("knot spans %v..%v", lo, hi)
	}
	// the box runs from 6 pixels above the entity's origin (y up) to 2 below it
	if !near(float64(hi[1]), 5.5+6.0/16, 1e-4) || !near(float64(lo[1]), 5.5-2.0/16, 1e-4) {
		t.Fatalf("knot at y %v..%v", lo[1], hi[1])
	}
}

// A leashed mob's rope is drawn from the mob to a player who holds it.
func TestLeashedMobDrawsItsRope(t *testing.T) {
	g, _, apply := mobGame(t)
	g.partial = 1
	g.camPos = vec3{0, 5, 0}
	apply(spawnMobPacket(20, 90, 3.5, 4, 3.5, 0, 0, nil))
	g.net.remote[9] = &remotePlayer{eid: 9, pos: vec3{8, 4, 3.5}, prev: vec3{8, 4, 3.5}}
	apply(attachPacket(20, 9, 1))
	e := g.net.entity(20)
	e.mob.m.prev = e.mob.m.pos
	groups := map[string][]vertex{}
	g.emitLeashes(groups)
	if len(groups["!leash"]) != 2*24*4 {
		t.Fatalf("%d rope vertices", len(groups["!leash"]))
	}
	apply(attachPacket(20, 12345, 1)) // a holder the client does not know: no rope
	groups = map[string][]vertex{}
	g.emitLeashes(groups)
	if len(groups["!leash"]) != 0 {
		t.Fatal("a rope was drawn to nothing")
	}
}
