package client

import (
	"fmt"
	"testing"
)

// The expected values of these tests are worked out by hand from 1.8.9's Container.slotClick and the
// transferStackInSlot of each window.

func ws(id uint16, n int8) wstack { return wstack{slot: slot{ID: id, N: n}} }

func fillSlot(w *clickWin, i int, id uint16, n int8) { w.slots[i] = ws(id, n) }

// is checks that a slot holds n of id (id 0: nothing).
func is(t *testing.T, w *clickWin, i int, id uint16, n int8) {
	t.Helper()
	if got := w.slots[i]; id == 0 && !got.empty() || id != 0 && (got.ID != id || got.N != n) {
		t.Fatalf("slot %d holds %d x%d, want %d x%d", i, got.ID, got.N, id, n)
	}
}

func cursorIs(t *testing.T, w *clickWin, id uint16, n int8) {
	t.Helper()
	if got := w.cursor; id == 0 && !got.empty() || id != 0 && (got.ID != id || got.N != n) {
		t.Fatalf("cursor holds %d x%d, want %d x%d", got.ID, got.N, id, n)
	}
}

func retIs(t *testing.T, got wstack, id uint16, n int8) {
	t.Helper()
	if id == 0 && !got.empty() || id != 0 && (got.ID != id || got.N != n) {
		t.Fatalf("the packet carries %d x%d, want %d x%d", got.ID, got.N, id, n)
	}
}

const (
	cobble, stick, diamond, planks, ironOre, coal, ironIngot = 4, 280, 264, 5, 15, 263, 265
	ironHelmet, diamondHelmet, bucket                        = 306, 310, 325
)

// Left and right clicks pick up, swap, merge and spread, and the packet carries the slot as it was.
func TestClickPicksUpSwapsMergesAndSplits(t *testing.T) {
	w := playerWin()
	fillSlot(w, 36, stick, 5)
	fillSlot(w, 37, diamond, 3)
	retIs(t, w.click(36, 0, 0), stick, 5)
	cursorIs(t, w, stick, 5)
	is(t, w, 36, 0, 0)
	retIs(t, w.click(37, 0, 0), diamond, 3) // onto diamonds: they trade places
	cursorIs(t, w, diamond, 3)
	is(t, w, 37, stick, 5)
	retIs(t, w.click(38, 0, 0), 0, 0) // an empty slot: put down; no item in the packet
	cursorIs(t, w, 0, 0)
	is(t, w, 38, diamond, 3)

	fillSlot(w, 10, stick, 60)
	fillSlot(w, 11, stick, 10)
	w.click(11, 0, 0)
	w.click(10, 0, 0) // 64 fit: 4 go, 6 stay on the mouse
	is(t, w, 10, stick, 64)
	cursorIs(t, w, stick, 6)

	w = playerWin()
	fillSlot(w, 9, cobble, 5)
	w.click(9, 1, 0) // right: half, rounded up
	cursorIs(t, w, cobble, 3)
	is(t, w, 9, cobble, 2)
	w.click(12, 1, 0) // one into an empty slot
	w.click(12, 1, 0) // and one more onto it
	is(t, w, 12, cobble, 2)
	cursorIs(t, w, cobble, 1)
	fillSlot(w, 13, diamond, 4)
	w.click(13, 1, 0) // a different item: they swap, whatever the button
	cursorIs(t, w, diamond, 4)
	is(t, w, 13, cobble, 1)
}

// A crafting result is taken whole by a left or a right click, uses up the grid, and a click with the same item
// on the mouse adds it. Nothing can be put into it.
func TestCraftingResult(t *testing.T) {
	w := playerWin()
	fillSlot(w, 1, planks, 3)
	fillSlot(w, 3, planks, 3) // two planks above each other make four sticks
	w.changed()
	is(t, w, 0, stick, 4)
	retIs(t, w.click(0, 0, 0), stick, 4)
	cursorIs(t, w, stick, 4)
	is(t, w, 1, planks, 2)
	is(t, w, 3, planks, 2)
	is(t, w, 0, stick, 4) // made again from what is left
	w.click(0, 1, 0)      // right click on a result is the same as left: the whole stack
	cursorIs(t, w, stick, 8)
	is(t, w, 1, planks, 1)
	w.click(0, 0, 0)
	cursorIs(t, w, stick, 12)
	is(t, w, 1, 0, 0)
	is(t, w, 0, 0, 0)

	fillSlot(w, 36, diamond, 1)
	w.click(36, 0, 0) // swap the sticks for a diamond, then try to put it into the result
	w.click(0, 0, 0)
	cursorIs(t, w, diamond, 1)
	is(t, w, 0, 0, 0)

	fillSlot(w, 1, planks, 1)
	fillSlot(w, 3, planks, 1)
	w.changed()
	w.cursor = ws(stick, 62) // 62 + 4 does not fit in a stack of 64: nothing is taken
	w.click(0, 0, 0)
	cursorIs(t, w, stick, 62)
	is(t, w, 1, planks, 1)
}

