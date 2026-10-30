package server

import (
	"gocraft/world"
)

// The spawn point. A flat world has it at the origin. A generated one has it where its generator
// says (Config.Spawn), with the player standing on the highest solid block of that column.

func (s *Server) spawnPoint() (x, y, z float64, cp world.ChunkPos) {
	if s.cfg.Spawn == nil {
		return 0.5, spawnY, 0.5, world.ChunkPos{}
	}
	s.spawnOnce.Do(func() {
		bx, bz := s.cfg.Spawn()
		by := 255
		for by > 0 && s.world.Block(bx, int32(by), bz) == 0 {
			by--
		}
		s.spawnAt = [3]float64{float64(bx) + 0.5, float64(by) + 1, float64(bz) + 0.5}
	})
	x, y, z = s.spawnAt[0], s.spawnAt[1], s.spawnAt[2]
	return x, y, z, world.ChunkPos{X: int32(x) >> 4, Z: int32(z) >> 4}
}
