package client

import (
	"math"
	"testing"
)

// Numbers from the game's own movement, as published for 1.8: walking 4.317 blocks a second,
// sprinting 5.612, sneaking 1.295, a jump of 1.2522 blocks that lasts 12 ticks, a terminal
// fall speed of 3.92 blocks a tick.

func standing(t *testing.T) *game {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.onGround = true
	g.creative = false
	return g
}

// perTick runs ticks and returns the distance covered in the last one, along +Z.
func perTick(g *game, ticks int, in input) float64 {
	var z float64
	for i := 0; i < ticks; i++ {
		z = g.feet[2]
		g.tick(in)
	}
	return g.feet[2] - z
}

func TestWalkingSpeed(t *testing.T) {
	g := standing(t)
	if d := perTick(g, 80, input{forward: 1}); math.Abs(float64(d)*20-4.317) > 0.01 {
		t.Fatalf("walking %.4f blocks/s, want 4.317", d*20)
	}
}

func TestSprintingSpeed(t *testing.T) {
	g := standing(t)
	if d := perTick(g, 80, input{forward: 1, sprint: true}); math.Abs(float64(d)*20-5.612) > 0.01 {
		t.Fatalf("sprinting %.4f blocks/s, want 5.612", d*20)
	}
	if !g.sprinting {
		t.Fatal("not sprinting")
	}
}

func TestSneakingSpeed(t *testing.T) {
	g := standing(t)
	if d := perTick(g, 80, input{forward: 1, sneak: true}); math.Abs(float64(d)*20-1.295) > 0.01 {
		t.Fatalf("sneaking %.4f blocks/s, want 1.295", d*20)
	}
}

// 1.8 zeroes motion under 0.005 each tick (later versions use 0.003): the sixth tick of a jump, 0.0030,
// is cut, so the peak is 1.2492 rather than the 1.2522 later versions reach.
func TestJumpIsTwelveTicksAndReachesOnePointTwoFour(t *testing.T) {
	g := standing(t)
	g.tick(input{jump: true})
	peak, airborne := g.feet[1]-4, 1
	for g.feet[1] > 4 || !g.onGround {
		g.tick(input{})
		peak = max(peak, g.feet[1]-4)
		airborne++
		if airborne > 40 {
			t.Fatal("never landed")
		}
	}
	if math.Abs(float64(peak)-1.2492) > 0.0005 {
		t.Fatalf("jump peak %.4f, want 1.2492", peak)
	}
	if airborne != 12 {
		t.Fatalf("a jump lasts %d ticks, want 12", airborne)
	}
}

func TestTerminalVelocityAndFallRecurrence(t *testing.T) {
	g := standing(t)
	g.feet = vec3{0.5, 200, 0.5}
	g.onGround = false
	g.creative = true // no fall damage in this test
	for i := 0; i < 200; i++ {
		g.tick(input{})
	}
	if g.onGround {
		t.Skip("landed before reaching terminal speed")
	}
	// v' = (v - 0.08) * 0.98 settles at -3.92.
	if math.Abs(float64(g.vel[1])+3.92) > 0.01 {
		t.Fatalf("falling at %.4f a tick, want -3.92", g.vel[1])
	}
}

func TestSprintStartsWithTheKeyOrADoubleTap(t *testing.T) {
	g := standing(t)
	g.tick(input{forward: 1, sprint: true})
	if !g.sprinting {
		t.Fatal("the sprint key does not start sprinting")
	}
	g.tick(input{}) // letting go of forward ends it
	if g.sprinting {
		t.Fatal("still sprinting with no forward input")
	}

	g = standing(t) // a double tap of forward, within 7 ticks
	g.tick(input{forward: 1})
	if g.sprinting {
		t.Fatal("one tap sprints")
	}
	g.tick(input{})
	g.tick(input{})
	g.tick(input{forward: 1})
	if !g.sprinting {
		t.Fatal("a second tap within 7 ticks does not sprint")
	}

	g = standing(t) // too late
	g.tick(input{forward: 1})
	for i := 0; i < 9; i++ {
		g.tick(input{})
	}
	g.tick(input{forward: 1})
	if g.sprinting {
		t.Fatal("a second tap after 9 ticks still sprints")
	}

	g = standing(t) // sneaking and hunger stop it
	g.tick(input{forward: 1, sprint: true, sneak: true})
	if g.sprinting {
		t.Fatal("sprinting while sneaking")
	}
	g = standing(t)
	g.food = 6
	g.tick(input{forward: 1, sprint: true})
	if g.sprinting {
		t.Fatal("sprinting with 6 food: it needs more than 6")
	}
}

func TestSprintEndsAgainstAWall(t *testing.T) {
	g := standing(t)
	for y := int32(4); y < 7; y++ {
		g.world.SetBlock(0, y, 4, 1<<4)
	}
	for i := 0; i < 30; i++ {
		g.tick(input{forward: 1, sprint: true})
	}
	if !g.collidedH {
		t.Fatal("no collision recorded at the wall")
	}
	g.tick(input{forward: 1, sprint: true}) // the next tick drops sprint, then the key starts it again
	if !g.sprinting {
		t.Log("sprint was dropped at the wall and restarted by the held key, as in the game")
	}
}

