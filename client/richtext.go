package client

import (
	"bytes"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"math/rand"
	"strings"
)

// Styled text: what a server sends in chat, signs and item names. A chat component is JSON with a
// colour, formats and children; older plugins also put section-sign codes (§a) in the text. Both
// become runs of one style each. The font is looked up per character, so accents and « » come out.

type textStyle struct {
	col                         [3]float32
	bold, italic, under, strike bool
	obf                         bool
}

var defaultStyle = textStyle{col: [3]float32{1, 1, 1}}

type styledRun struct {
	s  string
	st textStyle
}

// chatColours are the sixteen colours of the game, by code 0-9 a-f.
var chatColours = func() (c [16][3]float32) {
	for i, v := range [16]uint32{0x000000, 0x0000AA, 0x00AA00, 0x00AAAA, 0xAA0000, 0xAA00AA, 0xFFAA00, 0xAAAAAA,
		0x555555, 0x5555FF, 0x55FF55, 0x55FFFF, 0xFF5555, 0xFF55FF, 0xFFFF55, 0xFFFFFF} {
		c[i] = [3]float32{float32(v>>16&255) / 255, float32(v>>8&255) / 255, float32(v&255) / 255}
	}
	return
}()

var colourNames = map[string]int{"black": 0, "dark_blue": 1, "dark_green": 2, "dark_aqua": 3, "dark_red": 4, "dark_purple": 5,
	"gold": 6, "gray": 7, "dark_gray": 8, "blue": 9, "green": 10, "aqua": 11, "red": 12, "light_purple": 13, "yellow": 14, "white": 15}

// fontIndex maps a character to its cell of the font; ASCII is its own cell.
var fontIndex = func() map[rune]uint8 {
	m := map[rune]uint8{}
	i := 0
	for _, r := range fontTable {
		if r != 0 {
			if _, dup := m[r]; !dup {
				m[r] = uint8(i)
			}
		}
		i++
	}
	return m
}()

// glyphOf is the font cell a character is drawn with; what the font lacks shows as a question mark.
func glyphOf(r rune) uint8 {
	if r >= 32 && r < 127 {
		return uint8(r)
	}
	if i, ok := fontIndex[r]; ok {
		return i
	}
	return '?'
}

// legacyRuns splits a string at its § codes: a colour code clears the formats, as in the game.
func legacyRuns(s string, st textStyle) []styledRun {
	if !strings.ContainsRune(s, '§') {
		if s == "" {
			return nil
		}
		return []styledRun{{s, st}}
	}
	var out []styledRun
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, styledRun{cur.String(), st})
			cur.Reset()
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '§' || i+1 >= len(rs) {
			cur.WriteRune(rs[i])
			continue
		}
		i++
		c := strings.ToLower(string(rs[i]))[0]
		flush()
		switch {
		case c >= '0' && c <= '9':
			st = textStyle{col: chatColours[c-'0']}
		case c >= 'a' && c <= 'f':
			st = textStyle{col: chatColours[c-'a'+10]}
		case c == 'k':
			st.obf = true
		case c == 'l':
			st.bold = true
		case c == 'm':
			st.strike = true
		case c == 'n':
			st.under = true
		case c == 'o':
			st.italic = true
		case c == 'r':
			st = defaultStyle
		}
	}
	flush()
	return out
}

// translations are the few client-side strings servers still ask for by key.
var translations = map[string]string{
	"chat.type.text": "<%s> %s", "chat.type.emote": "* %s %s", "chat.type.announcement": "[%s] %s",
	"chat.type.admin": "[%s: %s]", "multiplayer.player.joined": "%s joined the game",
	"multiplayer.player.left": "%s left the game", "container.chest": "Chest", "container.chestDouble": "Large Chest",
	"container.inventory": "Inventory", "container.hopper": "Item Hopper", "container.dispenser": "Dispenser", "container.dropper": "Dropper",
	"container.crafting": "Crafting", "container.enderchest": "Ender Chest", "chat.type.achievement": "%s has just earned the achievement %s",
}

// parseChat turns a chat component, in JSON, into runs. Anything that is not JSON is plain text.
func parseChat(js string) []styledRun { return parseChatBase(js, defaultStyle) }

// parseChatBase is parseChat for text whose uncoloured parts take base's style.
func parseChatBase(js string, base textStyle) []styledRun {
	var v any
	if json.Unmarshal([]byte(js), &v) != nil {
		return legacyRuns(js, base)
	}
	return walkChat(v, base)
}

