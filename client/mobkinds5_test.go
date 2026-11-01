package client

import (
	"testing"

	"gocraft/proto"
)

// Where ModelSilverfish and ModelEnderMite put their segments: from -3.5 along the body, each half its own
// depth and half the next one's farther, and 24 minus its height down. (Worked out from the bytecode's arrays.)
func TestWormSegmentsLieAlongTheBody(t *testing.T) {
	s := silverfishModel()
	if len(s.parts) != 10 {
		t.Fatalf("%d parts, want 7 segments and 3 layers", len(s.parts))
	}
	z := []float32{-3.5, -1.5, 1, 4, 7, 9.5, 11.5}
	y := []float32{22, 21, 20, 21, 22, 23, 23}
	for i := 0; i < 7; i++ {
		if s.parts[i].pz != z[i] || s.parts[i].py != y[i] || s.parts[i].px != 0 {
			t.Errorf("segment %d at (%v,%v,%v), want (0,%v,%v)", i, s.parts[i].px, s.parts[i].py, s.parts[i].pz, y[i], z[i])
		}
	}
	// the layers sit on the third, fifth and second segments, at heights 16, 20 and 19
	for i, w := range [][2]float32{{16, z[2]}, {20, z[4]}, {19, z[1]}} {
		if p := s.parts[7+i]; p.py != w[0] || p.pz != w[1] {
			t.Errorf("layer %d at y %v z %v, want %v", i, p.py, p.pz, w)
		}
	}
	e := endermiteModel()
	if len(e.parts) != 4 {
		t.Fatalf("endermite has %d parts", len(e.parts))
	}
	for i, w := range [][2]float32{{21, -3.5}, {20, 0}, {21, 3}, {22, 4}} {
		if p := e.parts[i]; p.py != w[0] || p.pz != w[1] {
			t.Errorf("endermite segment %d at y %v z %v, want %v", i, p.py, p.pz, w)
		}
	}
}

func TestWormsWriggle(t *testing.T) {
	r := silverfishPose(nil, 0, 0, 0, 0, 0)
	// at age 0: the ends turn by cos 0 * pi * 0.05 * (1 + distance from the third) and the third does not slide
	if !near(r[0].y, 3.1415927*0.05*3, 1e-5) || r[2].dx != 0 || r[0].dx != 0 {
		t.Fatalf("segments %+v", r[:3])
	}
	r = silverfishPose(nil, 0, 0, 10, 0, 0)
	if r[0].dx == 0 || r[7].y != r[2].y || r[8].dx != r[4].dx || r[9].y != r[1].y {
		t.Fatalf("poses %+v: the layers follow their segments", r)
	}
	if r[7].dx != 0 { // the first layer takes only the turn of the third segment, which never slides
		t.Fatalf("layer 0 slides by %v", r[7].dx)
	}
	e := endermitePose(nil, 0, 0, 0, 0, 0)
	if len(e) != 4 || !near(e[0].y, 3.1415927*0.01*3, 1e-5) {
		t.Fatalf("endermite %+v", e)
	}
}

// Spawn Mob of each draws it, with the box and size the game gives it.
func TestMoreServerMobsAreDrawn(t *testing.T) {
	for _, c := range []struct {
		typ    byte
		id     string
		w, h   float32
		tex    string
		scale  float32
		extent float64 // how tall the drawn model is at most, in blocks
	}{
		{60, "Silverfish", 0.4, 0.3, "silverfish", 0, 0.5},
		{67, "Endermite", 0.4, 0.3, "endermite", 0, 0.5},
		{53, "Giant", 3.6, 10.8, "zombie/zombie", 6, 12},
		{96, "MushroomCow", 0.9, 1.3, "cow/mooshroom", 0, 2},
	} {
		g, _, apply := mobGame(t)
		g.partial = 1
		apply(spawnMobPacket(70, c.typ, 4.5, 4, 4.5, 0, 0, func(*proto.Writer) {}))
		e := g.net.entity(70)
		if e == nil || e.mob == nil || e.mob.m.kind.id != c.id {
			t.Fatalf("type %d: %+v", c.typ, e)
		}
		if w, h := e.mob.dims(); w != c.w || h != c.h {
			t.Errorf("%s box %v x %v, want %v x %v", c.id, w, h, c.w, c.h)
		}
		e.mob.m.prev = e.mob.m.pos
		groups := map[string][]vertex{}
		g.emitMob(groups, e.mob.m, 1)
		vs := groups[c.tex]
		if len(vs) == 0 {
			t.Errorf("%s draws nothing with %s (groups %v)", c.id, c.tex, len(groups))
			continue
		}
		lo, hi := standBounds(vs)
		if float64(hi[1]-lo[1]) > c.extent || hi[1]-lo[1] <= 0 {
			t.Errorf("%s is %v high", c.id, hi[1]-lo[1])
		}
	}
}
