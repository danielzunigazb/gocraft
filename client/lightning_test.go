package client

import (
	"math"
	"math/rand"
	"testing"

	"gocraft/proto"
)

func spawnBolt(eid int32, kind byte, x, y, z float64) *proto.Writer {
	w := newPacket(0x2C)
	w.VarInt(eid)
	w.Byte(kind)
	w.Int(int32(x * 32))
	w.Int(int32(y * 32))
	w.Int(int32(z * 32))
	return w
}

// Spawn Global Entity of type 1 is a bolt where the packet says; other types are left alone.
func TestLightningSpawn(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnBolt(70, 1, 10.5, 64, -20.5))
	e := g.net.entity(70)
	if e == nil || e.bolt == nil || e.pos != (vec3{10.5, 64, -20.5}) || e.bolt.state != 2 || e.bolt.living < 1 || e.bolt.living > 3 {
		t.Fatalf("bolt %+v", e)
	}
	if f := decodeFrame(t, spawnBolt(71, 2, 0, 0, 0), nil); f != nil {
		t.Fatal("a global entity that is not a bolt was applied")
	}
}

// The bolt flashes the sky while its state is not below zero and goes after its last strike.
func TestLightningLife(t *testing.T) {
	old := rng
	rng = rand.New(rand.NewSource(7))
	defer func() { rng = old }()
	g, _, apply := mobGame(t)
	apply(spawnBolt(70, 1, 0, 64, 0))
	e := g.net.entity(70)
	strikes := e.bolt.living
	flashes, states := 0, map[int]bool{}
	for i := 0; i < 400 && g.net.entity(70) != nil; i++ {
		g.tickEnts()
		if e.bolt.state >= 0 {
			flashes++
			if g.flash != 2 {
				t.Fatalf("tick %d: state %d but flash %d", i, e.bolt.state, g.flash)
			}
		}
		states[e.bolt.state] = true
	}
	if g.net.entity(70) != nil {
		t.Fatal("the bolt never went")
	}
	if !states[1] || !states[0] || !states[-1] {
		t.Fatalf("states seen %v", states)
	}
	if flashes < 2*strikes { // each strike shows in states 1 and 0 at least (the first in 1 and 0)
		t.Fatalf("%d flashes for %d strikes", flashes, strikes)
	}
	for i := 0; i < 3; i++ { // the flash dies out on its own
		g.tickEnts()
	}
	if g.flash != 0 {
		t.Fatalf("flash %d after the bolt", g.flash)
	}
}

// A strike is four passes of three strands (8 + 3 + 3 segments of a strip each); the main strand reaches the ground
// at the bolt's own column, whatever the seed.
func TestLightningGeometry(t *testing.T) {
	for _, seed := range []int64{0, 1, 123456789, -77} {
		vs := boltGeometry(nil, seed, 10, 64, 20)
		if len(vs) != 4*14*16 {
			t.Fatalf("seed %d: %d vertices, want %d", seed, len(vs), 4*14*16)
		}
		low, high := float32(1e9), float32(-1e9)
		for _, v := range vs {
			low, high = min(low, v.Y), max(high, v.Y)
		}
		if low != 64 || high != 64+128 {
			t.Fatalf("seed %d: spans y %v..%v", seed, low, high)
		}
		widths := map[float64]bool{0.09: true, 0.27: true, 0.45: true, 0.63: true} // 0.1+0.2k times 0.9
		for _, v := range vs {
			if v.Y != 64 {
				continue
			}
			dx, dz := math.Abs(float64(v.X)-10.5), math.Abs(float64(v.Z)-20.5)
			ok := false
			for w := range widths {
				if math.Abs(dx-w) < 1e-4 && math.Abs(dz-w) < 1e-4 {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("seed %d: a vertex on the ground is %v, %v off the column", seed, dx, dz)
			}
		}
	}
	a, b := boltGeometry(nil, 5, 0, 0, 0), boltGeometry(nil, 6, 0, 0, 0)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
		}
	}
	if same {
		t.Fatal("two seeds gave the same bolt")
	}
}

// The sky is pulled towards 0.8 while a bolt flashes.
func TestLightningSkyFlash(t *testing.T) {
	c := [3]float32{0.2, 0.4, 1}
	if got := lightningSky(c, 0, 0); got != c {
		t.Fatalf("no flash changed the sky: %v", got)
	}
	got := lightningSky(c, 2, 0)
	for i := range c {
		want := c[i]*0.55 + 0.8*0.45
		if math.Abs(float64(got[i]-want)) > 1e-6 {
			t.Fatalf("channel %d: %v, want %v", i, got[i], want)
		}
	}
	if half := lightningSky(c, 1, 0.5); math.Abs(float64(half[0]-(c[0]*(1-0.225)+0.8*0.225))) > 1e-6 {
		t.Fatalf("half-way flash %v", half)
	}
}
