package client

import "math"

// The mobs of the game that live in the player's worlds, with their models after 1.8's ModelZombie,
// ModelSkeleton, ModelCreeper, ModelPig, ModelCow, ModelSheep1/2 and ModelChicken.

const (
	pi    = float32(math.Pi)
	deg   = float32(math.Pi / 180)
	cosf  = 0
	armSw = float32(0.6662)
)

func cos32(x float32) float32 { return float32(math.Cos(float64(x))) }
func sin32(x float32) float32 { return float32(math.Sin(float64(x))) }

// biped is ModelBiped: head, body, two arms, two legs. Thin arms and legs make a skeleton.
func biped(texH float32, thin bool) *mmodel {
	aw, lw := float32(4), float32(4)
	ax, lx := float32(-3), float32(-2)
	if thin {
		aw, lw, ax, lx = 2, 2, -1, -1
	}
	ax2 := ax
	if !thin {
		ax2 = -1
	}
	return newModel(64, texH,
		part(0, 0, 0, box(0, 0, -4, -8, -4, 8, 8, 8)),
		part(0, 0, 0, box(16, 16, -4, 0, -2, 8, 12, 4)),
		part(-5, 2, 0, box(40, 16, ax, -2, lx, aw, 12, aw)),
		part(5, 2, 0, mirrored(box(40, 16, ax2, -2, lx, aw, 12, aw))),
		part(-1.9, 12, 0, box(0, 16, lx, 0, lx, lw, 12, lw)),
		part(1.9, 12, 0, mirrored(box(0, 16, lx, 0, lx, lw, 12, lw))),
	)
}

func bipedPose(zombie bool) func(m *mob, ls, lsa, age, hy, hp float32) []rot {
	return func(m *mob, ls, lsa, age, hy, hp float32) []rot {
		r := make([]rot, 6)
		r[0] = rot{x: hp * deg, y: hy * deg}
		r[2].x = cos32(ls*armSw+pi) * 2 * lsa * 0.5
		r[3].x = cos32(ls*armSw) * 2 * lsa * 0.5
		r[4].x = cos32(ls*armSw) * 1.4 * lsa
		r[5].x = cos32(ls*armSw+pi) * 1.4 * lsa
		if zombie { // arms held out in front
			r[2].x, r[3].x = -pi/2, -pi/2
		}
		sway := cos32(age*0.09)*0.05 + 0.05
		r[2].z += sway
		r[3].z -= sway
		r[2].x += sin32(age*0.067) * 0.05
		r[3].x -= sin32(age*0.067) * 0.05
		return r
	}
}

func quadruped(legH, headY, headZ, bodyY float32, head []mbox, body []mbox, extraBody ...mbox) *mmodel {
	legY := 24 - legH
	bodyParts := append([]mbox{}, body...)
	bodyParts = append(bodyParts, extraBody...)
	return newModel(64, 32,
		part(0, headY, headZ, head...),
		turned(part(0, bodyY, 2, bodyParts...), pi/2, 0, 0),
		part(-3, legY, 7, box(0, 16, -2, 0, -2, 4, legH, 4)),
		part(3, legY, 7, box(0, 16, -2, 0, -2, 4, legH, 4)),
		part(-3, legY, -5, box(0, 16, -2, 0, -2, 4, legH, 4)),
		part(3, legY, -5, box(0, 16, -2, 0, -2, 4, legH, 4)),
	)
}

func quadPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	return []rot{{x: hp * deg, y: hy * deg}, {}, {x: cos32(ls*armSw) * 1.4 * lsa}, {x: cos32(ls*armSw+pi) * 1.4 * lsa},
		{x: cos32(ls*armSw+pi) * 1.4 * lsa}, {x: cos32(ls*armSw) * 1.4 * lsa}}
}

