package client

import "math"

// Armor stands, after 1.8's ModelArmorStand (a ModelArmorStandArmor, which is a ModelBiped) and
// RenderArmorStand. The boxes, pivots and which part follows which rotation were read off the game's own
// bytecode: the head hangs from (0,1,0), the legs from y=12, the plate and the three rods of the stand turn
// with the body (the plate against the stand's yaw, so it keeps facing the same way). A small stand is
// drawn as a child model: the head at 0.75 and the rest at half size.

// The parts of the stand's model, in the order its boxes are listed.
const (
	sHead = iota
	sBody
	sRArm
	sLArm
	sRLeg
	sLLeg
	sRodR
	sRodL
	sWaist
	sBase
)

var standModel = newModel(64, 64,
	part(0, 1, 0, box(0, 0, -1, -7, -1, 2, 7, 2)),
	part(0, 0, 0, box(0, 26, -6, 0, -1.5, 12, 3, 3)),
	part(-5, 2, 0, box(24, 0, -2, -2, -1, 2, 12, 2)),
	part(5, 2, 0, mirrored(box(32, 16, 0, -2, -1, 2, 12, 2))),
	part(-1.9, 12, 0, box(8, 0, -1, 0, -1, 2, 11, 2)),
	part(1.9, 12, 0, mirrored(box(40, 16, -1, 0, -1, 2, 11, 2))),
	part(0, 0, 0, box(16, 0, -3, 3, -1, 2, 7, 2)),
	part(0, 0, 0, box(48, 16, 1, 3, -1, 2, 7, 2)),
	part(0, 0, 0, box(0, 48, -4, 10, -1, 8, 2, 2)),
	part(0, 12, 0, box(0, 32, -6, 11, -6, 12, 1, 12)),
)

// standState is what a server tells about an armor stand: its pose (EntityArmorStand's six rotations, in
// degrees: head, body, left arm, right arm, left leg, right leg), its flags, which way it faces and what it
// holds and wears.
type standState struct {
	pose           [6][3]float32
	arms, noBase   bool
	yaw, prevYaw   float32 // rotationYaw: the body follows it (updateDistance), and the server turns it over three ticks
	tyaw           float32
	held           slot
	heldX          itemExtra
	armor          [4]slot // boots, leggings, chestplate, helmet
	armorX         [4]itemExtra
	hurtTicks, age int
}

// newStand is a stand as the game spawns it: the default pose of EntityArmorStand.
func newStand(yaw float32) *standState {
	return &standState{
		pose: [6][3]float32{{0, 0, 0}, {0, 0, 0}, {-10, 0, -10}, {-15, 0, 10}, {-1, 0, -1}, {1, 0, 1}},
		yaw:  yaw, prevYaw: yaw, tyaw: yaw,
	}
}

// apply takes what the stand's metadata says: flags (4 arms, 8 no base plate) and the six rotations
// (entries 11 to 16).
func (s *standState) apply(m map[int]any) {
	if v, ok := m[10].(int); ok {
		s.arms, s.noBase = v&4 != 0, v&8 != 0
	}
	for i := 0; i < 6; i++ {
		if v, ok := m[11+i].([3]float32); ok {
			s.pose[i] = v
		}
	}
}

// tick is one tick of a stand on the client: the interpolation of onLivingUpdate takes its yaw a step towards
// the server's. inc is the ticks the entity has left to arrive.
func (s *standState) tick(inc int) {
	s.prevYaw = s.yaw
	s.age++
	if s.hurtTicks > 0 {
		s.hurtTicks--
	}
	if inc > 0 {
		s.yaw += wrapDeg(s.tyaw-s.yaw) / float32(inc)
	}
}

// look is the server turning the stand.
func (e *netEntity) look(yaw, pitch float32) {
	switch {
	case e.mob != nil:
		e.mob.look(yaw, pitch)
	case e.stand != nil:
		e.stand.tyaw = yaw
	}
}

const degToRad = 0.017453292

func (s *standState) rots() [10]rot {
	var r [10]rot
	at := func(k int) rot {
		p := s.pose[k]
		return rot{x: degToRad * p[0], y: degToRad * p[1], z: degToRad * p[2]}
	}
	r[sHead], r[sBody] = at(0), at(1)
	r[sLArm], r[sRArm] = at(2), at(3)
	r[sLLeg], r[sRLeg] = at(4), at(5)
	r[sRodR], r[sRodL], r[sWaist] = r[sBody], r[sBody], r[sBody]
	r[sBase] = rot{y: degToRad * -s.yaw}
	return r
}

