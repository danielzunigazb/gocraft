package client

import "math"

// When the game makes a sound, after 1.8: which block sounds like what, footsteps, the sounds of mobs,
// of the player, bows, explosions and menus. The volumes and pitches are those the 1.8.9 code passes to
// playSound; the files and categories come from sounds.json.

type blockSound uint8

const (
	bsStone blockSound = iota
	bsGrass
	bsGravel
	bsSand
	bsWood
	bsCloth
	bsSnow
	bsGlass
	bsLadder
	bsNone // air, water, lava: no sound of their own
)

var blockSounds [256]blockSound

func init() {
	for i := range blockSounds {
		blockSounds[i] = bsStone // what is not listed is hard
	}
	set := func(k blockSound, ids ...int) {
		for _, id := range ids {
			blockSounds[id] = k
		}
	}
	set(bsNone, 0, 8, 9, 10, 11, 51)
	set(bsGrass, 2, 6, 18, 31, 32, 37, 38, 39, 40, 59, 83, 104, 105, 106, 110, 111, 115, 141, 142, 161, 170, 175)
	set(bsGravel, 3, 13, 60, 82)
	set(bsSand, 12, 88)
	set(bsWood, 5, 17, 25, 26, 47, 50, 53, 54, 58, 63, 64, 68, 72, 75, 76, 84, 85, 86, 91, 96, 103, 107, 125, 126, 127, 134, 135, 136, 143, 146, 162, 163, 164, 178)
	for id := 183; id <= 197; id++ { // fences, fence gates and the wooden doors of 1.8
		set(bsWood, id)
	}
	set(bsCloth, 35, 81, 92, 171)
	set(bsSnow, 78, 80)
	set(bsGlass, 20, 79, 89, 95, 102, 123, 124, 138, 160, 169, 174)
	set(bsLadder, 65)
}

// dig is the sound of a block being broken or placed; step the one of a foot on it.
func (k blockSound) dig() string {
	switch k {
	case bsGrass:
		return "dig.grass"
	case bsGravel:
		return "dig.gravel"
	case bsSand:
		return "dig.sand"
	case bsWood, bsLadder:
		return "dig.wood"
	case bsCloth:
		return "dig.cloth"
	case bsSnow:
		return "dig.snow"
	case bsGlass:
		return "dig.glass"
	case bsStone:
		return "dig.stone"
	}
	return ""
}

func (k blockSound) step() string {
	switch k {
	case bsGrass:
		return "step.grass"
	case bsGravel:
		return "step.gravel"
	case bsSand:
		return "step.sand"
	case bsWood:
		return "step.wood"
	case bsCloth:
		return "step.cloth"
	case bsSnow:
		return "step.snow"
	case bsLadder:
		return "step.ladder"
	case bsGlass, bsStone: // glass is heard stepping as stone
		return "step.stone"
	}
	return ""
}

func soundOfBlock(st uint16) blockSound { return blockSounds[(st>>4)&255] }

func rpitch() float32 { return (rnd()-rnd())*0.2 + 1 } // the +-20% the living sounds are played at

func (g *game) sound(event string, at vec3, vol, pitch float32) { g.snd.play(event, at, vol, pitch) }

func centre(x, y, z int32) vec3 { return vec3{float64(x) + 0.5, float64(y) + 0.5, float64(z) + 0.5} }

// blockSounds plays a block that was broken or placed: ItemBlock.onItemUse and RenderGlobal.playAuxSFX 2001
// both play the block's dig sound at volume (1+v)/2 and pitch 0.8.
func (g *game) blockSounds(x, y, z int32, old, state uint16) {
	if g.snd == nil {
		return
	}
	switch {
	case old != 0 && state == 0:
		if k := soundOfBlock(old); k != bsNone {
			g.sound(k.dig(), centre(x, y, z), 1, 0.8)
		}
	case state != 0 && (old == 0 || isFluid(old>>4)):
		if k := soundOfBlock(state); k != bsNone {
			g.sound(k.dig(), centre(x, y, z), 1, 0.8)
		}
	}
}

