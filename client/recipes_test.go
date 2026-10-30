package client

import "testing"

func grid(w, h int, cells ...slot) []slot {
	g := make([]slot, w*h)
	copy(g, cells)
	return g
}

func craft(t *testing.T, gw, gh int, cells map[int]slot) slot {
	g := make([]slot, gw*gh)
	for i, s := range cells {
		g[i] = s
	}
	out, ok := findRecipe(g, gw, gh)
	if !ok {
		return slot{}
	}
	return out
}

var (
	oakLog = slot{ID: 17, N: 1}
	oakPl  = slot{ID: 5, N: 1}
	stk    = slot{ID: 280, N: 1}
	cobb   = slot{ID: 4, N: 1}
	ironI  = slot{ID: 265, N: 1}
)

func TestPlanksAndSticksAnywhereInTheGrid(t *testing.T) {
	for cell := 0; cell < 4; cell++ {
		if got := craft(t, 2, 2, map[int]slot{cell: oakLog}); got != (slot{ID: 5, N: 4}) {
			t.Errorf("log in cell %d made %v", cell, got)
		}
	}
	if got := craft(t, 2, 2, map[int]slot{0: {ID: 17, Dmg: 2, N: 1}}); got != (slot{ID: 5, Dmg: 2, N: 4}) {
		t.Errorf("a birch log made %v", got)
	}
	if got := craft(t, 2, 2, map[int]slot{0: {ID: 162, Dmg: 1, N: 1}}); got != (slot{ID: 5, Dmg: 5, N: 4}) {
		t.Errorf("a dark oak log made %v", got)
	}
	// Two planks one above the other make sticks, in either column of a 2x2 grid; side by side they do not.
	for _, cells := range []map[int]slot{{0: oakPl, 2: oakPl}, {1: oakPl, 3: oakPl}} {
		if got := craft(t, 2, 2, cells); got != (slot{ID: 280, N: 4}) {
			t.Errorf("%v made %v", cells, got)
		}
	}
	if got := craft(t, 2, 2, map[int]slot{0: oakPl, 1: oakPl}); got.ID == 280 {
		t.Error("planks side by side made sticks")
	}
	if got := craft(t, 3, 3, map[int]slot{4: oakPl, 7: oakPl}); got != (slot{ID: 280, N: 4}) {
		t.Errorf("sticks in the middle of a table: %v", got)
	}
}

func TestCraftingTableAndFurnace(t *testing.T) {
	if got := craft(t, 2, 2, map[int]slot{0: oakPl, 1: {ID: 5, Dmg: 3, N: 1}, 2: oakPl, 3: oakPl}); got != (slot{ID: 58, N: 1}) {
		t.Errorf("four planks of mixed kinds: %v", got) // any planks will do
	}
	ring := map[int]slot{}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 7, 8} {
		ring[i] = cobb
	}
	if got := craft(t, 3, 3, ring); got != (slot{ID: 61, N: 1}) {
		t.Errorf("eight cobblestone: %v", got)
	}
	ring[4] = cobb
	if got := craft(t, 3, 3, ring); got.ID == 61 {
		t.Error("a full block of cobblestone made a furnace")
	}
	// A furnace does not fit the 2x2 grid.
	if got := craft(t, 2, 2, map[int]slot{0: cobb, 1: cobb, 2: cobb, 3: cobb}); got.ID == 61 {
		t.Error("a furnace in a 2x2 grid")
	}
}

func TestToolsAndMirrors(t *testing.T) {
	pick := map[int]slot{0: ironI, 1: ironI, 2: ironI, 4: stk, 7: stk}
	if got := craft(t, 3, 3, pick); got != (slot{ID: 257, N: 1}) {
		t.Errorf("iron pickaxe: %v", got)
	}
	// An axe, and its mirror image, either side of the stick.
	for _, cells := range []map[int]slot{{0: ironI, 1: ironI, 3: ironI, 4: stk, 7: stk}, {0: ironI, 1: ironI, 3: stk, 4: ironI, 6: stk}} {
		if got := craft(t, 3, 3, cells); got != (slot{ID: 258, N: 1}) {
			t.Errorf("iron axe %v: %v", cells, got)
		}
	}
	// A sword in a 2x2 grid is not possible (it is three tall) but fits the table anywhere.
	if got := craft(t, 3, 3, map[int]slot{2: {ID: 264, N: 1}, 5: {ID: 264, N: 1}, 8: stk}); got != (slot{ID: 276, N: 1}) {
		t.Errorf("diamond sword on the right edge: %v", got)
	}
	if got := craft(t, 3, 3, map[int]slot{0: oakPl, 3: oakPl, 6: stk}); got != (slot{ID: 268, N: 1}) {
		t.Errorf("wooden sword: %v", got)
	}
	// With a stray item in the grid nothing is made.
	bad := map[int]slot{0: ironI, 1: ironI, 2: ironI, 4: stk, 7: stk, 8: cobb}
	if got := craft(t, 3, 3, bad); !got.empty() && got.ID == 257 {
		t.Error("an extra item still made a pickaxe")
	}
}

