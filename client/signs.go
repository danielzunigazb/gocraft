package client

import (
	"math"

	"gocraft/anvil"
	"gocraft/world"
)

// Signs, after 1.8's TileEntitySignRenderer: the board (and the stick of a standing sign) of ModelSign
// with entity/sign.png, turned by the block's meta, and its four lines of text in black (or the colours
// the text gives), each cut to 90 pixels and centred, drawn just in front of the board. A server sends the
// text with Update Sign (0x33); a saved world keeps it in the column's TileEntities.

const (
	idStandingSign, idWallSign = 63, 68
	signScale                  = 0.6666667 // f in the renderer
	signLineWidth              = 90        // GuiUtilRenderComponents.splitText's width
	signDistance               = 64        // blocks; a tile entity farther than this is not drawn
)

// signModel is ModelSign: the board and the stick.
var signModel = newModel(64, 32,
	part(0, 0, 0, box(0, 0, -12, -14, -1, 24, 12, 2)),
	part(0, 0, 0, box(0, 14, -1, -2, -1, 2, 14, 2)))

// signAt is the transform of a sign at a block from the sign's own space (x right, y up, z out of the
// face, in blocks) to the world: the renderer's translate, turn about y and, for a wall sign, the step
// back against the wall.
func signAt(pos [3]int32, st uint16) func(x, y, z float64) vec3 {
	meta := int(st & 15)
	angle := float64(meta) * 360 / 16
	wall := st>>4 == idWallSign
	if wall {
		angle = map[int]float64{2: 180, 4: 90, 5: -90}[meta]
	}
	t := -angle * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	ox, oy, oz := float64(pos[0])+0.5, float64(pos[1])+0.75*signScale, float64(pos[2])+0.5
	return func(x, y, z float64) vec3 {
		if wall {
			y, z = y-0.3125, z-0.4375
		}
		return vec3{ox + x*c + z*s, oy + y, oz - x*s + z*c}
	}
}

// netUpdateSign is Update Sign: the four lines of a sign, as chat components.
func (g *game) netUpdateSign(pos [3]int32, lines [4]string) {
	var t [4][]styledRun
	for i, l := range lines {
		t[i] = parseChatBase(l, signStyle)
	}
	g.setSignText(pos, t)
}

// signStyle is what sign text is when it says nothing of colour: black.
var signStyle = textStyle{col: [3]float32{0, 0, 0}}

func (g *game) setSignText(pos [3]int32, t [4][]styledRun) {
	if g.signs == nil {
		g.signs = map[[3]int32]*[4][]styledRun{}
	}
	if len(g.signs) >= 8192 {
		return
	}
	g.signs[pos] = &t
}

// loadSigns reads the text of a saved column's signs.
func (g *game) loadSigns(cp world.ChunkPos) {
	if g.save == nil {
		return
	}
	root, err := g.save.RawChunk(cp)
	if root == nil || err != nil {
		return
	}
	level, _ := root["Level"].(anvil.Compound)
	tes, _ := level["TileEntities"].([]any)
	for _, e := range tes {
		c, _ := e.(anvil.Compound)
		if id, _ := c["id"].(string); id != "Sign" {
			continue
		}
		x, _ := anvil.Number(c["x"])
		y, _ := anvil.Number(c["y"])
		z, _ := anvil.Number(c["z"])
		var lines [4]string
		for i := range lines {
			lines[i], _ = c["Text"+string(rune('1'+i))].(string)
		}
		g.netUpdateSign([3]int32{int32(x), int32(y), int32(z)}, lines)
	}
}

// emitSigns lays out every sign near the camera: the boards on entity/sign.png, the text on the font.
func (g *game) emitSigns(groups map[string][]vertex) {
	for pos, text := range g.signs {
		st := g.world.Block(pos[0], pos[1], pos[2])
		if id := st >> 4; id != idStandingSign && id != idWallSign {
			continue
		}
		c := centre(pos[0], pos[1], pos[2])
		if d := c.sub(g.camPos); d.dot(d) > signDistance*signDistance || g.offScreen(c, 1) {
			continue
		}
		at := signAt(pos, st)
		l := g.world.Light(pos[0], pos[1], pos[2])
		col := [4]uint8{255, 255, 255, l}
		out := groups["sign"]
		for pi := range signModel.parts {
			if pi == 1 && st>>4 == idWallSign { // a wall sign has no stick
				continue
			}
			for _, b := range signModel.quads[pi] {
				for _, q := range b {
					for _, v := range q {
						// The model's 1/16, then the renderer's scale (f, -f, -f).
						p := at(float64(v.x)/16*signScale, -float64(v.y)/16*signScale, -float64(v.z)/16*signScale)
						out = append(out, vertex{float32(p[0]), float32(p[1]), float32(p[2]), v.u, v.v, col})
					}
				}
			}
		}
		groups["sign"] = out
		groups["!font"] = g.signText(groups["!font"], text, at, l)
	}
}