// blockHitSound is the tap of mining: every fourth tick, at (1+v)/8 and pitch 0.5.
func (g *game) blockHitSound(pos [3]int32, st uint16, tick int) {
	if g.snd == nil || tick%4 != 0 {
		return
	}
	if k := soundOfBlock(st); k != bsNone {
		g.sound(k.step(), centre(pos[0], pos[1], pos[2]), 0.25, 0.5)
	}
}

// soundState is what the step sounds remember between ticks.
type soundState struct {
	walked, nextStep float32 // Entity.distanceWalkedOnStepModified and nextStepDistance
	mobs             map[*mob]*mobSoundState
	cleanup          int
}

// tickSound runs once a tick: the listener moves with the player, and the feet make their sound.
func (g *game) tickSound() {
	if g.snd == nil {
		return
	}
	g.snd.listen(g.pos, g.yaw)
	st := &g.sndState
	if g.flying { // EntityPlayer.canTriggerWalking: not while flying
		return
	}
	st.walked += float32(math.Hypot(float64(g.feet[0]-g.prevFeet[0]), float64(g.feet[2]-g.prevFeet[2]))) * 0.6
	if st.walked <= st.nextStep {
		return
	}
	below := g.world.Block(int32(math.Floor(float64(g.feet[0]))), int32(math.Floor(float64(g.feet[1]-0.2))), int32(math.Floor(float64(g.feet[2]))))
	k := soundOfBlock(below)
	if k == bsNone { // in the air or in water: the count goes on until there is ground
		return
	}
	st.nextStep = float32(int(st.walked)) + 1
	if above := g.world.Block(int32(math.Floor(float64(g.feet[0]))), int32(math.Floor(float64(g.feet[1]-0.2)))+1, int32(math.Floor(float64(g.feet[2])))); above>>4 == 78 {
		k = bsSnow // a layer of snow on top of what is stepped on
	}
	g.sound(k.step(), g.feet, 0.15, 1)
}

// ---- mobs ----

type mobSounds struct{ say, hurt, death, step string }

// mobSoundTable are the sounds of 1.8.9's mobs by kind (a creeper has no voice of its own: its hurt sound is its say).
var mobSoundTable = map[string]mobSounds{
	"Zombie":   {"mob.zombie.say", "mob.zombie.hurt", "mob.zombie.death", "mob.zombie.step"},
	"Skeleton": {"mob.skeleton.say", "mob.skeleton.hurt", "mob.skeleton.death", "mob.skeleton.step"},
	"Creeper":  {"", "mob.creeper.say", "mob.creeper.death", ""},
	"Pig":      {"mob.pig.say", "mob.pig.say", "mob.pig.death", "mob.pig.step"},
	"Cow":      {"mob.cow.say", "mob.cow.hurt", "mob.cow.hurt", "mob.cow.step"},
	"Sheep":    {"mob.sheep.say", "mob.sheep.say", "mob.sheep.say", "mob.sheep.step"},
	"Chicken":  {"mob.chicken.say", "mob.chicken.hurt", "mob.chicken.hurt", "mob.chicken.step"},
	"PvPBot":   {"", "game.player.hurt", "game.player.die", ""},
}

type mobSoundState struct {
	health         float32
	said           int // EntityLiving.livingSoundTime
	walked, next   float32
	fuse           int
	dead, seenOnce bool
}

