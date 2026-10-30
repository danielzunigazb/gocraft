package client

import (
	"math"

	"gocraft/world"
)

// What a click does, with 1.8's timing: holding the use button places every 4 ticks, holding the
// attack button breaks every 6 in creative.

const (
	useDelayTicks   = 4
	breakDelayTicks = 6
	sneakEye        = 1.54
)

func (g *game) reachDist() float32 {
	if g.creative {
		return 5
	}
	return 4.5
}

func (g *game) eyeOffset() float32 {
	if g.sneaking {
		return sneakEye
	}
	return eyeHeight
}

// target is the block the player points at: a ray from the eyes through the world's blocks, as the
// game traces it, as far as the reach.
func (g *game) target() (rayHit, bool) {
	look := look(g.yaw, g.pitch)
	return g.rayTrace(g.pos, g.pos.add(look.scale(float64(g.reachDist()))))
}

func (g *game) breakBlock() {
	h, ok := g.target()
	if !ok {
		return
	}
	if g.world.Block(h.pos[0], h.pos[1], h.pos[2])>>4 != 7 { // bedrock stays
		g.beBroken(h.pos)
		g.setBlock(h.pos[0], h.pos[1], h.pos[2], 0)
		g.sendDig(h.pos[0], h.pos[1], h.pos[2], h.side)
	}
}

// replaceable is Block.isReplaceable: what a placed block takes the place of instead of going
// beside it. Air, liquids, tall grass, dead bushes, vines, fire, a single snow layer, and the
// grass and fern of double plants.
func (g *game) replaceable(x, y, z int32) bool {
	st := g.world.Block(x, y, z)
	switch id := st >> 4; {
	case id == 0 || isFluid(id) || id == 31 || id == 32 || id == 106 || id == 51:
		return true
	case id == 78:
		return st&7 == 0
	case id == 175:
		lower := st
		if st&8 != 0 {
			lower = g.world.Block(x, y-1, z)
		}
		return lower>>4 == 175 && (lower&7 == 2 || lower&7 == 3)
	}
	return false
}

func overlaps(a, b aabb) bool {
	return a.max[0] > b.min[0] && a.min[0] < b.max[0] && a.max[1] > b.min[1] && a.min[1] < b.max[1] && a.max[2] > b.min[2] && a.min[2] < b.max[2]
}

// anyBlock reports whether a block's collision box overlaps the box.
func (g *game) anyBlock(box aabb) bool {
	for _, b := range g.boxesIn(box) {
		if overlaps(box, b) {
			return true
		}
	}
	return false
}

// canPlace refuses a block whose boxes would stand inside the player, as the game does.
func (g *game) canPlace(x, y, z int32, state uint16) bool {
	var nb nbFunc
	if neighbourDependent[state>>4] {
		nb = func(dx, dy, dz int32) uint16 { return g.world.Block(x+dx, y+dy, z+dz) }
	}
	o := vec3{float64(x), float64(y), float64(z)}
	pb := playerBox(g.feet)
	for _, b := range g.res.collisionBoxes(state, nb) {
		if overlaps(pb, aabb{o.add(box6lo(b)), o.add(box6hi(b))}) {
			return false
		}
	}
	return true
}

// placedState is Block.onBlockPlaced: the state a block takes from the face clicked, where on
// that face and which way the player looks. Stairs face the player and sit in the half clicked,
// slabs go to the half clicked, logs lie along the face's axis, torches lean on it.
func placedState(item uint16, side int, hitY float32, yawDeg float32) uint16 {
	id, meta := item>>4, item&15
	upperHalf := side == dDown || (side != dUp && hitY > 0.5)
	switch {
	case stairIDs[id]:
		// getHorizontalFacing: 0 south, 1 west, 2 north, 3 east, by yaw.
		facing := int(math.Floor(float64(yawDeg)/90+0.5)) & 3
		m := [4]uint16{2, 1, 3, 0}[facing] // stair metadata: east 0, west 1, south 2, north 3
		if upperHalf {
			m |= 4
		}
		return id<<4 | m
	case id == 44 || id == 126 || id == 182:
		m := meta & 7
		if upperHalf {
			m |= 8
		}
		return id<<4 | m
	case id == 17 || id == 162:
		axis := [6]uint16{0, 0, 8, 8, 4, 4}[side] // up and down: y; north and south: z; west and east: x
		return id<<4 | (meta & 3) | axis
	case id == 50:
		return id<<4 | [6]uint16{5, 5, 4, 3, 2, 1}[side]
	case id == idChest || id == idTrappedChest || id == idEnderChest || id == idFurnace || id == idLitFurnace || id == 23 || id == 158:
		return id<<4 | chestFacing(yawDeg) // faces the player who placed it
	}
	return item
}

// slabKind is the double slab a single slab makes, and whether the state is a single slab.
var doubleSlab = map[uint16]uint16{44: 43, 126: 125, 182: 181}

