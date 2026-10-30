package worldgen

import "math"

// MapGenCavesHell (class anz): the Nether's tunnels and rooms, carved after the surface. They go
// through netherrack, dirt and grass only, never through lava, and stay between y=1 and y=120.

var nhSinTable = func() (t [65536]float32) {
	for i := range t {
		t[i] = float32(math.Sin(float64(i) * math.Pi * 2 / 65536))
	}
	return
}()

func nhSin(f float32) float32 { return nhSinTable[int32(f*10430.378)&65535] }
func nhCos(f float32) float32 { return nhSinTable[int32(f*10430.378+16384)&65535] }

func nhFloor(d float64) int32 {
	i := int32(d)
	if d < float64(i) {
		return i - 1
	}
	return i
}

// carve is MapGenBase.generate: every chunk within 8 of this one may start tunnels that reach it.
func (n *Nether) carve(cx, cz int32, b *nhColumn) {
	const rng = 8
	r := NewRandom(n.seed)
	j := r.Long()
	k := r.Long()
	for l := cx - rng; l <= cx+rng; l++ {
		for i1 := cz - rng; i1 <= cz+rng; i1++ {
			r.SetSeed(int64(l)*j ^ int64(i1)*k ^ n.seed)
			n.startTunnels(r, l, i1, cx, cz, b)
		}
	}
}

func (n *Nether) startTunnels(r *Random, chunkX, chunkZ, tx, tz int32, b *nhColumn) {
	i := r.IntN(r.IntN(r.IntN(10)+1) + 1)
	if r.IntN(5) != 0 {
		i = 0
	}
	for j := int32(0); j < i; j++ {
		d0 := float64(chunkX*16 + r.IntN(16))
		d1 := float64(r.IntN(128))
		d2 := float64(chunkZ*16 + r.IntN(16))
		k := int32(1)
		if r.IntN(4) == 0 {
			n.tunnel(r.Long(), tx, tz, b, d0, d1, d2, 1+r.Float()*6, 0, 0, -1, -1, 0.5)
			k += r.IntN(4)
		}
		for l := int32(0); l < k; l++ {
			f := r.Float() * math.Pi * 2
			f1 := (r.Float() - 0.5) * 2 / 8
			f2 := r.Float()*2 + r.Float()
			n.tunnel(r.Long(), tx, tz, b, d0, d1, d2, f2*2, float32(f), f1, 0, 0, 0.5)
		}
	}
}

// tunnel is one winding passage (or, started with branch -1, a round room) that may split in two.
func (n *Nether) tunnel(seed int64, chunkX, chunkZ int32, b *nhColumn, x, y, z float64, radius, yaw, pitch float32, branch, maxBranch int32, yScale float64) {
	d0 := float64(chunkX*16 + 8)
	d1 := float64(chunkZ*16 + 8)
	var f, f1 float32
	r := NewRandom(seed)
	if maxBranch <= 0 {
		i := int32(8*16 - 16)
		maxBranch = i - r.IntN(i/4)
	}
	room := false
	if branch == -1 {
		branch = maxBranch / 2
		room = true
	}
	j := r.IntN(maxBranch/2) + maxBranch/4
	flag := r.IntN(6) == 0
	for ; branch < maxBranch; branch++ {
		d2 := 1.5 + float64(nhSin(float32(branch)*math.Pi/float32(maxBranch))*radius*1)
		d3 := d2 * yScale
		f2 := nhCos(pitch)
		f3 := nhSin(pitch)
		x += float64(nhCos(yaw) * f2)
		y += float64(f3)
		z += float64(nhSin(yaw) * f2)
		if flag {
			pitch *= 0.92
		} else {
			pitch *= 0.7
		}
		pitch += f1 * 0.1
		yaw += f * 0.1
		f1 *= 0.9
		f *= 0.75
		f1 += (r.Float() - r.Float()) * r.Float() * 2
		f += (r.Float() - r.Float()) * r.Float() * 4
		if !room && branch == j && radius > 1 {
			n.tunnel(r.Long(), chunkX, chunkZ, b, x, y, z, r.Float()*0.5+0.5, yaw-math.Pi/2, pitch/3, branch, maxBranch, 1)
			n.tunnel(r.Long(), chunkX, chunkZ, b, x, y, z, r.Float()*0.5+0.5, yaw+math.Pi/2, pitch/3, branch, maxBranch, 1)
			return
		}
		if !room && r.IntN(4) == 0 {
			continue
		}
		d4 := x - d0
		d5 := z - d1
		d6 := float64(maxBranch - branch)
		d7 := float64(radius + 2 + 16)
		if d4*d4+d5*d5-d6*d6 > d7*d7 {
			return
		}
		if x < d0-16-d2*2 || z < d1-16-d2*2 || x > d0+16+d2*2 || z > d1+16+d2*2 {
			continue
		}
		k2 := int(nhFloor(x-d2)) - int(chunkX)*16 - 1
		k := int(nhFloor(x+d2)) - int(chunkX)*16 + 1
		l2 := int(nhFloor(y-d3)) - 1
		l := int(nhFloor(y+d3)) + 1
		i3 := int(nhFloor(z-d2)) - int(chunkZ)*16 - 1
		i1 := int(nhFloor(z+d2)) - int(chunkZ)*16 + 1
		k2, k = max(k2, 0), min(k, 16)
		l2, l = max(l2, 1), min(l, 120)
		i3, i1 = max(i3, 0), min(i1, 16)
		hitLava := false
		for j1 := k2; !hitLava && j1 < k; j1++ {
			for k1 := i3; !hitLava && k1 < i1; k1++ {
				for l1 := l + 1; !hitLava && l1 >= l2-1; l1-- {
					if l1 >= 0 && l1 < nhHeight {
						if id := b[nhIdx(j1, l1, k1)] >> 4; id == 10 || id == 11 {
							hitLava = true
						}
						if l1 != l2-1 && j1 != k2 && j1 != k-1 && k1 != i3 && k1 != i1-1 {
							l1 = l2 // only the shell is checked: jump to the bottom
						}
					}
				}
			}
		}
		if hitLava {
			continue
		}
		for j3 := k2; j3 < k; j3++ {
			d10 := (float64(j3+int(chunkX)*16) + 0.5 - x) / d2
			for i2 := i3; i2 < i1; i2++ {
				d8 := (float64(i2+int(chunkZ)*16) + 0.5 - z) / d2
				for j2 := l; j2 > l2; j2-- {
					d9 := (float64(j2-1) + 0.5 - y) / d3
					if d9 > -0.7 && d10*d10+d9*d9+d8*d8 < 1 {
						switch b[nhIdx(j3, j2, i2)] >> 4 {
						case 87, 3, 2: // netherrack, dirt, grass
							b[nhIdx(j3, j2, i2)] = nhAir
						}
					}
				}
			}
		}
		if room {
			break
		}
	}
}
