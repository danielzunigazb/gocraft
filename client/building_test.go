package client

import (
	"gocraft/world"
	"math"
	"testing"
)

// aim turns the player to look at a point.
func aim(g *game, to vec3) {
	d := to.sub(g.pos)
	n := math.Sqrt(d.dot(d))
	g.yaw = float32(math.Atan2(-d[0], d[2]))
	g.pitch = float32(-math.Asin(d[1] / n))
}

func builder(t *testing.T) *game {
	g := testGame(t)
	g.creative = true
	g.feet = vec3{0.5, 4, 0.5}
	g.pos = vec3{0.5, 4 + eyeHeight, 0.5}
	g.onGround = true
	g.inv[0], g.sel = blockSlot(1<<4, 64), 0
	return g
}

func TestRayHitsTheFaceTowardsYou(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 5, 3, 1<<4)
	h, ok := g.rayTrace(vec3{0.5, 5.5, 0.5}, vec3{0.5, 5.5, 5})
	if !ok || h.pos != [3]int32{0, 5, 3} || h.side != dNorth || h.at[2] != 3 {
		t.Fatalf("hit %+v %v: want the north face of 0,5,3 at z=3", h, ok)
	}
	// From above it is the top face; from the east, the east face.
	h, _ = g.rayTrace(vec3{0.5, 8, 3.5}, vec3{0.5, 4, 3.5})
	if h.side != dUp || h.at[1] != 6 {
		t.Fatalf("from above: %+v", h)
	}
	g.world.SetBlock(5, 5, 5, 1<<4)
	h, _ = g.rayTrace(vec3{9, 5.5, 5.5}, vec3{3, 5.5, 5.5})
	if h.side != dEast || h.at[0] != 6 {
		t.Fatalf("from the east: %+v", h)
	}
}

func TestRayPassesOverASlabAndHitsItsHalf(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 5, 3, 44<<4) // a bottom slab: 0.5 high
	if _, ok := g.rayTrace(vec3{0.5, 5.7, 0.5}, vec3{0.5, 5.7, 5}); ok {
		t.Fatal("a ray above the half block still hit the slab")
	}
	h, ok := g.rayTrace(vec3{0.5, 5.3, 0.5}, vec3{0.5, 5.3, 5})
	if !ok || h.side != dNorth {
		t.Fatalf("a ray at the slab's height: %+v %v", h, ok)
	}
	// The top face of the slab is at 5.5, not 6.
	if h, _ := g.rayTrace(vec3{0.5, 8, 3.5}, vec3{0.5, 4, 3.5}); h.at[1] != 5.5 {
		t.Fatalf("slab top at %v", h.at[1])
	}
}

func TestStairsAreHitByTheirOwnSteps(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 5, 3, 53<<4) // stairs facing east: the high half is x 0.5..1
	if _, ok := g.rayTrace(vec3{0.2, 5.7, 0.5}, vec3{0.2, 5.7, 5}); ok {
		t.Fatal("a ray over the low half of a stair hit it")
	}
	if _, ok := g.rayTrace(vec3{0.8, 5.7, 0.5}, vec3{0.8, 5.7, 5}); !ok {
		t.Fatal("a ray over the high half missed the stair")
	}
}

func TestFenceCollidesTallButIsSelectedAtOneBlock(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 4, 3, 85<<4)
	if _, ok := g.rayTrace(vec3{0.5, 5.2, 0.5}, vec3{0.5, 5.2, 5}); ok {
		t.Fatal("a ray 1.2 up still selected the fence")
	}
	if _, ok := g.rayTrace(vec3{0.5, 4.7, 0.5}, vec3{0.5, 4.7, 5}); !ok {
		t.Fatal("a ray through the fence post missed")
	}
	// Collision is 1.5 high: a normal jump (1.25) cannot clear it.
	g.creative = false
	g.feet = vec3{0.5, 4, 1.5}
	for i := 0; i < 40; i++ {
		g.tick(input{forward: 1, jump: i%12 == 0})
	}
	if g.feet[2] > 3.1 { // it stops against the post, at z = 3.375 - 0.3
		t.Fatalf("got past a fence: z=%.2f", g.feet[2])
	}
}

