package client

import "math"

// Hostile mobs appear in the dark, as SpawnerAnimals makes them, and go again when far away.

// skylightSubtracted is how many levels of sky light the night takes away.
func skylightSubtracted(angle float32) int {
	f := 1 - (float32(math.Cos(float64(angle)*2*math.Pi))*2 + 0.5)
	f = max(0, min(1, f))
	return int((1 - (1 - f)) * 11)
}

// darkEnough is EntityMob.isValidLightLevel for a place.
func (g *game) darkEnough(x, y, z int32) bool {
	light := g.world.Light(x, y, z)
	sky, blk := int(light>>4), int(light&15)
	if sky > rng.Intn(32) { // a place the sky can reach is not dark enough, however late
		return false
	}
	eff := max(blk, sky-skylightSubtracted(celestialAngle(g.worldTime)))
	return eff <= rng.Intn(8)
}

func (g *game) countHostiles() int {
	n := 0
	for _, m := range g.mobs {
		if m.kind.hostile && !m.dead() {
			n++
		}
	}
	return n
}

// spawnAmbient brings bats to dark caves and squid to open water, as SpawnerAnimals does for ambient and
// water creatures: a few at a time, 24 to 48 blocks from the player, up to a limit in the area.
func (g *game) spawnAmbient() {
	if g.worldTime%20 != 0 {
		return
	}
	bats, squid := 0, 0
	for _, m := range g.mobs {
		switch m.kind.id {
		case "Bat":
			bats++
		case "Squid":
			squid++
		}
	}
	ang := rng.Float64() * 2 * math.Pi
	dist := 24 + rng.Float64()*24
	x := int32(math.Floor(float64(g.feet[0]) + math.Cos(ang)*dist))
	z := int32(math.Floor(float64(g.feet[2]) + math.Sin(ang)*dist))
	if bats < 15 { // dark air under y=63
		y := int32(1 + rng.Intn(61))
		if g.world.Block(x, y, z) == 0 && g.world.Block(x, y+1, z) == 0 {
			light := g.world.Light(x, y, z)
			if int(max(light&15, light>>4)) <= rng.Intn(4) {
				g.mobs = append(g.mobs, g.newMob(kinds["Bat"], vec3{float64(x) + 0.5, float64(y), float64(z) + 0.5}, rng.Float32()*360))
			}
		}
	}
	if squid < 10 && rng.Intn(4) == 0 { // deep water between y=45 and 63
		for y := int32(62); y > 45; y-- {
			if g.world.Block(x, y, z)>>4 == 9 && g.world.Block(x, y+1, z)>>4 == 9 && g.world.Block(x, y-1, z)>>4 == 9 {
				for n := 1 + rng.Intn(3); n > 0; n-- {
					g.mobs = append(g.mobs, g.newMob(kinds["Squid"], vec3{float64(x) + 0.5 + float64(rng.Intn(3)-1), float64(y), float64(z) + 0.5 + float64(rng.Intn(3)-1)}, rng.Float32()*360))
				}
				break
			}
		}
	}
}

// spawnHostiles tries once a tick to bring a group of monsters to a dark place 24 to 48 blocks off.
func (g *game) spawnHostiles() {
	if g.dimn.id != dimOverworld || g.creative || g.worldTime%2 != 0 || g.countHostiles() >= 40 {
		return
	}
	ang := rng.Float64() * 2 * math.Pi
	dist := 24 + rng.Float64()*24
	x := int32(math.Floor(float64(g.feet[0]) + math.Cos(ang)*dist))
	z := int32(math.Floor(float64(g.feet[2]) + math.Sin(ang)*dist))
	y := int32(math.Floor(float64(g.feet[1]))) + int32(rng.Intn(24)) - 12
	// Find the floor: a solid block with two free blocks above it.
	for ; y > 1; y-- {
		if g.opaqueAt(x, y-1, z) && g.world.Block(x, y, z) == 0 && g.world.Block(x, y+1, z) == 0 {
			break
		}
	}
	if y <= 1 || !g.opaqueAt(x, y-1, z) || !g.darkEnough(x, y, z) {
		return
	}
	choices := []string{"Zombie", "Skeleton", "Creeper", "Spider"}
	k := kinds[choices[rng.Intn(len(choices))]]
	n := 1 + rng.Intn(3)
	for i := 0; i < n; i++ {
		px, pz := float32(x)+0.5+float32(rng.Intn(5)-2), float32(z)+0.5+float32(rng.Intn(5)-2)
		if g.opaqueAt(int32(math.Floor(float64(px))), y, int32(math.Floor(float64(pz)))) || !g.opaqueAt(int32(math.Floor(float64(px))), y-1, int32(math.Floor(float64(pz)))) {
			continue
		}
		g.mobs = append(g.mobs, g.newMob(k, vec3{float64(px), float64(y), float64(pz)}, rng.Float32()*360))
	}
}
