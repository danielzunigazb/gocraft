package client

import (
	"math"

	"gocraft/worldgen"
)

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

// snowmanModel is ModelSnowMan: a head, two stick arms, a body and a bottom, each a box a half pixel smaller than its
// size says (the game gives them an expansion of -0.5), on a 64 x 64 sheet.
func snowmanModel() *mmodel {
	return newModel(64, 64,
		part(0, 4, 0, boxX(0, 0, -4, -8, -4, 8, 8, 8, -0.5)),
		part(0, 6, 0, boxX(32, 0, -1, 0, -1, 12, 2, 2, -0.5)),
		part(0, 6, 0, boxX(32, 0, -1, 0, -1, 12, 2, 2, -0.5)),
		part(0, 13, 0, boxX(0, 16, -5, -10, -5, 10, 10, 10, -0.5)),
		part(0, 24, 0, boxX(0, 36, -6, -12, -6, 12, 12, 12, -0.5)),
	)
}

// snowmanPose is ModelSnowMan.setRotationAngles: the head follows the look, the body a quarter of the way, and the
// arms stand out from its sides, turned with it.
func snowmanPose(m *mob, ls, lsa, age, hy, hp float32) []rot {
	by := hy * deg * 0.25
	f, f1 := sin32(by), cos32(by)
	return []rot{
		{x: hp * deg, y: hy * deg},
		{y: by, z: 1, dx: f1 * 5, dz: -f * 5},
		{y: pi + by, z: -1, dx: -f1 * 5, dz: f * 5},
		{y: by},
		{},
	}
}

func init() {
	snow := &mobKind{id: "SnowMan", w: 0.4, h: 1.8, health: 4, speed: 0.2, tex: "snowman", model: snowmanModel(), pose: snowmanPose}
	snow.dropsFn = func() []slot { return nil }
	snow.serverOnly = true
	kinds[snow.id] = snow
}

// emitSnowmanPumpkin is LayerSnowmanHead: a carved pumpkin on the head (the block model of id 86, 0.625 of a block
// across), moved 0.34375 up the head's frame and turned about it, carried by the head's own pose.
func (g *game) emitSnowmanPumpkin(groups map[string][]vertex, m *mob, head rot, pos vec3, yaw float32, light uint8) {
	vs := g.res.fast[86<<4|2].variants // meta 2 faces north: the carved face is on the item model's north side faces north: the carved face is on the item model's north side faces north: the carved face is on the item model's north side faces north: the carved face is on the item model's north side
	if len(vs) == 0 || len(vs[0].quads) == 0 {
		return
	}
	turn := partTurn(head)
	ya := float64(180-yaw) * math.Pi / 180
	yc, ys := math.Cos(ya), math.Sin(ya)
	world := func(x, y, z float32) (float64, float64, float64) { // a point of the head's frame, in 1/16, to the world
		wx, wy, wz := float64(-x)/16, 1.5078125-float64(y)/16, float64(z)/16
		return pos[0] + wx*yc + wz*ys, pos[1] + wy, pos[2] - wx*ys + wz*yc
	}
	for i := range vs[0].quads {
		q := &vs[0].quads[i]
		n := dirVec[q.dir]
		// the face's normal through the same turns, for its shade
		nx, ny, nz := float64(n[0])*-1, float64(n[1])*-1, float64(n[2]) // scale(0.625, -0.625, -0.625) then a half turn: x and y flip
		nx, nz = -nx, -nz                                               // ... and the half turn about y
		shade := entityShade(nx*yc+nz*ys, ny, -nx*ys+nz*yc)
		col := [4]uint8{uint8(255 * shade), uint8(255 * shade), uint8(255 * shade), light}
		for k := 0; k < 4; k++ {
			px, py, pz := q.apos[k][0]-0.5, q.apos[k][1]-0.5, q.apos[k][2]-0.5
			// scale(0.625, -0.625, -0.625), the half turn about y, then translate(0, -0.34375, 0), all in blocks
			lx, ly, lz := -0.625*px*16, (-0.625*py-0.34375)*16, 0.625*pz*16
			x := turn[0]*lx + turn[1]*ly + turn[2]*lz
			y := turn[3]*lx + turn[4]*ly + turn[5]*lz + head.dy + 4 // about the head's pivot, 4 pixels down the model
			z := turn[6]*lx + turn[7]*ly + turn[8]*lz
			wx, wy, wz := world(x, y, z)
			groups["!blocks"] = append(groups["!blocks"], vertex{float32(wx), float32(wy), float32(wz), q.auv[k][0], q.auv[k][1], col})
		}
	}
}
