package client

import (
	"math"
	"testing"
)

// What follows pins movement details to the game's own bytecode (1.8.9's EntityLivingBase, EntityPlayer and
// EntityPlayerSP), not to the published rounded figures of movement_test.go.

// The attribute values are the game's arithmetic run in Java 8: a double base of the float 0.1, each operation-2
// modifier multiplying the total in the order of the game's set (Slowness, sprint, Speed), then (float).
func TestMovementSpeedAttributeIsTheGames(t *testing.T) {
	for _, c := range []struct {
		name    string
		sprint  bool
		effects map[int]potionEffect
		want    float32
	}{
		{"walking", false, nil, 0.1},
		{"sprinting", true, nil, 0.13000001}, // not 0.13: the modifier is the float 0.3F widened
		{"speed I", false, map[int]potionEffect{effSpeed: {amp: 0}}, 0.120000005},
		{"speed II sprinting", true, map[int]potionEffect{effSpeed: {amp: 1}}, 0.18200001},
		{"slowness I sprinting", true, map[int]potionEffect{effSlowness: {amp: 0}}, 0.1105},
		{"slowness II, sprint, speed III", true, map[int]potionEffect{effSlowness: {amp: 1}, effSpeed: {amp: 2}}, 0.1456},
	} {
		g := standing(t)
		g.sprinting, g.effects = c.sprint, c.effects
		if got := g.groundSpeed(); got != c.want {
			t.Errorf("%s: movement speed %v, want %v", c.name, got, c.want)
		}
	}
	g := standing(t)
	g.effects = map[int]potionEffect{effSlowness: {amp: 9}} // ten levels of slowness would be below zero: the attribute stops at 0
	if got := g.groundSpeed(); got != 0 {
		t.Errorf("a speed below zero is %v, want 0", got)
	}
}

// The speed and the air factor a tick moves with are the ones the tick before ended with, so a sprint counts
// from the tick after it starts.
func TestSprintCountsOneTickAfterItStarts(t *testing.T) {
	sprinter, walker := standing(t), standing(t)
	sprinter.tick(input{})
	walker.tick(input{})
	sprinter.tick(input{forward: 1, sprint: true})
	walker.tick(input{forward: 1})
	if !sprinter.sprinting || walker.sprinting {
		t.Fatalf("sprinting: %v and %v", sprinter.sprinting, walker.sprinting)
	}
	if sprinter.vel[2] != walker.vel[2] {
		t.Fatalf("the tick a sprint starts still walks: %v against %v", sprinter.vel[2], walker.vel[2])
	}
	sprinter.tick(input{forward: 1, sprint: true})
	walker.tick(input{forward: 1})
	if sprinter.vel[2] <= walker.vel[2] {
		t.Fatalf("the next tick sprints: %v against %v", sprinter.vel[2], walker.vel[2])
	}
	if sprinter.jumpFactor != 0.025999999 || walker.jumpFactor != 0.02 { // 0.02F + 0.02F * 0.3D
		t.Fatalf("air factors %v and %v, want 0.026 and 0.02", sprinter.jumpFactor, walker.jumpFactor)
	}
}

// A new player has the air factor 0.02 but no movement speed yet: it can only start moving a tick later.
func TestANewPlayerCannotWalkInItsFirstTick(t *testing.T) {
	g := standing(t)
	g.tick(input{forward: 1})
	if g.feet[2] != 0.5 || g.vel[2] != 0 {
		t.Fatalf("moved in the first tick: z %v, speed %v", g.feet[2], g.vel[2])
	}
	g.tick(input{forward: 1})
	if g.vel[2] <= 0 {
		t.Fatal("still not moving in the second tick")
	}
}

// 1.8 has no faster stroke for a sprinting swimmer: the water slows to 0.8 either way.
func TestSprintingSwimmerIsNotFaster(t *testing.T) {
	swim := func(sprint bool) float64 {
		g := standing(t)
		g.creative = true
		for y := int32(4); y < 20; y++ {
			for x := int32(-3); x <= 3; x++ {
				for z := int32(-3); z <= 60; z++ {
					g.world.SetBlock(x, y, z, 9<<4)
				}
			}
		}
		g.feet = vec3{0.5, 12, 0.5}
		g.onGround = false
		for i := 0; i < 30; i++ {
			g.tick(input{forward: 1, sprint: sprint})
		}
		if sprint && !g.sprinting {
			t.Fatal("not sprinting in the water")
		}
		return g.feet[2]
	}
	if walk, sprint := swim(false), swim(true); walk != sprint {
		t.Fatalf("30 ticks of swimming reached z %v walking and %v sprinting", walk, sprint)
	}
}

func TestHoldingJumpInAFluidAddsTheGamesFloat(t *testing.T) {
	if swimUp != 0.03999999910593033 {
		t.Fatalf("the push up in a fluid is %v, want 0.03999999910593033", swimUp)
	}
}

