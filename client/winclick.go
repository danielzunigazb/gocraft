package client

import "bytes"

// The click engine of every window with slots, after 1.8.9's Container.slotClick: the same six modes (click,
// shift-click, number key, middle click, drop, drag, double click), the same rules for what a slot takes and
// the same routes for shift-click in each window. It works on a copy of the window's slots, so the player's
// own inventory, a chest, a furnace and a window of a server all go through one piece of code, and what it
// returns is what the game puts in the Click Window packet: the stack the click was about, as the game saw it
// (a server answers a click whose stack differs from its own with a rejection and a fresh copy of the window).
//
// Slots are numbered as the protocol numbers them: a window's own slots first, then the player's 27 of the
// main inventory, then the 9 of the hotbar, which are the last 9 of any window.

// wstack is an item stack with the tag a server gave it, which goes back unchanged in a click and is part of
// what makes two stacks the same (ItemStack.areItemStackTagsEqual).
type wstack struct {
	slot
	x itemExtra
}

func (a wstack) sameItem(b wstack) bool { return a.slot.same(b.slot) && bytes.Equal(a.x.nbt, b.x.nbt) }

// wmax is how many of a stack fit in one slot of a window: the item's own limit.
func (a wstack) wmax() int { return int(a.slot.maxStack()) }

// with is the stack with n of it, or nothing when n is not above 0.
func (a wstack) with(n int) wstack {
	if n <= 0 {
		return wstack{}
	}
	a.N = int8(n)
	return a
}

type slotRole uint8

const (
	roleNormal slotRole = iota
	roleResult          // a crafting result: taken from, never put into; taking it uses up what made it
	roleOutput          // a furnace's result: taken from, never put into
	roleArmor           // one piece of armour (or a pumpkin or a head on the helmet slot)
	roleFuel            // a furnace's fuel: what burns, and an empty bucket
)

// wrule is what a slot accepts.
type wrule struct {
	role slotRole
	arg  int  // armour: 0 helmet, 1 chestplate, 2 leggings, 3 boots
	own  bool // the slot belongs to the player's inventory (main, hotbar, armour): a number key swaps freely there
}

type clickWin struct {
	slots    []wstack
	rules    []wrule
	cursor   wstack
	creative bool

	// What differs between windows.
	transfer func(w *clickWin, i int) wstack // transferStackInSlot
	recipe   func(w *clickWin)               // onCraftMatrixChanged: sets the result from the grid
	taken    func(w *clickWin, i int)        // onPickupFromSlot of a result: uses up the grid

	// A drag in progress (Container.dragEvent, dragMode and dragSlots).
	dragEvent, dragMode int
	dragSlots           []int

	drops []wstack // what the click threw out into the world
}

func newClickWin(own int) *clickWin {
	n := own + 36
	w := &clickWin{slots: make([]wstack, n), rules: make([]wrule, n)}
	for i := own; i < n; i++ {
		w.rules[i].own = true
	}
	return w
}

// hotbar is the window slot of hotbar slot b (0 to 8).
func (w *clickWin) hotbar(b int) int { return len(w.slots) - 9 + b }

// invSlot is the window slot of inventory slot k as InventoryPlayer numbers them: the hotbar first (0 to 8),
// then the 27 above it.
func (w *clickWin) invSlot(k int) int {
	if k < 9 {
		return w.hotbar(k)
	}
	return len(w.slots) - 36 + k - 9
}

func armourOf(s slot) int {
	if k := armourKind(s.ID); k >= 0 {
		return k
	}
	if s.ID == 86 || s.ID == 397 { // a pumpkin and a head go on the head
		return 0
	}
	return -1
}

// valid is Slot.isItemValid.
func (w *clickWin) valid(i int, s wstack) bool {
	if s.empty() {
		return false
	}
	switch r := w.rules[i]; r.role {
	case roleResult, roleOutput:
		return false
	case roleArmor:
		return armourOf(s.slot) == r.arg
	case roleFuel:
		return fuelTime(s.slot) > 0 || s.ID == 325
	}
	return true
}

// limit is Slot.getItemStackLimit: how many of a stack the slot holds.
func (w *clickWin) limit(i int, s wstack) int {
	switch w.rules[i].role {
	case roleArmor:
		return 1
	case roleFuel:
		if s.ID == 325 {
			return 1
		}
	}
	return 64
}

