package client

import (
	"math"
	"strconv"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// The hand in first person, after 1.8's ItemRenderer.renderItemInFirstPerson: the arm when nothing
// is held, otherwise the item, swung, dipped, drawn back for a bow and raised to the mouth for
// food. It is drawn in a pass of its own, over a cleared depth buffer, with the hand's own
// projection (a fixed 70 degrees, whatever the field of view setting says).

const (
	handFOV    = 70 // degrees: EntityRenderer.getFOVModifier(partial, false)
	handNear   = 0.05
	handFar    = 256
	swingTicks = 6 // EntityLivingBase.getArmSwingAnimationEnd
	eatUseMax  = 32
)

// handUse is what the held item is being used for, which changes the pose.
type handUse int

const (
	useNone handUse = iota
	useEat
	useBlock
	useBow
)

// handState is what the hand remembers from tick to tick.
type handState struct {
	equip, prevEquip       float32 // 0..1: how far up the item is
	item                   slot    // the item being drawn: it lags the selected one while the hand dips
	armPitch, prevArmPitch float32 // the hand trails the view a little: degrees
	armYaw, prevArmYaw     float32
	walked, prevWalked     float32 // distance walked, for the view bob (distanceWalkedModified)
	bobYaw, prevBobYaw     float32 // cameraYaw: how much the view bobs
	bobPitch, prevBobPitch float32
	mesh                   entityMesh
	sprites                map[spriteKey][]fpQuad
}

type spriteKey struct {
	a    *atlas
	tile int
}

// ---- the transforms, one function each, as in the game ----

func ident() mat4 { return mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }

func translateM(x, y, z float32) mat4 {
	m := ident()
	m[12], m[13], m[14] = x, y, z
	return m
}

func scaleM(x, y, z float32) mat4 {
	m := ident()
	m[0], m[5], m[10] = x, y, z
	return m
}

func rotXM(deg float32) mat4 {
	s, c := sincosDeg(deg)
	m := ident()
	m[5], m[6], m[9], m[10] = c, s, -s, c
	return m
}

func rotYM(deg float32) mat4 {
	s, c := sincosDeg(deg)
	m := ident()
	m[0], m[2], m[8], m[10] = c, -s, s, c
	return m
}

func rotZM(deg float32) mat4 {
	s, c := sincosDeg(deg)
	m := ident()
	m[0], m[1], m[4], m[5] = c, s, -s, c
	return m
}

func sincosDeg(deg float32) (s, c float32) {
	a := float64(deg) * math.Pi / 180
	return float32(math.Sin(a)), float32(math.Cos(a))
}

// then multiplies the transforms in the order GL applies them: m.then(n) is glMultMatrix(n).
func (m mat4) then(n ...mat4) mat4 {
	for _, x := range n {
		m = mul(m, x)
	}
	return m
}

func (m mat4) point(v vec3) vec3 {
	return vec3{
		float64(m[0])*v[0] + float64(m[4])*v[1] + float64(m[8])*v[2] + float64(m[12]),
		float64(m[1])*v[0] + float64(m[5])*v[1] + float64(m[9])*v[2] + float64(m[13]),
		float64(m[2])*v[0] + float64(m[6])*v[1] + float64(m[10])*v[2] + float64(m[14]),
	}
}

// dir carries a direction (a normal) through the rotation and uniform scale of m.
func (m mat4) dir(v vec3) vec3 {
	return vec3{
		float64(m[0])*v[0] + float64(m[4])*v[1] + float64(m[8])*v[2],
		float64(m[1])*v[0] + float64(m[5])*v[1] + float64(m[9])*v[2],
		float64(m[2])*v[0] + float64(m[6])*v[1] + float64(m[10])*v[2],
	}.norm()
}

func sin32f(x float32) float32  { return float32(math.Sin(float64(x))) }
func cos32f(x float32) float32  { return float32(math.Cos(float64(x))) }
func sqrt32f(x float32) float32 { return float32(math.Sqrt(float64(x))) }

// swingTerms are the two curves of a swing (s in 0..1) the poses are built from:
// sin(s*s*pi) and sin(sqrt(s)*pi).
func swingTerms(s float32) (sq, root float32) {
	return sin32f(s * s * math.Pi), sin32f(sqrt32f(s) * math.Pi)
}

// swingTranslation is ItemRenderer.func_178105_d: the item moves forward through a swing.
func swingTranslation(s float32) mat4 {
	r := sqrt32f(s)
	return translateM(-0.4*sin32f(r*math.Pi), 0.2*sin32f(r*math.Pi*2), -0.2*sin32f(s*math.Pi))
}

// transformFirstPersonItem is ItemRenderer.transformFirstPersonItem: where the item sits, dipped by
// the equip progress (0 up, 1 down), turned through a swing, scaled to 0.4.
func transformFirstPersonItem(equip, swing float32) mat4 {
	sq, root := swingTerms(swing)
	return translateM(0.56, -0.52, -0.71999997).then(
		translateM(0, equip*-0.6, 0),
		rotYM(45),
		rotYM(sq*-20), rotZM(root*-20), rotXM(root*-80),
		scaleM(0.4, 0.4, 0.4))
}

// blockPose is ItemRenderer.func_178103_d: a sword held up to block.
func blockPose() mat4 {
	return translateM(-0.5, 0.2, 0).then(rotYM(30), rotXM(-80), rotYM(60))
}

// bowPose is ItemRenderer.func_178098_a: the bow drawn back, f the ticks it has been drawn.
func bowPose(f float32) mat4 {
	m := rotZM(-18).then(rotYM(-12), rotXM(-8), translateM(-0.9, 0.2, 0))
	t := f / bowFullDraw // the game's (f*f + 2f)/3 curve of the draw, on the fractional ticks
	p := min((t*t+t*2)/3, 1)
	if p > 0.1 {
		wobble := sin32f((f-0.1)*1.3) * (p - 0.1)
		m = m.then(translateM(0, wobble*0.01, 0))
	}
	return m.then(translateM(0, 0, p*0.1), scaleM(1, 1, 1+p*0.2))
}

// eatPose is ItemRenderer.func_178104_a: the food comes up to the mouth. f is the use ticks left,
// minus the partial tick, plus one; ratio is f over the whole 32.
func eatPose(f float32) mat4 {
	ratio := f / eatUseMax
	bounce := float32(math.Abs(float64(cos32f(f/4*math.Pi) * 0.1)))
	if ratio >= 0.8 {
		bounce = 0
	}
	k := 1 - float32(math.Pow(float64(ratio), 27))
	return translateM(0, bounce, 0).then(
		translateM(k*0.6, k*-0.5, 0),
		rotYM(k*90), rotXM(k*10), rotZM(k*30))
}

// armPose is ItemRenderer.func_178095_a for the right arm: its swing, and the turns that lay the
// model's arm (y down, as the skin is drawn) into the view. equip is 1 - progress.
func armPose(equip, swing float32) mat4 {
	sq, root := swingTerms(swing)
	r := sqrt32f(swing)
	return translateM(-0.3*sin32f(r*math.Pi), 0.4*sin32f(r*math.Pi*2), -0.4*sin32f(swing*math.Pi)).then(
		translateM(0.64000005, -0.6, -0.71999997),
		translateM(0, equip*-0.6, 0),
		rotYM(45),
		rotYM(root*70), rotZM(sq*-20),
		translateM(-1, 3.6, 3.5),
		rotZM(120), rotXM(200), rotYM(-135),
		translateM(5.6, 0, 0))
}

// viewBob is EntityRenderer.setupViewBobbing: the view sways with each step.
func viewBob(walked, bobYaw, bobPitch float32) mat4 {
	x := -walked
	return translateM(sin32f(x*math.Pi)*bobYaw*0.5, -float32(math.Abs(float64(cos32f(x*math.Pi)*bobYaw))), 0).then(
		rotZM(sin32f(x*math.Pi)*bobYaw*3),
		rotXM(float32(math.Abs(float64(cos32f(x*math.Pi-0.2)*bobYaw)))*5),
		rotXM(bobPitch))
}

// itemDisplay is the "firstperson" display transform of a flat item's model: where the picture
// sits in the hand. Rods are turned the other way.
func itemDisplay(id uint16) mat4 {
	yaw := float32(-135)
	if id == 346 || id == 398 { // fishing rod, carrot on a stick
		yaw = 45
	}
	return translateM(0, 4.0/16, 2.0/16).then(rotYM(yaw), rotXM(0), rotZM(25), scaleM(1.7, 1.7, 1.7))
}

// bowSprite is which picture the bow shows as it is drawn (RenderItem.renderItemModelForEntity).
func bowSprite(used int) string {
	switch {
	case used >= 18:
		return "bow_pulling_2"
	case used > 13:
		return "bow_pulling_1"
	case used > 0:
		return "bow_pulling_0"
	}
	return "bow"
}

// ---- the lag of the hand behind the view, the bob, the equip dip: once a tick ----

func wrapDeg(d float32) float32 {
	d = float32(math.Mod(float64(d)+180, 360))
	if d < 0 {
		d += 360
	}
	return d - 180
}

// sameForHand is whether picking this item up does not dip the hand: the same item, and for things
// that do not wear, the same variant. A stack growing or shrinking is not a change.
func sameForHand(a, b slot) bool {
	if a.empty() != b.empty() {
		return false
	}
	if a.empty() {
		return true
	}
	if a.ID != b.ID {
		return false
	}
	return a.durability() > 0 || a.Dmg == b.Dmg
}

// tickHand runs once a tick, after the player has moved.
func (g *game) tickHand() {
	h := &g.hand
	h.prevEquip = h.equip
	cur := g.held()
	dip := !sameForHand(h.item, cur)
	target := float32(1)
	if dip {
		target = 0
	}
	h.equip += max(-0.4, min(0.4, target-h.equip))
	if h.equip < 0.1 {
		h.item = cur
	}

	h.prevArmPitch, h.prevArmYaw = h.armPitch, h.armYaw
	h.armPitch += (g.pitch*180/math.Pi - h.armPitch) * 0.5
	h.armYaw += wrapDeg(g.yawDeg()-h.armYaw) * 0.5

	// distanceWalkedModified, and cameraYaw/cameraPitch (EntityPlayer.onLivingUpdate).
	d := float32(math.Hypot(float64(g.feet[0]-g.prevFeet[0]), float64(g.feet[2]-g.prevFeet[2])))
	h.prevWalked = h.walked
	h.walked += d * 0.6
	speed := float32(math.Hypot(float64(g.vel[0]), float64(g.vel[2])))
	lean := float32(math.Atan(float64(-g.vel[1]*0.2)) * 15)
	speed = min(speed, 0.1)
	if !g.onGround || g.health <= 0 {
		speed = 0
	}
	if g.onGround || g.health <= 0 {
		lean = 0
	}
	h.prevBobYaw, h.prevBobPitch = h.bobYaw, h.bobPitch
	h.bobYaw += (speed - h.bobYaw) * 0.4
	h.bobPitch += (lean - h.bobPitch) * 0.8
}

// handUseNow is what the player is doing with the held item, if anything.
func (g *game) handUseNow() handUse {
	if g.screen != screenNone {
		return useNone
	}
	switch {
	case g.bowTicks > 0 && g.held().ID == 261:
		return useBow
	case g.blocking:
		return useBlock
	case g.eating > 0:
		return useEat
	}
	return useNone
}

// handSwing is the progress of the arm's swing, 0 at rest, and up to 1.
func (g *game) handSwing(partial float32) float32 {
	if g.swingTicks <= 0 {
		return 0
	}
	return max(0, min((swingTicks-float32(g.swingTicks)+partial)/swingTicks, 1))
}

// ---- the geometry ----

// fpQuad is one face of a hand model, centred on the origin, with the normal for its lighting.
type fpQuad struct {
	p  [4]vec3
	uv [4][2]float32
	n  vec3
}

// extrude builds a flat item as 1.8's ItemModelGenerator does: a picture one texel thick, with the
// edge texels of the picture stood out as a rim wherever the neighbour is clear.
func extrude(a *atlas, tile int) []fpQuad {
	tp := a.tile
	if tp <= 0 {
		tp = 16
	}
	ox, oy := tile%a.cols*tp, tile/a.cols*tp
	opaque := func(x, y int) bool {
		if x < 0 || y < 0 || x >= tp || y >= tp {
			return false
		}
		return a.img.RGBAAt(ox+x, oy+y).A > 25
	}
	const t = float32(1.0 / 32) // half the thickness: one sixteenth of the item's size
	px := 1 / float32(tp)
	u0, v0 := a.coord(tile, 0, 0)
	u1, v1 := a.coord(tile, 16, 16)
	out := []fpQuad{
		{p: [4]vec3{{-.5, -.5, float64(t)}, {.5, -.5, float64(t)}, {.5, .5, float64(t)}, {-.5, .5, float64(t)}}, uv: [4][2]float32{{u0, v1}, {u1, v1}, {u1, v0}, {u0, v0}}, n: vec3{0, 0, 1}},
		{p: [4]vec3{{.5, -.5, float64(-t)}, {-.5, -.5, float64(-t)}, {-.5, .5, float64(-t)}, {.5, .5, float64(-t)}}, uv: [4][2]float32{{u1, v1}, {u0, v1}, {u0, v0}, {u1, v0}}, n: vec3{0, 0, -1}},
	}
	for y := 0; y < tp; y++ {
		for x := 0; x < tp; x++ {
			if !opaque(x, y) {
				continue
			}
			uc, vc := a.coord(tile, (float32(x)+0.5)*16/float32(tp), (float32(y)+0.5)*16/float32(tp))
			uv := [4][2]float32{{uc, vc}, {uc, vc}, {uc, vc}, {uc, vc}}
			xl, xr := -0.5+float32(x)*px, -0.5+float32(x+1)*px
			yt, yb := 0.5-float32(y)*px, 0.5-float32(y+1)*px
			if !opaque(x-1, y) {
				out = append(out, fpQuad{p: [4]vec3{{float64(xl), float64(yb), float64(-t)}, {float64(xl), float64(yb), float64(t)}, {float64(xl), float64(yt), float64(t)}, {float64(xl), float64(yt), float64(-t)}}, uv: uv, n: vec3{-1, 0, 0}})
			}
			if !opaque(x+1, y) {
				out = append(out, fpQuad{p: [4]vec3{{float64(xr), float64(yb), float64(t)}, {float64(xr), float64(yb), float64(-t)}, {float64(xr), float64(yt), float64(-t)}, {float64(xr), float64(yt), float64(t)}}, uv: uv, n: vec3{1, 0, 0}})
			}
			if !opaque(x, y-1) {
				out = append(out, fpQuad{p: [4]vec3{{float64(xl), float64(yt), float64(t)}, {float64(xr), float64(yt), float64(t)}, {float64(xr), float64(yt), float64(-t)}, {float64(xl), float64(yt), float64(-t)}}, uv: uv, n: vec3{0, 1, 0}})
			}
			if !opaque(x, y+1) {
				out = append(out, fpQuad{p: [4]vec3{{float64(xl), float64(yb), float64(-t)}, {float64(xr), float64(yb), float64(-t)}, {float64(xr), float64(yb), float64(t)}, {float64(xl), float64(yb), float64(t)}}, uv: uv, n: vec3{0, -1, 0}})
			}
		}
	}
	return out
}

func (h *handState) sprite(a *atlas, tile int) []fpQuad {
	k := spriteKey{a, tile}
	if q, ok := h.sprites[k]; ok {
		return q
	}
	if h.sprites == nil || len(h.sprites) > 256 {
		h.sprites = map[spriteKey][]fpQuad{}
	}
	q := extrude(a, tile)
	h.sprites[k] = q
	return q
}

// flatBlocks are the blocks 1.8 shows in the hand as a flat picture, not as a cube: plants, rails,
// torches, ladders and the like have a model of their own in models/item.
var flatBlocks = map[uint16]bool{
	6: true, 27: true, 28: true, 30: true, 31: true, 32: true, 37: true, 38: true, 39: true, 40: true, 50: true,
	65: true, 66: true, 69: true, 75: true, 76: true, 101: true, 102: true, 106: true, 111: true, 131: true, 157: true,
	160: true, 175: true,
}

// itemLight is the light the hand is lit with: that of the block at the eye, as func_178109_a has it.
func (g *game) handLight() uint8 {
	l := g.world.Light(int32(math.Floor(float64(g.pos[0]))), int32(math.Floor(float64(g.pos[1]))), int32(math.Floor(float64(g.pos[2]))))
	if g.opts.DynamicLights {
		if lh := lightOf(g.held()); lh > l&15 {
			l = l&0xF0 | lh
		}
	}
	return l
}

// camera basis in the world, to light a face by its normal in the world as RenderHelper does.
type camBasis struct{ right, up, back vec3 }

func (g *game) camBasis() camBasis {
	f := look(g.yaw, g.pitch)
	r := f.cross(vec3{0, 1, 0}).norm()
	return camBasis{r, r.cross(f), f.scale(-1)}
}

var itemL0, itemL1 = vec3{0.2, 1, -0.7}.norm(), vec3{-0.2, 1, 0.7}.norm()

// itemShade is RenderHelper.enableStandardItemLighting: two lights from above and an ambient 0.4.
func itemShade(nWorld vec3) float32 {
	s := 0.4 + 0.6*max(0, nWorld.dot(itemL0)) + 0.6*max(0, nWorld.dot(itemL1))
	return float32(min(s, 1))
}

// emitFP adds a quad, carried through m into the view, lit by its normal.
func emitFP(out []vertex, cb camBasis, m mat4, q fpQuad, tint [3]float32, light uint8) []vertex {
	nc := m.dir(q.n)
	nw := cb.right.scale(nc[0]).add(cb.up.scale(nc[1])).add(cb.back.scale(nc[2]))
	s := itemShade(nw)
	col := [4]uint8{toByte(tint[0] * s), toByte(tint[1] * s), toByte(tint[2] * s), light}
	for i := 0; i < 4; i++ {
		p := m.point(q.p[i])
		out = append(out, vertex{float32(p[0]), float32(p[1]), float32(p[2]), q.uv[i][0], q.uv[i][1], col})
	}
	return out
}

// handPass is one texture's worth of geometry for the hand.
type handPass struct {
	verts []vertex
	tex   uint32
}

// handGeometry lays out what is in the hand: the arm, or the item with its pose.
func (g *game) handGeometry(partial float32) (passes []handPass) {
	h := &g.hand
	cb := g.camBasis()
	light := g.handLight()
	equip := 1 - (h.prevEquip + (h.equip-h.prevEquip)*partial)
	swing := g.handSwing(partial)

	// The view bob and the trail of the hand behind the view come first, outermost.
	base := ident()
	if g.opts.ViewBobbing {
		walked := h.prevWalked + (h.walked-h.prevWalked)*partial
		by := h.prevBobYaw + (h.bobYaw-h.prevBobYaw)*partial
		bp := h.prevBobPitch + (h.bobPitch-h.prevBobPitch)*partial
		base = viewBob(walked, by, bp)
	}
	base = rotZM(g.cameraRoll(partial)).then(base) // the view's shake on a blow, outermost
	armPitch := h.prevArmPitch + (h.armPitch-h.prevArmPitch)*partial
	armYaw := h.prevArmYaw + (h.armYaw-h.prevArmYaw)*partial
	base = base.then(rotXM((g.pitch*180/math.Pi-armPitch)*0.1), rotYM(wrapDeg(g.yawDeg()-armYaw)*0.1))

	item := h.item
	if item.empty() { // the arm
		m := base.then(armPose(equip, swing))
		var v []vertex
		rots := make([]rot, 12)
		rots[pRArm].z, rots[pRSleeve].z = 0.1, 0.1 // ModelBiped's idle sway at age 0
		for _, pi := range [2]int{pRArm, pRSleeve} {
			p := &playerMdl.parts[pi]
			r := rots[pi]
			var centre vec3
			n := 0
			var quads [][4]vec3
			var uvs [][4][2]float32
			for _, bq := range playerMdl.quads[pi] {
				for _, q := range bq {
					var w [4]vec3
					var uv [4][2]float32
					for k := 0; k < 4; k++ {
						x, y, z := place(p, r, q[k])
						w[k] = vec3{float64(x / 16), float64(y / 16), float64(z / 16)}
						uv[k] = [2]float32{q[k].u, q[k].v}
						centre = centre.add(w[k])
						n++
					}
					quads = append(quads, w)
					uvs = append(uvs, uv)
				}
			}
			centre = centre.scale(float64(1 / float32(n)))
			for i, w := range quads {
				nrm := w[1].sub(w[0]).cross(w[3].sub(w[0])).norm()
				if nrm.dot(w[0].sub(centre)) < 0 { // the model's faces wind the other way: point the normal out
					nrm = nrm.scale(-1)
				}
				v = emitFP(v, cb, m, fpQuad{p: w, uv: uvs[i], n: nrm}, [3]float32{1, 1, 1}, light)
			}
		}
		return []handPass{{v, g.mobTex["look/skin"]}}
	}

	var m mat4
	switch g.handUseNow() {
	case useEat:
		f := float32(eatUseMax-g.eating) - partial + 1
		m = base.then(eatPose(f), transformFirstPersonItem(equip, 0))
	case useBlock:
		m = base.then(transformFirstPersonItem(equip, 0), blockPose())
	case useBow:
		f := max(float32(g.bowTicks)+partial-1, 0)
		m = base.then(transformFirstPersonItem(equip, 0), bowPose(f))
	default:
		m = base.then(swingTranslation(swing), transformFirstPersonItem(equip, swing))
	}

	var v []vertex
	if item.isBlock() {
		st := item.state()
		vs := g.res.fast[int(st)&4095].variants
		if len(vs) == 0 || len(vs[0].quads) == 0 {
			return nil
		}
		if flatBlocks[item.ID] {
			q0 := vs[0].quads[0]
			tint := white
			if q0.tint >= 0 {
				tint = g.res.tintFor(item.ID, int(item.Dmg), 1)
			}
			mm := m.then(itemDisplay(item.ID), scaleM(0.5, 0.5, 0.5))
			for _, q := range h.sprite(g.res.atlas, q0.tile) {
				v = emitFP(v, cb, mm, q, tint, light)
			}
			return []handPass{{v, g.tex}}
		}
		// Blocks carry no display transform of their own in 1.8: the unit cube, halved by RenderItem.
		mm := m.then(scaleM(0.5, 0.5, 0.5))
		for i := range vs[0].quads {
			q := &vs[0].quads[i]
			tint := white
			if q.tint >= 0 {
				tint = g.res.tintFor(item.ID, int(item.Dmg), 1)
			}
			var fq fpQuad
			for k := 0; k < 4; k++ {
				fq.p[k] = vec3{float64(q.apos[k][0] - 0.5), float64(q.apos[k][1] - 0.5), float64(q.apos[k][2] - 0.5)}
				fq.uv[k] = q.auv[k]
			}
			fq.n = vec3{float64(dirVec[q.dir][0]), float64(dirVec[q.dir][1]), float64(dirVec[q.dir][2])}
			v = emitFP(v, cb, mm, fq, tint, light)
		}
		return []handPass{{v, g.tex}}
	}

	tile, ok := g.res.itemTile(item)
	if item.ID == 261 && g.handUseNow() == useBow {
		if t, found := g.res.itemTiles[bowSprite(g.bowTicks)]; found {
			tile, ok = t, true
		}
	}
	if item.ID == 346 && g.net != nil && g.net.hookOf(g.net.eid) != nil { // a rod with its line out
		if t, found := g.res.itemTiles["fishing_rod_cast"]; found {
			tile, ok = t, true
		}
	}
	if !ok {
		return nil
	}
	mm := m.then(itemDisplay(item.ID), scaleM(0.5, 0.5, 0.5))
	for _, q := range h.sprite(g.res.items, tile) {
		v = emitFP(v, cb, mm, q, white, light)
	}
	return []handPass{{v, g.hud.itemTex}}
}

// drawHand draws the hand over the world. Nothing is drawn in the third-person views.
func (g *game) drawHand(w, hgt int) {
	if !g.inGame || g.perspective != 0 || g.net != nil && g.health <= 0 {
		return
	}
	passes := g.handGeometry(g.partial)
	if len(passes) == 0 || len(passes[0].verts) == 0 {
		return
	}
	proj := perspective(handFOV*math.Pi/180, float32(w)/float32(hgt), handNear, handFar)
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	gl.UseProgram(g.prog)
	gl.UniformMatrix4fv(g.uMVP, 1, false, &proj[0])
	gl.Uniform1i(g.uDynCount, 0)
	gl.Uniform1f(g.uFogS, 1e6)
	gl.Uniform1f(g.uFogE, 2e6)
	gl.Uniform1f(g.uAlpha, 1)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.BLEND)
	gl.ActiveTexture(gl.TEXTURE0)
	for _, p := range passes {
		gl.BindTexture(gl.TEXTURE_2D, p.tex)
		g.hand.mesh.draw(p.verts)
	}
	gl.Enable(gl.CULL_FACE)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
}

// handScreenshot sets the hand for a screenshot from the -hand option: "swing=3,bow=15,eat=10,block"
// puts the arm three ticks into a swing, a bow drawn for 15 ticks, and so on. The hand starts
// fully raised, since a screenshot runs no ticks.
func (g *game) handScreenshot(spec string) {
	h := &g.hand
	h.item, h.equip, h.prevEquip = g.held(), 1, 1
	for _, kv := range strings.Split(spec, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(kv), "=")
		n, _ := strconv.Atoi(v)
		switch k {
		case "swing":
			g.swingTicks = swingTicks - n
		case "bow":
			g.bowTicks = n
		case "eat":
			g.eating = n
		case "block":
			g.blocking = true
		}
	}
}
