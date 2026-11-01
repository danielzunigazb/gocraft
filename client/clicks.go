package client

import "math"

// The mouse buttons as 1.8.9's Minecraft.runTick takes them in. A press is only counted when it happens
// (KeyBinding.onTick raises its pressTime); what it does is decided at the start of the next tick, in this
// order: the attack presses, then the use presses, then the pick presses. While an item is in use (a sword
// held up, a bow drawn, a meal) the presses are thrown away. A click that hits nothing makes the next attack
// wait (leftClickCounter, the "miss penalty" of 1.8) until a tick finds the button up. And the click aims
// from where the tick left the player, along the yaw the last tick had (EntityLivingBase.getLook(1.0) reads
// rotationYawHead, which EntityPlayer.updateEntityActionState sets once a tick) and the pitch of now.
//
// A server sees exactly this: the swing and the attack go out between two movement packets, and an attack
// never goes out while the player is blocking.

// clickState is what Minecraft keeps of the buttons between ticks.
type clickState struct {
	attack, use, pick int     // presses not taken in yet (KeyBinding.pressTime)
	left              int     // leftClickCounter: ticks until an attack may be made again
	headYaw           float32 // the yaw the last tick had (rotationYawHead), in radians
	headSet           bool
}

// missPenalty is how many ticks a click that hits nothing holds back the next one (Minecraft.clickMouse).
const missPenalty = 10

// pressButton counts a press of a mouse button bound to the attack, the use or the pick (KeyBinding.onTick).
func (g *game) pressButton(code int) {
	c := &g.pvp.clicks
	switch code {
	case g.opts.Keys["attack"]:
		c.attack++
	case g.opts.Keys["use"]:
		c.use++
	case g.opts.Keys["pickItem"]:
		c.pick++
	}
}

// takeClicks is the part of runTick that handles the mouse, run at the start of each tick before the player
// moves. It returns with g.pos and g.yaw as they were.
func (g *game) takeClicks() {
	c := &g.pvp.clicks
	g.useDelay = max(g.useDelay-1, 0) // rightClickDelayTimer is counted down first thing in the tick
	if g.screen != screenNone {       // a screen takes the clicks; the attack waits until the button is let go
		c.attack, c.use, c.pick = 0, 0, 0
		c.left = 10000
		c.headYaw, c.headSet = g.yaw, true
		return
	}
	if c.left > 0 {
		c.left--
	}
	pos, yaw := g.pos, g.yaw
	g.pos = g.feet.add(vec3{0, float64(g.eyeOffset()), 0}) // getPositionEyes(1.0): where the last tick ended
	if g.usingItem() {
		c.attack, c.use, c.pick = 0, 0, 0
	} else {
		if c.headSet {
			g.yaw = c.headYaw
		}
		for ; c.attack > 0; c.attack-- {
			g.clickMouse()
			g.applyPendingHit() // attackTargetEntityWithCurrentItem pays for the blow right after the packet
		}
		g.yaw = yaw
		for ; c.use > 0; c.use-- {
			if !g.mining { // rightClickMouse does nothing while a block is being broken
				g.useDelay = useDelayTicks
				g.useBlock()
			}
		}
		for ; c.pick > 0; c.pick-- {
			g.pickBlock()
		}
	}
	if !g.keyDown("attack") { // sendClickBlockToController: a button that is up lifts the penalty
		c.left = 0
	}
	g.pos, g.yaw = pos, yaw
	c.headYaw, c.headSet = g.yaw, true
}

// clickMouse is Minecraft.clickMouse: the arm swings and the server hears of it, then the entity pointed at is
// hit, or a block is struck; anything else (the air, or an entity more than 3 blocks away) is a miss.
func (g *game) clickMouse() {
	c := &g.pvp.clicks
	if c.left > 0 {
		return
	}
	g.swingTicks = swingTicks
	g.sendSwing()
	if g.attackClick() {
		return
	}
	if g.creative { // clickBlock in creative breaks the block at once; a miss costs nothing
		g.breakDelay = breakDelayTicks
		g.breakBlock()
		return
	}
	h, ok := g.target()
	limit := float64(g.reachDist())
	if ok {
		limit = math.Sqrt(h.at.sub(g.pos).dot(h.at.sub(g.pos)))
	}
	if ok && !g.entityBeyondReach(limit) {
		return // a block: mining it is the held button's business (mine)
	}
	c.left = missPenalty
}

// entityBeyondReach is the other case of EntityRenderer.getMouseOver: an entity on the view's ray, nearer than the
// block it meets (or than the reach, with none), but more than 3 blocks away. The game makes that a miss, not a
// click on the block behind.
func (g *game) entityBeyondReach(limit float64) bool {
	end := g.pos.add(look(g.yaw, g.pitch).scale(limit))
	on := func(b aabb) bool {
		b.min, b.max = b.min.sub(vec3{0.1, 0.1, 0.1}), b.max.add(vec3{0.1, 0.1, 0.1})
		_, _, ok := segmentBox(b, g.pos, end)
		return ok
	}
	if g.net == nil {
		for _, m := range g.mobs {
			if !m.dead() && on(m.box()) {
				return true
			}
		}
		return false
	}
	for _, p := range g.net.remote {
		if !p.dead && on(playerBox(p.pos)) {
			return true
		}
	}
	for _, e := range g.net.ents {
		if e.obj == 0 && e.height() != 0 && on(e.box()) {
			return true
		}
	}
	return false
}

// syncHeld is PlayerControllerMP.syncCurrentPlayItem: the hotbar slot in hand goes to the server before anything
// that uses it (an attack, a click on a block or in the air, letting go of an item in use), so that a slot
// changed in the same tick is the one the server uses.
func (g *game) syncHeld() {
	n := g.net
	if n == nil || n.sentHeld == g.sel {
		return
	}
	w := newPacket(0x09)
	w.UShort(uint16(g.sel))
	n.c.send(w)
	n.sentHeld = g.sel
}
