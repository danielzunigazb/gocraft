package client

import "testing"

func inv(g *game, i int) slotRef { return slotRef{&g.inv[i], kInv, i} }

func TestLeftClickPicksUpPutsDownSwapsAndMerges(t *testing.T) {
	g := testGame(t)
	g.inv[0], g.inv[1], g.inv[2] = slot{ID: 280, N: 10}, slot{ID: 264, N: 3}, slot{ID: 280, N: 60}
	g.clickSlot(inv(g, 0), false, false) // pick up the sticks
	if g.cursor != (slot{ID: 280, N: 10}) || !g.inv[0].empty() {
		t.Fatalf("pick up: cursor %v slot %v", g.cursor, g.inv[0])
	}
	g.clickSlot(inv(g, 1), false, false) // onto diamonds: swap
	if g.cursor != (slot{ID: 264, N: 3}) || g.inv[1] != (slot{ID: 280, N: 10}) {
		t.Fatalf("swap: cursor %v slot %v", g.cursor, g.inv[1])
	}
	g.clickSlot(inv(g, 3), false, false) // an empty slot: put down
	if !g.cursor.empty() || g.inv[3] != (slot{ID: 264, N: 3}) {
		t.Fatalf("put down: cursor %v slot %v", g.cursor, g.inv[3])
	}
	g.clickSlot(inv(g, 1), false, false)
	g.clickSlot(inv(g, 2), false, false) // sticks onto 60 sticks: 64 fit, 6 stay on the mouse
	if g.inv[2].N != 64 || g.cursor != (slot{ID: 280, N: 6}) {
		t.Fatalf("merge: slot %v cursor %v", g.inv[2], g.cursor)
	}
}

func TestRightClickSplitsAndPlacesOne(t *testing.T) {
	g := testGame(t)
	g.inv[0] = slot{ID: 280, N: 9}
	g.clickSlot(inv(g, 0), true, false) // takes half, rounded up
	if g.cursor.N != 5 || g.inv[0].N != 4 {
		t.Fatalf("split 9: cursor %v slot %v", g.cursor, g.inv[0])
	}
	g.clickSlot(inv(g, 5), true, false) // one into an empty slot
	g.clickSlot(inv(g, 5), true, false) // and one more onto it
	if g.inv[5].N != 2 || g.cursor.N != 3 {
		t.Fatalf("one at a time: slot %v cursor %v", g.inv[5], g.cursor)
	}
	g.cursor, g.inv[6] = slot{ID: 264, N: 2}, slot{ID: 280, N: 4}
	g.clickSlot(inv(g, 6), true, false) // a different item: they swap
	if g.cursor != (slot{ID: 280, N: 4}) || g.inv[6] != (slot{ID: 264, N: 2}) {
		t.Fatalf("swap on right click: %v %v", g.cursor, g.inv[6])
	}
}

func TestShiftClickMovesBetweenHotbarAndInventory(t *testing.T) {
	g := testGame(t)
	g.inv[2] = slot{ID: 280, N: 8}
	g.clickSlot(inv(g, 2), false, true) // hotbar -> main inventory
	if !g.inv[2].empty() || g.inv[9] != (slot{ID: 280, N: 8}) {
		t.Fatalf("hotbar to main: %v %v", g.inv[2], g.inv[9])
	}
	g.clickSlot(inv(g, 9), false, true) // and back
	if g.inv[0] != (slot{ID: 280, N: 8}) {
		t.Fatalf("main to hotbar: %v", g.inv[:3])
	}
	g.inv[20] = slot{ID: 310, N: 1} // a diamond helmet goes on
	g.clickSlot(inv(g, 20), false, true)
	if g.armor[0].ID != 310 || !g.inv[20].empty() {
		t.Fatalf("armour: %v %v", g.armor, g.inv[20])
	}
}

func TestArmourSlotsOnlyTakeTheirPiece(t *testing.T) {
	g := testGame(t)
	g.cursor = slot{ID: 313, N: 1} // diamond boots
	g.clickSlot(slotRef{&g.armor[0], kArmor, 0}, false, false)
	if !g.armor[0].empty() || g.cursor.ID != 313 {
		t.Fatal("boots went on the head")
	}
	g.clickSlot(slotRef{&g.armor[3], kArmor, 3}, false, false)
	if g.armor[3].ID != 313 || !g.cursor.empty() {
		t.Fatalf("boots: %v cursor %v", g.armor, g.cursor)
	}
	g.cursor = slot{ID: 280, N: 1}
	g.clickSlot(slotRef{&g.armor[1], kArmor, 1}, false, false)
	if !g.armor[1].empty() {
		t.Fatal("a stick went in the chestplate slot")
	}
}

