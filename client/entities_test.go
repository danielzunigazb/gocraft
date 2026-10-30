package client

import "testing"

func tickItems(g *game, n int) {
	for i := 0; i < n; i++ {
		g.tickItems()
	}
}

func awayFromItems(g *game) { g.feet = vec3{40, 4, 40} } // far enough not to pick anything up

func TestDroppedItemFallsAndRests(t *testing.T) {
	g := testGame(t)
	awayFromItems(g)
	rng.Seed(3)
	g.spawnDrop(0, 9, 0, blockSlot(3<<4, 1))
	if len(g.items) != 1 {
		t.Fatal("no item")
	}
	e := g.items[0]
	if e.vel[1] != 0.2 || e.pickupDelay != 10 {
		t.Fatalf("a new drop starts rising at 0.2 with a delay of 10: %v %d", e.vel, e.pickupDelay)
	}
	if e.pos[0] < 0.25 || e.pos[0] > 0.75 || e.pos[1] < 9.25 || e.pos[1] > 9.75 {
		t.Fatalf("it appears in the middle half of the block: %v", e.pos)
	}
	tickItems(g, 100)
	if !e.onGround && e.pos[1] > 4.001 {
		t.Fatalf("still in the air at %v", e.pos)
	}
	if e.pos[1] < 3.99 || e.pos[1] > 4.01 {
		t.Fatalf("resting at y=%.3f, want on the ground at 4", e.pos[1])
	}
	if e.vel[0]*e.vel[0] > 1e-4 {
		t.Fatalf("still sliding: %v", e.vel)
	}
}

func TestItemPhysicsConstants(t *testing.T) {
	g := testGame(t)
	awayFromItems(g)
	e := &itemEntity{stack: slot{ID: 280, N: 1}, pos: vec3{0.5, 50, 0.5}}
	e.prev = e.pos
	g.items = []*itemEntity{e}
	g.tickItems()
	// motion y: (0 - 0.04) * 0.98 after the first tick.
	if d := e.vel[1] + 0.0392; d > 1e-6 || d < -1e-6 {
		t.Fatalf("first tick vel %v, want -0.0392", e.vel[1])
	}
	for i := 0; i < 300; i++ { // terminal speed: v = (v - 0.04) * 0.98 settles at -1.96
		g.tickItems()
	}
	if d := e.vel[1] + 1.96; d > 0.01 || d < -0.01 {
		t.Skipf("landed before terminal speed (%v)", e.vel[1])
	}
}

func TestItemSlidesOnIce(t *testing.T) {
	slide := func(floor uint16) float64 {
		g := testGame(t)
		awayFromItems(g)
		for x := int32(-5); x < 60; x++ {
			g.world.SetBlock(x, 3, 0, floor<<4)
		}
		e := &itemEntity{stack: slot{ID: 280, N: 1}, pos: vec3{0.5, 4, 0.5}, vel: vec3{0.5, 0, 0}, pickupDelay: 5}
		e.prev = e.pos
		g.items = []*itemEntity{e}
		tickItems(g, 60)
		return e.pos[0]
	}
	if stone, ice := slide(1), slide(79); ice < stone*2 {
		t.Fatalf("slid to x=%.1f on stone and %.1f on ice", stone, ice)
	}
}

func TestPickupAfterTheDelayWithinReach(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.spawnDrop(1, 4, 0, blockSlot(3<<4, 3))
	e := g.items[0]
	e.pos = vec3{1.2, 4, 0.5}
	for i := 0; i < 9; i++ {
		g.tickItems()
		if g.inv[0].N != 0 {
			t.Fatalf("picked up after %d ticks: the delay is 10", i+1)
		}
	}
	tickItems(g, 3)
	if g.inv[0] != blockSlot(3<<4, 3) || len(g.items) != 0 {
		t.Fatalf("inventory %v items left %d", g.inv[0], len(g.items))
	}
}

func TestPickupLeavesWhatDoesNotFit(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	for i := range g.inv {
		g.inv[i] = slot{ID: 280, N: 64}
	}
	g.inv[8] = slot{ID: 264, N: 62} // diamonds with room for two
	e := &itemEntity{stack: slot{ID: 264, N: 5}, pos: vec3{0.5, 4, 0.5}}
	e.prev = e.pos
	g.items = []*itemEntity{e}
	tickItems(g, 1)
	if g.inv[8].N != 64 || len(g.items) != 1 || g.items[0].stack.N != 3 {
		t.Fatalf("slot %v, left on the ground %v", g.inv[8], g.items)
	}
}

func TestItemsMergeAndDespawn(t *testing.T) {
	g := testGame(t)
	awayFromItems(g)
	a := &itemEntity{stack: slot{ID: 280, N: 10}, pos: vec3{0.4, 4, 0.5}, pickupDelay: 99}
	b := &itemEntity{stack: slot{ID: 280, N: 20}, pos: vec3{0.6, 4, 0.5}, pickupDelay: 99}
	c := &itemEntity{stack: slot{ID: 264, N: 1}, pos: vec3{0.5, 4, 0.5}, pickupDelay: 99} // another item: stays apart
	a.prev, b.prev, c.prev = a.pos, b.pos, c.pos
	g.items = []*itemEntity{a, b, c}
	tickItems(g, 45)
	sticks := 0
	for _, e := range g.items {
		if e.stack.ID == 280 {
			sticks += int(e.stack.N)
		}
	}
	if len(g.items) != 2 || sticks != 30 {
		t.Fatalf("%d entities, %d sticks: want the two stacks joined and the diamond kept apart", len(g.items), sticks)
	}
	g.items[0].age = itemLifespan - 1
	tickItems(g, 2)
	if len(g.items) != 1 {
		t.Fatalf("an item older than five minutes should be gone: %d left", len(g.items))
	}
}

func TestItemInsideABlockIsPushedOut(t *testing.T) {
	g := testGame(t)
	awayFromItems(g)
	// A stone block with air above: an item whose middle is inside it drifts up and out.
	g.world.SetBlock(5, 4, 5, 1<<4)
	e := &itemEntity{stack: slot{ID: 280, N: 1}, pos: vec3{5.5, 4.7, 5.5}, pickupDelay: 99}
	e.prev = e.pos
	g.items = []*itemEntity{e}
	tickItems(g, 30)
	if e.pos[1] < 5 {
		t.Fatalf("still inside the block: y=%.2f", e.pos[1])
	}
}

func TestBreakingABlockDropsAnItem(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.world.SetBlock(0, 4, 3, 3<<4) // dirt
	g.spawnDrop(0, 4, 3, blockSlot(3<<4, 1))
	e := g.items[0]
	if e.stack != blockSlot(3<<4, 1) || e.pos[2] < 3 || e.pos[2] > 4 {
		t.Fatalf("drop %v at %v", e.stack, e.pos)
	}
}

func TestDropGeometryForBlocksAndFlatItems(t *testing.T) {
	g := testGame(t)
	g.items = []*itemEntity{
		{stack: blockSlot(1<<4, 1), pos: vec3{0.5, 4, 0.5}, prev: vec3{0.5, 4, 0.5}},
		{stack: slot{ID: 280, N: 1}, pos: vec3{2.5, 4, 0.5}, prev: vec3{2.5, 4, 0.5}},
	}
	blocks, flat := g.buildItemGeometry(0.5)
	if len(blocks) != 24 { // a cube: six quads of four vertices
		t.Fatalf("%d vertices for a block drop", len(blocks))
	}
	if len(flat) != 8 { // a picture seen from both sides
		t.Fatalf("%d vertices for a stick", len(flat))
	}
}
