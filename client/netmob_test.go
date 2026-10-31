package client

import (
	"bufio"
	"bytes"
	"testing"

	"gocraft/proto"
	"gocraft/world"
)

// spawnMobPacket is Spawn Mob with the angles in 256ths of a turn and metadata entries written by meta.
func spawnMobPacket(eid int32, kind byte, x, y, z float64, yaw, head byte, meta func(w *proto.Writer)) *proto.Writer {
	w := newPacket(0x0F)
	w.VarInt(eid)
	w.Byte(kind)
	w.Int(int32(x * 32))
	w.Int(int32(y * 32))
	w.Int(int32(z * 32))
	w.Byte(yaw)
	w.Byte(0) // pitch
	w.Byte(head)
	w.UShort(0)
	w.UShort(0)
	w.UShort(0)
	if meta != nil {
		meta(w)
	}
	w.Byte(0x7F)
	return w
}

func metaByte(idx, v byte) func(w *proto.Writer) {
	return func(w *proto.Writer) {
		w.Byte(0<<5 | idx)
		w.Byte(v)
	}
}

func mobGame(t *testing.T) (*game, *netClient, func(*proto.Writer)) {
	g, c := windowGame(t)
	g.net.eid = 1
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true // the player waits for its column, as in the game
	return g, c, func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
}

// A mob gocraft has a model for gets one, with what its metadata says; one it has none for keeps only its name.
func TestServerMobKindsAndMetadata(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(20, 91, 3.5, 4, 3.5, 0, 0, func(w *proto.Writer) {
		metaByte(12, 0xFF)(w)  // age -1: a lamb
		metaByte(16, 14|16)(w) // red, and shorn
	}))
	e := g.net.entity(20)
	if e == nil || e.mob == nil {
		t.Fatal("a sheep has no model")
	}
	if m := e.mob.m; m.colour != 14 || !m.sheared || !e.mob.baby || m.drawScale != 0.5 {
		t.Fatalf("sheep: colour %d sheared %v baby %v scale %v", m.colour, m.sheared, e.mob.baby, m.drawScale)
	}
	if h := e.height(); !near(h, 0.65, 1e-6) {
		t.Fatalf("a lamb is %v high, want 0.65", h)
	}
	apply(spawnMobPacket(21, 51, 5.5, 4, 3.5, 0, 0, metaByte(13, 1))) // a wither skeleton
	if m := g.net.entity(21).mob; !m.witherSkel || m.m.tex != "skeleton/wither_skeleton" || m.m.drawScale != 1.2 || !near(g.net.entity(21).height(), 2.535, 1e-6) {
		t.Fatalf("wither skeleton: %v %q %v", m.witherSkel, m.m.tex, m.m.drawScale)
	}
	apply(spawnMobPacket(22, mobPigZombie, 7.5, 4, 3.5, 0, 0, nil))
	if m := g.net.entity(22).mob.m; m.kind.id != "Zombie" || m.tex != "zombie_pigman" {
		t.Fatalf("pigman drawn as %s with %q", m.kind.id, m.tex)
	}
	apply(spawnMobPacket(23, 58, 9.5, 4, 3.5, 0, 0, nil)) // an enderman: no model here
	if e := g.net.entity(23); e == nil || e.mob != nil {
		t.Fatal("an enderman should be tracked, without a model")
	}
	if _, ok := testRes.entity["zombie_pigman"]; !ok {
		t.Fatal("the pigman's texture is not loaded")
	}
}

