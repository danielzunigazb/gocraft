package client

import (
	"math"

	"gocraft/proto"
)

// Paintings, after 1.8's EntityPainting (EntityHanging) and RenderPainting. Spawn Painting gives the art's
// title, the block the painting hangs in and which way it faces; the entity's own position is the centre of
// the art (EntityHanging.updateBoundingBox), 0.46875 off the middle of the block towards the wall. The sizes
// and the places of the pictures on the sheet are the game's EnumArt, read off its bytecode.

type paintingArt struct {
	title      string
	w, h, u, v int // size in pixels, and the picture's corner on the 256 x 256 sheet
}

var paintingArts = []paintingArt{
	{"Kebab", 16, 16, 0, 0}, {"Aztec", 16, 16, 16, 0}, {"Alban", 16, 16, 32, 0}, {"Aztec2", 16, 16, 48, 0},
	{"Bomb", 16, 16, 64, 0}, {"Plant", 16, 16, 80, 0}, {"Wasteland", 16, 16, 96, 0},
	{"Pool", 32, 16, 0, 32}, {"Courbet", 32, 16, 32, 32}, {"Sea", 32, 16, 64, 32}, {"Sunset", 32, 16, 96, 32},
	{"Creebet", 32, 16, 128, 32}, {"Wanderer", 16, 32, 0, 64}, {"Graham", 16, 32, 16, 64},
	{"Match", 32, 32, 0, 128}, {"Bust", 32, 32, 32, 128}, {"Stage", 32, 32, 64, 128}, {"Void", 32, 32, 96, 128},
	{"SkullAndRoses", 32, 32, 128, 128}, {"Wither", 32, 32, 160, 128}, {"Fighters", 64, 32, 0, 96},
	{"Pointer", 64, 64, 0, 192}, {"Pigscene", 64, 64, 64, 192}, {"BurningSkull", 64, 64, 128, 192},
	{"Skeleton", 64, 48, 192, 64}, {"DonkeyKong", 64, 48, 192, 112},
}

const paintingSheet = "painting/paintings_kristoffer_zetterstrand"

// paintingState is a painting a server put on a wall.
type paintingState struct {
	art    paintingArt
	facing int      // EnumFacing's horizontal index: 0 south, 1 west, 2 north, 3 east
	hang   [3]int32 // the block it hangs in (in front of the wall)
}

// facingStep is the front offset (x, z) of each horizontal facing.
var facingStep = [4][2]float64{{0, 1}, {-1, 0}, {0, -1}, {1, 0}}

// paintingArtByTitle is the art with a title; a title the game does not know is a painting it does not draw.
func paintingArtByTitle(title string) (paintingArt, bool) {
	for _, a := range paintingArts {
		if a.title == title {
			return a, true
		}
	}
	return paintingArt{}, false
}

// half is EntityHanging.func_174858_a: a size of whole blocks puts the centre on a block's edge.
func half(px int) float64 {
	if px%32 == 0 {
		return 0.5
	}
	return 0
}

// center is where the entity is: the middle of the picture (EntityHanging.updateBoundingBox).
func (p *paintingState) center() vec3 {
	f := facingStep[p.facing]
	left := facingStep[(p.facing+3)%4] // rotateYCCW
	x := float64(p.hang[0]) + 0.5 - f[0]*0.46875
	z := float64(p.hang[2]) + 0.5 - f[1]*0.46875
	return vec3{x + half(p.art.w)*left[0], float64(p.hang[1]) + 0.5 + half(p.art.h), z + half(p.art.w)*left[1]}
}

// box is what a click hits: the picture's width and height by a thin slab.
func (p *paintingState) box(c vec3) aabb {
	hw, hh, hd := float64(p.art.w)/32, float64(p.art.h)/32, 1.0/32
	if p.facing == 1 || p.facing == 3 { // facing along x: the width runs along z
		hw, hd = hd, hw
	}
	return aabb{c.sub(vec3{hw, hh, hd}), c.add(vec3{hw, hh, hd})}
}

// parseSpawnPainting reads Spawn Painting: an entity id, the art's title, the block and the facing.
func parseSpawnPainting(r *proto.Reader) (eid int32, title string, pos [3]int32, facing int, ok bool) {
	eid = r.VarInt()
	title = r.String(13)
	x, y, z := proto.UnpackPos(r.Long())
	facing = int(r.Byte())
	return eid, title, [3]int32{int32(x), int32(y), int32(z)}, facing & 3, r.Err() == nil
}

