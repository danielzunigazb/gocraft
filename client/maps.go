package client

import (
	"image"
	"image/color"
	"strconv"

	"github.com/go-gl/gl/v3.3-core/gl"

	"gocraft/proto"
)

// Maps, after 1.8's MapData and MapItemRenderer. Map Data (0x34) carries 128 x 128 colour indices (a base colour
// times four shades) for a map id, in whole or as a rectangle of it. A map in an item frame is drawn on a quad of
// a block, over the frame, with the matrix RenderItemFrame gives it.

// mapBase are the 36 base colours of MapColor, as RGB.
var mapBase = [36]uint32{0, 8368696, 16247203, 13092807, 16711680, 10526975, 10987431, 31744, 16777215, 10791096, 9923917, 7368816,
	4210943, 9402184, 16776437, 14188339, 11685080, 6724056, 15066419, 8375321, 15892389, 5000268, 10066329, 5013401, 8339378,
	3361970, 6704179, 6717235, 10040115, 1644825, 16445005, 6085589, 4882687, 55610, 8476209, 7340544}

// mapShade is the factor of each of the four shades of a base colour out of 255: 180, 220, 255, 135.
var mapShade = [4]int{180, 220, 255, 135}

const mapSize = 128

// mapData is what the server said about a map.
type mapData struct {
	scale  byte
	colors [mapSize * mapSize]byte
	dirty  bool   // the picture changed and is not on the GPU yet
	tex    uint32 // its texture, once there is a context to make one
	icons  []mapIcon
}

// mapIcon is a marker on a map: a player, a frame, a marker.
type mapIcon struct{ kind, rot, x, z int8 }

// mapPixel is MapItemRenderer.updateMapTexture for one pixel: colour index 0 is a see-through checker of a dark that
// is barely there, any other is its base colour shaded.
func mapPixel(i int, c byte) color.RGBA {
	if c/4 == 0 {
		a := uint8(((i+i/mapSize)&1)*8 + 16) // Java's (i + i / 128 & 1) adds first
		return color.RGBA{0, 0, 0, a}
	}
	base := mapBase[min(int(c/4), len(mapBase)-1)]
	f := mapShade[c&3]
	return color.RGBA{uint8(int(base>>16&255) * f / 255), uint8(int(base>>8&255) * f / 255), uint8(int(base&255) * f / 255), 255}
}

// image is the map as the texture shows it.
func (m *mapData) image() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, mapSize, mapSize))
	for i, c := range m.colors {
		img.SetRGBA(i%mapSize, i/mapSize, mapPixel(i, c))
	}
	return img
}

// parseMap reads Map Data: the id, scale and icons, then the rectangle of colours (columns of 0 for none).
func parseMap(r *proto.Reader) (id int32, scale byte, icons []mapIcon, cols, rows, x0, z0 int, data []byte, ok bool) {
	id = r.VarInt()
	scale = r.Byte()
	n := int(r.VarInt())
	if n < 0 || n > 256 {
		return
	}
	for i := 0; i < n; i++ {
		d, x, z := r.Byte(), int8(r.Byte()), int8(r.Byte())
		icons = append(icons, mapIcon{kind: int8(d >> 4 & 15), rot: int8(d & 15), x: x, z: z})
	}
	cols = int(r.Byte())
	if cols > 0 {
		rows, x0, z0 = int(r.Byte()), int(r.Byte()), int(r.Byte())
		l := int(r.VarInt())
		if l < 0 || l != cols*rows || x0+cols > mapSize || z0+rows > mapSize {
			return
		}
		data = append([]byte(nil), r.Bytes(l)...)
	}
	return id, scale, icons, cols, rows, x0, z0, data, r.Err() == nil
}

// netMap is a Map Data packet applied: the rectangle goes into the map's colours (MapData.updateMapData).
func (g *game) netMap(id int32, scale byte, icons []mapIcon, cols, rows, x0, z0 int, data []byte) {
	n := g.net
	if n.maps == nil {
		n.maps = map[int32]*mapData{}
	}
	m := n.maps[id]
	if m == nil {
		if len(n.maps) >= 256 {
			return
		}
		m = &mapData{}
		n.maps[id] = m
	}
	m.scale, m.icons = scale, icons
	for i := 0; i < cols; i++ {
		for j := 0; j < rows; j++ {
			m.colors[x0+i+(z0+j)*mapSize] = data[i+j*cols]
		}
	}
	if cols > 0 {
		m.dirty = true
	}
}

// pumpMaps puts the maps that changed on the GPU; it has to be the thread that owns the context.
func (g *game) pumpMaps() {
	if g.net == nil {
		return
	}
	for id, m := range g.net.maps {
		if !m.dirty && m.tex != 0 {
			continue
		}
		img := m.image()
		if m.tex == 0 {
			m.tex = uploadTexture(img)
		} else {
			gl.BindTexture(gl.TEXTURE_2D, m.tex)
			gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, mapSize, mapSize, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))
		}
		m.dirty = false
		g.mobTex[mapKey(id)] = m.tex
	}
}

func mapKey(id int32) string { return "map/" + strconv.Itoa(int(id)) }

// emitMapInFrame adds the map an item frame holds: RenderItemFrame turns the item by a quarter-turn step of the
// frame's rotation, then flips it, scales a map's 128 pixels to a block, and puts it a hair in front of the item's
// plane (MapItemRenderer draws at z -0.01 after a translation of -1).
func (g *game) emitMapInFrame(groups map[string][]vertex, f *frameState, light uint8) {
	m := g.net.maps[int32(f.item.Dmg)]
	if m == nil {
		return
	}
	cx, cy, cz := float64(f.hang[0])+0.5, float64(f.hang[1])+0.5, float64(f.hang[2])+0.5
	mt := rotYM(float32(180-f.facing*90)).then(translateM(0, 0, 0.4375), rotZM(float32(f.rot%4*2)*360/8), rotZM(180),
		scaleM(1.0/mapSize, 1.0/mapSize, 1.0/mapSize), translateM(-64, -64, 0), translateM(0, 0, -1))
	const e = 0.01
	col := [4]uint8{255, 255, 255, light}
	key := mapKey(int32(f.item.Dmg))
	for _, c := range [4][4]float32{{0, mapSize, 0, 1}, {mapSize, mapSize, 1, 1}, {mapSize, 0, 1, 0}, {0, 0, 0, 0}} {
		p := mt.point(vec3{float64(c[0]), float64(c[1]), -e})
		groups[key] = append(groups[key], vertex{float32(cx + p[0]), float32(cy + p[1]), float32(cz + p[2]), c[2], c[3], col})
	}
}
