package worldgen

// MapGenCaves and MapGenRavine: tunnels walked through the stone by a random walk seeded from
// each chunk within 8 chunks of the one being generated.

const carveRange = 8

// carveSeeds runs fn for every chunk in range, with the random generator the game seeds for it.
func (g *Generator) carveSeeds(cx, cz int32, fn func(rnd *Random, x, z int32)) {
	rnd := NewRandom(g.seed)
	j := rnd.Long()
	k := rnd.Long()
	for l := cx - carveRange; l <= cx+carveRange; l++ {
		for i := cz - carveRange; i <= cz+carveRange; i++ {
			rnd.SetSeed(int64(l)*j ^ int64(i)*k ^ g.seed)
			fn(rnd, l, i)
		}
	}
}

func (g *Generator) carveCaves(b *baseChunk, cx, cz int32) {
	g.carveSeeds(cx, cz, func(rnd *Random, x, z int32) { g.caveChunk(b, rnd, x, z, cx, cz) })
}

func (g *Generator) carveRavines(b *baseChunk, cx, cz int32) {
	g.carveSeeds(cx, cz, func(rnd *Random, x, z int32) { g.ravineChunk(b, rnd, x, z, cx, cz) })
}

func cosF(f float32) float32 { return mhCos(f) }
func sinF(f float32) float32 { return mhSin(f) }

const piF = float32(3.14159265358979323846)

// caveChunk is MapGenCaves.recursiveGenerate: the caves that start in chunk (x, z).
func (g *Generator) caveChunk(b *baseChunk, rnd *Random, x, z, cx, cz int32) {
	i := rnd.IntN(rnd.IntN(rnd.IntN(15)+1) + 1)
	if rnd.IntN(7) != 0 {
		i = 0
	}
	for j := int32(0); j < i; j++ {
		d0 := float64(x*16 + rnd.IntN(16))
		d1 := float64(rnd.IntN(rnd.IntN(120) + 8))
		d2 := float64(z*16 + rnd.IntN(16))
		k := int32(1)
		if rnd.IntN(4) == 0 {
			g.caveTunnel(b, rnd.Long(), cx, cz, d0, d1, d2, 1+rnd.Float()*6, 0, 0, -1, -1, 0.5)
			k += rnd.IntN(4)
		}
		for l := int32(0); l < k; l++ {
			f := rnd.Float() * piF * 2
			f1 := (rnd.Float() - 0.5) * 2 / 8
			f2 := rnd.Float()*2 + rnd.Float()
			if rnd.IntN(10) == 0 {
				f2 *= rnd.Float()*rnd.Float()*3 + 1
			}
			g.caveTunnel(b, rnd.Long(), cx, cz, d0, d1, d2, f2, f, f1, 0, 0, 1)
		}
	}
}

func canCarve(st, above uint16) bool {
	switch id := st >> 4; id {
	case 1, 3, 2, 172, 159, 24, 179, 110, 78:
		return true
	case 12, 13:
		a := above >> 4
		return a != 8 && a != 9
	}
	return false
}

