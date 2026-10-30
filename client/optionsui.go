package client

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// leaveOptions saves the options and returns to the screen that opened them.
func (g *game) leaveOptions() {
	soundPage = false
	if err := g.opts.save(); err != nil {
		logf("saving options: %v", err)
	}
	g.applyVSync()
	g.screen = g.optionsFrom
}

func (g *game) openOptions() {
	g.optionsFrom = g.screen
	g.screen = screenOptions
}

// slider draws a 0..1 slider and returns its value, following the mouse while it is dragged.
func (g *game) slider(id, label string, v float32, x, y, w float32) float32 {
	hd := &g.hud
	s := float32(g.scaleFor(g.fb[0], g.fb[1]))
	px, py := x*s, y*s
	mx, my := g.mouse()
	inside := mx >= px && mx < px+w*s && my >= py && my < py+20*s
	down := g.win != nil && g.win.GetMouseButton(0) == 1
	switch {
	case !down:
		g.dragging = ""
	case inside && g.uiClick && g.dragging == "":
		g.dragging = id
	}
	if g.dragging == id {
		v = clampf((mx-px-4*s)/((w-8)*s), 0, 1)
	}
	one := [4]float32{1, 1, 1, 1}
	hd.rect(px, py, w/2*s, 20*s, 0, 46, w/2, 20, 256, 256, one)
	hd.rect(px+w/2*s, py, w/2*s, 20*s, 200-w/2, 46, w/2, 20, 256, 256, one)
	hv := float32(66)
	if inside || g.dragging == id {
		hv = 86
	}
	hx := px + v*(w-8)*s
	hd.rect(hx, py, 4*s, 20*s, 0, hv, 4, 20, 256, 256, one)
	hd.rect(hx+4*s, py, 4*s, 20*s, 196, hv, 4, 20, 256, 256, one)
	hd.flush(hd.widgets)
	col := [4]float32{1, 1, 1, 1}
	if inside || g.dragging == id {
		col = [4]float32{1, 1, 0.63, 1}
	}
	g.centredText(label, px+w/2*s, py+6*s, s, col)
	return v
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func brightnessLabel(v float32) string {
	switch {
	case v <= 0:
		return "Moody"
	case v >= 1:
		return "Bright"
	}
	return fmt.Sprintf("%d%%", int(v*100))
}

// soundPage is whether the options screen shows the sound volumes instead of the main options.
var soundPage bool

// drawSoundOptions is the page of volumes: the master volume and one slider per category.
func (g *game) drawSoundOptions(cx, s float32) {
	g.dirtBackground(0.45)
	w, h := float32(g.fb[0]), float32(g.fb[1])
	g.centredText("Music & Sounds", w/2, 15*s, s, [4]float32{1, 1, 1, 1})
	o := &g.opts
	if o.SoundVolume == nil {
		o.SoundVolume = map[string]float32{}
	}
	top := h/s/6 - 12
	vol := func(c string, x, y, width float32) {
		v := g.slider("snd-"+c, fmt.Sprintf("%s: %s", soundCategoryLabel(c), volumeLabel(o.soundVolume(c))), o.soundVolume(c), x, y, width)
		if v != o.soundVolume(c) {
			o.SoundVolume[c] = v
		}
	}
	vol("master", cx-100, top, 200)
	for i, c := range soundCategories[1:] {
		vol(c, cx-155+float32(i%2)*160, top+36+float32(i/2)*24, 150)
	}
	if g.button("Done", cx-100, h/s-28, 200, true) {
		soundPage = false
	}
}

func volumeLabel(v float32) string {
	if v <= 0 {
		return "OFF"
	}
	return fmt.Sprintf("%d%%", int(v*100+0.5))
}

func (g *game) drawOptions(cx, s float32) {
	if soundPage {
		g.drawSoundOptions(cx, s)
		return
	}
	g.dirtBackground(0.45)
	w, h := float32(g.fb[0]), float32(g.fb[1])
	g.centredText("Options", w/2, 15*s, s, [4]float32{1, 1, 1, 1})
	o := &g.opts
	lx, rx := cx-155, cx+5
	top := h/s/6 - 12

	o.FOV = 30 + 80*g.slider("fov", fmt.Sprintf("FOV: %d", int(o.FOV+0.5)), (o.FOV-30)/80, lx, top, 150)
	o.Sensitivity = g.slider("sens", fmt.Sprintf("Sensitivity: %d%%", int(o.Sensitivity*100+0.5)), o.Sensitivity, rx, top, 150)
	rd := g.slider("rd", fmt.Sprintf("Render Distance: %d chunks", o.RenderDistance), float32(o.RenderDistance-2)/14, lx, top+24, 150)
	o.RenderDistance = 2 + int(rd*14+0.5)
	o.Gamma = g.slider("gamma", "Brightness: "+brightnessLabel(o.Gamma), o.Gamma, rx, top+24, 150)

	scale := "Auto"
	if o.GuiScale > 0 {
		scale = fmt.Sprint(o.GuiScale)
	}
	if g.button("GUI Scale: "+scale, lx, top+48, 150, true) {
		o.GuiScale = (o.GuiScale + 1) % 5
	}
	if g.button("VSync: "+onOff(o.VSync), rx, top+48, 150, true) {
		o.VSync = !o.VSync
		g.applyVSync()
	}
	if g.button("Invert Mouse: "+onOff(o.InvertY), lx, top+72, 150, true) {
		o.InvertY = !o.InvertY
	}
	if g.button("Graphics: "+map[bool]string{true: "Fancy", false: "Fast"}[o.Fancy], lx, top+96, 150, true) {
		o.Fancy = !o.Fancy
		if g.stream != nil {
			g.remeshAll()
		}
	}
	if g.button("Dynamic Lights: "+onOff(o.DynamicLights), rx, top+72, 150, true) {
		o.DynamicLights = !o.DynamicLights
	}
	if g.button("Music & Sounds...", rx, top+96, 150, true) {
		soundPage = true
	}
	if g.button("Controls...", lx, top+120, 150, true) {
		g.screen = screenControls
	}
	if g.button("Resource Packs...", rx, top+120, 150, true) {
		g.packList = listPacks(g.opt.PacksDir)
		g.screen = screenPacks
	}
	if g.button("Done", cx-100, h/s-28, 200, true) {
		g.leaveOptions()
	}
}

func (g *game) drawControls(cx, s float32) {
	g.dirtBackground(0.45)
	w, h := float32(g.fb[0]), float32(g.fb[1])
	g.centredText("Controls", w/2, 15*s, s, [4]float32{1, 1, 1, 1})
	for i, a := range keyActions {
		col, row := i/6, i%6
		x := cx - 155 + float32(col)*160
		y := float32(36 + row*24)
		g.text(trimName(a.label, 14), x*s, (y+6)*s, s, [4]float32{1, 1, 1, 1}, true)
		name := keyName(g.opts.Keys[a.id])
		if g.rebinding == a.id {
			name = "> " + name + " <"
		}
		if g.button(name, x+78, y, 72, true) {
			g.rebinding = a.id
		}
	}
	if g.button("Reset Keys", cx-100, h/s-52, 200, true) {
		for _, a := range keyActions {
			g.opts.Keys[a.id] = a.def
		}
	}
	if g.button("Done", cx-100, h/s-28, 200, true) {
		g.rebinding = ""
		g.screen = screenOptions
	}
}

// listPacks are the resource packs found in a folder: zip files.
func listPacks(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// drawPacks is the resource pack screen: a click turns a pack on or off, the arrows move an enabled
// pack up or down in the order they are laid over the game's own pictures.
func (g *game) drawPacks(cx, s float32) {
	g.dirtBackground(0.45)
	w, h := float32(g.fb[0]), float32(g.fb[1])
	g.centredText("Resource Packs", w/2, 15*s, s, [4]float32{1, 1, 1, 1})
	g.centredText("The pack on top is drawn over the ones below it", w/2, 28*s, s, [4]float32{0.63, 0.63, 0.63, 1})
	// Enabled packs first, highest priority at the top, then the ones that are off.
	var rows []string
	for i := len(g.opts.Packs) - 1; i >= 0; i-- {
		rows = append(rows, g.opts.Packs[i])
	}
	for _, p := range g.packList {
		if indexOf(g.opts.Packs, p) < 0 {
			rows = append(rows, p)
		}
	}
	changed := false
	for i, name := range rows {
		if i >= 8 {
			break
		}
		y := float32(44 + i*22)
		on := indexOf(g.opts.Packs, name) >= 0
		label := name
		if !on {
			label = "[off] " + name
		}
		if g.button(trimName(label, 34), cx-120, y, 200, true) {
			if on {
				g.opts.Packs = append(g.opts.Packs[:indexOf(g.opts.Packs, name)], g.opts.Packs[indexOf(g.opts.Packs, name)+1:]...)
			} else {
				g.opts.Packs = append(g.opts.Packs, name)
			}
			changed = true
		}
		if on {
			k := indexOf(g.opts.Packs, name)
			if g.button("^", cx+84, y, 16, k < len(g.opts.Packs)-1) { // up means later in the list
				g.opts.Packs[k], g.opts.Packs[k+1] = g.opts.Packs[k+1], g.opts.Packs[k]
				changed = true
			}
			if g.button("v", cx+104, y, 16, k > 0) {
				g.opts.Packs[k], g.opts.Packs[k-1] = g.opts.Packs[k-1], g.opts.Packs[k]
				changed = true
			}
		}
	}
	if len(rows) == 0 {
		g.centredText("No packs in "+trimName(g.opt.PacksDir, 40), w/2, 60*s, s, [4]float32{0.63, 0.63, 0.63, 1})
	}
	if changed {
		g.packsDirty = true
	}
	if g.button("Done", cx-100, h/s-28, 200, true) {
		g.leavePacks()
	}
}

// leavePacks applies the choice if it changed and returns to the options.
func (g *game) leavePacks() {
	if g.packsDirty {
		g.packsDirty = false
		if err := g.reloadResources(); err != nil {
			logf("reloading resources: %v", err)
		}
	}
	g.screen = screenOptions
}
