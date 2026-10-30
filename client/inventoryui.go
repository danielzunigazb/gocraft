package client

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// The details of the inventory screens: the names of items (from the jar's own en_US.lang), tooltips,
// the tabs of the creative inventory, blocks drawn as little cubes, and the player in the window.

// ---- names ----

// parseLang reads a .lang file: key=value lines.
func parseLang(b []byte) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || line[0] == '#' {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[k] = v
		}
	}
	return m
}

var colourKeys = [16]string{"white", "orange", "magenta", "lightBlue", "yellow", "lime", "pink", "gray", "silver", "cyan", "purple", "blue", "brown", "green", "red", "black"}
var dyeKeys = [16]string{"black", "red", "green", "brown", "blue", "purple", "cyan", "silver", "gray", "pink", "lime", "yellow", "lightBlue", "magenta", "orange", "white"}
var woodKeys = [6]string{"oak", "spruce", "birch", "jungle", "acacia", "big_oak"}

// langKey is the key of a variant's name in en_US.lang, for the blocks and items whose variants have names.
func langKey(s slot) string {
	d := int(s.Dmg)
	pick := func(list []string) string { return list[d%len(list)] }
	switch s.ID {
	case 1:
		return "tile.stone." + pick([]string{"stone", "granite", "graniteSmooth", "diorite", "dioriteSmooth", "andesite", "andesiteSmooth"}) + ".name"
	case 3:
		return "tile.dirt." + pick([]string{"default", "coarse", "podzol"}) + ".name"
	case 5:
		return "tile.wood." + pick(woodKeys[:]) + ".name"
	case 6:
		return "tile.sapling." + pick(woodKeys[:]) + ".name"
	case 12:
		return "tile.sand." + pick([]string{"default", "red"}) + ".name"
	case 17:
		return "tile.log." + pick(woodKeys[:4]) + ".name"
	case 162:
		return "tile.log." + pick(woodKeys[4:]) + ".name"
	case 18:
		return "tile.leaves." + pick(woodKeys[:4]) + ".name"
	case 161:
		return "tile.leaves." + pick(woodKeys[4:]) + ".name"
	case 24:
		return "tile.sandStone." + pick([]string{"default", "chiseled", "smooth"}) + ".name"
	case 35:
		return "tile.cloth." + colourKeys[d&15] + ".name"
	case 95:
		return "tile.stainedGlass." + colourKeys[d&15] + ".name"
	case 160:
		return "tile.thinStainedGlass." + colourKeys[d&15] + ".name"
	case 159:
		return "tile.clayHardenedStained." + colourKeys[d&15] + ".name"
	case 171:
		return "tile.woolCarpet." + colourKeys[d&15] + ".name"
	case 43, 44:
		return "tile.stoneSlab." + [8]string{"stone", "sand", "wood", "cobble", "brick", "smoothStoneBrick", "netherBrick", "quartz"}[d&7] + ".name"
	case 125, 126:
		return "tile.woodSlab." + woodKeys[d%6] + ".name"
	case 98:
		return "tile.stonebricksmooth." + pick([]string{"default", "mossy", "cracked", "chiseled"}) + ".name"
	case 155:
		return "tile.quartzBlock." + pick([]string{"default", "chiseled", "lines"}) + ".name"
	case 139:
		return "tile.cobbleWall." + pick([]string{"normal", "mossy"}) + ".name"
	case 175:
		return "tile.doublePlant." + pick([]string{"sunflower", "syringa", "grass", "fern", "rose", "paeonia"}) + ".name"
	case 38:
		return "tile.flower2." + pick([]string{"poppy", "blueOrchid", "allium", "houstonia", "tulipRed", "tulipOrange", "tulipWhite", "tulipPink", "oxeyeDaisy"}) + ".name"
	case 351:
		return "item.dyePowder." + dyeKeys[d&15] + ".name"
	case 263:
		return []string{"item.coal.name", "item.charcoal.name"}[d&1]
	case 349:
		return "item.fish." + [4]string{"cod", "salmon", "clownfish", "pufferfish"}[d&3] + ".raw.name"
	case 350:
		return "item.fish." + [2]string{"cod", "salmon"}[d&1] + ".cooked.name"
	case 322:
		if d == 1 {
			return "item.appleGold.name"
		}
		return "item.appleGold.name"
	}
	return ""
}

