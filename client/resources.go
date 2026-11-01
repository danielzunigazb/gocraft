package client

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"gocraft/world"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"sort"
	"strings"
)

// bakedVariant is one weighted choice of a blockstate entry.
type bakedVariant struct {
	quads []quad
	boxes [][6]float32 // the model's cuboids, for collision
}

type atlas struct {
	img  *image.RGBA
	cols int
	tile int        // pixels of a tile: 16 for the game's own textures, more if a pack's are finer
	anim []animTile // the animated tiles (animtex.go)
}

// coord maps a texel position inside a tile (0..16) to atlas coordinates, keeping a sliver off the
// tile's edges so nearest sampling never reads the neighbouring tile.
func (a *atlas) coord(tile int, u, v float32) (float32, float32) {
	const inset = 0.02 // in sixteenths of a tile, whatever the tile's resolution
	u = inset + u*(16-2*inset)/16
	v = inset + v*(16-2*inset)/16
	n := float32(a.cols * 16)
	return (float32(tile%a.cols*16) + u) / n, (float32(tile/a.cols*16) + v) / n
}

// resources is everything read from the 1.8.9 jar that drawing blocks needs.
type resources struct {
	itemThird              map[string]itemXform // where each item's model sits in a hand seen from outside
	glyphSizes             []byte               // font/glyph_sizes.bin: where each character of the unicode pages starts and ends
	uniPNG                 map[int][]byte       // the unicode pages of the font, as PNG, decoded when a character of one is first drawn
	atlas                  *atlas
	states                 map[string]map[string][]*bakedVariant // blockstate file -> property string -> variants
	first                  map[string][]*bakedVariant            // a file's fallback when a property string is not listed
	table                  map[uint16]stateFn
	opaque                 [256]bool // full solid cubes: they hide the faces against them
	water                  int       // atlas tiles of the fluids
	lava                   int
	boxes                  map[uint16]cachedBox   // collision boxes, by block state, filled on use
	gui                    map[string]*image.RGBA // interface textures: "widgets", "icons"
	fast                   [4096]fastEntry        // per block state (ids below 256): what the mesher needs, read-only after loading
	grassTint, foliageTint [256][3]float32        // colour-map colours by biome id
	lang                   map[string]string      // en_US.lang: the game's own names
	iconFallback           map[uint16]int         // blocks with no blockstate: the tile standing in for their picture
	entity                 map[string]*image.RGBA // mob textures, by path under textures/entity
	light                  *world.LightRules      // what blocks do to light
	items                  *atlas                 // the pictures of items
	itemTiles              map[string]int         // model name -> tile in items
}

