package client

import (
	"bytes"
	"math"

	"gocraft/anvil"
)

// Mob spawners, after 1.8's MobSpawnerBaseLogic and TileEntityMobSpawnerRenderer. Update Block Entity (0x35, action 1)
// names the mob inside (EntityId). On the client the logic only turns it: while a player is within 16 blocks it
// advances the mob's rotation by 1000 / (delay + 200) degrees a tick, the delay counting down from 20 to 0, and the
// renderer draws that mob at 0.4375 of its size, turned ten times as far, over the cage.

const (
	idSpawner      = 52
	spawnerRange   = 16.0
	spawnerMinWait = 200 // MobSpawnerBaseLogic.minSpawnDelay: what a block event of 1 puts the delay back to
)

type spawnerState struct {
	mob          string // the mob's name in EntityList ("Zombie"), which is also the kind's id here
	delay        int
	rot, prevRot float32
}

// netSpawner records the mob a spawner holds.
func (g *game) netSpawner(pos [3]int32, nbt []byte) {
	root, err := anvil.ReadNBT(bytes.NewReader(nbt))
	if err != nil {
		return
	}
	name, _ := root["EntityId"].(string)
	if g.spawners == nil {
		g.spawners = map[[3]int32]*spawnerState{}
	}
	if len(g.spawners) >= 4096 {
		return
	}
	s := g.spawners[pos]
	if s == nil {
		s = &spawnerState{delay: 20}
		g.spawners[pos] = s
	}
	s.mob = name
}

// tickSpawners is MobSpawnerBaseLogic.updateSpawner on the client for the spawners a player is near.
func (g *game) tickSpawners() {
	for pos, s := range g.spawners {
		c := centre(pos[0], pos[1], pos[2])
		if d := c.sub(g.pos); d.dot(d) > spawnerRange*spawnerRange {
			continue // not activated
		}
		if s.delay > 0 {
			s.delay--
		}
		s.prevRot = s.rot
		s.rot = float32(math.Mod(float64(s.rot+1000/(float32(s.delay)+200)), 360))
	}
}

// spawnerEvent is Block Action 1 on a spawner: the delay goes back to its minimum.
func (g *game) spawnerEvent(pos [3]int32) {
	if s := g.spawners[pos]; s != nil {
		s.delay = spawnerMinWait
	}
}

// emitSpawners draws the mob in each spawner near the camera.
func (g *game) emitSpawners(groups map[string][]vertex) {
	for pos, s := range g.spawners {
		if g.world.Block(pos[0], pos[1], pos[2])>>4 != idSpawner {
			continue
		}
		k := kinds[s.mob]
		c := centre(pos[0], pos[1], pos[2])
		if k == nil || g.offScreen(c, 1) {
			continue
		}
		if d := c.sub(g.camPos); d.dot(d) > 64*64 {
			continue
		}
		// rotate((prev + (rot - prev) * partial) * 10) about the vertical, the feet 0.0536 up: the translate and
		// the tilt of the game's renderer leave them there (the tilt itself, 30 degrees forward, is not drawn)
		a := (s.prevRot + (s.rot-s.prevRot)*g.partial) * 10
		feet := vec3{float64(pos[0]) + 0.5, float64(pos[1]) + 0.4 - 0.4*math.Cos(30*math.Pi/180), float64(pos[2]) + 0.5}
		m := &mob{kind: k, pos: feet, prev: feet, yaw: a, prevYaw: a, health: k.health, onGround: true, size: 1, drawScale: 0.4375}
		g.emitMob(groups, m, g.partial)
	}
}
