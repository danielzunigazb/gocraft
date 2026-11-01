package client

import (
	"math"
)

// Mobs on a server, after 1.8's client: Spawn Mob gives the type, where it is and which way it and its head
// face; the entity packets move it there over three ticks (EntityLivingBase.setPositionAndRotation2 and
// onLivingUpdate), the body follows the head as EntityBodyHelper turns it, the legs swing with how far it
// went, and metadata says what kind of sheep, slime or villager it is. They are drawn with gocraft's own
// models; a type gocraft has none for keeps only its name (a lobby's holograms are armor stands).

// netMobTypes are the mob type ids of 1.8 (EntityList) gocraft has a model for, by the kind that draws them.
var netMobTypes = map[byte]string{
	50: "Creeper", 51: "Skeleton", 52: "Spider", 54: "Zombie", 55: "Slime", 57: "Zombie", 59: "CaveSpider",
	65: "Bat", 66: "Witch", 90: "Pig", 91: "Sheep", 92: "Cow", 93: "Chicken", 94: "Squid", 101: "Rabbit", 120: "Villager",
	58: "Enderman", 61: "Blaze", 95: "Wolf", 99: "VillagerGolem",
}

const (
	mobPigZombie = 57
	creeperFuse  = 30 // EntityCreeper.fuseTime
)

// netMobTextures are the textures of the server's mobs that gocraft's own do not use.
var netMobTextures = []string{"zombie_pigman", "skeleton/wither_skeleton", "wolf/wolf_tame", "wolf/wolf_angry"}

// netMob is the drawn part of a server's mob: a gocraft mob that the server moves.
type netMob struct {
	m                *mob
	yaw, pitch       float32 // rotationYaw and rotationPitch, which the server's packets turn over the same three ticks
	tyaw, tpitch     float32
	head             float32 // rotationYawHead: Entity Head Look sets it at once
	bodyHead         float32 // EntityBodyHelper.prevRenderYawHead
	bodyTicks        int     // EntityBodyHelper.rotationTickCounter
	creeperState     int     // -1 idle, 1 about to blow
	baby, witherSkel bool
}

// newNetMob makes the mob for a Spawn Mob of a type gocraft can draw, or nil.
func newNetMob(kind byte, at vec3, yaw, pitch, head float32) *netMob {
	k := kinds[netMobTypes[kind]]
	if k == nil {
		return nil
	}
	m := &mob{kind: k, pos: at, prev: at, yaw: head, prevYaw: head, health: k.health, onGround: true, size: 1}
	if kind == mobPigZombie {
		m.tex = "zombie_pigman"
	}
	return &netMob{m: m, yaw: yaw, pitch: pitch, tyaw: yaw, tpitch: pitch, head: head, bodyHead: head, creeperState: -1}
}

// applyMob takes what a mob's metadata says: its health, whether it is a baby, and what each kind keeps
// (a sheep's fleece, a slime's size, a villager's trade, a rabbit's coat, a skeleton's kind, a creeper's fuse).
func (n *netMob) apply(meta map[int]any) {
	m := n.m
	if v, ok := meta[6].(float32); ok { // the health is the metadata's: 0 is a dead mob
		m.health = v
	}
	if v, ok := meta[12].(int); ok { // EntityAgeable's age (below 0 a baby), and a zombie's child flag
		n.baby = v < 0 || m.kind.id == "Zombie" && v == 1
	}
	switch m.kind.id {
	case "Sheep":
		if v, ok := meta[16].(int); ok {
			m.colour, m.sheared = v&15, v&16 != 0
		}
	case "Slime":
		if v, ok := meta[16].(int); ok {
			m.size = max(v, 1)
		}
	case "Villager":
		if v, ok := meta[16].(int); ok {
			m.colour = max(v, 0)
		}
	case "Rabbit":
		if v, ok := meta[18].(int); ok && v >= 0 && v <= 5 {
			m.colour = v
		}
	case "Skeleton":
		if v, ok := meta[13].(int); ok {
			n.witherSkel = v == 1
			m.tex = ""
			if n.witherSkel {
				m.tex = "skeleton/wither_skeleton"
			}
		}
	case "Creeper":
		if v, ok := meta[16].(int); ok {
			n.creeperState = v
		}
	case "Wolf": // flags: 1 sitting, 2 angry, 4 tamed
		if v, ok := meta[16].(int); ok {
			m.angry = v&2 != 0
			switch {
			case v&2 != 0:
				m.tex = "wolf/wolf_angry"
			case v&4 != 0:
				m.tex = "wolf/wolf_tame"
			default:
				m.tex = ""
			}
		}
	}
	m.drawScale = n.drawScale()
}

