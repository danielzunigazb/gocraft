package client

import (
	"gocraft/proto"
)

// Windows a server opens: the menus of a lobby (a game selector, a profile), chests, hoppers. The server
// owns what they hold: gocraft shows it, sends the clicks as they are made and shows what the server
// says happened. Items carry names and lore (their NBT), which the menus are made of.

// itemExtra is what an item says besides what it is: its name, its lore, and its tag as the server sent
// it, which goes back unchanged in a click so the server finds the stack it expects.
type itemExtra struct {
	name      []styledRun
	lore      [][]styledRun
	nbt       []byte
	colour    int32 // display.color: what leather armour is dyed
	hasColour bool
}

type serverWindow struct {
	id      int8
	kind    string
	title   []styledRun
	n       int // slots of the window itself, before the player's 36
	slots   []slot
	extra   []itemExtra
	cursor  slot
	cursorX itemExtra
	action  int16     // the transaction number of the last click, which a server that refuses it answers with
	win     *clickWin // the click engine of the window, kept while it is open: a drag lives across several clicks
}

// readSlotX reads an item stack with its tag: the display name and lore are kept, the rest is skipped.
func readSlotX(r *proto.Reader) (slot, itemExtra, bool) {
	var ex itemExtra
	id := r.Short()
	if r.Err() != nil {
		return slot{}, ex, false
	}
	if id < 0 {
		return slot{}, ex, true
	}
	n, dmg := r.Byte(), r.Short()
	rest := r.Rest()
	if tag := r.Byte(); tag != 0 { // not TAG_End: a compound follows, named
		r.Bytes(int(r.UShort()))
		if tag != 10 || !readCompound(r, &ex, 0) {
			return slot{}, ex, false
		}
		if used := len(rest) - len(r.Rest()); used > 0 && used <= len(rest) {
			ex.nbt = append([]byte(nil), rest[:used]...)
		}
	}
	return slot{ID: uint16(id), Dmg: uint16(dmg), N: int8(n)}, ex, r.Err() == nil
}

// readCompound reads the entries of an item's tag; "display" is looked into, the rest is skipped.
func readCompound(r *proto.Reader, ex *itemExtra, depth int) bool {
	for r.Err() == nil {
		t := r.Byte()
		if t == 0 {
			return true
		}
		name := string(r.Bytes(int(r.UShort())))
		if depth == 0 && name == "display" && t == 10 {
			if !readDisplay(r, ex) {
				return false
			}
			continue
		}
		if !skipPayload(r, t, depth+1) {
			return false
		}
	}
	return false
}

func readDisplay(r *proto.Reader, ex *itemExtra) bool {
	for r.Err() == nil {
		t := r.Byte()
		if t == 0 {
			return true
		}
		name := string(r.Bytes(int(r.UShort())))
		switch {
		case name == "Name" && t == 8:
			ex.name = legacyRuns(string(r.Bytes(int(r.UShort()))), defaultStyle)
		case name == "color" && t == 3:
			ex.colour, ex.hasColour = r.Int(), true
		case name == "Lore" && t == 9:
			et, n := r.Byte(), int(r.Int())
			if et != 8 || n < 0 || n > 64 {
				return false
			}
			for i := 0; i < n && r.Err() == nil; i++ {
				ex.lore = append(ex.lore, legacyRuns(string(r.Bytes(int(r.UShort()))), textStyle{col: chatColours[5], italic: true}))
			}
		default:
			if !skipPayload(r, t, 2) {
				return false
			}
		}
	}
	return false
}

// ---- state ----

// playerWindowSlot is where the player's own inventory sits in a window with n slots of its own: the
// main inventory first, then the hotbar. It returns the index in the player's inventory window (9 to 44).
func playerWindowSlot(k int) int {
	if k < 27 {
		return 9 + k
	}
	return 36 + k - 27
}

func (g *game) openServerWindow(id int8, kind, titleJSON string, n int) {
	if g.screen == screenContainer {
		g.closeContainer()
	}
	title := parseChatBase(titleJSON, textStyle{col: [3]float32{0.25, 0.25, 0.25}})
	g.wui = winUI{}
	g.swin = &serverWindow{id: id, kind: kind, title: title, n: n, slots: make([]slot, n), extra: make([]itemExtra, n)}
	g.openScreen(screenServerWin)
	g.cx, g.cy = float64(g.fb[0])/2, float64(g.fb[1])/2
}

// closeServerWindow closes the window; send says the server is told (when the player closes it).
func (g *game) closeServerWindow(send bool) {
	w := g.swin
	g.swin = nil
	if w == nil {
		return
	}
	if send && g.net != nil {
		p := newPacket(0x0D)
		p.Byte(byte(w.id))
		g.net.c.send(p)
	}
	if g.screen == screenServerWin {
		g.closeScreen()
	}
}