func walkChat(v any, parent textStyle) []styledRun {
	switch t := v.(type) {
	case string:
		return legacyRuns(t, parent)
	case float64, bool:
		return legacyRuns(strings.TrimSpace(strings.ToLower(jsonScalar(t))), parent)
	case []any:
		var out []styledRun
		st := parent
		for i, e := range t {
			out = append(out, walkChat(e, st)...)
			if i == 0 { // the later elements are children of the first
				if m, ok := e.(map[string]any); ok {
					st = styleOf(m, parent)
				}
			}
		}
		return out
	case map[string]any:
		st := styleOf(t, parent)
		var out []styledRun
		if s, ok := t["text"].(string); ok {
			out = append(out, legacyRuns(s, st)...)
		}
		if k, ok := t["translate"].(string); ok {
			args, _ := t["with"].([]any)
			out = append(out, translateRuns(k, args, st)...)
		}
		if ex, ok := t["extra"].([]any); ok {
			for _, e := range ex {
				out = append(out, walkChat(e, st)...)
			}
		}
		return out
	}
	return nil
}

func jsonScalar(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func translateRuns(key string, args []any, st textStyle) []styledRun {
	f, ok := translations[key]
	if !ok {
		out := legacyRuns(key, st)
		for _, a := range args {
			out = append(out, legacyRuns(" ", st)...)
			out = append(out, walkChat(a, st)...)
		}
		return out
	}
	var out []styledRun
	n := 0
	for len(f) > 0 {
		i := strings.Index(f, "%s")
		if i < 0 {
			out = append(out, legacyRuns(f, st)...)
			break
		}
		out = append(out, legacyRuns(f[:i], st)...)
		if n < len(args) {
			out = append(out, walkChat(args[n], st)...)
		}
		n++
		f = f[i+2:]
	}
	return out
}

// styleOf applies the style keys of a component over its parent's.
func styleOf(m map[string]any, p textStyle) textStyle {
	if c, ok := m["color"].(string); ok {
		if i, ok := colourNames[c]; ok {
			p.col = chatColours[i]
		}
	}
	flag := func(k string, cur bool) bool {
		switch b := m[k].(type) {
		case bool:
			return b
		case string:
			return b == "true"
		}
		return cur
	}
	p.bold, p.italic = flag("bold", p.bold), flag("italic", p.italic)
	p.under, p.strike, p.obf = flag("underlined", p.under), flag("strikethrough", p.strike), flag("obfuscated", p.obf)
	return p
}

func plainRuns(rs []styledRun) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.s)
	}
	return b.String()
}

// glyph says how a character is drawn: from the ASCII sheet, or from a page of the unicode font (the
// network's bars, ticks and hearts), and how far it moves the pen in GUI pixels.
type glyph struct {
	adv        float32
	ascii      bool
	id         uint8 // the cell of font/ascii.png
	page, cell int   // the unicode page and the cell in it
	start, end int   // the columns of the cell the character fills
}

func (g *game) glyph(r rune) glyph {
	if r >= 32 && r < 127 {
		return glyph{adv: float32(g.glyphs[r] + 1), ascii: true, id: uint8(r)}
	}
	if i, ok := fontIndex[r]; ok {
		return glyph{adv: float32(g.glyphs[i] + 1), ascii: true, id: i}
	}
	if sz := g.res.glyphSizes; sz != nil && int(r) < len(sz) && sz[r] != 0 {
		v := int(sz[r])
		return glyph{adv: float32(v&15+1-v>>4)/2 + 1, page: int(r >> 8), cell: int(r & 255), start: v >> 4, end: v & 15}
	}
	return glyph{adv: float32(g.glyphs['?'] + 1), ascii: true, id: '?'}
}

// unicodePage is the texture of a page of the unicode font, uploaded when first needed.
func (g *game) unicodePage(page int) uint32 {
	if t, ok := g.uniTex[page]; ok {
		return t
	}
	var tex uint32
	if b, ok := g.res.uniPNG[page]; ok {
		if src, err := png.Decode(bytes.NewReader(b)); err == nil {
			img := image.NewRGBA(src.Bounds())
			draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
			tex = uploadTexture(img)
		}
	}
	if g.uniTex == nil {
		g.uniTex = map[int]uint32{}
	}
	g.uniTex[page] = tex
	return tex
}

// runsWidth is the width of styled text in GUI pixels; bold letters are a pixel wider. Like the game, it
// drops the fraction after each character.
func (g *game) runsWidth(rs []styledRun) int {
	n := 0
	for _, r := range rs {
		for _, c := range r.s {
			a := g.glyph(c).adv
			if r.st.bold {
				a++
			}
			n = int(float32(n) + a)
		}
	}
	return max(n-1, 0)
}

type placedGlyph struct {
	g      glyph
	x      float32 // pen position, GUI pixels from the start
	col    [3]float32
	bold   bool
	italic bool // the top of the glyph leans right and the bottom left, by one GUI pixel each
}

