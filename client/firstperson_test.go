package client

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestHandRestingPosition(t *testing.T) {
	// ItemRenderer.transformFirstPersonItem: at rest the item origin is at (0.56, -0.52, -0.72),
	// a fully dipped hand (equip 1) is 0.6 lower, and the scale is 0.4.
	m := transformFirstPersonItem(0, 0)
	if p := m.point(vec3{}); !near(p[0], 0.56, 1e-5) || !near(p[1], -0.52, 1e-5) || !near(p[2], -0.72, 1e-5) {
		t.Fatalf("rest origin %v", p)
	}
	if p := transformFirstPersonItem(1, 0).point(vec3{}); !near(p[1], -1.12, 1e-5) {
		t.Fatalf("dipped hand y %v, want -1.12", p[1])
	}
	if p := m.point(vec3{1, 0, 0}).sub(m.point(vec3{})); !near(p.dot(p), 0.16, 1e-5) {
		t.Fatalf("one unit of the item spans %v squared, want 0.4 squared", p.dot(p))
	}
}

func TestSwingCurves(t *testing.T) {
	sq, root := swingTerms(0)
	if sq != 0 || root != 0 {
		t.Fatalf("no swing: %v %v", sq, root)
	}
	sq, root = swingTerms(0.5)
	// sin(0.25 pi) and sin(sqrt(0.5) pi)
	if !near(sq, float32(math.Sin(math.Pi/4)), 1e-5) || !near(root, float32(math.Sin(math.Sqrt(0.5)*math.Pi)), 1e-5) {
		t.Fatalf("half swing: %v %v", sq, root)
	}
	// The item goes forward and across at the middle of a swing: f = -0.4 sin(sqrt(s) pi), f2 = -0.2 sin(s pi).
	p := swingTranslation(0.5).point(vec3{})
	if !near(p[0], -0.4*math.Sin(math.Sqrt(0.5)*math.Pi), 1e-5) || !near(p[2], -0.2, 1e-5) {
		t.Fatalf("swing translation %v", p)
	}
	if p := swingTranslation(0).point(vec3{}); p != (vec3{}) {
		t.Fatalf("a swing of 0 moves the item: %v", p)
	}
}

func TestBowSprite(t *testing.T) {
	for used, want := range map[int]string{0: "bow", 1: "bow_pulling_0", 13: "bow_pulling_0", 14: "bow_pulling_1", 17: "bow_pulling_1", 18: "bow_pulling_2", 40: "bow_pulling_2"} {
		if got := bowSprite(used); got != want {
			t.Errorf("drawn for %d ticks: %s, want %s", used, got, want)
		}
	}
}

func TestEatPose(t *testing.T) {
	// The first tick of a meal leaves the food where it was; from 80% of the meal on it no longer
	// bobs, and by the end it has come up to the mouth: 0.6 across and 0.5 down.
	if p := eatPose(32).point(vec3{}); !near(p[0], 0, 1e-5) || !near(p[1], 0, 1e-5) {
		t.Fatalf("start %v", p)
	}
	k := 1 - math.Pow(30.0/32, 27)
	if p := eatPose(30).point(vec3{}); !near(p[1], -0.5*k, 1e-4) || !near(p[0], 0.6*k, 1e-4) {
		t.Fatalf("no bob past 80%%, only the approach: %v", p)
	}
	if p := eatPose(1).point(vec3{}); p[0] < 0.55 || p[1] > -0.4 {
		t.Fatalf("late position %v", p)
	}
}

func TestSameForHand(t *testing.T) {
	sword, sword2 := slot{ID: 267, Dmg: 0, N: 1}, slot{ID: 267, Dmg: 30, N: 1}
	if !sameForHand(sword, sword2) {
		t.Error("a sword wearing down dips the hand")
	}
	if sameForHand(sword, slot{ID: 268, N: 1}) {
		t.Error("another item does not dip the hand")
	}
	if !sameForHand(slot{ID: 1, Dmg: 0, N: 64}, slot{ID: 1, Dmg: 0, N: 63}) {
		t.Error("a stack shrinking dips the hand")
	}
	if sameForHand(slot{ID: 35, Dmg: 0, N: 1}, slot{ID: 35, Dmg: 1, N: 1}) {
		t.Error("another colour of wool does not dip the hand")
	}
	if sameForHand(slot{}, sword) || sameForHand(sword, slot{}) {
		t.Error("an empty hand and a full one are the same")
	}
}

func TestEquipDipsAndRises(t *testing.T) {
	g := testGame(t)
	g.hand.item, g.hand.equip = slot{ID: 1, N: 1}, 1
	g.inv[0], g.sel = slot{ID: 1, N: 1}, 0
	g.tickHand()
	if g.hand.equip != 1 {
		t.Fatalf("the same item moved the hand: %v", g.hand.equip)
	}
	g.inv[0] = slot{ID: 267, N: 1}
	min := float32(1)
	for i := 0; i < 8; i++ {
		g.tickHand()
		if g.hand.equip < min {
			min = g.hand.equip
		}
	}
	if min > 0.2 {
		t.Fatalf("the hand did not go down for the new item: lowest %v", min)
	}
	if g.hand.item.ID != 267 || g.hand.equip < 0.99 {
		t.Fatalf("item %v equip %v: the new item did not come up", g.hand.item, g.hand.equip)
	}
	// Each tick moves it by at most 0.4.
	g.inv[0] = slot{ID: 1, N: 1}
	prev := g.hand.equip
	g.tickHand()
	if prev-g.hand.equip > 0.4+1e-5 {
		t.Fatalf("dipped by %v in a tick", prev-g.hand.equip)
	}
}

