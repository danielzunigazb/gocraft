package client

import (
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"

	"gocraft/world"
)

// The windows of chests and furnaces: their slots and rules, the GUI pictures, opening them, the chest
// itself as a model, and the facing a placed chest or furnace gets.

// guiTexture is a texture of the interface read from the jar, uploaded the first time it is wanted and
// again after the resources are reloaded.
func (g *game) guiTexture(name string) uint32 {
	if g.be.texOf != g.res {
		for _, t := range g.be.tex {
			gl.DeleteTextures(1, &t)
		}
		g.be.tex, g.be.texOf = map[string]uint32{}, g.res
	}
	if t, ok := g.be.tex[name]; ok {
		return t
	}
	t := uploadTexture(g.res.gui[name])
	g.be.tex[name] = t
	return t
}

// contHeight is how tall the open window is, in GUI pixels.
func (g *game) contHeight() int {
	if g.cont.kind == contChest {
		return 114 + g.cont.rows*18
	}
	return contH
}

// beSlots are the slots of a chest or furnace window, then the player's own, in GUI pixels.
func (g *game) beSlots() []placedSlot {
	var out []placedSlot
	invY, hotY := 84, 142
	switch g.cont.kind {
	case contChest:
		for i := 0; i < g.cont.rows*9; i++ {
			var p *slot
			if i < 27 {
				p = &g.cont.be.items[i]
			} else {
				p = &g.cont.be2.items[i-27]
			}
			out = append(out, placedSlot{8 + 18*(i%9), 18 + 18*(i/9), slotRef{p, kBE, i}})
		}
		invY, hotY = 103+(g.cont.rows-4)*18, 161+(g.cont.rows-4)*18
	case contFurnace:
		it := g.cont.be.items
		out = append(out, placedSlot{56, 17, slotRef{&it[0], kBE, 0}}, placedSlot{56, 53, slotRef{&it[1], kBE, 1}}, placedSlot{116, 35, slotRef{&it[2], kBE, 2}})
	}
	for r := 0; r < 3; r++ {
		for c := 0; c < 9; c++ {
			out = append(out, placedSlot{8 + 18*c, invY + 18*r, slotRef{&g.inv[9+r*9+c], kInv, 9 + r*9 + c}})
		}
	}
	for c := 0; c < 9; c++ {
		out = append(out, placedSlot{8 + 18*c, hotY, slotRef{&g.inv[c], kInv, c}})
	}
	return out
}

// beOutput is the furnace's result slot, which can be taken from but not put into.
func (g *game) beOutput(ref slotRef) bool {
	return ref.kind == kBE && g.cont.kind == contFurnace && ref.index == 2
}

// takeOutput is a click on the furnace's result: the stack goes to the mouse if it fits.
func (g *game) takeOutput(ref slotRef, shift bool) {
	s := ref.p
	if s.empty() {
		return
	}
	if shift {
		rest := g.mergeInto(*s, 9, 36)
		rest = g.mergeInto(rest, 0, 9)
		*s = rest
		return
	}
	cur := &g.cursor
	switch {
	case cur.empty():
		*cur, *s = *s, slot{}
	case cur.same(*s) && int(cur.N)+int(s.N) <= int(cur.maxStack()):
		cur.N += s.N
		*s = slot{}
	}
}

// quickMoveBE is shift-clicking in a chest or furnace window.
func (g *game) quickMoveBE(ref slotRef) {
	s := *ref.p
	if s.empty() {
		return
	}
	if ref.kind == kBE { // out of the chest or furnace into the inventory
		rest := g.mergeInto(s, 9, 36)
		rest = g.mergeInto(rest, 0, 9)
		*ref.p = rest
		return
	}
	it := g.cont.be.items
	var rest slot
	switch g.cont.kind {
	case contFurnace:
		_, smeltable := smeltResult(s)
		switch {
		case smeltable && (it[0].empty() || it[0].same(s) && it[0].N < it[0].maxStack()):
			rest = mergeInto1(&it[0], s)
		case fuelTime(s) > 0 && (it[1].empty() || it[1].same(s) && it[1].N < it[1].maxStack()):
			rest = mergeInto1(&it[1], s)
		case ref.index < 9:
			rest = g.mergeInto(s, 9, 36)
		default:
			rest = g.mergeInto(s, 0, 9)
		}
	default: // a chest takes it
		rest = s
		for i := 0; i < g.cont.rows*9 && !rest.empty(); i++ {
			rest = mergeInto1(g.cont.beSlot(i), rest)
		}
	}
	*ref.p = rest
}

