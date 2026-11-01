package client

import (
	"testing"

	"gocraft/proto"
	"gocraft/world"
)

func particlePacket(id int32, far bool, at, off vec3, speed float32, count int32, args ...int32) *proto.Writer {
	w := newPacket(0x2A)
	w.Int(id)
	w.Bool(far)
	for _, v := range []vec3{at, off} {
		w.Float(float32(v[0]))
		w.Float(float32(v[1]))
		w.Float(float32(v[2]))
	}
	w.Float(speed)
	w.Int(count)
	for _, a := range args {
		w.VarInt(a)
	}
	return w
}

func countKind(ps *particleSystem, k particleKind) int {
	n := 0
	for _, p := range ps.list {
		if p.kind == k {
			n++
		}
	}
	return n
}

// A count of 0 is one particle whose speed is the offsets times the speed (a red dust takes its colour from
// them); a count spreads that many. Farther than 16 blocks only the far-seen ones show.
func TestServerParticles(t *testing.T) {
	g, _, apply := mobGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	ps := g.particles()
	apply(particlePacket(ptRedDust, false, vec3{2, 5, 2}, vec3{0, 0, 1}, 1, 0)) // blue: red 0 is taken as 1, so magenta
	if countKind(ps, pReddust) != 1 {
		t.Fatalf("%d red dust", countKind(ps, pReddust))
	}
	p := ps.list[len(ps.list)-1]
	if p.rgb[1] != 0 || p.rgb[0] < 0.47 || p.rgb[2] < 0.47 {
		t.Fatalf("red dust colour %v, want red and blue, no green", p.rgb)
	}
	apply(particlePacket(ptCrit, false, vec3{2, 5, 2}, vec3{0.5, 0.5, 0.5}, 0.1, 12))
	if countKind(ps, pCrit) != 12 {
		t.Fatalf("%d crits, want 12", countKind(ps, pCrit))
	}
	apply(particlePacket(ptHeart, false, vec3{30, 5, 2}, vec3{}, 0, 3))
	if countKind(ps, pHeart) != 0 {
		t.Fatal("hearts 30 blocks away were made")
	}
	apply(particlePacket(ptHeart, true, vec3{30, 5, 2}, vec3{}, 0, 3))
	if countKind(ps, pHeart) != 3 {
		t.Fatalf("%d far-seen hearts, want 3", countKind(ps, pHeart))
	}
	apply(particlePacket(ptIconCrack, false, vec3{2, 5, 2}, vec3{}, 0, 2, 264, 0))   // pieces of a diamond
	apply(particlePacket(ptBlockCrack, false, vec3{2, 5, 2}, vec3{}, 0, 2, 1|1<<12)) // granite
	if countKind(ps, pItemCrack) != 2 || countKind(ps, pDig) != 2 {
		t.Fatalf("%d item pieces and %d block pieces, want 2 and 2", countKind(ps, pItemCrack), countKind(ps, pDig))
	}
	if ps.list[len(ps.list)-1].tile < 0 {
		t.Fatal("no tile for granite")
	}
}

// Every type a server may send is made and gone again within a few seconds, without trouble.
func TestEveryServerParticleLivesAndDies(t *testing.T) {
	g, _, _ := mobGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	ps := g.particles()
	for id := int32(0); id <= 41; id++ {
		args := make([]int32, particleArgs(id))
		if len(args) > 0 {
			args[0] = 1
		}
		g.netParticles(id, false, vec3{1.5, 6, 1.5}, vec3{0.3, 0.3, 0.3}, 0.05, 5, args)
	}
	if len(ps.list) == 0 {
		t.Fatal("no particles")
	}
	right, up := vec3{1, 0, 0}, vec3{0, 1, 0}
	for i := 0; i < 200; i++ {
		g.tickParticles()
		ps.build(g.res, right, up, 0.5, func(vec3) uint8 { return 0xF0 })
	}
	if len(ps.list) != 0 || len(ps.huge) != 0 {
		t.Fatalf("%d particles and %d emitters left after ten seconds", len(ps.list), len(ps.huge))
	}
}

