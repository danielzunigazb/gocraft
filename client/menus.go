package client

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/go-gl/glfw/v3.3/glfw"

	"gocraft/anvil"
)

type worldEntry struct {
	dir, name string
	mode      int32
	generated bool // made by the generator, not read from the 1.8.9 saves folder
}

var modeNames = [4]string{"Survival Mode", "Creative Mode", "Adventure Mode", "Spectator Mode"}

// scanSaves lists the worlds in a saves folder that have a level.dat, newest first.
func scanSaves(dir string) []worldEntry {
	entries, _ := os.ReadDir(dir)
	type timed struct {
		e worldEntry
		t int64
	}
	var found []timed
	for _, d := range entries {
		path := filepath.Join(dir, d.Name())
		st, err := os.Stat(filepath.Join(path, "level.dat"))
		if err != nil {
			continue
		}
		save, err := anvil.Open(path)
		if err != nil {
			continue
		}
		name := save.Level.Name
		if name == "" {
			name = d.Name()
		}
		found = append(found, timed{worldEntry{dir: path, name: name, mode: save.Level.GameType}, st.ModTime().UnixNano()})
		save.Close()
	}
	sort.Slice(found, func(i, j int) bool { return found[i].t > found[j].t })
	out := make([]worldEntry, len(found))
	for i, f := range found {
		out[i] = f.e
	}
	return out
}

// drawMenus draws the title, world list or pause screen and acts on clicks.
func (g *game) drawMenus() {
	if !g.screen.isMenu() {
		return
	}
	g.enableUI()
	hd := &g.hud
	w, h := g.fb[0], g.fb[1]
	s := float32(g.scaleFor(w, h))
	cx := float32(w) / 2
	col := [4]float32{1, 1, 1, 1}
	left := (float32(w)/s - 200) / 2 // a 200-wide button's left edge in GUI pixels

	switch g.screen {
	case screenTitle:
		g.dirtBackground(0.45)
		if hd.logo != 0 {
			x := cx - 137*s
			hd.rect(x, 30*s, 155*s, 44*s, 0, 0, 155, 44, 256, 256, col)
			hd.rect(x+155*s, 30*s, 119*s, 44*s, 0, 45, 119, 44, 256, 256, col)
			hd.flush(hd.logo)
		}
		top := float32(h)/s/4 + 48
		if g.button("Singleplayer", left, top, 200, true) {
			g.worldList = scanWorlds(g.saves)
			g.worldSel = 0
			g.screen = screenWorlds
		}
		if g.button("Multiplayer", left, top+24, 200, true) {
			g.multiAddr, g.multiName = g.opts.Server, g.opts.PlayerName
			if g.multiAddr == "" {
				g.multiAddr = "localhost:25565"
			}
			if g.multiName == "" {
				g.multiName = "Player"
			}
			g.screen = screenMulti
			g.netMsg = ""
		}
		if g.button("Options...", left, top+72, 98, true) {
			g.openOptions()
		}
		if g.button("Quit Game", left+102, top+72, 98, true) {
			g.win.SetShouldClose(true)
		}
		g.text("Minecraft 1.8.9 in Go", 4*s, float32(h)-12*s, s, col, true)

	case screenWorlds:
		g.dirtBackground(0.45)
		g.centredText("Select World", cx, 20*s, s, col)
		mx, my := g.mouse()
		for i, e := range g.worldList {
			if i >= 6 {
				break
			}
			y := (float32(32 + i*36)) * s
			x := left*s - 20*s
			wd := 240 * s
			hover := mx >= x && mx < x+wd && my >= y && my < y+34*s
			if hover && g.uiClick {
				g.worldSel = i
			}
			if i == g.worldSel {
				hd.rect(x-s, y-s, wd+2*s, 36*s, 0, 0, 1, 1, 1, 1, [4]float32{0.75, 0.75, 0.75, 1})
				hd.rect(x, y, wd, 34*s, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 1})
				hd.flush(hd.white)
			}
			g.text(trimName(e.name, 28), x+4*s, y+2*s, s, col, true)
			folder := filepath.Base(e.dir)
			if e.generated {
				folder = "Generated world"
			}
			g.text(trimName(folder, 28), x+4*s, y+12*s, s, [4]float32{0.5, 0.5, 0.5, 1}, true)
			g.text(modeNames[e.mode&3], x+4*s, y+22*s, s, [4]float32{0.5, 0.5, 0.5, 1}, true)
		}
		if len(g.worldList) == 0 {
			g.centredText("No worlds found in "+trimName(g.saves, 40), cx, 60*s, s, [4]float32{0.63, 0.63, 0.63, 1})
		}
		bottom := float32(h)/s - 28
		if g.button("Play Selected World", left-52, bottom-24, 150, len(g.worldList) > 0) {
			if err := g.loadWorld(g.worldList[g.worldSel].dir); err != nil {
				g.worldList = nil
			}
		}
		if g.button("Create New World", left+102, bottom-24, 150, true) {
			g.openNewWorld()
		}
		if g.button("Cancel", left, bottom, 200, true) {
			g.screen = screenTitle
		}

	case screenNewWorld:
		g.drawNewWorld(cx, s)

	case screenMulti:
		g.drawMulti(cx, s)

	case screenOptions:
		g.drawOptions(float32(w)/s/2, s)

	case screenControls:
		g.drawControls(float32(w)/s/2, s)

	case screenPacks:
		g.drawPacks(float32(w)/s/2, s)

	case screenPause:
		hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 0.6})
		hd.flush(hd.white)
		g.centredText("Game Menu", cx, 40*s, s, col)
		top := float32(h)/s/4 + 24
		if g.button("Back to Game", left, top, 200, true) {
			g.closeScreen()
		}
		if g.button("Options...", left, top+24, 200, true) {
			g.openOptions()
		}
		quit := "Save and Quit to Title"
		if g.net != nil {
			quit = "Disconnect"
		}
		g.drawExportButton(left, top+48, s)
		if g.button(quit, left, top+72, 200, true) {
			g.closeWorld()
			g.screen = screenTitle
			g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
		}
	}
	g.uiClick = false
}