// decr is Slot.decrStackSize: n of what is in the slot come out (all of a result, whatever n is).
func (w *clickWin) decr(i, n int) wstack {
	s := w.slots[i]
	if s.empty() {
		return wstack{}
	}
	if w.rules[i].role == roleResult || n >= int(s.N) {
		w.slots[i] = wstack{}
		return s
	}
	w.slots[i].N -= int8(n)
	return s.with(n)
}

// pickedUp is Slot.onPickupFromSlot: taking a crafting result uses up the grid.
func (w *clickWin) pickedUp(i int) {
	if w.rules[i].role == roleResult && w.taken != nil {
		w.taken(w, i)
	}
}

func (w *clickWin) changed() {
	if w.recipe != nil {
		w.recipe(w)
	}
}

// canAdd is Container.canAddItemToSlot: the slot is empty, or holds the same and (if the size matters) has room.
func (w *clickWin) canAdd(i int, s wstack, sizeMatters bool) bool {
	in := w.slots[i]
	if in.empty() {
		return true
	}
	if !s.empty() && s.sameItem(in) {
		extra := 0
		if !sizeMatters {
			extra = int(s.N)
		}
		return int(in.N)+extra <= s.wmax()
	}
	return false
}

// canMerge is Container.canMergeSlot: a double click does not take from a crafting result.
func (w *clickWin) canMerge(i int) bool { return w.rules[i].role != roleResult }

// merge is Container.mergeItemStack: the stack goes first onto the stacks of the same item in from..to-1 that
// have room, then whole into the first empty slot. It does not look at what a slot accepts, as in 1.8.9: the
// ranges of each window's shift-click are what keep things where they belong.
func (w *clickWin) merge(s *wstack, from, to int, reverse bool) bool {
	moved := false
	step, i := 1, from
	if reverse {
		step, i = -1, to-1
	}
	in := func(k int) bool { return k >= from && k < to }
	if s.wmax() > 1 {
		for ; s.N > 0 && in(i); i += step {
			t := &w.slots[i]
			if t.empty() || !t.sameItem(*s) {
				continue
			}
			mx := s.wmax()
			sum := int(t.N) + int(s.N)
			switch {
			case sum <= mx:
				s.N, t.N, moved = 0, int8(sum), true
			case int(t.N) < mx:
				s.N -= int8(mx - int(t.N))
				t.N, moved = int8(mx), true
			}
		}
	}
	if s.N > 0 {
		i = from
		if reverse {
			i = to - 1
		}
		for ; in(i); i += step {
			if w.slots[i].empty() {
				w.slots[i], s.N, moved = *s, 0, true
				break
			}
		}
	}
	return moved
}

// shiftFrom is the shape of every window's transferStackInSlot: route moves the live stack of slot i (and
// says whether anything moved); what is left stays, and a result that moved is taken.
func (w *clickWin) shiftFrom(i int, route func(s *wstack) bool) wstack {
	if i < 0 || i >= len(w.slots) || w.slots[i].empty() {
		return wstack{}
	}
	s := w.slots[i]
	orig := s
	if !route(&s) {
		return wstack{}
	}
	if s.N <= 0 {
		w.slots[i] = wstack{}
	} else {
		w.slots[i] = s
	}
	if s.N == orig.N {
		return wstack{}
	}
	w.pickedUp(i)
	return orig
}

// addToInventory is InventoryPlayer.addItemStackToInventory over the window's player slots: onto stacks of the
// same item with room, hotbar first, then into the first empty slot. It returns what did not fit.
func (w *clickWin) addToInventory(s wstack) wstack {
	for pass := 0; pass < 2 && !s.empty(); pass++ {
		for k := 0; k < 36 && !s.empty(); k++ {
			t := &w.slots[w.invSlot(k)]
			switch {
			case pass == 0 && !t.empty() && t.sameItem(s) && int(t.N) < s.wmax():
				n := min(int(s.N), s.wmax()-int(t.N))
				t.N += int8(n)
				s = s.with(int(s.N) - n)
			case pass == 1 && t.empty():
				n := min(int(s.N), s.wmax())
				*t = s.with(n)
				s = s.with(int(s.N) - n)
			}
		}
	}
	return s
}

