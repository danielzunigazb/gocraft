package client

import (
	"math"
	"testing"
	"time"

	"gocraft/proto"
)

func borderPacket(action int32, f func(w *proto.Writer)) *proto.Writer {
	w := newPacket(0x44)
	w.VarInt(action)
	f(w)
	return w
}

func dbl(w *proto.Writer, v float64) { w.Long(int64(math.Float64bits(v))) }

// The border's packets set its size, move it between sizes over a time and set its middle; it is blue while it
// holds, green growing and red shrinking, and its size follows the clock.
func TestWorldBorderPackets(t *testing.T) {
	g, _, apply := mobGame(t)
	clock := time.Unix(100, 0)
	apply(borderPacket(3, func(w *proto.Writer) { // initialize: centre, old and new size, speed, portal, warnings
		dbl(w, 10)
		dbl(w, -20)
		dbl(w, 100)
		dbl(w, 100)
		w.VarLong(0)
		w.VarInt(29999984)
		w.VarInt(15)
		w.VarInt(5)
	}))
	b := g.net.border
	b.now = func() time.Time { return clock }
	minX, minZ, maxX, maxZ := b.edges()
	if minX != -40 || maxX != 60 || minZ != -70 || maxZ != 30 || b.colour() != [3]uint8{0x20, 0xA0, 0xFF} {
		t.Fatalf("edges %v %v %v %v colour %v", minX, minZ, maxX, maxZ, b.colour())
	}
	apply(borderPacket(1, func(w *proto.Writer) { dbl(w, 100); dbl(w, 200); w.VarLong(10000) }))
	b.start = clock // the packet was read with the real clock: start the transition on the test's
	b.end = clock.Add(10 * time.Second)
	clock = clock.Add(5 * time.Second)
	if d := b.diameter(); d != 150 || b.colour() != [3]uint8{0x80, 0xFF, 0x20} {
		t.Fatalf("halfway: %v, colour %v, want 150 and green", d, b.colour())
	}
	clock = clock.Add(6 * time.Second)
	if d := b.diameter(); d != 200 || b.colour() != [3]uint8{0x20, 0xA0, 0xFF} {
		t.Fatalf("after: %v, colour %v", d, b.colour())
	}
	apply(borderPacket(1, func(w *proto.Writer) { dbl(w, 200); dbl(w, 50); w.VarLong(1000) }))
	if b.colour() != [3]uint8{0xFF, 0x30, 0x30} {
		t.Fatalf("shrinking is %v, want red", b.colour())
	}
	apply(borderPacket(0, func(w *proto.Writer) { dbl(w, 80) }))
	apply(borderPacket(2, func(w *proto.Writer) { dbl(w, 0); dbl(w, 0) }))
	if minX, _, maxX, _ = b.edges(); minX != -40 || maxX != 40 {
		t.Fatalf("set size and centre: %v %v", minX, maxX)
	}
}

// The wall is drawn only near a side, fading in to the fourth power of how close the camera is, one quad a block
// along the part within reach.
func TestWorldBorderWall(t *testing.T) {
	b := newWorldBorder()
	b.x, b.z = 0, 0
	b.from, b.to = 100, 100
	if vs, _ := b.wall(vec3{0, 10, 0}, 32, 0); vs != nil {
		t.Fatal("a wall in the middle of the border")
	}
	vs, a := b.wall(vec3{45, 10, 0}, 32, 0)
	if len(vs) != 64*4 {
		t.Fatalf("%d vertices, want a side of 64 quads", len(vs))
	}
	if want := math.Pow(1-5.0/32, 4); math.Abs(float64(a)-want) > 1e-6 {
		t.Fatalf("alpha %v, want %v", a, want)
	}
	for _, v := range vs {
		if v.X != 50 {
			t.Fatalf("a vertex at x %v off the east side", v.X)
		}
	}
	if vs[0].Y != 256 || vs[3].Y != 0 || vs[0].V != 0 || vs[0].U != 0 || vs[1].U != 0.5 || vs[2].V != 128 {
		t.Fatalf("quad %+v", vs[:4])
	}
	vs, _ = b.wall(vec3{45, 10, 45}, 32, 0.25) // a corner: two sides of 37 quads, the texture shifted
	if len(vs) != 74*4 || vs[0].U != 0.25 || vs[0].V != 0.25 {
		t.Fatalf("corner: %d vertices, first %+v", len(vs), vs[0])
	}
	if _, a := b.wall(vec3{49.9, 10, 0}, 32, 0); a < 0.98 {
		t.Fatalf("at the wall alpha is %v", a)
	}
	if _, a := b.wall(vec3{80, 10, 0}, 32, 0); a != 1 { // outside: the closest distance is negative, clamped
		t.Fatalf("outside alpha %v", a)
	}
}