// Shift-clicking a result crafts again and again until nothing more moves; the packet carries the first stack.
func TestShiftClickCraftsUntilTheGridIsUsedUp(t *testing.T) {
	w := playerWin()
	fillSlot(w, 1, planks, 3)
	fillSlot(w, 3, planks, 3)
	w.changed()
	retIs(t, w.click(0, 0, 1), stick, 4)
	is(t, w, 44, stick, 12) // the hotbar's last slot first: three crafts
	is(t, w, 1, 0, 0)
	is(t, w, 3, 0, 0)
	is(t, w, 0, 0, 0)

	for i := 9; i < 45; i++ { // no room: nothing is crafted
		fillSlot(w, i, diamond, 64)
	}
	fillSlot(w, 1, planks, 1)
	fillSlot(w, 3, planks, 1)
	w.changed()
	retIs(t, w.click(0, 0, 1), 0, 0)
	is(t, w, 1, planks, 1)
}

// Where a shift-click sends a stack from each part of the player's window.
func TestPlayerShiftRoutes(t *testing.T) {
	w := playerWin()
	fillSlot(w, 36, cobble, 10)
	retIs(t, w.click(36, 0, 1), cobble, 10) // hotbar to the main inventory, from its start
	is(t, w, 9, cobble, 10)
	is(t, w, 36, 0, 0)
	w.click(9, 0, 1) // and back: main to the hotbar
	is(t, w, 36, cobble, 10)
	fillSlot(w, 37, cobble, 60)
	w.click(36, 0, 1) // the hotbar again: to the main inventory, whatever else is in the hotbar
	is(t, w, 9, cobble, 10)

	w = playerWin()
	fillSlot(w, 9, ironHelmet, 1)
	fillSlot(w, 10, diamondHelmet, 1)
	w.click(9, 0, 1) // armour goes on
	is(t, w, 5, ironHelmet, 1)
	w.click(10, 0, 1) // the helmet slot is taken: to the hotbar
	is(t, w, 5, ironHelmet, 1)
	is(t, w, 36, diamondHelmet, 1)
	w.click(5, 0, 1) // off the armour slot: into the main inventory
	is(t, w, 5, 0, 0)
	is(t, w, 9, ironHelmet, 1)

	fillSlot(w, 1, stick, 3) // from the crafting grid: into the inventory, main first (9 holds the helmet)
	w.click(1, 0, 1)
	is(t, w, 10, stick, 3)
}

// The armour slots take only their piece; a pumpkin goes on the head; nothing else, however it is clicked.
func TestArmourSlotsTakeTheirPiece(t *testing.T) {
	w := playerWin()
	w.cursor = ws(ironHelmet, 1)
	w.click(6, 0, 0) // a helmet on the chestplate slot
	cursorIs(t, w, ironHelmet, 1)
	is(t, w, 6, 0, 0)
	w.click(5, 0, 0)
	cursorIs(t, w, 0, 0)
	is(t, w, 5, ironHelmet, 1)
	w.cursor = ws(diamondHelmet, 1)
	w.click(5, 0, 0) // swap
	cursorIs(t, w, ironHelmet, 1)
	w.click(5, 0, 0)
	w.cursor = ws(86, 1) // pumpkin
	w.click(5, 0, 0)
	is(t, w, 5, 86, 1)
	w = playerWin()
	w.cursor = ws(diamondHelmet, 5)
	w.click(5, 1, 0) // right click puts one
	is(t, w, 5, diamondHelmet, 1)
	cursorIs(t, w, diamondHelmet, 4)
}

