package client

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// The PvP mode's HUD: clicks per second, the keys held, the armour's and the hand's durability, the potion
// effects with their time left, the combo, how far the last blow reached, and the ping the server lists for the
// player. Each part can be turned off and put in a corner; the parts of a corner stack in a fixed order.
// Everything shown is what the game already knows; the reach is measured on the blow the player sent.

// pvpStats is what the HUD counts.
type pvpStats struct {
	left, right  []time.Time   // presses of the buttons in the last second
	combo        int           // blows that landed in a row without one taken
	dealt, taken int           // blows that landed and blows taken, since the join or the last respawn
	lastLanded   int           // the tick of the last blow that landed
	pending      map[int32]int // the tick of the last attack sent on each rival, until the server shows them hurt
	reach        float64       // how far from the eyes the last blow met the rival's box
	reachAt      time.Time     // when it was sent
}

const (
	landWindow   = 20 // ticks after an attack in which the server showing the rival hurt counts it as landed
	comboTimeout = 60 // ticks without a blow landing after which the combo is over
)

// press counts a click for the CPS: the attack button is the left one, the use button the right one.
func (s *pvpStats) press(right bool, now time.Time) {
	l := &s.left
	if right {
		l = &s.right
	}
	*l = append(trimSecond(*l, now), now)
}

func trimSecond(l []time.Time, now time.Time) []time.Time {
	out := l[:0]
	for _, t := range l {
		if now.Sub(t) < time.Second {
			out = append(out, t)
		}
	}
	return out
}

// cps is the clicks of each button in the last second.
func (s *pvpStats) cps(now time.Time) (left, right int) {
	s.left, s.right = trimSecond(s.left, now), trimSecond(s.right, now)
	return len(s.left), len(s.right)
}

// attacked is an attack sent on a rival, with how far it reached.
func (s *pvpStats) attacked(eid int32, tick int, reach float64, now time.Time) {
	if s.pending == nil {
		s.pending = map[int32]int{}
	}
	s.pending[eid] = tick
	s.reach, s.reachAt = reach, now
}

// rivalHurt is the server showing a rival hurt (Entity Status 2): a blow of ours if we sent one on them just before.
func (s *pvpStats) rivalHurt(eid int32, tick int) {
	at, ok := s.pending[eid]
	if !ok || tick-at > landWindow {
		return
	}
	delete(s.pending, eid)
	if tick-s.lastLanded > comboTimeout {
		s.combo = 0
	}
	s.combo++
	s.dealt++
	s.lastLanded = tick
}

// newFight starts the counts again: a respawn, another world of the server.
func (s *pvpStats) newFight() { s.dealt, s.taken, s.combo, s.pending = 0, 0, 0, nil }

// hurt is the player taking a blow: the combo is over.
func (s *pvpStats) hurt() {
	s.taken++
	s.combo = 0
}

// comboNow is the combo as the HUD shows it: over once no blow has landed for a while.
func (s *pvpStats) comboNow(tick int) int {
	if tick-s.lastLanded > comboTimeout {
		return 0
	}
	return s.combo
}

// blowReach is how far from the eyes the view's ray meets a rival's box (grown by a tenth, as the game aims),
// 0 with the eyes inside it.
func (g *game) blowReach(eid int32) float64 {
	box, ok := g.entityBox(eid)
	if !ok {
		return 0
	}
	box.min, box.max = box.min.sub(vec3{0.1, 0.1, 0.1}), box.max.add(vec3{0.1, 0.1, 0.1})
	if at, _, ok := segmentBox(box, g.pos, g.pos.add(look(g.yaw, g.pitch).scale(6))); ok {
		return math.Sqrt(at.sub(g.pos).dot(at.sub(g.pos)))
	}
	return 0
}

// ownPing is the latency the server's player list gives the player, if it lists them.
func (g *game) ownPing() (int, bool) {
	if g.net == nil || g.net.c == nil {
		return 0, false
	}
	for _, e := range g.net.tab {
		if e.name == g.net.c.name {
			return e.ping, true
		}
	}
	return 0, false
}

// ---- drawing ----

// hudBox is a part laid out in GUI pixels: its size, and how to draw it at a corner of the box.
type hudBox struct {
	w, h float32
	draw func(x, y float32)
}

var (
	pvpBack  = [4]float32{0, 0, 0, 0.4}
	pvpLit   = [4]float32{1, 1, 1, 0.75}
	pvpWhite = [4]float32{1, 1, 1, 1}
	pvpGrey  = [4]float32{0.67, 0.67, 0.67, 1}
	pvpBlack = [4]float32{0, 0, 0, 1}
)

// fill queues a plain rectangle in GUI pixels; flushFill draws what was queued.
func (g *game) fill(x, y, w, h float32, col [4]float32) {
	s := float32(g.scaleFor(g.fb[0], g.fb[1]))
	g.hud.rect(x*s, y*s, w*s, h*s, 0, 0, 1, 1, 1, 1, col)
}

func (g *game) flushFill() { g.hud.flush(g.hud.white) }

