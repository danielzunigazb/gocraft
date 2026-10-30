package client

import "testing"

// universocraft's client check drops the player onto snow layers on a platform and wants vanilla's
// numbers back: BlockSnow's box is (layers-1)/8 high, so three layers (meta 2) stop the feet at y=4.25,
// and a fall from y=9 reaches 8.9216 on the second tick (captured from a vanilla 1.8.9 client).
func TestLandsOnSnowLayers(t *testing.T) {
	g := testGame(t)
	for x := int32(-2); x <= 2; x++ {
		for z := int32(-2); z <= 2; z++ {
			g.world.SetBlock(x, 4, z, 78<<4|2)
		}
	}
	g.feet = vec3{0.5, 13, 0.5}
	g.prevFeet = g.feet
	run(g, 60, input{})
	if !g.onGround || !near(g.feet[1], 4.25, 1e-9) {
		t.Fatalf("feet y %v onGround %v, want standing on three snow layers at y=4.25", g.feet[1], g.onGround)
	}
}

func TestFirstFallTickMatchesVanilla(t *testing.T) {
	g := testGame(t)
	g.feet = vec3{8, 9, 8}
	g.prevFeet = g.feet
	g.onGround = false
	g.tick(input{}) // the move uses motionY=0; gravity is added after it
	if !near(g.feet[1], 9, 1e-9) {
		t.Fatalf("after one tick feet y %.10f, want 9", g.feet[1])
	}
	g.tick(input{})
	if !near(g.feet[1], 8.9216, 1e-4) {
		t.Fatalf("after two ticks feet y %.10f, want 8.9216", g.feet[1])
	}
}

// One layer has no box at all; two are 1/8 high.
func TestSnowLayerHeights(t *testing.T) {
	g := testGame(t)
	if b := g.res.collisionBoxes(78<<4|0, nil); len(b) != 0 {
		t.Fatalf("one layer collides: %v", b)
	}
	if b := g.res.collisionBoxes(78<<4|1, nil); len(b) != 1 || b[0][4] != 0.125 {
		t.Fatalf("two layers: %v", b)
	}
}
