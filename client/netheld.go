package client

import "math"

// What a player holds, seen from outside (LayerHeldItem): the item hangs from the right arm, moved to the
// hand, and turned by the "thirdperson" display of its model.

// itemXform is a model's display transform: rotation in degrees, translation in sixteenths, scale.
type itemXform struct{ rot, tr, sc [3]float32 }

func (x itemXform) matrix() mat4 {
	return translateM(x.tr[0]/16, x.tr[1]/16, x.tr[2]/16).then(rotYM(x.rot[1]), rotXM(x.rot[0]), rotZM(x.rot[2]), scaleM(x.sc[0], x.sc[1], x.sc[2]))
}

// heldModelName is which model an item is drawn with in the hand: a bow shows its drawing, a rod its line out.
func heldModelName(it slot, using bool, useTicks int, hookOut bool) string {
	switch {
	case it.ID == 261 && using:
		return bowSprite(useTicks)
	case it.ID == 346 && hookOut:
		return "fishing_rod_cast"
	}
	return itemModelName(it)
}

// heldMatrix puts the item's own space (the picture centred on the origin, one block across) into the world
// around a player at pos: the entity's turn and scale, the right arm as posed, the hand, the model's display.
func heldMatrix(rots []rot, body float32, sneaking, slim bool, display mat4) mat4 {
	r := rots[pRArm]
	const rad2deg = 180 / math.Pi
	pivotY := float32(2)
	if slim {
		pivotY = 2.5
	}
	arm := translateM((-5+r.dx)/16, (pivotY+r.dy)/16, r.dz/16).then(rotZM(r.z*rad2deg), rotYM(r.y*rad2deg), rotXM(r.x*rad2deg))
	m := rotYM(180-body).then(scaleM(-1, -1, 1), scaleM(playerScale, playerScale, playerScale), translateM(0, -1.5078125, 0),
		arm, translateM(-0.0625, 0.4375, 0.0625))
	if sneaking {
		m = m.then(translateM(0, 0.203125, 0))
	}
	return m.then(display, scaleM(0.5, 0.5, 0.5))
}

// emitHeld adds the item in a player's hand to the groups.
func (g *game) emitHeld(groups map[string][]vertex, it slot, rots []rot, pos vec3, body float32, sneaking, slim, using bool, useTicks int, hookOut bool, light uint8, far bool) {
	if it.empty() {
		return
	}
	add := func(key string, m mat4, q fpQuad, tint [3]float32) {
		nw := m.dir(q.n)
		s := itemShade(nw)
		col := [4]uint8{toByte(tint[0] * s), toByte(tint[1] * s), toByte(tint[2] * s), light}
		for i := 0; i < 4; i++ {
			p := m.point(q.p[i])
			groups[key] = append(groups[key], vertex{float32(pos[0] + p[0]), float32(pos[1] + p[1]), float32(pos[2] + p[2]), q.uv[i][0], q.uv[i][1], col})
		}
	}
	if it.isBlock() {
		vs := g.res.fast[int(it.state())&4095].variants
		if len(vs) == 0 || len(vs[0].quads) == 0 {
			return
		}
		m := heldMatrix(rots, body, sneaking, slim, ident())
		if flatBlocks[it.ID] { // a plant, a torch: its picture, with the display of the item model of the block
			q0 := vs[0].quads[0]
			tint := white
			if q0.tint >= 0 {
				tint = g.res.tintFor(it.ID, int(it.Dmg), 1)
			}
			for _, q := range g.hand.sprite(g.res.atlas, q0.tile) {
				add("!blocks", m, q, tint)
			}
			return
		}
		for i := range vs[0].quads { // the unit cube, halved
			q := &vs[0].quads[i]
			tint := white
			if q.tint >= 0 {
				tint = g.res.tintFor(it.ID, int(it.Dmg), 1)
			}
			var fq fpQuad
			for k := 0; k < 4; k++ {
				fq.p[k] = vec3{float64(q.apos[k][0] - 0.5), float64(q.apos[k][1] - 0.5), float64(q.apos[k][2] - 0.5)}
				fq.uv[k] = q.auv[k]
			}
			fq.n = vec3{float64(dirVec[q.dir][0]), float64(dirVec[q.dir][1]), float64(dirVec[q.dir][2])}
			add("!blocks", m, fq, tint)
		}
		return
	}
	name := heldModelName(it, using, useTicks, hookOut)
	tile, ok := g.res.itemTiles[name]
	if !ok {
		if tile, ok = g.res.itemTile(it); !ok {
			return
		}
	}
	x, found := g.res.itemThird[name]
	if !found { // an item model with no display of its own: a flat item held at the hand
		x = itemXform{tr: [3]float32{0, 3, 1}, sc: [3]float32{0.55, 0.55, 0.55}}
	}
	m := heldMatrix(rots, body, sneaking, slim, x.matrix())
	quads := g.hand.sprite(g.res.items, tile)
	if far && len(quads) > 2 { // too far to see the item's thickness: only its two faces
		quads = quads[:2]
	}
	for _, q := range quads {
		add("!items", m, q, white)
	}
}
