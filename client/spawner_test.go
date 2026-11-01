package client

import (
	"math"
	"testing"

	"gocraft/anvil"
	"gocraft/proto"
)

func spawnerPacket(t *testing.T, x, y, z int32, mob string) *proto.Writer {
	return blockEntityPacket(t, x, y, z, 1, anvil.Compound{"EntityId": mob, "Delay": int16(20)})
}

// A spawner's block entity (action 1) names the mob in the cage; the block going away takes it with it.
func TestSpawnerMobIsRecorded(t *testing.T) {
	g, _, apply := mobGame(t)
	g.setBlock(10, 5, 20, idSpawner<<4)
	apply(spawnerPacket(t, 10, 5, 20, "Zombie"))
	s := g.spawners[[3]int32{10, 5, 20}]
	if s == nil || s.mob != "Zombie" || s.delay != 20 {
		t.Fatalf("spawner %+v", s)
	}
	apply(spawnerPacket(t, 10, 5, 20, "Skeleton")) // the server says another mob: the same spawner, a new mob
	if len(g.spawners) != 1 || s.mob != "Skeleton" {
		t.Fatalf("spawner %+v among %d", s, len(g.spawners))
	}
	g.setBlock(10, 5, 20, 0)
	if len(g.spawners) != 0 {
		t.Fatal("a spawner outlived its block")
	}
}

// On the client the logic only turns the mob: 1000 / (delay + 200) degrees a tick while a player is within 16
// blocks, the delay counting down from 20, and a block event of 1 puts it back to 200.
func TestSpawnerTurnsTheMobWhenPlayerIsNear(t *testing.T) {
	g, _, apply := mobGame(t)
	g.setBlock(10, 5, 20, idSpawner<<4)
	apply(spawnerPacket(t, 10, 5, 20, "Zombie"))
	s := g.spawners[[3]int32{10, 5, 20}]
	g.pos = vec3{10.5, 6, 25} // 5 blocks away
	want, delay := float32(0), 20
	for i := 0; i < 30; i++ {
		g.tickSpawners()
		if delay > 0 {
			delay--
		}
		want = float32(math.Mod(float64(want+1000/(float32(delay)+200)), 360))
		if s.rot != want || s.delay != delay {
			t.Fatalf("tick %d: rot %v delay %d, want %v and %d", i+1, s.rot, s.delay, want, delay)
		}
	}
	if s.delay != 0 || s.rot < 100 || s.rot > 160 { // 20 ticks from ~4.55 degrees up to 5, then 5 a tick
		t.Fatalf("after 30 ticks: rot %v delay %d", s.rot, s.delay)
	}
	g.spawnerEvent([3]int32{10, 5, 20})
	if s.delay != 200 {
		t.Fatalf("a block event left the delay at %d", s.delay)
	}
	before := s.rot
	g.pos = vec3{100, 6, 25} // far away: not activated
	g.tickSpawners()
	if s.rot != before || s.delay != 200 {
		t.Fatal("a spawner out of range kept turning")
	}
}

// The mob is the one in the cage at 0.4375 of its size, turned ten times as far as the logic's rotation.
func TestSpawnerDrawsItsMob(t *testing.T) {
	g, _, apply := mobGame(t)
	g.camPos = vec3{10, 6, 18}
	g.partial = 1
	g.setBlock(10, 5, 20, idSpawner<<4)
	apply(spawnerPacket(t, 10, 5, 20, "Zombie"))
	groups := map[string][]vertex{}
	g.emitSpawners(groups)
	vs := groups["zombie/zombie"]
	if len(vs) != 6*24 {
		t.Fatalf("%d vertices for the zombie, want %d", len(vs), 6*24)
	}
	lo, hi := standBounds(vs)
	// a zombie is 1.95 tall (2.0078 of model): at 0.4375 it stands about 0.88 high on the cage's floor
	if h := float64(hi[1] - lo[1]); !near(h, 2.0*0.4375, 0.03) {
		t.Fatalf("the mob is %v high, want about %v", h, 2.0*0.4375)
	}
	if mid := float64(lo[0]+hi[0]) / 2; !near(mid, 10.5, 0.2) {
		t.Fatalf("the mob is at x %v, not in the cage", mid)
	}
	if !near(float64(lo[1]), 5+0.4-0.4*math.Cos(30*math.Pi/180), 0.02) {
		t.Fatalf("the mob's feet are at %v", lo[1])
	}
	// turned: the same spawner a tick later has its mob at another angle, so the vertices moved
	g.pos = vec3{10.5, 6, 25}
	g.tickSpawners()
	again := map[string][]vertex{}
	g.emitSpawners(again)
	moved := false
	for i, v := range again["zombie/zombie"] {
		if v != vs[i] {
			moved = true
		}
	}
	if !moved {
		t.Fatal("the mob did not turn")
	}
	// a name the game has no model for draws nothing, and so does a block that is no spawner
	apply(spawnerPacket(t, 10, 5, 20, "Nonexistent"))
	empty := map[string][]vertex{}
	g.emitSpawners(empty)
	if len(empty) != 0 {
		t.Fatalf("a mob with no model drew %d groups", len(empty))
	}
}

// The renderer turns the mob ten times as far as the logic's rotation, interpolated between ticks.
func TestSpawnerMobIsTurnedTenTimesTheRotation(t *testing.T) {
	g, _, apply := mobGame(t)
	g.camPos = vec3{10, 6, 18}
	g.setBlock(10, 5, 20, idSpawner<<4)
	apply(spawnerPacket(t, 10, 5, 20, "Zombie"))
	s := g.spawners[[3]int32{10, 5, 20}]
	s.prevRot, s.rot = 2, 6
	g.partial = 0.5 // (2 + (6 - 2) * 0.5) * 10 = 40 degrees
	got := map[string][]vertex{}
	g.emitSpawners(got)
	feet := vec3{10.5, 5 + 0.4 - 0.4*math.Cos(30*math.Pi/180), 20.5}
	want := map[string][]vertex{}
	g.emitMob(want, &mob{kind: kinds["Zombie"], pos: feet, prev: feet, yaw: 40, prevYaw: 40, health: 20, onGround: true, size: 1, drawScale: 0.4375}, 0.5)
	a, b := got["zombie/zombie"], want["zombie/zombie"]
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("%d and %d vertices", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("vertex %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}
