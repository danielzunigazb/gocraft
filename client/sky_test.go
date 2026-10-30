package client

import (
	"math"
	"testing"
)

func TestCelestialAngleMatchesTheGame(t *testing.T) {
	// 0 at noon and a half at midnight; the game eases the angle, so sunset (12000) is 0.2155
	// rather than a quarter and time 0 is 0.7845.
	for time, want := range map[int64]float32{6000: 0, 12000: 0.21548, 18000: 0.5, 0: 0.78452} {
		if got := celestialAngle(time); math.Abs(float64(got-want)) > 0.003 {
			t.Errorf("time %d: angle %.4f, want %.4f", time, got, want)
		}
	}
	if celestialAngle(30000) != celestialAngle(6000) || celestialAngle(-18000) != celestialAngle(6000) {
		t.Error("the day repeats every 24000 ticks, also before time zero")
	}
}

func TestSunBrightness(t *testing.T) {
	if sunBrightness(celestialAngle(6000)) != 1 {
		t.Error("noon is full brightness")
	}
	if b := sunBrightness(celestialAngle(18000)); math.Abs(float64(b)-0.2) > 1e-5 {
		t.Errorf("midnight %.3f, want 0.2", b)
	}
	// Dusk dims smoothly: strictly decreasing between noon and midnight.
	prev := float32(2)
	for tm := int64(6000); tm <= 18000; tm += 500 {
		b := sunBrightness(celestialAngle(tm))
		if b > prev+1e-6 {
			t.Fatalf("brightness rose at %d", tm)
		}
		prev = b
	}
}

func TestSkyAndFogColoursFollowTheSun(t *testing.T) {
	noon, night := skyColour(celestialAngle(6000)), skyColour(celestialAngle(18000))
	if noon[2] != 1 || night[0] != 0 || night[1] != 0 || night[2] != 0 {
		t.Fatalf("sky at noon %v, at midnight %v", noon, night)
	}
	if f := fogColour(celestialAngle(6000)); math.Abs(float64(f[0])-0.7529412) > 1e-5 {
		t.Fatalf("noon fog %v", f)
	}
	if f := fogColour(celestialAngle(18000)); f[0] > 0.06 || f[2] < 0.08 || f[2] > 0.1 {
		t.Fatalf("night fog %v: dark with a little blue left", f)
	}
}

func TestLightmapAtNoonAndAtNight(t *testing.T) {
	var day, night [16 * 16 * 3]uint8
	buildLightmap(1, 0, 0, &day)
	buildLightmap(0.2, 0, 0, &night)
	at := func(lm *[768]uint8, sky, blk int) [3]uint8 {
		i := (sky*16 + blk) * 3
		return [3]uint8{lm[i], lm[i+1], lm[i+2]}
	}
	// Full sky light at noon is white (0.99 after the game's 0.96 + 0.03).
	if c := at(&day, 15, 0); c[0] < 250 || c[1] < 250 || c[2] < 250 {
		t.Errorf("sky 15 at noon: %v", c)
	}
	// The same cell at night is much darker, and a torch (block 15) is not.
	if c := at(&night, 15, 0); c[0] > 100 {
		t.Errorf("sky 15 at night: %v", c)
	}
	if c := at(&night, 0, 15); c[0] < 240 {
		t.Errorf("a torch at night: %v", c)
	}
	// No light at all is nearly black: 0.03 of full.
	if c := at(&day, 0, 0); c[0] > 10 {
		t.Errorf("no light: %v", c)
	}
	// Torch light is warm: red above green above blue.
	if c := at(&night, 0, 8); !(c[0] > c[1] && c[1] > c[2]) {
		t.Errorf("torch light should be warm: %v", c)
	}
	// Brightening (gamma) lifts the dark parts and leaves the bright ones.
	var bright [768]uint8
	buildLightmap(1, 1, 0, &bright)
	if at(&bright, 4, 0)[0] <= at(&day, 4, 0)[0] || at(&bright, 15, 0)[0] < at(&day, 15, 0)[0]-2 {
		t.Errorf("gamma: dark %v -> %v, bright %v -> %v", at(&day, 4, 0), at(&bright, 4, 0), at(&day, 15, 0), at(&bright, 15, 0))
	}
	// Light levels are monotonic in each direction.
	for s := 1; s < 16; s++ {
		if at(&day, s, 0)[0] < at(&day, s-1, 0)[0] {
			t.Fatalf("sky light %d is darker than %d", s, s-1)
		}
	}
}

func TestBrightnessTableMatchesTheGame(t *testing.T) {
	if brightnessTable[15] != 1 || brightnessTable[0] != 0 {
		t.Fatalf("ends %v %v", brightnessTable[0], brightnessTable[15])
	}
	if d := brightnessTable[14] - 0.7778; d > 0.001 || d < -0.001 { // (1-f)/(3f+1) with f = 1/15
		t.Fatalf("level 14: %v", brightnessTable[14])
	}
}
