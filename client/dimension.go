package client

import (
	"math"
	"path/filepath"

	"github.com/go-gl/gl/v3.3-core/gl"

	"gocraft/world"
	"gocraft/worldgen"
)

// Dimensions: the Overworld, the Nether (-1) and the End (1). Each has a world of its own, kept in
// memory while the player is elsewhere; the edits of the Nether and the End go to DIM-1 and DIM1
// folders inside the world's overlay, so a saved world is never written to. The Overworld is the
// world a save was opened as; the others are made by worldgen from the same seed.

const (
	dimNether    = -1
	dimOverworld = 0
	dimEnd       = 1
)

// dimState is everything the game keeps about dimensions and portals.
type dimState struct {
	id     int                  // the dimension being played
	others map[int]*world.World // the worlds of the dimensions the player is not in
	nether *worldgen.Nether
	end    *worldgen.End

	inPortal    bool
	portalTicks int     // Entity.portalCounter: ticks spent in a portal
	cooldown    int     // Entity.timeUntilPortal
	overlay     float32 // EntityPlayerSP.timeInPortal: how far the purple screen has come
	prevOverlay float32
	fires       []tempFire

	endProg  uint32 // the End's sky shader
	animTick int    // ticks the animated textures have run
}

func dimFolder(id int) string {
	switch id {
	case dimNether:
		return "DIM-1"
	case dimEnd:
		return "DIM1"
	}
	return ""
}

// dimReset forgets the dimensions when a world is closed.
func (g *game) dimReset() { g.dimn = dimState{endProg: g.dimn.endProg} }

// worldSeed is what the world was generated from; a world with no save (the flat test world) has none.
func (g *game) worldSeed() int64 {
	if g.save != nil {
		return g.save.Level.Seed
	}
	return 0
}

// dimOverlayOf is the overlay folder the edits of a dimension go to. It has none for a world with no
// save.
func (g *game) dimOverlayOf(id int) (overlay, bool) {
	if g.save == nil {
		return overlay{}, false
	}
	o := overlayFor(g.save.Dir)
	if f := dimFolder(id); f != "" {
		o.dir = filepath.Join(o.dir, f)
	}
	return o, true
}

// dimOverlay is the overlay of the dimension being played.
func (g *game) dimOverlay() overlay {
	o, _ := g.dimOverlayOf(g.dimn.id)
	return o
}

// dimChunks is where a dimension's columns come from: its saved edits first, then its generator.
func (g *game) dimChunks(id int) func(world.ChunkPos) *world.Chunk {
	var gen func(world.ChunkPos) *world.Chunk
	switch id {
	case dimNether:
		if g.dimn.nether == nil {
			g.dimn.nether = worldgen.NewNether(g.worldSeed())
		}
		gen = g.dimn.nether.Func()
	case dimEnd:
		if g.dimn.end == nil {
			g.dimn.end = worldgen.NewEnd(g.worldSeed())
		}
		gen = g.dimn.end.Func()
	default:
		return func(world.ChunkPos) *world.Chunk { return nil }
	}
	ov, saved := g.dimOverlayOf(id)
	return func(cp world.ChunkPos) *world.Chunk {
		if saved {
			if c, err := ov.loadChunk(cp); err != nil {
				logf("column %v of the saved edits: %v", cp, err)
			} else if c != nil {
				return c
			}
		}
		return gen(cp)
	}
}

func (g *game) dimWorld(id int) *world.World {
	if w := g.dimn.others[id]; w != nil {
		return w
	}
	w := world.NewFrom(g.dimChunks(id))
	w.SetLightRules(g.res.light)
	return w
}

// saveDim writes the edited columns and the block entities of the dimension being played to its overlay.
func (g *game) saveDim() {
	if g.save == nil || g.world == nil || g.opt.Bench > 0 || g.opt.Screenshot != "" {
		return
	}
	o := g.dimOverlay()
	for _, cp := range g.world.TakeDirty() {
		if err := o.saveChunk(cp, g.world.Snapshot(cp)); err != nil {
			logf("saving column %v: %v", cp, err)
		}
	}
	g.saveBlockEntities(o)
}

