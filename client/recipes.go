package client

// The crafting recipes of 1.8.9, written out: shaped ones are a pattern of ingredients that may
// sit anywhere in the grid and be mirrored, shapeless ones only need the right ingredients.

// ing is one ingredient: an item, with a damage value that must match unless any is set.
type ing struct {
	id, dmg uint16
	any     bool
}

func anyOf(id uint16) ing      { return ing{id: id, any: true} }
func variant(id, d uint16) ing { return ing{id: id, dmg: d} }
func item(id uint16) ing       { return ing{id: id, any: true} }
func (i ing) empty() bool      { return i.id == 0 }
func (i ing) matches(s slot) bool {
	if i.empty() {
		return s.empty()
	}
	return !s.empty() && s.ID == i.id && (i.any || s.Dmg == i.dmg)
}

type recipe struct {
	w, h      int
	in        []ing // w*h, row by row; id 0 is an empty cell
	shapeless bool
	out       slot
}

var recipes []recipe

// shaped adds a pattern; each character of the rows is looked up in key, a space is empty.
func shaped(out slot, key map[byte]ing, rows ...string) {
	r := recipe{w: len(rows[0]), h: len(rows), out: out}
	for _, row := range rows {
		for i := 0; i < len(row); i++ {
			r.in = append(r.in, key[row[i]]) // an unknown character, a space, is the zero ingredient
		}
	}
	recipes = append(recipes, r)
}

func shapeless(out slot, in ...ing) {
	recipes = append(recipes, recipe{in: in, shapeless: true, out: out})
}

func put(id uint16, n int8) slot     { return slot{ID: id, N: n} }
func putD(id, d uint16, n int8) slot { return slot{ID: id, Dmg: d, N: n} }

// matchShaped tries the recipe at every position of a gw x gh grid, as written and mirrored.
func (r *recipe) matchShaped(grid []slot, gw, gh int) bool {
	for oy := 0; oy+r.h <= gh; oy++ {
		for ox := 0; ox+r.w <= gw; ox++ {
			for _, mirror := range [2]bool{false, true} {
				if r.fitsAt(grid, gw, gh, ox, oy, mirror) {
					return true
				}
			}
		}
	}
	return false
}

func (r *recipe) fitsAt(grid []slot, gw, gh, ox, oy int, mirror bool) bool {
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			rx, ry := x-ox, y-oy
			var want ing
			if rx >= 0 && ry >= 0 && rx < r.w && ry < r.h {
				if mirror {
					rx = r.w - 1 - rx
				}
				want = r.in[ry*r.w+rx]
			}
			if !want.matches(grid[y*gw+x]) {
				return false
			}
		}
	}
	return true
}

