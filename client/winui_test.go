package client

import (
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// uiGame is a player's window open on a screen of 1280 x 720, a clock the test turns and no server.
func uiGame(t *testing.T) (*game, *time.Time) {
	g := testGame(t)
	g.opts = defaultOptions()
	g.fb = [2]int{1280, 720}
	g.creative = false
	g.openContainer(contPlayer)
	clock := time.Unix(1000, 0)
	g.wui.now = func() time.Time { return clock }
	return g, &clock
}

// over moves the mouse to the middle of a window slot (-1: just outside the panel).
func over(t *testing.T, g *game, idx int) {
	t.Helper()
	if idx < 0 {
		g.cx, g.cy = 2, 2
		return
	}
	switch g.screen {
	case screenServerWin:
		slots, ph := g.swin.layout()
		s, l, tp := g.windowPanel(ph)
		g.cx, g.cy = float64(l+(float32(slots[idx].x)+8)*s), float64(tp+(float32(slots[idx].y)+8)*s)
	default:
		m := g.localMap()
		s, l, tp := g.contPanel()
		for _, ps := range g.containerSlots() {
			if m.indexOf(ps.ref.p) == idx {
				g.cx, g.cy = float64(l+(float32(ps.x)+8)*s), float64(tp+(float32(ps.y)+8)*s)
				g.winMove()
				return
			}
		}
		t.Fatalf("no slot %d", idx)
	}
	g.winMove()
}

func press(g *game, b glfw.MouseButton)   { g.winMouse(b, true) }
func release(g *game, b glfw.MouseButton) { g.winMouse(b, false) }

// With nothing on the mouse a click is made when the button goes down, and letting go does nothing; with a stack
// on it nothing happens until the button is let go.
func TestClickGoesOnPressWithAnEmptyMouseAndOnReleaseWithAStack(t *testing.T) {
	g, _ := uiGame(t)
	g.inv[9] = slot{ID: cobble, N: 8}
	over(t, g, 9)
	press(g, glfw.MouseButtonLeft)
	if g.cursor != (slot{ID: cobble, N: 8}) {
		t.Fatalf("on press: cursor %v", g.cursor)
	}
	release(g, glfw.MouseButtonLeft)
	if g.cursor.N != 8 || !g.inv[9].empty() {
		t.Fatalf("on release: cursor %v slot %v: the release must not click again", g.cursor, g.inv[9])
	}
	over(t, g, 12)
	press(g, glfw.MouseButtonLeft)
	if g.cursor.N != 8 || !g.inv[12].empty() {
		t.Fatalf("with a stack on the mouse the press must wait: cursor %v slot %v", g.cursor, g.inv[12])
	}
	release(g, glfw.MouseButtonLeft)
	if !g.cursor.empty() || g.inv[12] != (slot{ID: cobble, N: 8}) {
		t.Fatalf("after the release: cursor %v slot %v", g.cursor, g.inv[12])
	}
}

// Holding a button with a stack on the mouse and moving over slots paints it across them; the packets a
// server gets are the start, one per slot and the end, and the preview shows the shares before the release.
func TestPaintingAStackAcrossSlots(t *testing.T) {
	g, c := netPlayerGame(t)
	g.opts = defaultOptions()
	g.fb = [2]int{1280, 720}
	g.openContainer(contPlayer)
	drain(t, c)
	g.cursor = slot{ID: cobble, N: 7}
	over(t, g, 9)
	press(g, glfw.MouseButtonLeft)
	for _, i := range []int{9, 10, 11, 10} { // the same slot twice counts once
		over(t, g, i)
	}
	counts, left, ok := g.dragPreview(g.localMap())
	if !ok || len(counts) != 3 || counts[9] != 2 || counts[11] != 2 || left != 1 {
		t.Fatalf("preview %v, %d left, %v", counts, left, ok)
	}
	if g.inv[9].N != 0 || g.cursor.N != 7 {
		t.Fatal("nothing may change before the release")
	}
	release(g, glfw.MouseButtonLeft)
	if g.inv[9].N != 2 || g.inv[10].N != 2 || g.inv[11].N != 2 || g.cursor.N != 1 {
		t.Fatalf("after: %v %v %v cursor %v", g.inv[9], g.inv[10], g.inv[11], g.cursor)
	}
	ks := readClicks(t, c)
	if len(ks) != 5 {
		t.Fatalf("%d packets: %+v", len(ks), ks)
	}
	want := []struct {
		slot   int16
		button byte
	}{{-999, 0}, {9, 1}, {10, 1}, {11, 1}, {-999, 2}}
	for i, w := range want {
		if ks[i].slot != w.slot || ks[i].button != w.button || ks[i].mode != 5 || ks[i].hasItem {
			t.Fatalf("packet %d is %+v, want slot %d button %d mode 5 and no item", i, ks[i], w.slot, w.button)
		}
	}

	g.cursor = slot{ID: cobble, N: 4}
	over(t, g, 20)
	press(g, glfw.MouseButtonRight) // the right button: one on each
	over(t, g, 20)                  // a slot joins when the mouse moves over it, not when the button goes down
	over(t, g, 21)
	over(t, g, 22)
	release(g, glfw.MouseButtonRight)
	if g.inv[20].N != 1 || g.inv[22].N != 1 || g.cursor.N != 1 {
		t.Fatalf("right: %v %v cursor %v", g.inv[20], g.inv[22], g.cursor)
	}
	ks = readClicks(t, c)
	if ks[0].button != 4 || ks[1].button != 5 || ks[len(ks)-1].button != 6 {
		t.Fatalf("right drag codes %d %d %d, want 4 5 6", ks[0].button, ks[1].button, ks[len(ks)-1].button)
	}

	g.cursor = slot{ID: cobble, N: 4} // letting go of the other button cancels it
	over(t, g, 30)
	press(g, glfw.MouseButtonLeft)
	over(t, g, 31)
	release(g, glfw.MouseButtonRight)
	if g.inv[30].N != 0 || g.cursor.N != 4 || len(readClicks(t, c)) != 0 {
		t.Fatal("a drag ended by the other button must change and send nothing")
	}

	g.cursor = slot{ID: cobble, N: 4}
	g.inv[40-36] = slot{ID: cobble, N: 1}
	over(t, g, 32)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft) // pressed and let go over one slot, not moved: an ordinary click
	if g.inv[32].N != 4 || !g.cursor.empty() {
		t.Fatalf("an unmoved click: slot %v cursor %v", g.inv[32], g.cursor)
	}
}