func init() {
	zombie := &mobKind{id: "Zombie", w: 0.6, h: 1.95, health: 20, speed: 0.23, hostile: true, tex: "zombie/zombie", attack: 3, follow: 40,
		model: biped(64, false), pose: bipedPose(true)}
	zombie.dropsFn = func() []slot { return []slot{{ID: 367, N: int8(rng.Intn(3))}} }

	skeleton := &mobKind{id: "Skeleton", w: 0.6, h: 1.95, health: 20, speed: 0.25, hostile: true, tex: "skeleton/skeleton", attack: 2, follow: 16,
		model: biped(32, true), pose: bipedPose(false)}
	skeleton.dropsFn = func() []slot { return []slot{{ID: 352, N: int8(rng.Intn(3))}, {ID: 262, N: int8(rng.Intn(3))}} }

	creeper := &mobKind{id: "Creeper", w: 0.6, h: 1.7, health: 20, speed: 0.25, hostile: true, tex: "creeper/creeper", follow: 16,
		model: newModel(64, 32,
			part(0, 6, 0, box(0, 0, -4, -8, -4, 8, 8, 8)),
			part(0, 6, 0, box(16, 16, -4, 0, -2, 8, 12, 4)),
			part(-2, 18, 4, box(0, 16, -2, 0, -2, 4, 6, 4)),
			part(2, 18, 4, box(0, 16, -2, 0, -2, 4, 6, 4)),
			part(-2, 18, -4, box(0, 16, -2, 0, -2, 4, 6, 4)),
			part(2, 18, -4, box(0, 16, -2, 0, -2, 4, 6, 4))),
		pose: func(m *mob, ls, lsa, age, hy, hp float32) []rot {
			return []rot{{x: hp * deg, y: hy * deg}, {}, {x: cos32(ls*armSw) * 1.4 * lsa}, {x: cos32(ls*armSw+pi) * 1.4 * lsa},
				{x: cos32(ls*armSw+pi) * 1.4 * lsa}, {x: cos32(ls*armSw) * 1.4 * lsa}}
		}}
	creeper.dropsFn = func() []slot { return []slot{{ID: 289, N: int8(rng.Intn(3))}} }

	pig := &mobKind{id: "Pig", w: 0.9, h: 0.9, health: 10, speed: 0.25, tex: "pig/pig", pose: quadPose,
		model: quadruped(6, 12, -6, 11,
			[]mbox{box(0, 0, -4, -4, -8, 8, 8, 8), box(16, 16, -2, 0, -9, 4, 3, 1)}, []mbox{box(28, 8, -5, -10, -7, 10, 16, 8)})}
	pig.dropsFn = func() []slot { return []slot{{ID: 319, N: int8(1 + rng.Intn(3))}} }

	cow := &mobKind{id: "Cow", w: 0.9, h: 1.3, health: 10, speed: 0.2, tex: "cow/cow", pose: quadPose,
		model: quadruped(12, 4, -8, 5,
			[]mbox{box(0, 0, -4, -4, -6, 8, 8, 6), box(22, 0, -5, -5, -4, 1, 3, 1), box(22, 0, 4, -5, -4, 1, 3, 1)},
			[]mbox{box(18, 4, -6, -10, -7, 12, 18, 10)}, box(52, 0, -2, 2, -8, 4, 6, 1))}
	cow.model.parts[2].px, cow.model.parts[3].px, cow.model.parts[4].px, cow.model.parts[5].px = -4, 4, -4, 4
	cow.model.parts[2].pz, cow.model.parts[3].pz, cow.model.parts[4].pz, cow.model.parts[5].pz = 7, 7, -6, -6
	cow.dropsFn = func() []slot { return []slot{{ID: 334, N: int8(rng.Intn(3))}, {ID: 363, N: int8(1 + rng.Intn(3))}} }

	sheep := &mobKind{id: "Sheep", w: 0.9, h: 1.3, health: 8, speed: 0.23, tex: "sheep/sheep", fur: "sheep/sheep_fur", pose: quadPose,
		model: quadruped(12, 6, -8, 5,
			[]mbox{box(0, 0, -3, -4, -6, 6, 6, 8)}, []mbox{box(28, 8, -4, -10, -7, 8, 16, 6)}),
		fm: quadruped(12, 6, -8, 5,
			[]mbox{boxX(0, 0, -3, -4, -4, 6, 6, 6, 0.6)}, []mbox{boxX(28, 8, -4, -10, -7, 8, 16, 6, 1.75)})}
	for i := 2; i < 6; i++ {
		sheep.model.parts[i].py, sheep.fm.parts[i].py = 12, 12
		sheep.fm.parts[i].boxes = []mbox{boxX(0, 16, -2, 0, -2, 4, 6, 4, 0.5)}
	}
	sheep.fm = newModel(64, 32, sheep.fm.parts...)
	sheep.dropsFn = func() []slot { return []slot{{ID: 35, N: 1}} }

	chicken := &mobKind{id: "Chicken", w: 0.3, h: 0.7, health: 4, speed: 0.25, tex: "chicken",
		model: newModel(64, 32,
			part(0, 15, -4, box(0, 0, -2, -6, -2, 4, 6, 3), box(14, 0, -2, -4, -4, 4, 2, 2), box(14, 4, -1, -2, -3, 2, 2, 2)),
			turned(part(0, 16, 0, box(0, 9, -3, -4, -3, 6, 8, 6)), pi/2, 0, 0),
			part(-2, 19, 1, box(26, 0, -1, 0, -3, 3, 5, 3)),
			part(1, 19, 1, box(26, 0, -1, 0, -3, 3, 5, 3)),
			part(-4, 13, 0, box(24, 13, 0, 0, -3, 1, 4, 6)),
			part(4, 13, 0, box(24, 13, -1, 0, -3, 1, 4, 6))),
		pose: func(m *mob, ls, lsa, age, hy, hp float32) []rot {
			return []rot{{x: hp * deg, y: hy * deg}, {}, {x: cos32(ls*armSw) * 1.4 * lsa}, {x: cos32(ls*armSw+pi) * 1.4 * lsa}, {}, {}}
		}}
	chicken.dropsFn = func() []slot { return []slot{{ID: 288, N: int8(rng.Intn(3))}, {ID: 365, N: 1}} }

	// A practice opponent: the player's own model with an iron sword's blow (6), swinging every
	// half second. Not spawned by nature; F7 puts one in front of you.
	bot := &mobKind{id: "PvPBot", w: playerW, h: 1.8, health: 20, speed: 0.3, hostile: true, tex: "look/skin", attack: 7, follow: 48, interval: 10,
		pose: botPose} // its model is the player's, set once that exists (playermodel.go)
	bot.dropsFn = func() []slot { return nil }

	cow.flee, chicken.flee = 2.0, 1.4 // EntityAIPanic(1.25) for the sheep and the pig, (2.0) the cow, (1.4) the chicken

	for _, k := range []*mobKind{zombie, skeleton, creeper, pig, cow, sheep, chicken, bot} {
		kinds[k.id] = k
	}
}

// fleece colours, by dye damage 0..15, as RGB.
var fleece = [16][3]uint8{{255, 255, 255}, {217, 128, 51}, {179, 77, 217}, {102, 153, 217}, {230, 230, 51}, {128, 204, 26}, {242, 128, 166},
	{77, 77, 77}, {153, 153, 153}, {77, 128, 153}, {128, 64, 179}, {51, 77, 179}, {102, 77, 51}, {102, 128, 51}, {153, 51, 51}, {26, 26, 26}}
