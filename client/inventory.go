package client

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sort"
)

type screenKind int

const (
	screenNone screenKind = iota
	screenInventory
	screenContainer
	screenServerWin // a window the server opened: a menu, a chest
	screenDead      // the player died on a server: Respawn
	screenSignEdit  // the editor of a sign a server asked to be edited
	screenChat
	screenTitle
	screenWorlds
	screenPause
	screenOptions
	screenControls
	screenPacks
	screenMulti
)

// isMenu is a screen drawn by the menu code, where the world does not tick.
func (k screenKind) isMenu() bool { return k >= screenTitle }

// The creative inventory is laid out as in 1.8: a 195 x 136 panel with 9 x 5 item slots at (9, 18),
// the hotbar at (9, 112) and a scroll bar at (175, 18).
const (
	invW, invH   = 195, 136
	invCols      = 9
	invRows      = 5
	invGridX     = 9
	invGridY     = 18
	invHotbarY   = 112
	invScrollX   = 175
	invScrollH   = 112
	invScrollerW = 12
	invScrollerH = 15
	slotSize     = 18
)

// metaVariants is how many metadata values of a block are different items in the creative lists.
var metaVariants = map[uint16]int{1: 7, 3: 3, 5: 6, 6: 6, 12: 2, 17: 4, 18: 4, 24: 3, 35: 16, 38: 9, 43: 8, 44: 8, 95: 16,
	97: 6, 98: 4, 126: 6, 155: 3, 159: 16, 160: 16, 161: 2, 162: 2, 168: 3, 171: 16, 175: 6, 179: 3, 125: 6}

// notItems are blocks that exist in the world but are not placed from an inventory.
var notItems = map[uint16]bool{26: true, 34: true, 36: true, 51: true, 55: true, 59: true, 60: true, 62: true, 63: true, 64: true, 68: true,
	71: true, 74: true, 75: true, 83: true, 90: true, 92: true, 93: true, 94: true, 104: true, 105: true, 115: true, 117: true, 118: true,
	119: true, 124: true, 127: true, 132: true, 140: true, 141: true, 142: true, 144: true, 149: true, 150: true, 176: true, 177: true,
	178: true, 181: true, 193: true, 194: true, 195: true, 196: true, 197: true, 125: true}

// creativeItems lists every block the game can draw, in id order with its variants, then the items.
func (g *game) creativeItems() []slot {
	if g.creativeList != nil {
		return g.creativeList
	}
	none := func(int32, int32, int32) uint16 { return 0 }
	list := []slot{}
	for id := uint16(1); id < 256; id++ {
		if notItems[id] || isFluid(id) {
			continue
		}
		n := max(metaVariants[id], 1)
		for meta := 0; meta < n; meta++ {
			file, props := g.res.stateOf(id, meta, none)
			if len(g.res.pick(file, props)) > 0 {
				list = append(list, slot{id, uint16(meta), 1})
			}
		}
	}
	var ids []int
	for id := range itemDefs {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		variants := map[uint16]int{263: 2, 351: 16, 349: 4, 350: 2}[uint16(id)]
		for d := 0; d < max(variants, 1); d++ {
			if st := (slot{uint16(id), uint16(d), 1}); func() bool { _, ok := g.res.itemTile(st); return ok }() {
				list = append(list, st)
			}
		}
	}
	g.creativeList = list
	return list
}

// creativeStack is what clicking an item in the creative lists gives: a full stack.
func creativeStack(s slot) slot {
	s.N = s.maxStack()
	return s
}

