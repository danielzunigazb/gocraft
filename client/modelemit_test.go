package client

import (
	"math"
	"math/rand"
	"testing"
)

// referenceEmit is the emitter as it was before the matrices: every point turned and placed on its own.
func referenceEmit(m *mmodel, rots []rot, pos vec3, yaw, tilt float32, scale float32) []vertex {
	var out []vertex
	for pi := range m.parts {
		p := &m.parts[pi]
		r := rot{x: p.rx, y: p.ry, z: p.rz}
		if pi < len(rots) {
			o := rots[pi]
			r.x, r.y, r.z, r.dx, r.dy, r.dz, r.ox = r.x+o.x, r.y+o.y, r.z+o.z, o.dx, o.dy, o.dz, o.ox
		}
		for _, bq := range m.quads[pi] {
			for _, q := range bq {
				var w [4]vec3
				for k := 0; k < 4; k++ {
					x, y, z := place(p, r, q[k])
					w[k] = toWorld(x, y, z, yaw, pos, tilt)
					w[k] = pos.add(w[k].sub(pos).scale(float64(scale)))
				}
				for k := 0; k < 4; k++ {
					out = append(out, vertex{X: float32(w[k][0]), Y: float32(w[k][1]), Z: float32(w[k][2]), U: q[k].u, V: q[k].v})
				}
			}
		}
	}
	return out
}

func TestEmitModelMatchesPerPointPlacement(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 30; trial++ {
		rots := make([]rot, 12)
		for i := range rots {
			rots[i] = rot{x: float32(rng.Float64()*3 - 1.5), y: float32(rng.Float64()*3 - 1.5), z: float32(rng.Float64() - 0.5), dy: float32(rng.Intn(3)), dz: float32(rng.Intn(3)), ox: float32(rng.Intn(2)) * 0.3}
		}
		pos := vec3{rng.Float64() * 1000, rng.Float64() * 100, rng.Float64() * 1000}
		yaw, tilt := float32(rng.Float64()*360), float32(0)
		if trial%3 == 0 {
			tilt = 1.2
		}
		scale := float32(1)
		if trial%2 == 0 {
			scale = playerScale
		}
		want := referenceEmit(playerMdl, rots, pos, yaw, tilt, scale)
		got := emitModelScaled(nil, playerMdl, rots, nil, pos, yaw, tilt, [3]uint8{255, 255, 255}, 0xF0, float64(scale))
		if len(got) != len(want) {
			t.Fatalf("%d vertices, want %d", len(got), len(want))
		}
		for i := range got {
			d := math.Max(math.Abs(float64(got[i].X-want[i].X)), math.Max(math.Abs(float64(got[i].Y-want[i].Y)), math.Abs(float64(got[i].Z-want[i].Z))))
			if d > 2e-3 { // float32 at a thousand blocks out
				t.Fatalf("trial %d vertex %d is %.5f off: %v vs %v", trial, i, d, got[i], want[i])
			}
			if got[i].U != want[i].U || got[i].V != want[i].V {
				t.Fatalf("trial %d vertex %d: texture coordinates differ", trial, i)
			}
		}
	}
}