// Where the parts of a small stand are put (ModelBiped.render with isChild): the head at 1.5/2 of its size
// lowered by 16 pixels, the rest at 1/2 lowered by 24; the scale is about the feet, so what is left over of the
// translation is a shift of the stand's origin.
const (
	childHeadScale, childHeadShift = 0.75, 0.7578125 - 0.75*1.5078125
	childBodyScale, childBodyShift = 0.5, 0.7578125 - 0.5*1.5078125
)

// mask is which parts of the stand are drawn: the arms only when it has them, the plate unless it is
// without one. head picks the head alone, otherwise the head is left out (a small stand draws them apart).
func (s *standState) mask(head bool) []bool {
	m := make([]bool, 10)
	for i := range m {
		m[i] = !head
	}
	m[sRArm], m[sLArm] = m[sRArm] && s.arms, m[sLArm] && s.arms
	m[sBase] = m[sBase] && !s.noBase
	m[sHead] = head
	return m
}

// all is every part that is drawn.
func (s *standState) all() []bool {
	m := s.mask(false)
	m[sHead] = true
	return m
}

// emitStand adds an armor stand: its model unless it is invisible, the armour it wears and what it holds.
func (g *game) emitStand(groups map[string][]vertex, e *netEntity, small bool) {
	s := e.stand
	partial := g.partial
	pos := e.at(partial)
	yaw := s.prevYaw + wrapDeg(s.yaw-s.prevYaw)*partial
	l := g.world.Light(int32(math.Floor(pos[0])), int32(math.Floor(pos[1]+0.5)), int32(math.Floor(pos[2])))
	tint := [3]uint8{255, 255, 255}
	if s.hurtTicks > 0 {
		tint = [3]uint8{255, 110, 110}
	}
	rots := s.rots()
	if !e.invisible {
		const tex = "armorstand/wood"
		if small {
			groups[tex] = emitModelScaled(groups[tex], standModel, rots[:], s.mask(true), pos.add(vec3{0, childHeadShift, 0}), yaw, 0, tint, l, childHeadScale)
			groups[tex] = emitModelScaled(groups[tex], standModel, rots[:], s.mask(false), pos.add(vec3{0, childBodyShift, 0}), yaw, 0, tint, l, childBodyScale)
		} else {
			groups[tex] = emitModelScaled(groups[tex], standModel, rots[:], s.all(), pos, yaw, 0, tint, l, 1)
		}
	}
	// the armour is ModelArmorStandArmor: a biped whose head hangs one pixel lower and whose legs one higher
	ar := [12]rot{}
	ar[pHead], ar[pBody], ar[pRArm], ar[pLArm], ar[pRLeg], ar[pLLeg] = rots[sHead], rots[sBody], rots[sRArm], rots[sLArm], rots[sRLeg], rots[sLLeg]
	ar[pHead].dy, ar[pRLeg].dy, ar[pLLeg].dy = 1, -1, -1
	sc := armorScale{body: 1}
	if small {
		sc = armorScale{split: true, head: childHeadScale, headDY: childHeadShift, body: childBodyScale, bodyDY: childBodyShift}
	}
	emitArmorScaled(groups, s.armor, s.armorX, ar[:], pos, yaw, 0, tint, l, sc)
	if !s.held.empty() {
		d := pos.sub(g.camPos)
		g.emitHeldWith(groups, s.held, func(display mat4) mat4 { return standHeldMatrix(rots[sRArm], yaw, small, display) }, pos, false, 0, false, l, d.dot(d) > 24*24)
	}
}

// standHeldMatrix is heldMatrix for a stand: the same right arm and hand, an entity of full size, and for a
// small one the extra turn LayerHeldItem gives a child model.
func standHeldMatrix(r rot, yaw float32, small bool, display mat4) mat4 {
	const rad2deg = 180 / math.Pi
	arm := translateM((-5+r.dx)/16, (2+r.dy)/16, r.dz/16).then(rotZM(r.z*rad2deg), rotYM(r.y*rad2deg), rotXM(r.x*rad2deg))
	m := rotYM(180-yaw).then(scaleM(-1, -1, 1), translateM(0, -1.5078125, 0))
	if small {
		m = m.then(translateM(0, 0.625, 0), rotXM(20), scaleM(0.5, 0.5, 0.5))
	}
	return m.then(arm, translateM(-0.0625, 0.4375, 0.0625), display, scaleM(0.5, 0.5, 0.5))
}
