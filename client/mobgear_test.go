package client

import "testing"

func gearGame(t *testing.T, kind byte, pieces map[int16]uint16) (*game, *netEntity) {
	g, _, apply := mobGame(t)
	g.partial = 1
	apply(spawnMobPacket(50, kind, 3.5, 4, 3.5, 0, 0, nil))
	for sl, id := range pieces {
		apply(equipPacket(50, sl, id))
	}
	e := g.net.entity(50)
	e.mob.m.prev = e.mob.m.pos
	return g, e
}

// What a server puts on a zombie or skeleton (Entity Equipment) is kept on the mob and drawn: the armour in its two
// sizes (three pieces on the outer layer, the leggings on the inner one) and the item in the right hand.
func TestMobEquipmentIsDrawn(t *testing.T) {
	g, e := gearGame(t, 54, map[int16]uint16{4: 306, 3: 307, 2: 308, 1: 309, 0: 267})
	m := e.mob.m
	if m.armor[3].ID != 306 || m.armor[2].ID != 307 || m.armor[1].ID != 308 || m.armor[0].ID != 309 || m.held.ID != 267 {
		t.Fatalf("armour %v held %v", m.armor, m.held)
	}
	groups := map[string][]vertex{}
	g.emitMob(groups, m, 1)
	if n := len(groups["armor/iron_layer_1"]); n != 24+72+48 { // helmet, chestplate (body and arms), boots (legs)
		t.Fatalf("outer layer has %d vertices, want %d", n, 24+72+48)
	}
	if n := len(groups["armor/iron_layer_2"]); n != 72 { // leggings: body and legs
		t.Fatalf("inner layer has %d vertices, want 72", n)
	}
	if len(groups["!items"]) == 0 {
		t.Fatal("the sword is not drawn")
	}
	var sum float64 // a mob facing south (+z) has its right hand on the -x side
	for _, v := range groups["!items"] {
		sum += float64(v.X)
	}
	if mean := sum/float64(len(groups["!items"])) - 3.5; mean > -0.15 || mean < -0.5 {
		t.Fatalf("the sword is %v off the mob's middle, want it in the right hand (about -0.3)", mean)
	}
	// taking one off
	g.net.entity(50).mob.m.armor[3] = slot{}
	groups = map[string][]vertex{}
	g.emitMob(groups, m, 1)
	if n := len(groups["armor/iron_layer_1"]); n != 72+48 {
		t.Fatalf("without the helmet: %d vertices", n)
	}
}

// A skeleton's armour has the thin limbs of ModelSkeleton, a zombie's the full ones: at rest a chestplate spans the
// body and arms, 14 pixels over a skeleton's and 18 over a zombie's.
func TestSkeletonArmourHasThinLimbs(t *testing.T) {
	span := func(thin bool) float64 {
		groups := map[string][]vertex{}
		var rest [12]rot
		emitArmorScaled(groups, [4]slot{2: {ID: 307, N: 1}}, [4]itemExtra{}, rest[:], vec3{}, 0, 0, [3]uint8{255, 255, 255}, 255, armorScale{body: 1, thin: thin})
		lo, hi := standBounds(groups["armor/iron_layer_1"])
		return float64(hi[0] - lo[0])
	}
	if z, s := span(false), span(true); !near(z, 18.0/16, 1e-4) || !near(s, 14.0/16, 1e-4) {
		t.Fatalf("armour spans %v (zombie) and %v (skeleton), want %v and %v", z, s, 18.0/16, 14.0/16)
	}
	mob := func(kind byte) float64 {
		g, e := gearGame(t, kind, map[int16]uint16{3: 307})
		groups := map[string][]vertex{}
		g.emitMob(groups, e.mob.m, 1)
		lo, hi := standBounds(groups["armor/iron_layer_1"])
		return float64(hi[0] - lo[0])
	}
	if mob(51) >= mob(54) {
		t.Fatalf("a skeleton's armour (%v) is not thinner than a zombie's (%v)", mob(51), mob(54))
	}
}

// A mob gocraft draws but that wears nothing shows nothing extra, and what is not a biped keeps its model alone.
func TestMobWithoutGearDrawsNoExtras(t *testing.T) {
	g, e := gearGame(t, 54, nil)
	groups := map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	for key := range groups {
		if len(key) > 5 && key[:5] == "armor" || key == "!items" {
			t.Fatalf("an undressed zombie drew %q", key)
		}
	}
	g, e = gearGame(t, 90, map[int16]uint16{4: 306, 0: 267}) // a pig is not dressed
	groups = map[string][]vertex{}
	g.emitMob(groups, e.mob.m, 1)
	if len(groups["armor/iron_layer_1"]) != 0 || len(groups["!items"]) != 0 {
		t.Fatal("a pig was dressed")
	}
}