// switchWorld makes dimension to the one being played. The one left stays in memory as it is; what
// the player edited in it is saved first.
func (g *game) switchWorld(to int) {
	from := g.dimn.id
	if to == from {
		return
	}
	g.saveDim()
	if g.dimn.others == nil {
		g.dimn.others = map[int]*world.World{}
	}
	g.dimn.others[from] = g.world
	next := g.dimWorld(to)
	delete(g.dimn.others, to)
	for cp, m := range g.meshes {
		m.free()
		delete(g.meshes, cp)
	}
	g.dirty = map[world.ChunkPos]bool{}
	g.world = next
	g.dimn.id = to
	g.items, g.mobs, g.arrows = nil, nil, nil
	g.entityChunks = nil
	if o, ok := g.dimOverlayOf(to); ok {
		g.loadBlockEntities(o)
	} else {
		g.be.ents = map[[3]int32]*blockEntity{}
	}
}

// restoreDim puts a loaded world straight into the dimension the player left it in, before any
// column has been drawn.
func (g *game) restoreDim(id int) {
	if id != dimNether && id != dimEnd || id == g.dimn.id {
		return
	}
	if g.dimn.others == nil {
		g.dimn.others = map[int]*world.World{}
	}
	g.dimn.others[g.dimn.id] = g.world
	g.world = g.dimWorld(id)
	g.dimn.id = id
	if o, ok := g.dimOverlayOf(id); ok {
		g.loadBlockEntities(o)
	}
}

// fellOut is what happens below the world. In the Overworld the player goes back to the start; in
// the Nether and the End they come home to the Overworld's spawn.
func (g *game) fellOut() {
	if g.dimn.id != dimOverworld {
		g.enterDimension(dimOverworld, g.spawn)
		g.health = max(g.health, 1)
		return
	}
	g.feet, g.vel = g.spawn, vec3{}
}

// debugEnter puts the player in a dimension without a portal, for tests and screenshots.
func (g *game) debugEnter(id int) {
	switch id {
	case dimNether:
		if g.dimn.id != dimNether {
			g.enterDimension(dimNether, vec3{g.feet[0] / 8, 64, g.feet[2] / 8})
		}
	case dimEnd:
		if g.dimn.id != dimEnd {
			g.enterDimension(dimEnd, vec3{100.5, 50, 0.5})
		}
	default:
		if g.dimn.id != dimOverworld {
			g.enterDimension(dimOverworld, g.spawn)
		}
	}
}

// debugCycleDim (F8) goes Overworld, Nether, End and round again.
func (g *game) debugCycleDim() {
	g.debugEnter(map[int]int{dimOverworld: dimNether, dimNether: dimEnd, dimEnd: dimOverworld}[g.dimn.id])
}

// ---- what a dimension looks like ----

// dimAngle is the celestial angle the sky and light follow: the Nether's is always 0.5 and the End's
// 0 (WorldProviderHell and WorldProviderEnd.calculateCelestialAngle).
func (g *game) dimAngle() float32 {
	switch g.dimn.id {
	case dimNether:
		return 0.5
	case dimEnd:
		return 0
	}
	return celestialAngle(g.worldTime)
}

// dimFogColour is WorldProvider.getFogColor: the Nether's brown-red, the End's dark violet.
func (g *game) dimFogColour(angle float32) [3]float32 {
	switch g.dimn.id {
	case dimNether:
		return [3]float32{0.2, 0.03, 0.03}
	case dimEnd:
		const k = 0.15 // 10518688 = 0xA080A0, times (f*0 + 0.15)
		return [3]float32{160.0 / 255 * k, 128.0 / 255 * k, 160.0 / 255 * k}
	}
	return fogColour(angle)
}

// dimSkyColours are the colours of the sky gradient: the Nether and the End have no sky of their
// own, so it is the fog's colour all the way up.
func (g *game) dimSkyColours(angle float32) (top, horizon [3]float32) {
	if g.dimn.id != dimOverworld {
		f := g.dimFogColour(angle)
		return f, f
	}
	return skyColour(angle), fogColour(angle)
}

// dimFogRange is EntityRenderer.setupFog: the Nether's fog is thick and close (it starts at 5% of
// the view distance and ends at half of it, at most 96 blocks); the others' is the usual.
func (g *game) dimFogRange(far float32) (start, end float32) {
	if g.dimn.id == dimNether {
		return far * 0.05, min(far, 192) / 2
	}
	return far * 0.6, far * 0.95
}

// netherBrightness is WorldProviderHell.generateLightBrightnessTable: the Overworld's curve with a
// floor of 0.1, so the Nether is never pitch dark.
var netherBrightness = func() (t [16]float32) {
	for i := range t {
		f1 := 1 - float32(i)/15
		t[i] = (1-f1)/(f1*3+1)*(1-0.1) + 0.1
	}
	return
}()

