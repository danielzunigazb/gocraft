package client

// The player's inventory: 36 slots (the hotbar is the first nine), four armour slots and the stack
// the mouse holds. Containers (the player's 2x2 grid, a crafting table) are in container.go.

// giveSlot puts a stack into the inventory, topping up stacks of the same item first and then
// filling empty slots, hotbar before the rest; it returns what did not fit.
func (g *game) giveSlot(s slot) slot {
	for i := range g.inv {
		in := &g.inv[i]
		if !in.empty() && in.same(s) && in.N < in.maxStack() && !s.empty() {
			n := min(s.N, in.maxStack()-in.N)
			in.N += n
			if s.N -= n; s.N == 0 {
				return slot{}
			}
		}
	}
	for i := range g.inv {
		if g.inv[i].empty() {
			n := min(s.N, s.maxStack())
			g.inv[i] = slot{s.ID, s.Dmg, n}
			if s.N -= n; s.N == 0 {
				return slot{}
			}
		}
	}
	return s
}

// held is the stack in the selected hotbar slot.
func (g *game) held() slot { return g.inv[g.sel] }

// consume takes one item from the selected stack in survival.
func (g *game) consume() {
	if g.creative {
		return
	}
	if g.inv[g.sel].N--; g.inv[g.sel].N <= 0 {
		g.inv[g.sel] = slot{}
	}
}

// useDurability wears the held tool by n uses and breaks it at the end.
func (g *game) useDurability(n int) {
	s := &g.inv[g.sel]
	if s.empty() || s.durability() == 0 || g.creative {
		return
	}
	if s.Dmg += uint16(n); int(s.Dmg) > s.durability() {
		*s = slot{}
	}
}