// loadResources reads the game's jar and lays the resource packs over it, the last on top.
func loadResources(jarPath string, packs []string) (*resources, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, fmt.Errorf("open the 1.8.9 client jar: %w", err)
	}
	defer zr.Close()
	p := newPack()
	p.add(zr)
	for _, path := range packs {
		pz, err := zip.OpenReader(path)
		if err != nil {
			continue // a pack that has gone missing or is not a zip is skipped, not fatal
		}
		defer pz.Close()
		p.add(pz)
	}

	r := &resources{states: map[string]map[string][]*bakedVariant{}, first: map[string][]*bakedVariant{}, boxes: map[uint16]cachedBox{}, gui: map[string]*image.RGBA{}}
	index := map[string]int{}
	names := []string{""} // tile 0 is the missing texture
	tile := func(name string) int {
		if name == "" {
			return 0
		}
		i, ok := index[name]
		if !ok {
			i = len(names)
			index[name] = i
			names = append(names, name)
		}
		return i
	}
	r.water, r.lava = tile("water_still"), tile("lava_still")
	r.iconFallback = map[uint16]int{}
	for id, tex := range map[uint16]string{54: "planks_oak", 146: "planks_oak", 130: "obsidian", 69: "cobblestone", 77: "stone", 143: "planks_oak",
		70: "stone", 72: "planks_oak", 33: "piston_side", 29: "piston_side", 154: "hopper_outside", 26: "planks_oak", 63: "planks_oak", 68: "planks_oak",
		144: "soul_sand", 117: "brewing_stand_base", 118: "cauldron_side", 140: "flower_pot", 92: "cake_side", 138: "glass", 119: "obsidian"} {
		r.iconFallback[id] = tile(tex)
	}

	models := map[string]*model{}
	var files []string
	for name := range p.files {
		if strings.HasPrefix(name, "assets/minecraft/blockstates/") && strings.HasSuffix(name, ".json") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	for _, path := range files {
		file := strings.TrimSuffix(strings.TrimPrefix(path, "assets/minecraft/blockstates/"), ".json")
		b, err := p.read(path)
		if err != nil {
			return nil, err
		}
		var bs blockstateJSON
		if err := json.Unmarshal(b, &bs); err != nil {
			return nil, fmt.Errorf("blockstate %s: %w", file, err)
		}
		byProps := map[string][]*bakedVariant{}
		for props, raw := range bs.Variants {
			for _, v := range parseVariants(raw) {
				m := models["block/"+v.Model]
				if m == nil {
					if m, err = p.model("block/"+v.Model, 0); err != nil {
						continue // a model the jar lacks: that variant stays empty
					}
					models["block/"+v.Model] = m
				}
				byProps[props] = append(byProps[props], &bakedVariant{quads: m.bake(tile, v.X, v.Y), boxes: m.boxes(v.X, v.Y)})
			}
		}
		r.states[file] = byProps
		keys := make([]string, 0, len(byProps))
		for k := range byProps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			r.first[file] = byProps[keys[0]]
		}
	}

	r.atlas, err = buildAtlas(p, names, "blocks")
	if err != nil {
		return nil, err
	}
	if err := r.loadItems(p); err != nil {
		return nil, err
	}
	for _, name := range []string{"gui/widgets", "gui/icons", "gui/container/creative_inventory/tab_items", "font/ascii", "gui/container/inventory", "gui/container/crafting_table", "gui/container/furnace", "gui/container/generic_54", "gui/container/hopper", "gui/container/dispenser", "gui/container/creative_inventory/tabs", "gui/container/enchanting_table", "gui/container/anvil", "gui/container/brewing_stand"} {
		img, err := p.png("assets/minecraft/textures/" + name + ".png")
		if err != nil {
			return nil, err
		}
		r.gui[name[strings.LastIndex(name, "/")+1:]] = img
	}
	for _, name := range []string{"gui/options_background", "gui/title/minecraft"} { // menu art: the game still runs without it
		if img, err := p.png("assets/minecraft/textures/" + name + ".png"); err == nil {
			r.gui[name[strings.LastIndex(name, "/")+1:]] = img
		}
	}
	if b, err := p.read("assets/minecraft/font/glyph_sizes.bin"); err == nil && len(b) == 65536 {
		r.glyphSizes = b
	}
	r.uniPNG = map[int][]byte{}
	for i := 0; i < 256; i++ {
		if b, err := p.read(fmt.Sprintf("assets/minecraft/textures/font/unicode_page_%02x.png", i)); err == nil {
			r.uniPNG[i] = b
		}
	}
	if b, err := p.read("assets/minecraft/lang/en_US.lang"); err == nil {
		r.lang = parseLang(b)
	}
	for _, name := range []string{"grass", "foliage"} {
		img, err := p.png("assets/minecraft/textures/colormap/" + name + ".png")
		if err != nil {
			return nil, err
		}
		r.gui[name] = img
	}
	r.table = stateTable(&r.opaque)
	r.findOpaque()
	r.entity = map[string]*image.RGBA{}
	extra := []string{"environment/end_sky", "arrow", "steve", "alex", "chest/normal", "chest/normal_double", "chest/trapped", "chest/trapped_double"}
	for _, m := range armorMaterials {
		extra = append(extra, "armor/"+m+"_layer_1", "armor/"+m+"_layer_2")
	}
	extra = append(extra, "armor/leather_layer_1_overlay", "armor/leather_layer_2_overlay")
	extra = append(extra, netMobTextures...)
	extra = append(extra, "explosion", "sign") // the frames of a big explosion puff; the board of a sign
	extra = append(extra, crackPaths()...)
	for _, k := range kinds {
		extra = append(extra, k.tex, k.fur)
		extra = append(extra, k.texVariants...)
	}
	for _, path := range extra {
		{
			if path == "" {
				continue
			}
			dir := "entity/"
			if strings.HasPrefix(path, "environment/") || strings.HasPrefix(path, "painting/") { // the End's sky and the paintings are not entity textures
				dir = ""
			}
			if strings.HasPrefix(path, "armor/") { // what is worn is under models/
				dir = "models/"
			}
			if strings.HasPrefix(path, "blocks/") || strings.HasPrefix(path, "misc/") { // the cracks of mining, the border's wall
				dir = ""
			}
			if img, err := p.png("assets/minecraft/textures/" + dir + path + ".png"); err == nil {
				r.entity[path] = img
			}
		}
	}
	makeCrackStrip(r.entity)
	if img, err := p.png("assets/minecraft/textures/particle/particles.png"); err == nil { // the particle sprite sheet
		r.entity["particles"] = img
	}
	r.finish()
	r.light = &world.LightRules{}
	for id := 0; id < 256; id++ {
		r.light.Opacity[id] = r.opacity(uint16(id))
		r.light.Luminance[id] = blockLum[id]
	}
	return r, nil
}

