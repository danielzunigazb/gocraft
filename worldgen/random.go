// Package worldgen generates 1.8.9 terrain: the "default" world type, after ChunkProviderGenerate.
//
// Everything that decides where a block goes is a port of the game's own algorithm, including its
// random number generator (java.util.Random), so the same seed gives the same world. What is exact
// and what is approximate is listed in doc.go.
package worldgen

import "math"

// Random is java.util.Random: a 48-bit linear congruential generator.
type Random struct {
	draws        int // how many numbers were drawn, for tests
	seed         int64
	nextGaussian float64
	haveGaussian bool
}

const (
	randMult = 0x5DEECE66D
	randAdd  = 0xB
	randMask = (1 << 48) - 1
)

func NewRandom(seed int64) *Random {
	r := &Random{}
	r.SetSeed(seed)
	return r
}

func (r *Random) SetSeed(seed int64) {
	r.seed = (seed ^ randMult) & randMask
	r.haveGaussian = false
}

func (r *Random) next(bits uint) int32 {
	r.draws++
	r.seed = (r.seed*randMult + randAdd) & randMask
	return int32(uint32(uint64(r.seed) >> (48 - bits)))
}

// Int is nextInt(): any 32-bit value.
func (r *Random) Int() int32 { return r.next(32) }

// IntN is nextInt(bound), bound > 0.
func (r *Random) IntN(bound int32) int32 {
	if bound <= 0 {
		panic("worldgen: bound must be positive")
	}
	if bound&(bound-1) == 0 {
		return int32((int64(bound) * int64(r.next(31))) >> 31)
	}
	for {
		bits := r.next(31)
		val := bits % bound
		if bits-val+(bound-1) >= 0 { // int32 arithmetic: a negative sum is the overflow Java retries on
			return val
		}
	}
}

// Long is nextLong().
func (r *Random) Long() int64 { return int64(r.next(32))<<32 + int64(r.next(32)) }

func (r *Random) Bool() bool { return r.next(1) != 0 }

// Float is nextFloat().
func (r *Random) Float() float32 { return float32(r.next(24)) / float32(1<<24) }

// Double is nextDouble().
func (r *Random) Double() float64 {
	return float64(int64(r.next(26))<<27+int64(r.next(27))) * (1.0 / (1 << 53))
}

// Gaussian is nextGaussian().
func (r *Random) Gaussian() float64 {
	if r.haveGaussian {
		r.haveGaussian = false
		return r.nextGaussian
	}
	for {
		v1 := 2*r.Double() - 1
		v2 := 2*r.Double() - 1
		s := v1*v1 + v2*v2
		if s < 1 && s != 0 {
			m := math.Sqrt(-2 * math.Log(s) / s)
			r.nextGaussian, r.haveGaussian = v2*m, true
			return v1 * m
		}
	}
}

// Range is the common "nextInt(n) + lo" written out.
func (r *Random) Range(lo, n int32) int32 { return r.IntN(n) + lo }

// JavaHash is String.hashCode, which the game uses to turn a seed typed as text into a number.
func JavaHash(s string) int32 {
	var h int32
	for _, u := range utf16(s) {
		h = 31*h + int32(u)
	}
	return h
}

func utf16(s string) []uint16 {
	out := make([]uint16, 0, len(s))
	for _, r := range s {
		if r >= 0x10000 {
			r -= 0x10000
			out = append(out, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}

// MathHelper's sine table: the game does not call Math.sin but looks angles up in 65536 steps.
var sinTable [65536]float32

func init() {
	for i := range sinTable {
		sinTable[i] = float32(math.Sin(float64(i) * math.Pi * 2 / 65536))
	}
}

func mhSin(f float32) float32 { return sinTable[int32(f*10430.378)&65535] }
func mhCos(f float32) float32 { return sinTable[int32(f*10430.378+16384.0)&65535] }

func floorD(d float64) int32 {
	i := int32(d)
	if d < float64(i) {
		return i - 1
	}
	return i
}

func floorF(f float32) int32 {
	i := int32(f)
	if f < float32(i) {
		return i - 1
	}
	return i
}

func floorLong(d float64) int64 {
	i := int64(d)
	if d < float64(i) {
		return i - 1
	}
	return i
}
