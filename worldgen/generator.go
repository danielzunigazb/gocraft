package worldgen

import "gocraft/world"

// Chunk is the finished column at cp: base terrain, the decoration of the four chunks whose
// population reaches it, and light. It can be called for any coordinates, from any goroutine; the
// world has no edge that generation stops at.
func (g *Generator) Chunk(cp world.ChunkPos) *world.Chunk {
	z := &zone{ox: int(cp.X-1) * 16, oz: int(cp.Z-1) * 16}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			b := g.base(cp.X-1+int32(i), cp.Z-1+int32(j))
			blocks := new([65536]uint16)
			*blocks = b.blocks
			z.blocks[i][j] = blocks
			z.biomes[i][j] = &b.biomes
		}
	}
	// Decorating chunk P touches P and the chunks after it; the four that reach the middle one.
	for _, d := range [4][2]int32{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
		g.populate(z, cp.X+d[0], cp.Z+d[1])
	}
	c := &world.Chunk{Biomes: append([]uint8(nil), z.biomes[1][1][:]...)}
	mid := z.blocks[1][1]
	for i, st := range mid {
		switch st {
		case stFlowLava: // a lava source the carvers or springs placed settles into still lava
			st = stLava
		case stFlowWater:
			st = stWater
		}
		c.Blocks[i] = st
	}
	c.Light = g.light(z)
	return c
}

// Func is Chunk as the plain function the world and the server take.
func (g *Generator) Func() func(world.ChunkPos) *world.Chunk { return g.Chunk }

// Spawn is where the game puts the world's first spawn: a random pick among the cells of a
// "good" biome within 256 blocks of the origin, walked to the first grass found above sea level.
func (g *Generator) Spawn() (x, z int32) {
	st := g.stacks.Get().(*biomeStack)
	r := NewRandom(g.seed)
	const rng = 256
	l, i1 := (0-rng)>>2, (0-rng)>>2
	j1, k1 := ((0+rng)>>2)-l+1, ((0+rng)>>2)-i1+1
	cells := st.gen.ints(l, i1, j1, k1)
	g.stacks.Put(st)
	var px, pz int
	n := 0
	found := false
	for i, id := range cells {
		switch id {
		case 4, 5, 1, 3, 18, 19, 21, 22: // forest, taiga, plains, extreme hills, forest hills, taiga hills, jungle, jungle hills
		default:
			continue
		}
		cx := (l + i%j1) << 2
		cz := (i1 + i/j1) << 2
		if !found || r.IntN(int32(n+1)) == 0 {
			px, pz, found = cx, cz, true
			n++
		}
	}
	if !found {
		px, pz = 0, 0
	}
	for t := 0; t < 1000 && !g.canSpawnAt(px, pz); t++ {
		px += int(r.IntN(64)) - int(r.IntN(64))
		pz += int(r.IntN(64)) - int(r.IntN(64))
	}
	return int32(px), int32(pz)
}

// canSpawnAt is WorldProvider.canCoordinateBeSpawn: the first block at or above sea level with
// air over it must be grass.
func (g *Generator) canSpawnAt(x, zz int) bool {
	c := g.Chunk(world.ChunkPos{X: int32(x >> 4), Z: int32(zz >> 4)})
	y := seaLevel
	for y < 255 && c.Blocks[idx(x&15, y+1, zz&15)] != 0 {
		y++
	}
	return c.Blocks[idx(x&15, y, zz&15)]>>4 == 2
}
