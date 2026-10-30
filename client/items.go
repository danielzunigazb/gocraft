package client

import "fmt"

// slot is a stack of items: a block (id below 256, damage is its metadata variant) or an item.
type slot struct {
	ID, Dmg uint16
	N       int8
}

func (s slot) empty() bool             { return s.N <= 0 || s.ID == 0 }
func (s slot) isBlock() bool           { return s.ID < 256 }
func (s slot) state() uint16           { return s.ID<<4 | s.Dmg&15 }
func (s slot) same(o slot) bool        { return s.ID == o.ID && s.Dmg == o.Dmg }
func blockSlot(st uint16, n int8) slot { return slot{ID: st >> 4, Dmg: st & 15, N: n} }

// itemDef is what the game knows of an item beyond its picture.
type itemDef struct {
	name       string // the model in models/item
	stack      int8   // largest stack, 64 when 0
	durability int16
}

func (s slot) maxStack() int8 {
	if s.ID >= 256 {
		if d := itemDefs[s.ID]; d.stack != 0 {
			return d.stack
		}
	}
	return 64
}

var itemDefs = map[uint16]itemDef{
	256: {"iron_shovel", 1, 250}, 257: {"iron_pickaxe", 1, 250}, 258: {"iron_axe", 1, 250}, 259: {"flint_and_steel", 1, 64}, 260: {"apple", 0, 0},
	261: {"bow", 1, 384}, 262: {"arrow", 0, 0}, 263: {"coal", 0, 0}, 264: {"diamond", 0, 0}, 265: {"iron_ingot", 0, 0}, 266: {"gold_ingot", 0, 0},
	267: {"iron_sword", 1, 250}, 268: {"wooden_sword", 1, 59}, 269: {"wooden_shovel", 1, 59}, 270: {"wooden_pickaxe", 1, 59}, 271: {"wooden_axe", 1, 59},
	272: {"stone_sword", 1, 131}, 273: {"stone_shovel", 1, 131}, 274: {"stone_pickaxe", 1, 131}, 275: {"stone_axe", 1, 131},
	276: {"diamond_sword", 1, 1561}, 277: {"diamond_shovel", 1, 1561}, 278: {"diamond_pickaxe", 1, 1561}, 279: {"diamond_axe", 1, 1561},
	280: {"stick", 0, 0}, 281: {"bowl", 0, 0}, 282: {"mushroom_stew", 1, 0},
	283: {"golden_sword", 1, 32}, 284: {"golden_shovel", 1, 32}, 285: {"golden_pickaxe", 1, 32}, 286: {"golden_axe", 1, 32},
	287: {"string", 0, 0}, 288: {"feather", 0, 0}, 289: {"gunpowder", 0, 0},
	290: {"wooden_hoe", 1, 59}, 291: {"stone_hoe", 1, 131}, 292: {"iron_hoe", 1, 250}, 293: {"diamond_hoe", 1, 1561}, 294: {"golden_hoe", 1, 32},
	295: {"wheat_seeds", 0, 0}, 296: {"wheat", 0, 0}, 297: {"bread", 0, 0},
	298: {"leather_helmet", 1, 55}, 299: {"leather_chestplate", 1, 80}, 300: {"leather_leggings", 1, 75}, 301: {"leather_boots", 1, 65},
	302: {"chainmail_helmet", 1, 165}, 303: {"chainmail_chestplate", 1, 240}, 304: {"chainmail_leggings", 1, 225}, 305: {"chainmail_boots", 1, 195},
	306: {"iron_helmet", 1, 165}, 307: {"iron_chestplate", 1, 240}, 308: {"iron_leggings", 1, 225}, 309: {"iron_boots", 1, 195},
	310: {"diamond_helmet", 1, 363}, 311: {"diamond_chestplate", 1, 528}, 312: {"diamond_leggings", 1, 495}, 313: {"diamond_boots", 1, 429},
	314: {"golden_helmet", 1, 77}, 315: {"golden_chestplate", 1, 112}, 316: {"golden_leggings", 1, 105}, 317: {"golden_boots", 1, 91},
	318: {"flint", 0, 0}, 319: {"porkchop", 0, 0}, 320: {"cooked_porkchop", 0, 0}, 321: {"painting", 0, 0}, 322: {"golden_apple", 0, 0},
	323: {"sign", 16, 0}, 324: {"oak_door", 1, 0}, 325: {"bucket", 16, 0}, 326: {"water_bucket", 1, 0}, 327: {"lava_bucket", 1, 0},
	328: {"minecart", 1, 0}, 329: {"saddle", 1, 0}, 330: {"iron_door", 1, 0}, 331: {"redstone", 0, 0}, 332: {"snowball", 16, 0}, 333: {"boat", 1, 0},
	334: {"leather", 0, 0}, 335: {"milk_bucket", 1, 0}, 336: {"brick", 0, 0}, 337: {"clay_ball", 0, 0}, 338: {"reeds", 0, 0}, 339: {"paper", 0, 0},
	340: {"book", 0, 0}, 341: {"slime_ball", 0, 0}, 342: {"chest_minecart", 1, 0}, 343: {"furnace_minecart", 1, 0}, 344: {"egg", 16, 0},
	345: {"compass", 0, 0}, 346: {"fishing_rod", 1, 64}, 347: {"clock", 0, 0}, 348: {"glowstone_dust", 0, 0}, 349: {"cod", 0, 0}, 350: {"cooked_cod", 0, 0},
	351: {"dye_black", 0, 0}, 352: {"bone", 0, 0}, 353: {"sugar", 0, 0}, 354: {"cake", 1, 0}, 355: {"bed", 1, 0}, 356: {"repeater", 0, 0},
	357: {"cookie", 0, 0}, 358: {"filled_map", 0, 0}, 359: {"shears", 1, 238}, 360: {"melon", 0, 0}, 361: {"pumpkin_seeds", 0, 0}, 362: {"melon_seeds", 0, 0},
	363: {"beef", 0, 0}, 364: {"cooked_beef", 0, 0}, 365: {"chicken", 0, 0}, 366: {"cooked_chicken", 0, 0}, 367: {"rotten_flesh", 0, 0},
	368: {"ender_pearl", 16, 0}, 369: {"blaze_rod", 0, 0}, 370: {"ghast_tear", 0, 0}, 371: {"gold_nugget", 0, 0}, 372: {"nether_wart", 0, 0},
	374: {"glass_bottle", 0, 0}, 375: {"spider_eye", 0, 0}, 376: {"fermented_spider_eye", 0, 0}, 377: {"blaze_powder", 0, 0}, 378: {"magma_cream", 0, 0},
	379: {"brewing_stand", 0, 0}, 380: {"cauldron", 0, 0}, 381: {"ender_eye", 0, 0}, 382: {"speckled_melon", 0, 0}, 384: {"experience_bottle", 0, 0},
	385: {"fire_charge", 0, 0}, 388: {"emerald", 0, 0}, 389: {"item_frame", 0, 0}, 390: {"flower_pot", 0, 0}, 391: {"carrot", 0, 0}, 392: {"potato", 0, 0},
	393: {"baked_potato", 0, 0}, 394: {"poisonous_potato", 0, 0}, 396: {"golden_carrot", 0, 0}, 398: {"carrot_on_a_stick", 1, 25}, 399: {"nether_star", 0, 0},
	400: {"pumpkin_pie", 0, 0}, 404: {"comparator", 0, 0}, 405: {"netherbrick", 0, 0}, 406: {"quartz", 0, 0}, 409: {"prismarine_shard", 0, 0},
	410: {"prismarine_crystals", 0, 0}, 411: {"rabbit", 0, 0}, 412: {"cooked_rabbit", 0, 0}, 413: {"rabbit_stew", 1, 0}, 414: {"rabbit_foot", 0, 0},
	415: {"rabbit_hide", 0, 0}, 416: {"armor_stand", 16, 0}, 420: {"lead", 0, 0}, 427: {"spruce_door", 1, 0}, 428: {"birch_door", 1, 0}, 429: {"jungle_door", 1, 0}, 430: {"acacia_door", 1, 0}, 431: {"dark_oak_door", 1, 0}, 421: {"name_tag", 0, 0}, 423: {"mutton", 0, 0}, 424: {"cooked_mutton", 0, 0},
}

var dyeNames = [16]string{"black", "red", "green", "brown", "blue", "purple", "cyan", "silver", "gray", "pink", "lime", "yellow", "light_blue", "magenta", "orange", "white"}

// itemModelName is the model in models/item for an item, which is where its picture comes from.
func itemModelName(s slot) string {
	switch s.ID {
	case 263:
		return [2]string{"coal", "charcoal"}[s.Dmg&1]
	case 351:
		return "dye_" + dyeNames[s.Dmg&15]
	case 349:
		return [4]string{"cod", "salmon", "clownfish", "pufferfish"}[s.Dmg&3]
	case 350:
		return [2]string{"cooked_cod", "cooked_salmon"}[s.Dmg&1]
	}
	return itemDefs[s.ID].name
}

// itemName is a display name, in the game's English, for the few places that print one.
func itemName(s slot) string {
	if s.isBlock() {
		return fmt.Sprintf("Block %d:%d", s.ID, s.Dmg)
	}
	return itemModelName(s)
}

// durability is how many uses a tool has, 0 for an item that does not wear.
func (s slot) durability() int { return int(itemDefs[s.ID].durability) }
