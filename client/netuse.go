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
		h := e.height()
		if h == 0 { // a marker armor stand has no box to hit
			continue
		}
		b := aabb{e.pos.sub(vec3{0.3, 0, 0.3}), e.pos.add(vec3{0.3, h, 0.3})}
		if d, ok := g.rayBoxDist(b); ok && d < bestD {
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
	if eid, ok := g.entityTarget(); ok {
		w := newPacket(0x02)
		w.VarInt(eid)
		w.VarInt(0) // interact
		n.c.send(w)
		g.swingTicks = 6
		g.sendSwing()
		return
	}
	held, ex := g.held(), n.extra[36+g.sel]
	if h, ok := g.target(); ok {
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
		g.swingTicks = 6
		g.sendSwing()
	}
}

// thirdParty is whether the server is not gocraft's own: it decides what using an item does, and the client
// only shows it (no arrow is shot, no food eaten, no block placed here).
func (g *game) thirdParty() bool { return g.net != nil && !g.net.gocraft }

// usingItem is whether an item is in use: a bow drawn, food being eaten, a sword held across the chest.
func (g *game) usingItem() bool { return g.bowTicks > 0 || g.eating > 0 || g.blocking }
