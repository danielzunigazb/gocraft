package client

import "fmt"

// 1.8 stores a block as an id and 4 bits of metadata. The game's own model shapes decide, per
// block, which blockstate file and which property string that pair means; this is that mapping,
// written out for the blocks that have models.

var colours = [16]string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray", "silver", "cyan", "purple", "blue", "brown", "green", "red", "black"}
var woods = [6]string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak"}
var horiz = [4]string{"north", "east", "south", "west"}

// nbFunc returns the block state (id<<4 | metadata) at an offset from the block being drawn.
type nbFunc func(dx, dy, dz int32) uint16

type stateFn func(meta int, nb nbFunc) (file, props string)

const normal = "normal"

var blockNames = map[uint16]string{
	1: "stone", 2: "grass", 3: "dirt", 4: "cobblestone", 7: "bedrock", 12: "sand", 13: "gravel", 14: "gold_ore", 15: "iron_ore",
	16: "coal_ore", 19: "sponge", 20: "glass", 21: "lapis_ore", 22: "lapis_block", 24: "sandstone", 25: "noteblock",
	27: "golden_rail", 28: "detector_rail", 30: "web", 32: "dead_bush", 41: "gold_block", 42: "iron_block", 45: "brick_block",
	46: "tnt", 47: "bookshelf", 48: "mossy_cobblestone", 49: "obsidian", 56: "diamond_ore", 57: "diamond_block", 58: "crafting_table",
	73: "redstone_ore", 74: "lit_redstone_ore", 79: "ice", 80: "snow", 82: "clay", 84: "jukebox", 87: "netherrack", 88: "soul_sand",
	89: "glowstone", 95: "stained_glass", 103: "melon_block", 112: "nether_brick", 116: "enchanting_table", 121: "end_stone",
	122: "dragon_egg", 123: "redstone_lamp", 124: "lit_redstone_lamp", 129: "emerald_ore", 133: "emerald_block", 138: "beacon",
	152: "redstone_block", 153: "quartz_ore", 165: "slime", 168: "prismarine", 169: "sea_lantern", 170: "hay_block", 172: "hardened_clay",
	173: "coal_block", 174: "packed_ice", 179: "red_sandstone", 52: "mob_spawner", 137: "command_block",
}

func byMeta(mask int, names []string, suffix, props string) stateFn {
	return func(meta int, _ nbFunc) (string, string) { return names[meta&mask] + suffix, props }
}

func fixed(file, props string) stateFn {
	return func(int, nbFunc) (string, string) { return file, props }
}

func facing6(file string, names [8]string) stateFn {
	return func(meta int, _ nbFunc) (string, string) {
		return file, "facing=" + names[meta&7]
	}
}

var stairIDs = map[uint16]bool{53: true, 67: true, 108: true, 109: true, 114: true, 128: true, 134: true, 135: true, 136: true, 156: true, 163: true, 164: true, 180: true}

// Directions of a stair's facing as steps on x and z, and the turn the game calls rotateYCCW.
var (
	stairFacing = [4]string{"east", "west", "south", "north"}
	faceStep    = map[string][2]int32{"east": {1, 0}, "west": {-1, 0}, "south": {0, 1}, "north": {0, -1}}
	faceCCW     = map[string]string{"east": "north", "north": "west", "west": "south", "south": "east"}
	faceOpp     = map[string]string{"east": "west", "west": "east", "south": "north", "north": "south"}
)

func stairsOf(st uint16) (facing string, top bool) {
	return stairFacing[st&3], st&4 != 0
}

// stairShape is BlockStairs.getStairsShape: whether the step turns a corner, by the stairs beside it.
func stairShape(st uint16, nb nbFunc) string {
	facing, top := stairsOf(st)
	differs := func(f string) bool { // isDifferentStairs against the block on the given side
		o := nb(faceStep[f][0], 0, faceStep[f][1])
		if !stairIDs[o>>4] {
			return true
		}
		of, ot := stairsOf(o)
		return of != facing || ot != top
	}
	axisX := func(f string) bool { return f == "east" || f == "west" }
	if o := nb(faceStep[facing][0], 0, faceStep[facing][1]); stairIDs[o>>4] {
		if of, ot := stairsOf(o); ot == top && axisX(of) != axisX(facing) && differs(faceOpp[of]) {
			if of == faceCCW[facing] {
				return "outer_left"
			}
			return "outer_right"
		}
	}
	opp := faceOpp[facing]
	if o := nb(faceStep[opp][0], 0, faceStep[opp][1]); stairIDs[o>>4] {
		if of, ot := stairsOf(o); ot == top && axisX(of) != axisX(facing) && differs(of) {
			if of == faceCCW[facing] {
				return "inner_left"
			}
			return "inner_right"
		}
	}
	return "straight"
}