// Explosion: the blocks in its records (relative to the truncated middle) go without any pieces, the
// player is pushed, and a strong one throws big puffs for eight ticks.
func TestServerExplosion(t *testing.T) {
	g, _, apply := mobGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.world.SetBlock(5, 4, 5, 1<<4)
	g.world.SetBlock(6, 4, 5, 1<<4)
	w := newPacket(0x27)
	w.Float(5.9)
	w.Float(4.5)
	w.Float(5.2)
	w.Float(4)
	w.Int(2)
	w.Bytes([]byte{0, 0, 0, 1, 0, 0})
	w.Float(0.5)
	w.Float(0.25)
	w.Float(0)
	apply(w)
	if g.world.Block(5, 4, 5) != 0 || g.world.Block(6, 4, 5) != 0 {
		t.Fatal("the explosion left its blocks")
	}
	if g.vel[0] != 0.5 || g.vel[1] != 0.25 {
		t.Fatalf("pushed %v", g.vel)
	}
	ps := g.particles()
	if countKind(ps, pDig) != 0 {
		t.Fatalf("%d block pieces: an explosion shows puffs, not pieces", countKind(ps, pDig))
	}
	if len(ps.huge) != 1 || countKind(ps, pExplode) != 2 || countKind(ps, pSmoke) != 2 {
		t.Fatalf("%d emitters, %d puffs, %d smoke", len(ps.huge), countKind(ps, pExplode), countKind(ps, pSmoke))
	}
	for i := 0; i < 8; i++ {
		g.tickHuge()
	}
	if len(ps.huge) != 0 || countKind(ps, pLargeExplode) != 48 {
		t.Fatalf("after eight ticks: %d emitters, %d big puffs (want 0 and 48)", len(ps.huge), countKind(ps, pLargeExplode))
	}
}

// Another player's broken block arrives as Effect 2001 with its pieces; the block change itself shows none.
func TestBlockChangesAreQuietAndEffect2001Shows(t *testing.T) {
	g, _, apply := mobGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.world.SetBlock(2, 4, 2, 1<<4)
	bc := newPacket(0x23)
	bc.Long(packPos(2, 4, 2))
	bc.VarInt(0)
	apply(bc)
	ps := g.particles()
	if g.world.Block(2, 4, 2) != 0 || len(ps.list) != 0 {
		t.Fatalf("block %d, %d particles: a server's block change should be silent", g.world.Block(2, 4, 2), len(ps.list))
	}
	fx := newPacket(0x28)
	fx.Int(2001)
	fx.Long(packPos(2, 4, 2))
	fx.Int(1)
	fx.Bool(false)
	apply(fx)
	if countKind(ps, pDig) != 64 {
		t.Fatalf("%d pieces from Effect 2001, want 64", countKind(ps, pDig))
	}
}

// Animation 4 and 5 put three ticks of sparks around a mob; the enchanted kind is blue.
func TestCritAnimationSparks(t *testing.T) {
	g, _, apply := mobGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	apply(spawnMobPacket(60, 54, 2.5, 4, 2.5, 0, 0, nil))
	w := newPacket(0x0B)
	w.VarInt(60)
	w.Byte(5)
	apply(w)
	ps := g.particles()
	if len(ps.emitters) != 1 || !ps.emitters[0].magic {
		t.Fatalf("emitters %+v", ps.emitters)
	}
	for i := 0; i < 3; i++ {
		g.tickParticles()
	}
	if len(ps.emitters) != 0 || countKind(ps, pCrit) == 0 {
		t.Fatalf("%d emitters, %d sparks", len(ps.emitters), countKind(ps, pCrit))
	}
	for _, p := range ps.list {
		if p.kind == pCrit && (p.tile != 66 || p.rgb[0] >= p.rgb[2]) {
			t.Fatalf("a magic spark: tile %d colour %v", p.tile, p.rgb)
		}
	}
}

// A chest a server says is open has its lid lifted until it says it closed; a note block puffs a note.
func TestBlockActions(t *testing.T) {
	g, _, apply := mobGame(t)
	g.feet = vec3{0.5, 4, 0.5}
	g.world.SetBlock(2, 4, 2, idChest<<4|2)
	g.meshes[world.ChunkPos{X: 0, Z: 0}] = &chunkMesh{} // the column is on screen
	act := func(x, y, z int32, id, param byte, blk int32) {
		w := newPacket(0x24)
		w.Long(packPos(x, y, z))
		w.Byte(id)
		w.Byte(param)
		w.VarInt(blk)
		apply(w)
	}
	act(2, 4, 2, 1, 2, idChest)
	if be := g.beAt([3]int32{2, 4, 2}, false); be == nil || be.viewers != 2 {
		t.Fatalf("chest %+v", be)
	}
	groups := map[string][]vertex{}
	g.camPos = vec3{2, 6, -2}
	for i := 0; i < 40; i++ {
		g.emitChests(groups)
	}
	if be := g.beAt([3]int32{2, 4, 2}, false); be.lid < 0.99 {
		t.Fatalf("the lid is at %v after many frames, want open", be.lid)
	}
	act(2, 4, 2, 1, 0, idChest)
	for i := 0; i < 60; i++ {
		g.emitChests(groups)
	}
	if be := g.beAt([3]int32{2, 4, 2}, false); be.lid > 0.01 {
		t.Fatalf("the lid is at %v after it closed", be.lid)
	}
	act(3, 4, 2, 0, 12, 25)
	if countKind(g.particles(), pNote) != 1 {
		t.Fatalf("%d notes", countKind(g.particles(), pNote))
	}
	act(3, 4, 2, 9, 12, 25) // an instrument that does not exist: nothing
	act(5, 4, 5, 0, 1, 33)  // a piston: nothing to do here
}
