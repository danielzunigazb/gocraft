package client

// The windows with slots: the player's own (armour, a 2x2 grid and its result) and the crafting
// table's 3x3 grid. Clicks work as 1.8's Container.slotClick does: left picks up, puts down, swaps
// or merges a whole stack; right takes half or puts one down; shift moves a stack to the other part
// of the inventory.

type containerKind int

const (
	contNone containerKind = iota
	contPlayer
	contTable
	contChest
	contFurnace
)

type containerState struct {
	kind   containerKind
	grid   [9]slot
	result slot
	// A chest or furnace window: what it holds, the other half of a double chest, the rows of slots
	// it shows and where it is.
	be, be2 *blockEntity
	rows    int
	pos     [3]int32
}

func (c *containerState) size() (w, h int) {
	if c.kind == contTable {
		return 3, 3
	}
	return 2, 2
}

// recompute is onCraftMatrixChanged: what the grid makes right now.
func (g *game) recompute() {
	w, h := g.cont.size()
	out, ok := findRecipe(g.cont.grid[:w*h], w, h)
	if !ok {
		out = slot{}
	}
	g.cont.result = out
}

func (g *game) openContainer(k containerKind) {
	g.cont = containerState{kind: k}
	g.openScreen(screenContainer)
}

// closeContainer hands back what was left in the grid and on the mouse; what does not fit is thrown.
func (g *game) closeContainer() {
	w, h := g.cont.size()
	for i := 0; i < w*h; i++ {
		if s := g.cont.grid[i]; !s.empty() {
			g.throwItem(g.giveSlot(s))
		}
	}
	if !g.cursor.empty() {
		g.throwItem(g.giveSlot(g.cursor))
	}
	g.cont, g.cursor = containerState{}, slot{}
	g.closeScreen()
}

type slotKind byte

const (
	kInv slotKind = iota
	kGrid
	kResult
	kArmor
	kBE // a slot of a chest or furnace
)

type slotRef struct {
	p     *slot
	kind  slotKind
	index int // slot number within its kind
}

// armourKind is which armour slot an item goes in: 0 helmet, 1 chestplate, 2 leggings, 3 boots, or -1.
func armourKind(id uint16) int {
	if id < 298 || id > 317 {
		return -1
	}
	return int(id-298) % 4
}

func (g *game) acceptsIn(ref slotRef, s slot) bool {
	if ref.kind == kBE && g.cont.kind == contFurnace {
		switch ref.index {
		case 1:
			return fuelTime(s) > 0
		case 2:
			return false
		}
	}
	if ref.kind == kArmor {
		return armourKind(s.ID) == ref.index
	}
	if ref.kind == kResult {
		return false
	}
	return true
}

func (g *game) capacity(s slot) int {
	n := 0
	for _, in := range g.inv {
		switch {
		case in.empty():
			n += int(s.maxStack())
		case in.same(s):
			n += int(in.maxStack() - in.N)
		}
	}
	return n
}

// mergeInto adds a stack into inv[from:to] and returns what is left.
func (g *game) mergeInto(s slot, from, to int) slot {
	for pass := 0; pass < 2 && !s.empty(); pass++ {
		for i := from; i < to && !s.empty(); i++ {
			in := &g.inv[i]
			if pass == 0 && !in.empty() && in.same(s) && in.N < in.maxStack() {
				n := min(s.N, in.maxStack()-in.N)
				in.N, s.N = in.N+n, s.N-n
			} else if pass == 1 && in.empty() {
				n := min(s.N, s.maxStack())
				*in, s.N = slot{s.ID, s.Dmg, n}, s.N-n
			}
		}
	}
	if s.N <= 0 {
		return slot{}
	}
	return s
}

// consumeGrid uses up one of each ingredient after a craft.
func (g *game) consumeGrid() {
	w, h := g.cont.size()
	for i := 0; i < w*h; i++ {
		if g.cont.grid[i].N--; g.cont.grid[i].N <= 0 {
			g.cont.grid[i] = slot{}
		}
	}
}

func (g *game) clickResult(shift bool) {
	res := g.cont.result
	if res.empty() {
		return
	}
	if shift { // craft as many as the inventory has room for
		for n := 0; n < 64 && !g.cont.result.empty(); n++ {
			if g.capacity(g.cont.result) < int(g.cont.result.N) {
				break
			}
			g.giveSlot(g.cont.result)
			g.consumeGrid()
			g.recompute()
		}
		return
	}
	cur := &g.cursor
	switch {
	case cur.empty():
		*cur = res
	case cur.same(res) && int(cur.N)+int(res.N) <= int(cur.maxStack()):
		cur.N += res.N
	default:
		return
	}
	g.consumeGrid()
	g.recompute()
}