// label draws text at GUI pixels.
func (g *game) label(text string, x, y float32, col [4]float32, shadow bool) {
	s := float32(g.scaleFor(g.fb[0], g.fb[1]))
	g.text(text, x*s, y*s, s, col, shadow)
}

// textBox is a line or two of text on the translucent background.
func (g *game) textBox(lines ...string) hudBox {
	w := float32(0)
	for _, l := range lines {
		w = max(w, float32(g.textWidth(l)))
	}
	return hudBox{w + 6, float32(len(lines))*10 + 2, func(x, y float32) {
		g.fill(x, y, w+6, float32(len(lines))*10+2, pvpBack)
		g.flushFill()
		for i, l := range lines {
			g.label(l, x+3, y+2+float32(i)*10, pvpWhite, true)
		}
	}}
}

// pvpBoxes are the parts on screen, by corner.
func (g *game) pvpBoxes(now time.Time) [4][]hudBox {
	o := g.opts.PvP
	st := &g.pvp.stats
	var out [4][]hudBox
	add := func(id string, b hudBox) { out[o.Corner[id]] = append(out[o.Corner[id]], b) }
	l, r := st.cps(now)
	if o.shows("keystrokes") {
		add("keystrokes", g.keystrokesBox(l, r, o.shows("cps")))
	} else if o.shows("cps") {
		add("cps", g.textBox(fmt.Sprintf("CPS: %d | %d", l, r)))
	}
	if g.latched("sprint") || g.latched("sneak") {
		text := "[Sprinting (Toggled)]"
		if g.latched("sneak") {
			text = "[Sneaking (Toggled)]"
		}
		add("keystrokes", g.textBox(text))
	}
	if o.shows("combo") {
		c := st.comboNow(g.ticks)
		head := "No Combo"
		if c > 0 {
			head = fmt.Sprintf("Combo: %d", c)
		}
		add("combo", g.textBox(head, fmt.Sprintf("Hits: %d dealt, %d taken", st.dealt, st.taken)))
	}
	if o.shows("reach") {
		text := "Reach: -"
		if !st.reachAt.IsZero() && now.Sub(st.reachAt) < 3*time.Second {
			text = fmt.Sprintf("Reach: %.2f", st.reach)
		}
		add("reach", g.textBox(text))
	}
	if o.shows("ping") {
		text := "Ping: -"
		if ms, ok := g.ownPing(); ok {
			text = fmt.Sprintf("Ping: %d ms", ms)
		}
		add("ping", g.textBox(text))
	}
	if o.shows("potions") && len(g.effects) > 0 {
		add("potions", g.potionsBox())
	}
	if o.shows("armor") {
		if b, ok := g.armorBox(); ok {
			add("armor", b)
		}
	}
	return out
}

// drawPvPHUD lays the parts out in their corners and draws them.
func (g *game) drawPvPHUD(w, h int) {
	if !g.opts.PvP.On || !g.inGame || g.debug {
		return
	}
	g.enableUI()
	s := float32(g.scaleFor(w, h))
	gw, gh := float32(w)/s, float32(h)/s
	for corner, boxes := range g.pvpBoxes(time.Now()) {
		y := float32(2)
		if corner >= 2 {
			y = gh - 2
		}
		for _, b := range boxes {
			x := float32(2)
			if corner%2 == 1 {
				x = gw - 2 - b.w
			}
			if corner >= 2 {
				y -= b.h
				b.draw(x, y)
				y -= 2
			} else {
				b.draw(x, y)
				y += b.h + 2
			}
		}
	}
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
}

// keystrokesBox is the movement keys, the buttons (with their clicks a second) and the space bar, lit while held.
func (g *game) keystrokesBox(l, r int, cps bool) hudBox {
	const k, gap = 22, 2
	return hudBox{3*k + 2*gap, 3*(k+gap) + 12, func(x, y float32) {
		type key struct {
			x, y, w, h float32
			action     string
			lines      []string
		}
		name := func(a string) string { return trimName(keyName(g.opts.Keys[a]), 4) }
		mouse := func(n string, c int) []string {
			if cps {
				return []string{n, fmt.Sprintf("%d CPS", c)}
			}
			return []string{n}
		}
		keys := []key{
			{k + gap, 0, k, k, "forward", []string{name("forward")}},
			{0, k + gap, k, k, "left", []string{name("left")}},
			{k + gap, k + gap, k, k, "back", []string{name("back")}},
			{2 * (k + gap), k + gap, k, k, "right", []string{name("right")}},
			{0, 2 * (k + gap), (3*k + gap) / 2, k, "attack", mouse("LMB", l)},
			{(3*k+gap)/2 + gap, 2 * (k + gap), (3*k + gap) / 2, k, "use", mouse("RMB", r)},
			{0, 3 * (k + gap), 3*k + 2*gap, 10, "jump", nil},
		}
		for _, c := range keys {
			col := pvpBack
			if g.keyDown(c.action) {
				col = pvpLit
			}
			g.fill(x+c.x, y+c.y, c.w, c.h, col)
		}
		g.flushFill()
		for _, c := range keys {
			txt := pvpWhite
			if g.keyDown(c.action) {
				txt = pvpBlack
			}
			if c.action == "jump" { // the space bar: a bar across it
				g.fill(x+c.x+c.w/2-12, y+c.y+4, 24, 2, txt)
				g.flushFill()
				continue
			}
			top := y + c.y + (c.h-float32(len(c.lines))*9+1)/2
			for i, line := range c.lines {
				g.label(line, x+c.x+(c.w-float32(g.textWidth(line)))/2, top+float32(i)*9, txt, txt == pvpWhite)
			}
		}
	}}
}

