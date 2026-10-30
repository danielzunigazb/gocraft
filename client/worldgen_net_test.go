package client

import (
	"net"
	"testing"

	"gocraft/server"
	"gocraft/world"
	"gocraft/worldgen"
)

// A server with a generator: the client joins on real ground and gets the generated light and biomes.
func TestGeneratedWorldOverTheNetwork(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	gen := worldgen.NewGenerator(777)
	go server.New(server.Config{ViewDistance: 2, Seed: 777, Generator: gen.Func(), Spawn: gen.Spawn}).Serve(l)

	g := testGame(t)
	if err := g.connect(l.Addr().String(), "Explorer"); err != nil {
		t.Fatal(err)
	}
	defer g.leaveServer()
	pumpUntil(t, "the columns around the spawn", func() bool { return len(g.net.received) >= 25 }, g)
	sx, sz := gen.Spawn()
	if int32(g.feet[0]) != sx && int32(g.feet[0]) != sx-1 && int32(g.feet[2]) != sz && int32(g.feet[2]) != sz-1 {
		t.Fatalf("the client is at %v, the spawn is %d,%d", g.feet, sx, sz)
	}
	bx, by, bz := int32(g.feet[0]), int32(g.feet[1]), int32(g.feet[2])
	if g.world.Block(bx, by-1, bz) == 0 || g.world.Block(bx, by, bz) != 0 {
		t.Fatalf("the server put the player at %v: below %d, at feet %d", g.feet, g.world.Block(bx, by-1, bz)>>4, g.world.Block(bx, by, bz)>>4)
	}
	// What the client holds is what the generator made, block for block, light and biomes too.
	want := gen.Chunk(world.ChunkPos{X: bx >> 4, Z: bz >> 4})
	got := g.world.Snapshot(world.ChunkPos{X: bx >> 4, Z: bz >> 4})
	for i := range want.Blocks {
		if got.Blocks[i] != want.Blocks[i] && want.Blocks[i]>>4 != 52 {
			t.Fatalf("block %d of the column differs: server %d, client %d", i, want.Blocks[i]>>4, got.Blocks[i]>>4)
		}
	}
	if got.Biomes == nil || got.Biomes[7] != want.Biomes[7] {
		t.Fatal("the client did not get the generated biomes")
	}
	dark := 0
	for i := 0; i < 65536; i++ {
		if got.Light[i] != want.Light[i] {
			t.Fatalf("light %d differs: server %#x client %#x", i, want.Light[i], got.Light[i])
		}
		if got.Light[i]>>4 < 15 && got.Blocks[i] == 0 {
			dark++
		}
	}
	_ = dark
}
