package client

// Where the engine's slots live. A click works on a copy (clickWin), made from the game's own storage when the
// click happens and written back after it: the player's inventory and armour, the crafting grid, a chest or a
// furnace, or what a server keeps in the window it opened. Then, on a server, the click goes out as the game
// sends it; alone, what was thrown out lands in the world.

type winMap struct {
	w       *clickWin
	slot    []*slot      // where each window slot is kept
	extra   []*itemExtra // where its tag is kept, if a server gives tags
	cursor  *slot
	cursorX *itemExtra
	id      int8   // the window's id for a server: 0 is the player's own
	counter *int16 // the window's transaction number, which counts up with each click
	predict bool   // the engine knows this window's rules; if not, the click is sent as it is
	shown   bool   // the click is for a window the server knows of
}

func (m *winMap) load() {
	w := m.w
	for i, p := range m.slot {
		w.slots[i] = wstack{slot: *p}
		if m.extra != nil && m.extra[i] != nil {
			w.slots[i].x = *m.extra[i]
		}
	}
	w.cursor = wstack{slot: *m.cursor}
	if m.cursorX != nil {
		w.cursor.x = *m.cursorX
	}
}

// store writes the engine's slots back. A stack that is gone leaves no tag behind.
func (m *winMap) store() {
	w := m.w
	for i, p := range m.slot {
		s := w.slots[i]
		if s.empty() {
			s = wstack{}
		}
		*p = s.slot
		if m.extra != nil && m.extra[i] != nil {
			*m.extra[i] = s.x
		}
	}
	c := w.cursor
	if c.empty() {
		c = wstack{}
	}
	*m.cursor = c.slot
	if m.cursorX != nil {
		*m.cursorX = c.x
	}
}

// indexOf is the window slot kept at p, or -1.
func (m *winMap) indexOf(p *slot) int {
	for i, q := range m.slot {
		if q == p {
			return i
		}
	}
	return -1
}

// playerSlots points the last 36 slots of a window at the player's inventory: 27 above the hotbar, then the hotbar.
func (g *game) playerSlots(m *winMap, from int) {
	for k := 0; k < 27; k++ {
		m.slot[from+k] = &g.inv[9+k]
	}
	for k := 0; k < 9; k++ {
		m.slot[from+27+k] = &g.inv[k]
	}
}

// localMap is the window open on the game's screen (the player's, a crafting table, a chest or a furnace).
func (g *game) localMap() *winMap {
	m := &winMap{cursor: &g.cursor, predict: true}
	n := 0
	switch g.cont.kind {
	case contPlayer, contNone: // nothing open is the player's own, as containerSlots lays it out
		m.w = playerWin()
		m.slot = make([]*slot, 45)
		m.slot[0] = &g.cont.result
		for i := 0; i < 4; i++ {
			m.slot[1+i], m.slot[5+i] = &g.cont.grid[i], &g.armor[i]
		}
		g.playerSlots(m, 9)
		if g.net != nil { // on a server the player's window is window 0, and its tags are kept by slot
			m.extra = make([]*itemExtra, 45)
			for i := range m.extra {
				m.extra[i] = &g.net.extra[i]
			}
			m.cursorX, m.counter, m.shown = &g.cursorX, &g.net.transaction0, true
		}
		return m
	case contTable:
		m.w, n = tableWin(), 10
		m.slot = make([]*slot, 46)
		m.slot[0] = &g.cont.result
		for i := 0; i < 9; i++ {
			m.slot[1+i] = &g.cont.grid[i]
		}
		g.playerSlots(m, 10)
	case contChest:
		n = g.cont.rows * 9
		m.w = boxWin(n)
		m.slot = make([]*slot, n+36)
		for i := 0; i < n; i++ {
			m.slot[i] = g.cont.beSlot(i)
		}
		g.playerSlots(m, n)
	case contFurnace:
		m.w = furnaceWin()
		m.slot = make([]*slot, 39)
		for i := 0; i < 3; i++ {
			m.slot[i] = &g.cont.be.items[i]
		}
		g.playerSlots(m, 3)
	}
	return m
}

// serverMap is the window a server opened. The ones whose rules the engine knows are predicted; for the others
// (an enchanting table, an anvil, a brewing stand, a villager, a horse) the click goes out as the game would
// make it, and the server says what came of it.
func (g *game) serverMap() *winMap {
	sw := g.swin
	if sw == nil || g.net == nil {
		return nil
	}
	m := &winMap{cursor: &sw.cursor, cursorX: &sw.cursorX, id: sw.id, counter: &sw.action, shown: true}
	m.slot = make([]*slot, sw.n+36)
	m.extra = make([]*itemExtra, sw.n+36)
	for i := 0; i < sw.n; i++ {
		m.slot[i], m.extra[i] = &sw.slots[i], &sw.extra[i]
	}
	for k := 0; k < 36; k++ {
		pi := playerWindowSlot(k)
		if k >= 27 {
			m.slot[sw.n+k] = &g.inv[k-27]
		} else {
			m.slot[sw.n+k] = &g.inv[pi]
		}
		m.extra[sw.n+k] = &g.net.extra[pi]
	}
	switch kind := sw.kind; {
	case kind == "minecraft:furnace" && sw.n == 3:
		m.w, m.predict = furnaceWin(), true
	case kind == "minecraft:crafting_table" && sw.n == 10:
		m.w, m.predict = tableWin(), true
	case sw.n > 0 && (kind == "minecraft:chest" || kind == "minecraft:container" || kind == "minecraft:hopper" ||
		kind == "minecraft:dispenser" || kind == "minecraft:dropper"):
		m.w, m.predict = boxWin(sw.n), true
	default:
		m.w = newClickWin(sw.n)
	}
	return m
}

// windowClick is one click on a window (button and mode as the protocol has them; slot -999 is outside). The
// game's own copy of the window changes at once, as the game's does, and a server is told what was clicked and
// what the player saw there; it does not answer a click it accepts.
func (g *game) windowClick(m *winMap, idx, button, mode int) {
	if m == nil {
		return
	}
	var ret wstack
	if m.predict {
		m.load()
		m.w.creative = g.creative
		ret = m.w.click(idx, button, mode)
		m.store()
		drops := m.w.drops
		m.w.drops = nil
		if g.net == nil { // alone, what a click throws out lands in the world; a server does that itself
			for _, d := range drops {
				g.throwItem(d.slot)
			}
		}
	} else if idx >= 0 && idx < len(m.slot) {
		ret = wstack{slot: *m.slot[idx]}
		if m.extra != nil {
			ret.x = *m.extra[idx]
		}
	}
	if g.net == nil || !m.shown {
		return
	}
	*m.counter++
	p := newPacket(0x0E)
	p.Byte(byte(m.id))
	p.UShort(uint16(int16(idx)))
	p.Byte(byte(button))
	p.UShort(uint16(*m.counter))
	p.Byte(byte(mode))
	putSlotWire(p, ret.slot, ret.x)
	g.net.c.send(p)
}
