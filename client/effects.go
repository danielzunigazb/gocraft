package client

// Potion effects the server puts on the player. They change how fast and how high the player moves, as the
// movement speed attribute and the jump do in the game, and the HUD shows them.

type potionEffect struct {
	amp   int
	ticks int
}

const (
	effSpeed     = 1
	effSlowness  = 2
	effJump      = 8
	effBlindness = 15
	effRegen     = 10
	effPoison    = 19
	effWither    = 20
	effAbsorbing = 22
)

func (g *game) hasEffect(id int) (potionEffect, bool) {
	e, ok := g.effects[id]
	return e, ok
}

// The amounts of the movement speed modifiers, as the game writes them: floats widened to double.
const (
	speedAmount    = 0.20000000298023224  // Speed, a level
	slownessAmount = -0.15000000596046448 // Slowness, a level
	sprintAmount   = 0.30000001192092896  // sprinting
)

// groundSpeed is the movement speed attribute: 0.1, times a level of Speed, of Slowness and of sprinting
// (each modifier multiplies the total, as the attribute's operation 2 does, in the order the game's set of
// modifiers lists them: Slowness, sprint, Speed). The game reads it at the end of a tick (setMoveSpeeds).
func (g *game) groundSpeed() float32 {
	v := float64(landSpeed)
	if g.walkCap > 0 { // a server may change how fast the player walks: /walkspeed
		v = float64(g.walkCap)
	}
	if e, ok := g.effects[effSlowness]; ok {
		v *= 1 + slownessAmount*float64(e.amp+1)
	}
	if g.sprinting {
		v *= 1 + sprintAmount
	}
	if e, ok := g.effects[effSpeed]; ok {
		v *= 1 + speedAmount*float64(e.amp+1)
	}
	return float32(max(v, 0))
}

// jumpBoost is what Jump Boost adds to a jump: a tenth a level.
func (g *game) jumpBoost() float64 {
	if e, ok := g.effects[effJump]; ok {
		return float64(float32(e.amp+1) * 0.1)
	}
	return 0
}

// tickEffects counts the effects down; the server takes them off when they end, and so does this.
func (g *game) tickEffects() {
	for id, e := range g.effects {
		if e.ticks > 0 {
			e.ticks--
			g.effects[id] = e
			if e.ticks == 0 {
				delete(g.effects, id)
			}
		}
	}
}