// firstEmpty is InventoryPlayer.getFirstEmptyStack: the inventory slot (hotbar first) with nothing in it, or -1.
func (w *clickWin) firstEmpty() int {
	for k := 0; k < 36; k++ {
		if w.slots[w.invSlot(k)].empty() {
			return k
		}
	}
	return -1
}

func (w *clickWin) drop(s wstack) {
	if !s.empty() {
		w.drops = append(w.drops, s)
	}
}

func (w *clickWin) resetDrag() { w.dragEvent, w.dragSlots = 0, w.dragSlots[:0] }

// click is Container.slotClick. slotID -999 is outside the window; button and mode are the packet's. It
// returns the stack the packet carries (the empty one is "no item").
func (w *clickWin) click(slotID, button, mode int) (ret wstack) {
	defer w.changed()
	n := len(w.slots)
	inRange := slotID >= 0 && slotID < n
	switch {
	case mode == 5:
		w.dragClick(slotID, button)
	case w.dragEvent != 0:
		w.resetDrag()
	case (mode == 0 || mode == 1) && (button == 0 || button == 1):
		switch {
		case slotID == -999:
			if !w.cursor.empty() {
				if button == 0 {
					w.drop(w.cursor)
					w.cursor = wstack{}
				} else {
					w.drop(w.cursor.with(1))
					w.cursor = w.cursor.with(int(w.cursor.N) - 1)
				}
			}
		case !inRange:
		case mode == 1:
			for tries := 0; tries < 4096; tries++ {
				moved := w.transfer(w, slotID)
				if moved.empty() {
					break
				}
				if ret.empty() {
					ret = moved
				}
				// retrySlotClick: go on while the slot still holds the same item
				if s := w.slots[slotID]; s.empty() || s.ID != moved.ID {
					break
				}
			}
		default:
			ret = w.plainClick(slotID, button)
		}
	case mode == 2 && button >= 0 && button < 9 && inRange:
		w.numberKey(slotID, button)
	case mode == 3 && w.creative && w.cursor.empty() && inRange:
		if s := w.slots[slotID]; !s.empty() {
			w.cursor = s.with(s.wmax())
		}
	case mode == 4 && w.cursor.empty() && inRange:
		if s := w.slots[slotID]; !s.empty() {
			cnt := 1
			if button != 0 {
				cnt = int(s.N)
			}
			out := w.decr(slotID, cnt)
			w.pickedUp(slotID)
			w.drop(out)
		}
	case mode == 6 && inRange:
		w.doubleClick(slotID, button)
	}
	return ret
}

// plainClick is a left or right click on a slot (mode 0).
func (w *clickWin) plainClick(i, button int) (ret wstack) {
	in, cur := w.slots[i], w.cursor
	if !in.empty() {
		ret = in
	}
	switch {
	case in.empty():
		if !cur.empty() && w.valid(i, cur) {
			l := 1
			if button == 0 {
				l = int(cur.N)
			}
			l = min(l, w.limit(i, cur))
			if int(cur.N) >= l {
				w.slots[i] = cur.with(l)
				w.cursor = cur.with(int(cur.N) - l)
			}
		}
	case cur.empty():
		k := int(in.N)
		if button == 1 {
			k = (int(in.N) + 1) / 2
		}
		w.cursor = w.decr(i, k)
		w.pickedUp(i)
	case w.valid(i, cur):
		if in.sameItem(cur) {
			l := 1
			if button == 0 {
				l = int(cur.N)
			}
			l = min(l, w.limit(i, cur)-int(in.N), cur.wmax()-int(in.N))
			if l > 0 {
				w.slots[i].N += int8(l)
				w.cursor = cur.with(int(cur.N) - l)
			}
		} else if int(cur.N) <= w.limit(i, cur) {
			w.slots[i], w.cursor = cur, in
		}
	case in.sameItem(cur) && cur.wmax() > 1: // what the slot does not take, the mouse takes from it: a crafting result
		if k := int(in.N); k > 0 && k+int(cur.N) <= cur.wmax() {
			w.cursor.N += int8(k)
			w.decr(i, k)
			w.pickedUp(i)
		}
	}
	return ret
}

