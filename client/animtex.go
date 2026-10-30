package client

import (
	"encoding/json"
	"image"
	"image/draw"
	"io"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Animated block textures. A texture whose picture is taller than it is wide holds the frames of an
// animation one under the other, and a .mcmeta beside it says how fast they play. The atlas holds
// the first frame; the tiles listed here are updated in place as the game runs: the Nether portal,
// lava, fire and water.

type animTile struct {
	x, y   int       // the tile's corner in the atlas, in pixels
	size   int       // the tile's side
	frames [][]uint8 // RGBA bytes of every frame, scaled to the tile
	order  []int     // frame numbers in play order
	time   int       // ticks each frame stays
	shown  int       // the position in order that is in the atlas now
}

var animatedTiles = map[string]bool{"portal": true, "lava_still": true, "lava_flow": true, "fire_layer_0": true, "fire_layer_1": true, "water_still": true, "water_flow": true}

type mcmeta struct {
	Animation struct {
		Frametime int               `json:"frametime"`
		Frames    []json.RawMessage `json:"frames"`
	} `json:"animation"`
}

// animationsFor reads the frames of the animated tiles among names.
func animationsFor(p *pack, names []string, dir string, a *atlas) []animTile {
	var out []animTile
	for i, name := range names {
		if !animatedTiles[name] {
			continue
		}
		path := "assets/minecraft/textures/" + dir + "/" + name + ".png"
		f := p.files[path]
		if f == nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		src, _, err := image.Decode(rc)
		rc.Close()
		if err != nil {
			continue
		}
		b := src.Bounds()
		w := b.Dx()
		if w < 1 || b.Dy() < 2*w || b.Dy()%w != 0 {
			continue // a single picture: nothing to play
		}
		img := image.NewRGBA(b)
		draw.Draw(img, b, src, b.Min, draw.Src)
		n := b.Dy() / w
		t := animTile{x: i % a.cols * a.tile, y: i / a.cols * a.tile, size: a.tile, time: 1}
		for fi := 0; fi < n; fi++ {
			px := make([]uint8, a.tile*a.tile*4)
			for y := 0; y < a.tile; y++ {
				for x := 0; x < a.tile; x++ {
					c := img.RGBAAt(b.Min.X+x*w/a.tile, b.Min.Y+fi*w+y*w/a.tile)
					copy(px[(y*a.tile+x)*4:], []uint8{c.R, c.G, c.B, c.A})
				}
			}
			t.frames = append(t.frames, px)
		}
		for fi := range t.frames {
			t.order = append(t.order, fi)
		}
		if mf := p.files[path+".mcmeta"]; mf != nil {
			if mr, err := mf.Open(); err == nil {
				raw, _ := io.ReadAll(mr)
				mr.Close()
				var m mcmeta
				if json.Unmarshal(raw, &m) == nil {
					if m.Animation.Frametime > 0 {
						t.time = m.Animation.Frametime
					}
					if len(m.Animation.Frames) > 0 {
						t.order = t.order[:0]
						for _, fr := range m.Animation.Frames {
							var idx int
							var obj struct {
								Index int `json:"index"`
							}
							if json.Unmarshal(fr, &idx) == nil {
								t.order = append(t.order, idx)
							} else if json.Unmarshal(fr, &obj) == nil {
								t.order = append(t.order, obj.Index)
							}
						}
					}
				}
			}
		}
		for _, fi := range t.order {
			if fi < 0 || fi >= len(t.frames) {
				t.order = nil
			}
		}
		if len(t.order) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// tickAtlasAnimation advances the animated tiles one tick: the frame that is due goes to the atlas
// texture on the GPU.
func (g *game) tickAtlasAnimation() {
	if g.win == nil || g.tex == 0 || g.res == nil || g.res.atlas == nil || len(g.res.atlas.anim) == 0 {
		return
	}
	g.dimn.animTick++
	bound := false
	for i := range g.res.atlas.anim {
		t := &g.res.atlas.anim[i]
		pos := g.dimn.animTick / t.time % len(t.order)
		if pos == t.shown {
			continue
		}
		t.shown = pos
		if !bound {
			gl.ActiveTexture(gl.TEXTURE0)
			gl.BindTexture(gl.TEXTURE_2D, g.tex)
			bound = true
		}
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(t.x), int32(t.y), int32(t.size), int32(t.size), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(t.frames[t.order[pos]]))
	}
}