// Two clicks on one slot within a quarter of a second gather the item on the mouse; slower ones are two clicks.
func TestDoubleClickGathersOnTheMouse(t *testing.T) {
	g, clock := uiGame(t)
	g.inv[9] = slot{ID: cobble, N: 20}
	g.inv[10] = slot{ID: cobble, N: 30}
	over(t, g, 9)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	*clock = clock.Add(100 * time.Millisecond)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	if g.cursor != (slot{ID: cobble, N: 50}) || !g.inv[10].empty() {
		t.Fatalf("double click: cursor %v, next slot %v", g.cursor, g.inv[10])
	}

	g, clock = uiGame(t)
	g.inv[9] = slot{ID: cobble, N: 20}
	g.inv[10] = slot{ID: cobble, N: 30}
	over(t, g, 9)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	*clock = clock.Add(300 * time.Millisecond)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	if !g.cursor.empty() || g.inv[9] != (slot{ID: cobble, N: 20}) || g.inv[10].N != 30 {
		t.Fatalf("two slow clicks: cursor %v slots %v %v", g.cursor, g.inv[9], g.inv[10])
	}
}

// Shift-click moves the stack across; a click outside throws what the mouse holds; the middle button sends a
// clone click, which only creative honours.
func TestShiftOutsideAndMiddleClicks(t *testing.T) {
	g, _ := uiGame(t)
	g.inv[9] = slot{ID: cobble, N: 8}
	g.wui.shift = true
	over(t, g, 9)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	if !g.inv[9].empty() || g.inv[0].N != 8 {
		t.Fatalf("shift-click: main %v hotbar %v", g.inv[9], g.inv[0])
	}
	g.wui.shift = false

	g.cursor = slot{ID: diamond, N: 3}
	over(t, g, -1)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	if !g.cursor.empty() || len(g.items) != 1 || g.items[0].stack.N != 3 {
		t.Fatalf("outside: cursor %v, %d thrown", g.cursor, len(g.items))
	}

	g.inv[12] = slot{ID: diamond, N: 2}
	over(t, g, 12)
	press(g, glfw.MouseButtonMiddle)
	release(g, glfw.MouseButtonMiddle)
	if !g.cursor.empty() {
		t.Fatal("the middle button cloned outside creative")
	}
	g.creative = true
	press(g, glfw.MouseButtonMiddle)
	release(g, glfw.MouseButtonMiddle)
	if g.cursor != (slot{ID: diamond, N: 64}) || g.inv[12].N != 2 {
		t.Fatalf("clone: cursor %v slot %v", g.cursor, g.inv[12])
	}
}