func TestExtrudeSprite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.SetRGBA(5, 5, color.RGBA{200, 200, 200, 255})
	a := &atlas{img: img, cols: 1, tile: 16}
	qs := extrude(a, 0)
	// Front and back, and a rim on all four sides of the lone texel.
	if len(qs) != 6 {
		t.Fatalf("%d faces, want 6", len(qs))
	}
	for _, q := range qs[:2] {
		for _, p := range q.p {
			if math.Abs(float64(p[2])) != 1.0/32 {
				t.Fatalf("a face at depth %v, want a sixteenth thick", p[2])
			}
		}
	}
	img.SetRGBA(6, 5, color.RGBA{200, 200, 200, 255}) // two side by side: the shared edge has no rim
	if qs := extrude(a, 0); len(qs) != 2+6 {
		t.Fatalf("%d faces for two texels, want 8", len(qs))
	}
}

func TestHandGeometry(t *testing.T) {
	g := testGame(t)
	g.pos = vec3{0.5, 5.62, 0.5}
	// The arm: the box and the sleeve, six faces each.
	passes := g.handGeometry(0)
	if len(passes) != 1 || len(passes[0].verts) != 2*6*4 {
		t.Fatalf("arm: %d passes, %d vertices", len(passes), len(passes[0].verts))
	}
	// Every vertex is in front of the camera (-z), within a metre.
	for _, v := range passes[0].verts {
		if v.Z > -0.05 || v.Z < -3 {
			t.Fatalf("arm vertex at z %v", v.Z)
		}
	}
	// A block: its model's faces.
	g.hand.item = slot{ID: 1, N: 1}
	if p := g.handGeometry(0); len(p) != 1 || len(p[0].verts) < 24 {
		t.Fatalf("block: %v", p)
	}
	// An item: front, back and a rim; and the bow changes picture with the draw.
	g.hand.item = slot{ID: 267, N: 1}
	if p := g.handGeometry(0); len(p) != 1 || len(p[0].verts) < 4*10 {
		t.Fatalf("sword: %d passes", len(p))
	}
	g.hand.item, g.bowTicks = slot{ID: 261, N: 1}, 0
	idle := len(g.handGeometry(0)[0].verts)
	g.bowTicks = 20
	if len(g.handGeometry(0)[0].verts) == 0 || idle == 0 {
		t.Fatal("no bow drawn")
	}
	if _, ok := g.res.itemTiles["bow_pulling_2"]; !ok {
		t.Fatal("the drawn bow's picture is not in the item atlas")
	}
}

func TestViewBobIsStillAtRest(t *testing.T) {
	m := viewBob(3.3, 0, 0)
	if p := m.point(vec3{1, 2, 3}); !near(p[0], 1, 1e-5) || !near(p[1], 2, 1e-5) || !near(p[2], 3, 1e-5) {
		t.Fatalf("a bob of 0 moves things: %v", p)
	}
	if p := viewBob(0.5, 0.1, 0).point(vec3{}); p[1] >= 0 {
		t.Fatalf("a bobbing view should dip: %v", p)
	}
}

func TestArrowGeometryIsRenderArrow(t *testing.T) {
	a := &arrow{pos: vec3{10, 20, 30}, vel: vec3{0, 0, 1}}
	a.prev = a.pos
	v := arrowGeometry(nil, a, 0, 0xF0)
	// Four shaft strips and two end caps.
	if len(v) != 6*4 {
		t.Fatalf("%d vertices, want 24", len(v))
	}
	for i, x := range v {
		// The shaft's picture is u 0..0.5, v 0..0.15625; the cap's u 0..0.15625, v 0.15625..0.3125.
		if i < 16 {
			if x.U < 0 || x.U > 0.5 || x.V < 0 || x.V > 0.15625 {
				t.Fatalf("shaft vertex %d has uv %v,%v", i, x.U, x.V)
			}
		} else if x.U < 0 || x.U > 0.15625 || x.V < 0.15625 || x.V > 0.3125 {
			t.Fatalf("cap vertex %d has uv %v,%v", i, x.U, x.V)
		}
		if x.C != [4]uint8{255, 255, 255, 0xF0} {
			t.Fatalf("vertex %d colour %v: the arrow does not glow", i, x.C)
		}
	}
	// Along the flight the shaft runs from 12 units behind the position to 4 ahead, at 0.05625 each.
	lo, hi := float32(100), float32(-100)
	for _, x := range v[:16] {
		lo, hi = min(lo, x.Z-30), max(hi, x.Z-30)
	}
	if !near(lo, -12*0.05625, 1e-3) || !near(hi, 4*0.05625, 1e-3) {
		t.Fatalf("the shaft spans %v..%v, want %v..%v", lo, hi, -12*0.05625, 4*0.05625)
	}
	// It is 4 units across.
	for _, x := range v[:4] {
		if d := math.Hypot(float64(x.X-10), float64(x.Y-20)); d > 2*0.05625+0.01 {
			t.Fatalf("a shaft vertex is %v across", d)
		}
	}
}
