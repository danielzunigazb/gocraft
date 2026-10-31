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
	effRegen     = 10
	effPoison    = 19
	effWither    = 20
	effAbsorbing = 22
)

func (g *game) hasEffect(id int) (potionEffect, bool) {
	e, ok := g.effects[id]
	return e, ok
}

// groundSpeed is the movement speed attribute: 0.1, times 1.2 a level of Speed, times 0.85 a level of
// Slowness, times 1.3 when sprinting (each modifier multiplies the total, as the attribute's operation 2 does).
func (g *game) groundSpeed() float32 {
	v := float64(landSpeed)
	if g.walkCap > 0 { // a server may change how fast the player walks: /walkspeed
		v = float64(g.walkCap)
	}
	if e, ok := g.effects[effSpeed]; ok {
		v *= 1 + 0.2*float64(e.amp+1)
	}
	if e, ok := g.effects[effSlowness]; ok {
		v *= 1 - 0.15*float64(e.amp+1)
	}
	if g.sprinting {
		v *= 1 + sprintBonus
	}
	return float32(v)
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
