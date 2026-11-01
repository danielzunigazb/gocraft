package client

import (
	"testing"

	"gocraft/world"
)

// duel is a player on a server standing at the origin, facing south (+z), with a rival 1.7 blocks in front and a
// sword in hand. held says which keys are down.
func duel(t *testing.T, rivalZ float64) (g *game, c *netClient, held map[string]bool) {
	g, c = windowGame(t)
	g.opts = defaultOptions()
	g.net.eid, g.net.joined = 1, true
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	g.creative = false
	g.feet, g.prevFeet = vec3{0.5, 4, 0.5}, vec3{0.5, 4, 0.5}
	g.pos = g.feet.add(vec3{0, eyeHeight, 0})
	g.yaw, g.pitch, g.onGround = 0, 0, true
	g.inv[0], g.sel = slot{ID: 276, N: 1}, 0
	if rivalZ > 0 {
		at := vec3{0.5, 4, rivalZ}
		g.net.remote[9] = &remotePlayer{eid: 9, name: "Rival", pos: at, prev: at, target: at}
	}
	held = map[string]bool{}
	g.keyHeld = func(a string) bool { return held[a] }
	g.tick(input{}) // a first tick: the player has a yaw of the last tick, and the server has heard of the slot
	drain(t, c)
	return g, c, held
}

// ids are the packets sent, in order.
func ids(t *testing.T, c *netClient) []int32 {
	t.Helper()
	var out []int32
	for _, p := range drain(t, c) {
		out = append(out, p.id)
	}
	return out
}

func index(list []int32, id int32) int {
	for i, v := range list {
		if v == id {
			return i
		}
	}
	return -1
}

// A click is taken in at the next tick, and the swing, the slot and the attack go out in the game's order before
// that tick's movement packet.
func TestAttackGoesOutAtTheTickBeforeTheMovement(t *testing.T) {
	g, c, _ := duel(t, 2.2)
	g.sel = 1 // a slot changed in the same tick as the click: the server must know it before the blow
	g.inv[1] = slot{ID: 267, N: 1}
	g.pressButton(g.opts.Keys["attack"])
	if got := ids(t, c); len(got) != 0 {
		t.Fatalf("the press itself sent %#x: the game waits for the tick", got)
	}
	g.tick(input{})
	got := ids(t, c)
	swing, slot, attack, move := index(got, 0x0A), index(got, 0x09), index(got, 0x02), max(index(got, 0x03), index(got, 0x04), index(got, 0x05), index(got, 0x06))
	if swing < 0 || attack < 0 || move < 0 || !(swing < slot && slot < attack && attack < move) {
		t.Fatalf("sent %#x: want the swing, the held slot, the attack, then the movement", got)
	}
}

// 1.8's miss penalty: a click on nothing makes the attack wait while the button stays down; a tick that finds it up
// lifts it. Without it a client would hit as if it had a "no hit delay" mod.
func TestMissHoldsTheNextAttackBackWhileTheButtonIsDown(t *testing.T) {
	g, c, held := duel(t, 0)
	held["attack"] = true
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if got := ids(t, c); index(got, 0x0A) < 0 || index(got, 0x02) >= 0 {
		t.Fatalf("a click on the air sent %#x: want only the swing", got)
	}
	if g.pvp.clicks.left != missPenalty {
		t.Fatalf("the miss penalty is %d, want %d", g.pvp.clicks.left, missPenalty)
	}
	at := vec3{0.5, 4, 2.2} // a rival steps in front while the button is still down
	g.net.remote[9] = &remotePlayer{eid: 9, name: "Rival", pos: at, prev: at, target: at}
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if got := ids(t, c); index(got, 0x0A) >= 0 || index(got, 0x02) >= 0 {
		t.Fatalf("a click during the penalty sent %#x: it must do nothing at all", got)
	}
	held["attack"] = false // let go: the next tick lifts the penalty
	g.tick(input{})
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if got := ids(t, c); index(got, 0x02) < 0 {
		t.Fatalf("after letting go the attack sent %#x", got)
	}
}