// fastEntry is a block state's models resolved once, so meshing does no map lookups for the many
// blocks whose model does not depend on their neighbours.
type fastEntry struct {
	variants []*bakedVariant
	dep      bool // fences, panes, walls and doors look at their neighbours: resolved per block
}

// neighbourDependent are the blocks whose property string reads the blocks around them.
var neighbourDependent = map[uint16]bool{53: true, 67: true, 108: true, 109: true, 114: true, 128: true, 134: true, 135: true, 136: true, 156: true, 163: true, 164: true, 180: true, 85: true, 188: true, 189: true, 190: true, 191: true, 192: true, 113: true, 102: true, 101: true,
	160: true, 139: true, 64: true, 71: true, 193: true, 194: true, 195: true, 196: true, 197: true}

// finish precomputes what meshing reads every time: quads in block units and atlas coordinates,
// the per-state variants and the biome colours.
func (r *resources) finish() {
	done := map[*bakedVariant]bool{}
	for _, byProps := range r.states {
		for _, vs := range byProps {
			for _, v := range vs {
				if done[v] {
					continue
				}
				done[v] = true
				for i := range v.quads {
					q := &v.quads[i]
					for k := 0; k < 4; k++ {
						q.apos[k] = [3]float32{q.pos[k][0] / 16, q.pos[k][1] / 16, q.pos[k][2] / 16}
						q.auv[k][0], q.auv[k][1] = r.atlas.coord(q.tile, q.uv[k][0], q.uv[k][1])
					}
				}
			}
		}
	}
	none := func(int32, int32, int32) uint16 { return 0 }
	for id := uint16(1); id < 256; id++ {
		for meta := 0; meta < 16; meta++ {
			e := &r.fast[int(id)<<4|meta]
			if neighbourDependent[id] {
				e.dep = true
				continue
			}
			file, props := r.stateOf(id, meta, none)
			e.variants = r.pick(file, props)
		}
	}
	for b := 0; b < 256; b++ {
		r.grassTint[b] = r.climateColour("grass", uint8(b))
		r.foliageTint[b] = r.climateColour("foliage", uint8(b))
	}
}

// decodeSquare reads a texture and keeps its first square: an animated texture stacks its frames.
func decodeSquare(p *pack, path string) *image.RGBA {
	f := p.files[path]
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	src, _, err := image.Decode(rc)
	if err != nil {
		return nil
	}
	b := src.Bounds()
	w := b.Dx()
	if w < 1 || b.Dy() < w {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, w))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	return img
}

// buildAtlas packs textures into one image. Every tile has the size of the finest texture (at
// least 16): coarser ones are enlarged by whole pixels, as the game does, so a pack of 8x8
// textures looks the same, only blockier.
func buildAtlas(p *pack, names []string, dir string) (*atlas, error) {
	imgs := make([]*image.RGBA, len(names))
	tile := 16
	for i, name := range names {
		if name == "" {
			continue
		}
		if imgs[i] = decodeSquare(p, "assets/minecraft/textures/"+dir+"/"+name+".png"); imgs[i] != nil {
			tile = max(tile, imgs[i].Bounds().Dx())
		}
	}
	a := &atlas{cols: 1, tile: tile}
	for a.cols*a.cols < len(names) {
		a.cols *= 2
	}
	a.img = image.NewRGBA(image.Rect(0, 0, a.cols*tile, a.cols*tile))
	for i := range names {
		ox, oy := i%a.cols*tile, i/a.cols*tile
		src := imgs[i]
		if src == nil { // missing: the purple and black of the game, here plain magenta
			draw.Draw(a.img, image.Rect(ox, oy, ox+tile, oy+tile), &image.Uniform{color.RGBA{255, 0, 255, 255}}, image.Point{}, draw.Src)
			continue
		}
		w := src.Bounds().Dx()
		for y := 0; y < tile; y++ {
			for x := 0; x < tile; x++ {
				a.img.SetRGBA(ox+x, oy+y, src.RGBAAt(x*w/tile, y*w/tile))
			}
		}
	}
	a.anim = animationsFor(p, names, dir, a)
	return a, nil
}

func (a *atlas) hasAlpha(tile int) bool {
	x0, y0 := tile%a.cols*a.tile, tile/a.cols*a.tile
	for y := y0; y < y0+a.tile; y++ {
		for x := x0; x < x0+a.tile; x++ {
			if a.img.RGBAAt(x, y).A < 250 {
				return true
			}
		}
	}
	return false
}

