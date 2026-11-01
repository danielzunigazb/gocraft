package client

import (
	"gocraft/worldgen"
)

// Lightning, after 1.8's EntityLightningBolt and RenderLightningBolt. Spawn Global Entity (0x2C, type 1) puts a
// bolt at a place; on the client it does nothing but be seen: it thunders when it appears, flashes the sky while
// its state is not below zero, and strikes again (a new shape) a few times before it goes.

const objLightning = 0xFD // netEntity.obj for a bolt: not a mob, not clickable, put where the server says

// boltState is the bolt's own state: b ("state") counts down from 2 and goes to 1 for each new strike, living
// is how many strikes it has left, and seed is the shape of the current one.
type boltState struct {
	state, living int
	seed          int64
	hold          bool // a demo's bolt: stays as it is
}

// newBolt is the constructor of EntityLightningBolt.
func newBolt() *boltState {
	return &boltState{state: 2, living: rng.Intn(3) + 1, seed: rng.Int63()}
}

// tickBolt is EntityLightningBolt.onUpdate on the client. It returns false when the bolt is gone.
func (g *game) tickBolt(e *netEntity) bool {
	b := e.bolt
	if b.hold {
		g.flash = 2
		return true
	}
	if b.state == 2 {
		g.sound("ambient.weather.thunder", e.pos, 10000, 0.8+rng.Float32()*0.2)
		g.sound("random.explode", e.pos, 2, 0.5+rng.Float32()*0.2)
	}
	b.state--
	if b.state < 0 {
		if b.living == 0 {
			return false
		}
		if b.state < -rng.Intn(10) {
			b.living--
			b.state = 1
			b.seed = rng.Int63()
		}
	}
	if b.state >= 0 {
		g.flash = 2 // World.setLastLightningBolt(2): the sky flashes
	}
	return true
}

// boltColour is the colour of a bolt's strips: (0.45, 0.45, 0.5) at an alpha of 0.3, added to what is behind
// (blendFunc(SRC_ALPHA, ONE)); a one-one blend takes the alpha into the colour.
var boltColour = [4]uint8{34, 34, 38, 255} // 0.45, 0.45, 0.5 times 0.3, in bytes

// boltGeometry is RenderLightningBolt.doRender: four passes of three tapering strands (the main one and two
// branches), each strand a strip of five-sided segments 16 blocks long that wanders by what the bolt's seed
// gives. The quads of the strips go to out, around the point (x, y, z).
func boltGeometry(out []vertex, seed int64, x, y, z float64) []vertex {
	var ax, az [8]float64
	var d0, d1 float64
	r := worldgen.NewRandom(seed)
	for i := 7; i >= 0; i-- {
		ax[i], az[i] = d0, d1
		d0 += float64(r.IntN(11) - 5)
		d1 += float64(r.IntN(11) - 5)
	}
	for k1 := 0; k1 < 4; k1++ {
		r1 := worldgen.NewRandom(seed)
		for j := 0; j < 3; j++ {
			k, l := 7, 0
			if j > 0 {
				k = 7 - j
				l = k - 2
			}
			d2, d3 := ax[k]-d0, az[k]-d1
			for i1 := k; i1 >= l; i1-- {
				d4, d5 := d2, d3
				if j == 0 {
					d2 += float64(r1.IntN(11) - 5)
					d3 += float64(r1.IntN(11) - 5)
				} else {
					d2 += float64(r1.IntN(31) - 15)
					d3 += float64(r1.IntN(31) - 15)
				}
				d6 := 0.1 + float64(k1)*0.2
				if j == 0 {
					d6 *= float64(i1)*0.1 + 1
				}
				d7 := 0.1 + float64(k1)*0.2
				if j == 0 {
					d7 *= float64(i1-1)*0.1 + 1
				}
				var lo, hi [5]vec3 // the strip's pairs: the lower vertex and the upper one
				for j1 := 0; j1 < 5; j1++ {
					d8, d9 := x+0.5-d6, z+0.5-d6
					if j1 == 1 || j1 == 2 {
						d8 += d6 * 2
					}
					if j1 == 2 || j1 == 3 {
						d9 += d6 * 2
					}
					d10, d11 := x+0.5-d7, z+0.5-d7
					if j1 == 1 || j1 == 2 {
						d10 += d7 * 2
					}
					if j1 == 2 || j1 == 3 {
						d11 += d7 * 2
					}
					lo[j1] = vec3{d10 + d2, y + float64(i1*16), d11 + d3}
					hi[j1] = vec3{d8 + d4, y + float64((i1+1)*16), d9 + d5}
				}
				for s := 0; s < 4; s++ { // a strip of pairs, as quads
					for _, p := range [4]vec3{lo[s], hi[s], hi[s+1], lo[s+1]} {
						out = append(out, vertex{float32(p[0]), float32(p[1]), float32(p[2]), 0.5, 0.5, boltColour})
					}
				}
			}
		}
	}
	return out
}

// emitBolts adds the bolts in the world to the additive group.
func (g *game) emitBolts(groups map[string][]vertex) {
	if g.net == nil {
		return
	}
	for _, e := range g.net.ents {
		if e.bolt != nil && e.bolt.state >= 0 {
			groups["!bolt"] = boltGeometry(groups["!bolt"], e.bolt.seed, e.pos[0], e.pos[1], e.pos[2])
		}
	}
}

// lightningSky is World.getSkyColor's flash: while a bolt lights the sky it is pulled towards a pale 0.8.
func lightningSky(c [3]float32, flash int, partial float32) [3]float32 {
	if flash <= 0 {
		return c
	}
	f := min(float32(flash)-partial, 1) * 0.45
	for i := range c {
		c[i] = c[i]*(1-f) + 0.8*f
	}
	return c
}
