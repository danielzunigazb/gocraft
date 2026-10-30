package client

import "math"

// More mobs, after ModelSpider and ModelSlime: the spider and its small cousin, and the slime.

func spiderModel() *mmodel {
	const f, f2 = math.Pi / 4, 0.3926991
	legBox := func(mirror bool) mbox {
		if mirror {
			return box(18, 0, -1, -1, -1, 16, 2, 2)
		}
		return box(18, 0, -15, -1, -1, 16, 2, 2)
	}
	type leg struct {
		px, pz, ry, rz float32
		right          bool
	}
	legs := []leg{
		{-4, 2, f2 * 2, -f, false}, {4, 2, -f2 * 2, f, true},
		{-4, 1, f2, -f * 0.74, false}, {4, 1, -f2, f * 0.74, true},
		{-4, 0, -f2, -f * 0.74, false}, {4, 0, f2, f * 0.74, true},
		{-4, -1, -f2 * 2, -f, false}, {4, -1, f2 * 2, f, true},
	}
	parts := []mpart{
		part(0, 15, -3, box(32, 4, -4, -4, -8, 8, 8, 8)),  // the head
		part(0, 15, 0, box(0, 0, -3, -3, -3, 6, 6, 6)),    // the neck
		part(0, 15, 9, box(0, 12, -5, -4, -6, 10, 8, 12)), // the body
	}
	for _, l := range legs {
		parts = append(parts, turned(part(l.px, 15, l.pz, legBox(l.right)), 0, l.ry, l.rz))
	}
	return newModel(64, 32, parts...)
}

// spiderPose is ModelSpider.setRotationAngles: the legs fan out and step in pairs.
func spiderPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 11)
	r[0] = rot{x: hp * deg, y: hy * deg}
	c := func(a float32) float32 { return -(cos32(ls*0.6662*2+a) * 0.4) * lsa }
	s := func(a float32) float32 { return float32(math.Abs(float64(sin32(ls*0.6662+a)*0.4))) * lsa }
	f3, f4, f5, f6 := c(0), c(pi), c(pi/2), c(3*pi/2)
	f7, f8, f9, f10 := s(0), s(pi), s(pi/2), s(3*pi/2)
	r[3].y, r[4].y = f3, -f3
	r[5].y, r[6].y = f4, -f4
	r[7].y, r[8].y = f5, -f5
	r[9].y, r[10].y = f6, -f6
	r[3].z, r[4].z = f7, -f7
	r[5].z, r[6].z = f8, -f8
	r[7].z, r[8].z = f9, -f9
	r[9].z, r[10].z = f10, -f10
	return r
}

func slimeModel() *mmodel {
	return newModel(64, 32,
		part(0, 0, 0, box(0, 16, -3, 17, -3, 6, 6, 6), box(32, 0, -3.25, 18, -3.5, 2, 2, 2), box(32, 4, 1.25, 18, -3.5, 2, 2, 2), box(32, 8, 0, 21, -3.5, 1, 1, 1)),
		part(0, 0, 0, box(0, 0, -4, 16, -4, 8, 8, 8)), // the outer, see-through body
	)
}

// villagerModel is ModelVillager: a big-nosed head, a robe, folded arms and two legs. A witch has the
// same body with a pointed hat and a mole on her nose.
func villagerModel(witch bool, texH float32) *mmodel {
	head := []mbox{box(0, 0, -4, -10, -4, 8, 10, 8), box(24, 0, -1, -1, -6, 2, 4, 2)}
	if witch {
		head = append(head,
			box(0, 64, -5, -10.03125, -5, 10, 2, 10), // the brim and the hat on it, one above the other
			box(0, 76, -3.25, -14.03125, -3, 7, 4, 7),
			box(0, 87, -1.5, -18.03125, -1, 4, 4, 4),
			boxX(0, 95, 0.25, -20.03125, 1, 1, 2, 1, -0.25),
			boxX(0, 0, 0, 3-1, -6.75, 1, 1, 1, -0.25)) // the mole
	}
	return newModel(64, texH,
		part(0, 0, 0, head...),
		part(0, 0, 0, box(16, 20, -4, 0, -3, 8, 12, 6), boxX(0, 38, -4, 0, -3, 8, 18, 6, 0.5)),
		turned(part(0, 3, -1, box(44, 22, -8, -2, -2, 4, 8, 4), mirrored(box(44, 22, 4, -2, -2, 4, 8, 4)), box(40, 38, -4, 2, -2, 8, 4, 4)), -0.75, 0, 0),
		part(-2, 12, 0, box(0, 22, -2, 0, -2, 4, 12, 4)),
		part(2, 12, 0, mirrored(box(0, 22, -2, 0, -2, 4, 12, 4))),
	)
}

func villagerPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	return []rot{{x: hp * deg, y: hy * deg}, {}, {}, {x: cos32(ls*armSw) * 1.4 * lsa * 0.5}, {x: cos32(ls*armSw+pi) * 1.4 * lsa * 0.5}}
}

// rabbitModel is ModelRabbit.
func rabbitModel() *mmodel {
	return newModel(64, 32,
		part(3, 17.5, 3.7, box(26, 24, -1, 5.5, -3.7, 2, 1, 7)), // left foot
		part(-3, 17.5, 3.7, box(8, 24, -1, 5.5, -3.7, 2, 1, 7)), // right foot
		part(3, 17.5, 3.7, box(30, 15, -1, 0, 0, 2, 4, 5)),      // left thigh
		part(-3, 17.5, 3.7, box(16, 15, -1, 0, 0, 2, 4, 5)),     // right thigh
		turned(part(0, 19, 8, box(0, 0, -3, -2, -10, 6, 5, 10)), -0.3490659, 0, 0),
		turned(part(3, 17, -1, box(8, 15, -1, 0, -1, 2, 7, 2)), -0.17453292, 0, 0),
		turned(part(-3, 17, -1, box(0, 15, -1, 0, -1, 2, 7, 2)), -0.17453292, 0, 0),
		part(0, 16, -1, box(32, 0, -2.5, -4, -5, 5, 4, 5)),
		turned(part(0, 16, -1, box(52, 0, -2.5, -9, -1, 2, 5, 1)), 0, -0.2617994, 0),
		turned(part(0, 16, -1, box(58, 0, 0.5, -9, -1, 2, 5, 1)), 0, 0.2617994, 0),
		turned(part(0, 20, 7, box(52, 6, -1.5, 0, 0, 3, 3, 2)), -0.3490659, 0, 0),
		part(0, 16, -1, box(32, 9, -0.5, -2.5, -5.5, 1, 1, 1)),
	)
}

// rabbitPose: the legs swing with the hops, the head looks.
func rabbitPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 12)
	k := cos32(ls*armSw) * 0.9 * lsa
	if m != nil && !m.onGround { // in the air the haunches stretch back
		k = float32(-min(max(m.vel[1], -0.4), 0.45) * 1.8)
	}
	r[0].x, r[1].x, r[2].x, r[3].x = k, k, k, k
	r[5].x, r[6].x = -k*0.6, -k*0.6
	for _, i := range []int{7, 8, 9, 11} { // the head, ears and nose turn together
		r[i].x, r[i].y = hp*deg, hy*deg
	}
	return r
}

// squidModel is ModelSquid: a big head and eight tentacles in a ring.
func squidModel() *mmodel {
	parts := []mpart{part(0, 8, 0, box(0, 0, -6, -8, -6, 12, 16, 12))}
	for i := 0; i < 8; i++ {
		d := float32(i) * pi * 2 / 8
		parts = append(parts, turned(part(cos32(d)*5, 15, sin32(d)*5, box(48, 0, -1, 0, -1, 2, 18, 2)), 0, float32(i)*pi*-2/8+pi/2, 0))
	}
	return newModel(64, 32, parts...)
}

func squidPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 9)
	for i := 1; i < 9; i++ {
		r[i].x = sin32(age*0.3+float32(i)*0.8)*0.5 + cos32(ls*armSw)*lsa*0.4 // and kick when it swims
	}
	return r
}

// batModel is ModelBat, flattened: a head with ears, a body and two wings of two parts each.
func batModel() *mmodel {
	return newModel(64, 64,
		part(0, 0, 0, box(0, 0, -3, -3, -3, 6, 6, 6), box(24, 0, -4, -6, -2, 3, 4, 1), mirrored(box(24, 0, 1, -6, -2, 3, 4, 1))),
		part(0, 0, 0, box(0, 16, -3, 4, -3, 6, 12, 6), box(0, 34, -5, 16, 0, 10, 6, 1)),
		part(0, 0, 0, box(42, 0, -12, 1, 1.5, 10, 16, 1)),          // right wing
		part(-12, 1, 1.5, box(24, 16, -8, 0, 0, 8, 12, 1)),         // its tip
		part(0, 0, 0, mirrored(box(42, 0, 2, 1, 1.5, 10, 16, 1))),  // left wing
		part(12, 1, 1.5, mirrored(box(24, 16, 0, 0, 0, 8, 12, 1))), // its tip
	)
}

func batPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	a := cos32(age*1.3)*pi*0.25 + sin32(ls)*lsa*0.1
	// The wing tips hang from the wing: their pivot moves with it.
	px, pz := float32(-12), float32(1.5)
	c, s := cos32(a), sin32(a)
	dx, dz := px*c+pz*s-px, -px*s+pz*c-pz
	return []rot{{x: hp * deg, y: hy * deg}, {}, {y: a}, {y: a * 1.5, dx: dx, dz: dz}, {y: -a}, {y: -a * 1.5, dx: -dx, dz: dz}}
}

func init() {
	spider := &mobKind{id: "Spider", w: 1.4, h: 0.9, health: 16, speed: 0.3, hostile: true, tex: "spider/spider", attack: 2, follow: 16,
		model: spiderModel(), pose: spiderPose, climbs: true, leaps: true, day: true}
	spider.dropsFn = func() []slot {
		out := []slot{{ID: 287, N: int8(rng.Intn(3))}}
		if rng.Intn(3) == 0 {
			out = append(out, slot{ID: 375, N: 1}) // a spider eye
		}
		return out
	}
	cave := &mobKind{id: "CaveSpider", w: 0.7, h: 0.5, health: 12, speed: 0.3, hostile: true, tex: "spider/cave_spider", attack: 2, follow: 16,
		model: spiderModel(), pose: spiderPose, climbs: true, leaps: true, day: true, scale: 0.7}
	cave.dropsFn = func() []slot { return []slot{{ID: 287, N: int8(rng.Intn(3))}} }
	slime := &mobKind{id: "Slime", w: 0.6, h: 0.6, health: 1, speed: 0.2, hostile: true, tex: "slime/slime", follow: 16,
		model: slimeModel(), slime: true,
		pose: func(m *mob, ls, lsa, age, hy, hp float32) []rot { return []rot{{}, {}} }}
	slime.dropsFn = func() []slot { return nil }
	villager := &mobKind{id: "Villager", w: 0.6, h: 1.8, health: 20, speed: 0.3, tex: "villager/farmer", model: villagerModel(false, 64), pose: villagerPose,
		texVariants: []string{"villager/farmer", "villager/librarian", "villager/priest", "villager/smith", "villager/butcher"}}
	villager.dropsFn = func() []slot { return nil }
	witch := &mobKind{id: "Witch", w: 0.6, h: 1.95, health: 26, speed: 0.25, tex: "witch", model: villagerModel(true, 128), pose: villagerPose}
	witch.dropsFn = func() []slot {
		var out []slot
		for _, id := range []uint16{370, 376, 374, 289, 348, 287, 280} { // a few of what a witch may leave
			if rng.Intn(3) == 0 {
				out = append(out, slot{ID: id, N: 1 + int8(rng.Intn(2))})
			}
		}
		return out
	}
	rabbit := &mobKind{id: "Rabbit", w: 0.6, h: 0.7, health: 3, speed: 0.3, tex: "rabbit/brown", model: rabbitModel(), pose: rabbitPose, hops: true,
		texVariants: []string{"rabbit/brown", "rabbit/white", "rabbit/black", "rabbit/white_splotched", "rabbit/gold", "rabbit/salt"}}
	rabbit.dropsFn = func() []slot {
		out := []slot{{ID: 415, N: int8(rng.Intn(2))}, {ID: 411, N: int8(rng.Intn(2))}} // hide and raw rabbit
		if rng.Intn(10) == 0 {
			out = append(out, slot{ID: 414, N: 1}) // the foot
		}
		return out
	}
	squid := &mobKind{id: "Squid", w: 0.95, h: 0.95, health: 10, speed: 0.2, tex: "squid", model: squidModel(), pose: squidPose, swims: true, yoff: -0.53}
	squid.dropsFn = func() []slot { return []slot{{ID: 351, N: int8(1 + rng.Intn(3))}} } // ink sacs
	bat := &mobKind{id: "Bat", w: 0.5, h: 0.9, health: 6, speed: 0.3, tex: "bat", model: batModel(), pose: batPose, flies: true, yoff: -0.35}
	bat.dropsFn = func() []slot { return nil }
	for _, k := range []*mobKind{spider, cave, slime, villager, witch, rabbit, squid, bat} {
		kinds[k.id] = k
	}
}
