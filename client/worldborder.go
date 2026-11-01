package client

import (
	"math"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// The world border, after 1.8's WorldBorder and RenderGlobal.renderWorldBorder: a square of a diameter that may
// grow or shrink over a time, seen as a wall of the forcefield texture that fades in as the player comes within a
// view distance of it, blue while it holds, green growing, red shrinking.

type worldBorder struct {
	x, z       float64 // the middle
	from, to   float64 // diameters at the start and the end of a transition
	start, end time.Time
	now        func() time.Time // for tests
}

func newWorldBorder() *worldBorder { return &worldBorder{from: 6e7, to: 6e7} }

func (b *worldBorder) time() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// diameter is the border's size now: it moves from where it was to where it goes over the time given.
func (b *worldBorder) diameter() float64 {
	if b.from == b.to {
		return b.to
	}
	if span := b.end.Sub(b.start); span > 0 {
		if f := float64(b.time().Sub(b.start)) / float64(span); f < 1 {
			return b.from + (b.to-b.from)*max(f, 0)
		}
	}
	b.from = b.to
	return b.to
}

// transition sets the border moving from one size to another over ms milliseconds (a size alone is instant).
func (b *worldBorder) transition(from, to float64, ms int64) {
	b.from, b.to = from, to
	b.start = b.time()
	b.end = b.start.Add(time.Duration(ms) * time.Millisecond)
}

// edges are the four sides, limited to the world's 30 million blocks.
func (b *worldBorder) edges() (minX, minZ, maxX, maxZ float64) {
	d := b.diameter() / 2
	const w = 29999984
	return max(b.x-d, -w), max(b.z-d, -w), min(b.x+d, w), min(b.z+d, w)
}

// colour is the colour of the wall: blue still, green growing, red shrinking.
func (b *worldBorder) colour() [3]uint8 {
	switch {
	case b.from == b.to:
		return [3]uint8{0x20, 0xA0, 0xFF}
	case b.to > b.from:
		return [3]uint8{0x80, 0xFF, 0x20}
	}
	return [3]uint8{0xFF, 0x30, 0x30}
}

// wall lays out the part of the border within reach of the camera, and how opaque it is: the wall is drawn only
// when the camera is within reach of a side, and the nearer, the more opaque (to the fourth power).
func (b *worldBorder) wall(cam vec3, reach float64, shift float32) (vs []vertex, alpha float32) {
	minX, minZ, maxX, maxZ := b.edges()
	if cam[0] < maxX-reach && cam[0] > minX+reach && cam[2] < maxZ-reach && cam[2] > minZ+reach {
		return nil, 0
	}
	near := min(maxX-cam[0], cam[0]-minX, maxZ-cam[2], cam[2]-minZ)
	a := math.Pow(1-near/reach, 4)
	if a <= 0 {
		return nil, 0
	}
	c := b.colour()
	col := [4]uint8{c[0], c[1], c[2], 0xFF}
	// side adds the quads of one side, a block apart along it; at fixes the other axis, alongX says which runs.
	side := func(at float64, lo, hi float64, alongX bool) {
		var f float32
		for p := lo; p < hi; p, f = p+1, f+0.5 {
			w := min(1, hi-p)
			u0, u1 := shift+f, shift+float32(w)*0.5+f
			v0, v1 := shift, shift+128
			pt := func(along, y float64, u, v float32) vertex {
				if alongX {
					return vertex{float32(along), float32(y), float32(at), u, v, col}
				}
				return vertex{float32(at), float32(y), float32(along), u, v, col}
			}
			vs = append(vs, pt(p, 256, u0, v0), pt(p+w, 256, u1, v0), pt(p+w, 0, u1, v1), pt(p, 0, u0, v1))
		}
	}
	zlo, zhi := max(math.Floor(cam[2]-reach), minZ), min(math.Ceil(cam[2]+reach), maxZ)
	xlo, xhi := max(math.Floor(cam[0]-reach), minX), min(math.Ceil(cam[0]+reach), maxX)
	if cam[0] > maxX-reach {
		side(maxX, zlo, zhi, false)
	}
	if cam[0] < minX+reach {
		side(minX, zlo, zhi, false)
	}
	if cam[2] > maxZ-reach {
		side(maxZ, xlo, xhi, true)
	}
	if cam[2] < minZ+reach {
		side(minZ, xlo, xhi, true)
	}
	return vs, float32(min(a, 1))
}

// drawBorder draws the wall.
func (g *game) drawBorder() {
	if g.net == nil || g.net.border == nil {
		return
	}
	tex, ok := g.mobTex["misc/forcefield"]
	if !ok {
		return
	}
	shift := float32(time.Now().UnixMilli()%3000) / 3000
	vs, alpha := g.net.border.wall(g.camPos, float64(g.viewDist()*16), shift)
	if len(vs) == 0 {
		return
	}
	g.borderVerts = append(g.borderVerts[:0], vs...)
	// The game adds the colour to what is behind (SRC_ALPHA, ONE). That blend gave wrong colours in a headless
	// renderer, so the wall is blended over it instead: the same wall, a little darker.
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Disable(gl.CULL_FACE)
	gl.DepthMask(false)
	gl.Enable(gl.POLYGON_OFFSET_FILL)
	gl.PolygonOffset(-3, -3)
	gl.Uniform1f(g.uAlpha, alpha)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	g.borderMesh.draw(g.borderVerts)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
	gl.Uniform1f(g.uAlpha, 1)
	gl.Disable(gl.POLYGON_OFFSET_FILL)
	gl.DepthMask(true)
	gl.Enable(gl.CULL_FACE)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Disable(gl.BLEND)
}

// netWorldBorder is World Border (0x44).
func (g *game) netWorldBorder(action int32, x, z, from, to float64, ms int64) {
	n := g.net
	if n.border == nil {
		n.border = newWorldBorder()
	}
	b := n.border
	switch action {
	case 0: // set size
		b.transition(from, from, 0)
	case 1: // lerp size
		b.transition(from, to, ms)
	case 2: // set centre
		b.x, b.z = x, z
	case 3: // initialize
		b.x, b.z = x, z
		b.transition(from, to, ms)
	}
}