// mergeInto1 adds a stack into one slot and returns what is left.
func mergeInto1(in *slot, s slot) slot {
	switch {
	case in.empty():
		n := min(s.N, s.maxStack())
		*in, s.N = slot{s.ID, s.Dmg, n}, s.N-n
	case in.same(s):
		n := min(s.N, in.maxStack()-in.N)
		in.N, s.N = in.N+n, s.N-n
	}
	if s.N <= 0 {
		return slot{}
	}
	return s
}

func (c *containerState) beSlot(i int) *slot {
	if i < 27 {
		return &c.be.items[i]
	}
	return &c.be2.items[i-27]
}

// ---- opening ----

// chestPartner is the chest next to this one that makes a double chest with it, and whether this one
// is the west or north half (the first 27 slots of the window).
func (g *game) chestPartner(p [3]int32) (partner [3]int32, first, ok bool) {
	id := g.world.Block(p[0], p[1], p[2]) >> 4
	for _, d := range [][3]int32{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}} {
		q := [3]int32{p[0] + d[0], p[1], p[2] + d[2]}
		if g.world.Block(q[0], q[1], q[2])>>4 == id {
			return q, d[0] > 0 || d[2] > 0, true // the partner to the east or south: this is the first half
		}
	}
	return p, false, false
}

// openBlockEntity opens the chest or furnace at p, and says whether the click was for it.
func (g *game) openBlockEntity(p [3]int32) bool {
	if g.net != nil || g.opt.Bench > 0 {
		return false
	}
	id := g.world.Block(p[0], p[1], p[2]) >> 4
	switch {
	case isChestID(id):
		if g.res.opaque[g.world.Block(p[0], p[1]+1, p[2])>>4&255] { // a block above shuts it
			return true
		}
		be := g.beAt(p, true)
		c := containerState{kind: contChest, be: be, rows: 3, pos: p}
		if q, first, ok := g.chestPartner(p); ok && !g.res.opaque[g.world.Block(q[0], q[1]+1, q[2])>>4&255] {
			be2 := g.beAt(q, true)
			c.rows, c.be2 = 6, be2
			if first { // the other chest is the east or south half
				c.be, c.be2 = be, be2
			} else {
				c.be, c.be2 = be2, be
			}
		}
		g.cont = c
	case isFurnaceID(id):
		g.cont = containerState{kind: contFurnace, be: g.beAt(p, true), pos: p}
	default:
		return false
	}
	g.openScreen(screenContainer)
	return true
}

// ---- drawing ----

// drawBEBackground draws the picture of a chest or furnace window and the labels, and returns the
// texture of the part drawn so the slots go over it.
func (g *game) drawBEBackground(left, top, s float32) {
	hd := &g.hud
	one := [4]float32{1, 1, 1, 1}
	switch g.cont.kind {
	case contChest:
		rows := float32(g.cont.rows)
		hd.rect(left, top, contW*s, (rows*18+17)*s, 0, 0, contW, rows*18+17, 256, 256, one)
		hd.rect(left, top+(rows*18+17)*s, contW*s, 96*s, 0, 126, contW, 96, 256, 256, one)
		hd.flush(g.guiTexture("generic_54"))
		title := "Chest"
		if g.cont.rows == 6 {
			title = "Large Chest"
		}
		g.text(title, left+8*s, top+6*s, s, [4]float32{0.25, 0.25, 0.25, 1}, false)
		g.text("Inventory", left+8*s, top+(rows*18+14-0)*s+4*s, s, [4]float32{0.25, 0.25, 0.25, 1}, false)
	case contFurnace:
		be := g.cont.be
		hd.rect(left, top, contW*s, contH*s, 0, 0, contW, contH, 256, 256, one)
		if be.burning() { // the flame shrinks as the fuel burns
			total := max(be.burnTotal, furnaceCookTime)
			k := float32(be.burn * 13 / total)
			hd.rect(left+56*s, top+(36+12-k)*s, 14*s, (k+1)*s, 176, 12-k, 14, k+1, 256, 256, one)
		}
		l := float32(be.cook * 24 / max(be.cookTotal, 1)) // the arrow fills from the left
		hd.rect(left+79*s, top+34*s, (l+1)*s, 16*s, 176, 14, l+1, 16, 256, 256, one)
		hd.flush(g.guiTexture("furnace"))
		g.text("Furnace", left+(contW/2-float32(g.textWidth("Furnace"))/2)*s, top+6*s, s, [4]float32{0.25, 0.25, 0.25, 1}, false)
		g.text("Inventory", left+8*s, top+72*s, s, [4]float32{0.25, 0.25, 0.25, 1}, false)
	}
}

