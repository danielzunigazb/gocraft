package client

import "math"

// More of the mobs a server shows, with their models after 1.8's ModelBlaze, ModelIronGolem, ModelWolf and
// ModelEnderman: the NPCs of a lobby are often one of them.

// blazeModel is ModelBlaze: the head and twelve rods in three rings that turn about it.
func blazeModel() *mmodel {
	parts := []mpart{part(0, 0, 0, box(0, 0, -4, -4, -4, 8, 8, 8))}
	for i := 0; i < 12; i++ {
		parts = append(parts, part(0, 0, 0, box(0, 16, 0, 0, 0, 2, 8, 2)))
	}
	return newModel(64, 32, parts...)
}

func blazePose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 13)
	r[0] = rot{x: hp * deg, y: hy * deg}
	f := age * pi * -0.1
	for i := 0; i < 4; i++ {
		r[1+i] = rot{dx: cos32(f) * 9, dy: -2 + cos32((float32(i*2)+age)*0.25), dz: sin32(f) * 9}
		f++
	}
	f = pi/4 + age*pi*0.03
	for j := 4; j < 8; j++ {
		r[1+j] = rot{dx: cos32(f) * 7, dy: 2 + cos32((float32(j*2)+age)*0.25), dz: sin32(f) * 7}
		f++
	}
	f = 0.47123894 + age*pi*-0.05
	for k := 8; k < 12; k++ {
		r[1+k] = rot{dx: cos32(f) * 5, dy: 11 + cos32((float32(k)*1.5+age)*0.5), dz: sin32(f) * 5}
		f++
	}
	return r
}

// golemModel is ModelIronGolem, on a 128 x 128 texture.
func golemModel() *mmodel {
	return newModel(128, 128,
		part(0, -7, -2, box(0, 0, -4, -12, -5.5, 8, 10, 8), box(24, 0, -1, -5, -7.5, 2, 4, 2)),
		part(0, -7, 0, box(0, 40, -9, -2, -6, 18, 12, 11), boxX(0, 70, -4.5, 10, -3, 9, 5, 6, 0.5)),
		part(0, -7, 0, box(60, 21, -13, -2.5, -3, 4, 30, 6)),
		part(0, -7, 0, box(60, 58, 9, -2.5, -3, 4, 30, 6)),
		part(-4, 11, 0, box(37, 0, -3.5, -3, -3, 6, 16, 5)),
		part(5, 11, 0, mirrored(box(60, 0, -3.5, -3, -3, 6, 16, 5))),
	)
}

// golemSwing is ModelIronGolem.func_78172_a: a triangle wave of the walk, from -1 to 1.
func golemSwing(t, d float32) float32 {
	return (float32(math.Abs(math.Mod(float64(t), float64(d))-float64(d)*0.5)) - d*0.25) / (d * 0.25)
}

func golemPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	f := golemSwing(ls, 13)
	return []rot{{x: hp * deg, y: hy * deg}, {}, {x: (-0.2 + 1.5*f) * lsa}, {x: (-0.2 - 1.5*f) * lsa}, {x: -1.5 * f * lsa}, {x: 1.5 * f * lsa}}
}

// wolfModel is ModelWolf: the body and the mane lie along the animal, so they are boxes stood up and tipped over.
func wolfModel() *mmodel {
	return newModel(64, 32,
		part(-1, 13.5, -7, box(0, 0, -3, -3, -2, 6, 6, 4), box(16, 14, -3, -5, 0, 2, 2, 1), box(16, 14, 1, -5, 0, 2, 2, 1), box(0, 10, -1.5, 0, -5, 3, 3, 4)),
		turned(part(0, 14, 2, box(18, 14, -4, -2, -3, 6, 9, 6)), pi/2, 0, 0),
		turned(part(-1, 14, -3, box(21, 0, -4, -3, -3, 8, 6, 7)), pi/2, 0, 0),
		part(-2.5, 16, 7, box(0, 18, -1, 0, -1, 2, 8, 2)),
		part(0.5, 16, 7, box(0, 18, -1, 0, -1, 2, 8, 2)),
		part(-2.5, 16, -4, box(0, 18, -1, 0, -1, 2, 8, 2)),
		part(0.5, 16, -4, box(0, 18, -1, 0, -1, 2, 8, 2)),
		part(-1, 12, 8, box(9, 18, -1, 0, -1, 2, 8, 2)),
	)
}

func wolfPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	a, b := cos32(ls*armSw)*1.4*lsa, cos32(ls*armSw+pi)*1.4*lsa
	tail := pi / 5 // a wild wolf's tail; a tame one's rises with its health, an angry one's stands
	if m != nil && m.angry {
		tail = 1.5393804
	}
	return []rot{{x: hp * deg, y: hy * deg}, {}, {}, {x: a}, {x: b}, {x: b}, {x: a}, {x: tail}}
}

// endermanModel is ModelEnderman: a biped of 30-pixel arms and legs raised on 14 pixels, and a jaw on the head.
func endermanModel() *mmodel {
	return newModel(64, 32,
		part(0, -13, 0, box(0, 0, -4, -8, -4, 8, 8, 8)),
		part(0, -13, 0, boxX(0, 16, -4, -8, -4, 8, 8, 8, -0.5)),
		part(0, -14, 0, box(32, 16, -4, 0, -2, 8, 12, 4)),
		part(-3, -12, 0, box(56, 0, -1, -2, -1, 2, 30, 2)),
		part(5, -12, 0, mirrored(box(56, 0, -1, -2, -1, 2, 30, 2))),
		part(-2, -5, 0.1, box(56, 0, -1, 0, -1, 2, 30, 2)),
		part(2, -5, 0.1, mirrored(box(56, 0, -1, 0, -1, 2, 30, 2))),
	)
}

func endermanPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	clamp := func(v float32) float32 { return min(max(v*0.5, -0.4), 0.4) }
	return []rot{{x: hp * deg, y: hy * deg}, {x: hp * deg, y: hy * deg}, {},
		{x: clamp(cos32(ls*armSw+pi) * 2 * lsa * 0.5)}, {x: clamp(cos32(ls*armSw) * 2 * lsa * 0.5)},
		{x: clamp(cos32(ls*armSw) * 1.4 * lsa)}, {x: clamp(cos32(ls*armSw+pi) * 1.4 * lsa)}}
}

func init() {
	blaze := &mobKind{id: "Blaze", w: 0.6, h: 1.8, health: 20, speed: 0.23, hostile: true, tex: "blaze", model: blazeModel(), pose: blazePose, fullbright: true}
	golem := &mobKind{id: "VillagerGolem", w: 1.4, h: 2.7, health: 100, speed: 0.25, tex: "iron_golem", model: golemModel(), pose: golemPose}
	wolf := &mobKind{id: "Wolf", w: 0.6, h: 0.8, health: 8, speed: 0.3, tex: "wolf/wolf", model: wolfModel(), pose: wolfPose}
	ender := &mobKind{id: "Enderman", w: 0.6, h: 2.9, health: 40, speed: 0.3, tex: "enderman/enderman", model: endermanModel(), pose: endermanPose,
		fm: endermanModel(), fur: "enderman/enderman_eyes"}
	for _, k := range []*mobKind{blaze, golem, wolf, ender} {
		k.dropsFn = func() []slot { return nil }
		kinds[k.id] = k
	}
}