func (r *recipe) matchShapeless(grid []slot) bool {
	var have []slot
	for _, s := range grid {
		if !s.empty() {
			have = append(have, s)
		}
	}
	if len(have) != len(r.in) {
		return false
	}
	used := make([]bool, len(have))
	for _, want := range r.in {
		found := false
		for i, s := range have {
			if !used[i] && want.matches(s) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// findRecipe is CraftingManager.findMatchingRecipe: the first recipe the grid makes.
func findRecipe(grid []slot, gw, gh int) (slot, bool) {
	for i := range recipes {
		r := &recipes[i]
		if (r.shapeless && r.matchShapeless(grid)) || (!r.shapeless && r.matchShaped(grid, gw, gh)) {
			return r.out, true
		}
	}
	return slot{}, false
}

func init() {
	planks := anyOf(5)
	cobble := anyOf(4)
	stick := item(280)

	// Logs into planks, four of the matching kind.
	for d := uint16(0); d < 4; d++ {
		shapeless(putD(5, d, 4), variant(17, d))
	}
	shapeless(putD(5, 4, 4), variant(162, 0))
	shapeless(putD(5, 5, 4), variant(162, 1))

	shaped(put(280, 4), map[byte]ing{'p': planks}, "p", "p")
	shaped(put(58, 1), map[byte]ing{'p': planks}, "pp", "pp")
	shaped(put(61, 1), map[byte]ing{'c': cobble}, "ccc", "c c", "ccc")
	shaped(put(54, 1), map[byte]ing{'p': planks}, "ppp", "p p", "ppp")
	shaped(put(50, 4), map[byte]ing{'c': item(263), 's': stick}, "c", "s")
	shaped(put(65, 3), map[byte]ing{'s': stick}, "s s", "sss", "s s")
	shaped(put(323, 3), map[byte]ing{'p': planks, 's': stick}, "ppp", "ppp", " s ")
	shaped(put(96, 2), map[byte]ing{'p': planks}, "ppp", "ppp")
	shaped(put(281, 4), map[byte]ing{'p': planks}, "p p", " p ")
	shaped(put(333, 1), map[byte]ing{'p': planks}, "p p", "ppp")
	shaped(put(25, 1), map[byte]ing{'p': planks, 'r': item(331)}, "ppp", "prp", "ppp")
	shaped(put(84, 1), map[byte]ing{'p': planks, 'd': item(264)}, "ppp", "pdp", "ppp")
	shaped(put(47, 1), map[byte]ing{'p': planks, 'b': item(340)}, "ppp", "bbb", "ppp")
	shaped(put(116, 1), map[byte]ing{'b': item(340), 'd': item(264), 'o': anyOf(49)}, " b ", "ddd", "ooo")

	// Wood-specific: stairs, slabs, fences, gates and doors for each planks kind.
	doors := [6]uint16{324, 427, 428, 429, 430, 431}
	fences := [6]uint16{85, 188, 189, 190, 192, 191}
	gates := [6]uint16{107, 183, 184, 185, 187, 186}
	stairs := [6]uint16{53, 134, 135, 136, 163, 164}
	for d := uint16(0); d < 6; d++ {
		p := variant(5, d)
		shaped(put(stairs[d], 4), map[byte]ing{'p': p}, "p  ", "pp ", "ppp")
		shaped(putD(126, d, 6), map[byte]ing{'p': p}, "ppp")
		shaped(put(fences[d], 3), map[byte]ing{'p': p, 's': stick}, "psp", "psp")
		shaped(put(gates[d], 1), map[byte]ing{'p': p, 's': stick}, "sps", "sps")
		shaped(put(doors[d], 3), map[byte]ing{'p': p}, "pp", "pp", "pp")
	}
	shaped(put(330, 3), map[byte]ing{'i': item(265)}, "ii", "ii", "ii")

	// Stone and the like.
	shaped(put(67, 4), map[byte]ing{'c': cobble}, "c  ", "cc ", "ccc")
	shaped(putD(44, 3, 6), map[byte]ing{'c': cobble}, "ccc")
	shaped(put(139, 6), map[byte]ing{'c': cobble}, "ccc", "ccc")
	shaped(putD(139, 1, 6), map[byte]ing{'c': anyOf(48)}, "ccc", "ccc")
	shaped(putD(44, 0, 6), map[byte]ing{'s': variant(1, 0)}, "sss")
	shaped(putD(98, 0, 4), map[byte]ing{'s': variant(1, 0)}, "ss", "ss")
	shaped(put(109, 4), map[byte]ing{'s': variant(98, 0)}, "s  ", "ss ", "sss")
	shaped(putD(44, 5, 6), map[byte]ing{'s': variant(98, 0)}, "sss")
	shaped(put(24, 1), map[byte]ing{'s': variant(12, 0)}, "ss", "ss")
	shaped(putD(24, 2, 4), map[byte]ing{'s': variant(24, 0)}, "ss", "ss")
	shaped(putD(24, 1, 1), map[byte]ing{'s': variant(44, 1)}, "s", "s")
	shaped(put(128, 4), map[byte]ing{'s': anyOf(24)}, "s  ", "ss ", "sss")
	shaped(putD(44, 1, 6), map[byte]ing{'s': anyOf(24)}, "sss")
	shaped(put(45, 1), map[byte]ing{'b': item(336)}, "bb", "bb")
	shaped(put(108, 4), map[byte]ing{'b': anyOf(45)}, "b  ", "bb ", "bbb")
	shaped(putD(44, 4, 6), map[byte]ing{'b': anyOf(45)}, "bbb")
	shaped(put(82, 1), map[byte]ing{'c': item(337)}, "cc", "cc")
	shaped(put(112, 1), map[byte]ing{'b': item(405)}, "bb", "bb")
	shaped(put(114, 4), map[byte]ing{'b': anyOf(112)}, "b  ", "bb ", "bbb")
	shaped(putD(44, 6, 6), map[byte]ing{'b': anyOf(112)}, "bbb")
	shaped(put(113, 6), map[byte]ing{'b': anyOf(112)}, "bbb", "bbb")
	shaped(put(155, 1), map[byte]ing{'q': item(406)}, "qq", "qq")
	shaped(put(156, 4), map[byte]ing{'q': anyOf(155)}, "q  ", "qq ", "qqq")
	shaped(putD(44, 7, 6), map[byte]ing{'q': anyOf(155)}, "qqq")
	shaped(put(89, 1), map[byte]ing{'g': item(348)}, "gg", "gg")
	shaped(put(102, 16), map[byte]ing{'g': anyOf(20)}, "ggg", "ggg")
	shaped(put(101, 16), map[byte]ing{'i': item(265)}, "iii", "iii")
	shaped(put(46, 1), map[byte]ing{'g': item(289), 's': variant(12, 0)}, "gsg", "sgs", "gsg")
	shapeless(put(35, 1), item(287), item(287), item(287), item(287))
	shaped(put(171, 3), map[byte]ing{'w': anyOf(35)}, "ww")
	shaped(put(355, 1), map[byte]ing{'w': anyOf(35), 'p': planks}, "www", "ppp")

	// Storage blocks and back.
	blocks := []struct {
		block, d  uint16
		item, idm uint16
	}{{173, 0, 263, 0}, {42, 0, 265, 0}, {41, 0, 266, 0}, {57, 0, 264, 0}, {22, 0, 351, 4}, {133, 0, 388, 0}, {152, 0, 331, 0}, {170, 0, 296, 0}}
	for _, b := range blocks {
		in := ing{id: b.item, dmg: b.idm}
		shaped(putD(b.block, b.d, 1), map[byte]ing{'x': in}, "xxx", "xxx", "xxx")
		if b.block != 170 {
			shapeless(putD(b.item, b.idm, 9), variant(b.block, b.d))
		} else {
			shapeless(put(296, 9), variant(170, 0))
		}
	}
	shapeless(put(371, 9), item(266))
	shaped(put(266, 1), map[byte]ing{'n': item(371)}, "nnn", "nnn", "nnn")

	// Tools and weapons by material: planks, cobblestone, iron, gold, diamond.
	mats := []struct {
		m                                ing
		sword, shovel, pickaxe, axe, hoe uint16
	}{{planks, 268, 269, 270, 271, 290}, {cobble, 272, 273, 274, 275, 291}, {item(265), 267, 256, 257, 258, 292},
		{item(266), 283, 284, 285, 286, 294}, {item(264), 276, 277, 278, 279, 293}}
	for _, t := range mats {
		k := map[byte]ing{'m': t.m, 's': stick}
		shaped(put(t.sword, 1), k, "m", "m", "s")
		shaped(put(t.shovel, 1), k, "m", "s", "s")
		shaped(put(t.pickaxe, 1), k, "mmm", " s ", " s ")
		shaped(put(t.axe, 1), k, "mm", "ms", " s")
		shaped(put(t.hoe, 1), k, "mm", " s", " s")
	}
	// Armour: leather, iron, gold, diamond (chainmail is not made).
	for _, a := range []struct {
		m                        ing
		helm, chest, legs, boots uint16
	}{{item(334), 298, 299, 300, 301}, {item(265), 306, 307, 308, 309}, {item(266), 314, 315, 316, 317}, {item(264), 310, 311, 312, 313}} {
		k := map[byte]ing{'m': a.m}
		shaped(put(a.helm, 1), k, "mmm", "m m")
		shaped(put(a.chest, 1), k, "m m", "mmm", "mmm")
		shaped(put(a.legs, 1), k, "mmm", "m m", "m m")
		shaped(put(a.boots, 1), k, "m m", "m m")
	}

	// Other things worth carrying.
	shaped(put(261, 1), map[byte]ing{'s': stick, 't': item(287)}, " st", "s t", " st")
	shaped(put(262, 4), map[byte]ing{'f': item(318), 's': stick, 'h': item(288)}, "f", "s", "h")
	shaped(put(325, 1), map[byte]ing{'i': item(265)}, "i i", " i ")
	shaped(put(359, 1), map[byte]ing{'i': item(265)}, " i", "i ")
	shapeless(put(259, 1), item(265), item(318))
	shaped(put(345, 1), map[byte]ing{'i': item(265), 'r': item(331)}, " i ", "iri", " i ")
	shaped(put(347, 1), map[byte]ing{'g': item(266), 'r': item(331)}, " g ", "grg", " g ")
	shaped(put(346, 1), map[byte]ing{'s': stick, 't': item(287)}, "  s", " st", "s t")
	shaped(put(328, 1), map[byte]ing{'i': item(265)}, "i i", "iii")
	shaped(put(66, 16), map[byte]ing{'i': item(265), 's': stick}, "i i", "isi", "i i")
	shaped(put(27, 6), map[byte]ing{'g': item(266), 's': stick, 'r': item(331)}, "g g", "gsg", "grg")
	shaped(put(339, 3), map[byte]ing{'r': item(338)}, "rrr")
	shapeless(put(340, 1), item(339), item(339), item(339), item(334))
	shapeless(put(353, 1), item(338))
	shaped(put(297, 1), map[byte]ing{'w': item(296)}, "www")
	shaped(put(357, 8), map[byte]ing{'w': item(296), 'c': variant(351, 3)}, "wcw")
	shaped(put(354, 1), map[byte]ing{'m': item(335), 's': item(353), 'e': item(344), 'w': item(296)}, "mmm", "ses", "www")
	shaped(put(322, 1), map[byte]ing{'g': item(266), 'a': item(260)}, "ggg", "gag", "ggg")
	shaped(put(396, 1), map[byte]ing{'n': item(371), 'c': item(391)}, "nnn", "ncn", "nnn")
	shapeless(put(282, 1), anyOf(39), anyOf(40), item(281))
	shapeless(put(400, 1), anyOf(86), item(353), item(344))
	shapeless(put(362, 1), item(360))
	shapeless(put(361, 4), anyOf(86))
	shapeless(put(377, 2), item(369))
	shaped(put(69, 1), map[byte]ing{'s': stick, 'c': cobble}, "s", "c")
	shapeless(put(77, 1), variant(1, 0))
	shapeless(put(143, 1), planks)
	shaped(put(70, 1), map[byte]ing{'s': variant(1, 0)}, "ss")
	shaped(put(72, 1), map[byte]ing{'p': planks}, "pp")
	shaped(put(76, 1), map[byte]ing{'r': item(331), 's': stick}, "r", "s")
	shaped(put(123, 1), map[byte]ing{'r': item(331), 'g': item(348)}, " r ", "rgr", " r ")
	shaped(put(33, 1), map[byte]ing{'p': planks, 'c': cobble, 'i': item(265), 'r': item(331)}, "ppp", "cic", "crc")
	shapeless(put(29, 1), anyOf(33), item(341))
	shaped(put(23, 1), map[byte]ing{'c': cobble, 'b': item(261), 'r': item(331)}, "ccc", "cbc", "crc")
	shaped(put(158, 1), map[byte]ing{'c': cobble, 'r': item(331)}, "ccc", "c c", "crc")
	shaped(put(154, 1), map[byte]ing{'i': item(265), 'c': anyOf(54)}, "i i", "ici", " i ")
	shaped(put(356, 1), map[byte]ing{'t': anyOf(76), 'r': item(331), 's': variant(1, 0)}, "trt", "sss")
	shaped(put(379, 1), map[byte]ing{'b': item(369), 'c': cobble}, " b ", "ccc")
	shaped(put(380, 1), map[byte]ing{'i': item(265)}, "i i", "i i", "iii")
	shaped(put(390, 1), map[byte]ing{'b': item(336)}, "b b", " b ")
	shaped(put(389, 1), map[byte]ing{'s': stick, 'l': item(334)}, "sss", "sls", "sss")
	shaped(put(321, 1), map[byte]ing{'s': stick, 'w': anyOf(35)}, "sss", "sws", "sss")
	shaped(put(416, 1), map[byte]ing{'s': stick, 'p': variant(44, 0)}, "sss", " s ", "sps")
}
