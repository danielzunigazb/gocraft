package client

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"sort"
	"strings"
)

// Block models are the game's own JSON, read from the player's 1.8.9 jar: blockstates/*.json pick a
// model by property values, models/block/*.json list cuboids with textured faces. This file bakes
// them into plain quads; nothing of Mojang's is copied into this repository.

type pack struct {
	files map[string]*zip.File
}

func newPack() *pack { return &pack{files: map[string]*zip.File{}} }

// add lays another zip over the pack: its files hide those of the same name below.
func (p *pack) add(zr *zip.ReadCloser) {
	for _, f := range zr.File {
		p.files[strings.ReplaceAll(f.Name, "\\", "/")] = f
	}
}

func (p *pack) read(name string) ([]byte, error) {
	f := p.files[name]
	if f == nil {
		return nil, fmt.Errorf("%s is not in the jar", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// png decodes an image of the jar to RGBA.
func (p *pack) png(name string) (*image.RGBA, error) {
	b, err := p.read(name)
	if err != nil {
		return nil, err
	}
	src, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img, nil
}

type rotJSON struct {
	Origin  [3]float64 `json:"origin"`
	Axis    string     `json:"axis"`
	Angle   float64    `json:"angle"`
	Rescale bool       `json:"rescale"`
}

type faceJSON struct {
	UV        *[4]float64 `json:"uv"`
	Texture   string      `json:"texture"`
	Cullface  string      `json:"cullface"`
	Rotation  int         `json:"rotation"`
	TintIndex *int        `json:"tintindex"`
}

type elementJSON struct {
	From     [3]float64          `json:"from"`
	To       [3]float64          `json:"to"`
	Rotation *rotJSON            `json:"rotation"`
	Shade    *bool               `json:"shade"`
	Faces    map[string]faceJSON `json:"faces"`
}

type modelJSON struct {
	Parent   string            `json:"parent"`
	Textures map[string]string `json:"textures"`
	Elements []elementJSON     `json:"elements"`
}

// model is a model with its parents merged in.
type model struct {
	textures map[string]string
	elements []elementJSON
}

func (p *pack) model(name string, depth int) (*model, error) {
	if depth > 16 {
		return nil, fmt.Errorf("model %s: parents too deep", name)
	}
	b, err := p.read("assets/minecraft/models/" + name + ".json")
	if err != nil {
		return nil, err
	}
	var j modelJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, fmt.Errorf("model %s: %w", name, err)
	}
	m := &model{textures: map[string]string{}}
	if j.Parent != "" && !strings.HasPrefix(j.Parent, "builtin/") {
		pm, err := p.model(j.Parent, depth+1)
		if err != nil {
			return nil, err
		}
		for k, v := range pm.textures {
			m.textures[k] = v
		}
		m.elements = pm.elements
	}
	for k, v := range j.Textures {
		m.textures[k] = v
	}
	if len(j.Elements) > 0 {
		m.elements = j.Elements
	}
	return m, nil
}

// resolveTexture follows "#name" references to a texture path, and returns the atlas name.
func (m *model) resolveTexture(ref string) string {
	for i := 0; i < 16 && strings.HasPrefix(ref, "#"); i++ {
		ref = m.textures[ref[1:]]
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return ""
	}
	return strings.TrimPrefix(ref, "blocks/")
}

// Directions, in the order the mesher and the culling use.
const (
	dDown = iota
	dUp
	dNorth
	dSouth
	dWest
	dEast
)

var dirNames = map[string]int{"down": dDown, "up": dUp, "north": dNorth, "south": dSouth, "west": dWest, "east": dEast}
var dirVec = [6][3]int32{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}

// quad is one baked face: corners in 1/16 block units, the texture's pixel corners (0..16), the
// atlas tile, a tint index (-1 none), the neighbour direction that hides it (-1 none) and its shade.
type quad struct {
	apos  [4][3]float32 // pos in block units, filled once the atlas is known
	auv   [4][2]float32 // uv as atlas coordinates, likewise
	pos   [4][3]float32
	uv    [4][2]float32
	tile  int
	tint  int
	cull  int
	shade float32
	dir   int // the direction the face looks, after rotations
}

// faceCorners lists each face's four corners as indices into (x1,y1,z1,x2,y2,z2): 0..2 pick min
// x/y/z, 3..5 pick max. This is the order the game's face bakery uses, so a face's UV corners
// follow it: (u1,v1), (u1,v2), (u2,v2), (u2,v1).
var faceCorners = [6][4][3]int{
	dDown:  {{0, 1, 5}, {0, 1, 2}, {3, 1, 2}, {3, 1, 5}},
	dUp:    {{0, 4, 2}, {0, 4, 5}, {3, 4, 5}, {3, 4, 2}},
	dNorth: {{3, 4, 2}, {3, 1, 2}, {0, 1, 2}, {0, 4, 2}},
	dSouth: {{0, 4, 5}, {0, 1, 5}, {3, 1, 5}, {3, 4, 5}},
	dWest:  {{0, 4, 2}, {0, 1, 2}, {0, 1, 5}, {0, 4, 5}},
	dEast:  {{3, 4, 5}, {3, 1, 5}, {3, 1, 2}, {3, 4, 2}},
}

func defaultUV(d int, f, t [3]float64) [4]float64 {
	x1, y1, z1, x2, y2, z2 := f[0], f[1], f[2], t[0], t[1], t[2]
	switch d {
	case dDown:
		return [4]float64{x1, 16 - z2, x2, 16 - z1}
	case dUp:
		return [4]float64{x1, z1, x2, z2}
	case dNorth:
		return [4]float64{16 - x2, 16 - y2, 16 - x1, 16 - y1}
	case dSouth:
		return [4]float64{x1, 16 - y2, x2, 16 - y1}
	case dWest:
		return [4]float64{z1, 16 - y2, z2, 16 - y1}
	}
	return [4]float64{16 - z2, 16 - y2, 16 - z1, 16 - y1} // east
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }

// rotateAxis turns a point around an axis through origin; positive angles follow the game's.
func rotateAxis(p [3]float64, axis string, angle float64, origin [3]float64) [3]float64 {
	c, s := math.Cos(rad(angle)), math.Sin(rad(angle))
	x, y, z := p[0]-origin[0], p[1]-origin[1], p[2]-origin[2]
	switch axis {
	case "x":
		y, z = y*c-z*s, y*s+z*c
	case "y":
		x, z = x*c-z*s, x*s+z*c
	case "z":
		x, y = x*c-y*s, x*s+y*c
	}
	return [3]float64{x + origin[0], y + origin[1], z + origin[2]}
}

func faceNormal(d int) [3]float64 {
	v := dirVec[d]
	return [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
}

// nearestDir is the direction a rotated normal points to.
func nearestDir(n [3]float64) int {
	best, bd := 0, -2.0
	for d := 0; d < 6; d++ {
		v := faceNormal(d)
		if dot := v[0]*n[0] + v[1]*n[1] + v[2]*n[2]; dot > bd+1e-9 {
			best, bd = d, dot
		}
	}
	return best
}

var shadeOf = [6]float32{dDown: 0.5, dUp: 1.0, dNorth: 0.8, dSouth: 0.8, dWest: 0.6, dEast: 0.6}

// bake turns a model into quads, turned by the variant's x and y rotations (multiples of 90).
func (m *model) bake(tileOf func(name string) int, rx, ry int) []quad {
	var out []quad
	for _, el := range m.elements {
		shade := el.Shade == nil || *el.Shade
		ext := [6]float64{el.From[0], el.From[1], el.From[2], el.To[0], el.To[1], el.To[2]}
		for fname, f := range el.Faces {
			d, ok := dirNames[fname]
			if !ok {
				continue
			}
			var q quad
			q.tile = tileOf(m.resolveTexture(f.Texture))
			q.tint, q.cull = -1, -1
			if f.TintIndex != nil {
				q.tint = *f.TintIndex
			}
			if c, ok := dirNames[f.Cullface]; ok {
				q.cull = c
			}
			uv := defaultUV(d, el.From, el.To)
			if f.UV != nil {
				uv = *f.UV
			}
			base := [4][2]float32{{float32(uv[0]), float32(uv[1])}, {float32(uv[0]), float32(uv[3])}, {float32(uv[2]), float32(uv[3])}, {float32(uv[2]), float32(uv[1])}}
			for i := 0; i < 4; i++ {
				q.uv[i] = base[(i+f.Rotation/90)%4]
				c := faceCorners[d][i]
				p := [3]float64{ext[c[0]], ext[c[1]], ext[c[2]]}
				if r := el.Rotation; r != nil && r.Angle != 0 {
					p = rotateAxis(p, r.Axis, r.Angle, r.Origin)
					if r.Rescale {
						k := 1 / math.Cos(rad(math.Abs(r.Angle)))
						for a, name := range [3]string{"x", "y", "z"} {
							if name != r.Axis {
								p[a] = (p[a]-r.Origin[a])*k + r.Origin[a]
							}
						}
					}
				}
				if rx != 0 {
					p = rotateAxis(p, "x", float64(rx), [3]float64{8, 8, 8})
				}
				if ry != 0 {
					p = rotateAxis(p, "y", float64(ry), [3]float64{8, 8, 8})
				}
				q.pos[i] = [3]float32{float32(p[0]), float32(p[1]), float32(p[2])}
			}
			n := faceNormal(d)
			if el.Rotation != nil && el.Rotation.Angle != 0 {
				n = rotateAxis(n, el.Rotation.Axis, el.Rotation.Angle, [3]float64{})
			}
			if rx != 0 {
				n = rotateAxis(n, "x", float64(rx), [3]float64{})
			}
			if ry != 0 {
				n = rotateAxis(n, "y", float64(ry), [3]float64{})
			}
			q.dir = nearestDir(n)
			q.shade = 1
			if shade {
				q.shade = shadeOf[q.dir]
			}
			if q.cull >= 0 {
				// The cull direction turns with the block.
				cn := faceNormal(q.cull)
				if rx != 0 {
					cn = rotateAxis(cn, "x", float64(rx), [3]float64{})
				}
				if ry != 0 {
					cn = rotateAxis(cn, "y", float64(ry), [3]float64{})
				}
				q.cull = nearestDir(cn)
			}
			out = append(out, q)
		}
	}
	// Map iteration order is random; keep the output stable.
	sort.SliceStable(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out
}

// boxes lists the cuboid elements of the model as boxes in block units, turned by the variant's
// rotations: the shapes a player collides with, for blocks whose model is their shape.
func (m *model) boxes(rx, ry int) [][6]float32 {
	var out [][6]float32
	for _, el := range m.elements {
		if el.Rotation != nil && el.Rotation.Angle != 0 {
			continue // a tilted plane (a flower, a plant) is not solid
		}
		lo, hi := [3]float32{16, 16, 16}, [3]float32{}
		for c := 0; c < 8; c++ {
			p := [3]float64{el.From[0], el.From[1], el.From[2]}
			if c&1 != 0 {
				p[0] = el.To[0]
			}
			if c&2 != 0 {
				p[1] = el.To[1]
			}
			if c&4 != 0 {
				p[2] = el.To[2]
			}
			if rx != 0 {
				p = rotateAxis(p, "x", float64(rx), [3]float64{8, 8, 8})
			}
			if ry != 0 {
				p = rotateAxis(p, "y", float64(ry), [3]float64{8, 8, 8})
			}
			for k := 0; k < 3; k++ {
				lo[k], hi[k] = min(lo[k], float32(p[k])), max(hi[k], float32(p[k]))
			}
		}
		out = append(out, [6]float32{r16(lo[0]), r16(lo[1]), r16(lo[2]), r16(hi[0]), r16(hi[1]), r16(hi[2])})
	}
	return out
}

// r16 turns 1/16 units into block units, rounding away the error of the rotations.
func r16(v float32) float32 { return float32(math.Round(float64(v)*1000)/1000) / 16 }

// variant is one choice in a blockstate: a model and its rotation, baked.
type variantJSON struct {
	Model string `json:"model"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

// blockstate maps a property string ("facing=north,half=top", or "normal") to its variants.
type blockstateJSON struct {
	Variants map[string]json.RawMessage `json:"variants"`
}

func parseVariants(raw json.RawMessage) []variantJSON {
	var many []variantJSON
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	var one variantJSON
	if json.Unmarshal(raw, &one) == nil {
		return []variantJSON{one}
	}
	return nil
}