func (g *Generator) caveTunnel(b *baseChunk, seed int64, cx, cz int32, px, py, pz float64, width, yaw, pitch float32, step, length int32, vscale float64) {
	d0 := float64(cx*16 + 8)
	d1 := float64(cz*16 + 8)
	var f, f1 float32
	rnd := NewRandom(seed)
	if length <= 0 {
		i := int32(carveRange*16 - 16)
		length = i - rnd.IntN(i/4)
	}
	flag2 := false
	if step == -1 {
		step = length / 2
		flag2 = true
	}
	j := rnd.IntN(length/2) + length/4
	flag := rnd.IntN(6) == 0
	for ; step < length; step++ {
		d2 := 1.5 + float64(sinF(float32(step)*piF/float32(length))*width*1)
		d3 := d2 * vscale
		f2 := cosF(pitch)
		f3 := sinF(pitch)
		px += float64(cosF(yaw) * f2)
		py += float64(f3)
		pz += float64(sinF(yaw) * f2)
		if flag {
			pitch *= 0.92
		} else {
			pitch *= 0.7
		}
		pitch += f1 * 0.1
		yaw += f * 0.1
		f1 *= 0.9
		f *= 0.75
		f1 += (rnd.Float() - rnd.Float()) * rnd.Float() * 2
		f += (rnd.Float() - rnd.Float()) * rnd.Float() * 4
		if !flag2 && step == j && width > 1 && length > 0 {
			g.caveTunnel(b, rnd.Long(), cx, cz, px, py, pz, rnd.Float()*0.5+0.5, yaw-piF/2, pitch/3, step, length, 1)
			g.caveTunnel(b, rnd.Long(), cx, cz, px, py, pz, rnd.Float()*0.5+0.5, yaw+piF/2, pitch/3, step, length, 1)
			return
		}
		if !flag2 && rnd.IntN(4) == 0 {
			continue
		}
		d4 := px - d0
		d5 := pz - d1
		d6 := float64(length - step)
		d7 := float64(width + 2 + 16)
		if d4*d4+d5*d5-d6*d6 > d7*d7 {
			return
		}
		if px < d0-16-d2*2 || pz < d1-16-d2*2 || px > d0+16+d2*2 || pz > d1+16+d2*2 {
			continue
		}
		k2 := int(floorD(px-d2)) - int(cx)*16 - 1
		k := int(floorD(px+d2)) - int(cx)*16 + 1
		l2 := int(floorD(py-d3)) - 1
		l := int(floorD(py+d3)) + 1
		i3 := int(floorD(pz-d2)) - int(cz)*16 - 1
		i1 := int(floorD(pz+d2)) - int(cz)*16 + 1
		k2 = max(k2, 0)
		k = min(k, 16)
		l2 = max(l2, 1)
		l = min(l, 248)
		i3 = max(i3, 0)
		i1 = min(i1, 16)
		water := false
		for j1 := k2; !water && j1 < k; j1++ {
			for k1 := i3; !water && k1 < i1; k1++ {
				for l1 := l + 1; !water && l1 >= l2-1; l1-- {
					if l1 >= 0 && l1 < 256 {
						if id := b.blocks[idx(j1, l1, k1)] >> 4; id == 8 || id == 9 {
							water = true
						}
						if l1 != l2-1 && j1 != k2 && j1 != k-1 && k1 != i3 && k1 != i1-1 {
							l1 = l2
						}
					}
				}
			}
		}
		if water {
			continue
		}
		for j3 := k2; j3 < k; j3++ {
			d10 := (float64(j3+int(cx)*16) + 0.5 - px) / d2
			for i2 := i3; i2 < i1; i2++ {
				d8 := (float64(i2+int(cz)*16) + 0.5 - pz) / d2
				grass := false
				if d10*d10+d8*d8 < 1 {
					for j2 := l; j2 > l2; j2-- {
						d9 := (float64(j2-1) + 0.5 - py) / d3
						if d9 > -0.7 && d10*d10+d9*d9+d8*d8 < 1 {
							st := b.blocks[idx(j3, j2, i2)]
							above := b.blocks[idx(j3, j2+1, i2)]
							if id := st >> 4; id == 2 || id == 110 {
								grass = true
							}
							if canCarve(st, above) {
								if j2-1 < 10 {
									b.blocks[idx(j3, j2, i2)] = stFlowLava
								} else {
									b.blocks[idx(j3, j2, i2)] = stAir
									if above>>4 == 12 {
										if above == stRedSand {
											b.blocks[idx(j3, j2+1, i2)] = stRedSandst
										} else {
											b.blocks[idx(j3, j2+1, i2)] = stSandstone
										}
									}
									if grass && b.blocks[idx(j3, j2-1, i2)]>>4 == 3 {
										b.blocks[idx(j3, j2-1, i2)] = biomeByID(int(b.biomes[j3+i2*16])).Top
									}
								}
							}
						}
					}
				}
			}
		}
		if flag2 {
			break
		}
	}
}

// ravineChunk is MapGenRavine.recursiveGenerate.
func (g *Generator) ravineChunk(b *baseChunk, rnd *Random, x, z, cx, cz int32) {
	if rnd.IntN(50) != 0 {
		return
	}
	d0 := float64(x*16 + rnd.IntN(16))
	d1 := float64(rnd.IntN(rnd.IntN(40)+8) + 20)
	d2 := float64(z*16 + rnd.IntN(16))
	f := rnd.Float() * piF * 2
	f1 := (rnd.Float() - 0.5) * 2 / 8
	f2 := (rnd.Float()*2 + rnd.Float()) * 2
	g.ravineTunnel(b, rnd.Long(), cx, cz, d0, d1, d2, f2, f, f1, 0, 0, 3)
}