// signText lays out the four lines on the board: in font pixels, line j at j*10-20 and centred, scaled by
// signScale/64 with y down, a little in front of the board.
func (g *game) signText(dst []vertex, text *[4][]styledRun, at func(x, y, z float64) vec3, light uint8) []vertex {
	const k = 0.015625 * signScale
	for j, rs := range text {
		if len(rs) == 0 {
			continue
		}
		if lines := g.wrapRuns(rs, signLineWidth); len(lines) > 0 {
			rs = lines[0]
		}
		w := 0
		for _, r := range rs {
			for _, ch := range r.s {
				w += int(g.glyph(ch).adv)
				if r.st.bold {
					w++
				}
			}
		}
		pen := float64(-(w / 2))
		y0 := float64(j*10 - len(text)*5)
		for _, r := range rs {
			c := [4]uint8{toByte(r.st.col[0]), toByte(r.st.col[1]), toByte(r.st.col[2]), light}
			for _, ch := range r.s {
				gl := g.glyph(ch)
				if gl.ascii && gl.id != ' ' {
					for b := 0; b <= boolInt(r.st.bold); b++ {
						u0, v0 := float32(int(gl.id)%16*8)/128, float32(int(gl.id)/16*8)/128
						x0, x1 := pen+float64(b), pen+float64(b)+8
						corner := func(x, y float64, u, v float32) vertex {
							p := at(x*k, 0.5*signScale-y*k, 0.07*signScale)
							return vertex{float32(p[0]), float32(p[1]), float32(p[2]), u, v, c}
						}
						dst = append(dst, corner(x0, y0+8, u0, v0+8.0/128), corner(x0, y0, u0, v0), corner(x1, y0, u0+8.0/128, v0), corner(x1, y0+8, u0+8.0/128, v0+8.0/128))
					}
				}
				pen += float64(gl.adv)
				if r.st.bold {
					pen++
				}
			}
		}
	}
	return dst
}

// signsDemo puts a lobby's join signs on a wall and two standing signs, turned, in front of the camera
// (-demo -screen signs).
func (g *game) signsDemo() {
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	base := g.feet.add(f.scale(4))
	bx, by, bz := int32(math.Floor(base[0])), int32(math.Floor(g.feet[1])), int32(math.Floor(base[2]))
	texts := [][4]string{
		{`{"text":"[SkyWars]","color":"dark_green","bold":true}`, `"Solo"`, `{"text":"8/12","color":"dark_blue"}`, `{"text":"Esperando","color":"dark_red"}`},
		{`{"text":"[BedWars]","color":"dark_red","bold":true}`, `"Dúos"`, `"16/16"`, `{"text":"¡Lleno!","color":"red"}`},
		{`"§1§lArenaPvP"`, `"§0Ranked"`, `""`, `"§2» Jugar «"`},
	}
	for i := int32(0); i < 3; i++ { // a wall of stone with signs on its near side
		x := bx + i - 1
		for y := by; y < by+2; y++ {
			g.setBlock(x, y, bz+1, 1<<4)
		}
		g.setBlock(x, by+1, bz, idWallSign<<4|2)
		g.netUpdateSign([3]int32{x, by + 1, bz}, texts[i])
	}
	for i, meta := range []uint16{8, 6} { // standing, facing the camera and turned
		at := g.feet.add(f.scale(2.5)).add(right.scale(float64(i)*3 - 1.5))
		p := [3]int32{int32(math.Floor(at[0])), by, int32(math.Floor(at[2]))}
		g.setBlock(p[0], p[1], p[2], idStandingSign<<4|meta)
		g.netUpdateSign(p, [4]string{`""`, `"§lgocraft"`, `"1.8.9 en Go"`, `""`})
	}
}
