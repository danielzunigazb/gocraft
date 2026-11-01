package client

import "math"

// Fighting other players on a server. The server owns health and decides every blow; the client
// aims, tells the server what it did (the attack, sprinting, blocking) and takes what comes back:
// its health, the knockback, and who got hurt.

// rayBoxDist is how far, squared, along the player's view a box is hit within reach 3, and whether
// it is. A box the eye is inside counts as at distance 0, as in the game.
func (g *game) rayBoxDist(b aabb) (float32, bool) {
	end := g.pos.add(look(g.yaw, g.pitch).scale(3))
	b.min, b.max = b.min.sub(vec3{0.1, 0.1, 0.1}), b.max.add(vec3{0.1, 0.1, 0.1}) // the game grows the hitbox by a tenth
	if at, _, ok := segmentBox(b, g.pos, end); ok {
		return float32(at.sub(g.pos).dot(at.sub(g.pos))), true
	}
	if g.pos[0] >= b.min[0] && g.pos[0] <= b.max[0] && g.pos[1] >= b.min[1] && g.pos[1] <= b.max[1] && g.pos[2] >= b.min[2] && g.pos[2] <= b.max[2] {
		return 0, true
	}
	return 0, false
}

// playerTarget is the other player pointed at within reach 3, and how far it is (squared); a block
// in front of it hides it.
func (g *game) playerTarget() (*remotePlayer, float32, bool) {
	if g.net == nil {
		return nil, 0, false
	}
	var best *remotePlayer
	bestD := float32(math.Inf(1))
	for _, p := range g.net.remote {
		if p.dead {
			continue
		}
		if d, ok := g.rayBoxDist(playerBox(p.pos)); ok && d < bestD {
			best, bestD = p, d
		}
	}
	if best == nil {
		return nil, 0, false
	}
	end := g.pos.add(look(g.yaw, g.pitch).scale(3))
	if h, ok := g.rayTrace(g.pos, end); ok && float32(h.at.sub(g.pos).dot(h.at.sub(g.pos))) < bestD {
		return nil, 0, false
	}
	return best, bestD, true
}

// attackEntity is the click on another player or a mob of the server: the server hears of the blow and works
// out everything else. The game's client does nothing more: EntityPlayer.attackTargetEntityWithCurrentItem
// only acts when attackEntityFrom says the blow landed, and in the client's world it never does. So the
// player keeps their stride and their sprint (the server slows its own copy of them), the hunger and the
// weapon's wear are the server's, and a sprinting player is not told to stop and start again with each hit.
func (g *game) attackEntity(eid int32) {
	w := newPacket(0x02)
	w.VarInt(eid)
	w.VarInt(1) // attack
	g.net.c.send(w)
}

// sendActions tells the server when the sprint or the sword's block starts or stops.
func (g *game) sendActions() {
	n := g.net
	if g.sprinting != n.sentSprint {
		w := newPacket(0x0B)
		w.VarInt(n.eid)
		if g.sprinting {
			w.VarInt(3)
		} else {
			w.VarInt(4)
		}
		w.VarInt(0)
		n.c.send(w)
		n.sentSprint = g.sprinting
	}
	if g.sneaking != n.sentSneak {
		w := newPacket(0x0B)
		w.VarInt(n.eid)
		if g.sneaking {
			w.VarInt(0)
		} else {
			w.VarInt(1)
		}
		w.VarInt(0)
		n.c.send(w)
		n.sentSneak = g.sneaking
	}
	if using := g.bowTicks > 0 || g.eating > 0; using != n.sentUsing && g.thirdParty() {
		if !using { // PlayerControllerMP.onStoppedUsingItem: the server shoots the arrow, or stops the meal
			w := newPacket(0x07)
			w.Byte(5)
			w.Long(0)
			w.Byte(0)
			n.c.send(w)
		}
		n.sentUsing = using
	}
	if g.blocking != n.sentBlocking {
		if g.blocking && g.thirdParty() { // the click itself already went out (useNet)
		} else if g.blocking { // right click with the sword in the air
			w := newPacket(0x08)
			w.Long(packPos(-1, -1, -1))
			w.Byte(255)
			w.UShort(0xFFFF)
			w.Byte(0)
			w.Byte(0)
			w.Byte(0)
			n.c.send(w)
		} else { // release the use item
			w := newPacket(0x07)
			w.Byte(5)
			w.Long(0)
			w.Byte(0)
			n.c.send(w)
		}
		n.sentBlocking = g.blocking
	}
}