func (g *Generator) ravineTunnel(b *baseChunk, seed int64, cx, cz int32, px, py, pz float64, width, yaw, pitch float32, step, length int32, vscale float64) {
	rnd := NewRandom(seed)
	d0 := float64(cx*16 + 8)
	d1 := float64(cz*16 + 8)
	var f, f1 float32
	if length <= 0 {
		i := int32(carveRange*16 - 16)
		length = i - rnd.IntN(i/4)
	}
	flag1 := false
	if step == -1 {
		step = length / 2
		flag1 = true
	}
	var shape [1024]float32
	f2 := float32(1)
	for j := 0; j < 256; j++ {
		if j == 0 || rnd.IntN(3) == 0 {
			f2 = 1 + rnd.Float()*rnd.Float()*1
		}
		shape[j] = f2 * f2
	}
	for ; step < length; step++ {
		d9 := 1.5 + float64(sinF(float32(step)*piF/float32(length))*width*1)
		d2 := d9 * vscale
		d9 *= float64(rnd.Float())*0.25 + 0.75
		d2 *= float64(rnd.Float())*0.25 + 0.75
		f3 := cosF(pitch)
		f4 := sinF(pitch)
		px += float64(cosF(yaw) * f3)
		py += float64(f4)
		pz += float64(sinF(yaw) * f3)
		pitch *= 0.7
		pitch += f1 * 0.05
		yaw += f * 0.05
		f1 *= 0.8
		f *= 0.5
		f1 += (rnd.Float() - rnd.Float()) * rnd.Float() * 2
		f += (rnd.Float() - rnd.Float()) * rnd.Float() * 4
		if !flag1 && rnd.IntN(4) == 0 {
			continue
		}
		d4 := px - d0
		d5 := pz - d1
		d6 := float64(length - step)
		d7 := float64(width + 2 + 16)
		if d4*d4+d5*d5-d6*d6 > d7*d7 {
			return
		}
		if px < d0-16-d9*2 || pz < d1-16-d9*2 || px > d0+16+d9*2 || pz > d1+16+d9*2 {
			continue
		}
		k2 := int(floorD(px-d9)) - int(cx)*16 - 1
		k := int(floorD(px+d9)) - int(cx)*16 + 1
		l2 := int(floorD(py-d2)) - 1
		l := int(floorD(py+d2)) + 1
		i3 := int(floorD(pz-d9)) - int(cz)*16 - 1
		i1 := int(floorD(pz+d9)) - int(cz)*16 + 1
		k2 = max(k2, 0)
		k = min(k, 16)
		l2 = max(l2, 1)
		l = min(l, 248)
		i3 = max(i3, 0)
		i1 = min(i1, 16)
		water := false
		for j1 := k2; !water && j1 < k; j1++ {
			for k1 := i3; !water && k1 < i1; k1++ {
				for l1 := l + 1; !water && l1 >= l2-1; l1-- {
					if l1 >= 0 && l1 < 256 {
						if id := b.blocks[idx(j1, l1, k1)] >> 4; id == 8 || id == 9 {
							water = true
						}
						if l1 != l2-1 && j1 != k2 && j1 != k-1 && k1 != i3 && k1 != i1-1 {
							l1 = l2
						}
					}
				}
			}
		}
		if water {
			continue
		}
		for j3 := k2; j3 < k; j3++ {
			d10 := (float64(j3+int(cx)*16) + 0.5 - px) / d9
			for l3 := i3; l3 < i1; l3++ {
				d11 := (float64(l3+int(cz)*16) + 0.5 - pz) / d9
				grass := false
				if d10*d10+d11*d11 < 1 {
					for i2 := l; i2 > l2; i2-- {
						d12 := (float64(i2-1) + 0.5 - py) / d2
						if (d10*d10+d11*d11)*float64(shape[i2-1])+d12*d12/6 < 1 {
							st := b.blocks[idx(j3, i2, l3)]
							if st>>4 == 2 {
								grass = true
							}
							if id := st >> 4; id == 1 || id == 3 || id == 2 {
								if i2-1 < 10 {
									b.blocks[idx(j3, i2, l3)] = stFlowLava
								} else {
									b.blocks[idx(j3, i2, l3)] = stAir
									if grass && b.blocks[idx(j3, i2-1, l3)]>>4 == 3 {
										b.blocks[idx(j3, i2-1, l3)] = biomeByID(int(b.biomes[j3+l3*16])).Top
									}
								}
							}
						}
					}
				}
			}
		}
		if flag1 {
			break
		}
	}
}