func TestCraftingThroughTheWindow(t *testing.T) {
	g := testGame(t)
	g.openContainerForTest(contPlayer)
	g.inv[0] = slot{ID: 17, N: 3} // three logs
	g.clickSlot(inv(g, 0), false, false)
	g.clickSlot(slotRef{&g.cont.grid[0], kGrid, 0}, true, false) // one log into the grid
	if g.cont.result != (slot{ID: 5, N: 4}) {
		t.Fatalf("result %v", g.cont.result)
	}
	// With logs still on the mouse the planks cannot be taken: the cursor must be empty or match.
	g.clickSlot(slotRef{&g.cont.result, kResult, 0}, false, false)
	if g.cursor.ID != 17 || g.cont.result.empty() {
		t.Fatalf("took the result while holding something else: cursor %v", g.cursor)
	}
	g.clickSlot(inv(g, 0), false, false)                           // put the two spare logs back
	g.clickSlot(slotRef{&g.cont.result, kResult, 0}, false, false) // now take the planks
	if g.cursor != (slot{ID: 5, N: 4}) || !g.cont.grid[0].empty() || !g.cont.result.empty() {
		t.Fatalf("after taking: cursor %v grid %v result %v", g.cursor, g.cont.grid[0], g.cont.result)
	}
	// A second craft stacks onto the planks the mouse holds.
	g.clickSlot(inv(g, 0), false, false) // pick the logs up again (swapping the planks into the slot)
	g.clickSlot(slotRef{&g.cont.grid[0], kGrid, 0}, true, false)
	g.clickSlot(inv(g, 0), false, false) // planks back on the mouse, spare logs in the slot
	g.clickSlot(slotRef{&g.cont.result, kResult, 0}, false, false)
	if g.cursor != (slot{ID: 5, N: 8}) {
		t.Fatalf("stacking crafts: cursor %v", g.cursor)
	}
}

func TestShiftClickCraftsAsManyAsFit(t *testing.T) {
	g := testGame(t)
	g.openContainerForTest(contTable)
	g.cont.grid[0] = slot{ID: 17, N: 5}
	g.recompute()
	g.clickSlot(slotRef{&g.cont.result, kResult, 0}, false, true)
	total := 0
	for _, s := range g.inv {
		if s.ID == 5 {
			total += int(s.N)
		}
	}
	if total != 20 || !g.cont.grid[0].empty() {
		t.Fatalf("five logs make %d planks (want 20), %v left in the grid", total, g.cont.grid[0])
	}
}

func TestShiftClickCraftStopsWhenTheInventoryIsFull(t *testing.T) {
	g := testGame(t)
	g.openContainerForTest(contTable)
	for i := range g.inv {
		g.inv[i] = slot{ID: 4, N: 64}
	}
	g.cont.grid[0] = slot{ID: 17, N: 5}
	g.recompute()
	g.clickSlot(slotRef{&g.cont.result, kResult, 0}, false, true)
	if g.cont.grid[0].N != 5 {
		t.Fatalf("crafted into a full inventory: %v left", g.cont.grid[0])
	}
}

func TestClosingReturnsTheGrid(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.openContainerForTest(contTable)
	g.cont.grid[4] = slot{ID: 4, N: 7}
	g.cursor = slot{ID: 264, N: 2}
	g.closeContainerForTest()
	got := 0
	for _, s := range g.inv {
		if s.ID == 4 || s.ID == 264 {
			got += int(s.N)
		}
	}
	if got != 9 || !g.cursor.empty() {
		t.Fatalf("%d items came back, cursor %v", got, g.cursor)
	}
}

// openContainerForTest and closeContainerForTest do the window's bookkeeping without a screen.
func (g *game) openContainerForTest(k containerKind) { g.cont = containerState{kind: k} }
func (g *game) closeContainerForTest() {
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
}