// The number key swaps the slot with that hotbar slot.
func TestNumberKeys(t *testing.T) {
	w := playerWin()
	fillSlot(w, 9, ironIngot, 5)
	w.click(9, 2, 2) // key 3: hotbar slot 2 is 38
	is(t, w, 38, ironIngot, 5)
	is(t, w, 9, 0, 0)
	fillSlot(w, 9, coal, 2)
	w.click(9, 2, 2) // both full: they trade
	is(t, w, 38, coal, 2)
	is(t, w, 9, ironIngot, 5)
	fillSlot(w, 36, ironHelmet, 1)
	w.click(5, 0, 2) // the helmet from key 1 onto the empty helmet slot
	is(t, w, 5, ironHelmet, 1)
	is(t, w, 36, 0, 0)
	fillSlot(w, 36, diamond, 1)
	w.click(5, 0, 2) // a diamond is not a helmet: the helmet comes off into the key's slot and the diamond makes room
	is(t, w, 5, 0, 0)
	is(t, w, 36, ironHelmet, 1)
	is(t, w, 37, diamond, 1)
	fillSlot(w, 39, diamond, 1)
	w.click(6, 0, 2) // key 4 onto the empty chestplate slot, which refuses a diamond
	is(t, w, 6, 0, 0)
	is(t, w, 39, diamond, 1)

	c := boxWin(27)
	fillSlot(c, 0, diamond, 10)
	fillSlot(c, c.hotbar(0), stick, 5)
	c.click(0, 0, 2) // from a chest: the diamonds come to the hotbar, the sticks to the first empty slot
	is(t, c, c.hotbar(0), diamond, 10)
	is(t, c, c.hotbar(1), stick, 5)
	is(t, c, 0, 0, 0)
	c.click(1, 0, 2) // an empty chest slot takes the hotbar's whole stack
	is(t, c, 1, diamond, 10)
	is(t, c, c.hotbar(0), 0, 0)

	for i := 0; i < 36; i++ { // an inventory with no room: the trade is refused
		fillSlot(c, 27+i, planks, 64)
	}
	fillSlot(c, 2, diamond, 3)
	c.click(2, 0, 2)
	is(t, c, 2, diamond, 3)
}

// Q drops one, Ctrl+Q the stack; a click outside the window throws the mouse's stack or one of it.
func TestDroppingStacks(t *testing.T) {
	w := playerWin()
	fillSlot(w, 9, cobble, 5)
	w.click(9, 0, 4)
	is(t, w, 9, cobble, 4)
	w.click(9, 1, 4)
	is(t, w, 9, 0, 0)
	if len(w.drops) != 2 || w.drops[0].N != 1 || w.drops[1].N != 4 {
		t.Fatalf("dropped %+v, want 1 then 4", w.drops)
	}
	w = playerWin()
	w.cursor = ws(cobble, 6)
	w.click(-999, 1, 0)
	cursorIs(t, w, cobble, 5)
	w.click(-999, 0, 0)
	cursorIs(t, w, 0, 0)
	if len(w.drops) != 2 || w.drops[0].N != 1 || w.drops[1].N != 5 {
		t.Fatalf("thrown %+v", w.drops)
	}
	fillSlot(w, 9, cobble, 5)
	w.cursor = ws(stick, 1)
	w.click(9, 0, 4) // with something on the mouse, Q does nothing
	is(t, w, 9, cobble, 5)
	// Q over a crafting result takes the whole of it and uses up the grid
	fillSlot(w, 1, planks, 2)
	fillSlot(w, 3, planks, 2)
	w.cursor = wstack{}
	w.drops = nil
	w.changed()
	w.click(0, 0, 4)
	if len(w.drops) != 1 || w.drops[0].ID != stick || w.drops[0].N != 4 {
		t.Fatalf("thrown %+v, want four sticks", w.drops)
	}
	is(t, w, 1, planks, 1)
}

// The middle button clones a stack in creative, and only there.
func TestMiddleClickClones(t *testing.T) {
	w := playerWin()
	fillSlot(w, 9, diamond, 3)
	w.click(9, 2, 3)
	cursorIs(t, w, 0, 0)
	w.creative = true
	w.click(9, 2, 3)
	cursorIs(t, w, diamond, 64)
	is(t, w, 9, diamond, 3)
}

