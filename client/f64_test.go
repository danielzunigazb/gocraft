package client

import (
	"math"
	"testing"
)

// These tests need positions in doubles. Several fail with float32 positions, which is what the old
// contactEps hack papered over, and together they pin the movement to the numbers of 1.8.9's doubles.

// wallWorld builds a stone floor and a stone wall at x = base+5 around base, along z.
func wallWorld(g *game, base int32) {
	for z := int32(-8); z < 8; z++ {
		for y := int32(4); y < 8; y++ {
			g.world.SetBlock(base+5, y, z, 1<<4)
		}
		for x := base - 8; x < base+8; x++ {
			g.world.SetBlock(x, 3, z, 1<<4)
		}
	}
}

// A player who pushes against a wall must stop at the wall, from any place along it and far from the
// origin. The game keeps the bounding box as state and takes the position from it; here the position is
// the state and the box is rebuilt from it each tick. That is exact, because a block's edge minus the
// width (a float, 24 bits) and the average of the two edges are exact doubles up to ~2^28 blocks: the
// box that comes back is the box that was clipped, and no tolerance is needed.
func TestWallStopsThePlayerFromAnyPlace(t *testing.T) {
	for _, base := range []int32{0, 10000, 5_000_000, -3_000_000} {
		g := standing(t)
		wallWorld(g, base)
		for i := 0; i < 400; i++ {
			// A start place that is not a round number, pushing towards +x (the game's yaw -90).
			g.feet = vec3{float64(base) + 4.0 + float64(i)*0.0017321, 4, float64(i%7) * 0.113}
			g.vel, g.onGround, g.sprinting = vec3{}, true, false
			g.yaw = -math.Pi / 2
			for k := 0; k < 12; k++ {
				g.tick(input{forward: 1})
				if edge := g.feet[0] + playerHalfW; edge > float64(base)+5 {
					t.Fatalf("base %d, start %d, tick %d: the body reaches x=%.17g, inside the wall at %d", base, i, k, edge, base+5)
				}
			}
			// And it ends up touching it, to the last bit.
			if edge := g.feet[0] + playerHalfW; edge != float64(base)+5 {
				t.Fatalf("base %d, start %d: stopped at %.17g, not against the wall at %d", base, i, edge, base+5)
			}
		}
	}
}

// Walking in a straight line, a player covers the same distance every tick however far from the origin.
// With float32 positions, at z=10000 the step (0.2158) would jitter by half a percent: a float's spacing
// there is a thousandth of a block. The steady distance is the closed form of the game's recurrence:
// the input is added to the motion, the player moves by it, then the motion is cut by the friction.
func TestWalkingFiveThousandBlocksHasNoDrift(t *testing.T) {
	g := standing(t)
	g.creative = true // no hostile spawns, no hunger over a long walk; the walking itself is the same
	// The numbers the game uses, in the floats it uses them: ground friction 0.6F * 0.91F, the speed
	// attribute 0.1F, a forward input of 0.98F, all widened to double when they meet the motion.
	slip := slipperiness(2) * 0.91
	f := float32(0.16277136) / (slip * slip * slip)
	friction := landSpeed * f
	accel := float64(float32(0.98) * friction)
	drag := float64(slip)
	want := accel / (1 - drag)
	if math.Abs(want*20-4.317) > 0.001 {
		t.Fatalf("the closed form walks %.4f blocks a second, want the published 4.317", want*20)
	}

	g.feet = vec3{0.5, 4, 10000.5}
	g.prevFeet = g.feet
	g.yaw = 0 // facing +z
	var steps []float64
	for g.feet[2] < 15000.5 {
		z := g.feet[2]
		g.tick(input{forward: 1})
		steps = append(steps, g.feet[2]-z)
		if len(steps) > 40000 {
			t.Fatal("never got there")
		}
	}
	if len(steps) < 22000 || len(steps) > 24000 {
		t.Fatalf("5000 blocks took %d ticks, want about %.0f", len(steps), 5000/want)
	}
	worst := 0.0
	for _, s := range steps[80:] { // past the first few ticks of acceleration
		worst = max(worst, math.Abs(s-want))
	}
	// Positions near 15000 are spaced 1.8e-12 apart, so a difference of two is good to about 4e-12.
	if worst > 2e-11 {
		t.Fatalf("a tick's distance strays %.3g from the %.12f it should be: drift", worst, want)
	}
	if g.feet[0] != 0.5 {
		t.Fatalf("walked sideways to x=%.17g", g.feet[0])
	}
}

// A fall in the air follows the closed form of motionY = (motionY - 0.08) * 0.98F in doubles, for twenty
// seconds and 1500 blocks, to a few billionths of a block.
func TestTwentySecondFallMatchesTheClosedForm(t *testing.T) {
	g := standing(t)
	g.feet = vec3{0.5, 3000, 0.5}
	g.prevFeet = g.feet
	g.onGround = false
	d := dragY // 0.98F as a double
	c := 0.08 * d / (1 - d)
	for n := 1; n <= 400; n++ {
		g.tick(input{})
		// y after n ticks: the sum of the motions the player moved by (the first is 0), v_k = -c (1 - d^k).
		want := 3000 - c*(float64(n)-(1-math.Pow(d, float64(n)))/(1-d))
		if math.Abs(g.feet[1]-want) > 1e-8 {
			t.Fatalf("tick %d: y = %.12f, the closed form gives %.12f", n, g.feet[1], want)
		}
	}
	// The motion tends to -0.08*0.98/0.02 = -3.92 blocks a tick; after 400 ticks it is v_400 = -c (1 - d^400).
	if want := -c * (1 - math.Pow(d, 400)); math.Abs(g.vel[1]-want) > 1e-9 {
		t.Fatalf("motion %.12f after 400 ticks, want %.12f", g.vel[1], want)
	}
}

// A position given from outside can sit a hair inside a block (eye height minus 1.62 does not come out
// exactly). The movement code stops boxes that touch blocks and has nothing for one already inside, so
// the spawn code must bring it to the surface: otherwise it falls through the floor.
func TestPositionAHairInsideTheGroundStandsOnIt(t *testing.T) {
	g := standing(t)
	for _, y := range []float64{4 - 1e-7, 4 - 1e-9, 5.62 - 1.62, float64(float32(5.62)) - 1.62, 4} {
		g.feet = g.freeSpot(vec3{0.5, y, 0.5})
		if g.feet[1] != 4 {
			t.Fatalf("a start at y=%.17g came out at %.17g, want the surface at 4", y, g.feet[1])
		}
		g.vel, g.onGround = vec3{}, false
		for i := 0; i < 10; i++ {
			g.tick(input{})
		}
		if g.feet[1] != 4 || !g.onGround {
			t.Fatalf("a start at y=%.17g ended at %.17g (on ground %v)", y, g.feet[1], g.onGround)
		}
	}
	// Against a wall: a hair into it, the player is put out and then stopped by it.
	wallWorld(g, 0)
	g.feet = g.freeSpot(vec3{5 - playerHalfW + 1e-7, 4, 0.5})
	if g.feet[0]+playerHalfW > 5 {
		t.Fatalf("still inside the wall at x=%.17g", g.feet[0])
	}
	g.yaw = -math.Pi / 2
	g.onGround = true
	for i := 0; i < 8; i++ {
		g.tick(input{forward: 1})
	}
	if g.feet[0]+playerHalfW > 5 {
		t.Fatalf("walked into the wall: x=%.17g", g.feet[0])
	}
}