// emitPainting adds a painting: for each 16 x 16 square of the art, the picture on the front and the planks on
// the back and the edges, lit by the block in front of that square (RenderPainting.setLightmap).
func (g *game) emitPainting(groups map[string][]vertex, e *netEntity) {
	p := e.painting
	c := e.pos
	a := p.art
	yaw := float64(p.facing * 90)
	ya := (180 - yaw) * math.Pi / 180
	yc, ys := math.Cos(ya), math.Sin(ya)
	at := func(x, y, z float32) (float32, float32, float32) { // the renderer's pixels to the world: a turn about y, scaled by 1/16
		fx, fy, fz := float64(x)*0.0625, float64(y)*0.0625, float64(z)*0.0625
		return float32(c[0] + fx*yc + fz*ys), float32(c[1] + fy), float32(c[2] - fx*ys + fz*yc)
	}
	// the shading of a face, as the entity models give theirs: the sides darker than the front
	shadeOf := func(nx, ny, nz float64) float32 {
		x, y, z := nx*yc+nz*ys, ny, -nx*ys+nz*yc
		switch {
		case y > 0.5:
			return 1
		case y < -0.5:
			return 0.5
		case math.Abs(x) > math.Abs(z):
			return 0.6
		}
		return 0.8
	}
	const f2, f3, f4, f5, f6 = 0.5, 0.75, 0.8125, 0.0, 0.0625
	const f9, f11 = 0.001953125, 0.7519531
	x0, y0 := float32(-a.w)/2, float32(-a.h)/2
	key := paintingSheet
	for i := 0; i < a.w/16; i++ {
		for j := 0; j < a.h/16; j++ {
			f15, f16 := x0+float32((i+1)*16), x0+float32(i*16)
			f17, f18 := y0+float32((j+1)*16), y0+float32(j*16)
			l := g.paintingLight(p, c, (f15+f16)/2, (f17+f18)/2)
			u19 := float32(a.u+a.w-i*16) / 256
			u20 := float32(a.u+a.w-(i+1)*16) / 256
			v21 := float32(a.v+a.h-j*16) / 256
			v22 := float32(a.v+a.h-(j+1)*16) / 256
			quad := func(nx, ny, nz float64, v [4][5]float32) {
				s := shadeOf(nx, ny, nz)
				col := [4]uint8{uint8(255 * s), uint8(255 * s), uint8(255 * s), l}
				for _, q := range v {
					x, y, z := at(q[0], q[1], q[2])
					groups[key] = append(groups[key], vertex{x, y, z, q[3], q[4], col})
				}
			}
			quad(0, 0, -1, [4][5]float32{{f15, f18, -f2, u20, v21}, {f16, f18, -f2, u19, v21}, {f16, f17, -f2, u19, v22}, {f15, f17, -f2, u20, v22}})
			quad(0, 0, 1, [4][5]float32{{f15, f17, f2, f3, f5}, {f16, f17, f2, f4, f5}, {f16, f18, f2, f4, f6}, {f15, f18, f2, f3, f6}})
			quad(0, 1, 0, [4][5]float32{{f15, f17, -f2, f3, f9}, {f16, f17, -f2, f4, f9}, {f16, f17, f2, f4, f9}, {f15, f17, f2, f3, f9}})
			quad(0, -1, 0, [4][5]float32{{f15, f18, f2, f3, f9}, {f16, f18, f2, f4, f9}, {f16, f18, -f2, f4, f9}, {f15, f18, -f2, f3, f9}})
			quad(-1, 0, 0, [4][5]float32{{f15, f17, f2, f11, f5}, {f15, f18, f2, f11, f6}, {f15, f18, -f2, f11, f6}, {f15, f17, -f2, f11, f5}})
			quad(1, 0, 0, [4][5]float32{{f16, f17, -f2, f11, f5}, {f16, f18, -f2, f11, f6}, {f16, f18, f2, f11, f6}, {f16, f17, f2, f11, f5}})
		}
	}
}

// paintingLight is the light of the block in front of a square of the painting, x and y being the square's
// middle in pixels from the painting's centre (RenderPainting.setLightmap).
func (g *game) paintingLight(p *paintingState, c vec3, x, y float32) uint8 {
	bx, by, bz := math.Floor(c[0]), math.Floor(c[1]+float64(y)/16), math.Floor(c[2])
	switch p.facing {
	case 2: // north
		bx = math.Floor(c[0] + float64(x)/16)
	case 1: // west
		bz = math.Floor(c[2] - float64(x)/16)
	case 0: // south
		bx = math.Floor(c[0] - float64(x)/16)
	case 3: // east
		bz = math.Floor(c[2] + float64(x)/16)
	}
	return g.world.Light(int32(bx), int32(by), int32(bz))
}