func TestShapelessAndStairs(t *testing.T) {
	if got := craft(t, 2, 2, map[int]slot{0: {ID: 318, N: 1}, 3: ironI}); got != (slot{ID: 259, N: 1}) {
		t.Errorf("flint and steel: %v", got)
	}
	if got := craft(t, 3, 3, map[int]slot{0: {ID: 5, Dmg: 1, N: 1}, 3: {ID: 5, Dmg: 1, N: 1}, 4: {ID: 5, Dmg: 1, N: 1}, 6: {ID: 5, Dmg: 1, N: 1}, 7: {ID: 5, Dmg: 1, N: 1}, 8: {ID: 5, Dmg: 1, N: 1}}); got != (slot{ID: 134, N: 4}) {
		t.Errorf("spruce stairs: %v", got)
	}
	// Mirrored stairs, rising to the other side.
	if got := craft(t, 3, 3, map[int]slot{2: cobb, 4: cobb, 5: cobb, 6: cobb, 7: cobb, 8: cobb}); got != (slot{ID: 67, N: 4}) {
		t.Errorf("mirrored cobblestone stairs: %v", got)
	}
	if got := craft(t, 3, 3, map[int]slot{3: oakPl, 4: oakPl, 5: oakPl}); got != (slot{ID: 126, Dmg: 0, N: 6}) {
		t.Errorf("slabs: %v", got)
	}
	if got := craft(t, 3, 3, map[int]slot{3: {ID: 4, N: 1}, 4: cobb, 5: cobb}); got != (slot{ID: 44, Dmg: 3, N: 6}) {
		t.Errorf("cobblestone slabs: %v", got)
	}
}

func TestBlocksAndTheirIngots(t *testing.T) {
	nine := map[int]slot{}
	for i := 0; i < 9; i++ {
		nine[i] = ironI
	}
	if got := craft(t, 3, 3, nine); got != (slot{ID: 42, N: 1}) {
		t.Errorf("iron block: %v", got)
	}
	if got := craft(t, 2, 2, map[int]slot{3: {ID: 42, N: 1}}); got != (slot{ID: 265, N: 9}) {
		t.Errorf("an iron block back into ingots: %v", got)
	}
	lapis := map[int]slot{}
	for i := 0; i < 9; i++ {
		lapis[i] = slot{ID: 351, Dmg: 4, N: 1}
	}
	if got := craft(t, 3, 3, lapis); got != (slot{ID: 22, N: 1}) {
		t.Errorf("lapis block from blue dye: %v", got)
	}
	lapis[0] = slot{ID: 351, Dmg: 1, N: 1} // red dye is not lapis
	if got := craft(t, 3, 3, lapis); got.ID == 22 {
		t.Error("red dye made a lapis block")
	}
}

func TestEveryRecipeIsWellFormedAndDrawable(t *testing.T) {
	g := testGame(t)
	if len(recipes) < 180 {
		t.Fatalf("only %d recipes", len(recipes))
	}
	for i, r := range recipes {
		if r.out.empty() {
			t.Fatalf("recipe %d has no result", i)
		}
		if !r.shapeless && len(r.in) != r.w*r.h {
			t.Fatalf("recipe %d (%v) has %d cells for %dx%d", i, r.out, len(r.in), r.w, r.h)
		}
		if r.w > 3 || r.h > 3 {
			t.Fatalf("recipe %d is bigger than the table", i)
		}
		if r.shapeless && len(r.in) > 9 {
			t.Fatalf("recipe %d has %d ingredients", i, len(r.in))
		}
		for _, in := range r.in {
			if !in.empty() && in.id >= 256 && itemDefs[in.id].name == "" && in.id != 263 && in.id != 351 {
				t.Errorf("recipe %d uses unknown item %d", i, in.id)
			}
		}
		if r.out.isBlock() {
			if _, _, ok := g.blockIcon(r.out.state()); !ok {
				t.Errorf("recipe %d makes block %v with no icon", i, r.out)
			}
		} else if _, ok := g.res.itemTile(r.out); !ok {
			t.Errorf("recipe %d makes %v with no picture", i, r.out)
		}
	}
}

func TestNoRecipeShadowsAnother(t *testing.T) {
	// Feeding each recipe its own ingredients must give that recipe's result (a later recipe that
	// matches the same grid would never be reached).
	for i, r := range recipes {
		if r.shapeless {
			continue
		}
		cells := make([]slot, 9)
		for y := 0; y < r.h; y++ {
			for x := 0; x < r.w; x++ {
				if in := r.in[y*r.w+x]; !in.empty() {
					cells[y*3+x] = slot{ID: in.id, Dmg: in.dmg, N: 1}
				}
			}
		}
		out, ok := findRecipe(cells, 3, 3)
		if !ok || !out.same(r.out) {
			t.Errorf("recipe %d (%v) crafts %v from its own ingredients", i, r.out, out)
		}
	}
}