func (g *game) openScreen(k screenKind) {
	g.screen = k
	if g.win != nil {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
}

func (g *game) closeScreen() {
	g.screen = screenNone
	g.haveCursor = false
	if g.opt.Bench == 0 && g.win != nil {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
	}
}

// panel is where the inventory sits on screen, in framebuffer pixels.
func (g *game) panel() (s, left, top float32) {
	w, h := g.fb[0], g.fb[1]
	s = float32(g.scaleFor(w, h))
	return s, (float32(w) - invW*s) / 2, (float32(h) - invH*s) / 2
}

func (g *game) maxScroll() int {
	rows := (len(g.creativeShown()) + invCols - 1) / invCols
	return max(rows-invRows, 0)
}

// slotAt is what is under the cursor: an item of the grid (kind 'g', index into the list) or a hotbar slot ('h').
func (g *game) slotAt() (kind byte, i int) {
	s, left, top := g.panel()
	fbScale := float64(g.fb[0]) / float64(max(g.winSize(), 1))
	mx, my := float32(g.cx*fbScale), float32(g.cy*fbScale)
	x, y := (mx-left)/s, (my-top)/s
	in := func(sx, sy float32) bool { return x >= sx && x < sx+16+2 && y >= sy && y < sy+16+2 }
	for r := 0; r < invRows; r++ {
		for c := 0; c < invCols; c++ {
			if in(float32(invGridX+c*slotSize)-1, float32(invGridY+r*slotSize)-1) {
				idx := (g.invScroll+r)*invCols + c
				if idx < len(g.creativeShown()) {
					return 'g', idx
				}
				return 0, 0
			}
		}
	}
	for c := 0; c < invCols; c++ {
		if in(float32(invGridX+c*slotSize)-1, invHotbarY-1) {
			return 'h', c
		}
	}
	return 0, 0
}

func (g *game) winSize() int { w, _ := g.win.GetSize(); return w }

func (g *game) winHeight() int { _, h := g.win.GetSize(); return h }

// panelMouse is the mouse in the creative panel's GUI pixels.
func (g *game) panelMouse() (x, y float32) {
	s, left, top := g.panel()
	fbScale := float64(g.fb[0]) / float64(max(g.winSize(), 1))
	return (float32(g.cx*fbScale) - left) / s, (float32(g.cy*fbScale) - top) / s
}

func (g *game) clickInventory() {
	if px, py := g.panelMouse(); true {
		if t := tabAt(px, py); t >= 0 { // a tab: show its items from the top
			g.be.tab, g.invScroll = t, 0
			return
		}
	}
	switch kind, i := g.slotAt(); kind {
	case 'g':
		g.inv[g.sel] = creativeStack(g.creativeShown()[i])
	case 'h':
		g.sel = i
	}
}

// iconQuads queues a block's icon in the atlas texture at x, y, size pixels.
func (g *game) iconQuads(state uint16, x, y, size float32) {
	if g.icon3D(state, x, y, size) { // a cube, as the inventory draws blocks
		return
	}
	tile, tint, ok := g.blockIcon(state)
	if !ok {
		return
	}
	hd := &g.hud
	u0, v0 := g.res.atlas.coord(tile, 0, 0)
	u1, v1 := g.res.atlas.coord(tile, 16, 16)
	q := [4][4]float32{{x, y, u0, v0}, {x, y + size, u0, v1}, {x + size, y + size, u1, v1}, {x + size, y, u1, v0}}
	for _, k := range [6]int{0, 1, 2, 0, 2, 3} {
		hd.verts = append(hd.verts, q[k][0], q[k][1], q[k][2], q[k][3], tint[0], tint[1], tint[2], 1)
	}
}

func (g *game) drawInventory(w, h int) {
	hd := &g.hud
	s, left, top := g.panel()
	one := [4]float32{1, 1, 1, 1}

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.UseProgram(hd.prog)
	gl.Uniform2f(hd.uScreen, float32(w), float32(h))
	gl.ActiveTexture(gl.TEXTURE0)

	// The world dims behind the panel.
	hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0.06, 0.06, 0.75})
	hd.flush(hd.white)

	hd.rect(left, top, invW*s, invH*s, 0, 0, invW, invH, 256, 256, one)
	if g.maxScroll() > 0 {
		sy := float32(invGridY) + float32(g.invScroll)*float32(invScrollH-invScrollerH-2)/float32(g.maxScroll())
		hd.rect(left+invScrollX*s, top+sy*s, invScrollerW*s, invScrollerH*s, 232, 0, invScrollerW, invScrollerH, 256, 256, one)
	} else {
		hd.rect(left+invScrollX*s, top+invGridY*s, invScrollerW*s, invScrollerH*s, 244, 0, invScrollerW, invScrollerH, 256, 256, one)
	}
	hd.flush(hd.tabItems)
	g.drawTabs(left, top, s)
	g.text(creativeTabNames[g.be.tab%creativeTabCount], left+8*s, top+6*s, s, [4]float32{0.25, 0.25, 0.25, 1}, false)

	items := g.creativeShown()
	for r := 0; r < invRows; r++ {
		for c := 0; c < invCols; c++ {
			if idx := (g.invScroll+r)*invCols + c; idx < len(items) {
				g.drawSlotIcon(items[idx], left+float32(invGridX+c*slotSize)*s, top+float32(invGridY+r*slotSize)*s, 16*s)
			}
		}
	}
	for c := 0; c < 9; c++ {
		g.drawSlotIcon(g.inv[c], left+float32(invGridX+c*slotSize)*s, top+invHotbarY*s, 16*s)
	}
	hd.flush(g.tex)
	hd.flushItems()
	for c := 0; c < 9; c++ {
		g.drawCount(int(g.inv[c].N), left+float32(invGridX+c*slotSize)*s, top+invHotbarY*s, s)
	}

	// The slot under the cursor and the selected hotbar slot are lit.
	kind, i := g.slotAt()
	light := func(sx, sy float32, a float32) {
		hd.rect(left+sx*s, top+sy*s, 16*s, 16*s, 0, 0, 1, 1, 1, 1, [4]float32{1, 1, 1, a})
	}
	switch kind {
	case 'g':
		light(float32(invGridX+(i%invCols)*slotSize), float32(invGridY+(i/invCols-g.invScroll)*slotSize), 0.45)
	case 'h':
		light(float32(invGridX+i*slotSize), invHotbarY, 0.45)
	}
	light(float32(invGridX+g.sel*slotSize), invHotbarY, 0.25)
	hd.flush(hd.white)
	// What the mouse points at is named.
	mx, my := float32(g.cx*float64(g.fb[0])/float64(max(g.winSize(), 1))), float32(g.cy*float64(g.fb[1])/float64(max(g.winHeight(), 1)))
	switch {
	case kind == 'g':
		g.drawTooltip(g.displayName(items[i]), mx, my, s)
	case kind == 'h':
		g.drawTooltip(g.displayName(g.inv[i]), mx, my, s)
	default:
		if px, py := g.panelMouse(); tabAt(px, py) >= 0 {
			g.drawTooltip(creativeTabNames[tabAt(px, py)], mx, my, s)
		}
	}
	gl.Disable(gl.BLEND)
}