// Over a slot, a number key swaps it with that hotbar slot (only with nothing on the mouse) and Q drops one,
// Ctrl+Q all of it.
func TestNumberKeysAndDropOverASlot(t *testing.T) {
	g, _ := uiGame(t)
	g.inv[9] = slot{ID: ironIngot, N: 5}
	over(t, g, 9)
	if !g.winKey(glfw.Key3) || g.inv[2] != (slot{ID: ironIngot, N: 5}) || !g.inv[9].empty() {
		t.Fatalf("key 3: hotbar %v main %v", g.inv[2], g.inv[9])
	}
	g.cursor = slot{ID: stick, N: 1}
	over(t, g, 11)
	g.inv[11] = slot{ID: coal, N: 2}
	g.winKey(glfw.Key1)
	if g.inv[11].ID != coal || !g.inv[0].empty() {
		t.Fatalf("a key with a stack on the mouse swapped: %v %v", g.inv[11], g.inv[0])
	}
	g.cursor = slot{}
	g.inv[11] = slot{ID: cobble, N: 5}
	over(t, g, 11)
	if !g.winKey(glfw.KeyQ) || g.inv[11].N != 4 || len(g.items) != 1 {
		t.Fatalf("Q: slot %v, %d thrown", g.inv[11], len(g.items))
	}
	g.wui.ctrl = true
	g.winKey(glfw.KeyQ)
	if !g.inv[11].empty() || len(g.items) != 2 || g.items[1].stack.N != 4 {
		t.Fatalf("Ctrl+Q: slot %v, %d thrown", g.inv[11], len(g.items))
	}
	over(t, g, 12) // an empty slot has nothing to drop
	if g.winKey(glfw.KeyQ) {
		t.Fatal("Q over an empty slot did something")
	}
}

// The same on a server's window: the clicks are predicted and sent, a double click gathers through mode 6.
func TestServerWindowMouse(t *testing.T) {
	g, c := netPlayerGame(t)
	g.opts = defaultOptions()
	g.fb = [2]int{1280, 720}
	clock := time.Unix(1000, 0)
	openChest(t, g, 3, `{"text":"Kit"}`, 27)
	g.wui.now = func() time.Time { return clock }
	g.swin.slots[0] = slot{ID: cobble, N: 20}
	g.swin.slots[1] = slot{ID: cobble, N: 30}
	drain(t, c)
	over(t, g, 0)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	clock = clock.Add(120 * time.Millisecond)
	press(g, glfw.MouseButtonLeft)
	release(g, glfw.MouseButtonLeft)
	if g.swin.cursor != (slot{ID: cobble, N: 50}) || !g.swin.slots[1].empty() {
		t.Fatalf("cursor %v, slot 1 %v", g.swin.cursor, g.swin.slots[1])
	}
	ks := readClicks(t, c)
	if len(ks) != 2 || ks[0].mode != 0 || ks[1].mode != 6 || ks[1].slot != 0 {
		t.Fatalf("sent %+v, want a click and then mode 6 at slot 0", ks)
	}
	over(t, g, 27+3) // a number key over the player's part of the window
	g.winKey(glfw.Key2)
	if k := readClicks(t, c); len(k) != 0 {
		t.Fatalf("a number key with the mouse holding a stack sent %+v", k)
	}
}
