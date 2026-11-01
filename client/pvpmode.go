package client

import (
	"fmt"
	"strings"
)

// The PvP mode: a switch (a key, and the options screen) that turns on a set of aids of the kind PvP clients
// such as Lunar or Badlion bring, limited to what only shows information or saves the player a key: counters
// and displays on the HUD, toggle sprint and toggle sneak (the server sees the same packets as with the key
// held), the 1.7 look of a block-hit, and no view bobbing. None of it changes how the player moves, what is
// sent, or what can be seen of the world: nothing that clicks, aims, reaches or lights up for the player.

// pvpOptions is the mode and its parts, saved in options.txt with the rest.
type pvpOptions struct {
	On           bool
	Parts        map[string]bool // HUD parts, by id (pvpParts)
	Corner       map[string]int  // where each part sits: 0 top left, 1 top right, 2 bottom left, 3 bottom right
	ToggleSprint bool
	ToggleSneak  bool
	BlockHit     bool // the sword's block-hit drawn as 1.7 did: the swing goes on with the sword up
	NoBob        bool // no view bobbing while the mode is on
}

// pvpParts are the HUD parts in the order they stack in a corner, with their label and default corner.
var pvpParts = []struct {
	id, label string
	corner    int
}{
	{"keystrokes", "Keystrokes", 0}, {"cps", "CPS", 0}, {"combo", "Combo", 0}, {"reach", "Reach Display", 0},
	{"ping", "Ping", 0}, {"potions", "Potion Status", 1}, {"armor", "Armor Status", 3},
}

var cornerNames = [4]string{"Top Left", "Top Right", "Bottom Left", "Bottom Right"}

func defaultPvP() pvpOptions {
	o := pvpOptions{Parts: map[string]bool{}, Corner: map[string]int{}, BlockHit: true, NoBob: true}
	for _, p := range pvpParts {
		o.Parts[p.id], o.Corner[p.id] = true, p.corner
	}
	return o
}

// parse reads one line of options.txt that belongs to the mode, and says whether it was one.
func (o *pvpOptions) parse(k, v string) bool {
	on := v == "true"
	switch k {
	case "pvpMode":
		o.On = on
	case "pvpToggleSprint":
		o.ToggleSprint = on
	case "pvpToggleSneak":
		o.ToggleSneak = on
	case "pvpBlockHit":
		o.BlockHit = on
	case "pvpNoBob":
		o.NoBob = on
	default:
		if id, ok := strings.CutPrefix(k, "pvpCorner_"); ok {
			var c int
			if _, err := fmt.Sscanf(v, "%d", &c); err == nil && c >= 0 && c < 4 && o.known(id) {
				o.Corner[id] = c
			}
			return true
		}
		if id, ok := strings.CutPrefix(k, "pvp_"); ok {
			if o.known(id) {
				o.Parts[id] = on
			}
			return true
		}
		return false
	}
	return true
}

func (o *pvpOptions) known(id string) bool {
	for _, p := range pvpParts {
		if p.id == id {
			return true
		}
	}
	return false
}

func (o pvpOptions) write(b *strings.Builder) {
	fmt.Fprintf(b, "pvpMode:%v\npvpToggleSprint:%v\npvpToggleSneak:%v\npvpBlockHit:%v\npvpNoBob:%v\n", o.On, o.ToggleSprint, o.ToggleSneak, o.BlockHit, o.NoBob)
	for _, p := range pvpParts {
		fmt.Fprintf(b, "pvp_%s:%v\npvpCorner_%s:%d\n", p.id, o.Parts[p.id], p.id, o.Corner[p.id])
	}
}

// shows is whether a HUD part is on screen: the mode on, and the part with it.
func (o pvpOptions) shows(id string) bool { return o.On && o.Parts[id] }