// netSetSlot is a slot the server filled: in the player's own window, the open window, or the mouse (-1).
func (g *game) netSetSlot(win int8, idx int, it slot, ex itemExtra) {
	n := g.net
	switch {
	case win == 0:
		g.setWindowSlot(idx, it)
		if idx >= 1 && idx <= 4 && g.cont.kind == contPlayer { // the 2x2 grid, while the window is open
			g.cont.grid[idx-1] = it
			g.recompute()
		}
		if idx >= 0 && idx < len(n.extra) {
			n.extra[idx] = ex
		}
	case win == -1 && idx == -1: // what the mouse holds (InventoryPlayer.setItemStack), whichever window is open
		if w := g.swin; w != nil {
			w.cursor, w.cursorX = it, ex
		} else {
			g.cursor, g.cursorX = it, ex
		}
	case g.swin != nil && g.swin.id == win:
		w := g.swin
		switch {
		case idx >= 0 && idx < w.n:
			w.slots[idx], w.extra[idx] = it, ex
		case idx >= w.n && idx < w.n+36:
			pi := playerWindowSlot(idx - w.n)
			g.setWindowSlot(pi, it)
			n.extra[pi] = ex
		}
	}
}

// itemAt is the stack in a window slot (0 to n+35).
func (g *game) windowItem(i int) (slot, itemExtra) {
	w := g.swin
	if i < w.n {
		return w.slots[i], w.extra[i]
	}
	pi := playerWindowSlot(i - w.n)
	var s slot
	if pi >= 36 {
		s = g.inv[pi-36]
	} else {
		s = g.inv[pi]
	}
	return s, g.net.extra[pi]
}

// ---- layout ----

type winSlot struct{ x, y, idx int }

// winLayout gives the slots of the window in GUI pixels from the panel's corner, and the panel's height.
func (w *serverWindow) layout() (slots []winSlot, h int) {
	add := func(x, y int) { slots = append(slots, winSlot{x, y, len(slots)}) }
	player := func(invY, hotY int) {
		for r := 0; r < 3; r++ {
			for c := 0; c < 9; c++ {
				add(8+18*c, invY+18*r)
			}
		}
		for c := 0; c < 9; c++ {
			add(8+18*c, hotY)
		}
	}
	switch {
	case w.kind == "minecraft:hopper" && w.n == 5:
		for i := 0; i < 5; i++ {
			add(44+18*i, 20)
		}
		player(51, 109)
		return slots, 133
	case (w.kind == "minecraft:dispenser" || w.kind == "minecraft:dropper") && w.n == 9:
		for i := 0; i < 9; i++ {
			add(62+18*(i%3), 17+18*(i/3))
		}
		player(84, 142)
		return slots, 166
	}
	rows := max((w.n+8)/9, 1)
	for i := 0; i < w.n; i++ {
		add(8+18*(i%9), 18+18*(i/9))
	}
	player(103+(rows-4)*18, 161+(rows-4)*18)
	return slots, 114 + rows*18
}

func (g *game) windowPanel(h int) (s, left, top float32) {
	s = float32(g.scaleFor(g.fb[0], g.fb[1]))
	return s, (float32(g.fb[0]) - contW*s) / 2, (float32(g.fb[1]) - float32(h)*s) / 2
}

// ---- drawing ----