// netVelocity is the server pushing the player (knockback): the motion is replaced, in blocks a tick, and
// nothing else (Entity.setVelocity). The player is still on the ground until the move that follows takes
// them off it, and so the first tick of a knockback is slowed by the ground's friction as the game's is.
func (g *game) netVelocity(vx, vy, vz int16) {
	if g.net != nil {
		g.net.lastKnock = g.ticks
	}
	g.vel = vec3{float64(vx) / 8000, float64(vy) / 8000, float64(vz) / 8000}
}

// netStatus is Entity Status: 2 is a hurt entity, 3 a dead one.
func (g *game) netStatus(eid int32, status byte) {
	if eid == g.net.eid { // the player's own: the view shakes and it hurts
		switch status {
		case 2:
			g.hurtTime, g.hurtResist = 10, 20
			g.playerHurtSound(false)
		case 3:
			g.playerHurtSound(true)
		}
		return
	}
	p := g.net.remote[eid]
	if p == nil {
		if e := g.net.entity(eid); e != nil && e.mob != nil {
			e.mob.status(status)
		}
		return
	}
	switch status {
	case 2:
		p.hurt = 10
	case 3:
		p.dead = true
	}
}

// cameraRoll is how far the view is turned about its axis, in degrees as the game's modelview has it: the
// shake of a blow (hurtCameraEffect: up to 14 degrees over ten ticks) and the fall of a dead player's view.
func (g *game) cameraRoll(partial float32) float32 {
	roll := float32(0)
	if g.health <= 0 && g.net != nil {
		roll += 40 - 8000/(float32(g.deathTime)+partial+200)
	}
	if f := float32(g.hurtTime) - partial; f >= 0 {
		f /= 10
		roll -= sin32f(f*f*f*f*math.Pi) * 14
	}
	return roll
}

// rolledUp is the camera's up vector after the roll: the view turned the way the modelview was.
func (g *game) rolledUp(dir vec3) vec3 {
	up := vec3{0, 1, 0}
	roll := g.cameraRoll(g.partial)
	if roll == 0 {
		return up
	}
	a := float64(-roll) * math.Pi / 180
	right := dir.cross(up).norm()
	return up.scale(math.Cos(a)).sub(right.scale(math.Sin(a)))
}

// logCorrection says in the log where the server put the player and what the player was doing, each time it does
// (Player Position And Look after the join): a server that keeps sending the player back is judging their
// movement, and this is what its judgement was made of.
func (g *game) logCorrection(to vec3, flags byte) {
	n := g.net
	if !n.joined || !n.terrainDone {
		return
	}
	d := to.sub(g.feet)
	since := g.ticks - n.lastCorrection
	n.lastCorrection = g.ticks
	logf("server moved the player: from %.3f %.3f %.3f to %.3f %.3f %.3f (%.3f away, flags %#x); %d ticks since the last; velocity %.3f %.3f %.3f ground=%v sprint=%v sneak=%v flying=%v hurt=%d knock %d ticks ago",
		g.feet[0], g.feet[1], g.feet[2], to[0], to[1], to[2], math.Sqrt(d.dot(d)), flags, since, g.vel[0], g.vel[1], g.vel[2],
		g.onGround, g.sprinting, g.sneaking, g.flying, g.hurtTime, g.ticks-n.lastKnock)
}
