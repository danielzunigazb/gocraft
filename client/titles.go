package client

// What a server writes over the screen: titles (Title packet) and the action bar (a chat message of position 2),
// drawn as GuiIngame.renderTitle and the record-playing overlay do.

type titleState struct {
	title, subtitle       []styledRun
	fadeIn, stay, fadeOut int // ticks, 10 / 70 / 20 unless the server says
	timer                 int // ticks left of the title
	action                []styledRun
	actionTicks           int
}

func (t *titleState) reset() { t.fadeIn, t.stay, t.fadeOut = 10, 70, 20 }

// netTitle applies a Title packet: 0 title, 1 subtitle, 2 times, 3 clear, 4 reset.
func (g *game) netTitle(action int32, text []styledRun, in, stay, out int) {
	t := &g.titles
	if t.fadeIn == 0 && t.stay == 0 && t.fadeOut == 0 {
		t.reset()
	}
	switch action {
	case 0:
		t.title, t.timer = text, t.fadeIn+t.stay+t.fadeOut
	case 1:
		t.subtitle = text
	case 2:
		t.fadeIn, t.stay, t.fadeOut = in, stay, out
		if t.timer > 0 { // the new times count for the title on screen
			t.timer = t.fadeIn + t.stay + t.fadeOut
		}
	case 3:
		t.title, t.subtitle, t.timer = nil, nil, 0
	case 4:
		t.title, t.subtitle, t.timer = nil, nil, 0
		t.reset()
	}
}

// titleAlpha is how opaque the title is, 0 to 255, partial ticks into its life.
func (t *titleState) alpha(partial float32) int {
	if t.timer <= 0 {
		return 0
	}
	f := float32(t.timer) - partial
	a := float32(255)
	switch {
	case t.timer > t.fadeOut+t.stay: // fading in
		a = (float32(t.fadeIn+t.stay+t.fadeOut) - f) * 255 / float32(max(t.fadeIn, 1))
	case t.timer <= t.fadeOut: // fading out
		a = f * 255 / float32(max(t.fadeOut, 1))
	}
	return int(min(max(a, 0), 255))
}

func (g *game) tickTitles() {
	if g.titles.timer > 0 {
		g.titles.timer--
	}
	if g.titles.actionTicks > 0 {
		g.titles.actionTicks--
	}
}

func (g *game) drawTitles(w, h int) {
	t := &g.titles
	if g.net == nil || (t.timer <= 0 && t.actionTicks <= 0) {
		return
	}
	g.enableUI()
	s := float32(g.scaleFor(w, h))
	if a := t.alpha(g.partial); a > 8 && t.title != nil {
		af := float32(a) / 255
		tw := float32(g.runsWidth(t.title)) * 4 * s
		g.drawRuns(t.title, float32(w)/2-tw/2, float32(h)/2-10*4*s, 4*s, af, true)
		if t.subtitle != nil {
			sw := float32(g.runsWidth(t.subtitle)) * 2 * s
			g.drawRuns(t.subtitle, float32(w)/2-sw/2, float32(h)/2+5*2*s, 2*s, af, true)
		}
	}
	if t.actionTicks > 0 && t.action != nil {
		f := float32(t.actionTicks) - g.partial
		a := min(int(f*256/20), 255)
		if a > 8 {
			aw := float32(g.runsWidth(t.action)) * s
			g.drawRuns(t.action, float32(w)/2-aw/2, float32(h)-68*s-4*s, s, float32(a)/255, true)
		}
	}
}
