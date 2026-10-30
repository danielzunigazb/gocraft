package client

// Food: what an item restores and how long it takes to eat, after 1.8's ItemFood.

type foodDef struct {
	points int
	sat    float32 // saturation modifier: the player gets points * modifier * 2
	always bool    // may be eaten with a full stomach
}

var foods = map[uint16]foodDef{
	260: {4, 0.3, false}, 297: {5, 0.6, false}, 319: {3, 0.3, false}, 320: {8, 0.8, false}, 322: {4, 1.2, true}, 349: {2, 0.1, false}, 350: {5, 0.6, false},
	357: {2, 0.1, false}, 360: {2, 0.3, false}, 363: {3, 0.3, false}, 364: {8, 0.8, false}, 365: {2, 0.3, false}, 366: {6, 0.6, false}, 367: {4, 0.1, false},
	382: {2, 0.3, false}, 391: {3, 0.6, false}, 392: {1, 0.3, false}, 393: {5, 0.6, false}, 394: {2, 0.3, false}, 396: {6, 1.2, false}, 400: {8, 0.3, false},
	411: {3, 0.3, false}, 412: {5, 0.6, false}, 413: {10, 0.6, false}, 423: {2, 0.3, false}, 424: {6, 0.8, false}, 282: {6, 0.6, false},
}

const eatTicks = 32

// tickEating advances eating while the use button is held over a food item.
func (g *game) tickEating() {
	s := g.held()
	f, isFood := foods[s.ID]
	if !isFood || s.isBlock() || g.screen != screenNone || !g.keyDown("use") || (g.food >= 20 && !f.always) {
		g.eating = 0
		return
	}
	if g.eating++; g.eating < eatTicks {
		if g.eating >= 8 && g.eating%4 == 0 { // a bite every fourth tick once there is little left to eat
			g.eatSound(false)
		}
		return
	}
	g.eating = 0
	g.eatSound(true)
	if g.thirdParty() { // the server feeds the player and takes the item; the release is not sent for a meal that is over
		g.net.sentUsing = false
		return
	}
	g.food = min(g.food+f.points, 20)
	g.saturation = min(g.saturation+float32(f.points)*f.sat*2, float32(g.food))
	if s.ID == 413 || s.ID == 282 { // stews leave the bowl
		g.consume()
		if g.inv[g.sel].empty() {
			g.inv[g.sel] = slot{ID: 281, N: 1}
		}
		return
	}
	g.consume()
}
