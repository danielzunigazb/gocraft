package client

import (
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Columns do not own buffers. Their quads live in a few big vertex buffers ("arenas"), and a frame's
// visible ranges are sent to the driver in one multi-draw per arena instead of one draw (and one
// vertex array bind) per range. Space is handed out in quads by a first-fit free list.

const arenaQuads = 1 << 18 // 262,144 quads (1M vertices, 24 MB) per arena

type quadSpan struct{ start, n int32 }

type vertexArena struct {
	vao, vbo uint32
	free     []quadSpan // sorted by start, never touching: neighbours are merged
	// The draw list of the pass being built: one entry per range.
	counts []int32
	bases  []int32
	zeros  []unsafe.Pointer // the index offsets, all 0: the shared index buffer starts every draw
}

func newVertexArena() *vertexArena {
	a := &vertexArena{free: []quadSpan{{0, arenaQuads}}}
	gl.GenVertexArrays(1, &a.vao)
	gl.GenBuffers(1, &a.vbo)
	gl.BindVertexArray(a.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, a.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, arenaQuads*4*vertexBytes, nil, gl.DYNAMIC_DRAW)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, quadEBO)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, vertexBytes, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, vertexBytes, 12)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 4, gl.UNSIGNED_BYTE, true, vertexBytes, 20)
	return a
}

// alloc takes n quads, first fit, or says there is no room.
func (a *vertexArena) alloc(n int32) (int32, bool) {
	for i, f := range a.free {
		if f.n < n {
			continue
		}
		start := f.start
		if f.n == n {
			a.free = append(a.free[:i], a.free[i+1:]...)
		} else {
			a.free[i] = quadSpan{f.start + n, f.n - n}
		}
		return start, true
	}
	return 0, false
}

// release gives quads back, merging with the free space on either side.
func (a *vertexArena) release(start, n int32) {
	i := 0
	for i < len(a.free) && a.free[i].start < start {
		i++
	}
	a.free = append(a.free, quadSpan{})
	copy(a.free[i+1:], a.free[i:])
	a.free[i] = quadSpan{start, n}
	if i+1 < len(a.free) && a.free[i].start+a.free[i].n == a.free[i+1].start { // with the one after
		a.free[i].n += a.free[i+1].n
		a.free = append(a.free[:i+1], a.free[i+2:]...)
	}
	if i > 0 && a.free[i-1].start+a.free[i-1].n == a.free[i].start { // with the one before
		a.free[i-1].n += a.free[i].n
		a.free = append(a.free[:i], a.free[i+1:]...)
	}
}

// add appends the quad range [first, first+n) of this arena to the pass being built, in pieces the
// shared 16-bit index buffer covers.
func (a *vertexArena) add(first, n int32) {
	for n > 0 {
		k := min(n, maxQuadsPerDraw)
		a.counts = append(a.counts, k*6)
		a.bases = append(a.bases, first*4)
		a.zeros = append(a.zeros, nil)
		first += k
		n -= k
	}
}

// flush sends the pass's ranges to the driver in a single call and empties the list.
func (a *vertexArena) flush() {
	if len(a.counts) == 0 {
		return
	}
	gl.BindVertexArray(a.vao)
	gl.MultiDrawElementsBaseVertex(gl.TRIANGLES, &a.counts[0], gl.UNSIGNED_SHORT, &a.zeros[0], int32(len(a.counts)), &a.bases[0])
	rc.draws++
	rc.drawRanges += len(a.counts)
	a.counts, a.bases, a.zeros = a.counts[:0], a.bases[:0], a.zeros[:0]
}

// arenaSet is the arenas of one kind of geometry (solid, or water).
type arenaSet struct{ arenas []*vertexArena }

// place finds room for n quads, adding an arena when the existing ones are full.
func (s *arenaSet) place(n int32) (*vertexArena, int32) {
	for _, a := range s.arenas {
		if start, ok := a.alloc(n); ok {
			return a, start
		}
	}
	a := newVertexArena()
	s.arenas = append(s.arenas, a)
	start, _ := a.alloc(n)
	return a, start
}

