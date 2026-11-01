package client

// The boss bar, after GuiIngame.renderBossHealth: when a Wither or an Ender Dragon is near, a bar of 182 pixels
// near the top of the screen shows its name and how much of its health is left. Plugins that put a message
// there (BarAPI and its kind) spawn a Wither or a Dragon out of sight and name it, so the bar is all that shows.

const (
	mobEnderDragon = 63
	mobWither      = 64
)

// bossBar is the boss the bar shows, its name and how full it is, if any. The newest boss in range wins, as
// the last one drawn does in the game.
func (n *netState) bossBar() (name []styledRun, frac float32, ok bool) {
	var best *netEntity
	for _, e := range n.ents {
		if (e.kind == mobEnderDragon || e.kind == mobWither) && e.obj == 0 && (best == nil || e.eid > best.eid) {
			best = e
		}
	}
	if best == nil {
		return nil, 0, false
	}
	name = best.name
	max := float32(300)
	if best.kind == mobEnderDragon {
		max = 200
	}
	if len(name) == 0 {
		name = legacyRuns(map[byte]string{mobEnderDragon: "Ender Dragon", mobWither: "Wither"}[best.kind], defaultStyle)
	}
	frac = 1
	if best.healthSet {
		frac = min(max0(best.health/max), 1)
	}
	return name, frac, true
}

func max0(v float32) float32 {
	if v < 0 {
		return 0
	}
	return v
}

// drawBossBar draws the name at the top and the bar under it: the empty bar of icons.png and the full one over
// it as far as the health goes (int(health * 183) pixels).
func (g *game) drawBossBar(w, h int) {
	if g.net == nil {
		return
	}
	name, frac, ok := g.net.bossBar()
	if !ok {
		return
	}
	g.enableUI()
	s := g.scaleFor(w, h)
	fs := float32(s)
	sw := float32((w + s - 1) / s)
	hd := &g.hud
	const barW, barY = 182, 12
	x := sw/2 - barW/2
	hd.rect(x*fs, barY*fs, barW*fs, 5*fs, 0, 74, barW, 5, 256, 256, [4]float32{1, 1, 1, 1})
	if l := int(frac * (barW + 1)); l > 0 {
		hd.rect(x*fs, barY*fs, float32(l)*fs, 5*fs, 0, 79, float32(l), 5, 256, 256, [4]float32{1, 1, 1, 1})
	}
	hd.flush(hd.icons)
	g.drawRuns(name, (sw/2-float32(g.runsWidth(name))/2)*fs, (barY-10)*fs, fs, 1, true)
}
