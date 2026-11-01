package client

// The windows the engine knows, each with its own slots, rules and shift-click routes, as 1.8.9's
// ContainerPlayer, ContainerWorkbench, ContainerChest (and ContainerHopper and ContainerDispenser, which route
// the same way) and ContainerFurnace have them.

// craftFrom sets the result of a crafting window from the n slots of its grid starting at from.
func craftFrom(w *clickWin, from, gw, gh int) {
	grid := make([]slot, gw*gh)
	for i := range grid {
		grid[i] = w.slots[from+i].slot
	}
	out, ok := findRecipe(grid, gw, gh)
	if !ok {
		out = slot{}
	}
	w.slots[0] = wstack{slot: out}
}

// useUpGrid is SlotCrafting.onPickupFromSlot: one of each ingredient is gone, and the result is made again.
func useUpGrid(w *clickWin, from, n int) {
	for i := from; i < from+n; i++ {
		if !w.slots[i].empty() {
			if w.slots[i].N--; w.slots[i].N <= 0 {
				w.slots[i] = wstack{}
			}
		}
	}
	w.changed()
}

// playerWin is the player's own window: 0 the crafting result, 1 to 4 its grid, 5 to 8 the armour from the
// helmet down, then the 27 above the hotbar and the hotbar.
func playerWin() *clickWin {
	w := newClickWin(9)
	w.rules[0].role = roleResult
	for k := 0; k < 4; k++ {
		w.rules[5+k] = wrule{role: roleArmor, arg: k, own: true}
	}
	w.recipe = func(w *clickWin) { craftFrom(w, 1, 2, 2) }
	w.taken = func(w *clickWin, _ int) { useUpGrid(w, 1, 4) }
	w.transfer = func(w *clickWin, i int) wstack {
		return w.shiftFrom(i, func(s *wstack) bool {
			switch k := armourKind(s.ID); {
			case i == 0:
				return w.merge(s, 9, 45, true)
			case i < 9:
				return w.merge(s, 9, 45, false)
			case k >= 0 && w.slots[5+k].empty():
				return w.merge(s, 5+k, 6+k, false)
			case i < 36:
				return w.merge(s, 36, 45, false)
			default:
				return w.merge(s, 9, 36, false)
			}
		})
	}
	return w
}

// tableWin is the crafting table: 0 the result, 1 to 9 the grid, then the player's.
func tableWin() *clickWin {
	w := newClickWin(10)
	w.rules[0].role = roleResult
	w.recipe = func(w *clickWin) { craftFrom(w, 1, 3, 3) }
	w.taken = func(w *clickWin, _ int) { useUpGrid(w, 1, 9) }
	w.transfer = func(w *clickWin, i int) wstack {
		return w.shiftFrom(i, func(s *wstack) bool {
			switch {
			case i == 0:
				return w.merge(s, 10, 46, true)
			case i < 10:
				return w.merge(s, 10, 46, false)
			case i < 37:
				return w.merge(s, 37, 46, false)
			default:
				return w.merge(s, 10, 37, false)
			}
		})
	}
	return w
}

// boxWin is a window of n plain slots over the player's: a chest (27 or 54), a hopper (5), a dispenser or
// dropper (9), and what a server calls a container. Shift-click sends a stack across, to the end of the
// player's slots counted from the back, and the player's own to the front of the box.
func boxWin(n int) *clickWin {
	w := newClickWin(n)
	w.transfer = func(w *clickWin, i int) wstack {
		return w.shiftFrom(i, func(s *wstack) bool {
			if i < n {
				return w.merge(s, n, len(w.slots), true)
			}
			return w.merge(s, 0, n, false)
		})
	}
	return w
}

// furnaceWin is the furnace: 0 what is smelted, 1 the fuel, 2 the result, then the player's.
func furnaceWin() *clickWin {
	w := newClickWin(3)
	w.rules[1].role = roleFuel
	w.rules[2].role = roleOutput
	w.transfer = func(w *clickWin, i int) wstack {
		return w.shiftFrom(i, func(s *wstack) bool {
			_, smelts := smeltResult(s.slot)
			switch {
			case i == 2:
				return w.merge(s, 3, 39, true)
			case i < 2:
				return w.merge(s, 3, 39, false)
			case smelts:
				return w.merge(s, 0, 1, false)
			case fuelTime(s.slot) > 0:
				return w.merge(s, 1, 2, false)
			case i < 30:
				return w.merge(s, 30, 39, false)
			default:
				return w.merge(s, 3, 30, false)
			}
		})
	}
	return w
}