// drawRuns queues styled text at x, y (framebuffer pixels). alpha scales every colour's opacity.
func (g *game) drawRuns(rs []styledRun, x, y, scale, alpha float32, shadow bool) {
	hd := &g.hud
	var placed []placedGlyph
	var lines [][4]float32 // x, y offset (in 8ths of the height), width, colour index
	var lineCols [][3]float32
	pen := float32(0)
	for _, r := range rs {
		start := pen
		for _, c := range r.s {
			gl := g.glyph(c)
			if r.st.obf && gl.ascii && gl.id != ' ' {
				for tries := 0; tries < 8; tries++ { // another letter of the same width
					if o := uint8(33 + rand.Intn(94)); g.glyphs[o] == g.glyphs[gl.id] {
						gl = g.glyph(rune(o))
						break
					}
				}
			}
			placed = append(placed, placedGlyph{gl, pen, r.st.col, r.st.bold, r.st.italic})
			pen += gl.adv
			if r.st.bold {
				pen++
			}
		}
		for _, k := range []struct {
			on  bool
			off float32
		}{{r.st.under, 8}, {r.st.strike, 4}} {
			if k.on {
				lines = append(lines, [4]float32{start, k.off, pen - start, float32(len(lineCols))})
				lineCols = append(lineCols, r.st.col)
			}
		}
	}
	for pass := 0; pass < 2; pass++ {
		if pass == 0 && !shadow {
			continue
		}
		off := float32(0)
		shade := float32(1)
		if pass == 0 {
			off, shade = scale, 0.25
		}
		colour := func(c [3]float32) [4]float32 { return [4]float32{c[0] * shade, c[1] * shade, c[2] * shade, alpha} }
		quad := func(p placedGlyph, bx float32) {
			cx := x + off + (p.x+bx)*scale
			lean := float32(0)
			if p.italic {
				lean = scale // FontRenderer: the top is moved by +1 and the bottom by -1
			}
			if p.g.ascii {
				if p.g.id != ' ' {
					hd.rectLean(cx, y+off, 8*scale, 8*scale, float32(int(p.g.id)%16*8), float32(int(p.g.id)/16*8), 8, 8, 128, 128, colour(p.col), lean)
				}
				return
			}
			uw := float32(p.g.end+1-p.g.start) - 0.02
			hd.rectLean(cx, y+off, uw/2*scale, 8*scale, float32(p.g.cell%16*16+p.g.start), float32(p.g.cell/16*16), uw, 16, 256, 256, colour(p.col), lean)
		}
		for _, p := range placed {
			if p.g.ascii {
				quad(p, 0)
				if p.bold {
					quad(p, 1)
				}
			}
		}
		hd.flush(hd.font)
		pages := map[int]bool{}
		for _, p := range placed {
			if !p.g.ascii {
				pages[p.g.page] = true
			}
		}
		for page := range pages {
			for _, p := range placed {
				if !p.g.ascii && p.g.page == page {
					quad(p, 0)
					if p.bold {
						quad(p, 1)
					}
				}
			}
			hd.flush(g.unicodePage(page))
		}
		for _, l := range lines {
			c := colour(lineCols[int(l[3])])
			hd.rect(x+off+l[0]*scale, y+off+l[1]*scale, l[2]*scale, scale, 0, 0, 1, 1, 1, 1, c)
		}
		if len(lines) > 0 {
			hd.flush(hd.white)
		}
	}
}

type styledCell struct {
	r  rune
	st textStyle
}

// wrapRuns breaks styled text into lines no wider than maxW GUI pixels, at spaces where it can.
func (g *game) wrapRuns(rs []styledRun, maxW int) [][]styledRun {
	var cells []styledCell
	for _, r := range rs {
		for _, c := range r.s {
			cells = append(cells, styledCell{c, r.st})
		}
	}
	var lines [][]styledCell
	for len(cells) > 0 {
		w, cut, lastSpace := 0, len(cells), -1
		for i, c := range cells {
			if c.r == '\n' {
				cut, lastSpace = i, -2
				break
			}
			a := g.glyph(c.r).adv
			if c.st.bold {
				a++
			}
			w = int(float32(w) + a)
			if c.r == ' ' {
				lastSpace = i
			}
			if w-1 > maxW {
				cut = i
				if cut == 0 {
					cut = 1
				}
				break
			}
		}
		next := cut
		switch {
		case lastSpace == -2: // a newline: the line ends there and the newline goes
			next = cut + 1
		case cut < len(cells) && lastSpace > 0:
			cut, next = lastSpace, lastSpace+1
		}
		lines = append(lines, cells[:cut])
		cells = cells[min(next, len(cells)):]
	}
	out := make([][]styledRun, 0, len(lines))
	for _, l := range lines {
		var runs []styledRun
		for _, c := range l {
			if n := len(runs); n > 0 && runs[n-1].st == c.st {
				runs[n-1].s += string(c.r)
			} else {
				runs = append(runs, styledRun{string(c.r), c.st})
			}
		}
		out = append(out, runs)
	}
	return out
}