// nameOverrides are the registry names whose plain capitalisation is not what the game calls the thing.
var nameOverrides = map[string]string{
	"grass": "Grass Block", "stonebrick": "Stone Bricks", "web": "Cobweb", "tallgrass": "Tall Grass", "deadbush": "Dead Bush",
	"yellow_flower": "Dandelion", "red_flower": "Poppy", "brick_block": "Bricks", "quartz": "Nether Quartz", "netherbrick": "Nether Brick",
	"speckled_melon": "Glistering Melon", "beef": "Raw Beef", "porkchop": "Raw Porkchop", "chicken": "Raw Chicken", "mutton": "Raw Mutton",
	"rabbit": "Raw Rabbit", "ender_eye": "Eye of Ender", "experience_bottle": "Bottle o' Enchanting", "slime_ball": "Slimeball",
	"clay_ball": "Clay", "carrot_on_a_stick": "Carrot on a Stick", "leather_helmet": "Leather Cap", "leather_chestplate": "Leather Tunic",
	"leather_leggings": "Leather Pants", "leather_boots": "Leather Boots", "chainmail_helmet": "Chain Helmet", "chainmail_chestplate": "Chain Chestplate",
	"chainmail_leggings": "Chain Leggings", "chainmail_boots": "Chain Boots", "golden_apple": "Golden Apple", "melon": "Melon Slice", "melon_block": "Melon",
	"reeds": "Sugar Canes", "bow": "Bow", "mob_spawner": "Monster Spawner", "end_stone": "End Stone", "log": "Wood", "log2": "Wood", "planks": "Wooden Planks",
	"lit_furnace": "Furnace", "lit_redstone_ore": "Redstone Ore", "double_stone_slab": "Stone Slab", "wooden_slab": "Wooden Slab", "stone_slab": "Stone Slab",
	"fence": "Oak Fence", "fence_gate": "Oak Fence Gate", "wooden_door": "Oak Door", "wooden_button": "Button", "stone_button": "Button",
	"golden_rail": "Powered Rail", "gold_nugget": "Gold Nugget", "firework_charge": "Firework Star", "fireworks": "Firework Rocket", "dragon_egg": "Dragon Egg",
	"lit_pumpkin": "Jack o'Lantern", "hardened_clay": "Hardened Clay", "stained_hardened_clay": "Stained Clay", "waterlily": "Lily Pad",
	"cobblestone_wall": "Cobblestone Wall", "snow_layer": "Snow", "snow": "Snow Block", "noteblock": "Note Block", "fish": "Raw Fish", "bed": "Bed",
	"wheat": "Wheat", "wheat_seeds": "Seeds", "string": "String", "bone": "Bone", "snowball": "Snowball", "brick": "Brick",
}

var smallWords = map[string]bool{"of": true, "on": true, "a": true, "the": true, "and": true}