// A press and a release inside one tick lifts the penalty at once: fast clicking on the air is not held back.
func TestQuickMissIsNotHeldBack(t *testing.T) {
	g, _, _ := duel(t, 0)
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{}) // the button is already up when the tick takes the press in
	if g.pvp.clicks.left != 0 {
		t.Fatalf("penalty %d after a press that was let go within the tick", g.pvp.clicks.left)
	}
}

// A rival on the view's ray but more than 3 blocks away makes a miss, even with a block behind them.
func TestRivalBeyondReachIsAMissNotABlock(t *testing.T) {
	g, c, held := duel(t, 4.3)
	g.world.SetBlock(0, 5, 6, 1<<4) // a wall behind the rival, within the 4.5 a block may be reached at
	held["attack"] = true
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if index(ids(t, c), 0x02) >= 0 || g.pvp.clicks.left != missPenalty {
		t.Fatalf("a rival out of reach in front of a block: penalty %d", g.pvp.clicks.left)
	}
	g2, _, held2 := duel(t, 0)
	g2.world.SetBlock(0, 5, 3, 1<<4)
	held2["attack"] = true
	g2.pressButton(g2.opts.Keys["attack"])
	g2.tick(input{})
	if g2.pvp.clicks.left != 0 {
		t.Fatalf("a click on a block is not a miss, penalty %d", g2.pvp.clicks.left)
	}
}

// While an item is in use (here a sword held up) the attack presses are thrown away: a vanilla client never sends
// an attack while it blocks, and a server's anticheat takes one that does for an "autoblock".
func TestNoAttackWhileBlocking(t *testing.T) {
	g, c, _ := duel(t, 2.2)
	g.blocking = true
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if got := ids(t, c); index(got, 0x0A) >= 0 || index(got, 0x02) >= 0 {
		t.Fatalf("sent %#x while blocking", got)
	}
	if g.pvp.clicks.attack != 0 {
		t.Fatal("the press was kept for later")
	}
}

// Both buttons pressed within one tick: the attack goes first, then the use, as runTick takes them in. That is
// how a block-hit reaches a server: the blow, and then the sword up.
func TestAttackThenUseInTheSameTick(t *testing.T) {
	g, c, _ := duel(t, 2.2)
	g.pressButton(g.opts.Keys["use"])
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	got := ids(t, c)
	if a, u := index(got, 0x02), index(got, 0x08); a < 0 || u < 0 || a > u {
		t.Fatalf("sent %#x: want the attack before the use", got)
	}
}

// The click aims from where the last tick left the player, not from where the camera is drawn between ticks, and
// along the yaw of the last tick: a turn made after it is not in the aim yet (getLook reads rotationYawHead).
func TestClickAimsFromTheTick(t *testing.T) {
	g, c, _ := duel(t, 2.2)
	g.pos = g.pos.add(vec3{0, 0, -3}) // the camera drawn far behind, as an interpolated frame could be
	g.yaw += 1.2                      // and the view turned away from the rival since the last tick
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if index(ids(t, c), 0x02) < 0 {
		t.Fatal("the click did not aim from the tick's eyes and yaw")
	}
	g2, c2, _ := duel(t, 2.2)
	g2.yaw += 1.2
	g2.tick(input{}) // a tick with the view away: now that is the aim
	drain(t, c2)
	g2.pressButton(g2.opts.Keys["attack"])
	g2.tick(input{})
	if index(ids(t, c2), 0x02) >= 0 {
		t.Fatal("a tick later the turn should be in the aim")
	}
}

// With a screen open the clicks are the screen's, and an attack waits until the button has been let go.
func TestScreenTakesTheClicks(t *testing.T) {
	g, c, held := duel(t, 2.2)
	g.screen = screenInventory
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	g.screen = screenNone
	held["attack"] = true // the button that closed it is still down
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if index(ids(t, c), 0x02) >= 0 {
		t.Fatal("an attack went out through a screen, or right after it with the button still down")
	}
}
