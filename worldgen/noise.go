package worldgen

import "math"

// Improved (classic) Perlin noise, summed in octaves, and the simplex noise the surface uses:
// NoiseGeneratorImproved, NoiseGeneratorOctaves, NoiseGeneratorSimplex, NoiseGeneratorPerlin.

var (
	gradX = [16]float64{1, -1, 1, -1, 1, -1, 1, -1, 0, 0, 0, 0, 1, 0, -1, 0}
	gradY = [16]float64{1, 1, -1, -1, 0, 0, 0, 0, 1, -1, 1, -1, 1, -1, 1, -1}
	gradZ = [16]float64{0, 0, 0, 0, 1, 1, -1, -1, 1, 1, -1, -1, 0, 1, 0, -1}
)

type improvedNoise struct {
	perm       [512]int32
	xo, yo, zo float64
}

func newImprovedNoise(r *Random) *improvedNoise {
	n := &improvedNoise{}
	n.xo = r.Double() * 256
	n.yo = r.Double() * 256
	n.zo = r.Double() * 256
	for i := int32(0); i < 256; i++ {
		n.perm[i] = i
	}
	for l := int32(0); l < 256; l++ {
		j := r.IntN(256-l) + l
		k := n.perm[l]
		n.perm[l] = n.perm[j]
		n.perm[j] = k
		n.perm[l+256] = n.perm[l]
	}
	return n
}

func lerp(t, a, b float64) float64 { return a + t*(b-a) }

func grad(h int32, x, y, z float64) float64 {
	i := h & 15
	return gradX[i]*x + gradY[i]*y + gradZ[i]*z
}

func grad2(h int32, x, z float64) float64 {
	i := h & 15
	return gradX[i]*x + gradZ[i]*z
}

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

// populate adds this octave's noise into out, as populateNoiseArray does.
func (n *improvedNoise) populate(out []float64, xo, yo, zo float64, xSize, ySize, zSize int, xScale, yScale, zScale, noiseScale float64) {
	if ySize == 1 {
		idx := 0
		inv := 1 / noiseScale
		for i := 0; i < xSize; i++ {
			d := xo + float64(i)*xScale + n.xo
			fi := int32(d)
			if d < float64(fi) {
				fi--
			}
			xi := fi & 255
			d -= float64(fi)
			fx := fade(d)
			for j := 0; j < zSize; j++ {
				e := zo + float64(j)*zScale + n.zo
				fk := int32(e)
				if e < float64(fk) {
					fk--
				}
				zi := fk & 255
				e -= float64(fk)
				fz := fade(e)
				a := n.perm[xi] + 0
				b := n.perm[a] + zi
				c := n.perm[xi+1] + 0
				d2 := n.perm[c] + zi
				v1 := lerp(fx, grad2(n.perm[b], d, e), grad(n.perm[d2], d-1, 0, e))
				v2 := lerp(fx, grad(n.perm[b+1], d, 0, e-1), grad(n.perm[d2+1], d-1, 0, e-1))
				out[idx] += lerp(fz, v1, v2) * inv
				idx++
			}
		}
		return
	}
	idx := 0
	inv := 1 / noiseScale
	last := int32(-1)
	var d1, d2, d3, d4 float64
	for i := 0; i < xSize; i++ {
		x := xo + float64(i)*xScale + n.xo
		xf := int32(x)
		if x < float64(xf) {
			xf--
		}
		xi := xf & 255
		x -= float64(xf)
		fx := fade(x)
		for l := 0; l < zSize; l++ {
			z := zo + float64(l)*zScale + n.zo
			zf := int32(z)
			if z < float64(zf) {
				zf--
			}
			zi := zf & 255
			z -= float64(zf)
			fz := fade(z)
			for k := 0; k < ySize; k++ {
				y := yo + float64(k)*yScale + n.yo
				yf := int32(y)
				if y < float64(yf) {
					yf--
				}
				yi := yf & 255
				y -= float64(yf)
				fy := fade(y)
				if k == 0 || yi != last {
					last = yi
					k4 := n.perm[xi] + yi
					l4 := n.perm[k4] + zi
					i5 := n.perm[k4+1] + zi
					j5 := n.perm[xi+1] + yi
					k5 := n.perm[j5] + zi
					l5 := n.perm[j5+1] + zi
					d1 = lerp(fx, grad(n.perm[l4], x, y, z), grad(n.perm[k5], x-1, y, z))
					d2 = lerp(fx, grad(n.perm[i5], x, y-1, z), grad(n.perm[l5], x-1, y-1, z))
					d3 = lerp(fx, grad(n.perm[l4+1], x, y, z-1), grad(n.perm[k5+1], x-1, y, z-1))
					d4 = lerp(fx, grad(n.perm[i5+1], x, y-1, z-1), grad(n.perm[l5+1], x-1, y-1, z-1))
				}
				out[idx] += lerp(fz, lerp(fy, d1, d2), lerp(fy, d3, d4)) * inv
				idx++
			}
		}
	}
}

