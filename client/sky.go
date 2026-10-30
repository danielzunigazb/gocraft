package client

import "math"

// Time of day, the sky and the lightmap, after 1.8's World, WorldProvider and
// EntityRenderer.updateLightmap. Sky light and block light are stored per block; what colour they
// end up as depends on the sun, the gamma setting and the flicker of torches, so the colours are
// worked out per frame in a small texture and the vertices only carry the two light levels.

const dayLength = 24000

// celestialAngle is WorldProvider.calculateCelestialAngle: 0 at noon, 0.25 at sunset, 0.5 at
// midnight, 0.75 at sunrise, eased as the game eases it.
func celestialAngle(time int64) float32 {
	i := time % dayLength
	if i < 0 {
		i += dayLength
	}
	f := float32(i)/dayLength - 0.25
	if f < 0 {
		f++
	}
	if f > 1 {
		f--
	}
	f1 := f
	f = 1 - float32(math.Cos(float64(f)*math.Pi)+1)/2
	return f1 + (f-f1)/3
}

// sunBrightness is World.getSunBrightness for clear weather: 1 by day down to 0.2 at night.
func sunBrightness(angle float32) float32 {
	f1 := 1 - (float32(math.Cos(float64(angle)*2*math.Pi))*2 + 0.2)
	f1 = max(0, min(1, f1))
	return (1-f1)*0.8 + 0.2
}

// dayFactor is the 0..1 curve the sky and fog colours follow.
func dayFactor(angle float32) float32 {
	return max(0, min(1, float32(math.Cos(float64(angle)*2*math.Pi))*2+0.5))
}

// skyColour is the colour overhead: the plains blue, darkened at night.
func skyColour(angle float32) [3]float32 {
	f := dayFactor(angle)
	return [3]float32{0.4745 * f, 0.6510 * f, 1.0 * f}
}

// fogColour is the colour of the horizon and of the distance fog.
func fogColour(angle float32) [3]float32 {
	f := float32(math.Cos(float64(angle)*2*math.Pi))*2 + 0.5
	f = max(0, min(1, f))
	return [3]float32{0.7529412 * (f*0.94 + 0.06), 0.84705883 * (f*0.94 + 0.06), 1.0 * (f*0.91 + 0.09)}
}

// brightnessTable is the Overworld's light curve: level 0 is dark, 15 is full.
var brightnessTable = func() (t [16]float32) {
	for i := range t {
		f := 1 - float32(i)/15
		t[i] = (1 - f) / (f*3 + 1)
	}
	return
}()

// buildLightmap fills a 16 x 16 table: block light along x, sky light along y, as RGB bytes.
func buildLightmap(sun, gamma, flicker float32, out *[16 * 16 * 3]uint8) {
	for sky := 0; sky < 16; sky++ {
		for blk := 0; blk < 16; blk++ {
			f2 := brightnessTable[sky] * (sun*0.95 + 0.05)
			f3 := brightnessTable[blk] * (flicker*0.1 + 1.5)
			f4 := f2 * (sun*0.65 + 0.35)
			f6 := f3 * ((f3*0.6+0.4)*0.6 + 0.4)
			f7 := f3 * (f3*f3*0.6 + 0.4)
			c := [3]float32{f4 + f3, f4 + f6, f2 + f7}
			for k := range c {
				v := c[k]*0.96 + 0.03
				v = max(0, min(1, v))
				inv := 1 - v
				v = v*(1-gamma) + (1-inv*inv*inv*inv)*gamma // the game's gamma curve
				out[(sky*16+blk)*3+k] = toByte(max(0, min(1, v)))
			}
		}
	}
}

// lightColour is what one light level pair looks like with the current sun, for geometry made on the CPU.
func lightColour(sky, blk uint8, lm *[16 * 16 * 3]uint8) [3]float32 {
	i := (int(sky&15)*16 + int(blk&15)) * 3
	return [3]float32{float32(lm[i]) / 255, float32(lm[i+1]) / 255, float32(lm[i+2]) / 255}
}