func (g *game) useBlock() {
	if g.net != nil && !g.net.gocraft { // another server decides what a click does
		g.useNet()
		return
	}
	if g.useOnMob() { // shears, a bucket: a click for a mob
		return
	}
	h, ok := g.target()
	if !ok {
		return
	}
	if !g.sneaking && g.openBlockEntity(h.pos) { // a chest or a furnace opens instead of placing
		return
	}
	if g.world.Block(h.pos[0], h.pos[1], h.pos[2])>>4 == 58 && !g.sneaking { // a crafting table opens instead of placing
		g.openContainer(contTable)
		return
	}
	if g.useFlintAndSteel(h) {
		return
	}
	h0 := g.held()
	if h0.empty() || !h0.isBlock() {
		return // items that are not blocks are used in other ways
	}
	st := h0.state()
	hitY := float32(h.at[1] - float64(h.pos[1]))
	clicked := g.world.Block(h.pos[0], h.pos[1], h.pos[2])

	// A slab onto the matching half of a slab of its own kind makes a double slab.
	if d, isSlab := doubleSlab[st>>4]; isSlab && clicked>>4 == st>>4 && clicked&7 == st&7 {
		bottom := clicked&8 == 0
		if (bottom && h.side == dUp) || (!bottom && h.side == dDown) {
			if g.canPlace(h.pos[0], h.pos[1], h.pos[2], d<<4|st&7) {
				g.setBlock(h.pos[0], h.pos[1], h.pos[2], d<<4|st&7)
				g.sendPlace(h.pos[0], h.pos[1], h.pos[2], h.side)
				g.consume()
			}
			return
		}
	}

	// onItemUse: the block clicked is replaced if it can be; otherwise the block goes beside it.
	x, y, z := h.pos[0], h.pos[1], h.pos[2]
	if !g.replaceable(x, y, z) {
		o := dirVec[h.side]
		x, y, z = x+o[0], y+o[1], z+o[2]
	}
	if !world.InRange(y) || !g.replaceable(x, y, z) {
		return
	}
	placed := placedState(st, h.side, hitY, g.yawDeg())
	if !g.canPlace(x, y, z, placed) {
		return
	}
	g.setBlock(x, y, z, placed)
	g.afterPlace(x, y, z)
	g.sendPlace(h.pos[0], h.pos[1], h.pos[2], h.side)
	g.consume()
}

func (g *game) pickBlock() { // creative only
	h, ok := g.target()
	if !ok || !g.creative {
		return
	}
	if st := g.world.Block(h.pos[0], h.pos[1], h.pos[2]); st != 0 {
		g.inv[g.sel] = blockSlot(st, 1)
	}
}

// tickActions repeats a held button once its delay has run out.
func (g *game) tickActions() {
	g.useDelay = max(g.useDelay-1, 0)
	g.breakDelay = max(g.breakDelay-1, 0)
	if g.screen != screenNone || g.win == nil || g.opt.Bench > 0 { // a benchmark never clicks
		return
	}
	was := g.usingItem()
	g.tickBow()
	g.tickEating()
	class, _ := toolOf(g.held())
	g.blocking = class == "sword" && g.keyDown("use") && g.screen == screenNone
	// As in the game, the click repeats only while no item is in use: a drawn bow is not asked for again.
	if g.keyDown("use") && g.useDelay == 0 && !(g.thirdParty() && was) {
		g.useDelay = useDelayTicks
		g.useBlock()
	}
	if g.creative && g.keyDown("attack") && g.breakDelay == 0 {
		g.breakDelay = breakDelayTicks
		g.breakBlock()
	}
	if !g.creative && g.breakDelay == 0 {
		g.mine()
	}
}

// edgeClamp is a sneaking player's edge protection: the move is cut back in 0.05 steps until the
// body would still have ground under it a block down, first along x, then z, then both.
func (g *game) edgeClamp(box aabb, mv vec3) vec3 {
	const step = 0.05
	bare := func(x, z float64) bool { return !g.anyBlock(box.offset(vec3{x, -1, z})) }
	cut := func(v float64) float64 {
		switch {
		case v < step && v >= -step:
			return 0
		case v > 0:
			return v - step
		}
		return v + step
	}
	for mv[0] != 0 && bare(mv[0], 0) {
		mv[0] = cut(mv[0])
	}
	for mv[2] != 0 && bare(0, mv[2]) {
		mv[2] = cut(mv[2])
	}
	for mv[0] != 0 && mv[2] != 0 && bare(mv[0], mv[2]) {
		mv[0], mv[2] = cut(mv[0]), cut(mv[2])
	}
	return mv
}

// dropHeld throws one item of the held stack, or all of it.
func (g *game) dropHeld(all bool) {
	s := &g.inv[g.sel]
	if s.empty() {
		return
	}
	n := int8(1)
	if all {
		n = s.N
	}
	g.throwItem(slot{s.ID, s.Dmg, n})
	if s.N -= n; s.N <= 0 {
		*s = slot{}
	}
}
