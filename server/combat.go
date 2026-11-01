package server

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"gocraft/proto"
	"gocraft/world"
)

const spawnY = float64(world.SpawnY)

var spawnChunk = world.ChunkPos{}

func newTicker() *time.Ticker { return time.NewTicker(50 * time.Millisecond) }

// Fighting between players, after 1.8's EntityLivingBase.attackEntityFrom and
// EntityPlayer.attackTargetEntityWithCurrentItem: the weapon's damage, critical hits, the 20 ticks
// of invulnerability and its half-time rule, blocking with a sword, armour, knockback, sprint
// knockback, falling, the void and respawning. The server owns health; clients only hear of it.

const (
	maxHealth      = 20
	hurtTime       = 20 // maxHurtResistantTime
	respawnTicks   = 30
	attackReach    = 6.0 // eye to hit box, generous: the client aims within 3
	eyeHeight      = 1.62
	playerW        = 0.6
	playerH        = 1.8
	voidY          = -20
	regenInterval  = 80
	knockback      = 0.4
	sprintKnock    = 0.5
	statusHurt     = 2
	statusDeath    = 3
	blockFrontOnly = false // 1.8 halves a blocked blow from any side; 1.9's shield looks at the attacker
)

// weaponBonus is what an item adds to the player's own 1 damage: swords 4, axes 3, pickaxes 2 and
// shovels 1, plus the material's (wood 0, stone 1, iron 2, diamond 3, gold 0).
func weaponBonus(id int16) float32 {
	type tool struct {
		base float32
		ids  [5]int16 // wood, stone, iron, diamond, gold
	}
	tools := []tool{
		{4, [5]int16{268, 272, 267, 276, 283}}, // swords
		{3, [5]int16{271, 275, 258, 279, 286}}, // axes
		{2, [5]int16{270, 274, 257, 278, 285}}, // pickaxes
		{1, [5]int16{269, 273, 256, 277, 284}}, // shovels
	}
	bonus := [5]float32{0, 1, 2, 3, 0}
	for _, t := range tools {
		for tier, tid := range t.ids {
			if tid == id {
				return t.base + bonus[tier]
			}
		}
	}
	return 0
}

func isSword(id int16) bool {
	switch id {
	case 267, 268, 272, 276, 283:
		return true
	}
	return false
}

// armourPoints is what a piece of armour is worth: helmet, chestplate, leggings, boots by material.
func armourPoints(id int16) int {
	if id < 298 || id > 317 {
		return 0
	}
	mat := int(id-298) / 4 // leather, chainmail, iron, diamond, gold
	return [5][4]int{{1, 3, 2, 1}, {2, 5, 4, 1}, {2, 6, 5, 2}, {3, 8, 6, 3}, {2, 5, 3, 1}}[mat][int(id-298)%4]
}

// armour is the defence points of the four worn pieces (window slots 5 to 8). The caller holds p.mu.
func (p *Player) armour() int {
	n := 0
	for i := 5; i <= 8; i++ {
		if it := p.inv[i]; it.id >= 0 {
			n += armourPoints(it.id)
		}
	}
	return n
}

func (p *Player) healthPacket() []byte {
	p.mu.Lock()
	h := p.health
	p.mu.Unlock()
	w := proto.NewPacket(0x06)
	w.Float(h)
	w.VarInt(20)
	w.Float(5)
	return w.Frame()
}

func statusPacket(eid int32, status byte) []byte {
	w := proto.NewPacket(0x1A)
	w.Int(eid)
	w.Byte(status)
	return w.Frame()
}

func velocityPacket(eid int32, v [3]float64) []byte {
	w := proto.NewPacket(0x12)
	w.VarInt(eid)
	for _, c := range v {
		c = math.Max(-3.9, math.Min(3.9, c))
		w.UShort(uint16(int16(c * 8000)))
	}
	return w.Frame()
}

