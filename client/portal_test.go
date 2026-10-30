package client

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"gocraft/anvil"
	"gocraft/world"
)

// buildFrame puts an obsidian frame around an interior of 2 wide and 3 tall whose lowest block
// is at x, y, z. The corners are left out, as vanilla does not need them.
func buildFrame(g *game, x, y, z int32, axisX bool) {
	set := func(a, b, c int32) { g.setBlock(a, b, c, idObsidian<<4) }
	for i := int32(0); i < 2; i++ {
		if axisX {
			set(x+i, y-1, z)
			set(x+i, y+3, z)
		} else {
			set(x, y-1, z+i)
			set(x, y+3, z+i)
		}
	}
	for j := int32(0); j < 3; j++ {
		if axisX {
			set(x-1, y+j, z)
			set(x+2, y+j, z)
		} else {
			set(x, y+j, z-1)
			set(x, y+j, z+2)
		}
	}
}

func countPortal(g *game, x0, y0, z0, x1, y1, z1 int32) (n int) {
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			for z := z0; z <= z1; z++ {
				if g.blockAt(x, y, z)>>4 == idPortal {
					n++
				}
			}
		}
	}
	return n
}

// savedGame is a game whose world is a save with a folder of its own, under the test's temp dirs.
func savedGame(t *testing.T) *game {
	g := testGame(t)
	g.save = anvil.Virtual(filepath.Join(t.TempDir(), "w"), anvil.Level{Name: "w", Seed: 4242, GameType: 1})
	g.creative = true
	return g
}

// Lighting a frame fills it with portal blocks of the right axis; a frame with a side missing
// does not light.
func TestFrameLights(t *testing.T) {
	for _, axisX := range []bool{true, false} {
		g := testGame(t)
		buildFrame(g, 10, 4, 10, axisX)
		if !g.tryPortal(vec3i{10, 5, 10}) {
			t.Fatalf("axis x=%v: the frame did not light", axisX)
		}
		if n := countPortal(g, 8, 3, 8, 13, 8, 13); n != 6 {
			t.Fatalf("axis x=%v: %d portal blocks, want 2 x 3", axisX, n)
		}
		want := uint16(2)
		if axisX {
			want = 1
		}
		if m := g.blockAt(10, 5, 10) & 15; m != want {
			t.Fatalf("axis x=%v: portal meta %d, want %d", axisX, m, want)
		}
		// Lit again, a full frame is not lit twice.
		if g.tryPortal(vec3i{10, 5, 10}) {
			t.Fatal("a frame that already has a portal lit again")
		}
	}
	g := testGame(t)
	buildFrame(g, 10, 4, 10, true)
	g.setBlock(12, 5, 10, 0) // a side missing
	if g.tryPortal(vec3i{10, 5, 10}) {
		t.Fatal("a frame with a gap lit")
	}
	small := testGame(t) // 2 x 2 is too short: a frame needs at least 3 inside
	for _, p := range [][3]int32{{10, 3, 10}, {11, 3, 10}, {10, 6, 10}, {11, 6, 10}, {9, 4, 10}, {9, 5, 10}, {12, 4, 10}, {12, 5, 10}} {
		small.setBlock(p[0], p[1], p[2], idObsidian<<4)
	}
	if small.tryPortal(vec3i{10, 4, 10}) {
		t.Fatal("a 2 x 2 frame lit")
	}
}

// Breaking the frame puts the portal out; breaking an inside block does too.
func TestBreakingTheFramePutsThePortalOut(t *testing.T) {
	g := testGame(t)
	buildFrame(g, 10, 4, 10, true)
	g.tryPortal(vec3i{10, 5, 10})
	g.setBlock(9+1, 3, 10, 0) // the bottom of the frame
	if n := countPortal(g, 8, 3, 8, 13, 8, 13); n != 0 {
		t.Fatalf("%d portal blocks left after the frame broke", n)
	}
}

// Flint and steel clicked on the inside floor lights the frame, as BlockFire.onBlockAdded does.
func TestFlintAndSteelLightsAPortal(t *testing.T) {
	g := testGame(t)
	buildFrame(g, 20, 4, 20, false)
	g.inv[0], g.sel = slot{ID: 259, N: 1}, 0
	if !g.useFlintAndSteel(rayHit{pos: [3]int32{20, 3, 20}, side: dUp}) {
		t.Fatal("the flint and steel was not used")
	}
	if n := countPortal(g, 18, 3, 18, 23, 8, 23); n != 6 {
		t.Fatalf("%d portal blocks, want 6", n)
	}
	// Away from a frame it makes a flame, which goes out on its own on a block that does not burn for ever.
	g.inv[0] = slot{ID: 259, N: 1}
	g.useFlintAndSteel(rayHit{pos: [3]int32{40, 3, 40}, side: dUp})
	if g.blockAt(40, 4, 40)>>4 != idFire {
		t.Fatal("no flame")
	}
	for i := 0; i < 130; i++ {
		g.tickFires()
	}
	if g.blockAt(40, 4, 40) != 0 {
		t.Fatal("the flame did not go out")
	}
}

