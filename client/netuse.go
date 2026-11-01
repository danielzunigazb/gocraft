package client

import "math"

// Right click on a server that is not gocraft's own. The server decides what a click does, and lobbies are
// made of it: a compass in the hand opens the game selector, a button or a chest answers, an NPC talks.
// So the click is only sent, as the game sends it, and nothing is changed here until the server says.

// entityTarget is the player or entity pointed at within reach 3 (not behind a block), if any.
func (g *game) entityTarget() (int32, bool) {
	n := g.net
	best, bestD, found := int32(0), float32(math.Inf(1)), false
	if p, d, ok := g.playerTarget(); ok {
		best, bestD, found = p.eid, d, true
	}
	for _, e := range n.ents {
		if e.obj != 0 || e.height() == 0 { // arrows and hooks cannot be pointed at; a marker armor stand has no box
			continue
		}
		if d, ok := g.rayBoxDist(e.box()); ok && d < bestD {
			best, bestD, found = e.eid, d, true
		}
	}
	if found {
		end := g.pos.add(look(g.yaw, g.pitch).scale(3))
		if h, ok := g.rayTrace(g.pos, end); ok && float32(h.at.sub(g.pos).dot(h.at.sub(g.pos))) < bestD {
			return 0, false
		}
	}
	return best, found
}

// useNet is the use click on a third-party server: an entity, a block, or the item in the air.
func (g *game) useNet() {
	n := g.net
	held, ex := g.held(), n.extra[36+g.sel]
	if eid, ok := g.entityTarget(); ok {
		// Minecraft.rightClickMouse: the entity is offered the click, and if it does not take it the item is used
		// as it would be in the air. So a rod is cast at a rival's face and a sword blocks with one in front.
		g.sendInteract(eid)
		if g.interactConsumes(eid) {
			return
		}
	} else if h, ok := g.target(); ok {
		w := newPacket(0x08)
		w.Long(packPos(h.pos[0], h.pos[1], h.pos[2]))
		w.Byte(byte(h.side))
		putSlotWire(w, held, ex)
		for k := 0; k < 3; k++ { // where on the face, in sixteenths
			w.Byte(byte(min(max(int((h.at[k]-float64(h.pos[k]))*16), 0), 15)))
		}
		n.c.send(w)
		g.swingTicks = 6
		g.sendSwing()
		if held.empty() || held.isBlock() {
			return
		}
		// An item that is not a block did nothing to the block, so the game goes on to use it: the click on
		// a block and the click in the air both go out (that is how a rod is cast at the ground).
	}
	if held.empty() {
		return
	}
	w := newPacket(0x08)
	w.Long(packPos(-1, -1, -1))
	w.Byte(255)
	putSlotWire(w, held, ex)
	w.Byte(0)
	w.Byte(0)
	w.Byte(0)
	n.c.send(w)
	if held.ID == 346 { // ItemFishingRod swings the arm as it casts and as it reels in
		if n.hookOf(n.eid) == nil { // casting (a hook that is out is reeled in, silently): the client plays the cast
			g.sound("random.bow", g.pos, 0.5, 0.4/(rnd()*0.4+0.8))
		}
		g.swingTicks = 6
		g.sendSwing()
	}
}

// sendInteract is what the game sends when the player right clicks an entity (PlayerControllerMP.func_178894_a and
// interactWithEntitySendPacket): Use Entity "interact at" with the point of the entity that was hit, then "interact".
func (g *game) sendInteract(eid int32) {
	var hit vec3
	if box, ok := g.entityBox(eid); ok {
		feet := vec3{(box.min[0] + box.max[0]) / 2, box.min[1], (box.min[2] + box.max[2]) / 2} // the entity's position
		box.min, box.max = box.min.sub(vec3{0.1, 0.1, 0.1}), box.max.add(vec3{0.1, 0.1, 0.1})  // the game grows the box by a tenth
		if at, _, ok := segmentBox(box, g.pos, g.pos.add(look(g.yaw, g.pitch).scale(3))); ok {
			hit = at.sub(feet)
		}
	}
	w := newPacket(0x02)
	w.VarInt(eid)
	w.VarInt(2) // interact at
	w.Float(float32(hit[0]))
	w.Float(float32(hit[1]))
	w.Float(float32(hit[2]))
	g.net.c.send(w)
	w = newPacket(0x02)
	w.VarInt(eid)
	w.VarInt(0) // interact
	g.net.c.send(w)
}

// entityBox is the box of a player or entity the server shows, for aiming at it.
func (g *game) entityBox(eid int32) (aabb, bool) {
	if p := g.net.remote[eid]; p != nil {
		return playerBox(p.pos), true
	}
	if e := g.net.entity(eid); e != nil {
		return e.box(), true
	}
	return aabb{}, false
}

// interactConsumes is whether the entity takes a right click for itself, as the game's client works it out (the
// entity's interactFirst and interactAt): a villager trades, a horse, a boat and a minecart are mounted, an armor
// stand swaps its equipment. Anything else, a player above all, leaves the click to the item in the hand.
func (g *game) interactConsumes(eid int32) bool {
	e := g.net.entity(eid)
	if e == nil {
		return false
	}
	switch {
	case e.obj == 1 || e.obj == 10: // a boat, a minecart
		return true
	case e.obj != 0:
		return false
	case e.kind == 120 || e.kind == 100: // a villager, a horse
		return true
	case e.kind == mobArmorStand:
		return !e.marker
	}
	return false
}

// thirdParty is whether the server is not gocraft's own: it decides what using an item does, and the client
// only shows it (no arrow is shot, no food eaten, no block placed here).
func (g *game) thirdParty() bool { return g.net != nil && !g.net.gocraft }

// usingItem is whether an item is in use: a bow drawn, food being eaten, a sword held across the chest.
func (g *game) usingItem() bool { return g.bowTicks > 0 || g.eating > 0 || g.blocking }