// attack is a player's blow at another, from the 0x02 Use Entity packet.
func (s *Server) attack(a, v *Player) {
	if !s.cfg.Survival || a == v {
		return
	}
	a.mu.Lock()
	ax, ay, az := a.x, a.y+eyeHeight, a.z
	held := a.inv[36+a.held]
	crit := a.fall > 0 && !a.onGround
	sprint := a.sprinting
	adead := a.dead > 0
	yaw := a.yaw
	a.mu.Unlock()
	v.mu.Lock()
	vx, vy, vz := v.x, v.y, v.z
	v.mu.Unlock()
	if adead || reachSq(ax, ay, az, vx, vy, vz) > attackReach*attackReach {
		return
	}
	dmg := 1 + weaponBonus(held.id)
	if crit {
		dmg *= 1.5
	}
	var extra [3]float64
	if sprint { // EntityPlayer: sprinting adds a knockback level: a push along the attacker's facing
		r := float64(yaw) * math.Pi / 180
		extra = [3]float64{-math.Sin(r) * sprintKnock, 0.1, math.Cos(r) * sprintKnock}
	}
	if !s.hurt(v, dmg, a, false, a.name, extra) {
		return
	}
	if sprint { // ...and costs the attacker the sprint
		a.mu.Lock()
		a.sprinting = false
		a.mu.Unlock()
	}
}

