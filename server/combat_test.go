package server

import (
	"math"
	"net"
	"testing"
	"time"

	"gocraft/proto"
)

// pvp starts a survival server whose clock the test drives, and logs two players in.
func pvp(t *testing.T, cfg Config) (*Server, *client, *client, *Player, *Player) {
	t.Helper()
	cfg.ViewDistance = 1
	s := New(cfg)
	s.manual = true
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go s.Serve(l)
	a := login(t, l.Addr().String(), "Alice")
	b := login(t, l.Addr().String(), "Bob")
	pa, pb := waitPlayer(t, s, "Alice"), waitPlayer(t, s, "Bob")
	waitFor(t, "both in the world", func() bool { return pa.inWorld() && pb.inWorld() })
	return s, a, b, pa, pb
}

func waitPlayer(t *testing.T, s *Server, name string) *Player {
	t.Helper()
	var p *Player
	waitFor(t, name+" joined", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, o := range s.players {
			if o.name == name {
				p = o
				return true
			}
		}
		return false
	})
	return p
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// place puts a player at a position as the server hears it, on the ground.
func (c *client) place(x, y, z float64, yaw float32, ground bool) {
	w := proto.NewPacket(0x06)
	w.Double(x)
	w.Double(y)
	w.Double(z)
	w.Float(yaw)
	w.Float(0)
	w.Bool(ground)
	c.send(w)
}

func (c *client) attack(eid int32) {
	w := proto.NewPacket(0x02)
	w.VarInt(eid)
	w.VarInt(1)
	c.send(w)
}

// setSlot is what a gocraft client says it carries, through the plugin channel.
func (c *client) setSlot(n int, id, count, dmg int16) {
	w := proto.NewPacket(0x17)
	w.String(invChannel)
	w.UShort(uint16(n))
	w.UShort(uint16(id))
	w.Byte(byte(count))
	w.UShort(uint16(dmg))
	c.send(w)
}

func (c *client) action(a int32) {
	w := proto.NewPacket(0x0B)
	w.VarInt(0)
	w.VarInt(a)
	w.VarInt(0)
	c.send(w)
}

func (c *client) block() { // right click with the held item in the air
	w := proto.NewPacket(0x08)
	w.Long(proto.PackPos(-1, 255, -1))
	w.Byte(255)
	w.UShort(0xFFFF)
	w.Byte(0)
	w.Byte(0)
	w.Byte(0)
	c.send(w)
}

// healthOf waits for the next health packet and returns it.
func (c *client) healthOf() float32 {
	r := c.until(0x06)
	return r.Float()
}

func (c *client) velocity() (eid int32, v [3]float64) {
	r := c.until(0x12)
	eid = r.VarInt()
	for i := range v {
		v[i] = float64(r.Short()) / 8000
	}
	return
}

func item(p *Player, n int) int16 { p.mu.Lock(); defer p.mu.Unlock(); return p.inv[n].id }
func isBlocking(p *Player) bool   { p.mu.Lock(); defer p.mu.Unlock(); return p.blocking }
func armourOf(p *Player) int      { p.mu.Lock(); defer p.mu.Unlock(); return p.armour() }

func close2(a, b float64) bool { return math.Abs(a-b) < 1e-3 }

// stand puts both players on the ground, Bob 1.5 blocks south of Alice, and waits for the server to know.
func stand(t *testing.T, a, b *client, pa, pb *Player) {
	t.Helper()
	a.place(0.5, 4, 0.5, 0, true)
	b.place(0.5, 4, 2.0, 180, true)
	waitFor(t, "positions", func() bool {
		pa.mu.Lock()
		pb.mu.Lock()
		defer pa.mu.Unlock()
		defer pb.mu.Unlock()
		return pa.z == 0.5 && pb.z == 2.0 && pb.onGround
	})
}

func hp(p *Player) float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.health
}

