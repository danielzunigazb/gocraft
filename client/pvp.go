package client

import (
	"fmt"
	"math"

	"gocraft/proto"
)

// Fighting other players on a server. The server owns health and decides every blow; the client
// aims, tells the server what it did (the attack, sprinting, blocking) and takes what comes back:
// its health, the knockback, and who got hurt.

// pvpState is what the game keeps for fighting: the mouse buttons as the game takes them in (clicks.go).
type pvpState struct {
	clicks clickState
	use    itemUse // the item in use on a third-party server (itemuse.go)
}

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
// out the damage. What the client does of its own is what EntityPlayer.attackTargetEntityWithCurrentItem does
// when attackEntityFrom says the blow landed. For another player it says so (EntityOtherPlayerMP.attackEntityFrom
// is `return true`): a sprinting attacker loses their sprint and a third of their speed (0.6 of their motion),
// and pays the hunger and the weapon's wear. A server expects exactly that: it does it to its own copy of the
// player, and a movement check that sees the attacker go on at a sprint's speed after a blow puts them back.
// For a mob (EntityLivingBase.attackEntityFrom is false in the client's world) the client does nothing.
func (g *game) attackEntity(eid int32) {
	g.syncHeld()
	w := newPacket(0x02)
	w.VarInt(eid)
	w.VarInt(1) // attack
	g.net.c.send(w)
	if g.net.remote[eid] != nil {
		g.net.hitPending = true // what it costs the player is paid at the start of the next tick, see applyPendingHit
	}
}

// applyPendingHit is what a blow on another player costs the attacker, paid where the game pays it: its clicks
// are taken in at the start of a tick, after the server's packets of the frame, so a knockback that arrived just
// before the click is slowed to 0.6 with the rest of the motion. Paying it in the click itself, between ticks, let
// a knockback that was still queued replace the slowdown, and the player went on 0.14 blocks ahead of where a
// server that expected the slowdown put them.
func (g *game) applyPendingHit() {
	if g.net == nil || !g.net.hitPending {
		return
	}
	g.net.hitPending = false
	if g.sprinting {
		g.vel[0], g.vel[2] = g.vel[0]*0.6, g.vel[2]*0.6
		g.setSprinting(false)
	}
	g.exhaust(0.3)
	if class, _ := toolOf(g.held()); class == "sword" {
		g.useDurability(1)
	} else if class != "" {
		g.useDurability(2)
	}
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
	if g.thirdParty() { // the item in use goes out with the click and the release (itemuse.go)
		return
	}
	if g.blocking != n.sentBlocking {
		if g.blocking { // right click with the sword in the air
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
		if proto.Note != nil {
			under := g.blockUnderFeet()
			proto.Note(fmt.Sprintf("knockback %.3f %.3f %.3f taken at the frame: motion was %.3f %.3f %.3f ground=%v sprint=%v, under the feet block %d slipperiness %.2f",
				float64(vx)/8000, float64(vy)/8000, float64(vz)/8000, g.vel[0], g.vel[1], g.vel[2], g.onGround, g.sprinting, under, slipperiness(under)))
		}
	}
	g.vel = vec3{float64(vx) / 8000, float64(vy) / 8000, float64(vz) / 8000}
}

// netStatus is Entity Status: 2 is a hurt entity, 3 a dead one.
func (g *game) netStatus(eid int32, status byte) {
	if eid == g.net.eid { // the player's own: the view shakes and it hurts
		switch status {
		case 9: // the server finished the meal or the drink
			g.useFinished()
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
	under := g.blockUnderFeet()
	text := fmt.Sprintf("server moved the player: from %.3f %.3f %.3f to %.3f %.3f %.3f (%.3f away, flags %#x); %d ticks since the last; velocity %.3f %.3f %.3f ground=%v sprint=%v sneak=%v flying=%v hurt=%d knock %d ticks ago; under the feet block %d slipperiness %.2f",
		g.feet[0], g.feet[1], g.feet[2], to[0], to[1], to[2], math.Sqrt(d.dot(d)), flags, since, g.vel[0], g.vel[1], g.vel[2],
		g.onGround, g.sprinting, g.sneaking, g.flying, g.hurtTime, g.ticks-n.lastKnock, under, slipperiness(under))
	logf("%s", text)
	if proto.Note != nil {
		proto.Note(text)
	}
}

// noteReach puts in the -netlog how far the nearest rival was when the player swung, and whether the click found
// them: the distance from the eyes to their box, where the view's ray meets it (-1 for never), how far apart the
// feet are, and how far the rival is drawn from where the server last put them. A blow that did not register because
// the server's invulnerability stopped it and one that never left the client because the rival was too far are
// different things, and the packets alone cannot tell them apart.
func (g *game) noteReach(target int32, hit bool) {
	if proto.Note == nil || g.net == nil {
		return
	}
	var best *remotePlayer
	bestD := math.Inf(1)
	for _, p := range g.net.remote {
		if p.dead {
			continue
		}
		if d := p.pos.sub(g.feet); d.dot(d) < bestD {
			best, bestD = p, d.dot(d)
		}
	}
	if best == nil || bestD > 10*10 {
		proto.Note(fmt.Sprintf("swing hit=%v: nobody within 10 blocks", hit))
		return
	}
	box := playerBox(best.pos)
	box.min, box.max = box.min.sub(vec3{0.1, 0.1, 0.1}), box.max.add(vec3{0.1, 0.1, 0.1}) // the game grows the box by a tenth
	eyeToBox := math.Sqrt(boxDistSq(box, g.pos))
	ray := -1.0
	if at, _, ok := segmentBox(box, g.pos, g.pos.add(look(g.yaw, g.pitch).scale(8))); ok {
		ray = math.Sqrt(at.sub(g.pos).dot(at.sub(g.pos)))
	}
	f := best.pos.sub(g.feet)
	lag := best.target.sub(best.pos)
	proto.Note(fmt.Sprintf("swing hit=%v (target %d) nearest=%d(%s) eyeToBox=%.2f ray=%.2f feet=%.2f (dy %+.2f) drawn %.2f behind the server's position",
		hit, target, best.eid, best.name, eyeToBox, ray, math.Hypot(f[0], f[2]), f[1], math.Sqrt(lag.dot(lag))))
}

// boxDistSq is how far, squared, a point is from a box (0 inside it).
func boxDistSq(b aabb, p vec3) float64 {
	var d float64
	for a := 0; a < 3; a++ {
		switch {
		case p[a] < b.min[a]:
			d += (b.min[a] - p[a]) * (b.min[a] - p[a])
		case p[a] > b.max[a]:
			d += (p[a] - b.max[a]) * (p[a] - b.max[a])
		}
	}
	return d
}
