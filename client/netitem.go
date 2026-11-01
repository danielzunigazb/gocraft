package client

import (
	"math"
)

// Dropped items on a server, after 1.8's client: the server spawns an item as an object (type 2), tells
// what it is with metadata 10 a moment later, and moves it with the usual entity packets. Between them
// the client runs the item's own physics (EntityItem.onUpdate runs on both sides), but merging, pickup
// and despawning are the server's: it says so with Collect Item and Destroy Entities. A collected item
// flies to whoever took it for three ticks (EntityPickupFX).

const (
	objItem       = 2
	pickupFXTicks = 3   // EntityPickupFX.maxAge
	pickupFXLift  = 0.5 // the item flies to the collector's feet plus this
)

// pickupFX is an item that was just collected, on its way to the one who took it.
type pickupFX struct {
	item itemEntity // as it was when it was taken
	to   int32      // the collector's entity id
	age  int
}

// netSpawnItem is Spawn Object for an item: an EntityItem at the server's position, with the motion of its
// constructor unless the packet carries one.
func (g *game) netSpawnItem(eid int32, at, vel vec3, hasVel bool) {
	e := &itemEntity{net: true, srv: at, pickupDelay: dropPickupDelay, hover: rng.Float32() * math.Pi * 2}
	if !hasVel {
		vel = vec3{float64(rng.Float32())*0.2 - 0.1, 0.2, float64(rng.Float32())*0.2 - 0.1}
	}
	e.vel = vel
	g.placeNetItem(e, at)
	e.prev = e.pos
	n := g.net
	if n.items == nil {
		n.items = map[int32]*itemEntity{}
	}
	if old := n.items[eid]; old != nil { // an id the server reuses: the old item is gone
		old.dead = true
	}
	if len(n.items) >= 4096 {
		return
	}
	n.items[eid] = e
	g.items = append(g.items, e)
}

// placeNetItem is Entity.setPositionAndRotation2 for an item: it is put where the server says, and lifted
// onto whatever block its box (narrowed by 1/32 on each side) would be inside.
func (g *game) placeNetItem(e *itemEntity, p vec3) {
	e.pos = p
	box := itemBox(p)
	box.min[0], box.max[0] = box.min[0]+1.0/32, box.max[0]-1.0/32
	box.min[2], box.max[2] = box.min[2]+1.0/32, box.max[2]-1.0/32
	top, hit := 0.0, false
	for _, b := range g.boxesIn(box) {
		if overlaps(box, b) {
			hit = true
			top = max(top, b.max[1])
		}
	}
	if hit {
		e.pos[1] = p[1] + top - box.min[1]
	}
}

// moveNetItem is a teleport (abs) or a relative move of an item: the server's own position is kept exact,
// in 32nds, and the item is put there.
func (g *game) moveNetItem(e *itemEntity, to vec3) {
	e.srv = to
	g.placeNetItem(e, to)
}

// netItem is the item the server knows by eid, if it is one.
func (n *netState) netItem(eid int32) *itemEntity {
	if n.items == nil {
		return nil
	}
	return n.items[eid]
}

// removeNetItem takes an item out of the world (Destroy Entities, or a pickup).
func (n *netState) removeNetItem(eid int32) {
	if e := n.netItem(eid); e != nil {
		e.dead = true
		delete(n.items, eid)
	}
}

// netCollect is Collect Item: the server gave an item (or an arrow, or an orb) to someone. A pop is heard
// where it lay, it flies to them and is gone (NetHandlerPlayClient.handleCollectItem).
func (g *game) netCollect(item, collector int32) {
	n := g.net
	if e := n.netItem(item); e != nil {
		g.sound("random.pop", e.pos, 0.2, ((rnd()-rnd())*0.7+1)*2)
		if len(n.pickups) < 256 {
			n.pickups = append(n.pickups, &pickupFX{item: *e, to: collector})
		}
		n.removeNetItem(item)
		return
	}
	if e := n.entity(item); e != nil { // an arrow picked up, or an orb of experience taken
		if e.obj == objOrb {
			g.sound("random.orb", e.pos, 0.1, (rnd()-rnd())*0.35+0.9)
		} else {
			g.sound("random.pop", e.pos, 0.2, ((rnd()-rnd())*0.7+1)*2)
		}
		delete(n.ents, item)
	}
}

// tickPickups ages the collected items; each lasts three ticks.
func (g *game) tickPickups() {
	live := g.net.pickups[:0]
	for _, p := range g.net.pickups {
		if p.age++; p.age < pickupFXTicks {
			live = append(live, p)
		}
	}
	clear(g.net.pickups[len(live):])
	g.net.pickups = live
}

// collectorAt is where the one who took an item stands, between the last tick and this one; the item goes
// to where it lay when the collector is not known (the game then uses the player, as here).
func (g *game) collectorAt(eid int32, partial float32) vec3 {
	f := float64(partial)
	if p := g.net.remote[eid]; p != nil && eid != g.net.eid {
		return p.prev.add(p.pos.sub(p.prev).scale(f))
	}
	return g.prevFeet.add(g.feet.sub(g.prevFeet).scale(f))
}

// pickupAt is where a collected item is drawn: from where it lay towards the collector, faster at the end
// (EntityPickupFX.renderParticle squares the fraction).
func (g *game) pickupAt(p *pickupFX, partial float32) vec3 {
	f := (float64(p.age) + float64(partial)) / pickupFXTicks
	f *= f
	to := g.collectorAt(p.to, partial)
	to[1] += pickupFXLift
	return p.item.pos.add(to.sub(p.item.pos).scale(f))
}
