package client

import (
	"bytes"
	"testing"
	"time"
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

// The counters: clicks of the last second, a blow counts as landed when the server shows the rival hurt shortly
// after it was sent, a blow taken ends the combo, and the combo is over after three seconds without one landing.
func TestPvPCounters(t *testing.T) {
	var s pvpStats
	now := time.Now()
	for i := 0; i < 12; i++ {
		s.press(false, now.Add(-time.Duration(i)*100*time.Millisecond))
	}
	s.press(true, now)
	if l, r := s.cps(now); l != 10 || r != 1 {
		t.Fatalf("CPS %d %d, want 10 1 (two presses are older than a second)", l, r)
	}
	s.rivalHurt(9, 5) // hurt by somebody else: not ours
	if s.dealt != 0 {
		t.Fatal("a blow we did not send counted")
	}
	s.attacked(9, 10, 2.5, now)
	s.rivalHurt(9, 14)
	s.attacked(9, 25, 2.9, now)
	s.rivalHurt(9, 30)
	if s.combo != 2 || s.dealt != 2 || s.comboNow(31) != 2 {
		t.Fatalf("combo %d dealt %d", s.combo, s.dealt)
	}
	s.attacked(9, 40, 3, now)
	s.rivalHurt(9, 40+landWindow+1) // too late to be that blow
	if s.dealt != 2 {
		t.Fatal("a hurt long after the attack counted as landed")
	}
	s.hurt()
	if s.combo != 0 || s.taken != 1 {
		t.Fatalf("after a blow taken: combo %d taken %d", s.combo, s.taken)
	}
	s.attacked(9, 100, 2, now)
	s.rivalHurt(9, 101)
	if s.comboNow(101+comboTimeout+1) != 0 {
		t.Fatal("the combo did not run out")
	}
}

// Through the game: the attack records how far it reached, the server's hurt status lands it, and the player's own
// hurt status counts as a blow taken.
func TestCountersFromThePackets(t *testing.T) {
	g, _, _ := duel(t, 2.2)
	g.pressButton(g.opts.Keys["attack"])
	g.tick(input{})
	if r := g.pvp.stats.reach; !near(r, 1.3, 1e-6) { // from the eyes at z 0.5 to the grown box at z 1.8
		t.Fatalf("reach %v, want 1.3", r)
	}
	st := newPacket(0x1A)
	st.Int(9)
	st.Byte(2)
	decodeFrame(t, st, nil)(g)
	st = newPacket(0x1A)
	st.Int(g.net.eid)
	st.Byte(2)
	decodeFrame(t, st, nil)(g)
	if s := g.pvp.stats; s.dealt != 1 || s.taken != 1 || s.combo != 0 {
		t.Fatalf("dealt %d taken %d combo %d", s.dealt, s.taken, s.combo)
	}
}

// The HUD's parts go to the corners the options say, the ones turned off are not there, and the effects read as the
// inventory writes them.
func TestPvPHUDLayout(t *testing.T) {
	g, _, _ := duel(t, 0)
	g.fb = [2]int{1280, 720}
	g.opts.PvP.On = true
	g.effects = map[int]potionEffect{effSpeed: {amp: 1, ticks: 1830}}
	g.armor[0] = slot{ID: 310, N: 1}
	g.net.c.name = "Eve"
	g.net.tab = map[string]*tabEntry{"x": {name: "Eve", ping: 87}}
	count := func(b [4][]hudBox) (n [4]int) {
		for i := range b {
			n[i] = len(b[i])
		}
		return
	}
	if n := count(g.pvpBoxes(time.Now())); n != [4]int{4, 1, 0, 1} { // keys (with the CPS), combo, reach, ping | potions | | armour
		t.Fatalf("parts by corner %v", n)
	}
	g.opts.PvP.Parts["keystrokes"] = false // the CPS gets a line of its own
	g.opts.PvP.Parts["ping"] = false
	g.opts.PvP.Corner["armor"] = 2
	if n := count(g.pvpBoxes(time.Now())); n != [4]int{3, 1, 1, 0} {
		t.Fatalf("parts by corner %v", n)
	}
	if ms, ok := g.ownPing(); !ok || ms != 87 {
		t.Fatalf("own ping %d %v", ms, ok)
	}
	if n, left := potionLine(effSpeed, g.effects[effSpeed]); n != "Speed II" || left != "1:31" {
		t.Fatalf("%q %q", n, left)
	}
	if _, left := potionLine(12, potionEffect{ticks: 30000, max: true}); left != "**:**" {
		t.Fatalf("a full-length effect shows %q", left)
	}
	g.opts.PvP.On = false
	if n := count(g.pvpBoxes(time.Now())); n != [4]int{} {
		t.Fatalf("with the mode off: %v", n)
	}
}
