package client

import (
	"bytes"
	"testing"
)

// The mode, its parts, their corners and the toggles are kept in options.txt with the rest, and so is its key.
func TestPvPOptionsAreSaved(t *testing.T) {
	testGame(t) // a config folder of the test's own
	o := defaultOptions()
	if o.PvP.On || !o.PvP.Parts["cps"] || o.PvP.Corner["armor"] != 3 || o.Keys["pvpMode"] != 25 {
		t.Fatalf("defaults: %+v, key %d", o.PvP, o.Keys["pvpMode"])
	}
	o.PvP.On, o.PvP.ToggleSprint, o.PvP.BlockHit = true, true, false
	o.PvP.Parts["reach"], o.PvP.Corner["keystrokes"] = false, 2
	o.Keys["pvpMode"] = 34 // G
	if err := o.save(); err != nil {
		t.Fatal(err)
	}
	got := loadOptions("")
	if !got.PvP.On || !got.PvP.ToggleSprint || got.PvP.BlockHit || got.PvP.Parts["reach"] || !got.PvP.Parts["cps"] ||
		got.PvP.Corner["keystrokes"] != 2 || got.Keys["pvpMode"] != 34 {
		t.Fatalf("read back %+v, key %d", got.PvP, got.Keys["pvpMode"])
	}
}

// frames runs ticks with what the keys say and returns every packet sent, as bytes.
func frames(g *game, c *netClient, ticks int) [][]byte {
	var out [][]byte
	for i := 0; i < ticks; i++ {
		g.tick(g.readInput())
		for len(c.out) > 0 {
			out = append(out, <-c.out)
		}
	}
	return out
}

// Toggle sprint and toggle sneak hold the key for the player: the server gets exactly what it gets from a player
// holding it, packet for packet.
func TestTogglesSendWhatAHeldKeySends(t *testing.T) {
	for _, key := range []string{"sprint", "sneak"} {
		held, hk, _ := duel(t, 0)
		heldKeys := map[string]bool{"forward": true, key: true}
		held.keyHeld = func(a string) bool { return heldKeys[a] }
		toggled, tc, _ := duel(t, 0)
		toggled.opts.PvP.On, toggled.opts.PvP.ToggleSprint, toggled.opts.PvP.ToggleSneak = true, true, true
		toggledKeys := map[string]bool{"forward": true, key: true}
		toggled.keyHeld = func(a string) bool { return toggledKeys[a] }
		toggled.pvpKey(toggled.opts.Keys[key]) // pressed once
		toggledKeys[key] = false               // and let go
		a, b := frames(held, hk, 30), frames(toggled, tc, 30)
		if len(a) != len(b) {
			t.Fatalf("%s: %d packets held, %d toggled", key, len(a), len(b))
		}
		for i := range a {
			if !bytes.Equal(a[i], b[i]) {
				t.Fatalf("%s: packet %d differs: held %x, toggled %x", key, i, a[i], b[i])
			}
		}
		if key == "sprint" && !toggled.sprinting || key == "sneak" && !toggled.sneaking {
			t.Fatalf("%s: the toggle did not hold it", key)
		}
		toggled.pvpKey(toggled.opts.Keys[key]) // pressed again: let go
		frames(toggled, tc, 2)
		if key == "sneak" && toggled.sneaking || key == "sprint" && toggled.latched("sprint") {
			t.Fatalf("%s: a second press did not let it go", key)
		}
	}
}

// Without the mode, or without the toggle, the sprint key is only held while it is down.
func TestNoToggleOutsideTheMode(t *testing.T) {
	g, _, _ := duel(t, 0)
	g.opts.PvP.ToggleSprint = true
	g.pvpKey(g.opts.Keys["sprint"])
	if g.latched("sprint") {
		t.Fatal("the sprint latched with the PvP mode off")
	}
	g.pvpKey(g.opts.Keys["pvpMode"])
	if !g.opts.PvP.On {
		t.Fatal("the mode's key did not switch it on")
	}
	g.pvpKey(g.opts.Keys["pvpMode"])
	if g.opts.PvP.On {
		t.Fatal("the mode's key did not switch it off")
	}
}

// The mode takes the view bobbing off while it is on, and the block-hit is drawn as 1.7 did only with it.
func TestModeTakesTheBobbingOff(t *testing.T) {
	g := testGame(t)
	g.opts = defaultOptions()
	if !g.viewBobbing() || g.blockHit17() {
		t.Fatal("without the mode the options rule")
	}
	g.opts.PvP.On = true
	if g.viewBobbing() || !g.blockHit17() {
		t.Fatal("the mode keeps the bobbing or the 1.8 block")
	}
}

// The 1.7 block-hit: with the sword up, the swing of the blow before it goes on being drawn; 1.8 leaves it out.
func TestBlockHitDrawsTheSwing(t *testing.T) {
	g := testGame(t)
	g.opts = defaultOptions()
	g.inv[0], g.sel = slot{ID: 276, N: 1}, 0
	g.hand.item, g.hand.equip, g.hand.prevEquip = g.inv[0], 1, 1
	g.blocking = true
	at := func(swing int) []vertex {
		g.swingTicks = swing
		var v []vertex
		for _, p := range g.handGeometry(0) {
			v = append(v, p.verts...)
		}
		return v
	}
	same := func(a, b []vertex) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	if rest, mid := at(0), at(3); !same(rest, mid) {
		t.Fatal("1.8 draws the swing while blocking")
	}
	g.opts.PvP.On = true
	if rest, mid := at(0), at(3); same(rest, mid) || len(rest) == 0 {
		t.Fatal("the 1.7 block-hit does not draw the swing")
	}
}
