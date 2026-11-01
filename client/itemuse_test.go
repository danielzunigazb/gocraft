package client

import "testing"

// releases counts the Player Digging "release use item" packets sent.
func releases(t *testing.T, c *netClient) (n int, all []int32) {
	t.Helper()
	for _, p := range drain(t, c) {
		all = append(all, p.id)
		if p.id == 0x07 && p.r.Byte() == 5 {
			n++
		}
	}
	return n, all
}

// A short tap of the use button with a sword: the click goes out, the sword is up for that tick (the player is slowed
// as the server's copy is), and the next tick, finding the button up, sends the release. Before, the client missed the
// tap: the server held the sword up with no end and the client neither slowed down nor let go.
func TestShortTapOfTheSwordIsReleased(t *testing.T) {
	g, c, _ := duel(t, 0)
	g.pressButton(g.opts.Keys["use"])
	g.tick(input{forward: 1})
	n, sent := releases(t, c)
	if index(sent, 0x08) < 0 || n != 0 || !g.blocking || !g.usingItem() {
		t.Fatalf("after the tap's tick: sent %#x, blocking %v: want the click out and the sword up", sent, g.blocking)
	}
	g.tick(input{forward: 1})
	if n, sent = releases(t, c); n != 1 || g.blocking {
		t.Fatalf("the tick after the tap sent %#x (blocking %v): want the release", sent, g.blocking)
	}
	if mv := index(sent, 0x04); mv >= 0 && mv < index(sent, 0x07) {
		t.Fatalf("sent %#x: the release goes before the movement packet", sent)
	}
}

// Held, the sword stays up and nothing more is sent until the button is let go.
func TestHeldSwordStaysUpUntilLetGo(t *testing.T) {
	g, c, held := duel(t, 0)
	held["use"] = true
	g.pressButton(g.opts.Keys["use"])
	for i := 0; i < 10; i++ {
		g.tick(input{forward: 1, sprint: true})
		if !g.blocking || g.sprinting {
			t.Fatalf("tick %d: blocking %v sprinting %v", i, g.blocking, g.sprinting)
		}
	}
	if n, sent := releases(t, c); n != 0 || len(sent) == 0 || countOf(sent, 0x08) != 1 {
		t.Fatalf("sent %#x while holding: want one use and no release", sent)
	}
	held["use"] = false
	g.tick(input{})
	if n, _ := releases(t, c); n != 1 {
		t.Fatalf("%d releases after letting go", n)
	}
}

func countOf(list []int32, id int32) int {
	n := 0
	for _, v := range list {
		if v == id {
			n++
		}
	}
	return n
}

// A potion is drunk over 32 ticks with the player slowed, and the client goes on drinking until the server says it is
// over: a client that stopped on its own count would run at full speed while the server still had it drinking.
func TestPotionIsDrunkUntilTheServerSaysSo(t *testing.T) {
	g, _, held := duel(t, 0)
	g.inv[0] = slot{ID: 373, N: 1, Dmg: 8194} // a potion of swiftness
	held["use"] = true
	g.pressButton(g.opts.Keys["use"])
	for i := 0; i < 40; i++ {
		g.tick(input{forward: 1, sprint: true})
	}
	if !g.usingItem() || g.sprinting || g.eating != 40 {
		t.Fatalf("after 40 ticks: in use %v, sprinting %v, eating %d: still drinking until the server says", g.usingItem(), g.sprinting, g.eating)
	}
	w := newPacket(0x1A)
	w.Int(g.net.eid)
	w.Byte(9)
	decodeFrame(t, w, nil)(g)
	if g.usingItem() || g.eating != 0 {
		t.Fatal("the server's word did not end the drink")
	}
	g.inv[0] = slot{ID: 373, N: 1, Dmg: 16386} // a splash potion is thrown, never drunk
	g.pressButton(g.opts.Keys["use"])
	g.tick(input{})
	if g.usingItem() {
		t.Fatal("a splash potion was put in use")
	}
}

// Another slot in hand ends the item in use without a release (the server sees the slot change and ends its own), and
// so does a new stack the server puts in the held slot.
func TestItemInUseEndsWithTheHand(t *testing.T) {
	g, c, held := duel(t, 0)
	held["use"] = true
	g.pressButton(g.opts.Keys["use"])
	g.tick(input{})
	drain(t, c)
	g.sel = 1
	g.tick(input{})
	if n, _ := releases(t, c); n != 0 || g.blocking {
		t.Fatalf("after changing slot: %d releases, blocking %v", n, g.blocking)
	}
	g2, _, held2 := duel(t, 0)
	held2["use"] = true
	g2.pressButton(g2.opts.Keys["use"])
	g2.tick(input{})
	g2.netSetSlot(0, 36, slot{ID: 276, N: 1, Dmg: 1}, itemExtra{}) // the server resends the sword
	g2.tick(input{})
	if g2.blocking {
		t.Fatal("the stack the server put in hand is not the one in use any more")
	}
}

// Food only goes in use when the player may eat it, and a golden apple always.
func TestFoodInUseOnlyWhenHungry(t *testing.T) {
	g, _, _ := duel(t, 0)
	g.food = 20
	if d, _, _ := g.useDuration(slot{ID: 364, N: 1}); d != 0 {
		t.Fatal("a steak went in use with a full stomach")
	}
	if d, k, _ := g.useDuration(slot{ID: 322, N: 1}); d != 32 || k != useEat {
		t.Fatal("a golden apple must always be edible")
	}
	if d, _, _ := g.useDuration(slot{ID: 261, N: 1}); d != 0 {
		t.Fatal("a bow without arrows went in use")
	}
}