// EntityPlayerSP.setSprinting gives a sprint 600 ticks; a key held starts it again, a double tap does not.
func TestSprintRunsOutAfterSixHundredTicks(t *testing.T) {
	g := standing(t)
	g.tick(input{})
	g.setSprinting(true)
	for i := 1; i <= 600; i++ {
		g.tick(input{forward: 1}) // no sprint key
		g.feet[2], g.vel = 0.5, vec3{}
		if want := i < 600; g.sprinting != want {
			t.Fatalf("after %d ticks sprinting is %v", i, g.sprinting)
		}
	}
	k := standing(t)
	k.tick(input{})
	for i := 0; i < 700; i++ {
		k.tick(input{forward: 1, sprint: true})
		k.feet[2], k.vel = 0.5, vec3{}
		if !k.sprinting {
			t.Fatalf("the sprint key did not hold the sprint at tick %d", i)
		}
	}
}

func TestUsingAnItemSlowsAndForgetsATap(t *testing.T) {
	g := standing(t)
	g.tick(input{})
	g.eating = 3
	if !g.usingItem() {
		t.Fatal("eating is using an item")
	}
	g.blocking, g.eating = true, 0
	g.sprintToggle = 5
	g.tick(input{forward: 1})
	if g.sprintToggle != 0 {
		t.Fatalf("a first tap of forward is remembered while an item is used: %d", g.sprintToggle)
	}
	if g.sprinting {
		t.Fatal("sprinting while using an item")
	}
}

func TestSprintNeedsFoodOrPermissionToFlyAndSight(t *testing.T) {
	hungry := func(setup func(g *game)) bool {
		g := standing(t)
		g.tick(input{})
		g.food = 3
		setup(g)
		g.tick(input{forward: 1, sprint: true})
		return g.sprinting
	}
	if hungry(func(g *game) {}) {
		t.Error("sprinting with 3 food")
	}
	if !hungry(func(g *game) { g.allowFly = true }) {
		t.Error("a player a server lets fly can sprint hungry")
	}
	g := standing(t)
	g.tick(input{})
	g.effects = map[int]potionEffect{effBlindness: {amp: 0, ticks: 100}}
	g.tick(input{forward: 1, sprint: true})
	if g.sprinting {
		t.Error("sprinting while blind")
	}
}

// After the move the game asks again whether the player is on a ladder: one that has left it at the top is
// not pushed up any more.
func TestLadderIsAskedAgainAfterTheMove(t *testing.T) {
	g := standing(t)
	for y := int32(4); y < 20; y++ {
		g.world.SetBlock(0, y, 3, 1<<4) // a wall
	}
	for y := int32(4); y < 8; y++ {
		g.world.SetBlock(0, y, 2, 65<<4|3) // a ladder up to y 7
	}
	g.feet = vec3{0.5, 7.95, 2.69}
	g.onGround = false
	g.tick(input{}) // sets the speeds
	g.feet = vec3{0.5, 7.95, 2.69}
	g.vel = vec3{0, 0.1, 0.1}
	g.tick(input{})
	if !g.collidedH {
		t.Fatal("no contact with the wall")
	}
	if g.feet[1] < 8 {
		t.Fatalf("still on the ladder at %.3f", g.feet[1])
	}
	if g.vel[1] > 0.05 { // (0.1 - 0.08) * 0.98, not the ladder's 0.2 pushed through gravity
		t.Fatalf("pushed up from a ladder it had left: %.4f", g.vel[1])
	}
}

func TestFlightThrustFollowsTheServersSpeed(t *testing.T) {
	thrust := func(cap float32) float64 {
		g := standing(t)
		g.creative, g.flying, g.onGround = true, true, false
		g.feet = vec3{0.5, 100, 0.5}
		g.flyCap = cap
		g.tick(input{})
		g.vel = vec3{}
		g.tick(input{jump: true})
		return g.vel[1]
	}
	if def, fast := thrust(0), thrust(0.1); math.Abs(def-0.09) > 1e-6 || math.Abs(fast-0.18) > 1e-6 {
		t.Fatalf("hover climb %.6f at 0.05 and %.6f at 0.1, want 0.09 and 0.18", def, fast)
	}
}

// Landing ends a flight once the player has moved, in the same tick: the flag is cleared after the move. A
// player that hovers just above the ground has not landed (it has no downward motion to stop).
func TestLandingEndsAFlightAfterTheMove(t *testing.T) {
	g := standing(t)
	g.creative, g.flying, g.onGround = true, true, false
	g.feet = vec3{0.5, 6, 0.5}
	g.tick(input{})
	if !g.flying {
		t.Fatal("a hovering player stopped flying")
	}
	for i := 0; i < 60; i++ {
		g.tick(input{sneak: true}) // descend
		if g.onGround != !g.flying {
			t.Fatalf("tick %d: on the ground %v but flying %v", i, g.onGround, g.flying)
		}
		if g.onGround {
			return
		}
	}
	t.Fatal("never landed")
}