func TestPlacingOnTopAndOnTheSide(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 4, 3, 1<<4)
	aim(g, vec3{0.5, 5, 3.5}) // the top face of the block
	g.useBlock()
	if g.world.Block(0, 5, 3)>>4 != 1 {
		t.Fatalf("nothing above the block: %d", g.world.Block(0, 5, 3)>>4)
	}
	if g.inv[0].N != 64 { // creative does not use blocks up
		t.Fatalf("count %d", g.inv[0].N)
	}
	g.creative = false
	aim(g, vec3{0.5, 4.5, 3}) // its near face
	g.useBlock()
	if g.world.Block(0, 4, 2)>>4 != 1 || g.inv[0].N != 63 {
		t.Fatalf("the side block: %d, count %d", g.world.Block(0, 4, 2)>>4, g.inv[0].N)
	}
}

func TestPlacingReplacesTallGrassAndSnow(t *testing.T) {
	for _, c := range []struct {
		name string
		put  uint16
		want bool
	}{{"tall grass", 31<<4 | 1, true}, {"dead bush", 32 << 4, true}, {"a single snow layer", 78 << 4, true},
		{"two snow layers", 78<<4 | 1, false}, {"a flower", 38 << 4, false}, {"wheat", 59 << 4, false}, {"fern in a double plant", 175<<4 | 3, true}} {
		g := builder(t)
		g.world.SetBlock(0, 4, 3, c.put)
		aim(g, vec3{0.5, 4.3, 3.5})
		g.useBlock()
		got := g.world.Block(0, 4, 3)>>4 == 1
		if got != c.want {
			t.Errorf("%s: replaced=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestPlacingGoesBesideWhatCannotBeReplaced(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 4, 3, 38<<4) // a flower
	aim(g, vec3{0.5, 4.3, 3.5})
	g.useBlock()
	if g.world.Block(0, 4, 3)>>4 != 38 {
		t.Fatal("a flower was replaced")
	}
}

func TestNoPlacingInsideThePlayer(t *testing.T) {
	g := builder(t)
	g.feet = vec3{1.3, 4, 0.5} // standing at x=1, beside the block we click
	g.pos = vec3{1.3, 4 + eyeHeight, 0.5}
	g.world.SetBlock(0, 4, 0, 1<<4)
	aim(g, vec3{1, 4.5, 0.5}) // the east face of 0,4,0: the new block would be at 1,4,0, where the player stands
	g.useBlock()
	if g.world.Block(1, 4, 0)>>4 != 0 {
		t.Fatal("a block was placed inside the player")
	}
	g.feet = vec3{3.5, 4, 0.5} // the player steps away: the same click now works
	g.pos = vec3{3.5, 4 + eyeHeight, 0.5}
	aim(g, vec3{1, 4.5, 0.5})
	g.useBlock()
	if g.world.Block(1, 4, 0)>>4 != 1 {
		t.Fatal("a free cell was refused")
	}
}

func TestSlabsAndStairsTakeTheirOrientationFromTheClick(t *testing.T) {
	// Slab: clicked on the upper half of a side face it goes in the top half.
	if got := placedState(44<<4, dNorth, 0.8, 0); got != 44<<4|8 {
		t.Errorf("slab on the upper half of a side: %d", got)
	}
	if got := placedState(44<<4, dNorth, 0.2, 0); got != 44<<4 {
		t.Errorf("slab on the lower half of a side: %d", got)
	}
	if got := placedState(44<<4, dUp, 0.9, 0); got != 44<<4 {
		t.Errorf("slab on a top face is always the bottom half: %d", got)
	}
	if got := placedState(44<<4|3, dDown, 0.1, 0); got != 44<<4|3|8 {
		t.Errorf("slab on a bottom face keeps its kind and goes up: %d", got)
	}
	// Stairs face the way the player looks: yaw 0 is south, 90 west, 180 north, 270 east.
	for yaw, want := range map[float32]uint16{0: 2, 90: 1, 180: 3, 270: 0, -90: 0} {
		if got := placedState(53<<4, dUp, 0.5, yaw); got&3 != want {
			t.Errorf("stairs placed facing yaw %v: facing bits %d, want %d", yaw, got&3, want)
		}
	}
	if got := placedState(53<<4, dNorth, 0.9, 0); got&4 == 0 {
		t.Error("stairs on the upper half of a side go upside down")
	}
	// Logs lie along the axis of the face they are put on.
	for side, want := range map[int]uint16{dUp: 0, dDown: 0, dNorth: 8, dSouth: 8, dWest: 4, dEast: 4} {
		if got := placedState(17<<4|2, side, 0.5, 0); got != 17<<4|2|want {
			t.Errorf("log on face %d: %d", side, got)
		}
	}
}

func TestTwoSlabsMakeADoubleSlab(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 4, 3, 44<<4|3) // cobblestone slab, bottom half
	g.inv[0] = blockSlot(44<<4|3, 64)
	aim(g, vec3{0.5, 4.5, 3.5}) // its top face
	g.useBlock()
	if got := g.world.Block(0, 4, 3); got != 43<<4|3 {
		t.Fatalf("got %d, want a double cobblestone slab", got)
	}
}

func TestStairShapeAtCorners(t *testing.T) {
	g := builder(t)
	g.world.SetBlock(0, 5, 0, 53<<4|3) // facing north
	g.world.SetBlock(0, 5, -1, 53<<4)  // in front of it, facing east
	nb := func(dx, dy, dz int32) uint16 { return g.world.Block(0+dx, 5+dy, 0+dz) }
	if got := stairShape(53<<4|3, nb); got != "outer_right" {
		t.Fatalf("a stair with another turning away in front: %s, want outer_right", got)
	}
	g.world.SetBlock(0, 5, -1, 0)
	g.world.SetBlock(0, 5, 1, 53<<4) // behind it, facing east
	if got := stairShape(53<<4|3, nb); got != "inner_right" {
		t.Fatalf("with one behind: %s, want inner_right", got)
	}
	g.world.SetBlock(0, 5, 1, 0)
	if got := stairShape(53<<4|3, nb); got != "straight" {
		t.Fatalf("alone: %s", got)
	}
}

func TestStairCollisionIsAnLShape(t *testing.T) {
	g := builder(t)
	boxes := g.res.collisionBoxes(53<<4, func(int32, int32, int32) uint16 { return 0 }) // facing east, bottom
	if len(boxes) != 2 {
		t.Fatalf("%d boxes for a stair: %v", len(boxes), boxes)
	}
	var low, high box6
	for _, b := range boxes {
		if b[4] == 0.5 {
			low = b
		} else {
			high = b
		}
	}
	if low != (box6{0, 0, 0, 1, 0.5, 1}) || high != (box6{0.5, 0.5, 0, 1, 1, 1}) {
		t.Fatalf("low %v high %v", low, high)
	}
	// And a player can walk up the step (0.5) but not over the top (1.0).
	g.creative = false
	g.world.SetBlock(0, 4, 3, 53<<4|1) // facing west: low side toward +x... high side at x 0..0.5
	g.feet = vec3{0.5, 4, 0.5}
}

func TestPlacingATorchLightsTheWorldAndRemeshesIt(t *testing.T) {
	g := builder(t)
	g.world.SetLightRules(g.res.light)
	g.setBlock(3, 4, 3, 50<<4|5)
	if got := g.world.Light(3, 4, 3) & 15; got != 14 {
		t.Fatalf("a torch lights its own cell with %d, want 14", got)
	}
	if got := g.world.Light(6, 4, 3) & 15; got != 11 {
		t.Fatalf("three blocks away %d, want 11", got)
	}
	if !g.dirty[chunkOf(3, 3)] {
		t.Fatal("the column was not marked for meshing")
	}
	g.setBlock(3, 4, 3, 0)
	if got := g.world.Light(6, 4, 3) & 15; got != 0 {
		t.Fatalf("after breaking the torch %d", got)
	}
	// Solid stone has no light, and a roof over open ground dims what is under it.
	g.setBlock(10, 6, 10, 1<<4)
	if got := g.world.Light(10, 5, 10) >> 4; got != 14 {
		t.Fatalf("under a roof of one block: sky %d, want 14", got)
	}
}

func chunkOf(x, z int32) world.ChunkPos { return world.ChunkPos{X: x >> 4, Z: z >> 4} }
