package client

import "math"

// The field of view moves with how fast the player goes, as 1.8.9's does: AbstractClientPlayer.getFovModifier is
// the movement speed over the walking speed, halfway to 1 (so sprinting widens the view by 15%, Speed I by
// 10%), 10% more while flying and narrower as a bow is drawn; and EntityRenderer.updateFovModifierHand takes
// half of the way to it each tick, so the view eases in and out. The hand's own field of view stays at 70.

// fovTarget is the modifier the player's state asks for.
func (g *game) fovTarget() float32 {
	f := float64(1)
	if g.flying {
		f *= 1.1
	}
	walk := float64(g.walkSpeedNow())
	f *= (float64(g.groundSpeed())/walk + 1) / 2
	if walk == 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		f = 1
	}
	if g.bowTicks > 0 { // a drawn bow narrows the view as it is drawn: by up to 15%
		f1 := min(float64(g.bowTicks)/20, 1)
		f *= 1 - f1*f1*0.15
	}
	return float32(f)
}

// updateFov is EntityRenderer.updateFovModifierHand, once a tick.
func (g *game) updateFov() {
	if !g.fovSet {
		g.fovMod, g.fovSet = 1, true
	}
	g.fovModPrev = g.fovMod
	g.fovMod += (g.fovTarget() - g.fovMod) * 0.5
	g.fovMod = min(max(g.fovMod, 0.1), 1.5)
}

// worldFOV is the field of view of the world, in degrees, between the last tick's modifier and this one's.
func (g *game) worldFOV() float32 {
	if !g.fovSet {
		return g.opts.FOV
	}
	return g.opts.FOV * (g.fovModPrev + (g.fovMod-g.fovModPrev)*g.partial)
}