// buildLightmapDim is buildLightmap for the dimension being played: its own brightness curve, and
// the End's fixed ambient light (EntityRenderer.updateLightmap, dimension 1).
func (g *game) buildLightmapDim(sun float32, out *[16 * 16 * 3]uint8) {
	id := g.dimn.id
	if id == dimOverworld {
		buildLightmap(sun, g.opts.Gamma, g.flicker, out)
		return
	}
	table := &brightnessTable
	if id == dimNether {
		table = &netherBrightness
	}
	gamma, flicker := g.opts.Gamma, g.flicker
	for sky := 0; sky < 16; sky++ {
		for blk := 0; blk < 16; blk++ {
			f2 := table[sky] * (sun*0.95 + 0.05)
			f3 := table[blk] * (flicker*0.1 + 1.5)
			f4 := f2 * (sun*0.65 + 0.35)
			f6 := f3 * ((f3*0.6+0.4)*0.6 + 0.4)
			f7 := f3 * (f3*f3*0.6 + 0.4)
			c := [3]float32{f4 + f3, f4 + f6, f2 + f7}
			for k := range c {
				v := c[k]*0.96 + 0.03
				if id == dimEnd { // the End ignores the sky: a flat 0.22 / 0.28 / 0.25 plus the torch light
					v = [3]float32{0.22 + f3*0.75, 0.28 + f6*0.75, 0.25 + f7*0.75}[k]
				}
				v = max(0, min(1, v))
				inv := 1 - v
				v = v*(1-gamma) + (1-inv*inv*inv*inv)*gamma
				out[(sky*16+blk)*3+k] = toByte(max(0, min(1, v)))
			}
		}
	}
}

const endSkyFS = `#version 330 core
in vec2 ndc;
uniform vec3 forward;
uniform vec3 right;
uniform vec3 up;
uniform vec2 tanFov;
uniform sampler2D tex;
out vec4 frag;
void main() {
	vec3 d = normalize(forward + right * (ndc.x * tanFov.x) + up * (ndc.y * tanFov.y));
	vec3 a = abs(d);
	vec2 uv;
	if (a.y >= a.x && a.y >= a.z) uv = d.xz / a.y;
	else if (a.x >= a.z) uv = d.zy / a.x;
	else uv = d.xy / a.z;
	uv = uv * 0.5 + 0.5;
	// RenderGlobal.renderSkyEnd: six faces of end_sky.png, each tinted (40, 40, 40)
	frag = vec4(texture(tex, uv).rgb * (40.0 / 255.0), 1.0);
}`

// drawEndSky draws the End's sky: the stars texture on the six sides of a box round the camera.
func (g *game) drawEndSky(fovY float32, w, h int, f vec3) {
	tex, ok := g.mobTex["environment/end_sky"]
	if !ok {
		return
	}
	if g.dimn.endProg == 0 {
		p, err := program(skyVS, endSkyFS)
		if err != nil {
			logf("end sky shader: %v", err)
			g.dimn.endProg = ^uint32(0) // do not try again
			return
		}
		g.dimn.endProg = p
	}
	if g.dimn.endProg == ^uint32(0) {
		return
	}
	right := f.cross(vec3{0, 1, 0}).norm()
	up := right.cross(f)
	th := float32(math.Tan(float64(fovY) / 2))
	p := g.dimn.endProg
	gl.UseProgram(p)
	gl.Uniform3f(uniform(p, "forward"), float32(f[0]), float32(f[1]), float32(f[2]))
	gl.Uniform3f(uniform(p, "right"), float32(right[0]), float32(right[1]), float32(right[2]))
	gl.Uniform3f(uniform(p, "up"), float32(up[0]), float32(up[1]), float32(up[2]))
	gl.Uniform2f(uniform(p, "tanFov"), th*float32(w)/float32(h), th)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.Uniform1i(uniform(p, "tex"), 0)
	gl.BindVertexArray(g.skyVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
}

// drawPortalOverlay is GuiIngame.renderPortal: a purple wash over the screen that grows while the
// player stands in a portal.
func (g *game) drawPortalOverlay(w, h int) {
	t := g.dimn.prevOverlay + (g.dimn.overlay-g.dimn.prevOverlay)*g.partial
	if t <= 0 || g.screen.isMenu() {
		return
	}
	if t < 1 {
		t = t * t
		t = t*0.8 + 0.2
	}
	g.enableUI()
	hd := &g.hud
	hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0.45, 0.15, 0.75, 0.55 * t})
	hd.flush(hd.white)
}
