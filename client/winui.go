package client

import (
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// What the mouse and keys do in a window with slots, after 1.8.9's GuiContainer: with nothing on the mouse a
// click is sent when the button goes down; with a stack on it, nothing is sent until the button is let go,
// because holding it and moving over slots paints the stack across them; two clicks on one slot within a
// quarter of a second gather the item; the number keys swap the slot under the mouse with a hotbar slot, Q
// drops from it and the pick key clones it. The same code serves the player's window, a crafting table, a
// chest, a furnace and a window a server opened.

const doubleClickTime = 250 * time.Millisecond

type winUI struct {
	lastSlot   int // the slot of the last click, -1 for none
	lastAt     time.Time
	lastButton int
	double     bool // the click that just went down is the second of a double click
	ignoreUp   bool // the click was sent when the button went down: letting go sends nothing
	dragging   bool // a stack is being painted across slots
	dragBtn    int
	dragLimit  int   // 0 the left button (evenly), 1 the right (one each), 2 the middle (fill)
	dragSlots  []int // the slots painted, in the order they were reached
	shifted    wstack
	now        func() time.Time // for tests
	shift      bool             // tests: Shift is held when there is no window to ask
	ctrl       bool
}

func (u *winUI) time() time.Time {
	if u.now != nil {
		return u.now()
	}
	return time.Now()
}

// winHit is what the mouse points at in the open window: its slots, the slot under it (-1 for none) and
// whether it is outside the panel.
func (g *game) winHit() (m *winMap, idx int, outside bool) {
	mx, my := g.mouse()
	idx = -1
	switch g.screen {
	case screenContainer:
		if m = g.localMap(); m == nil {
			return
		}
		if ps, ok := g.slotUnderMouse(); ok {
			idx = m.indexOf(ps.ref.p)
		}
		s, left, top := g.contPanel()
		x, y := (mx-left)/s, (my-top)/s
		outside = x < 0 || y < 0 || x >= contW || y >= float32(g.contHeight())
	case screenServerWin:
		if m = g.serverMap(); m == nil {
			return
		}
		slots, ph := g.swin.layout()
		s, left, top := g.windowPanel(ph)
		if sl, ok := g.windowSlotAt(mx, my, left, top, s, slots); ok {
			idx = sl.idx
		}
		x, y := (mx-left)/s, (my-top)/s
		outside = x < 0 || y < 0 || x >= contW || y >= float32(ph)
	}
	return
}

func (g *game) inWindow() bool { return g.screen == screenContainer || g.screen == screenServerWin }

func (g *game) shiftHeld() bool {
	return g.wui.shift || g.win != nil && (g.win.GetKey(glfw.KeyLeftShift) == glfw.Press || g.win.GetKey(glfw.KeyRightShift) == glfw.Press)
}

func (g *game) ctrlHeld() bool {
	return g.wui.ctrl || g.win != nil && (g.win.GetKey(glfw.KeyLeftControl) == glfw.Press || g.win.GetKey(glfw.KeyRightControl) == glfw.Press)
}

// sameInventory is whether two slots belong to the same Inventory in the game: the player's own (main, hotbar
// and armour) are one, a window's own slots another, and a crafting result stands alone.
func (m *winMap) sameInventory(i, j int) bool {
	a, b := m.w.rules[i], m.w.rules[j]
	if a.role == roleResult || b.role == roleResult {
		return i == j
	}
	return a.own == b.own
}

// stackAt is the stack in slot i as the window shows it, with its tag.
func (m *winMap) stackAt(i int) wstack {
	s := wstack{slot: *m.slot[i]}
	if m.extra != nil && m.extra[i] != nil {
		s.x = *m.extra[i]
	}
	return s
}

func dragCode(event, limit int) int { return (limit&3)<<2 | event&3 }

// winMouse is a mouse button going down or up over a window.
func (g *game) winMouse(b glfw.MouseButton, down bool) {
	if b != glfw.MouseButtonLeft && b != glfw.MouseButtonRight && b != glfw.MouseButtonMiddle {
		return
	}
	if down && b == glfw.MouseButtonLeft && g.screen == screenServerWin && g.swin != nil { // the buttons of an enchanting table
		_, ph := g.swin.layout()
		s, left, top := g.windowPanel(ph)
		mx, my := g.mouse()
		if g.windowButton((mx-left)/s, (my-top)/s) {
			return
		}
	}
	m, idx, outside := g.winHit()
	if m == nil {
		return
	}
	if down {
		g.winDown(m, idx, outside, int(b))
	} else {
		g.winUp(m, idx, outside, int(b))
	}
}

// winDown is GuiContainer.mouseClicked.
func (g *game) winDown(m *winMap, idx int, outside bool, b int) {
	u := &g.wui
	now := u.time()
	u.double = idx == u.lastSlot && now.Sub(u.lastAt) < doubleClickTime && u.lastButton == b
	u.ignoreUp = false
	l := idx
	if outside {
		l = -999
	}
	pick := b == int(glfw.MouseButtonMiddle)
	if l != -1 && !u.dragging {
		if m.cursor.empty() {
			if pick {
				g.windowClick(m, l, b, 3)
			} else {
				mode := 0
				if l != -999 && g.shiftHeld() {
					u.shifted = wstack{}
					if l >= 0 && !m.slot[l].empty() {
						u.shifted = m.stackAt(l)
					}
					mode = 1
				} else if l == -999 {
					mode = 4
				}
				g.windowClick(m, l, b, mode)
			}
			u.ignoreUp = true
		} else {
			u.dragging, u.dragBtn, u.dragSlots = true, b, u.dragSlots[:0]
			u.dragLimit = map[int]int{0: 0, 1: 1}[b]
			if pick {
				u.dragLimit = 2
			}
		}
	}
	u.lastSlot, u.lastAt, u.lastButton = idx, now, b
}

// winUp is GuiContainer.mouseReleased.
func (g *game) winUp(m *winMap, idx int, outside bool, state int) {
	u := &g.wui
	k := idx
	if outside {
		k = -999
	}
	if u.double && idx >= 0 && state == 0 && m.w.canMerge(idx) {
		if g.shiftHeld() {
			if !u.shifted.empty() {
				for i := range m.slot {
					if s := m.stackAt(i); !s.empty() && m.sameInventory(i, idx) && m.w.canAdd(i, u.shifted, true) {
						g.windowClick(m, i, state, 1)
					}
				}
			}
		} else {
			g.windowClick(m, idx, state, 6)
		}
		u.double, u.lastAt = false, time.Time{}
		return
	}
	if u.dragging && u.dragBtn != state {
		u.dragging, u.dragSlots, u.ignoreUp = false, u.dragSlots[:0], true
		return
	}
	if u.ignoreUp {
		u.ignoreUp = false
		return
	}
	if u.dragging && len(u.dragSlots) > 0 {
		g.windowClick(m, -999, dragCode(0, u.dragLimit), 5)
		for _, i := range u.dragSlots {
			g.windowClick(m, i, dragCode(1, u.dragLimit), 5)
		}
		g.windowClick(m, -999, dragCode(2, u.dragLimit), 5)
	} else if !m.cursor.empty() {
		if state == int(glfw.MouseButtonMiddle) {
			g.windowClick(m, k, state, 3)
		} else {
			shift := k != -999 && g.shiftHeld()
			if shift {
				u.shifted = wstack{}
				if k >= 0 && !m.slot[k].empty() {
					u.shifted = m.stackAt(k)
				}
			}
			mode := 0
			if shift {
				mode = 1
			}
			g.windowClick(m, k, state, mode)
		}
	}
	if m.cursor.empty() {
		u.lastAt = time.Time{}
	}
	u.dragging = false
}

// winMove is the mouse moving over a window with a button down (GuiContainer.mouseClickMove): while painting, a
// slot that can take the stack joins the ones painted.
func (g *game) winMove() {
	u := &g.wui
	if !u.dragging {
		return
	}
	m, idx, _ := g.winHit()
	if m == nil || idx < 0 || m.cursor.empty() {
		return
	}
	m.load()
	cur := m.w.cursor
	if int(cur.N) > len(u.dragSlots) && m.w.canAdd(idx, cur, true) && m.w.valid(idx, cur) {
		for _, d := range u.dragSlots {
			if d == idx {
				return
			}
		}
		u.dragSlots = append(u.dragSlots, idx)
	}
}

// winKey is a key pressed over a window: a number key swaps the slot under the mouse with that hotbar slot
// (only with nothing on the mouse), the pick key clones it and the drop key drops one (with Ctrl, all).
func (g *game) winKey(key glfw.Key) bool {
	m, idx, _ := g.winHit()
	if m == nil || idx < 0 {
		return false
	}
	if key >= glfw.Key1 && key <= glfw.Key9 {
		if m.cursor.empty() {
			g.windowClick(m, idx, int(key-glfw.Key1), 2)
		}
		return true
	}
	if m.slot[idx].empty() {
		return false
	}
	switch code, ok := lwjglOf[key]; {
	case ok && code == g.opts.Keys["pickItem"]:
		g.windowClick(m, idx, 0, 3)
		return true
	case ok && code == g.opts.Keys["drop"]:
		button := 0
		if g.ctrlHeld() {
			button = 1
		}
		g.windowClick(m, idx, button, 4)
		return true
	}
	return false
}

// dragPreview is what the painting in progress would give each slot, by window slot, and what would stay on the
// mouse: the counts the game draws over the slots while the button is held.
func (g *game) dragPreview(m *winMap) (counts map[int]int, left int, ok bool) {
	u := &g.wui
	if m == nil || !u.dragging || len(u.dragSlots) < 2 || m.cursor.empty() {
		return nil, 0, false
	}
	m.load()
	cs, left := m.w.dragShares(u.dragSlots, u.dragLimit)
	counts = make(map[int]int, len(cs))
	for k, i := range u.dragSlots {
		if cs[k] >= 0 {
			counts[i] = cs[k]
		}
	}
	return counts, left, true
}