// pvpKey is a key pressed in the game: the mode's own key switches it, and with the toggles on the sprint and
// sneak keys latch instead of having to be held.
func (g *game) pvpKey(code int) {
	if code == 0 {
		return
	}
	o := &g.opts.PvP
	switch code {
	case g.opts.Keys["pvpMode"]:
		o.On = !o.On
		g.pvp.sprintLatch, g.pvp.sneakLatch = false, false
		g.addChatRuns(legacyRuns("§7PvP mode: "+map[bool]string{true: "§aON", false: "§cOFF"}[o.On], defaultStyle))
		if err := g.opts.save(); err != nil {
			logf("saving options: %v", err)
		}
	case g.opts.Keys["sprint"]:
		if o.On && o.ToggleSprint {
			g.pvp.sprintLatch = !g.pvp.sprintLatch
		}
	case g.opts.Keys["sneak"]:
		if o.On && o.ToggleSneak {
			g.pvp.sneakLatch = !g.pvp.sneakLatch
		}
	}
}

// latched is whether the mode holds the sprint or the sneak key down for the player. The game then reads the key as
// held (KeyBinding.isKeyDown), so it sprints, sneaks and tells the server exactly as with the key under the finger.
func (g *game) latched(action string) bool {
	o := g.opts.PvP
	switch action {
	case "sprint":
		return o.On && o.ToggleSprint && g.pvp.sprintLatch
	case "sneak":
		return o.On && o.ToggleSneak && g.pvp.sneakLatch
	}
	return false
}

// viewBobbing is whether the view and the hand sway with each step: the option, unless the PvP mode takes it off.
func (g *game) viewBobbing() bool {
	return g.opts.ViewBobbing && !(g.opts.PvP.On && g.opts.PvP.NoBob)
}

// blockHit17 is whether a block-hit is drawn as in 1.7.
func (g *game) blockHit17() bool { return g.opts.PvP.On && g.opts.PvP.BlockHit }

// pvpPage is which page of the PvP options the options screen shows: 0 none, 1 the switches, 2 where the parts go.
var pvpPage int

// drawPvPOptions is the options page of the mode.
func (g *game) drawPvPOptions(cx, s float32) {
	g.dirtBackground(0.45)
	w, h := float32(g.fb[0]), float32(g.fb[1])
	o := &g.opts.PvP
	if o.Parts == nil {
		*o = defaultPvP()
	}
	lx, rx := cx-155, cx+5
	top := h/s/6 - 12
	if pvpPage == 2 {
		g.centredText("PvP HUD Positions", w/2, 15*s, s, [4]float32{1, 1, 1, 1})
		for i, p := range pvpParts {
			y := top + float32(i)*22
			g.text(p.label, lx*s, (y+6)*s, s, [4]float32{1, 1, 1, 1}, true)
			if g.button(cornerNames[o.Corner[p.id]], rx, y, 150, true) {
				o.Corner[p.id] = (o.Corner[p.id] + 1) % 4
			}
		}
		if g.button("Done", cx-100, h/s-28, 200, true) {
			pvpPage = 1
		}
		return
	}
	g.centredText("PvP Mode", w/2, 15*s, s, [4]float32{1, 1, 1, 1})
	flip := func(label string, v *bool, x, y float32) {
		if g.button(label+": "+onOff(*v), x, y, 150, true) {
			*v = !*v
		}
	}
	flip("PvP Mode", &o.On, lx, top)
	flip("No View Bobbing", &o.NoBob, rx, top)
	for i, p := range pvpParts {
		v := o.Parts[p.id]
		flip(p.label, &v, []float32{lx, rx}[i%2], top+24*float32(1+i/2))
		o.Parts[p.id] = v
	}
	flip("1.7 Block-hit", &o.BlockHit, rx, top+24*4)
	flip("Toggle Sprint", &o.ToggleSprint, lx, top+24*5)
	flip("Toggle Sneak", &o.ToggleSneak, rx, top+24*5)
	if !o.ToggleSprint {
		g.pvp.sprintLatch = false
	}
	if !o.ToggleSneak {
		g.pvp.sneakLatch = false
	}
	if g.button("HUD Positions...", cx-100, top+24*6, 200, true) {
		pvpPage = 2
	}
	g.centredText("Key: "+keyName(g.opts.Keys["pvpMode"])+" (Controls)", w/2, (h/s-40)*s, s, [4]float32{0.63, 0.63, 0.63, 1})
	if g.button("Done", cx-100, h/s-28, 200, true) {
		pvpPage = 0
	}
}