// The server's moves are reached in three ticks; walking swings the legs and turns the body to the way it
// faces; standing, the body follows the head after ten ticks (EntityBodyHelper).
func TestServerMobMovesAndTurns(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(30, 54, 3.5, 4, 3.5, 0, 0, nil))
	e := g.net.entity(30)
	m := e.mob.m
	w := newPacket(0x17) // move +1 x, facing west (yaw 90 = 64/256)
	w.VarInt(30)
	w.Byte(32)
	w.Byte(0)
	w.Byte(0)
	w.Byte(64)
	w.Byte(0)
	w.Bool(true)
	apply(w)
	for i := 0; i < 3; i++ {
		g.tick(input{})
	}
	if !near(m.pos[0], 4.5, 1e-9) {
		t.Fatalf("after three ticks x %v, want 4.5", m.pos[0])
	}
	if m.limbAmount <= 0 || m.limbSwing <= 0 {
		t.Fatalf("walking did not swing the legs: %v %v", m.limbSwing, m.limbAmount)
	}
	if !near(e.mob.yaw, 90, 1e-4) || !near(m.yaw, 90, 1e-4) {
		t.Fatalf("yaw %v body %v, want 90 and the body with it", e.mob.yaw, m.yaw)
	}

	h := newPacket(0x19) // the head turns back south, the body stays a while
	h.VarInt(30)
	h.Byte(0)
	apply(h)
	g.tick(input{})
	if !near(m.yaw, 75, 1e-4) {
		t.Fatalf("one tick after the head turned: body %v, want 75 (no more than 75 from the head)", m.yaw)
	}
	for i := 0; i < 25; i++ {
		g.tick(input{})
	}
	if !near(m.yaw, 0, 1e-4) || !near(m.headYaw, 0, 1e-4) {
		t.Fatalf("after standing: body %v head %v, want both 0", m.yaw, m.headYaw)
	}
}

// A blow flashes the mob and jerks its legs; Entity Status 3 lays it down; a creeper about to blow swells.
func TestServerMobHurtDeathAndFuse(t *testing.T) {
	g, _, apply := mobGame(t)
	apply(spawnMobPacket(40, 50, 3.5, 4, 3.5, 0, 0, nil))
	e := g.net.entity(40)
	apply(statusPacket(40, 2))
	if e.mob.m.hurtTime != 10 || e.mob.m.limbAmount != 1.5 {
		t.Fatalf("hurt: %d %v", e.mob.m.hurtTime, e.mob.m.limbAmount)
	}
	w := newPacket(0x1C)
	w.VarInt(40)
	metaByte(16, 1)(w)
	w.Byte(0x7F)
	apply(w)
	for i := 0; i < 40; i++ {
		g.tick(input{})
	}
	if e.mob.m.fuse != creeperFuse {
		t.Fatalf("fuse %d after 40 ticks lit, want %d", e.mob.m.fuse, creeperFuse)
	}
	apply(statusPacket(40, 3))
	g.tick(input{})
	if !e.mob.m.dead() || e.mob.m.deathTime != 1 {
		t.Fatalf("dead %v deathTime %d", e.mob.m.dead(), e.mob.m.deathTime)
	}
}

// A left click on a server's mob sends the attack; unlike a blow to a player it does not slow a sprint
// (the client's attackEntityFrom is false for a mob). Arrows are never pointed at.
func TestAttackingAServerMob(t *testing.T) {
	g, c, apply := mobGame(t)
	g.feet, g.prevFeet = vec3{0.5, 4, 0.5}, vec3{0.5, 4, 0.5}
	g.pos = g.feet.add(vec3{0, eyeHeight, 0})
	g.yaw, g.pitch = 0, 0                                 // looking south, along +z
	apply(spawnMobPacket(50, 54, 0.5, 4, 2.5, 0, 0, nil)) // a zombie, eye high
	g.net.addEntity(&netEntity{eid: 51, obj: objArrow, pos: vec3{0.5, 5, 1.5}, prev: vec3{0.5, 5, 1.5}, target: vec3{0.5, 5, 1.5}})
	for len(c.out) > 0 {
		<-c.out
	}
	g.sprinting, g.vel = true, vec3{0, 0, 0.2}
	if !g.attackClick() {
		t.Fatal("the zombie in front was not hit")
	}
	f := <-c.out
	id, r, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(f)))
	if err != nil || id != 0x02 {
		t.Fatalf("sent 0x%02x %v", id, err)
	}
	if eid, kind := r.VarInt(), r.VarInt(); eid != 50 || kind != 1 {
		t.Fatalf("attacked %d with action %d, want the zombie (50) and 1", eid, kind)
	}
	if !g.sprinting || g.vel[2] != 0.2 {
		t.Fatalf("hitting a mob slowed the sprint: sprinting %v vel %v", g.sprinting, g.vel)
	}
}