// titleName turns a registry name like "diamond_sword" into "Diamond Sword".
func titleName(reg string) string {
	if n, ok := nameOverrides[reg]; ok {
		return n
	}
	words := strings.Split(reg, "_")
	for i, w := range words {
		if w == "" || (i > 0 && smallWords[w]) {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// displayName is what the game calls an item: from the lang file where the variant has a key there,
// otherwise from its registry name.
func (g *game) displayName(s slot) string {
	if s.empty() {
		return ""
	}
	if k := langKey(s); k != "" {
		if v, ok := g.res.lang[k]; ok {
			return v
		}
	}
	if s.isBlock() {
		file, _ := g.res.stateOf(s.ID, int(s.Dmg), func(int32, int32, int32) uint16 { return 0 })
		if file == "" {
			return titleName(fmt.Sprintf("block_%d", s.ID))
		}
		return titleName(file)
	}
	return titleName(itemModelName(s))
}

// drawTooltip draws a name in a box by the mouse, as the game does: dark, with a purple edge.
func (g *game) drawTooltip(text string, mx, my, s float32) {
	if text == "" {
		return
	}
	hd := &g.hud
	w := float32(g.textWidth(text)) * s
	x, y := mx+12*s, my-12*s
	if x+w+8*s > float32(g.fb[0]) {
		x = mx - 16*s - w
	}
	y = max(y, 2*s)
	hd.rect(x-4*s, y-3*s, w+8*s, 14*s, 0, 0, 1, 1, 1, 1, [4]float32{0.31, 0, 1, 0.9}) // the edge
	hd.rect(x-3*s, y-2*s, w+6*s, 12*s, 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0, 0.06, 0.95})
	hd.flush(hd.white)
	g.text(text, x, y, s, [4]float32{1, 1, 1, 1}, true)
}

// ---- blocks as little cubes ----

// flatIcons are the blocks that are drawn flat in the inventory, as pictures of what they look like
// from the front: plants, torches, rails, ladders and the like.
var flatIcons = idSet(6, 27, 28, 31, 32, 37, 38, 39, 40, 50, 55, 65, 66, 69, 75, 76, 77, 106, 111, 131, 132, 143, 157, 175, 83, 59, 115)

var faceNormals = [6][3]float32{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}

// rotateIcon turns a point of the unit block about its centre as the inventory does for a block: a
// quarter turn and a half about y (225 degrees) and then 30 about x, so the top and two sides show.
func rotateIcon(p [3]float32) [3]float32 {
	const sy, cy = -0.70710678, -0.70710678 // sin and cos of 225 degrees
	const sx, cx = 0.5, 0.8660254           // sin and cos of 30 degrees
	x, y, z := p[0], p[1], p[2]
	x, z = x*cy+z*sy, -x*sy+z*cy
	y, z = y*cx-z*sx, y*sx+z*cx
	return [3]float32{x, y, z}
}

// icon3D queues a block as a cube in the slot at x, y; false when the block is drawn flat.
func (g *game) icon3D(state uint16, x, y, size float32) bool {
	id := state >> 4
	if flatIcons[id] {
		return false
	}
	file, props := g.res.stateOf(id, int(state&15), func(int32, int32, int32) uint16 { return 0 })
	vs := g.res.pick(file, props)
	if len(vs) == 0 || len(vs[0].quads) == 0 {
		return false
	}
	type face struct {
		p   [4][3]float32
		uv  [4][2]float32
		col [3]float32
		z   float32
	}
	var faces []face
	var tint [3]float32
	tinted := false
	for i := range vs[0].quads {
		q := &vs[0].quads[i]
		n := rotateIcon(faceNormals[q.dir])
		if n[2] <= 0.01 { // turned away
			continue
		}
		f := face{uv: q.auv, col: [3]float32{q.shade, q.shade, q.shade}}
		if q.tint >= 0 {
			if !tinted {
				tint, tinted = g.res.tintFor(id, int(state&15), 1), true
			}
			f.col = [3]float32{tint[0] * q.shade, tint[1] * q.shade, tint[2] * q.shade}
		}
		for k := 0; k < 4; k++ {
			f.p[k] = rotateIcon([3]float32{q.apos[k][0] - 0.5, q.apos[k][1] - 0.5, q.apos[k][2] - 0.5})
			f.z += f.p[k][2] / 4
		}
		faces = append(faces, f)
	}
	if len(faces) == 0 {
		return false
	}
	sort.SliceStable(faces, func(i, j int) bool { return faces[i].z < faces[j].z }) // far to near
	hd := &g.hud
	sc := size * 0.62
	cx, cy := x+size/2, y+size/2
	for _, f := range faces {
		for _, k := range [6]int{0, 1, 2, 0, 2, 3} {
			hd.verts = append(hd.verts, cx+f.p[k][0]*sc, cy-f.p[k][1]*sc, f.uv[k][0], f.uv[k][1], f.col[0], f.col[1], f.col[2], 1)
		}
	}
	return true
}

// ---- the creative tabs ----

const creativeTabCount = 10

var creativeTabNames = [creativeTabCount]string{"Building Blocks", "Decoration Blocks", "Redstone", "Transportation", "Miscellaneous",
	"Foodstuffs", "Tools", "Combat", "Brewing", "Materials"}

// creativeTabIcons are what the tabs show: a brick, a flower, redstone, a rail, a bucket, an apple, an axe,
// a sword, a bottle and a stick.
var creativeTabIcons = [creativeTabCount]slot{blockSlot(45<<4, 1), blockSlot(175<<4, 1), {ID: 331, N: 1}, blockSlot(27<<4, 1), {ID: 327, N: 1},
	{ID: 260, N: 1}, {ID: 258, N: 1}, {ID: 283, N: 1}, {ID: 374, N: 1}, {ID: 280, N: 1}}

var (
	tabRedstone  = idSet(23, 25, 29, 33, 46, 69, 70, 72, 76, 77, 96, 107, 123, 131, 143, 146, 147, 148, 151, 152, 154, 158, 167, 183, 184, 185, 186, 187, 331, 324, 330, 356, 404, 410)
	tabTransport = idSet(27, 28, 66, 157, 328, 329, 333, 342, 343, 398, 407, 408, 417, 418, 419)
	tabDecor     = idSet(6, 20, 30, 31, 32, 37, 38, 39, 40, 47, 50, 54, 58, 61, 65, 78, 79, 81, 84, 85, 86, 91, 95, 102, 103, 106, 111, 113, 116, 120, 122, 130, 138, 139, 145, 160, 170, 171, 175, 188, 189, 190, 191, 192, 321, 323, 354, 355, 379, 380, 389, 390, 397, 416, 425)
	tabFood      = idSet(260, 282, 297, 319, 320, 322, 349, 350, 357, 360, 363, 364, 365, 366, 367, 391, 392, 393, 394, 396, 400, 411, 412, 413, 423, 424)
	tabTools     = idSet(256, 257, 258, 259, 269, 270, 271, 273, 274, 275, 277, 278, 279, 284, 285, 286, 290, 291, 292, 293, 294, 325, 326, 327, 335, 345, 346, 347, 358, 359, 420, 421)
	tabCombat    = idSet(261, 262, 267, 268, 272, 276, 283, 298, 299, 300, 301, 302, 303, 304, 305, 306, 307, 308, 309, 310, 311, 312, 313, 314, 315, 316, 317, 344)
	tabBrewing   = idSet(369, 370, 372, 373, 374, 375, 376, 377, 378, 382, 384, 414)
	tabMaterials = idSet(263, 264, 265, 266, 280, 281, 287, 288, 289, 295, 296, 318, 334, 336, 337, 339, 340, 341, 348, 351, 352, 353, 361, 362, 368, 371, 381, 385, 388, 399, 402, 405, 406, 409, 410, 415)
)

// creativeTabOf is the tab an item is listed under.
func creativeTabOf(s slot) int {
	id := s.ID
	switch {
	case tabRedstone[id]:
		return 2
	case tabTransport[id]:
		return 3
	case tabBrewing[id]:
		return 8
	case tabFood[id]:
		return 5
	case tabCombat[id]:
		return 7
	case tabTools[id]:
		return 6
	case tabMaterials[id] && id >= 256:
		return 9
	case tabDecor[id]:
		return 1
	case id < 256:
		return 0
	}
	return 4
}

// creativeShown is the list the creative inventory shows: the items of the chosen tab.
func (g *game) creativeShown() []slot {
	all := g.creativeItems()
	if len(all) == 0 {
		return all
	}
	if g.be.tabFor != &all[0] {
		for i := range g.be.tabLists {
			g.be.tabLists[i] = nil
		}
		for _, s := range all {
			t := creativeTabOf(s)
			g.be.tabLists[t] = append(g.be.tabLists[t], s)
		}
		g.be.tabFor = &all[0]
	}
	return g.be.tabLists[g.be.tab%creativeTabCount]
}

// tabRect is where a tab is, in the panel's GUI pixels: x, y and whether it is in the upper row.
func tabRect(i int) (x, y float32, upper bool) {
	col := i % 5
	x = float32(col * 28)
	if col > 0 {
		x += float32(col)
	}
	if i < 5 {
		return x, -28, true
	}
	return x, invH - 4, false
}

// tabAt is the tab under a point of the panel, or -1.
func tabAt(px, py float32) int {
	for i := 0; i < creativeTabCount; i++ {
		x, y, upper := tabRect(i)
		h := float32(28)
		if !upper {
			y += 4
		}
		if px >= x && px < x+28 && py >= y && py < y+h {
			return i
		}
	}
	return -1
}

// drawTabs draws the tabs of the creative inventory around the panel, from the jar's tabs.png.
func (g *game) drawTabs(left, top, s float32) {
	hd := &g.hud
	one := [4]float32{1, 1, 1, 1}
	for pass := 0; pass < 2; pass++ { // the chosen tab goes over its neighbours
		for i := 0; i < creativeTabCount; i++ {
			sel := i == g.be.tab
			if sel != (pass == 1) {
				continue
			}
			x, y, upper := tabRect(i)
			u, v := float32(i%5*28), float32(0)
			if !upper {
				v = 64
			}
			if sel {
				v += 32
			}
			hd.rect(left+x*s, top+y*s, 28*s, 32*s, u, v, 28, 32, 256, 256, one)
		}
	}
	hd.flush(g.guiTexture("tabs"))
	for i := 0; i < creativeTabCount; i++ {
		x, y, upper := tabRect(i)
		dy := float32(9)
		if upper {
			dy = 7
		}
		g.drawSlotIcon(creativeTabIcons[i], left+(x+6)*s, top+(y+dy)*s, 16*s)
	}
	hd.flush(g.tex)
	hd.flushItems()
}

// ---- the player in the window ----

// drawPlayerPreview draws the player's model in the inventory window, turned towards the mouse.
func (g *game) drawPlayerPreview(left, top, s float32) {
	if playerMdl == nil || g.mobTex == nil {
		return
	}
	mx, my := g.mouse()
	fx, fy := left+51*s, top+75*s
	dx, dy := float64(fx-mx)/float64(s), float64(fy-70*s-my)/float64(s)
	body := float32(math.Atan(-dx/40)) * 20 * 180 / math.Pi // the body turns a little, the head more
	head := body
	pitch := -float32(math.Atan(dy/40)) * 20
	st := poseState{headYaw: head, headPitch: pitch, swing: -1, age: float32(g.worldTime), sneaking: g.sneaking}
	vs := emitModel(nil, playerMdl, playerRots(st), vec3{}, body+180, 0, [3]uint8{255, 255, 255}, 0xF0)
	type quad struct {
		v [4]vertex
		z float32
	}
	quads := make([]quad, 0, len(vs)/4)
	for i := 0; i+3 < len(vs); i += 4 {
		q := quad{}
		for k := 0; k < 4; k++ {
			q.v[k] = vs[i+k]
			q.z += vs[i+k].Z / 4
		}
		quads = append(quads, q)
	}
	sort.SliceStable(quads, func(i, j int) bool { return quads[i].z < quads[j].z })
	hd := &g.hud
	sc := 30 * s
	for _, q := range quads {
		for _, k := range [6]int{0, 1, 2, 0, 2, 3} {
			v := q.v[k]
			hd.verts = append(hd.verts, fx+v.X*sc, fy-v.Y*sc, v.U, v.V, float32(v.C[0])/255, float32(v.C[1])/255, float32(v.C[2])/255, 1)
		}
	}
	hd.flush(g.mobTex["look/skin"])
}
