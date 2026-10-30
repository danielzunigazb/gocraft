package client

import (
	"math"

	"gocraft/world"
)

// vec3 is a position or a motion in doubles, as 1.8 keeps them (posX, motionX, the bounding box are all
// double); only the render turns them into float32, at the GL boundary.
type vec3 [3]float64

func (a vec3) add(b vec3) vec3      { return vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a vec3) sub(b vec3) vec3      { return vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a vec3) scale(s float64) vec3 { return vec3{a[0] * s, a[1] * s, a[2] * s} }
func (a vec3) dot(b vec3) float64   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a vec3) cross(b vec3) vec3 {
	return vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (a vec3) norm() vec3 { return a.scale(1 / math.Sqrt(a.dot(a))) }

// f32 is the vector as the GPU reads it.
func (a vec3) f32() [3]float32 { return [3]float32{float32(a[0]), float32(a[1]), float32(a[2])} }

// mat4 is column-major, as OpenGL reads it.
type mat4 [16]float32

func mul(a, b mat4) (r mat4) {
	for c := 0; c < 4; c++ {
		for row := 0; row < 4; row++ {
			for k := 0; k < 4; k++ {
				r[c*4+row] += a[k*4+row] * b[c*4+k]
			}
		}
	}
	return r
}

func perspective(fovy, aspect, near, far float32) (m mat4) {
	t := float32(1 / math.Tan(float64(fovy)/2))
	m[0], m[5] = t/aspect, t
	m[10] = (far + near) / (near - far)
	m[11] = -1
	m[14] = 2 * far * near / (near - far)
	return m
}

func lookAt(eye, centre, up vec3) mat4 {
	f := centre.sub(eye).norm()
	s := f.cross(up).norm()
	u := s.cross(f)
	return mat4{
		float32(s[0]), float32(u[0]), float32(-f[0]), 0,
		float32(s[1]), float32(u[1]), float32(-f[1]), 0,
		float32(s[2]), float32(u[2]), float32(-f[2]), 0,
		float32(-s.dot(eye)), float32(-u.dot(eye)), float32(f.dot(eye)), 1,
	}
}

// look is the unit view direction for Minecraft's angles: yaw 0 faces +Z (south) and grows to the
// right (west is 90); pitch grows downwards.
func look(yaw, pitch float32) vec3 {
	cp := math.Cos(float64(pitch))
	return vec3{
		-math.Sin(float64(yaw)) * cp,
		-math.Sin(float64(pitch)),
		math.Cos(float64(yaw)) * cp,
	}
}

// raycast walks the voxels along a ray and returns the first solid block and the empty cell
// before it, which is where a placed block goes.
func raycast(w *world.World, o, d vec3, maxDist float64) (hit, prev [3]int32, ok bool) {
	var cell, step [3]int32
	var tMax, tDelta [3]float64
	for i := 0; i < 3; i++ {
		cell[i] = int32(math.Floor(o[i]))
		switch {
		case d[i] > 0:
			step[i] = 1
			tDelta[i] = 1 / d[i]
			tMax[i] = (float64(cell[i]+1) - o[i]) / d[i]
		case d[i] < 0:
			step[i] = -1
			tDelta[i] = -1 / d[i]
			tMax[i] = (o[i] - float64(cell[i])) / -d[i]
		default:
			tMax[i] = math.Inf(1)
		}
	}
	prev = cell
	for {
		if id := w.Block(cell[0], cell[1], cell[2]) >> 4; id != 0 && !(id >= 8 && id <= 11) {
			return cell, prev, true
		}
		axis := 0
		if tMax[1] < tMax[axis] {
			axis = 1
		}
		if tMax[2] < tMax[axis] {
			axis = 2
		}
		if tMax[axis] > maxDist {
			return cell, prev, false
		}
		prev = cell
		cell[axis] += step[axis]
		tMax[axis] += tDelta[axis]
	}
}