// fullFaces is a bitmask of the directions in which quads cover the whole side of the block.
func fullFaces(quads []quad) (mask int) {
	for _, q := range quads {
		at := float32(0) // the plane a full face lies in: 16 for up, south and east
		if q.dir == dUp || q.dir == dSouth || q.dir == dEast {
			at = 16
		}
		ax := [6]int{1, 1, 2, 2, 0, 0}[q.dir]
		full := true
		lo, hi := [3]float32{16, 16, 16}, [3]float32{}
		for _, v := range q.pos {
			if v[ax] != at {
				full = false
			}
			for k := 0; k < 3; k++ {
				lo[k], hi[k] = min(lo[k], v[k]), max(hi[k], v[k])
			}
		}
		for k := 0; k < 3 && full; k++ {
			if k != ax && (lo[k] != 0 || hi[k] != 16) {
				full = false
			}
		}
		if full {
			mask |= 1 << q.dir
		}
	}
	return mask
}

func (r *resources) findOpaque() {
	none := func(int32, int32, int32) uint16 { return 0 }
	for id := uint16(1); id < 256; id++ {
		if isFluid(id) {
			continue
		}
		file, props := r.stateOf(id, 0, none)
		vs := r.pick(file, props)
		if len(vs) == 0 {
			continue
		}
		q := vs[0].quads
		if fullFaces(q) != 63 {
			continue
		}
		clear := true
		for _, x := range q {
			if r.atlas.hasAlpha(x.tile) {
				clear = false
			}
		}
		r.opaque[id] = clear
	}
}

// stateOf is the blockstate file and property string for a block.
func (r *resources) stateOf(id uint16, meta int, nb nbFunc) (string, string) {
	if fn, ok := r.table[id]; ok {
		return fn(meta, nb)
	}
	return blockNames[id], normal
}

func (r *resources) pick(file, props string) []*bakedVariant {
	if file == "" {
		return nil
	}
	if v := r.states[file][props]; len(v) > 0 {
		return v
	}
	if v := r.states[file][normal]; len(v) > 0 {
		return v
	}
	return r.first[file]
}

// itemModelNames lists every model an item can be drawn with, variants included.
func itemModelNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for id := range itemDefs {
		for dmg := uint16(0); dmg < 16; dmg++ {
			add(itemModelName(slot{ID: id, Dmg: dmg, N: 1}))
		}
	}
	for _, n := range []string{"bow_pulling_0", "bow_pulling_1", "bow_pulling_2", "fishing_rod_cast", "bottle_splash", "bottle_drinkable"} { // a bow drawn back and a rod that is cast show these
		add(n)
	}
	sort.Strings(out)
	return out
}

// loadItems reads each item's model for its picture and packs the pictures into one atlas.
func (r *resources) loadItems(p *pack) error {
	r.itemTiles = map[string]int{}
	names := []string{""}
	index := map[string]int{}
	for _, m := range itemModelNames() {
		b, err := p.read("assets/minecraft/models/item/" + m + ".json")
		if err != nil {
			continue // an item the jar has no model for is not drawn
		}
		var j modelJSON
		if json.Unmarshal(b, &j) != nil {
			continue
		}
		tex := strings.TrimPrefix(j.Textures["layer0"], "items/")
		if tex == "" {
			continue
		}
		var d struct {
			Display map[string]struct {
				Rotation, Translation, Scale []float32
			} `json:"display"`
		}
		if json.Unmarshal(b, &d) == nil {
			if t, ok := d.Display["thirdperson"]; ok && len(t.Rotation) == 3 && len(t.Translation) == 3 && len(t.Scale) == 3 {
				if r.itemThird == nil {
					r.itemThird = map[string]itemXform{}
				}
				r.itemThird[m] = itemXform{[3]float32{t.Rotation[0], t.Rotation[1], t.Rotation[2]}, [3]float32{t.Translation[0], t.Translation[1], t.Translation[2]}, [3]float32{t.Scale[0], t.Scale[1], t.Scale[2]}}
			}
		}
		i, ok := index[tex]
		if !ok {
			i = len(names)
			index[tex] = i
			names = append(names, tex)
		}
		r.itemTiles[m] = i
	}
	var err error
	r.items, err = buildAtlas(p, names, "items")
	return err
}

// itemTile is the picture of an item stack in the item atlas.
func (r *resources) itemTile(s slot) (int, bool) {
	i, ok := r.itemTiles[itemModelName(s)]
	return i, ok
}