func (g *game) quickMove(ref slotRef) {
	s := *ref.p
	if s.empty() {
		return
	}
	var rest slot
	switch {
	case ref.kind == kInv && ref.index < 9:
		rest = g.mergeInto(s, 9, 36)
	case ref.kind == kInv:
		if k := armourKind(s.ID); k >= 0 && g.armor[k].empty() { // armour goes on
			g.armor[k], *ref.p = slot{s.ID, s.Dmg, 1}, slot{}
			return
		}
		rest = g.mergeInto(s, 0, 9)
	default: // from the grid or an armour slot back into the inventory
		rest = g.mergeInto(s, 9, 36)
		rest = g.mergeInto(rest, 0, 9)
	}
	*ref.p = rest
	if ref.kind == kGrid {
		g.recompute()
	}
}

// clickSlot is one mouse click on a slot.
func (g *game) clickSlot(ref slotRef, right, shift bool) {
	if ref.kind == kResult {
		g.clickResult(shift)
		return
	}
	if g.beOutput(ref) {
		g.takeOutput(ref, shift)
		return
	}
	if shift {
		if g.cont.kind == contChest || g.cont.kind == contFurnace {
			g.quickMoveBE(ref)
		} else {
			g.quickMove(ref)
		}
		return
	}
	s, cur := ref.p, &g.cursor
	limit := func(st slot) int8 {
		if ref.kind == kArmor {
			return 1
		}
		return st.maxStack()
	}
	if !cur.empty() && !g.acceptsIn(ref, *cur) {
		return
	}
	switch {
	case !right && cur.empty() && s.empty():
	case !right && cur.empty():
		*cur, *s = *s, slot{}
	case !right && s.empty():
		n := min(cur.N, limit(*cur))
		*s = slot{cur.ID, cur.Dmg, n}
		cur.N -= n
	case !right && cur.same(*s):
		n := min(cur.N, limit(*s)-s.N)
		s.N, cur.N = s.N+n, cur.N-n
	case !right:
		*cur, *s = *s, *cur
	case cur.empty() && s.empty():
	case cur.empty(): // right click: take half
		n := (s.N + 1) / 2
		*cur = slot{s.ID, s.Dmg, n}
		if s.N -= n; s.N <= 0 {
			*s = slot{}
		}
	case s.empty():
		*s = slot{cur.ID, cur.Dmg, 1}
		cur.N--
	case cur.same(*s) && s.N < limit(*s):
		s.N++
		cur.N--
	default:
		*cur, *s = *s, *cur
	}
	if cur.N <= 0 {
		*cur = slot{}
	}
	if ref.kind == kGrid {
		g.recompute()
	}
}

type placedSlot struct {
	x, y int
	ref  slotRef
}

// containerSlots lays out the open window's slots, in GUI pixels from its top left corner.
func (g *game) containerSlots() []placedSlot {
	if g.cont.kind == contChest || g.cont.kind == contFurnace {
		return g.beSlots()
	}
	var out []placedSlot
	if g.cont.kind == contTable {
		out = append(out, placedSlot{124, 35, slotRef{&g.cont.result, kResult, 0}})
		for i := 0; i < 9; i++ {
			out = append(out, placedSlot{30 + 18*(i%3), 17 + 18*(i/3), slotRef{&g.cont.grid[i], kGrid, i}})
		}
	} else {
		out = append(out, placedSlot{144, 36, slotRef{&g.cont.result, kResult, 0}})
		for i := 0; i < 4; i++ {
			out = append(out, placedSlot{88 + 18*(i%2), 26 + 18*(i/2), slotRef{&g.cont.grid[i], kGrid, i}})
			out = append(out, placedSlot{8, 8 + 18*i, slotRef{&g.armor[i], kArmor, i}})
		}
	}
	for r := 0; r < 3; r++ {
		for c := 0; c < 9; c++ {
			out = append(out, placedSlot{8 + 18*c, 84 + 18*r, slotRef{&g.inv[9+r*9+c], kInv, 9 + r*9 + c}})
		}
	}
	for c := 0; c < 9; c++ {
		out = append(out, placedSlot{8 + 18*c, 142, slotRef{&g.inv[c], kInv, c}})
	}
	return out
}
