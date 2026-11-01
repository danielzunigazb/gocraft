package client

import (
	"math"
	"strconv"

	"gocraft/proto"
)

// Entities the server shows that gocraft has no model for still matter when they carry a name: the
// holograms of a lobby (armor stands with a custom name) and the names over players. They are tracked
// by the packets that spawn, move and describe them, and drawn as text hanging in the world.

type netEntity struct {
	eid          int32
	kind         byte  // the mob type
	obj          byte  // the object type, for what the server spawned as an object (a fishing hook, an arrow); 0 for a mob
	owner        int32 // an object's data: the entity that cast it
	prev, target vec3  // where it was a tick ago, and where the server last put it
	inc          int   // ticks left to reach target
	vel          vec3  // blocks a tick, for what flies
	stuck        bool  // an arrow in a block
	pos          vec3
	name         []styledRun
	showName     bool
	invisible    bool
	small        bool
	marker       bool
	mob          *netMob // a mob gocraft has a model for
	health       float32 // metadata 6, for what shows a health bar
	healthSet    bool
	orb          int // an experience orb's value
	age          int // ticks since it appeared: an orb's colour pulses with it
}

const (
	mobArmorStand = 30
	tagDistance   = 48.0 // blocks; farther names are not drawn
)

// height is the entity's height as the game has it, which is what its name floats over.
func (e *netEntity) height() float64 {
	if e.mob != nil {
		_, h := e.mob.dims()
		return float64(h)
	}
	if e.kind != mobArmorStand {
		return 1.8
	}
	switch {
	case e.marker:
		return 0
	case e.small:
		return 0.9875
	}
	return 1.975
}

// moveTo is the server putting the entity somewhere: it gets there in three ticks, five for a hook
// (Entity.setPositionAndRotation2 and EntityFishHook's two more).
func (e *netEntity) moveTo(t vec3) {
	if e.obj != 0 && e.obj != objFishHook { // other objects are put where the server says at once
		e.pos, e.target, e.inc = t, t, 0
		return
	}
	e.target, e.inc = t, 3
	if e.obj == objFishHook {
		e.inc = 5
	}
}

// tickEnts moves the entities one tick towards where the server put them, as the game does.
func (g *game) tickEnts() {
	for _, e := range g.net.ents {
		e.prev = e.pos
		e.age++
		if e.mob != nil {
			tickNetMob(e)
			continue
		}
		if isProjectile(e.obj) {
			g.tickProjectile(e)
			e.target = e.pos // the server's next position is relative to where this one is
			continue
		}
		if e.inc > 0 {
			e.pos = e.pos.add(e.target.sub(e.pos).scale(1 / float64(e.inc)))
			e.inc--
		}
	}
}

// readMeta reads an entity metadata list (1.8): entries of (type<<5 | index) and a value, up to 0x7F.
func readMeta(r *proto.Reader) (map[int]any, bool) {
	m := map[int]any{}
	for i := 0; i < 64; i++ {
		b := r.Byte()
		if r.Err() != nil {
			return m, false
		}
		if b == 0x7F {
			return m, true
		}
		idx := int(b & 0x1F)
		switch b >> 5 {
		case 0:
			m[idx] = int(int8(r.Byte()))
		case 1:
			m[idx] = int(r.Short())
		case 2:
			m[idx] = int(r.Int())
		case 3:
			m[idx] = r.Float()
		case 4:
			m[idx] = r.String(32767)
		case 5:
			s, ok := readSlotData(r)
			if !ok {
				return m, false
			}
			m[idx] = s
		case 6:
			r.Int()
			r.Int()
			r.Int()
		case 7:
			r.Float()
			r.Float()
			r.Float()
		}
	}
	return m, false
}

// apply takes what a metadata list says about the entity: its flags, name and whether the name shows.
func (e *netEntity) apply(m map[int]any) {
	if v, ok := m[0].(int); ok {
		e.invisible = v&0x20 != 0
	}
	if s, ok := m[2].(string); ok {
		e.name = legacyRuns(s, defaultStyle)
	}
	if v, ok := m[3].(int); ok {
		e.showName = v != 0
	}
	if e.kind == mobArmorStand {
		if v, ok := m[10].(int); ok {
			e.small, e.marker = v&1 != 0, v&16 != 0
		}
	}
	if v, ok := m[6].(float32); ok {
		e.health, e.healthSet = v, true
	}
	if e.mob != nil {
		e.mob.apply(m)
	}
}