func stairs(file string) stateFn {
	return func(meta int, nb nbFunc) (string, string) {
		half := "bottom"
		if meta&4 != 0 {
			half = "top"
		}
		st := uint16(meta) // the shape only reads the facing and half bits
		return file, fmt.Sprintf("facing=%s,half=%s,shape=%s", stairFacing[meta&3], half, stairShape(st, nb))
	}
}

func slab(files [8]string) stateFn {
	return func(meta int, _ nbFunc) (string, string) {
		half := "bottom"
		if meta&8 != 0 {
			half = "top"
		}
		return files[meta&7], "half=" + half
	}
}

func logStates(names []string) stateFn {
	return func(meta int, _ nbFunc) (string, string) {
		axis := [4]string{"y", "x", "z", "none"}[(meta>>2)&3]
		return names[meta&3] + "_log", "axis=" + axis
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// connects says whether a fence, pane or wall joins the block next to it.
func connects(self, other uint16, opaque *[256]bool) bool {
	if other == self || other == 107 || (other >= 183 && other <= 187) {
		return true
	}
	return other != 0 && opaque[other&255] // a full solid block
}

func fenceLike(file string, selfIDs ...uint16) func(opaque *[256]bool) stateFn {
	return func(opaque *[256]bool) stateFn {
		return func(_ int, nb nbFunc) (string, string) {
			self := selfIDs[0]
			join := func(dx, dz int32) bool {
				o := nb(dx, 0, dz) >> 4
				for _, id := range selfIDs {
					if o == id {
						return true
					}
				}
				return connects(self, o, opaque)
			}
			return file, fmt.Sprintf("east=%s,north=%s,south=%s,west=%s", boolStr(join(1, 0)), boolStr(join(0, -1)), boolStr(join(0, 1)), boolStr(join(-1, 0)))
		}
	}
}

// stateTable builds the mapping; opaque (which blocks are full solid cubes) is what fences and
// panes connect to.
func stateTable(opaque *[256]bool) map[uint16]stateFn {
	t := map[uint16]stateFn{
		2: fixed("grass", "snowy=false"),
		3: func(m int, _ nbFunc) (string, string) {
			return [3]string{"dirt", "coarse_dirt", "podzol"}[min(m&3, 2)], map[bool]string{true: "snowy=false", false: normal}[m&3 == 2]
		},
		110: fixed("mycelium", "snowy=false"),
		1:   byMeta(7, []string{"stone", "granite", "smooth_granite", "diorite", "smooth_diorite", "andesite", "smooth_andesite", "stone"}, "", normal),
		5:   byMeta(7, []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "oak", "oak"}, "_planks", normal),
		6:   byMeta(7, []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "oak", "oak"}, "_sapling", normal),
		12:  byMeta(1, []string{"sand", "red_sand"}, "", normal),
		17:  logStates(woods[:4]),
		162: func(m int, nb nbFunc) (string, string) { return logStates(woods[4:])(m&1|(m&12), nb) },
		18:  byMeta(3, woods[:4], "_leaves", normal),
		161: byMeta(1, woods[4:], "_leaves", normal),
		24:  byMeta(3, []string{"sandstone", "chiseled_sandstone", "smooth_sandstone", "sandstone"}, "", normal),
		179: byMeta(3, []string{"red_sandstone", "chiseled_red_sandstone", "smooth_red_sandstone", "red_sandstone"}, "", normal),
		98:  byMeta(3, []string{"stonebrick", "mossy_stonebrick", "cracked_stonebrick", "chiseled_stonebrick"}, "", normal),
		97:  byMeta(7, []string{"stone_monster_egg", "cobblestone_monster_egg", "stone_brick_monster_egg", "mossy_brick_monster_egg", "cracked_brick_monster_egg", "chiseled_brick_monster_egg", "stone_monster_egg", "stone_monster_egg"}, "", normal),
		155: func(m int, _ nbFunc) (string, string) {
			switch m {
			case 0:
				return "quartz_block", normal
			case 1:
				return "chiseled_quartz_block", normal
			}
			return "quartz_column", "axis=" + [3]string{"y", "x", "z"}[min(m-2, 2)]
		},
		168: byMeta(3, []string{"prismarine", "prismarine_bricks", "dark_prismarine", "prismarine"}, "", normal),
		35:  byMeta(15, colours[:], "_wool", normal),
		171: byMeta(15, colours[:], "_carpet", normal),
		95:  byMeta(15, colours[:], "_stained_glass", normal),
		159: byMeta(15, colours[:], "_stained_hardened_clay", normal),
		31:  byMeta(3, []string{"dead_bush", "tall_grass", "fern", "tall_grass"}, "", normal),
		37:  fixed("dandelion", normal),
		38:  byMeta(15, []string{"poppy", "blue_orchid", "allium", "houstonia", "red_tulip", "orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy", "poppy", "poppy", "poppy", "poppy", "poppy", "poppy", "poppy"}, "", normal),
		175: func(m int, _ nbFunc) (string, string) {
			if m&8 != 0 { // the upper half's flower is that of the lower half, which this mapping cannot see
				return "double_grass", "half=upper"
			}
			return [8]string{"sunflower", "syringa", "double_grass", "double_fern", "double_rose", "paeonia", "double_grass", "double_grass"}[m&7], "half=lower"
		},
		39:  fixed("brown_mushroom", normal),
		40:  fixed("red_mushroom", normal),
		43:  slab([8]string{"stone_double_slab", "sandstone_double_slab", "wood_old_double_slab", "cobblestone_double_slab", "brick_double_slab", "stone_brick_double_slab", "nether_brick_double_slab", "quartz_double_slab"}),
		44:  slab([8]string{"stone_slab", "sandstone_slab", "wood_old_slab", "cobblestone_slab", "brick_slab", "stone_brick_slab", "nether_brick_slab", "quartz_slab"}),
		125: byMeta(7, []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak", "oak", "oak"}, "_double_slab", normal),
		126: func(m int, nb nbFunc) (string, string) {
			return slab([8]string{"oak_slab", "spruce_slab", "birch_slab", "jungle_slab", "acacia_slab", "dark_oak_slab", "oak_slab", "oak_slab"})(m, nb)
		},
		181: fixed("red_sandstone_double_slab", normal),
		182: slab([8]string{"red_sandstone_slab", "red_sandstone_slab", "red_sandstone_slab", "red_sandstone_slab", "red_sandstone_slab", "red_sandstone_slab", "red_sandstone_slab", "red_sandstone_slab"}),
		53:  stairs("oak_stairs"), 67: stairs("stone_stairs"), 108: stairs("brick_stairs"), 109: stairs("stone_brick_stairs"),
		114: stairs("nether_brick_stairs"), 128: stairs("sandstone_stairs"), 134: stairs("spruce_stairs"), 135: stairs("birch_stairs"),
		136: stairs("jungle_stairs"), 156: stairs("quartz_stairs"), 163: stairs("acacia_stairs"), 164: stairs("dark_oak_stairs"),
		180: stairs("red_sandstone_stairs"),
		50:  torchFn("torch"), 75: torchFn("unlit_redstone_torch"), 76: torchFn("redstone_torch"),
		65: func(m int, _ nbFunc) (string, string) {
			return "ladder", "facing=" + [8]string{"north", "north", "north", "south", "west", "east", "north", "north"}[m&7]
		},
		61:  facing6("furnace", [8]string{"north", "north", "north", "south", "west", "east", "north", "north"}),
		62:  facing6("lit_furnace", [8]string{"north", "north", "north", "south", "west", "east", "north", "north"}),
		23:  facing6("dispenser", [8]string{"down", "up", "north", "south", "west", "east", "north", "north"}),
		158: facing6("dropper", [8]string{"down", "up", "north", "south", "west", "east", "north", "north"}),
		86: func(m int, _ nbFunc) (string, string) {
			return "pumpkin", "facing=" + [4]string{"south", "west", "north", "east"}[m&3]
		},
		91: func(m int, _ nbFunc) (string, string) {
			return "lit_pumpkin", "facing=" + [4]string{"south", "west", "north", "east"}[m&3]
		},
		78:  func(m int, _ nbFunc) (string, string) { return "snow_layer", fmt.Sprintf("layers=%d", m&7+1) },
		59:  func(m int, _ nbFunc) (string, string) { return "wheat", fmt.Sprintf("age=%d", m&7) },
		141: func(m int, _ nbFunc) (string, string) { return "carrots", fmt.Sprintf("age=%d", m&7) },
		142: func(m int, _ nbFunc) (string, string) { return "potatoes", fmt.Sprintf("age=%d", m&7) },
		60:  func(m int, _ nbFunc) (string, string) { return "farmland", fmt.Sprintf("moisture=%d", m&7) },
		81:  func(m int, _ nbFunc) (string, string) { return "cactus", fmt.Sprintf("age=%d", m&15) },
		83:  func(m int, _ nbFunc) (string, string) { return "reeds", fmt.Sprintf("age=%d", m&15) },
		111: fixed("waterlily", normal),
		66:  func(m int, _ nbFunc) (string, string) { return "rail", "shape=" + railShapes[m&15] },
		27:  railFn("golden_rail"), 28: railFn("detector_rail"), 157: railFn("activator_rail"),
		96: trapdoor("trapdoor"), 167: trapdoor("iron_trapdoor"),
		85:  fenceLike("fence", 85)(opaque),
		188: fenceLike("spruce_fence", 188)(opaque), 189: fenceLike("birch_fence", 189)(opaque), 190: fenceLike("jungle_fence", 190)(opaque),
		191: fenceLike("dark_oak_fence", 191)(opaque), 192: fenceLike("acacia_fence", 192)(opaque), 113: fenceLike("nether_brick_fence", 113)(opaque),
		102: fenceLike("glass_pane", 102, 20, 95, 160)(opaque), 101: fenceLike("iron_bars", 101)(opaque),
		160: func(m int, nb nbFunc) (string, string) {
			_, props := fenceLike("", 160, 102, 20, 95)(opaque)(m, nb)
			return colours[m&15] + "_stained_glass_pane", props
		},
		139: func(m int, nb nbFunc) (string, string) {
			join := func(dx, dz int32) bool {
				o := nb(dx, 0, dz) >> 4
				return o == 139 || connects(139, o, opaque)
			}
			n, e, so, w := join(0, -1), join(1, 0), join(0, 1), join(-1, 0)
			up := !((n && so && !e && !w) || (e && w && !n && !so)) || nb(0, 1, 0)>>4 != 0
			file := "cobblestone_wall"
			if m&1 != 0 {
				file = "mossy_cobblestone_wall"
			}
			return file, fmt.Sprintf("east=%s,north=%s,south=%s,up=%s,west=%s", boolStr(e), boolStr(n), boolStr(so), boolStr(up), boolStr(w))
		},
		107: gate("fence_gate"), 183: gate("spruce_fence_gate"), 184: gate("birch_fence_gate"), 185: gate("jungle_fence_gate"),
		186: gate("dark_oak_fence_gate"), 187: gate("acacia_fence_gate"),
		64: door("wooden_door"), 71: door("iron_door"), 193: door("spruce_door"), 194: door("birch_door"), 195: door("jungle_door"),
		196: door("acacia_door"), 197: door("dark_oak_door"),
		106: func(m int, nb nbFunc) (string, string) {
			return "vine", fmt.Sprintf("east=%s,north=%s,south=%s,up=false,west=%s", boolStr(m&8 != 0), boolStr(m&4 != 0), boolStr(m&1 != 0), boolStr(m&2 != 0))
		},
		145: func(m int, _ nbFunc) (string, string) {
			return "anvil", fmt.Sprintf("damage=%d,facing=%s", m>>2&3, [4]string{"south", "west", "north", "east"}[m&3])
		},
		90: func(m int, _ nbFunc) (string, string) { // a Nether portal: meta 1 is the x axis, 2 the z axis
			if m&3 == 1 {
				return "portal", "axis=x"
			}
			return "portal", "axis=z"
		},
		120: func(m int, _ nbFunc) (string, string) {
			return "end_portal_frame", fmt.Sprintf("eye=%s,facing=%s", boolStr(m&4 != 0), [4]string{"south", "west", "north", "east"}[m&3])
		},
		51: fixed("fire", "alt=false,east=false,flip=false,north=false,south=false,upper=0,west=false"), // a flame on the floor
	}
	return t
}

var railShapes = [16]string{"north_south", "east_west", "ascending_east", "ascending_west", "ascending_north", "ascending_south", "south_east", "south_west", "north_west", "north_east", "north_south", "north_south", "north_south", "north_south", "north_south", "north_south"}

// door reads both halves: the lower one says which way it faces and whether it is open, the upper
// one which side its hinge is on.
func door(file string) stateFn {
	return func(meta int, nb nbFunc) (string, string) {
		lower, upper, half := meta, 0, "lower"
		if meta&8 == 0 {
			upper = int(nb(0, 1, 0) & 15)
		} else {
			lower, upper, half = int(nb(0, -1, 0)&15), meta, "upper"
		}
		hinge := "left"
		if upper&1 != 0 {
			hinge = "right"
		}
		return file, fmt.Sprintf("facing=%s,half=%s,hinge=%s,open=%s", [4]string{"east", "south", "west", "north"}[lower&3], half, hinge, boolStr(lower&4 != 0))
	}
}

func gate(file string) stateFn {
	return func(meta int, _ nbFunc) (string, string) {
		return file, fmt.Sprintf("facing=%s,in_wall=false,open=%s", [4]string{"south", "west", "north", "east"}[meta&3], boolStr(meta&4 != 0))
	}
}

func railFn(file string) stateFn {
	return func(m int, _ nbFunc) (string, string) {
		return file, fmt.Sprintf("powered=%s,shape=%s", boolStr(m&8 != 0), railShapes[m&7])
	}
}

func torchFn(file string) stateFn {
	return func(m int, _ nbFunc) (string, string) {
		return file, "facing=" + [8]string{"up", "east", "west", "south", "north", "up", "up", "up"}[m&7]
	}
}

func trapdoor(file string) stateFn {
	return func(m int, _ nbFunc) (string, string) {
		half := "bottom"
		if m&8 != 0 {
			half = "top"
		}
		return file, fmt.Sprintf("facing=%s,half=%s,open=%s", [4]string{"north", "south", "west", "east"}[m&3], half, boolStr(m&4 != 0))
	}
}

// biomeClimate is each 1.8 biome's temperature and rainfall, which pick a colour from the grass
// and foliage colour maps. Biomes not listed are plains.
var biomeClimate = map[uint8][2]float32{
	0: {0.5, 0.5}, 2: {2, 0}, 3: {0.2, 0.3}, 4: {0.7, 0.8}, 5: {0.25, 0.8}, 6: {0.8, 0.9}, 7: {0.5, 0.5}, 8: {2, 0}, 9: {0.5, 0.5},
	10: {0, 0.5}, 11: {0, 0.5}, 12: {0, 0.5}, 13: {0, 0.5}, 14: {0.9, 1}, 15: {0.9, 1}, 16: {0.8, 0.4}, 17: {2, 0}, 18: {0.7, 0.8},
	19: {0.25, 0.8}, 20: {0.2, 0.3}, 21: {0.95, 0.9}, 22: {0.95, 0.9}, 23: {0.95, 0.8}, 24: {0.5, 0.5}, 25: {0.2, 0.3}, 26: {0.05, 0.3},
	27: {0.6, 0.6}, 28: {0.6, 0.6}, 29: {0.7, 0.8}, 30: {-0.5, 0.4}, 31: {-0.5, 0.4}, 32: {0.3, 0.8}, 33: {0.3, 0.8}, 34: {0.2, 0.3},
	35: {1.2, 0}, 36: {1.2, 0}, 37: {2, 0}, 38: {2, 0}, 39: {2, 0},
}

var (
	tintSpruce = [3]float32{0.380, 0.600, 0.380} // fixed colours of the game
	tintBirch  = [3]float32{0.502, 0.655, 0.333}
	tintWater  = [3]float32{0.247, 0.463, 0.894}
)

// tintFor is the colour a face's tint index takes for a block in a biome.
func (r *resources) tintFor(id uint16, meta int, biome uint8) [3]float32 {
	switch id {
	case 18:
		switch meta & 3 {
		case 1:
			return tintSpruce
		case 2:
			return tintBirch
		}
		return r.foliageTint[biome]
	case 161, 106:
		return r.foliageTint[biome]
	case 8, 9:
		return tintWater
	}
	return r.grassTint[biome]
}

// climateColour reads a colour map the way the game does: temperature across, rainfall times
// temperature down, with the bottom-right triangle of the map unused.
func (r *resources) climateColour(name string, biome uint8) [3]float32 {
	c, ok := biomeClimate[biome]
	if !ok {
		c = [2]float32{0.8, 0.4}
	}
	temp := max(0, min(1, c[0]))
	rain := max(0, min(1, c[1])) * temp
	img := r.gui[name]
	b := img.Bounds()
	x, y := int((1-temp)*float32(b.Dx()-1)), int((1-rain)*float32(b.Dy()-1))
	px := img.RGBAAt(x, y)
	return [3]float32{float32(px.R) / 255, float32(px.G) / 255, float32(px.B) / 255}
}

func isFluid(id uint16) bool { return id >= 8 && id <= 11 }
