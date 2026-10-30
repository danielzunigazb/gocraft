package client

import "math"

// Projectiles the server shows: arrows, snowballs, eggs, ender pearls, potions. The server sends their position
// now and then and sets it directly (they are not interpolated, as in the game); in between, the client flies
// them from their velocity, as EntityArrow and EntityThrowable do, so that they move smoothly.

const (
	objArrow      = 60
	objSnowball   = 61
	objEgg        = 62
	objFireball   = 63
	objSmallFire  = 64
	objEnderPearl = 65
	objEyeOfEnder = 72
	objPotion     = 73
	objExpBottle  = 75
)

// thrownItem is the item an object is drawn as, for the throwables: RenderSnowball shows the item.
func thrownItem(obj byte, data int32) (slot, bool) {
	switch obj {
	case objSnowball:
		return slot{ID: 332, N: 1}, true
	case objEgg:
		return slot{ID: 344, N: 1}, true
	case objEnderPearl:
		return slot{ID: 368, N: 1}, true
	case objEyeOfEnder:
		return slot{ID: 381, N: 1}, true
	case objExpBottle:
		return slot{ID: 384, N: 1}, true
	case objPotion:
		return slot{ID: 373, Dmg: uint16(data), N: 1}, true
	}
	return slot{}, false
}

func isProjectile(obj byte) bool {
	return obj == objArrow || obj == objFireball || obj == objSmallFire || obj >= objSnowball && obj <= objEnderPearl || obj == objEyeOfEnder || obj == objPotion || obj == objExpBottle
}

// tickProjectile flies one projectile a tick.
func (g *game) tickProjectile(e *netEntity) {
	if e.stuck {
		return
	}
	if e.vel[0] == 0 && e.vel[1] == 0 && e.vel[2] == 0 && e.obj != objArrow {
		return
	}
	gravity, drag := 0.03, 0.99
	switch e.obj {
	case objArrow:
		gravity = arrowGravity
	case objPotion:
		gravity = 0.05
	case objExpBottle:
		gravity = 0.07
	case objFireball, objSmallFire:
		gravity, drag = 0, 0.95
	}
	next := e.pos.add(e.vel)
	if e.obj == objArrow { // an arrow stops in the block it hits
		if h, ok := g.rayTrace(e.pos, next); ok {
			e.pos, e.stuck = h.at, true
			return
		}
	}
	e.pos = next
	if st := g.world.Block(int32(math.Floor(e.pos[0])), int32(math.Floor(e.pos[1])), int32(math.Floor(e.pos[2]))) >> 4; st == 8 || st == 9 {
		drag = 0.6
		if e.obj != objArrow {
			drag = 0.8
		}
	}
	e.vel = e.vel.scale(drag)
	e.vel[1] -= gravity
}

// camRightUp is the camera's right and up, for pictures that face it.
func (g *game) camRightUp() (right, up vec3) {
	f := look(g.yaw, g.pitch)
	right = f.cross(vec3{0, 1, 0}).norm()
	return right, right.cross(f)
}

// emitProjectiles lays out the arrows and the thrown items.
func (g *game) emitProjectiles(groups map[string][]vertex) {
	n := g.net
	if n == nil {
		return
	}
	right, up := g.camRightUp()
	for _, e := range n.ents {
		if !isProjectile(e.obj) {
			continue
		}
		p := e.at(g.partial)
		l := g.world.Light(int32(math.Floor(p[0])), int32(math.Floor(p[1])), int32(math.Floor(p[2])))
		if e.obj == objArrow {
			a := &arrow{pos: e.pos, prev: e.prev, vel: e.vel}
			groups["arrow"] = arrowGeometry(groups["arrow"], a, g.partial, l)
			continue
		}
		it, ok := thrownItem(e.obj, e.owner)
		if !ok {
			continue
		}
		name := itemModelName(it)
		if e.obj == objPotion {
			name = "bottle_splash"
		}
		tile, ok := g.res.itemTiles[name]
		if !ok {
			continue
		}
		u0, v0 := g.res.items.coord(tile, 0, 0)
		u1, v1 := g.res.items.coord(tile, 16, 16)
		groups["!items"] = emitBillboard(groups["!items"], p, right, up, 0.25, u0, v0, u1, v1, l)
	}
}