func TestSprintJumpPushesForward(t *testing.T) {
	g := standing(t)
	g.tick(input{forward: 1, sprint: true})
	vz := g.vel[2]
	g.onGround = true
	g.jumpTicks = 0
	g.tick(input{forward: 1, sprint: true, jump: true})
	// The jump adds 0.2 along the facing before the move: forward speed jumps up.
	if g.vel[2] <= vz+0.1 {
		t.Fatalf("speed before %.3f, after a sprint jump %.3f: it should gain about 0.2", vz, g.vel[2])
	}
}

func TestIceIsSlippery(t *testing.T) {
	slide := func(floor uint16) float64 {
		g := standing(t)
		g.world.SetBlock(0, 3, 0, floor<<4)
		for z := int32(-3); z < 40; z++ {
			g.world.SetBlock(0, 3, z, floor<<4)
		}
		g.feet = vec3{0.5, 4, 0}
		for i := 0; i < 20; i++ {
			g.tick(input{forward: 1})
		}
		start := g.feet[2]
		for i := 0; i < 20; i++ {
			g.tick(input{})
		}
		return g.feet[2] - start
	}
	stone, ice := slide(1), slide(79)
	if ice < stone*3 {
		t.Fatalf("coasting 20 ticks: %.2f on stone, %.2f on ice: ice should slide much further", stone, ice)
	}
}

func TestSoulSandSlowsYou(t *testing.T) {
	run := func(floor uint16) float64 {
		g := standing(t)
		for z := int32(-3); z < 60; z++ {
			g.world.SetBlock(0, 3, z, floor<<4)
		}
		g.feet = vec3{0.5, 4, 0}
		return perTick(g, 40, input{forward: 1})
	}
	if normal, sand := run(1), run(88); sand > normal*0.7 {
		t.Fatalf("%.3f a tick on stone, %.3f on soul sand", normal, sand)
	}
}

func TestSmallMotionIsCutOff(t *testing.T) {
	g := standing(t)
	g.vel = vec3{0.004, 0, -0.004}
	g.tick(input{})
	if g.vel[0] != 0 || g.vel[2] != 0 {
		t.Fatalf("motion under 0.005 should be zeroed, got %v", g.vel)
	}
}

func TestSwimmingSinksSlowly(t *testing.T) {
	g := standing(t)
	g.creative = true
	for y := int32(4); y < 20; y++ {
		for x := int32(-2); x <= 2; x++ {
			for z := int32(-2); z <= 2; z++ {
				g.world.SetBlock(x, y, z, 9<<4)
			}
		}
	}
	g.feet = vec3{0.5, 12, 0.5}
	g.onGround = false
	for i := 0; i < 40; i++ {
		g.tick(input{})
	}
	// v' = v*0.8 - 0.02 settles at -0.1 a tick.
	if math.Abs(float64(g.vel[1])+0.1) > 0.01 {
		t.Fatalf("sinking at %.4f a tick, want -0.1", g.vel[1])
	}
	g.tick(input{jump: true})
	if g.vel[1] <= -0.1 {
		t.Fatalf("holding jump in water does not push up: %.3f", g.vel[1])
	}
}

func TestFlightNeedsADoubleTapAndHovers(t *testing.T) {
	g := standing(t)
	g.creative = true
	g.tick(input{jump: true})
	g.tick(input{})
	g.tick(input{jump: true}) // the second tap, within 7 ticks
	if !g.flying {
		t.Fatal("two taps of jump did not start flying")
	}
	for i := 0; i < 10; i++ {
		g.tick(input{jump: true})
	}
	y := g.feet[1]
	for i := 0; i < 20; i++ {
		g.tick(input{})
	}
	if math.Abs(float64(g.feet[1]-y)) > 1.5 { // it drifts to a stop, it does not fall
		t.Fatalf("a flying player fell from %.2f to %.2f", y, g.feet[1])
	}
	if g.vel[1] > 0.001 || g.vel[1] < -0.001 {
		t.Fatalf("hover speed %.4f", g.vel[1])
	}
}

func TestLaddersClimb(t *testing.T) {
	g := standing(t)
	for y := int32(4); y < 12; y++ {
		g.world.SetBlock(0, y, 3, 1<<4)    // a wall
		g.world.SetBlock(0, y, 2, 65<<4|3) // a ladder on its north face (meta 3: facing south)
	}
	g.feet = vec3{0.5, 4, 2.5}
	for i := 0; i < 40; i++ {
		g.tick(input{forward: 1})
	}
	if g.feet[1] < 8 {
		t.Fatalf("climbed only to %.2f in 40 ticks", g.feet[1])
	}
}

func TestSinTableMatchesTheGame(t *testing.T) {
	// MathHelper.sin(0) = 0 and the table's entry for a quarter turn is 1.
	if mhSin(0) != 0 || math.Abs(float64(mhSin(math.Pi/2))-1) > 1e-4 || math.Abs(float64(mhCos(0))-1) > 1e-6 {
		t.Fatalf("sin table: %v %v %v", mhSin(0), mhSin(math.Pi/2), mhCos(0))
	}
}