// box is what a click hits: a mob's own box, or a player-sized one for what gocraft has no model for.
func (e *netEntity) box() aabb {
	hw := 0.3
	if e.mob != nil {
		w, _ := e.mob.dims()
		hw = float64(w / 2)
	}
	return aabb{e.pos.sub(vec3{hw, 0, hw}), e.pos.add(vec3{hw, e.height(), hw})}
}

func (n *netState) entity(eid int32) *netEntity {
	if n.ents == nil {
		return nil
	}
	return n.ents[eid]
}

func (n *netState) addEntity(e *netEntity) {
	if n.ents == nil {
		n.ents = map[int32]*netEntity{}
	}
	if len(n.ents) < 4096 {
		n.ents[e.eid] = e
	}
}

// projectPoint puts a world position on the screen: its pixel and how far along the view it is.
func projectPoint(m *mat4, p vec3, w, h int) (sx, sy, depth float32, ok bool) {
	x, y, z := float32(p[0]), float32(p[1]), float32(p[2])
	cx := m[0]*x + m[4]*y + m[8]*z + m[12]
	cy := m[1]*x + m[5]*y + m[9]*z + m[13]
	cw := m[3]*x + m[7]*y + m[11]*z + m[15]
	if cw < 0.1 {
		return 0, 0, 0, false
	}
	return (cx/cw*0.5 + 0.5) * float32(w), (1 - (cy/cw*0.5 + 0.5)) * float32(h), cw, true
}

// drawTags draws the names of the entities and players the server shows, over their heads.
func (g *game) drawTags(mvp *mat4, w, h int) {
	n := g.net
	if n == nil {
		return
	}
	g.enableUI()
	tanHalf := float32(math.Tan(float64(g.opts.FOV) * math.Pi / 360))
	tag := func(runs []styledRun, at vec3) {
		if len(runs) == 0 || at.sub(g.camPos).dot(at.sub(g.camPos)) > tagDistance*tagDistance {
			return
		}
		sx, sy, d, ok := projectPoint(mvp, at, w, h)
		if !ok {
			return
		}
		s := 0.02666667 * float32(h) / 2 / (tanHalf * d) // the game's 0.0266667 blocks per font pixel
		if s < 0.35 {
			return
		}
		tw := float32(g.runsWidth(runs)) * s
		g.hud.rect(sx-tw/2-s, sy-9*s, tw+2*s, 9*s, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 0.25})
		g.hud.flush(g.hud.white)
		g.drawRuns(runs, sx-tw/2, sy-8*s, s, 1, true)
	}
	for _, e := range n.ents {
		if e.showName && len(e.name) > 0 {
			tag(e.name, e.at(g.partial).add(vec3{0, e.height() + 0.5, 0}))
		}
	}
	for _, p := range n.remote {
		for _, l := range n.playerLabels(p, g.camPos) {
			tag(l.runs, l.at)
		}
	}
}

// nameLine is the height of a line of a player's label, in blocks: the font's 9 pixels times 1.15 at 0.02666667
// blocks a pixel (RenderPlayer.renderOffsetLivingLabel).
const nameLine = 9 * 1.15 * 0.02666667

type tagLabel struct {
	runs []styledRun
	at   vec3
}

// playerLabels are the lines over a player: the score of the objective shown under names, when the player is
// within 10 blocks, and above it the name with its team's prefix and suffix.
func (n *netState) playerLabels(p *remotePlayer, cam vec3) []tagLabel {
	name := p.name
	if name == "" {
		name = n.names[p.uuid]
	}
	if name == "" {
		return nil
	}
	at := p.pos.add(vec3{0, 1.8 + 0.5, 0})
	var out []tagLabel
	if sb := n.sb; sb != nil {
		if o := sb.objectives[sb.slots[2]]; o != nil {
			if d := p.pos.sub(cam); d.dot(d) < 100 {
				out = append(out, tagLabel{legacyRuns(strconv.Itoa(int(sb.scores[o.name][name]))+" "+o.display, defaultStyle), at})
				at = at.add(vec3{0, nameLine, 0})
			}
		}
	}
	return append(out, tagLabel{legacyRuns(n.sb.format(name), defaultStyle), at})
}