// Through a portal in the Overworld, the Nether is at one eighth the distance, a portal is made
// there, and the way back leads to the portal the player came from.
func TestTravelScaleAndReturn(t *testing.T) {
	g := savedGame(t)
	buildFrame(g, 80, 4, 160, true)
	g.tryPortal(vec3i{80, 5, 160})
	g.feet = vec3{80.5, 4, 160.5}
	g.prevFeet = g.feet
	g.travelPortal()
	if g.dimn.id != dimNether {
		t.Fatalf("dimension %d after the portal", g.dimn.id)
	}
	// 80/8 = 10 and 160/8 = 20, within the 16 blocks the portal maker looks.
	if math.Abs(float64(g.feet[0])-10) > 17 || math.Abs(float64(g.feet[2])-20) > 17 {
		t.Fatalf("arrived at %v, want near 10, 20", g.feet)
	}
	if countPortal(g, int32(g.feet[0])-6, int32(g.feet[1])-2, int32(g.feet[2])-6, int32(g.feet[0])+6, int32(g.feet[1])+6, int32(g.feet[2])+6) < 6 {
		t.Fatal("no portal at the arrival")
	}
	// Back: eight times the distance, and the Overworld portal is the nearest one.
	g.dimn.cooldown = 0
	g.travelPortal()
	if g.dimn.id != dimOverworld {
		t.Fatalf("dimension %d after the way back", g.dimn.id)
	}
	if math.Abs(float64(g.feet[0])-80.5) > 1 || math.Abs(float64(g.feet[2])-160.5) > 1.01 || math.Abs(float64(g.feet[1])-4) > 1 {
		t.Fatalf("came back to %v, want the portal at 80.5, 4, 160.5", g.feet)
	}
	if n := countPortal(g, 78, 3, 158, 83, 8, 163); n != 6 {
		t.Fatalf("the first portal has %d blocks after the round trip", n)
	}
}

// Standing in a portal: survival waits four seconds, creative almost none; stepping out lets the
// count run down; just after arriving the portal does not take the player again.
func TestPortalTimer(t *testing.T) {
	g := savedGame(t)
	buildFrame(g, 10, 4, 10, true)
	g.tryPortal(vec3i{10, 5, 10})
	g.inGame = true
	g.creative = false
	g.feet = vec3{10.5, 4, 10.5}
	for i := 0; i < portalWait; i++ {
		g.tickPortal()
		if g.dimn.id != dimOverworld {
			t.Fatalf("travelled after only %d ticks in survival", i)
		}
	}
	for i := 0; i < 5 && g.dimn.id == dimOverworld; i++ {
		g.tickPortal()
	}
	if g.dimn.id != dimNether {
		t.Fatal("survival never travelled")
	}
	// Creative: a couple of ticks.
	g2 := savedGame(t)
	buildFrame(g2, 10, 4, 10, true)
	g2.tryPortal(vec3i{10, 5, 10})
	g2.inGame = true
	g2.feet = vec3{10.5, 4, 10.5}
	for i := 0; i < 3 && g2.dimn.id == dimOverworld; i++ {
		g2.tickPortal()
	}
	if g2.dimn.id != dimNether {
		t.Fatal("creative did not travel at once")
	}
	// Still in the portal at the other end: the cooldown keeps it from going straight back.
	for i := 0; i < 30; i++ {
		g2.tickPortal()
	}
	if g2.dimn.id != dimNether {
		t.Fatal("went straight back through the portal it arrived in")
	}
}

// The End: a platform of obsidian under the player at 100, 48, 0, and out through the world's spawn.
func TestEndPlatform(t *testing.T) {
	g := savedGame(t)
	g.spawn = vec3{0.5, 4, 0.5}
	g.enterDimension(dimEnd, vec3{100.5, 50, 0.5})
	if g.dimn.id != dimEnd {
		t.Fatal("not in the End")
	}
	for x := int32(98); x <= 102; x++ {
		for z := int32(-2); z <= 2; z++ {
			if g.blockAt(x, 48, z)>>4 != idObsidian {
				t.Fatalf("no obsidian at %d, 48, %d", x, z)
			}
			if g.blockAt(x, 49, z) != 0 || g.blockAt(x, 50, z) != 0 {
				t.Fatalf("no room to stand at %d, %d", x, z)
			}
		}
	}
	if g.feet[0] != 100.5 || g.feet[1] != 49 {
		t.Fatalf("the player is at %v", g.feet)
	}
	g.fellOut() // the void: home to the Overworld's spawn
	if g.dimn.id != dimOverworld {
		t.Fatal("the void did not send the player home")
	}
}