// numberKey is the number key over a slot (mode 2): the slot and that hotbar slot trade places.
func (w *clickWin) numberKey(i, b int) {
	hb := w.hotbar(b)
	in, h := w.slots[i], w.slots[hb]
	if i == hb {
		return
	}
	ok := h.empty() || w.rules[i].own && w.valid(i, h)
	empty := -1
	if !ok {
		if empty = w.firstEmpty(); empty >= 0 {
			ok = true
		}
	}
	switch {
	case !in.empty() && ok:
		w.slots[hb] = in
		if (!w.rules[i].own || !w.valid(i, h)) && !h.empty() {
			if empty >= 0 {
				w.addToInventory(h)
				w.decr(i, int(in.N))
				w.slots[i] = wstack{}
				w.pickedUp(i)
			}
		} else {
			w.decr(i, int(in.N))
			w.slots[i] = h
			w.pickedUp(i)
		}
	case in.empty() && !h.empty() && w.valid(i, h):
		w.slots[hb] = wstack{}
		w.slots[i] = h
	}
}

// doubleClick is mode 6: with the mouse holding something, it collects every stack of the same item it can,
// those not yet full last.
func (w *clickWin) doubleClick(i, button int) {
	cur := w.cursor
	if cur.empty() || !w.slots[i].empty() {
		return
	}
	n := len(w.slots)
	start, step := 0, 1
	if button != 0 {
		start, step = n-1, -1
	}
	for pass := 0; pass < 2; pass++ {
		for k := start; k >= 0 && k < n && int(w.cursor.N) < w.cursor.wmax(); k += step {
			t := w.slots[k]
			if t.empty() || !w.canAdd(k, w.cursor, true) || !w.canMerge(k) || !w.cursor.sameItem(t) {
				continue
			}
			if pass == 0 && int(t.N) == t.wmax() {
				continue
			}
			take := min(w.cursor.wmax()-int(w.cursor.N), int(t.N))
			w.decr(k, take)
			w.cursor.N += int8(take)
			w.pickedUp(k)
		}
	}
}

// dragShares is what painting a stack over slots gives them, in the way mode says (0 evenly, 1 one each, 2 fill
// to the limit): how many each slot ends up holding (-1 for a slot that takes nothing of it) and how many stay
// on the mouse. It is the arithmetic of Container.computeStackSize, for the drop and for the preview.
func (w *clickWin) dragShares(slots []int, mode int) (counts []int, left int) {
	cur := w.cursor
	left = int(cur.N)
	counts = make([]int, len(slots))
	for k, i := range slots {
		counts[k] = -1
		if !w.canAdd(i, cur, true) || !w.valid(i, cur) || int(cur.N) < len(slots) {
			continue
		}
		have := 0
		if !w.slots[i].empty() {
			have = int(w.slots[i].N)
		}
		give := 1
		switch mode {
		case 0:
			give = int(cur.N) / len(slots)
		case 2:
			give = cur.wmax()
		}
		give = min(give+have, cur.wmax(), w.limit(i, cur))
		left -= give - have
		counts[k] = give
	}
	return counts, left
}

// dragClick is the paint-to-spread click (mode 5): button says the step (start, a slot, end) in its low two
// bits and the way (left spreads evenly, right one each, middle fills) above them.
func (w *clickWin) dragClick(slotID, button int) {
	prev := w.dragEvent
	w.dragEvent = button & 3
	switch {
	case (prev != 1 || w.dragEvent != 2) && prev != w.dragEvent:
		w.resetDrag()
	case w.cursor.empty():
		w.resetDrag()
	case w.dragEvent == 0:
		w.dragMode = button >> 2 & 3
		if w.dragMode == 0 || w.dragMode == 1 || w.dragMode == 2 && w.creative {
			w.dragEvent, w.dragSlots = 1, w.dragSlots[:0]
		} else {
			w.resetDrag()
		}
	case w.dragEvent == 1:
		if slotID >= 0 && slotID < len(w.slots) && w.canAdd(slotID, w.cursor, true) && w.valid(slotID, w.cursor) && int(w.cursor.N) > len(w.dragSlots) {
			for _, d := range w.dragSlots {
				if d == slotID {
					return
				}
			}
			w.dragSlots = append(w.dragSlots, slotID)
		}
	case w.dragEvent == 2:
		if len(w.dragSlots) > 0 {
			cur := w.cursor
			counts, left := w.dragShares(w.dragSlots, w.dragMode)
			for k, i := range w.dragSlots {
				if counts[k] >= 0 {
					w.slots[i] = cur.with(counts[k])
				}
			}
			w.cursor = cur.with(left)
		}
		w.resetDrag()
	default:
		w.resetDrag()
	}
}
