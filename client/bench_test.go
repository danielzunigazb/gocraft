package client

import (
	"os"
	"path/filepath"
	"testing"

	"gocraft/anvil"
	"gocraft/world"
)

// benchWorld opens the player's New World (a forest: the heavy case) with the real jar.
func benchWorld(b *testing.B) (*game, world.ChunkPos) {
	dir := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "saves", "New World")
	if _, err := os.Stat(dir); err != nil {
		b.Skip("no 1.8.9 world")
	}
	jar := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "client.jar")
	resOnce.Do(func() {
		r, err := loadResources(jar, nil)
		if err != nil {
			b.Fatal(err)
		}
		testRes = r
	})
	b.Setenv("XDG_DATA_HOME", b.TempDir()) // never the player's own overlay
	save, err := anvil.Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(save.Close)
	g := &game{world: world.New(), res: testRes, health: maxHealth, food: maxFood}
	g.applySave(save)
	return g, world.ChunkPos{X: int32(g.pos[0]) >> 4, Z: int32(g.pos[2]) >> 4}
}

func BenchmarkMeshChunk(b *testing.B) {
	g, c := benchWorld(b)
	for dx := int32(-1); dx <= 1; dx++ { // load the neighbours first, so only meshing is timed
		for dz := int32(-1); dz <= 1; dz++ {
			g.world.Snapshot(world.ChunkPos{X: c.X + dx, Z: c.Z + dz})
		}
	}
	var verts int
	m := &mesher{}
	var md meshData
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.mesh(g.world, c, g.res, false, &md)
		verts = len(md.solid) + len(md.water)
	}
	b.ReportMetric(float64(verts), "verts/chunk")
}

func BenchmarkMeshChunkFancy(b *testing.B) {
	g, c := benchWorld(b)
	m := &mesher{}
	var md meshData
	m.mesh(g.world, c, g.res, true, &md)
	var verts int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.mesh(g.world, c, g.res, true, &md)
		verts = len(md.solid) + len(md.water)
	}
	b.ReportMetric(float64(verts), "verts/chunk")
}