// The Nether and the End keep their own edits, and what is saved goes under DIM-1 and DIM1 of the
// overlay, never into the Overworld's own columns or the save itself.
func TestDimensionsKeepTheirOwnEdits(t *testing.T) {
	g := savedGame(t)
	g.inGame = true
	root := g.save.Dir
	g.switchWorld(dimNether)
	g.setBlock(5, 60, 5, 1<<4) // a block of stone in the Nether
	g.switchWorld(dimOverworld)
	if g.blockAt(5, 60, 5) != 0 {
		t.Fatal("the Nether's block showed in the Overworld")
	}
	g.switchWorld(dimNether)
	if g.blockAt(5, 60, 5)>>4 != 1 {
		t.Fatal("the Nether forgot its edit while the player was away")
	}
	g.saveWorld()
	base := overlayFor(g.save.Dir).dir
	if _, err := os.Stat(filepath.Join(base, "DIM-1", "c.0.0.bin")); err != nil {
		t.Fatalf("the Nether's column was not saved in DIM-1: %v", err)
	}
	if ents, _ := os.ReadDir(base); func() bool {
		for _, e := range ents {
			if e.Name() == "c.0.0.bin" {
				return true
			}
		}
		return false
	}() {
		t.Fatal("a Nether column was saved among the Overworld's")
	}
	if _, err := os.Stat(root); err == nil {
		if ents, _ := os.ReadDir(root); len(ents) != 0 {
			t.Fatalf("the save folder itself was written to: %v", ents)
		}
	}
	// The player's dimension is remembered.
	if p, ok := overlayFor(g.save.Dir).loadPlayer(); !ok || p.Dim != dimNether {
		t.Fatalf("saved player %+v", p)
	}
	// Opening again puts them back in the Nether with the edit.
	h := testGame(t)
	h.save = g.save
	h.world = world.New()
	h.restoreDim(dimNether)
	if h.dimn.id != dimNether || h.blockAt(5, 60, 5)>>4 != 1 {
		t.Fatal("the Nether did not come back with its edit")
	}
}

// What a dimension looks like: the Nether's fog, thick and brown-red, its light never dark, and the
// End's fixed ambient light.
func TestDimensionLook(t *testing.T) {
	g := testGame(t)
	g.dimn.id = dimNether
	if f := g.dimFogColour(0.3); f != [3]float32{0.2, 0.03, 0.03} {
		t.Fatalf("Nether fog %v", f)
	}
	if s, e := g.dimFogRange(160); s != 8 || e != 80 {
		t.Fatalf("Nether fog range %v..%v, want 8..80", s, e)
	}
	var nether, over [768]uint8
	g.buildLightmapDim(0.2, &nether)
	g.dimn.id = dimOverworld
	g.buildLightmapDim(0.2, &over)
	if nether[0] <= over[0] { // sky 0, block 0: the Nether has a floor of 0.1
		t.Fatalf("the Nether's darkest %d is not lighter than the Overworld's %d", nether[0], over[0])
	}
	g.dimn.id = dimEnd
	var end [768]uint8
	g.buildLightmapDim(1, &end)
	if end[0] < 50 || end[0] != end[(15*16)*3] { // sky level makes no difference in the End
		t.Fatalf("End light %d / %d", end[0], end[(15*16)*3])
	}
}

// With no portal waiting at the other end, the way back makes one, within 16 blocks of where eight
// times the distance leads.
func TestReturnMakesAPortalWhenNoneIsThere(t *testing.T) {
	g := savedGame(t)
	buildFrame(g, 80, 4, 160, true)
	g.tryPortal(vec3i{80, 5, 160})
	g.feet = vec3{80.5, 4, 160.5}
	g.travelPortal()
	if g.dimn.id != dimNether {
		t.Fatal("not in the Nether")
	}
	over := g.dimn.others[dimOverworld] // put the Overworld's portal out while the player is away
	for x := int32(78); x <= 83; x++ {
		for y := int32(3); y <= 8; y++ {
			for z := int32(158); z <= 163; z++ {
				if over.Block(x, y, z)>>4 == idPortal {
					over.SetBlock(x, y, z, 0)
				}
			}
		}
	}
	g.dimn.cooldown = 0
	nx, nz := float64(g.feet[0])*8, float64(g.feet[2])*8 // where eight times the distance leads from the Nether portal
	g.travelPortal()
	if g.dimn.id != dimOverworld {
		t.Fatal("not back in the Overworld")
	}
	dx, dz := math.Abs(float64(g.feet[0])-nx), math.Abs(float64(g.feet[2])-nz)
	if dx > 17 || dz > 17 {
		t.Fatalf("came back to %v, want within 16 of %.0f, %.0f", g.feet, nx, nz)
	}
	x, y, z := int32(g.feet[0]), int32(g.feet[1]), int32(g.feet[2])
	if countPortal(g, x-3, y-2, z-3, x+3, y+5, z+3) < 6 {
		t.Fatal("no portal was made at the Overworld end")
	}
}