func (g *game) drawServerWindow(w, h int) {
	win := g.swin
	if win == nil {
		return
	}
	slots, ph := win.layout()
	hd := &g.hud
	s, left, top := g.windowPanel(ph)
	one := [4]float32{1, 1, 1, 1}
	g.enableUI()
	hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0.06, 0.06, 0.6})
	hd.flush(hd.white)
	invLabel := 0
	switch {
	case win.kind == "minecraft:hopper" && win.n == 5:
		hd.rect(left, top, contW*s, 133*s, 0, 0, contW, 133, 256, 256, one)
		hd.flush(g.guiTexture("hopper"))
		invLabel = 39
	case (win.kind == "minecraft:dispenser" || win.kind == "minecraft:dropper") && win.n == 9:
		hd.rect(left, top, contW*s, 166*s, 0, 0, contW, 166, 256, 256, one)
		hd.flush(g.guiTexture("dispenser"))
		invLabel = 72
	default:
		rows := float32(max((win.n+8)/9, 1))
		hd.rect(left, top, contW*s, (rows*18+17)*s, 0, 0, contW, rows*18+17, 256, 256, one)
		hd.rect(left, top+(rows*18+17)*s, contW*s, 96*s, 0, 126, contW, 96, 256, 256, one)
		hd.flush(g.guiTexture("generic_54"))
		invLabel = int(rows*18) + 17 + 3
	}
	g.drawRuns(win.title, left+8*s, top+6*s, s, 1, false)
	g.text("Inventory", left+8*s, top+float32(invLabel)*s, s, [4]float32{0.25, 0.25, 0.25, 1}, false)
	prev, remnant, painting := g.dragPreview(g.serverMap()) // a stack painted across slots shows what each would hold
	shown := func(i int) slot {
		if c, ok := prev[i]; painting && ok {
			return slot{win.cursor.ID, win.cursor.Dmg, int8(c)}
		}
		it, _ := g.windowItem(i)
		return it
	}
	for _, sl := range slots {
		g.drawSlotIcon(shown(sl.idx), left+float32(sl.x)*s, top+float32(sl.y)*s, 16*s)
	}
	hd.flush(g.tex)
	hd.flushItems()
	for _, sl := range slots {
		g.drawCount(int(shown(sl.idx).N), left+float32(sl.x)*s, top+float32(sl.y)*s, s)
		if _, ok := prev[sl.idx]; painting && ok {
			hd.rect(left+float32(sl.x)*s, top+float32(sl.y)*s, 16*s, 16*s, 0, 0, 1, 1, 1, 1, [4]float32{1, 1, 1, 0.5})
			hd.flush(hd.white)
		}
	}
	mx, my := g.mouse()
	if sl, ok := g.windowSlotAt(mx, my, left, top, s, slots); ok {
		hd.rect(left+float32(sl.x)*s, top+float32(sl.y)*s, 16*s, 16*s, 0, 0, 1, 1, 1, 1, [4]float32{1, 1, 1, 0.4})
		hd.flush(hd.white)
		if it, ex := g.windowItem(sl.idx); win.cursor.empty() && !it.empty() {
			g.drawItemTooltip(it, ex, mx, my, s)
		}
	}
	if !win.cursor.empty() { // the stack on the mouse
		g.drawSlotIcon(win.cursor, mx-8*s, my-8*s, 16*s)
		hd.flush(g.tex)
		hd.flushItems()
		n := int(win.cursor.N)
		if painting {
			n = remnant
		}
		g.drawCount(n, mx-8*s, my-8*s, s)
	}
}

func (g *game) windowSlotAt(mx, my, left, top, s float32, slots []winSlot) (winSlot, bool) {
	x, y := (mx-left)/s, (my-top)/s
	for _, sl := range slots {
		if x >= float32(sl.x) && x < float32(sl.x+16) && y >= float32(sl.y) && y < float32(sl.y+16) {
			return sl, true
		}
	}
	return winSlot{}, false
}

// drawItemTooltip draws an item's name and lore by the mouse, with the colours the server gave them.
func (g *game) drawItemTooltip(it slot, ex itemExtra, mx, my, s float32) {
	var lines [][]styledRun
	if len(ex.name) > 0 {
		lines = append(lines, ex.name)
	} else {
		lines = append(lines, []styledRun{{g.displayName(it), defaultStyle}})
	}
	lines = append(lines, ex.lore...)
	wd := 0
	for _, l := range lines {
		wd = max(wd, g.runsWidth(l))
	}
	hd := &g.hud
	tw, th := float32(wd)*s, float32(len(lines)*10)*s
	x, y := mx+12*s, my-12*s
	if x+tw+8*s > float32(g.fb[0]) {
		x = mx - 16*s - tw
	}
	y = min(max(y, 2*s), float32(g.fb[1])-th-4*s)
	hd.rect(x-4*s, y-3*s, tw+8*s, th+4*s, 0, 0, 1, 1, 1, 1, [4]float32{0.31, 0, 1, 0.9})
	hd.rect(x-3*s, y-2*s, tw+6*s, th+2*s, 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0, 0.06, 0.95})
	hd.flush(hd.white)
	for i, l := range lines {
		g.drawRuns(l, x, y+float32(i*10)*s, s, 1, true)
	}
}

// ---- clicks ----

func (g *game) sendWindowClick(idx int, button, mode byte) {
	g.windowClick(g.serverMap(), idx, int(button), int(mode))
}

// putSlotWire writes an item stack as the protocol has it, with the tag the server sent.
func putSlotWire(p *proto.Writer, s slot, ex itemExtra) {
	if s.empty() {
		p.UShort(0xFFFF)
		return
	}
	p.UShort(s.ID)
	p.Byte(byte(s.N))
	p.UShort(s.Dmg)
	if len(ex.nbt) > 0 {
		p.Bytes(ex.nbt)
	} else {
		p.Byte(0)
	}
}