func TestKnockbackFromTheAttacker(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.attack(pb.eid)
	eid, v := b.velocity()
	if eid != pb.eid {
		t.Fatalf("velocity for entity %d, want %d", eid, pb.eid)
	}
	// Pushed away from Alice along +z by 0.4, and up by 0.4 because Bob stands on the ground.
	if !close2(v[0], 0) || !close2(v[1], 0.4) || !close2(v[2], 0.4) {
		t.Fatalf("knockback %v, want 0, 0.4, 0.4", v)
	}
	if h := b.healthOf(); h != 19 {
		t.Fatalf("health %v after a hand's 1 damage", h)
	}
}

func TestNoUpwardKnockbackInTheAir(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	b.place(0.5, 6, 2.0, 180, false)
	waitFor(t, "Bob airborne", func() bool { pb.mu.Lock(); defer pb.mu.Unlock(); return !pb.onGround })
	a.attack(pb.eid)
	if _, v := b.velocity(); !close2(v[1], 0) || !close2(v[2], 0.4) {
		t.Fatalf("knockback in the air %v, want no lift", v)
	}
}

func TestSprintKnockbackAddsToIt(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.place(0.5, 4, 0.5, 0, true) // facing south (+z)
	a.action(3)                   // start sprinting
	waitFor(t, "sprint", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return pa.sprinting })
	a.attack(pb.eid)
	_, v := b.velocity()
	// 0.4 away from Alice plus 0.5 along her facing (+z); lift 0.4 from the ground capped, plus 0.1.
	if !close2(v[2], 0.9) || !close2(v[1], 0.5) {
		t.Fatalf("sprint knockback %v, want z 0.9 and y 0.5", v)
	}
	waitFor(t, "the sprint to end", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return !pa.sprinting }) // a sprint hit costs the sprint
}

func TestInvulnerabilityAndHalfTimeRule(t *testing.T) {
	s, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.attack(pb.eid) // 1 damage
	b.until(0x12)
	if h := hp(pb); h != 19 {
		t.Fatalf("health %v", h)
	}
	a.attack(pb.eid) // the same blow inside the 20 ticks does nothing
	time.Sleep(50 * time.Millisecond)
	if h := hp(pb); h != 19 {
		t.Fatalf("a second equal blow took health: %v", h)
	}
	a.setSlot(36, 267, 1, 0) // an iron sword: 1+4+2 = 7
	waitFor(t, "sword", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return pa.inv[36].id == 267 })
	a.attack(pb.eid) // a harder blow counts by the difference: 7-1 = 6
	waitFor(t, "the difference", func() bool { return hp(pb) == 13 })
	for i := 0; i < 20; i++ {
		s.tick()
	}
	a.attack(pb.eid) // the invulnerability is over: the whole 7
	waitFor(t, "a whole blow", func() bool { return hp(pb) == 6 })
}

func TestBlockingHalvesTheBlow(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.setSlot(36, 267, 1, 0)
	b.setSlot(36, 268, 1, 0) // Bob holds a wooden sword
	b.block()
	waitFor(t, "blocking", func() bool { return item(pa, 36) == 267 && isBlocking(pb) })
	a.attack(pb.eid) // 7 -> (1+7)/2 = 4
	waitFor(t, "a blocked blow", func() bool { return hp(pb) == 16 })
	// Releasing ends it.
	rel := proto.NewPacket(0x07)
	rel.Byte(5)
	rel.Long(0)
	rel.Byte(0)
	b.send(rel)
	waitFor(t, "released", func() bool { pb.mu.Lock(); defer pb.mu.Unlock(); return !pb.blocking })
}

func TestArmourTakesFourPercentAPoint(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.setSlot(36, 267, 1, 0)
	for i, id := range []int16{310, 311, 312, 313} { // a full diamond set: 20 points
		b.setSlot(5+i, id, 1, 0)
	}
	waitFor(t, "armour", func() bool { return item(pa, 36) == 267 && armourOf(pb) == 20 })
	a.attack(pb.eid) // 7 * (25-20)/25 = 1.4
	waitFor(t, "armoured blow", func() bool { return math.Abs(float64(hp(pb))-18.6) < 1e-4 })
}

