package client

// The item in use on a third-party server, as EntityPlayer keeps it (itemInUse, itemInUseCount). A right click
// with a sword, a bow (with arrows), something to eat or to drink puts it in use at once, and it stays in use
// until the use button is found up at the start of a tick (then the release goes out), until the hand holds
// something else (then it just ends, as the server's copy does), or until the server says a meal or a drink is
// over (Entity Status 9). While it is in use the player walks at a fifth of the speed, cannot sprint and cannot
// attack, on the client as in the server's prediction of it.
//
// Taking "in use" from the button held at each tick instead, as gocraft did, lost a short tap: the click had gone
// out and the server held the sword up, but the client never knew, did not slow down and never sent the release.

type itemUse struct {
	on       bool
	item     slot
	sel      int     // the hotbar slot it was taken from
	kind     handUse // useBlock, useBow or useEat (a drink is drawn as a meal, as ItemRenderer does)
	drink    bool
	count    int // ticks left: it goes below zero while the server has not yet said the meal is over
	dur      int
	replaced bool // the server put a stack in that slot: the game's item in use is no longer the one in hand
}

// useDuration is how long an item can be used (Item.getMaxItemUseDuration) and how, when right clicking it would put
// it in use now (onItemRightClick calls setItemInUse): 0 for an item that is not used that way.
func (g *game) useDuration(s slot) (int, handUse, bool) {
	if s.empty() || s.isBlock() {
		return 0, useNone, false
	}
	if class, _ := toolOf(s); class == "sword" {
		return 72000, useBlock, false
	}
	switch {
	case s.ID == 261:
		if g.creative || g.hasArrow() {
			return 72000, useBow, false
		}
		return 0, useNone, false
	case s.ID == 373 && s.Dmg&16384 == 0, s.ID == 335: // a potion that is not thrown, a bucket of milk
		return 32, useEat, true
	}
	f, ok := foods[s.ID]
	if !ok || g.creative || g.abilFlags&1 != 0 || (g.food >= 20 && !f.always) { // EntityPlayer.canEat
		return 0, useNone, false
	}
	return 32, useEat, false
}

// startUsing is the use of the item in the air (PlayerControllerMP.sendUseItem), once its packet has gone out.
func (g *game) startUsing(s slot) {
	if d, k, drink := g.useDuration(s); d > 0 {
		g.pvp.use = itemUse{on: true, item: s, sel: g.sel, kind: k, drink: drink, count: d, dur: d}
		g.useView()
	}
}

// stopUsing is PlayerControllerMP.onStoppedUsingItem: the release goes to the server and the item is no longer in use.
func (g *game) stopUsing() {
	g.syncHeld()
	w := newPacket(0x07)
	w.Byte(5) // RELEASE_USE_ITEM, at the origin, facing down
	w.Long(0)
	w.Byte(0)
	g.net.c.send(w)
	g.pvp.use = itemUse{}
	g.useView()
}

// tickItemUse is the start of EntityPlayer.onUpdate on a third-party server: the item in use ends when the hand holds
// another one, or counts a tick down (with the sounds of a meal past its first bites). A client never finishes a meal
// by itself; the server does, and says so.
func (g *game) tickItemUse() {
	if !g.thirdParty() {
		return
	}
	u := &g.pvp.use
	if u.on {
		if g.sel != u.sel || u.replaced || !sameStack(g.held(), u.item) {
			*u = itemUse{} // clearItemInUse: nothing is sent, the server sees the hand change and does the same
		} else {
			if u.kind == useEat && u.count <= 25 && u.count%4 == 0 {
				g.useSound(u.drink)
			}
			u.count--
		}
	}
	u.replaced = false
	g.useView()
}

// useView sets what the hand, the field of view and the movement read from the item in use.
func (g *game) useView() {
	u := g.pvp.use
	g.blocking = u.on && u.kind == useBlock
	g.bowTicks, g.eating = 0, 0
	switch {
	case u.on && u.kind == useBow:
		g.bowTicks = u.dur - u.count // getItemInUseDuration
	case u.on && u.kind == useEat:
		g.eating = u.dur - u.count
	}
}

// useFinished is Entity Status 9 for the player: the server ate the meal or drank the potion (onItemUseFinish).
func (g *game) useFinished() {
	if u := g.pvp.use; u.on {
		if u.kind == useEat {
			g.useSound(u.drink)
		}
		g.pvp.use = itemUse{}
		g.useView()
	}
}

func (g *game) useSound(drink bool) {
	if drink {
		if g.snd != nil {
			g.sound("random.drink", g.pos, 0.5, rnd()*0.1+0.9)
		}
		return
	}
	g.eatSound(false)
}

// sameStack is whether two stacks are the same item for the item in use: the same id and damage.
func sameStack(a, b slot) bool { return a.ID == b.ID && a.Dmg == b.Dmg && !a.empty() && !b.empty() }
