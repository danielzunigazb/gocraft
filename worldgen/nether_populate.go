package worldgen

// The Nether's decoration, after ChunkProviderHell.populate (read from its bytecode): lava traps,
// fire, two kinds of glowstone, mushrooms, quartz and lava in the walls. Decorating chunk P changes
// the blocks of P and of the chunk after it in each direction, so a column is finished inside a
// zone of 3 x 3 columns, applying the four decorations that reach the middle one, as the Overworld
// generator does.

// nhZone is the 3 x 3 columns around the one being made.
type nhZone struct {
	ox, oz int // block position of the zone's corner
	col    [3][3]*nhColumn
}

func (z *nhZone) at(x, y, zz int) (c *nhColumn, i int, ok bool) {
	rx, rz := x-z.ox, zz-z.oz
	if y < 0 || y >= nhHeight || rx < 0 || rz < 0 || rx >= 48 || rz >= 48 {
		return nil, 0, false
	}
	return z.col[rx>>4][rz>>4], nhIdx(rx&15, y, rz&15), true
}

func (z *nhZone) get(x, y, zz int) uint16 {
	if c, i, ok := z.at(x, y, zz); ok {
		return c[i]
	}
	return nhAir
}

func (z *nhZone) set(x, y, zz int, st uint16) {
	if c, i, ok := z.at(x, y, zz); ok {
		c[i] = st
	}
}

func (z *nhZone) id(x, y, zz int) uint16 { return z.get(x, y, zz) >> 4 }

// popRandom is the Random a chunk's decoration draws from: seeded from the world seed and the chunk,
// as the Overworld's is. (The game uses the Random the last terrain left, which depends on the order
// chunks were made in.)
func (n *Nether) popRandom(px, pz int32) *Random {
	r := NewRandom(n.seed)
	k := r.Long()/2*2 + 1
	l := r.Long()/2*2 + 1
	r.SetSeed((int64(px)*k + int64(pz)*l) ^ n.seed)
	return r
}

func (n *Nether) populate(z *nhZone, px, pz int32) {
	r := n.popRandom(px, pz)
	bx, bz := int(px)*16, int(pz)*16
	pos16 := func(yBound, yOff int32) (int, int, int) { // nextInt(16)+8, nextInt(yBound)+yOff, nextInt(16)+8
		x := int(r.IntN(16)) + 8
		y := int(r.IntN(yBound)) + int(yOff)
		zz := int(r.IntN(16)) + 8
		return bx + x, y, bz + zz
	}
	for i := 0; i < 8; i++ {
		x, y, zz := pos16(120, 4)
		nhLavaTrap(z, x, y, zz, false)
	}
	for j := 0; j < int(r.IntN(r.IntN(10)+1))+1; j++ {
		x, y, zz := pos16(120, 4)
		nhFireGen(z, r, x, y, zz)
	}
	for k := 0; k < int(r.IntN(r.IntN(10)+1)); k++ {
		x, y, zz := pos16(120, 4)
		nhGlowstoneGen(z, r, x, y, zz)
	}
	for l := 0; l < 10; l++ {
		x, y, zz := pos16(128, 0)
		nhGlowstoneGen(z, r, x, y, zz)
	}
	if r.Bool() {
		x, y, zz := pos16(128, 0)
		nhMushroomGen(z, r, x, y, zz, nhBrownMush)
	}
	if r.Bool() {
		x, y, zz := pos16(128, 0)
		nhMushroomGen(z, r, x, y, zz, nhRedMush)
	}
	for i := 0; i < 16; i++ {
		x := bx + int(r.IntN(16))
		y := int(r.IntN(108)) + 10
		zz := bz + int(r.IntN(16))
		nhQuartzVein(z, r, x, y, zz, 14)
	}
	for j := 0; j < 16; j++ {
		x := bx + int(r.IntN(16))
		y := int(r.IntN(108)) + 10
		zz := bz + int(r.IntN(16))
		nhLavaTrap(z, x, y, zz, true)
	}
}

// nhLavaTrap is WorldGenHellLava: a pocket of lava in a netherrack wall. The wall-only form
// (inWall) needs all five neighbours to be netherrack; the other also takes a pocket that opens on
// one side.
func nhLavaTrap(z *nhZone, x, y, zz int, inWall bool) {
	if z.id(x, y+1, zz) != 87 {
		return
	}
	if cur := z.id(x, y, zz); cur != 0 && cur != 87 {
		return
	}
	n := 0
	for _, d := range [5][3]int{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}, {0, -1, 0}} {
		if z.id(x+d[0], y+d[1], zz+d[2]) == 87 {
			n++
		}
	}
	air := 0
	for _, d := range [5][3]int{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}, {0, -1, 0}} {
		if z.get(x+d[0], y+d[1], zz+d[2]) == 0 {
			air++
		}
	}
	if !inWall && n == 4 && air == 1 || n == 5 {
		z.set(x, y, zz, nhLava) // the game sets flowing lava and lets it tick; still lava is the same to look at
	}
}

