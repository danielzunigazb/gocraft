package client

// More of the mobs a server shows, drawn after 1.8's own models (read off the bytecode of ModelSilverfish and
// ModelEnderMite): the silverfish and the endermite, two little worms of a few segments that wriggle as they go,
// the giant, a zombie six times as big (RenderGiantZombie scales by 6), and the mooshroom, a cow in red. The
// mooshroom's mushrooms on its back (LayerMooshroomMushrooms) are not drawn yet.

// segmentWorm is what ModelSilverfish and ModelEnderMite share: the segments of a worm, each a box laid along
// the body, side by side from the tail, which wriggle by their own phase. size is {width, height, depth} of each
// and tex the corner of its texture. The returned z is where each sits along the body.
func segmentWorm(size [][3]float32, tex [][2]float32) (parts []mpart, z []float32) {
	f := float32(-3.5)
	for i := range size {
		s := size[i]
		parts = append(parts, part(0, 24-s[1], f, box(tex[i][0], tex[i][1], s[0]*-0.5, 0, s[2]*-0.5, s[0], s[1], s[2])))
		z = append(z, f)
		if i < len(size)-1 {
			f += (s[2] + size[i+1][2]) * 0.5
		}
	}
	return parts, z
}

var silverfishSize = [][3]float32{{3, 2, 2}, {4, 3, 2}, {6, 4, 3}, {3, 3, 3}, {2, 2, 3}, {2, 1, 2}, {1, 1, 2}}

// silverfishModel is ModelSilverfish: seven segments and three flat layers across them that look like wings.
func silverfishModel() *mmodel {
	parts, z := segmentWorm(silverfishSize, [][2]float32{{0, 0}, {0, 4}, {0, 9}, {0, 16}, {0, 22}, {11, 0}, {13, 4}})
	d := func(i int) float32 { return silverfishSize[i][2] }
	parts = append(parts,
		part(0, 16, z[2], box(20, 0, -5, 0, d(2)*-0.5, 10, 8, d(2))),
		part(0, 20, z[4], box(20, 11, -3, 0, d(4)*-0.5, 6, 4, d(4))),
		part(0, 19, z[1], box(20, 18, -3, 0, d(4)*-0.5, 6, 5, d(1))))
	return newModel(64, 32, parts...)
}

// The two worms wriggle the same way: each segment turns about y and slides sideways by a sine with its own phase,
// the more the farther it is from the third (the middle one). ModelSilverfish turns by 0.05 and slides by 0.2,
// ModelEnderMite by 0.01 and 0.1.

func silverfishPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 10)
	for i := 0; i < 7; i++ {
		k := float32(abs(int32(i) - 2))
		r[i] = rot{y: cos32(age*0.9+float32(i)*0.15*pi) * pi * 0.05 * (1 + k), dx: sin32(age*0.9+float32(i)*0.15*pi) * pi * 0.2 * k}
	}
	// the layers take the turn of the segment they sit on (the first one only its turn, the others its slide too)
	r[7] = rot{y: r[2].y}
	r[8] = rot{y: r[4].y, dx: r[4].dx}
	r[9] = rot{y: r[1].y, dx: r[1].dx}
	return r
}

func endermiteModel() *mmodel {
	parts, _ := segmentWorm([][3]float32{{4, 3, 2}, {6, 4, 5}, {3, 3, 1}, {1, 2, 1}}, [][2]float32{{0, 0}, {0, 5}, {0, 14}, {0, 18}})
	return newModel(64, 32, parts...)
}

func endermitePose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 4)
	for i := range r {
		k := float32(abs(int32(i) - 2))
		r[i] = rot{y: cos32(age*0.9+float32(i)*0.15*pi) * pi * 0.01 * (1 + k), dx: sin32(age*0.9+float32(i)*0.15*pi) * pi * 0.1 * k}
	}
	return r
}

func init() {
	silverfish := &mobKind{id: "Silverfish", w: 0.4, h: 0.3, health: 8, speed: 0.6, hostile: true, tex: "silverfish", model: silverfishModel(), pose: silverfishPose}
	endermite := &mobKind{id: "Endermite", w: 0.4, h: 0.3, health: 8, speed: 0.25, hostile: true, tex: "endermite", model: endermiteModel(), pose: endermitePose}
	// RenderGiantZombie: the zombie's model and picture, drawn six times as big; the box is EntityGiantZombie's
	// 0.6 x 1.8 times 6
	giant := &mobKind{id: "Giant", w: 3.6, h: 10.8, health: 100, speed: 0.5, hostile: true, tex: "zombie/zombie", model: biped(64, false), pose: bipedPose(true), scale: 6}
	cow := kinds["Cow"]
	mooshroom := &mobKind{id: "MushroomCow", w: cow.w, h: cow.h, health: 10, speed: 0.2, tex: "cow/mooshroom", model: cow.model, pose: quadPose}
	for _, k := range []*mobKind{silverfish, endermite, giant, mooshroom} {
		k.dropsFn = func() []slot { return nil }
		k.serverOnly = true
		kinds[k.id] = k
	}
}