// Painting: the left button spreads evenly, the right one puts one on each, and only slots that can take it count.
func TestDragSpreadsTheStack(t *testing.T) {
	drag := func(w *clickWin, start, add, end int, slots ...int) {
		w.click(-999, start, 5)
		for _, s := range slots {
			w.click(s, add, 5)
		}
		w.click(-999, end, 5)
	}
	w := playerWin()
	w.cursor = ws(cobble, 7)
	drag(w, 0, 1, 2, 9, 10, 11)
	is(t, w, 9, cobble, 2)
	is(t, w, 11, cobble, 2)
	cursorIs(t, w, cobble, 1)

	w = playerWin()
	w.cursor = ws(cobble, 7)
	drag(w, 4, 5, 6, 9, 10, 11)
	is(t, w, 10, cobble, 1)
	cursorIs(t, w, cobble, 4)

	w = playerWin()
	fillSlot(w, 9, cobble, 5)
	fillSlot(w, 10, diamond, 1) // another item: not part of it
	w.cursor = ws(cobble, 7)
	drag(w, 0, 1, 2, 9, 10, 11)
	is(t, w, 9, cobble, 8)
	is(t, w, 10, diamond, 1)
	is(t, w, 11, cobble, 3)
	cursorIs(t, w, cobble, 1)

	w = playerWin()
	w.cursor = ws(cobble, 2)
	drag(w, 0, 1, 2, 9, 10, 11, 12) // only as many slots as there are items
	is(t, w, 9, cobble, 1)
	is(t, w, 10, cobble, 1)
	is(t, w, 11, 0, 0)
	cursorIs(t, w, 0, 0)

	w = playerWin()
	w.cursor = ws(cobble, 7)
	drag(w, 0, 1, 2, 9, 9, 9) // the same slot again is one slot
	is(t, w, 9, cobble, 7)
	cursorIs(t, w, 0, 0)

	w = playerWin()
	w.cursor = ws(cobble, 5)
	drag(w, 8, 9, 10, 9, 10) // the middle button fills, but only in creative
	is(t, w, 9, 0, 0)
	w.creative = true
	drag(w, 8, 9, 10, 9, 10)
	is(t, w, 9, cobble, 64)
	cursorIs(t, w, 0, 0)

	w = playerWin()
	w.cursor = ws(cobble, 7)
	w.click(-999, 0, 5)
	w.click(9, 1, 5)
	w.click(10, 0, 0) // an ordinary click in the middle of a drag cancels it
	w.click(-999, 2, 5)
	is(t, w, 9, 0, 0)

	w = playerWin()
	w.cursor = ws(cobble, 7)
	w.click(0, 0, 5)
	w.click(0, 1, 5)
	w.click(0, 2, 5) // a drag over the crafting result puts nothing there
	is(t, w, 0, 0, 0)
}

// A double click gathers the stacks of the item, those that are not full first, and leaves a result alone.
func TestDoubleClickGathers(t *testing.T) {
	w := playerWin()
	fillSlot(w, 9, cobble, 20)
	fillSlot(w, 10, cobble, 64)
	fillSlot(w, 11, cobble, 40)
	fillSlot(w, 12, stick, 5)
	w.cursor = ws(cobble, 10)
	w.click(20, 0, 6)
	cursorIs(t, w, cobble, 64)
	is(t, w, 9, 0, 0)
	is(t, w, 10, cobble, 64)
	is(t, w, 11, cobble, 6)
	is(t, w, 12, stick, 5)

	w = playerWin()
	fillSlot(w, 1, planks, 2)
	fillSlot(w, 3, planks, 2)
	w.changed()
	fillSlot(w, 9, stick, 3)
	w.cursor = ws(stick, 1)
	w.click(20, 0, 6)
	cursorIs(t, w, stick, 4) // not the four of the result
	is(t, w, 0, stick, 4)

	w = playerWin()
	fillSlot(w, 9, cobble, 64)
	fillSlot(w, 10, cobble, 5)
	w.cursor = ws(cobble, 60) // the full stack is left alone: only four of the five are needed
	w.click(20, 0, 6)
	cursorIs(t, w, cobble, 64)
	is(t, w, 10, cobble, 1)
	is(t, w, 9, cobble, 64)
}

// Stacks of the same item with different tags are different stacks: they do not merge.
func TestTagsKeepStacksApart(t *testing.T) {
	w := boxWin(27)
	tagged := ws(diamond, 3)
	tagged.x.nbt = []byte{10, 0, 0, 0}
	w.slots[0] = ws(diamond, 3)
	w.cursor = tagged
	w.click(0, 0, 0) // not the same: they swap, nothing merges
	if w.slots[0].N != 3 || w.cursor.N != 3 || w.slots[0].x.nbt == nil || w.cursor.x.nbt != nil {
		t.Fatalf("slot %+v cursor %+v", w.slots[0], w.cursor)
	}
	w.slots[0], w.cursor = ws(diamond, 3), wstack{}
	w.slots[w.hotbar(0)] = tagged
	w.click(0, 0, 1) // shift: the plain stack does not join the tagged one in the hotbar
	if got := w.slots[w.hotbar(0)]; got.N != 3 || got.x.nbt == nil {
		t.Fatalf("the tagged stack became %+v", got)
	}
	is(t, w, 62, diamond, 3)
}

