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
	// The click engine of the window, kept while it is open: a drag lives across several clicks.
	win     *clickWin
	winKind containerKind
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
	g.cont, g.wui = containerState{kind: k}, winUI{}
	g.openScreen(screenContainer)
	if k == contPlayer && g.net != nil { // Minecraft.runTick: "open inventory" goes to the server before the screen opens
		p := newPacket(0x16)
		p.VarInt(2)
		g.net.c.send(p)
	}
}

// closeContainer hands back what was left in the grid and on the mouse; what does not fit is thrown. On a
// server the player's window is closed with a Close Window and what it held is the server's to throw
// (Container.onContainerClosed): the game only forgets it.
func (g *game) closeContainer() {
	if g.net != nil {
		if g.cont.kind == contPlayer {
			p := newPacket(0x0D)
			p.Byte(0)
			g.net.c.send(p)
		}
		g.cont, g.cursor, g.cursorX = containerState{}, slot{}, itemExtra{}
		g.closeScreen()
		return
	}
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

// clickSlot is one mouse click on a slot of the open window: left or right, with shift or not. The click engine
// does the rest (winclick.go), and tells a server.
func (g *game) clickSlot(ref slotRef, right, shift bool) {
	m := g.localMap()
	if m == nil {
		return
	}
	idx := m.indexOf(ref.p)
	if idx < 0 {
		return
	}
	button, mode := 0, 0
	if right {
		button = 1
	}
	if shift {
		mode = 1
	}
	g.windowClick(m, idx, button, mode)
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
