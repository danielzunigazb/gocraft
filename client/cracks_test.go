package client

import (
	"testing"
)

// Another player's mining arrives as Block Break Animation: a stage per breaker, replaced when they move
// to another block, ended by a stage out of 0..9, and forgotten after 400 ticks without news.
func TestBlockBreakAnimation(t *testing.T) {
	g, _, apply := mobGame(t)
	g.world.SetBlock(3, 4, 3, 1<<4)
	anim := func(eid int32, x, y, z int32, stage byte) {
		w := newPacket(0x25)
		w.VarInt(eid)
		w.Long(packPos(x, y, z))
		w.Byte(stage)
		apply(w)
	}
	anim(9, 3, 4, 3, 4)
	if d := g.damage[9]; d == nil || d.stage != 4 || d.pos != [3]int32{3, 4, 3} {
		t.Fatalf("damage %+v", g.damage[9])
	}
	if _, ok := testRes.entity[crackStrip]; !ok {
		t.Fatal("the stages were not made into a strip")
	}
	vs := g.buildCracks(nil)
	if len(vs) != 24 { // stone: six faces
		t.Fatalf("%d crack vertices, want 24", len(vs))
	}
	for _, v := range vs {
		if v.U < 0.4-1e-6 || v.U > 0.5+1e-6 {
			t.Fatalf("u %v outside stage 4's tenth of the strip", v.U)
		}
	}
	anim(9, 3, 4, 3, 255)
	if g.damage[9] != nil {
		t.Fatal("stage 255 did not end the cracks")
	}
	anim(9, 3, 4, 3, 2)
	for i := 0; i < crackStale+1; i++ {
		g.tick(input{})
	}
	if g.damage[9] != nil {
		t.Fatal("cracks nobody updated outlived 400 ticks")
	}
}

// The player's own mining cracks the block in ten stages as it goes, and the cracks go when it breaks
// or the button is let go.
func TestMiningShowsCracks(t *testing.T) {
	g := standing(t)
	g.world.SetBlock(0, 4, 2, 1<<4) // stone in front, at the feet
	g.pos = g.feet.add(vec3{0, eyeHeight, 0})
	g.yaw, g.pitch = 0, 0.6
	h, ok := g.target()
	if !ok || h.pos != [3]int32{0, 4, 2} {
		t.Skipf("the stone is not targeted from here: %v %v", h.pos, ok)
	}
	held := true
	g.keyHeld = func(string) bool { return held }
	stages := map[int]bool{}
	for i := 0; i < 400 && g.world.Block(0, 4, 2) != 0; i++ {
		g.mine()
		if d := g.damage[selfBreaker]; d != nil {
			stages[d.stage] = true
		}
	}
	if g.world.Block(0, 4, 2) != 0 {
		t.Fatal("the stone was not mined")
	}
	if len(stages) < 9 || g.damage[selfBreaker] != nil {
		t.Fatalf("stages seen %v, left %+v", stages, g.damage[selfBreaker])
	}
	g.world.SetBlock(0, 4, 2, 1<<4)
	for i := 0; i < 20; i++ {
		g.mine()
	}
	held = false
	g.mine()
	if g.damage[selfBreaker] != nil {
		t.Fatal("letting go left the cracks")
	}
}
