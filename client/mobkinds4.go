package client

import "gocraft/worldgen"

// Two more of the mobs a server shows, with their models after 1.8's ModelMagmaCube and ModelGhast (read off the
// bytecode): the magma cube, a slime of lava, and the ghast, a big floating head with nine tentacles.

// magmaModel is ModelMagmaCube: eight one-pixel slices of the body, stacked (two of them from another part of the
// sheet) and a core of 4 pixels.
func magmaModel() *mmodel {
	parts := make([]mpart, 0, 9)
	for i := 0; i < 8; i++ {
		u, v := float32(0), float32(i)
		switch i {
		case 2:
			u, v = 24, 10
		case 3:
			u, v = 24, 19
		}
		parts = append(parts, part(0, 0, 0, box(u, v, -4, float32(16+i), -4, 8, 1, 8)))
	}
	parts = append(parts, part(0, 0, 0, box(0, 16, -2, 18, -2, 4, 4, 4)))
	return newModel(64, 32, parts...)
}

// ghastModel is ModelGhast: the body, a cube of 16, and nine tentacles hung under it in a grid of three by three,
// each as long as Random(1660) says (8 to 14 pixels). The whole model is moved down 0.6 of a block, which is 9.6
// pixels, by ModelGhast.render.
func ghastModel() *mmodel {
	const down = 9.6
	parts := []mpart{part(0, 8+down, 0, box(0, 0, -8, -8, -8, 16, 16, 16))}
	r := worldgen.NewRandom(1660)
	for j := 0; j < 9; j++ {
		x := (float32(j%3)-float32(j/3%2)*0.5+0.25)/2*2 - 1
		z := float32(j/3)/2*2 - 1
		length := float32(r.IntN(7) + 8)
		parts = append(parts, part(x*5, 15+down, z*5, box(0, 0, -1, 0, -1, 2, length, 2)))
	}
	return newModel(64, 32, parts...)
}

func ghastPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	r := make([]rot, 10)
	for i := 1; i < 10; i++ {
		r[i] = rot{x: 0.2*sin32(age*0.3+float32(i-1)) + 0.4}
	}
	return r
}

func init() {
	magma := &mobKind{id: "LavaSlime", w: 0.6, h: 0.6, health: 1, speed: 0.2, hostile: true, tex: "slime/magmacube", follow: 16,
		model: magmaModel(), slime: true, fullbright: true,
		pose: func(m *mob, ls, lsa, age, hy, hp float32) []rot { return make([]rot, 9) }}
	// a ghast is 4 blocks across and drawn at 4.5 times its model (RenderGhast); the second texture is the one it
	// has while it is about to shoot
	ghast := &mobKind{id: "Ghast", w: 4, h: 4, health: 10, speed: 0.2, hostile: true, tex: "ghast/ghast", model: ghastModel(), pose: ghastPose,
		scale: 4.5, texVariants: []string{"ghast/ghast", "ghast/ghast_shooting"}}
	for _, k := range []*mobKind{magma, ghast} {
		k.dropsFn = func() []slot { return nil }
		k.serverOnly = true
		kinds[k.id] = k
	}
}