// mobSounds is called for every mob each tick: its voice now and then (EntityLiving.onEntityUpdate), the
// sound of a blow it took or of its death, its footsteps and the hiss of a creeper that lit its fuse.
func (g *game) mobSounds(m *mob) {
	if g.snd == nil {
		return
	}
	table, ok := mobSoundTable[m.kind.id]
	if !ok {
		return
	}
	st := &g.sndState
	if st.mobs == nil {
		st.mobs = map[*mob]*mobSoundState{}
	}
	ms := st.mobs[m]
	if ms == nil {
		ms = &mobSoundState{health: m.health}
		st.mobs[m] = ms
	}
	if st.cleanup++; st.cleanup >= 600 { // forget the mobs that are gone
		st.cleanup = 0
		live := map[*mob]bool{}
		for _, o := range g.mobs {
			live[o] = true
		}
		for k := range st.mobs {
			if !live[k] {
				delete(st.mobs, k)
			}
		}
	}
	at := vec3{m.pos[0], m.pos[1] + float64(m.kind.h)/2, m.pos[2]}
	if d := at.sub(g.pos); d.dot(d) > 24*24 {
		ms.health = m.health // too far to hear: still keep track of it
		return
	}
	if m.health < ms.health && !ms.dead { // a blow: its hurt sound, or its death sound if it was the last
		if m.dead() {
			ms.dead = true
			if table.death != "" {
				g.sound(table.death, at, 1, rpitch())
			}
		} else if table.hurt != "" {
			g.sound(table.hurt, at, 1, rpitch())
		}
	}
	ms.health = m.health
	if m.dead() {
		return
	}
	if rng.Intn(1000) < ms.said { // it speaks now and then: about once every 4 seconds on average
		ms.said = -80
		if table.say != "" {
			g.sound(table.say, at, 1, rpitch())
		}
	}
	ms.said++
	if m.kind.id == "Creeper" && m.fuse > 0 && ms.fuse == 0 {
		g.sound("creeper.primed", at, 1, 0.5)
	}
	ms.fuse = m.fuse
	if table.step != "" && m.onGround {
		ms.walked += float32(math.Hypot(float64(m.pos[0]-m.prev[0]), float64(m.pos[2]-m.prev[2]))) * 0.6
		if ms.walked > ms.next {
			ms.next = float32(int(ms.walked)) + 1
			g.sound(table.step, m.pos, 0.15, 1)
		}
	}
}

// ---- the player, bows, explosions, menus ----

// playerHurtSound is the sound of the player taking a blow, or dying.
func (g *game) playerHurtSound(dying bool) {
	if g.snd == nil {
		return
	}
	if dying {
		g.sound("game.player.die", g.pos, 1, rpitch())
		return
	}
	g.sound("game.player.hurt", g.pos, 1, rpitch())
}

// eatSound is EntityPlayer.updateItemUse's bite every fourth tick of eating, and the burp at the end.
func (g *game) eatSound(finished bool) {
	if g.snd == nil {
		return
	}
	if finished {
		g.sound("random.burp", g.pos, 0.5, rnd()*0.1+0.9)
		return
	}
	g.sound("random.eat", g.pos, 0.5+0.5*float32(rng.Intn(2)), rpitch())
}

// bowSound is the twang of a shot: ItemBow plays it at volume 1 and pitch 1 / (r * 0.4 + 1.2) + draw * 0.5.
func (g *game) bowSound(power float32) {
	g.sound("random.bow", g.pos, 1, 1/(rnd()*0.4+1.2)+power*0.5)
}

// skeletonBowSound is a skeleton's shot (EntitySkeleton: pitch 1 / (r * 0.4 + 0.8)).
func (g *game) skeletonBowSound(at vec3) { g.sound("random.bow", at, 1, 1/(rnd()*0.4+0.8)) }

// arrowHitSound is an arrow sticking in something (EntityArrow: pitch 1.2 / (r * 0.2 + 0.9)).
func (g *game) arrowHitSound(at vec3) { g.sound("random.bowhit", at, 1, 1.2/(rnd()*0.2+0.9)) }

// explosionSound is World.newExplosion's roar: volume 4, pitch (1 + (r - r) * 0.2) * 0.7.
func (g *game) explosionSound(at vec3) { g.sound("random.explode", at, 4, (1+(rnd()-rnd())*0.2)*0.7) }

// pickupSound is EntityPlayer.onItemPickup's pop.
func (g *game) pickupSound() { g.sound("random.pop", g.pos, 0.2, ((rnd()-rnd())*0.7+1)*2) }

// uiSound is a menu button being pressed.
func (g *game) uiSound() { g.snd.playUI("gui.button.press", 1, 1) }