// nhFireGen is WorldGenFire: up to 64 flames on netherrack.
func nhFireGen(z *nhZone, r *Random, x, y, zz int) {
	for i := 0; i < 64; i++ {
		px := x + int(r.IntN(8)) - int(r.IntN(8))
		py := y + int(r.IntN(4)) - int(r.IntN(4))
		pz := zz + int(r.IntN(8)) - int(r.IntN(8))
		if z.get(px, py, pz) == 0 && z.id(px, py-1, pz) == 87 {
			z.set(px, py, pz, nhFire)
		}
	}
}

// nhGlowstoneGen is WorldGenGlowStone1 and 2 (the bytecode of the two is the same): a clump grown
// from a block hanging from netherrack.
func nhGlowstoneGen(z *nhZone, r *Random, x, y, zz int) {
	if z.get(x, y, zz) != 0 || z.id(x, y+1, zz) != 87 {
		return
	}
	z.set(x, y, zz, nhGlowstone)
	for i := 0; i < 1500; i++ {
		px := x + int(r.IntN(8)) - int(r.IntN(8))
		py := y - int(r.IntN(12))
		pz := zz + int(r.IntN(8)) - int(r.IntN(8))
		if z.get(px, py, pz) != 0 {
			continue
		}
		touching := 0
		for _, d := range [6][3]int{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}} {
			if z.id(px+d[0], py+d[1], pz+d[2]) == 89 {
				touching++
			}
			if touching > 1 {
				break
			}
		}
		if touching == 1 {
			z.set(px, py, pz, nhGlowstone)
		}
	}
}

// nhMushroomGen is WorldGenFlowers for a mushroom: up to 64 tries on a solid block, in the dark.
func nhMushroomGen(z *nhZone, r *Random, x, y, zz int, st uint16) {
	for i := 0; i < 64; i++ {
		px := x + int(r.IntN(8)) - int(r.IntN(8))
		py := y + int(r.IntN(4)) - int(r.IntN(4))
		pz := zz + int(r.IntN(8)) - int(r.IntN(8))
		if z.get(px, py, pz) != 0 || py < 0 || py >= nhHeight {
			continue
		}
		switch z.id(px, py-1, pz) { // a full block to stand on
		case 87, 88, 13, 89, 153, 7:
			z.set(px, py, pz, st)
		}
	}
}

// nhQuartzVein is WorldGenMinable with netherrack as the host: a strand of ore blocks.
func nhQuartzVein(z *nhZone, r *Random, x, y, zz int, size int) {
	f := r.Float() * 3.1415927
	fs := float32(size)
	d0 := float64(float32(x+8) + nhSin(f)*fs/8)
	d1 := float64(float32(x+8) - nhSin(f)*fs/8)
	d2 := float64(float32(zz+8) + nhCos(f)*fs/8)
	d3 := float64(float32(zz+8) - nhCos(f)*fs/8)
	d4 := float64(y + int(r.IntN(3)) - 2)
	d5 := float64(y + int(r.IntN(3)) - 2)
	for i := 0; i < size; i++ {
		f1 := float32(i) / float32(size)
		d6 := d0 + (d1-d0)*float64(f1)
		d7 := d4 + (d5-d4)*float64(f1)
		d8 := d2 + (d3-d2)*float64(f1)
		d9 := r.Double() * float64(size) / 16
		d10 := float64(nhSin(3.1415927*f1)+1)*d9 + 1
		d11 := d10
		j, k, l := int(nhFloor(d6-d10/2)), int(nhFloor(d7-d11/2)), int(nhFloor(d8-d10/2))
		i1, j1, k1 := int(nhFloor(d6+d10/2)), int(nhFloor(d7+d11/2)), int(nhFloor(d8+d10/2))
		for l1 := j; l1 <= i1; l1++ {
			e12 := (float64(l1) + 0.5 - d6) / (d10 / 2)
			if e12*e12 >= 1 {
				continue
			}
			for i2 := k; i2 <= j1; i2++ {
				e13 := (float64(i2) + 0.5 - d7) / (d11 / 2)
				if e12*e12+e13*e13 >= 1 {
					continue
				}
				for j2 := l; j2 <= k1; j2++ {
					e14 := (float64(j2) + 0.5 - d8) / (d10 / 2)
					if e12*e12+e13*e13+e14*e14 < 1 && z.id(l1, i2, j2) == 87 {
						z.set(l1, i2, j2, nhQuartzOre)
					}
				}
			}
		}
	}
}
