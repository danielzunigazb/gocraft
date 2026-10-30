package client

import "math"

// Mining with tools and what blocks drop, after 1.8's ItemTool, ItemSword, ItemShears and the
// blocks' own getItemDropped: speed by material, the pick needed to harvest, wear on the tool.

var tierSpeed = [5]float32{2, 4, 6, 8, 12} // wood, stone, iron, diamond, gold
var tierLevel = [5]int{0, 1, 2, 3, 0}      // the harvest level each can reach

func toolOf(s slot) (class string, tier int) {
	if s.empty() {
		return "", 0
	}
	sets := map[string][5]uint16{
		"pickaxe": {270, 274, 257, 278, 285}, "axe": {271, 275, 258, 279, 286}, "shovel": {269, 273, 256, 277, 284},
		"sword": {268, 272, 267, 276, 283}, "hoe": {290, 291, 292, 293, 294},
	}
	for c, ids := range sets {
		for t, id := range ids {
			if s.ID == id {
				return c, t
			}
		}
	}
	if s.ID == 359 {
		return "shears", 0
	}
	return "", 0
}

var axeBlocks = idSet(5, 17, 25, 47, 53, 54, 58, 63, 64, 68, 72, 85, 96, 107, 125, 126, 134, 135, 136, 143, 146, 162, 163, 164, 183, 184, 185, 186, 187,
	188, 189, 190, 191, 192, 193, 194, 195, 196, 197, 84, 99, 100, 103, 86, 91, 130)
var shovelBlocks = idSet(2, 3, 12, 13, 60, 78, 80, 82, 88, 110, 3)

