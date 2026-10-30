package client

import "testing"

func TestDrawPower(t *testing.T) {
	// ItemBow: (f*f + f*2) / 3, capped at 1; a full draw takes 20 ticks.
	if got := drawPower(20); got != 1 {
		t.Fatalf("full draw %v", got)
	}
	if got := drawPower(10); !near(got, (0.25+1)/3, 1e-6) {
		t.Fatalf("half draw %v", got)
	}
	if got := drawPower(40); got != 1 {
		t.Fatalf("overdrawn %v", got)
	}
}

func TestArrowFliesAndSticks(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	a := &arrow{pos: vec3{0.5, 5.6, 0.5}, vel: vec3{0, 0, 3}, damage: 2}
	a.prev = a.pos
	g.arrows = []*arrow{a}
	g.tickArrow(a)
	// One tick: moves by its velocity, then drag 0.99 and gravity 0.05.
	if !near(a.pos[2], 3.5, 1e-4) || !near(a.vel[2], 2.97, 1e-4) || !near(a.vel[1], -0.05, 1e-4) {
		t.Fatalf("after a tick: pos %v vel %v", a.pos, a.vel)
	}
	for i := 0; i < 200 && !a.inGround; i++ {
		g.tickArrow(a)
	}
	if !a.inGround {
		t.Fatalf("the arrow never landed: %v", a.pos)
	}
	if a.pos[1] > 4.2 {
		t.Fatalf("stuck above the floor: %v", a.pos)
	}
}

func TestArrowHurtsMob(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 20.5}
	m := g.newMob(kinds["Zombie"], vec3{0.5, 4, 8.5}, 0)
	g.mobs = []*mob{m}
	a := &arrow{pos: vec3{0.5, 5.0, 0.5}, vel: vec3{0, 0, 3}, damage: 2}
	a.prev = a.pos
	for i := 0; i < 10 && m.health == m.kind.health; i++ {
		g.tickArrow(a)
	}
	// ceil(speed 3 * 2) = 6, less the tick of drag before it arrived.
	if lost := m.kind.health - m.health; lost < 5 || lost > 6 {
		t.Fatalf("arrow took %v health", lost)
	}
}

func TestSkeletonShoots(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 10.5}
	m := g.newMob(kinds["Skeleton"], vec3{0.5, 4, 0.5}, 0)
	g.mobs = []*mob{m}
	for i := 0; i < 30 && len(g.arrows) == 0; i++ {
		g.tickMobs()
	}
	if len(g.arrows) == 0 {
		t.Fatal("the skeleton never shot")
	}
}

func TestBotFightsBack(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.pos = vec3{0.5, 4 + eyeHeight, 0.5}
	g.onGround = true
	g.spawnBot()
	if len(g.mobs) != 1 {
		t.Fatal("no bot")
	}
	for i := 0; i < 120 && g.health == maxHealth; i++ {
		g.tickMobs()
		g.hurtResist = max(g.hurtResist-1, 0)
	}
	if g.health >= maxHealth {
		t.Fatal("the bot never reached or hit the player")
	}
}