func TestCriticalHitWhenFalling(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.setSlot(36, 267, 1, 0)
	a.place(0.5, 4.6, 0.5, 0, false) // up in the air...
	a.place(0.5, 4.4, 0.5, 0, false) // ...and coming down: fall distance > 0
	waitFor(t, "falling", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return pa.fall > 0 && pa.inv[36].id == 267 })
	a.attack(pb.eid) // 7 * 1.5 = 10.5
	waitFor(t, "a critical", func() bool { return math.Abs(float64(hp(pb))-9.5) < 1e-4 })
}

func TestTooFarIsNotAHit(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	b.place(0.5, 4, 20, 180, true)
	waitFor(t, "far away", func() bool { pb.mu.Lock(); defer pb.mu.Unlock(); return pb.z == 20 })
	a.attack(pb.eid)
	time.Sleep(60 * time.Millisecond)
	if h := hp(pb); h != 20 {
		t.Fatalf("a blow from 19 blocks landed: %v", h)
	}
}

func TestDeathAndRespawn(t *testing.T) {
	s, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.setSlot(36, 276, 1, 0) // diamond sword: 8
	waitFor(t, "sword", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return pa.inv[36].id == 276 })
	pb.mu.Lock()
	pb.health = 5
	pb.mu.Unlock()
	a.attack(pb.eid)
	// Everyone hears of the death, and the chat says who did it.
	r := a.until(0x1A)
	if r.Int() != pb.eid {
		t.Fatal("status for another entity")
	}
	if st := r.Byte(); st != statusHurt {
		t.Fatalf("first status %d, want hurt", st)
	}
	r = a.until(0x1A)
	r.Int()
	if st := r.Byte(); st != statusDeath {
		t.Fatalf("second status %d, want death", st)
	}
	for {
		c := a.until(0x02)
		if msg := c.String(400); msg == `{"text":"Bob was slain by Alice"}` {
			break
		}
	}
	if h := hp(pb); h != 0 {
		t.Fatalf("health %v, want 0", h)
	}
	// Dead players cannot be hit again. The blow goes out asynchronously, so a position sent behind it
	// (one connection is read in order) says the server has dealt with it before the ticks run.
	a.attack(pb.eid)
	a.place(0.5, 4, 0.75, 0, true)
	waitFor(t, "the blow on the dead", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return pa.z == 0.75 })
	// 30 ticks later: full health at the spawn, and the others see them again.
	for i := 0; i < respawnTicks; i++ {
		s.tick()
	}
	waitFor(t, "respawn", func() bool { return hp(pb) == maxHealth })
	for h := b.healthOf(); h != 0; h = b.healthOf() { // past the join's 20, to the death's 0...
	}
	if h := b.healthOf(); h != 20 { // ...then the respawn's
		t.Fatalf("respawn health %v", h)
	}
	a.until(0x13) // destroyed...
	a.until(0x0C) // ...and spawned again
	pb.mu.Lock()
	x, y, z := pb.x, pb.y, pb.z
	pb.mu.Unlock()
	if x != 0.5 || y != spawnY || z != 0.5 {
		t.Fatalf("respawned at %v %v %v", x, y, z)
	}
}

func TestFallDamageAndTheVoid(t *testing.T) {
	s, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	b.place(0.5, 14, 2, 0, false)
	b.place(0.5, 9, 2, 0, false)
	b.place(0.5, 4.2, 2, 0, false) // as in the game, the step that lands is not counted: 9.8 blocks fallen
	b.place(0.5, 4, 2, 0, true)    // ceil(9.8-3) = 7 damage, ignoring armour
	waitFor(t, "fall damage", func() bool { return hp(pb) == 13 })
	// Under the world the player dies at the next tick.
	b.place(0.5, -30, 2, 0, false)
	waitFor(t, "below the world", func() bool { pb.mu.Lock(); defer pb.mu.Unlock(); return pb.y < voidY })
	s.tick()
	waitFor(t, "void death", func() bool { return hp(pb) == 0 })
}