// potionIcon is where an effect's picture is in inventory.png: Potion.setIconIndex(x, y), 18 pixels a cell from
// (0, 198). The instant effects and Saturation have none.
var potionIcon = map[int]int{1: 0, 2: 1, 3: 2, 4: 3, 5: 4, 8: 10, 9: 11, 10: 7, 11: 14, 12: 15, 13: 16, 14: 8, 15: 13, 16: 12, 17: 9, 18: 5, 19: 6, 20: 17, 21: 18, 22: 18}

var potionNames = map[int]string{1: "Speed", 2: "Slowness", 3: "Haste", 4: "Mining Fatigue", 5: "Strength", 6: "Instant Health", 7: "Instant Damage",
	8: "Jump Boost", 9: "Nausea", 10: "Regeneration", 11: "Resistance", 12: "Fire Resistance", 13: "Water Breathing", 14: "Invisibility",
	15: "Blindness", 16: "Night Vision", 17: "Hunger", 18: "Weakness", 19: "Poison", 20: "Wither", 21: "Health Boost", 22: "Absorption", 23: "Saturation"}

// potionLine is an effect as the inventory names it (InventoryEffectRenderer: II, III and IV after the name) and
// its time left as Potion.getDurationString writes it.
func potionLine(id int, e potionEffect) (name, left string) {
	name = potionNames[id]
	if name == "" {
		name = fmt.Sprintf("Effect %d", id)
	}
	if e.amp >= 1 && e.amp <= 3 {
		name += [4]string{"", " II", " III", " IV"}[e.amp]
	}
	if e.max {
		return name, "**:**"
	}
	sec := e.ticks / 20
	return name, fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func (g *game) potionsBox() hudBox {
	ids := make([]int, 0, len(g.effects))
	for id := range g.effects {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	w := float32(0)
	for _, id := range ids {
		n, t := potionLine(id, g.effects[id])
		w = max(w, float32(max(g.textWidth(n), g.textWidth(t))))
	}
	return hudBox{w + 25, float32(len(ids)) * 20, func(x, y float32) {
		g.fill(x, y, w+25, float32(len(ids))*20, pvpBack)
		g.flushFill()
		s := float32(g.scaleFor(g.fb[0], g.fb[1]))
		for i, id := range ids {
			if icon, ok := potionIcon[id]; ok {
				g.hud.rect((x+1)*s, (y+1+float32(i)*20)*s, 18*s, 18*s, float32(icon%8*18), float32(198+icon/8*18), 18, 18, 256, 256, pvpWhite)
			}
		}
		g.hud.flush(g.hud.invTex)
		for i, id := range ids {
			n, t := potionLine(id, g.effects[id])
			g.label(n, x+22, y+2+float32(i)*20, pvpWhite, true)
			g.label(t, x+22, y+11+float32(i)*20, pvpGrey, true)
		}
	}}
}

// durabilityColour is green with most of an item's durability left, yellow with some, red near breaking.
func durabilityColour(left, of int) [4]float32 {
	switch f := float32(left) / float32(max(of, 1)); {
	case f > 0.6:
		return [4]float32{0.33, 1, 0.33, 1}
	case f > 0.3:
		return [4]float32{1, 1, 0.33, 1}
	}
	return [4]float32{1, 0.33, 0.33, 1}
}

// armorBox is the helmet down to the boots, and the item in hand when it wears, each with the durability left.
func (g *game) armorBox() (hudBox, bool) {
	var items []slot
	for _, a := range g.armor {
		if !a.empty() {
			items = append(items, a)
		}
	}
	if h := g.held(); !h.empty() && h.durability() > 0 {
		items = append(items, h)
	}
	if len(items) == 0 {
		return hudBox{}, false
	}
	text := func(s slot) string {
		if s.durability() <= 0 {
			return ""
		}
		return fmt.Sprint(s.durability() - int(s.Dmg))
	}
	w := float32(0)
	for _, s := range items {
		w = max(w, float32(g.textWidth(text(s))))
	}
	return hudBox{w + 20, float32(len(items)) * 16, func(x, y float32) {
		s := float32(g.scaleFor(g.fb[0], g.fb[1]))
		for i, it := range items {
			g.drawSlotIcon(it, x*s, (y+float32(i)*16)*s, 16*s)
		}
		g.hud.flush(g.tex)
		g.hud.flushItems()
		for i, it := range items {
			if t := text(it); t != "" {
				g.label(t, x+18, y+4+float32(i)*16, durabilityColour(it.durability()-int(it.Dmg), it.durability()), true)
			}
		}
	}}, true
}