// drawScale is how much bigger than its model the mob is drawn: a baby is half, a wither skeleton 1.2.
func (n *netMob) drawScale() float32 {
	s := float32(1)
	if n.witherSkel {
		s = 1.2
	}
	if n.baby {
		s *= 0.5
	}
	return s
}

// dims is the size of the mob's box, as the game has it for its kind, size and age.
func (n *netMob) dims() (w, h float32) {
	w, h = n.m.dims()
	if n.witherSkel {
		w, h = 0.72, 2.535 // EntitySkeleton.setSkeletonType(1): setSize(0.72F, 2.535F)
	}
	if n.baby {
		w, h = w/2, h/2
	}
	return w, h
}

// look is the server turning the mob (a look, a move with a look, a teleport); the move that goes with it
// was given to the entity already.
func (n *netMob) look(yaw, pitch float32) {
	n.tyaw, n.tpitch = yaw, pitch
}

// tickNetMob is one tick of a server's mob on the client: onEntityUpdate keeps the last position, the
// interpolation of onLivingUpdate takes it a step towards the server's, moveEntityWithHeading swings the
// legs and EntityBodyHelper turns the body.
func tickNetMob(e *netEntity) {
	n, m := e.mob, e.mob.m
	m.prev, m.prevYaw = m.pos, m.yaw
	m.age++
	if e.inc > 0 {
		k := float32(e.inc)
		e.pos = e.pos.add(e.target.sub(e.pos).scale(1 / float64(e.inc)))
		n.yaw += wrapDeg(n.tyaw-n.yaw) / k
		n.pitch += (n.tpitch - n.pitch) / k
		e.inc--
	}
	m.pos = e.pos
	if m.hurtTime > 0 {
		m.hurtTime--
	}
	if m.dead() {
		m.deathTime++
	}
	if m.kind.id == "Creeper" && !m.dead() { // EntityCreeper.onUpdate runs on both sides
		m.fuse = min(max(m.fuse+n.creeperState, 0), creeperFuse)
	}
	dx, dz := m.pos[0]-m.prev[0], m.pos[2]-m.prev[2]
	f := float32(math.Sqrt(dx*dx+dz*dz)) * 4
	m.limbAmount += (min(f, 1) - m.limbAmount) * 0.4
	m.limbSwing += m.limbAmount
	n.turnBody(dx*dx+dz*dz > 2.500000277905201e-7)
	m.headYaw, m.pitch = wrapDeg(n.head-m.yaw), n.pitch
}

// turnBody is EntityBodyHelper.updateRenderAngles: a moving mob faces the way it goes (rotationYaw) and its
// head turns with it; a standing one turns its body after its head, slower and slower after ten ticks.
func (n *netMob) turnBody(moving bool) {
	m := n.m
	if moving {
		m.yaw = n.yaw
		n.head = angleWithin(m.yaw, n.head, 75)
		n.bodyHead, n.bodyTicks = n.head, 0
		return
	}
	limit := float32(75)
	if float32(math.Abs(float64(n.head-n.bodyHead))) > 15 {
		n.bodyTicks, n.bodyHead = 0, n.head
	} else {
		n.bodyTicks++
		if n.bodyTicks > 10 {
			limit = max(1-float32(n.bodyTicks-10)/10, 0) * 75
		}
	}
	m.yaw = angleWithin(n.head, m.yaw, limit)
}

// angleWithin is EntityBodyHelper.computeAngleWithBound: b, unless it is more than limit degrees from a.
func angleWithin(a, b, limit float32) float32 {
	f := wrapDeg(a - b)
	f = min(max(f, -limit), limit)
	return a - f
}

// netMobHurt is Entity Status 2 and 3 for a mob: it flashes red and its legs jerk, or it dies.
func (n *netMob) status(st byte) {
	switch st {
	case 2:
		n.m.hurtTime, n.m.limbAmount = 10, 1.5
	case 3:
		n.m.health = 0
	}
}
