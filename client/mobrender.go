package client

import (
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// drawMobs draws every mob with its model, grouped by texture.
func (g *game) drawMobs() {
	if g.entGroups == nil {
		g.entGroups = map[string][]vertex{}
	}
	if g.net != nil {
		if g.skinDone == nil {
			g.skinDone = make(chan skinResult, 32)
		}
		g.pumpSkins()
		g.pumpMaps()
	}
	groups := g.entGroups // the same buffers every frame: nothing is allocated once they have grown
	for k := range groups {
		groups[k] = groups[k][:0]
	}
	g.emitChests(groups)
	g.emitSigns(groups)
	g.emitSkulls(groups)
	g.emitSpawners(groups)
	g.emitBolts(groups)
	g.emitLeashes(groups)
	if len(g.mobs) == 0 && len(g.arrows) == 0 && g.perspective == 0 && (g.net == nil || len(g.net.remote) == 0 && len(g.net.ents) == 0) && len(groups) == 0 {
		return
	}
	if g.perspective != 0 {
		g.emitSelf(groups)
	}
	partial := g.partial
	for _, m := range g.mobs {
		g.emitMob(groups, m, partial)
	}
	if g.net != nil {
		for _, e := range g.net.ents {
			if e.mob != nil && !e.invisible && !g.offScreen(e.pos, 3) {
				g.emitMob(groups, e.mob.m, partial)
			}
			if e.stand != nil && !g.offScreen(e.pos, 3) {
				g.emitStand(groups, e, e.small)
			}
			if e.painting != nil && !g.offScreen(e.pos, 4) {
				g.emitPainting(groups, e)
			}
			if e.frame != nil && !g.offScreen(e.pos, 2) {
				g.emitFrame(groups, e)
			}
		}
	}
	if g.net != nil {
		for _, p := range g.net.remote {
			if g.offScreen(p.pos, 3) {
				continue
			}
			g.emitRemote(groups, p)
		}
	}
	g.emitHooks(groups)
	g.emitProjectiles(groups)
	for _, a := range g.arrows {
		pos := a.prev.add(a.pos.sub(a.prev).scale(float64(partial)))
		l := g.world.Light(int32(math.Floor(float64(pos[0]))), int32(math.Floor(float64(pos[1]))), int32(math.Floor(float64(pos[2]))))
		groups["arrow"] = arrowGeometry(groups["arrow"], a, partial, l)
	}
	gl.Disable(gl.CULL_FACE)
	gl.Enable(gl.BLEND) // a slime's outer body is see-through
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	for path, verts := range groups {
		tex, ok := g.mobTex[path]
		switch path { // what is held is made of the item and block atlases
		case "!items":
			tex, ok = g.hud.itemTex, true
		case "!blocks":
			tex, ok = g.tex, true
		case "!font": // the text of signs
			tex, ok = g.hud.font, true
		case "!bolt": // lightning is drawn last, added to what is behind it
			continue
		case "!leash": // the rope: flat colours
			tex, ok = g.hud.white, true
		}
		if !ok || len(verts) == 0 {
			continue
		}
		gl.BindTexture(gl.TEXTURE_2D, tex)
		g.entMobs.draw(verts)
	}
	if verts := groups["!bolt"]; len(verts) > 0 {
		gl.BlendFunc(gl.ONE, gl.ONE) // blendFunc(SRC_ALPHA, ONE) at alpha 0.3, with the alpha in the colour
		gl.BindTexture(gl.TEXTURE_2D, g.hud.white)
		g.entMobs.draw(verts)
	}
	gl.Disable(gl.BLEND)
	gl.Enable(gl.CULL_FACE)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
}

// emitMob lays out one mob with its model, between its last tick and this one.
func (g *game) emitMob(groups map[string][]vertex, m *mob, partial float32) {
	k := m.kind
	pos := m.prev.add(m.pos.sub(m.prev).scale(float64(partial)))
	yaw := m.prevYaw + float32(math.Mod(float64(m.yaw-m.prevYaw)+540, 360)-180)*partial
	age := float32(m.age) + partial
	ls := m.limbSwing - m.limbAmount*(1-partial)
	lsa := m.limbAmount
	rots := k.pose(m, ls, lsa, age, m.headYaw, m.pitch)
	light := g.world.Light(int32(math.Floor(float64(pos[0]))), int32(math.Floor(float64(pos[1]+0.5))), int32(math.Floor(float64(pos[2]))))
	if k.fullbright {
		light = 0xFF
	}
	tint := [3]uint8{255, 255, 255}
	if m.hurtTime > 0 || m.dead() {
		tint = [3]uint8{255, 110, 110}
	}
	tilt := float32(0)
	if m.dead() {
		f := (float32(m.deathTime) + partial - 1) / 20 * 1.6
		f = float32(math.Sqrt(math.Max(0, float64(f))))
		tilt = min(f, 1) * math.Pi / 2
	}
	sx, sy := float32(1), float32(1) // its size: spiders, slimes and a creeper about to blow
	switch {
	case k.slime:
		sx = float32(max(m.size, 1))
		sy = sx
	case k.scale != 0:
		sx, sy = k.scale, k.scale
	case k.id == "Creeper" && m.fuse > 0: // swells as it is about to blow
		f := float32(m.fuse) / 28
		f *= f
		f *= f
		sx, sy = 1+f*0.4, 1+f*0.1
	}
	if m.burning > 0 && m.hurtTime == 0 && !m.dead() {
		tint = [3]uint8{255, 170, 90}
	}
	tex := k.tex
	if len(k.texVariants) > 0 {
		tex = k.texVariants[m.colour%len(k.texVariants)]
	}
	if m.tex != "" {
		tex = m.tex
	}
	if m.drawScale != 0 {
		sx, sy = sx*m.drawScale, sy*m.drawScale
	}
	pos[1] += float64(k.yoff)
	start := len(groups[tex])
	groups[tex] = emitModel(groups[tex], k.model, rots, pos, yaw, tilt, tint, light)
	scaleVerts(groups[tex][start:], pos, sx, sy)
	g.emitMobGear(groups, m, rots, pos, yaw, tilt, tint, light, sx, sy)
	if k.id == "SnowMan" && !m.sheared && len(rots) > 0 {
		start := len(groups["!blocks"])
		g.emitSnowmanPumpkin(groups, m, rots[0], pos, yaw, light)
		scaleVerts(groups["!blocks"][start:], pos, sx, sy)
	}
	if k.fm != nil && !m.sheared {
		c := fleece[m.colour&15]
		start := len(groups[k.fur])
		groups[k.fur] = emitModel(groups[k.fur], k.fm, rots, pos, yaw, tilt, [3]uint8{uint8(int(c[0]) * int(tint[0]) / 255), uint8(int(c[1]) * int(tint[1]) / 255), uint8(int(c[2]) * int(tint[2]) / 255)}, light)
		scaleVerts(groups[k.fur][start:], pos, sx, sy)
	}
}

// emitMobGear adds what a server dressed a biped mob in (a zombie's or skeleton's armour) and what it holds in
// its right hand, on its posed model: the armour is ModelZombie's or ModelSkeleton's at the two sizes, the item
// hangs from the right arm as it does from a player's.
func (g *game) emitMobGear(groups map[string][]vertex, m *mob, rots []rot, pos vec3, yaw, tilt float32, tint [3]uint8, light uint8, sx, sy float32) {
	k := m.kind
	if (k.id != "Zombie" && k.id != "Skeleton") || len(rots) < 6 {
		return
	}
	var a [12]rot // the armour model's six parts, in the player's numbering
	a[pHead], a[pBody], a[pRArm], a[pLArm], a[pRLeg], a[pLLeg] = rots[0], rots[1], rots[2], rots[3], rots[4], rots[5]
	worn := false
	for _, it := range m.armor {
		worn = worn || !it.empty()
	}
	if worn {
		tmp := map[string][]vertex{}
		emitArmorScaled(tmp, m.armor, m.armorX, a[:], pos, yaw, tilt, tint, light, armorScale{body: 1, thin: k.id == "Skeleton"})
		for key, vs := range tmp {
			scaleVerts(vs, pos, sx, sy)
			groups[key] = append(groups[key], vs...)
		}
	}
	if !m.held.empty() && !m.dead() {
		d := pos.sub(g.camPos)
		g.emitHeldWith(groups, m.held, func(display mat4) mat4 { return standHeldMatrix(rots[2], yaw, false, display) }, pos, false, 0, false, light, d.dot(d) > 24*24, false)
	}
}

// scaleVerts makes a mob's vertices bigger about its feet: sideways by sx, upwards by sy.
func scaleVerts(vs []vertex, pos vec3, sx, sy float32) {
	if sx == 1 && sy == 1 {
		return
	}
	for i := range vs {
		vs[i].X = float32(pos[0] + (float64(vs[i].X)-pos[0])*float64(sx))
		vs[i].Y = float32(pos[1] + (float64(vs[i].Y)-pos[1])*float64(sy))
		vs[i].Z = float32(pos[2] + (float64(vs[i].Z)-pos[2])*float64(sx))
	}
}

// emitSelf lays out the player's own model for the third-person views.
func (g *game) emitSelf(groups map[string][]vertex) {
	partial := g.partial
	pos := g.prevFeet.add(g.feet.sub(g.prevFeet).scale(float64(partial)))
	body := g.prevBodyYaw + float32(math.Mod(float64(g.bodyYaw-g.prevBodyYaw)+540, 360)-180)*partial
	head := float32(math.Mod(float64(g.yawDeg()-body)+540, 360) - 180)
	fwd := vec3{float64(-mhSin(g.yawDeg() * deg)), 0, float64(mhCos(g.yawDeg() * deg))}
	st := poseState{
		ls: g.pLimbSwing - g.pLimbAmount*(1-partial), lsa: g.pLimbAmount, age: float32(g.worldTime) + partial,
		headYaw: head, headPitch: g.pitch * 180 / math.Pi, sneaking: g.sneaking, swing: -1, airborne: !g.onGround && !g.flying || g.flying,
		speedFwd: float32(g.vel.dot(fwd)), blocking: g.blocking, holding: !g.held().empty(), aiming: g.bowTicks > 0,
	}
	if g.swingTicks > 0 {
		st.swing = (6 - float32(g.swingTicks) + partial) / 6
	}
	l := g.world.Light(int32(math.Floor(float64(pos[0]))), int32(math.Floor(float64(pos[1]+0.5))), int32(math.Floor(float64(pos[2]))))
	tint := [3]uint8{255, 255, 255}
	if g.hurtResist > 10 {
		tint = [3]uint8{255, 110, 110}
	}
	rots := playerRots(st)
	groups["look/skin"] = emitModelScaled(groups["look/skin"], playerMdl, rots, nil, pos, body, 0, tint, l, playerScale)
	var extras [4]itemExtra
	if g.net != nil {
		extras = [4]itemExtra{g.net.extra[8], g.net.extra[7], g.net.extra[6], g.net.extra[5]}
	}
	emitArmor(groups, [4]slot{g.armor[3], g.armor[2], g.armor[1], g.armor[0]}, extras, rots, pos, body, 0, tint, l, playerScale)
	groups["look/cape"] = emitModelScaled(groups["look/cape"], capeMdl, []rot{capeRot(st)}, nil, pos, body, 0, [3]uint8{255, 255, 255}, l, playerScale)
	groups["look/wings"] = emitModelScaled(groups["look/wings"], wingMdl, wingRots(st), nil, pos, body, 0, [3]uint8{255, 255, 255}, l, playerScale)
	hook := g.net != nil && g.net.hookOf(g.net.eid) != nil
	g.emitHeld(groups, g.held(), rots, pos, body, g.sneaking, false, g.bowTicks > 0, g.bowTicks, hook, l, false)
}

// playerScale is RenderPlayer.preRenderCallback's 0.9375: a player's model, and what it wears, is that much smaller
// than the boxes it is made of (the model is 1.875 tall, not two blocks).
const playerScale = 0.9375

// swordIDs are the swords: held and in use, the arm goes across the chest.
var swordIDs = idSet(267, 268, 272, 276, 283)

// emitRemote lays out another player of the server with the player model, the armour they wear and what
// they hold.
func (g *game) emitRemote(groups map[string][]vertex, p *remotePlayer) {
	partial := g.partial
	pos := p.prev.add(p.pos.sub(p.prev).scale(float64(partial)))
	body := p.prevBody + wrapDeg(p.body-p.prevBody)*partial
	head := p.prevHead + wrapDeg(p.headYaw-p.prevHead)*partial // the angles are interpolated between ticks too
	pitch := p.prevPitch + (p.pitch-p.prevPitch)*partial
	st := poseState{
		ls: p.ls - p.lsa*(1-partial), lsa: p.lsa, age: float32(g.worldTime) + partial,
		headYaw: wrapDeg(head - body), headPitch: pitch, swing: -1, sneaking: p.sneaking,
		holding: !p.held.empty(), blocking: p.using && swordIDs[p.held.ID], aiming: p.using && p.held.ID == 261,
	}
	if p.swing > 0 {
		st.swing = (6 - float32(p.swing) + partial) / 6
	}
	l := g.world.Light(int32(math.Floor(pos[0])), int32(math.Floor(pos[1]+0.5)), int32(math.Floor(pos[2])))
	tint := [3]uint8{255, 255, 255}
	if p.hurt > 0 || p.dead {
		tint = [3]uint8{255, 110, 110}
	}
	tilt := float32(0)
	if p.dead { // falls over as a mob does when it dies
		f := float32(math.Sqrt(math.Max(0, float64((float32(p.deathTime)+partial-1)/20*1.6))))
		tilt = min(f, 1) * math.Pi / 2
	}
	var rb [12]rot
	playerRotsInto(rb[:], st)
	rots := rb[:]
	skin, slim := g.skinFor(p)
	if !p.invisible {
		mdl := playerMdl
		if slim {
			mdl = playerSlim
		}
		groups[skin] = emitModelScaled(groups[skin], mdl, rots, nil, pos, body, tilt, tint, l, playerScale)
	}
	emitArmor(groups, p.armor, p.armorX, rots, pos, body, tilt, tint, l, playerScale)
	if !p.dead {
		hook := g.net != nil && g.net.hookOf(p.eid) != nil
		d := pos.sub(g.camPos)
		g.emitHeld(groups, p.held, rots, pos, body, p.sneaking, slim, p.using, p.useTicks, hook, l, d.dot(d) > 24*24)
	}
}

// offScreen is whether something at p, about r blocks across, cannot be seen: behind the camera, or farther
// than the world is drawn. A player is not worth building a model for then.
func (g *game) offScreen(p vec3, r float64) bool {
	d := p.sub(g.camPos)
	if d.dot(g.camDir) < -r {
		return true
	}
	far := float64(g.viewDist()*16) + 32
	return d.dot(d) > far*far
}