// octaves is NoiseGeneratorOctaves.
type octaves struct{ gens []*improvedNoise }

func newOctaves(r *Random, n int) *octaves {
	o := &octaves{gens: make([]*improvedNoise, n)}
	for i := range o.gens {
		o.gens[i] = newImprovedNoise(r)
	}
	return o
}

// generate is generateNoiseOctaves with all three axes; the array is x-major, then z, then y.
func (o *octaves) generate(out []float64, xOff, yOff, zOff, xSize, ySize, zSize int, xScale, yScale, zScale float64) []float64 {
	need := xSize * ySize * zSize
	if len(out) < need {
		out = make([]float64, need)
	} else {
		for i := range out[:need] {
			out[i] = 0
		}
	}
	d3 := 1.0
	for _, g := range o.gens {
		d0 := float64(xOff) * d3 * xScale
		d1 := float64(yOff) * d3 * yScale
		d2 := float64(zOff) * d3 * zScale
		k := floorLong(d0)
		l := floorLong(d2)
		d0 -= float64(k)
		d2 -= float64(l)
		k %= 16777216
		l %= 16777216
		d0 += float64(k)
		d2 += float64(l)
		g.populate(out, d0, d1, d2, xSize, ySize, zSize, xScale*d3, yScale*d3, zScale*d3, d3)
		d3 /= 2
	}
	return out
}

// generate2D is the two-axis form (a single layer in y): xOff, zOff, xSize, zSize.
func (o *octaves) generate2D(out []float64, xOff, zOff, xSize, zSize int, xScale, zScale float64) []float64 {
	return o.generate(out, xOff, 10, zOff, xSize, 1, zSize, xScale, 1, zScale)
}

// ---- simplex ----

var simplexGrad = [12][3]int{{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0}, {1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1}, {0, 1, 1}, {0, -1, 1}, {0, 1, -1}, {0, -1, -1}}

var (
	simplexF2 = math.Sqrt(3)*0.5 - 0.5
	simplexG2 = (3 - math.Sqrt(3)) / 6
)

type simplexNoise struct {
	perm       [512]int32
	xo, yo, zo float64
}

func newSimplexNoise(r *Random) *simplexNoise {
	n := &simplexNoise{}
	n.xo = r.Double() * 256
	n.yo = r.Double() * 256
	n.zo = r.Double() * 256
	for i := int32(0); i < 256; i++ {
		n.perm[i] = i
	}
	for l := int32(0); l < 256; l++ {
		j := r.IntN(256-l) + l
		k := n.perm[l]
		n.perm[l] = n.perm[j]
		n.perm[j] = k
		n.perm[l+256] = n.perm[l]
	}
	return n
}

func simplexFloor(d float64) int32 {
	if d > 0 {
		return int32(d)
	}
	return int32(d) - 1
}

func simplexDot(g [3]int, x, y float64) float64 { return float64(g[0])*x + float64(g[1])*y }

func (n *simplexNoise) corner(h int32, x, y float64) float64 {
	t := 0.5 - x*x - y*y
	if t < 0 {
		return 0
	}
	t *= t
	return t * t * simplexDot(simplexGrad[h], x, y)
}