// reachSq is the squared distance from a point to the hit box of a player standing at x, y, z.
func reachSq(ex, ey, ez, x, y, z float64) float64 {
	cl := func(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
	dx := ex - cl(ex, x-playerW/2, x+playerW/2)
	dy := ey - cl(ey, y, y+playerH)
	dz := ez - cl(ez, z-playerW/2, z+playerW/2)
	return dx*dx + dy*dy + dz*dz
}

// hurt is attackEntityFrom: it says whether the blow counted. from is nil for falling and the
// void; unblockable skips blocking and armour; cause names the killer in the death message; extra
// is velocity added when the blow counts (the sprint knockback), sent with the usual one.
func (s *Server) hurt(v *Player, amount float32, from *Player, unblockable bool, cause string, extra [3]float64) bool {
	if !s.cfg.Survival {
		return false
	}
	v.mu.Lock()
	if v.dead > 0 {
		v.mu.Unlock()
		return false
	}
	applied, fresh := amount, true
	if v.hurtResist > hurtTime/2 { // inside the invulnerability: only a harder blow counts, by the difference
		if amount <= v.lastDamage {
			v.mu.Unlock()
			return false
		}
		applied, fresh = amount-v.lastDamage, false
		v.lastDamage = amount
	} else {
		v.lastDamage, v.hurtResist = amount, hurtTime
	}
	blocked := v.blocking && !unblockable && applied > 0
	if blocked && blockFrontOnly && from != nil {
		from.mu.Lock()
		dx, dz := v.x-from.x, v.z-from.z
		from.mu.Unlock()
		r := float64(v.yaw) * math.Pi / 180
		blocked = dx*-math.Sin(r)+dz*math.Cos(r) < 0
	}
	if blocked {
		applied = (1 + applied) * 0.5
	}
	if !unblockable {
		applied = applied * float32(25-v.armour()) / 25
	}
	v.health -= applied
	dead := v.health <= 0
	if dead {
		v.health = 0
		v.dead = respawnTicks
		v.blocking, v.sprinting = false, false
	}
	eid, vx, vz, onGround := v.eid, v.x, v.z, v.onGround
	var kb [3]float64
	push := extra != [3]float64{}
	if fresh && from != nil {
		push = true
		from.mu.Lock()
		ax, az := from.x, from.z
		from.mu.Unlock()
		dx, dz := ax-vx, az-vz
		for dx*dx+dz*dz < 1e-4 {
			dx, dz = (rand.Float64()-rand.Float64())*0.01, (rand.Float64()-rand.Float64())*0.01
		}
		d := math.Sqrt(dx*dx + dz*dz)
		// The server's own motion for a player is 0, so halving it leaves 0 before the push.
		kb = [3]float64{-dx / d * knockback, 0, -dz / d * knockback}
		if onGround {
			kb[1] = math.Min(knockback, kb[1]+knockback)
		}
	}
	for i := range kb {
		kb[i] += extra[i]
	}
	v.mu.Unlock()

	if fresh {
		s.Broadcast(statusPacket(eid, statusHurt))
	}
	if push { // the velocity of the knockback and the sprint's, in one packet like the game's end-of-tick one
		v.send(velocityPacket(eid, kb))
	}
	v.send(v.healthPacket())
	if dead {
		s.Broadcast(statusPacket(eid, statusDeath))
		s.Broadcast(chatPacket(deathMessage(v.name, cause, from != nil)))
	}
	return true
}

func deathMessage(name, cause string, byPlayer bool) string {
	switch {
	case byPlayer:
		return fmt.Sprintf("%s was slain by %s", name, cause)
	case cause == "":
		return name + " died"
	}
	return name + " " + cause
}

// trackFall is Entity.updateFallState from the positions the client reports: the distance fallen
// is counted while airborne and paid on landing. The caller holds p.mu. It returns the damage.
func (p *Player) trackFall(oldY float64, onGround, inWater bool) float32 {
	var dmg float32
	switch {
	case inWater:
		p.fall = 0
	case onGround:
		if p.fall > 3 {
			dmg = float32(math.Ceil(float64(p.fall) - 3))
		}
		p.fall = 0
	case p.y < oldY:
		p.fall += float32(oldY - p.y)
	}
	p.onGround = onGround
	return dmg
}

// tick is one 20 Hz step of the world's rules: invulnerability counts down, the dead respawn,
// the living heal and the ones under the world die.
func (s *Server) tick() {
	defer s.autosave()
	for _, p := range s.inWorld() {
		p.mu.Lock()
		if p.hurtResist > 0 {
			p.hurtResist--
		}
		respawn, heal, void := false, false, false
		switch {
		case p.dead > 0:
			if p.dead--; p.dead == 0 {
				respawn = true
			}
		case s.cfg.Survival && p.y < voidY:
			void = true
		case s.cfg.Survival && p.health < maxHealth:
			if p.regen++; p.regen >= regenInterval {
				p.regen = 0
				p.health = min(p.health+1, maxHealth)
				heal = true
			}
		}
		p.mu.Unlock()
		switch {
		case respawn:
			s.respawn(p)
		case heal:
			p.send(p.healthPacket())
		case void:
			p.mu.Lock()
			p.hurtResist = 0 // the void kills through any invulnerability
			p.mu.Unlock()
			s.hurt(p, maxHealth*4, nil, true, "fell out of the world", [3]float64{})
		}
	}
}

// respawn puts a dead player back at the spawn with full health, and shows them again to the others.
func (s *Server) respawn(p *Player) {
	sx, sy, sz, scp := s.spawnPoint()
	p.mu.Lock()
	p.health, p.hurtResist, p.lastDamage, p.fall, p.regen = maxHealth, 0, 0, 0, 0
	p.x, p.y, p.z = sx, sy, sz
	p.yaw, p.pitch = 0, 0
	p.onGround = false
	p.vel = [3]float64{}
	p.chunk = scp
	p.mu.Unlock()
	p.send(p.healthPacket())
	p.teleport(sx, sy, sz, 0, 0)
	p.streamChunks(s)
	s.sendOthers(p, p.destroyPacket(), p.spawnPacket(), p.headPacket())
}

// inWorld lists the players who have finished joining.
func (s *Server) inWorld() []*Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Player, 0, len(s.players))
	for _, p := range s.players {
		if p.inWorld() {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) tickLoop(stop <-chan struct{}) {
	t := newTicker()
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.tick()
		}
	}
}
