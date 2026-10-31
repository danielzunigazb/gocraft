package client

import (
	"testing"

	"gocraft/world"
)

func benchGame(b *testing.B, players int) *game {
	b.Helper()
	g := testGame(&testing.T{}) // loads the shared resources once
	g.net = &netState{received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	for i := 0; i < players; i++ {
		at := vec3{float64(i % 10 * 2), 5, float64(i / 10 * 2)}
		g.net.remote[int32(100+i)] = &remotePlayer{eid: int32(100 + i), pos: at, prev: at, target: at, yaw: float32(i * 7), held: slot{ID: 276, N: 1}, ls: float32(i), lsa: 0.5,
			armor: [4]slot{{ID: 313, N: 1}, {ID: 312, N: 1}, {ID: 311, N: 1}, {ID: 310, N: 1}}}
	}
	return g
}

// BenchmarkEmitRemotePlayers is what drawing 60 armoured players with swords costs on the CPU each frame.
func BenchmarkEmitRemotePlayers(b *testing.B) {
	g := benchGame(b, 60)
	groups := map[string][]vertex{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k := range groups {
			groups[k] = groups[k][:0]
		}
		for _, p := range g.net.remote {
			g.emitRemote(groups, p)
		}
	}
}