// at is the 2D noise at one point.
func (n *simplexNoise) at(x, y float64) float64 {
	d3 := 0.5 * (math.Sqrt(3) - 1)
	d4 := (x + y) * d3
	i := simplexFloor(x + d4)
	j := simplexFloor(y + d4)
	d5 := (3 - math.Sqrt(3)) / 6
	d6 := float64(i+j) * d5
	d9 := x - (float64(i) - d6)
	d10 := y - (float64(j) - d6)
	var b0, b1 int32
	if d9 > d10 {
		b0, b1 = 1, 0
	} else {
		b0, b1 = 0, 1
	}
	d11 := d9 - float64(b0) + d5
	d12 := d10 - float64(b1) + d5
	d13 := d9 - 1 + 2*d5
	d14 := d10 - 1 + 2*d5
	k := i & 255
	l := j & 255
	i1 := n.perm[k+n.perm[l]] % 12
	j1 := n.perm[k+b0+n.perm[l+b1]] % 12
	k1 := n.perm[k+1+n.perm[l+1]] % 12
	return 70 * (n.corner(i1, d9, d10) + n.corner(j1, d11, d12) + n.corner(k1, d13, d14))
}

// populate adds this octave's noise over a w x h area (z-major: index z*w + x), scaled by amp.
func (n *simplexNoise) populate(out []float64, xo, zo float64, w, h int, xs, zs, amp float64) {
	idx := 0
	for j := 0; j < h; j++ {
		d0 := (zo+float64(j))*zs + n.yo
		for k := 0; k < w; k++ {
			d1 := (xo+float64(k))*xs + n.xo
			d5 := (d1 + d0) * simplexF2
			l := simplexFloor(d1 + d5)
			i1 := simplexFloor(d0 + d5)
			d6 := float64(l+i1) * simplexG2
			d9 := d1 - (float64(l) - d6)
			d10 := d0 - (float64(i1) - d6)
			var b0, b1 int32
			if d9 > d10 {
				b0, b1 = 1, 0
			} else {
				b0, b1 = 0, 1
			}
			d11 := d9 - float64(b0) + simplexG2
			d12 := d10 - float64(b1) + simplexG2
			d13 := d9 - 1 + 2*simplexG2
			d14 := d10 - 1 + 2*simplexG2
			j1 := l & 255
			k1 := i1 & 255
			l1 := n.perm[j1+n.perm[k1]] % 12
			i2 := n.perm[j1+b0+n.perm[k1+b1]] % 12
			j2 := n.perm[j1+1+n.perm[k1+1]] % 12
			out[idx] += 70 * (n.corner(l1, d9, d10) + n.corner(i2, d11, d12) + n.corner(j2, d13, d14)) * amp
			idx++
		}
	}
}

// perlin is NoiseGeneratorPerlin: octaves of simplex noise.
type perlin struct{ gens []*simplexNoise }

func newPerlin(r *Random, n int) *perlin {
	p := &perlin{gens: make([]*simplexNoise, n)}
	for i := range p.gens {
		p.gens[i] = newSimplexNoise(r)
	}
	return p
}

// at is func_151601_a: the sum of octaves at a point.
func (p *perlin) at(x, z float64) float64 {
	d0, d1 := 0.0, 1.0
	for _, g := range p.gens {
		d0 += g.at(x*d1, z*d1) / d1
		d1 /= 2
	}
	return d0
}

// area fills out (z-major, w x h) the way func_151599_a does, for the surface depth.
func (p *perlin) area(out []float64, x, z float64, w, h int, xs, zs, lacunarity float64) []float64 {
	if len(out) < w*h {
		out = make([]float64, w*h)
	} else {
		for i := range out[:w*h] {
			out[i] = 0
		}
	}
	d1, d0 := 1.0, 1.0
	for _, g := range p.gens {
		g.populate(out, x, z, w, h, xs*d0*d1, zs*d0*d1, 0.55/d0)
		d0 *= 0.5 // each octave: half the frequency, twice the weight
		d1 *= lacunarity
	}
	return out
}
