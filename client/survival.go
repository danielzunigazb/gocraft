package client

import ()

// Survival rules for blocks: health and food, fall damage, how long a block takes to mine by hand,
// what it drops and the counts of the hotbar stacks. Tools, mobs and crafting are not in yet.

const (
	maxHealth = 20
	maxFood   = 20
)

// hardness is 1.8's block hardness; blocks not listed take 1.0, plants and other soft blocks are 0
// and -1 can never be mined.
var hardness = map[uint16]float32{
	1: 1.5, 2: 0.6, 3: 0.5, 4: 2, 5: 2, 6: 0, 7: -1, 12: 0.5, 13: 0.6, 14: 3, 15: 3, 16: 3, 17: 2, 18: 0.2, 19: 0.6, 20: 0.3, 21: 3, 22: 3,
	24: 0.8, 27: 0.7, 28: 0.7, 30: 4, 31: 0, 32: 0, 35: 0.8, 37: 0, 38: 0, 39: 0, 40: 0, 41: 3, 42: 5, 43: 2, 44: 2, 45: 2, 46: 0, 47: 1.5,
	48: 2, 49: 50, 50: 0, 53: 2, 56: 3, 57: 5, 58: 2.5, 59: 0, 61: 3.5, 65: 0.4, 66: 0.7, 67: 2, 78: 0.1, 79: 0.5, 80: 0.2, 81: 0.4, 82: 0.6,
	83: 0, 85: 2, 87: 0.4, 88: 0.5, 89: 0.3, 95: 0.3, 98: 1.5, 102: 0.3, 103: 1, 106: 0.2, 108: 2, 109: 1.5, 110: 0.6, 111: 0, 112: 2, 121: 3,
	125: 2, 126: 2, 129: 3, 133: 5, 152: 5, 153: 3, 159: 1.25, 161: 0.2, 162: 2, 165: 0, 169: 0.3, 170: 0.5, 171: 0.1, 172: 1.25, 173: 5,
	175: 0, 179: 0.8,
}

// needsPickaxe are the blocks that drop nothing unless mined with a pickaxe.
var needsPickaxe = map[uint16]bool{1: true, 4: true, 14: true, 15: true, 16: true, 21: true, 22: true, 24: true, 41: true, 42: true, 43: true,
	44: true, 45: true, 48: true, 49: true, 56: true, 57: true, 61: true, 67: true, 79: true, 87: true, 98: true, 108: true, 109: true,
	112: true, 121: true, 129: true, 133: true, 152: true, 153: true, 159: true, 172: true, 173: true, 179: true, 180: true, 182: true}

func blockHardness(id uint16) float32 {
	if h, ok := hardness[id]; ok {
		return h
	}
	return 1
}

// mine advances breaking the targeted block while the button is held, with the held tool.
func (g *game) mine() {
	h, ok := g.target()
	hit := h.pos
	if !ok || g.opt.Bench > 0 || !g.keyDown("attack") {
		g.mining = false
		return
	}
	if !g.mining || g.mineAt != hit {
		g.mining, g.mineAt, g.mineTicksDone = true, hit, 0
	}
	g.swingTicks = max(g.swingTicks, 2) // the arm keeps swinging while a block is mined
	state := g.world.Block(hit[0], hit[1], hit[2])
	tool := g.held()
	need, harvest := g.breakInfo(state, tool)
	if need < 0 {
		return
	}
	g.blockHitEffect(hit, h.side, state)
	g.blockHitSound(hit, state, g.mineTicksDone)
	if g.mineTicksDone++; g.mineTicksDone < need {
		return
	}
	g.beBroken(hit)
	g.setBlock(hit[0], hit[1], hit[2], 0)
	g.exhaust(0.025)
	for _, d := range g.dropsFor(state, harvest, tool) {
		g.spawnDrop(float64(hit[0]), float64(hit[1]), float64(hit[2]), d)
	}
	class, _ := toolOf(tool)
	g.useDurability(wear(class, state))
	g.mining, g.breakDelay = false, 5 // the game waits 5 ticks before the next block
}

// exhaust adds to the exhaustion that makes the player hungry (FoodStats.addExhaustion).
func (g *game) exhaust(n float32) {
	if !g.creative {
		g.exhaustion = min(g.exhaustion+n, 40)
	}
}

// tickFood is FoodStats.onUpdate: exhaustion burns saturation and then food, a full stomach heals
// quickly, a nearly full one slowly, an empty one starves the player down to half a heart.
func (g *game) tickFood() {
	if g.creative || g.net != nil { // a server keeps the player's food and health
		return
	}
	if g.exhaustion > 4 {
		g.exhaustion -= 4
		if g.saturation > 0 {
			g.saturation = max(g.saturation-1, 0)
		} else {
			g.food = max(g.food-1, 0)
		}
	}
	switch {
	case g.saturation > 0 && g.health < maxHealth && g.food >= 20:
		if g.foodTimer++; g.foodTimer >= 10 {
			f := min(g.saturation, 6)
			g.health = min(g.health+f/6, maxHealth)
			g.exhaust(f)
			g.foodTimer = 0
		}
	case g.food >= 18 && g.health < maxHealth:
		if g.foodTimer++; g.foodTimer >= 80 {
			g.health = min(g.health+1, maxHealth)
			g.exhaust(3)
			g.foodTimer = 0
		}
	case g.food <= 0:
		if g.foodTimer++; g.foodTimer >= 80 {
			if g.health > 1 { // on Normal, starvation stops at half a heart
				g.hurt(1)
			}
			g.foodTimer = 0
		}
	default:
		g.foodTimer = 0
	}
}

// hurt takes health; at zero the player returns to the start with full health and food.
func (g *game) hurt(amount float32) {
	if g.creative || amount <= 0 || g.net != nil { // on a server the damage is its own to deal
		return
	}
	g.exhaust(0.3)
	g.playerHurtSound(g.health-amount <= 0)
	if g.health -= amount; g.health <= 0 {
		g.health, g.food = maxHealth, maxFood
		g.feet, g.vel, g.fall = g.spawn, vec3{}, 0
	}
}

// tickSurvival runs the per-tick rules: fall damage and natural regeneration.
func (g *game) tickSurvival(_ float64) {
	g.tickFood()
}

// drawCount writes a stack size at the bottom right of an item slot with the game's font.
func (g *game) drawCount(n int, x, y, s float32) {
	if n <= 1 {
		return
	}
	hd := &g.hud
	digits := []int{}
	for v := n; v > 0; v /= 10 {
		digits = append([]int{v % 10}, digits...)
	}
	width := float32(len(digits)*6-1) * s
	cx := x + 19*s - 2*s - width
	cy := y + 9*s
	for pass, col := range [][4]float32{{0.25, 0.25, 0.25, 1}, {1, 1, 1, 1}} {
		off := float32(1-pass) * s
		for i, d := range digits {
			c := 48 + d
			hd.rect(cx+float32(i)*6*s+off, cy+off, 8*s, 8*s, float32(c%16*8), float32(c/16*8), 8, 8, 128, 128, col)
		}
	}
	hd.flush(hd.font)
}
