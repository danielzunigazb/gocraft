package client

import "math"

// Face-sorted meshes. A quad that faces away from the camera is thrown away by the GPU after its
// vertices have been shaded, so drawing it costs the vertex work for nothing: roughly half of a
// scene's vertices. Quads are therefore stored sorted by the direction they face, and a whole group
// is skipped on the CPU when the camera is behind all of it.
//
// The direction is the quad's geometric normal from the winding of its vertices, which is what the
// GPU's own back-face test uses, so a group that is skipped is one the GPU would have discarded.

const (
	faceClasses = 7 // -X, +X, -Y, +Y, -Z, +Z, and anything not square to an axis (always drawn)
	faceOther   = 6
	faceSlack   = 0.01 // a group is only skipped when the camera is this far behind its plane
)

// faceIndex says where each group of quads is. A group is the quads of one section (16 blocks high)
// facing one way; groups are stored by direction first and by section second, so that the visible
// sections of one direction are one run.
type faceIndex struct {
	// start[k][s] is the first vertex of direction k in section s; start[k][16] ends direction k.
	start [faceClasses][17]int32
	// bound[k][s] is the extreme plane of the group on its axis: the lowest for a direction facing
	// the positive way (the camera must be above it to see any), the highest for the negative way.
	bound [faceClasses][16]float32
}

// quadFace is the direction a quad (four vertices, wound like the index buffer takes them) faces
// and the coordinate of its plane on that axis.
func quadFace(v []vertex) (class int, plane float32) {
	ax, ay, az := float64(v[1].X-v[0].X), float64(v[1].Y-v[0].Y), float64(v[1].Z-v[0].Z)
	bx, by, bz := float64(v[2].X-v[0].X), float64(v[2].Y-v[0].Y), float64(v[2].Z-v[0].Z)
	nx, ny, nz := ay*bz-az*by, az*bx-ax*bz, ax*by-ay*bx
	px, py, pz := math.Abs(nx), math.Abs(ny), math.Abs(nz)
	m := math.Max(px, math.Max(py, pz))
	if m == 0 {
		return faceOther, 0
	}
	const tol = 1e-4
	switch {
	case px == m && py <= tol*m && pz <= tol*m:
		return 0 + b2i(nx > 0), (v[0].X + v[1].X + v[2].X + v[3].X) / 4
	case py == m && px <= tol*m && pz <= tol*m:
		return 2 + b2i(ny > 0), (v[0].Y + v[1].Y + v[2].Y + v[3].Y) / 4
	case pz == m && px <= tol*m && py <= tol*m:
		return 4 + b2i(nz > 0), (v[0].Z + v[1].Z + v[2].Z + v[3].Z) / 4
	}
	return faceOther, 0
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// sortFaces reorders verts (the quads of 16 sections, section s being sec[s]..sec[s+1]) by direction
// and then by section. It writes the result into dst, which it returns grown as needed.
func sortFaces(verts []vertex, sec *[17]int32, dst []vertex, fi *faceIndex) []vertex {
	*fi = faceIndex{}
	var count [faceClasses][16]int32
	var lo, hi [faceClasses][16]float32
	for s := 0; s < 16; s++ {
		for q := sec[s]; q < sec[s+1]; q += 4 {
			k, plane := quadFace(verts[q : q+4])
			if count[k][s] == 0 {
				lo[k][s], hi[k][s] = plane, plane
			} else {
				lo[k][s], hi[k][s] = min(lo[k][s], plane), max(hi[k][s], plane)
			}
			count[k][s] += 4
		}
	}
	pos := int32(0)
	for k := 0; k < faceClasses; k++ {
		for s := 0; s < 16; s++ {
			fi.start[k][s] = pos
			pos += count[k][s]
			switch {
			case k == faceOther:
			case k&1 == 1:
				fi.bound[k][s] = lo[k][s]
			default:
				fi.bound[k][s] = hi[k][s]
			}
		}
		fi.start[k][16] = pos
	}
	if int(pos) != len(verts) {
		panic("sortFaces: quads lost")
	}
	if cap(dst) < len(verts) {
		dst = make([]vertex, len(verts))
	}
	dst = dst[:len(verts)]
	cursor := fi.start
	for s := 0; s < 16; s++ {
		for q := sec[s]; q < sec[s+1]; q += 4 {
			k, _ := quadFace(verts[q : q+4])
			copy(dst[cursor[k][s]:], verts[q:q+4])
			cursor[k][s] += 4
		}
	}
	return dst
}

// faceSections is the set of sections whose quads facing direction k can be seen from cam: the bit
// of a section is clear when it has none, or when the camera is behind all of them.
func faceSections(fi *[17]int32, bound *[16]float32, k int, cam vec3) (mask uint16) {
	for s := 0; s < 16; s++ {
		if fi[s+1] == fi[s] {
			continue
		}
		visible := true
		if k != faceOther {
			c := cam[k>>1]
			if k&1 == 1 {
				visible = c > float64(bound[s])-faceSlack
			} else {
				visible = c < float64(bound[s])+faceSlack
			}
		}
		if visible {
			mask |= 1 << s
		}
	}
	return mask
}