// ---- what a placed block faces ----

// chestFacing is the metadata of a block that faces the player who placed it (a chest, furnace or
// dispenser): the player looks south (yaw 0) and the block faces north.
func chestFacing(yawDeg float32) uint16 {
	facing := int(math.Floor(float64(yawDeg)/90+0.5)) & 3 // 0 south, 1 west, 2 north, 3 east
	return [4]uint16{2, 5, 3, 4}[facing]
}

// afterPlace is what a placed block does to its neighbours: two chests side by side face the same way.
func (g *game) afterPlace(x, y, z int32) {
	st := g.world.Block(x, y, z)
	if !isChestID(st >> 4) {
		return
	}
	for _, d := range [][3]int32{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}} {
		nx, nz := x+d[0], z+d[2]
		n := g.world.Block(nx, y, nz)
		if n>>4 != st>>4 {
			continue
		}
		// A pair along x faces north or south, a pair along z faces west or east.
		alongX := d[0] != 0
		face := n & 7
		isNS := face == 2 || face == 3
		if alongX != isNS { // the neighbour faces across the pair: the new one turns to match it
			face = st & 7
			if alongX != (face == 2 || face == 3) {
				face = map[bool]uint16{true: 3, false: 5}[alongX]
			}
			g.setBlock(nx, y, nz, st>>4<<4|face)
		}
		g.setBlock(x, y, z, st>>4<<4|face)
		return
	}
}

// ---- the chest as a model ----

var chestModel, chestLarge *mmodel

func init() {
	chestModel = newModel(64, 64,
		part(1, 7, 15, box(0, 0, 0, -5, -14, 14, 5, 14)), // the lid
		part(8, 7, 15, box(0, 0, -1, -2, -15, 2, 4, 1)),  // the latch
		part(1, 6, 1, box(0, 19, 0, 0, 0, 14, 10, 14)))   // the base
	chestLarge = newModel(128, 64,
		part(1, 7, 15, box(0, 0, 0, -5, -14, 30, 5, 14)),
		part(16, 7, 15, box(0, 0, -1, -2, -15, 2, 4, 1)),
		part(1, 6, 1, box(0, 19, 0, 0, 0, 30, 10, 14)))
}

type chestInst struct {
	pos    [3]int32
	id     uint16
	facing uint16
	double bool
}

type chestCache struct {
	ver uint64
	w   *world.World
	at  []chestInst
}

// scanChests finds the chests of a column, with the pair each belongs to.
func (g *game) scanChests(cp world.ChunkPos) []chestInst {
	var found [][3]int32
	g.world.WithChunk(cp, func(c *world.Chunk) {
		for i, b := range c.Blocks {
			if id := b >> 4; id == idChest || id == idTrappedChest {
				found = append(found, [3]int32{cp.X<<4 | int32(i&15), int32(i >> 8), cp.Z<<4 | int32(i>>4&15)})
			}
		}
	})
	var out []chestInst
	for _, p := range found {
		st := g.world.Block(p[0], p[1], p[2])
		face := st & 7
		if face < 2 || face > 5 {
			face = 2
		}
		in := chestInst{pos: p, id: st >> 4, facing: face}
		// A pair along x faces north or south; the half with the other on its east side draws it.
		nx, nz := int32(1), int32(0)
		if face == 4 || face == 5 {
			nx, nz = 0, 1
		}
		if g.world.Block(p[0]+nx, p[1], p[2]+nz)>>4 == in.id {
			in.double = true
		} else if g.world.Block(p[0]-nx, p[1], p[2]-nz)>>4 == in.id {
			continue // the other half draws the whole
		}
		out = append(out, in)
	}
	return out
}

// chestsNear are the chests of the columns that are on screen.
func (g *game) chestsNear() []chestInst {
	if g.be.chest == nil {
		g.be.chest = map[world.ChunkPos]chestCache{}
	}
	var out []chestInst
	for cp := range g.meshes {
		var ver uint64
		if g.stream != nil {
			ver = uint64(g.stream.version[cp])
		}
		c, ok := g.be.chest[cp]
		if !ok || c.ver != ver || c.w != g.world {
			c = chestCache{ver: ver, w: g.world, at: g.scanChests(cp)}
			g.be.chest[cp] = c
		}
		out = append(out, c.at...)
	}
	if len(g.be.chest) > len(g.meshes)+64 { // forget the columns that left
		for cp := range g.be.chest {
			if g.meshes[cp] == nil {
				delete(g.be.chest, cp)
			}
		}
	}
	return out
}