func idSet(ids ...uint16) map[uint16]bool {
	m := map[uint16]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// harvestNeeded is the harvest level a pickaxe must have to get anything from a block, -1 if none
// is needed.
func harvestNeeded(id uint16) int {
	switch id {
	case 49: // obsidian
		return 3
	case 14, 41, 56, 57, 73, 74, 129, 133:
		return 2
	case 15, 21, 22, 42:
		return 1
	}
	if needsPickaxe[id] {
		return 0
	}
	return -1
}

// breakInfo is how many ticks a block takes with the held tool and whether the tool harvests it.
func (g *game) breakInfo(state uint16, tool slot) (ticks int, harvest bool) {
	id := state >> 4
	h := blockHardness(id)
	if h < 0 {
		return -1, false
	}
	class, tier := toolOf(tool)
	speed := float32(1)
	effective := false
	switch {
	case needsPickaxe[id]:
		effective = class == "pickaxe"
	case axeBlocks[id]:
		effective = class == "axe"
	case shovelBlocks[id]:
		effective = class == "shovel"
	}
	if effective {
		speed = tierSpeed[tier]
	}
	switch {
	case class == "shears" && (id == 18 || id == 161 || id == 30 || id == 106):
		speed = 15
	case class == "shears" && id == 35:
		speed = 5
	case class == "sword" && id == 30:
		speed = 15
	case class == "sword" && (id == 18 || id == 161 || id == 31 || id == 32 || id == 106 || id == 86 || id == 103):
		speed = 1.5
	}
	need := harvestNeeded(id)
	harvest = need < 0 || (class == "pickaxe" && tierLevel[tier] >= need)
	if (id == 78 || id == 80) && class != "shovel" {
		harvest = false
	}
	if id == 30 && class != "sword" && class != "shears" {
		harvest = false
	}
	if !g.onGround {
		speed /= 5
	}
	if g.world != nil && g.fluidIn(false) {
		speed /= 5
	}
	if h == 0 {
		return 0, harvest
	}
	per := float32(100)
	if harvest {
		per = 30
	}
	return int(math.Ceil(float64(h * per / speed))), harvest
}

// mineTicks is the time with bare hands, standing on the ground.
func mineTicks(id uint16) int {
	g := &game{onGround: true}
	t, _ := g.breakInfo(id<<4, slot{})
	return t
}

func randBetween(lo, hi int8) int8 { return lo + int8(rng.Intn(int(hi-lo)+1)) }

// dropsFor is what breaking a block leaves, which depends on whether the tool harvests it.
func (g *game) dropsFor(state uint16, harvest bool, tool slot) []slot {
	id, meta := state>>4, state&15
	if !harvest {
		return nil
	}
	one := func(s slot) []slot { return []slot{s} }
	blk := func(st uint16) []slot { return one(blockSlot(st, 1)) }
	switch id {
	case 1:
		if meta == 0 {
			return blk(4 << 4)
		}
		return blk(1<<4 | meta)
	case 2:
		return blk(3 << 4)
	case 13:
		if rng.Intn(10) == 0 {
			return one(slot{ID: 318, N: 1})
		}
		return blk(state)
	case 16:
		return one(slot{ID: 263, N: 1})
	case 17:
		return blk(17<<4 | meta&3)
	case 162:
		return blk(162<<4 | meta&1)
	case 18, 161:
		var out []slot
		if rng.Intn(20) == 0 {
			sap := uint16(6<<4) | meta&3
			if id == 161 {
				sap = 6<<4 | 4 | meta&1
			}
			out = append(out, blockSlot(sap, 1))
		}
		if id == 18 && meta&3 == 0 && rng.Intn(200) == 0 {
			out = append(out, slot{ID: 260, N: 1})
		}
		if cl, _ := toolOf(tool); cl == "shears" {
			return blk(id<<4 | meta&3)
		}
		return out
	case 21:
		return one(slot{ID: 351, Dmg: 4, N: randBetween(4, 8)})
	case 56:
		return one(slot{ID: 264, N: 1})
	case 73, 74:
		return one(slot{ID: 331, N: randBetween(4, 5)})
	case 129:
		return one(slot{ID: 388, N: 1})
	case 153:
		return one(slot{ID: 406, N: 1})
	case 79, 20, 95, 102, 160, 52, 106, 46, 50 + 0*id:
		if id == 50 {
			return blk(50 << 4)
		}
		if cl, _ := toolOf(tool); cl == "shears" && (id == 106) {
			return blk(state)
		}
		return nil
	case 31:
		if cl, _ := toolOf(tool); cl == "shears" {
			return blk(31<<4 | 1)
		}
		if rng.Intn(8) == 0 {
			return one(slot{ID: 295, N: 1})
		}
		return nil
	case 32:
		return one(slot{ID: 280, N: randBetween(0, 2)})
	case 30:
		return one(slot{ID: 287, N: 1})
	case 78:
		return one(slot{ID: 332, N: 1})
	case 80:
		return one(slot{ID: 332, N: 4})
	case 82:
		return one(slot{ID: 337, N: 4})
	case 89:
		return one(slot{ID: 348, N: randBetween(2, 4)})
	case 169:
		return one(slot{ID: 410, N: randBetween(2, 3)})
	case 103:
		return one(slot{ID: 360, N: randBetween(3, 7)})
	case 47:
		return one(slot{ID: 340, N: 3})
	case 83:
		return one(slot{ID: 338, N: 1})
	case 64:
		return one(slot{ID: 324, N: 1})
	case 71:
		return one(slot{ID: 330, N: 1})
	case 26:
		return one(slot{ID: 355, N: 1})
	case 63, 68:
		return one(slot{ID: 323, N: 1})
	case 175:
		if meta&8 != 0 || meta&7 == 2 || meta&7 == 3 {
			if cl, _ := toolOf(tool); cl == "shears" {
				return blk(175<<4 | meta&7)
			}
			return nil
		}
		return blk(175<<4 | meta&7)
	case 55:
		return one(slot{ID: 331, N: 1})
	case 59, 141, 142, 104, 105, 127:
		return nil
	case 8, 9, 10, 11, 90, 119, 51:
		return nil
	}
	if orientedMeta[id] {
		return blk(id<<4 | meta&boolMeta(id == 44))
	}
	return blk(state)
}

// wear is the durability a tool loses for breaking a block, 0 for blocks it breaks for free.
func wear(class string, state uint16) int {
	if blockHardness(state>>4) <= 0 {
		return 0
	}
	switch class {
	case "sword", "hoe":
		return 2
	case "shears":
		if id := state >> 4; id == 18 || id == 161 || id == 30 || id == 106 || id == 35 {
			return 1
		}
		return 0
	case "":
		return 0
	}
	return 1
}

func boolMeta(slab bool) uint16 {
	if slab {
		return 7
	}
	return 0
}

// orientedMeta are blocks whose metadata is a facing or state, not a variant of the item.
var orientedMeta = map[uint16]bool{44: true, 53: true, 67: true, 108: true, 109: true, 114: true, 128: true, 134: true, 135: true, 136: true,
	156: true, 163: true, 164: true, 180: true, 65: true, 61: true, 23: true, 158: true, 86: true, 91: true, 96: true, 66: true, 27: true,
	28: true, 157: true, 50: true, 75: true, 76: true}
