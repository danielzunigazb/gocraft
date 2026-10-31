package client

import (
	"image"
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// The cracks of a block being mined, after 1.8's RenderGlobal: everyone mining has one entry
// (sendBlockBreakProgress, keyed by who mines), the stage 0..9 of destroy_stage_N.png drawn over the
// block's own faces, multiplying what is under it (blend DST_COLOR, SRC_COLOR), pulled a little towards
// the camera so it never fights the block for depth. The player's own comes from mining; others' from
// Block Break Animation (0x25). An entry nobody updates goes after 400 ticks; none is drawn farther than
// 32 blocks.

const (
	crackStages   = 10
	crackStale    = 400 // ticks (RenderGlobal.cleanupDamagedBlocks)
	crackDistance = 32
	selfBreaker   = math.MinInt32 // the key of the player's own mining when there is no server id for it
)

// crackStrip is the name of the ten stages side by side, made from the jar's pictures at load.
const crackStrip = "blocks/destroy_strip"

// crackPaths are the jar's stage pictures, loaded with the entity textures.
func crackPaths() []string {
	out := make([]string, crackStages)
	for i := range out {
		out[i] = "blocks/destroy_stage_" + string(rune('0'+i))
	}
	return out
}

// makeCrackStrip puts the ten stages side by side, each scaled to 16 by 16.
func makeCrackStrip(entity map[string]*image.RGBA) {
	strip := image.NewRGBA(image.Rect(0, 0, 16*crackStages, 16))
	for i, p := range crackPaths() {
		src := entity[p]
		if src == nil {
			return
		}
		w, h := src.Bounds().Dx(), src.Bounds().Dy()
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				strip.SetRGBA(i*16+x, y, src.RGBAAt(x*w/16, y*h/16))
			}
		}
		delete(entity, p)
	}
	entity[crackStrip] = strip
}

type blockDamage struct {
	pos   [3]int32
	stage int
	at    int // the tick it was last set
}

// setDamage is RenderGlobal.sendBlockBreakProgress: a stage 0..9 for who mines, anything else ends it.
func (g *game) setDamage(breaker int32, pos [3]int32, stage int) {
	if stage < 0 || stage >= crackStages {
		delete(g.damage, breaker)
		return
	}
	if g.damage == nil {
		g.damage = map[int32]*blockDamage{}
	}
	d := g.damage[breaker]
	if d == nil || d.pos != pos {
		if len(g.damage) >= 256 {
			return
		}
		d = &blockDamage{pos: pos}
		g.damage[breaker] = d
	}
	d.stage, d.at = stage, g.ticks
}

// selfKey is whose entry the player's own mining is: its entity id on a server.
func (g *game) selfKey() int32 {
	if g.net != nil {
		return g.net.eid
	}
	return selfBreaker
}

// tickDamage forgets the cracks nobody has touched for 400 ticks.
func (g *game) tickDamage() {
	for k, d := range g.damage {
		if g.ticks-d.at > crackStale {
			delete(g.damage, k)
		}
	}
}

// buildCracks lays out the faces of every damaged block in reach, textured with its stage.
func (g *game) buildCracks(dst []vertex) []vertex {
	for _, d := range g.damage {
		c := centre(d.pos[0], d.pos[1], d.pos[2]).sub(g.camPos)
		if c.dot(c) > crackDistance*crackDistance {
			continue
		}
		st := g.world.Block(d.pos[0], d.pos[1], d.pos[2])
		if st == 0 || int(st) >= len(g.res.fast) {
			continue
		}
		e := &g.res.fast[st]
		vs := e.variants
		if e.dep {
			file, props := g.res.stateOf(st>>4, int(st&15), func(x, y, z int32) uint16 { return g.world.Block(d.pos[0]+x, d.pos[1]+y, d.pos[2]+z) })
			vs = g.res.pick(file, props)
		}
		if len(vs) == 0 {
			continue
		}
		u0 := float32(d.stage) / crackStages
		col := [4]uint8{255, 255, 255, 0xFF} // full light: the cracks only multiply what is drawn under them
		for i := range vs[0].quads {
			q := &vs[0].quads[i]
			for k := 0; k < 4; k++ {
				dst = append(dst, vertex{float32(d.pos[0]) + q.apos[k][0], float32(d.pos[1]) + q.apos[k][1], float32(d.pos[2]) + q.apos[k][2],
					u0 + q.uv[k][0]/16/crackStages, q.uv[k][1] / 16, col})
			}
		}
	}
	return dst
}

// drawCracks draws the cracks over the blocks being mined.
func (g *game) drawCracks() {
	if len(g.damage) == 0 {
		return
	}
	tex, ok := g.mobTex[crackStrip]
	if !ok {
		return
	}
	g.crackVerts = g.buildCracks(g.crackVerts[:0])
	if len(g.crackVerts) == 0 {
		return
	}
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.DST_COLOR, gl.SRC_COLOR)
	gl.Enable(gl.POLYGON_OFFSET_FILL)
	gl.PolygonOffset(-3, -3)
	gl.DepthMask(false)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	g.crackMesh.draw(g.crackVerts)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
	gl.DepthMask(true)
	gl.PolygonOffset(0, 0)
	gl.Disable(gl.POLYGON_OFFSET_FILL)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Disable(gl.BLEND)
}

// cracksDemo puts a row of stone, dirt and planks in front of the camera with the ten stages of cracks on
// them, for a screenshot (-screen cracks).
func (g *game) cracksDemo() {
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	for i := 0; i < crackStages; i++ {
		at := g.feet.add(f.scale(4)).add(right.scale(float64(i) - 4.5))
		x, y, z := int32(math.Floor(at[0])), int32(math.Floor(g.feet[1])), int32(math.Floor(at[2]))
		g.setBlock(x, y, z, []uint16{1 << 4, 3 << 4, 5 << 4}[i%3])
		g.setDamage(int32(i), [3]int32{x, y, z}, i)
	}
}