// emitChests lays out the chests with the entity model; the lid lifts while the chest is open.
func (g *game) emitChests(groups map[string][]vertex) {
	for _, c := range g.chestsNear() {
		p := c.pos
		var be *blockEntity
		if g.be.ents != nil {
			be = g.be.ents[p]
		}
		target := float32(0)
		if g.screen == screenContainer && g.cont.kind == contChest && (g.cont.be == be || g.cont.be2 == be) && be != nil {
			target = 1
		}
		angle := float32(0)
		if be != nil {
			be.lid += (target - be.lid) * 0.2
			if be.lid < 0.005 {
				be.lid = 0
			}
			f := 1 - be.lid
			angle = (1 - f*f*f) * math.Pi / 2
		}
		mdl, tex := chestModel, "chest/normal"
		if c.id == idTrappedChest {
			tex = "chest/trapped"
		}
		cx, cz := float32(p[0])+0.5, float32(p[2])+0.5
		origin := vec3{float64(p[0]), float64(p[1]) - 0.5078125, float64(p[2]) + 1}
		if c.double {
			mdl, tex = chestLarge, tex+"_double"
			if c.facing == 2 || c.facing == 3 { // the pair lies along x
				cx = float32(p[0]) + 1
			} else { // along z
				cz = float32(p[2]) + 1
				origin = vec3{float64(p[0]) - 0.5, origin[1], float64(p[2]) + 1.5}
			}
		}
		rots := []rot{{x: -angle}, {x: -angle}, {}}
		light := g.world.Light(p[0], p[1], p[2])
		out := groups[tex]
		start := len(out)
		out = emitModel(out, mdl, rots, origin, 0, 0, [3]uint8{255, 255, 255}, light)
		j := float64(0) // facing south is the model's own
		switch c.facing {
		case 4:
			j = 90
		case 5:
			j = -90
		case 2:
			j = 180
		default:
			j = 0
		}
		if j != 0 {
			sn, cs := float32(math.Sin(j*math.Pi/180)), float32(math.Cos(j*math.Pi/180))
			for i := start; i < len(out); i++ {
				dx, dz := out[i].X-cx, out[i].Z-cz
				out[i].X, out[i].Z = cx+dx*cs+dz*sn, cz-dx*sn+dz*cs
			}
		}
		groups[tex] = out
	}
}

// beScreenshot sets up a screenshot of the chests in the world ("chests") or of a window ("chest",
// "bigchest", "furnace"); it is for -screenshot only.
func (g *game) beScreenshot(kind string) {
	g.fb = [2]int{g.opt.Width, g.opt.Height}
	g.setBlock(0, 4, 3, idChest<<4|2)
	g.setBlock(2, 4, 3, idChest<<4|2)
	g.setBlock(3, 4, 3, idChest<<4|2)
	g.setBlock(-2, 4, 3, idLitFurnace<<4|2)
	g.setBlock(-4, 4, 3, idTrappedChest<<4|2)
	g.setBlock(-4, 4, 4, idChest<<4|4) // a pair along z, facing west
	g.setBlock(-4, 4, 5, idChest<<4|4)
	g.beAt([3]int32{0, 4, 3}, true).lid = 0.6 // one is half open
	switch kind {
	case "chest", "bigchest":
		p := [3]int32{0, 4, 3}
		if kind == "bigchest" {
			p = [3]int32{2, 4, 3}
		}
		g.openBlockEntity(p)
		it := g.cont.be.items
		it[0], it[1], it[2], it[9] = slot{ID: 264, N: 12}, slot{ID: 276, N: 1}, blockSlot(1<<4, 64), slot{ID: 297, N: 5}
		if g.cont.be2 != nil {
			g.cont.be2.items[3] = slot{ID: 322, N: 3}
		}
	case "furnace":
		g.setBlock(5, 4, 3, idFurnace<<4)
		g.openBlockEntity([3]int32{5, 4, 3})
		it := g.cont.be.items
		it[0], it[1] = slot{ID: 15, N: 6}, slot{ID: 263, N: 3}
		for i := 0; i < 90; i++ {
			it[2] = slot{ID: 265, N: 2}
			g.cont.be.tickFurnace()
		}
	}
	g.inv[0], g.inv[1] = slot{ID: 261, N: 1}, blockSlot(5<<4, 33)
	g.cx, g.cy = float64(g.fb[0])/2, float64(g.fb[1])/2
}