// The chest, hopper and dispenser route the same way: out to the back of the player's slots, in to the front.
func TestBoxShiftRoutes(t *testing.T) {
	w := boxWin(27)
	fillSlot(w, 0, cobble, 10)
	fillSlot(w, 62, cobble, 60)
	retIs(t, w.click(0, 0, 1), cobble, 10)
	is(t, w, 62, cobble, 64)
	is(t, w, 61, cobble, 6) // the four that did not fit stayed behind and then went to the next empty slot
	is(t, w, 0, 0, 0)

	fillSlot(w, 40, diamond, 2) // from the player's part: into the chest
	w.click(40, 0, 1)
	is(t, w, 0, diamond, 2)
	fillSlot(w, 41, diamond, 3)
	w.click(41, 0, 1)
	is(t, w, 0, diamond, 5)

	h := boxWin(5)
	fillSlot(h, 0, ironIngot, 4)
	h.click(0, 0, 1)
	is(t, h, 40, ironIngot, 4)

	for i := 0; i < 63; i++ { // no room anywhere: nothing moves
		fillSlot(w, i, diamond, 64)
	}
	retIs(t, w.click(0, 0, 1), 0, 0)
}

// The furnace: what smelts goes to the top slot, fuel to the middle, the rest across the player's parts, and the
// result slot is taken from.
func TestFurnace(t *testing.T) {
	w := furnaceWin()
	fillSlot(w, 3, ironOre, 8)
	fillSlot(w, 4, coal, 8)
	fillSlot(w, 5, cobble, 8)
	w.click(3, 0, 1)
	is(t, w, 0, ironOre, 8)
	w.click(4, 0, 1)
	is(t, w, 1, coal, 8)
	w.click(5, 0, 1) // cobblestone smelts, but the top slot is taken: it does not move
	is(t, w, 5, cobble, 8)
	fillSlot(w, 6, diamond, 8)
	w.click(6, 0, 1) // the rest of the main part goes to the hotbar
	is(t, w, 30, diamond, 8)
	w.click(30, 0, 1) // and the hotbar to the main part, from its start
	is(t, w, 3, diamond, 8)
	fillSlot(w, 2, ironIngot, 8)
	w.click(2, 0, 1) // the result goes out, to the back
	is(t, w, 38, ironIngot, 8)

	w = furnaceWin()
	w.cursor = ws(ironIngot, 4)
	w.click(1, 0, 0) // not fuel
	cursorIs(t, w, ironIngot, 4)
	w.cursor = ws(bucket, 3)
	w.click(1, 0, 0) // a bucket is fuel's slot's one exception, and only one
	is(t, w, 1, bucket, 1)
	cursorIs(t, w, bucket, 2)
	fillSlot(w, 2, ironIngot, 5)
	w.cursor = ws(diamond, 1)
	w.click(2, 0, 0) // the result slot takes nothing in
	cursorIs(t, w, diamond, 1)
	w.cursor = wstack{}
	w.click(2, 1, 0) // a right click takes half of a furnace's result (unlike a crafting result, which comes whole)
	cursorIs(t, w, ironIngot, 3)
	is(t, w, 2, ironIngot, 2)
	w.click(2, 0, 0) // and a click with the same item on the mouse takes the rest
	cursorIs(t, w, ironIngot, 5)
	is(t, w, 2, 0, 0)
}

// Every route is safe over a window of whatever size.
func TestClicksNeverPanic(t *testing.T) {
	for _, mk := range []func() *clickWin{playerWin, tableWin, furnaceWin, func() *clickWin { return boxWin(27) }, func() *clickWin { return boxWin(5) }} {
		w := mk()
		for i := range w.slots {
			fillSlot(w, i, uint16(1+i%6*40), int8(1+i%64))
		}
		for mode := -1; mode < 8; mode++ {
			for button := -1; button < 12; button++ {
				for _, i := range []int{-1000, -999, -1, 0, 1, 2, len(w.slots) - 1, len(w.slots)} {
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Fatalf("mode %d button %d slot %d: %v", mode, button, i, r)
							}
						}()
						w.creative = button%2 == 0
						w.click(i, button, mode)
						if w.cursor.N < 0 {
							t.Fatalf("a negative stack on the mouse after %s", fmt.Sprint(mode, button, i))
						}
					}()
				}
			}
		}
	}
}