// flush draws what every arena has queued.
func (s *arenaSet) flush() {
	for _, a := range s.arenas {
		a.flush()
	}
}

// chunkMesh is one column's quads (or its water's) in an arena.
type chunkMesh struct {
	arena        *vertexArena
	first, quads int32 // where it lives, in quads
	minY, maxY   float32
	water        *chunkMesh // the blended part of the same column
	// face[k][s] is the first quad of direction k in section s, relative to first; face[k][16] ends k.
	face  [faceClasses][17]int32
	bound [faceClasses][16]float32
	// vis[s] says which faces of section s a see-through path joins (see sectionVis).
	vis [16]uint16
	// mask is the sections drawn this frame, set by the visibility traversal.
	mask uint16
}

// maxQuadsPerDraw is how many quads the shared index buffer covers; bigger ranges draw in pieces.
const maxQuadsPerDraw = 16384

var quadEBO uint32 // indices 0,1,2, 0,2,3 for quad after quad, shared by every mesh

func initQuadIndices() {
	idx := make([]uint16, 0, maxQuadsPerDraw*6) // 16 bits: a draw covers at most 65536 vertices
	for q := uint16(0); q < maxQuadsPerDraw; q++ {
		b := q * 4
		idx = append(idx, b, b+1, b+2, b, b+2, b+3)
	}
	gl.GenBuffers(1, &quadEBO)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, quadEBO)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(idx)*2, gl.Ptr(idx), gl.STATIC_DRAW)
}

// uploadMesh puts sorted quads in an arena of the set.
func uploadMesh(set *arenaSet, data []vertex, fi *faceIndex) *chunkMesh {
	m := &chunkMesh{}
	for k := range m.face {
		for s := range m.face[k] {
			m.face[k][s] = fi.start[k][s] / 4
		}
	}
	m.bound = fi.bound
	m.quads = int32(len(data) / 4)
	if m.quads == 0 {
		return m
	}
	m.arena, m.first = set.place(m.quads)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.arena.vbo)
	gl.BufferSubData(gl.ARRAY_BUFFER, int(m.first)*4*vertexBytes, len(data)*vertexBytes, gl.Ptr(data))
	rc.uploads++
	rc.uploadBytes += len(data) * vertexBytes
	return m
}

func (m *chunkMesh) free() {
	if m.water != nil {
		m.water.free()
	}
	if m.arena != nil {
		m.arena.release(m.first, m.quads)
		m.arena = nil
	}
}

// sectionRuns lists, as quad ranges, the runs of visible sections of one direction: neighbouring
// visible sections go in one range, and a section with no quads does not break a run. There are at
// most 8 runs.
func sectionRuns(sec *[17]int32, mask uint16, runs *[8][2]int32) (n int) {
	a, b, open := int32(0), int32(0), false
	flush := func() {
		if open && b > a {
			runs[n] = [2]int32{a, b}
			n++
		}
		open = false
	}
	for s := 0; s < 16; s++ {
		lo, hi := sec[s], sec[s+1]
		switch {
		case mask&(1<<s) != 0:
			if !open {
				a, open = lo, true
			}
			b = hi
		case hi > lo: // a hidden section with geometry ends the run
			flush()
		}
	}
	flush()
	return n
}

// queue adds the visible quads of a column to its arena's pass: the sections the cave culling
// reached, and of those only the directions the camera is in front of. It says how many quads.
func (m *chunkMesh) queue(mask uint16, cam vec3) (quads int32) {
	if m.quads == 0 || mask == 0 {
		return 0
	}
	for k := 0; k < faceClasses; k++ {
		mk := mask & faceSections(&m.face[k], &m.bound[k], k, cam)
		if mk == 0 {
			continue
		}
		var runs [8][2]int32
		n := sectionRuns(&m.face[k], mk, &runs)
		for _, r := range runs[:n] {
			m.arena.add(m.first+r[0], r[1]-r[0])
			quads += r[1] - r[0]
		}
	}
	return quads
}