func TestRegeneration(t *testing.T) {
	s, a, b, pa, pb := pvp(t, Config{Survival: true})
	stand(t, a, b, pa, pb)
	a.attack(pb.eid)
	waitFor(t, "hurt", func() bool { return hp(pb) == 19 })
	for i := 0; i < regenInterval; i++ {
		s.tick()
	}
	waitFor(t, "healed", func() bool { return hp(pb) == 20 })
}

func TestCreativeServerHurtsNobody(t *testing.T) {
	_, a, b, pa, pb := pvp(t, Config{})
	stand(t, a, b, pa, pb)
	a.attack(pb.eid)
	time.Sleep(60 * time.Millisecond)
	if h := hp(pb); h != 20 {
		t.Fatalf("a creative server took health: %v", h)
	}
}

func TestSurvivalBreaksOnFinish(t *testing.T) {
	s, a, _, pa, _ := pvp(t, Config{Survival: true})
	a.place(0.5, 4, 0.5, 0, true)
	waitFor(t, "position", func() bool { pa.mu.Lock(); defer pa.mu.Unlock(); return pa.z == 0.5 })
	dig := func(status byte) {
		w := proto.NewPacket(0x07)
		w.Byte(status)
		w.Long(proto.PackPos(1, 3, 0)) // the grass beside Alice
		w.Byte(1)
		a.send(w)
	}
	dig(0) // survival: starting to dig breaks nothing
	time.Sleep(50 * time.Millisecond)
	if s.world.Block(1, 3, 0)>>4 != 2 {
		t.Fatal("survival broke a block when digging only started")
	}
	dig(2)
	waitFor(t, "the block to go", func() bool { return s.world.Block(1, 3, 0) == 0 })
}

func TestSurvivalIgnoresTheCreativePacket(t *testing.T) {
	_, a, _, pa, _ := pvp(t, Config{Survival: true})
	w := proto.NewPacket(0x10) // what a creative client sends: a survival server must not obey it
	w.UShort(36)
	w.UShort(276)
	w.Byte(1)
	w.UShort(0)
	w.Byte(0)
	a.send(w)
	a.setSlot(37, 267, 1, 0) // a later packet that is obeyed, so the first has surely been read
	waitFor(t, "the channel packet", func() bool { return item(pa, 37) == 267 })
	if got := item(pa, 36); got != -1 {
		t.Fatalf("a survival player gave themselves item %d with the creative packet", got)
	}
}

func TestCreativeIgnoresTheChannelAndTakesTheCreativePacket(t *testing.T) {
	_, a, _, pa, _ := pvp(t, Config{})
	a.setSlot(36, 267, 1, 0)
	w := proto.NewPacket(0x10)
	w.UShort(37)
	w.UShort(276)
	w.Byte(1)
	w.UShort(0)
	w.Byte(0)
	a.send(w)
	waitFor(t, "the creative packet", func() bool { return item(pa, 37) == 276 })
	if got := item(pa, 36); got != -1 {
		t.Fatalf("the channel set a slot on a creative server: %d", got)
	}
}

func TestJoinSaysTheServerIsGocraft(t *testing.T) {
	addr := start(t)
	c := dial(t, addr)
	c.handshake(Protocol, 2)
	p := proto.NewPacket(0)
	p.String("Dora")
	c.send(p)
	r := c.until(0x3F)
	if ch, brand := r.String(20), r.String(64); ch != "MC|Brand" || brand != "gocraft" {
		t.Fatalf("plugin message %q %q", ch, brand)
	}
}
