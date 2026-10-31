package client

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"

	"gocraft/world"
)

// Streaming and drawing of the world's columns, built after Horizon's own renderer: meshing runs on
// worker goroutines and never on the game thread, uploads to the GPU are limited per frame, a
// column is only drawn if it can be seen, and buffers are reused instead of allocated again.

type meshJob struct {
	res *resources
	w   *world.World
	cp  world.ChunkPos
	ver uint32
	// fancy is the leaves setting the job was queued with.
	fancy bool
}

type meshResult struct {
	w   *world.World
	cp  world.ChunkPos
	ver uint32
	md  *meshData
	dur time.Duration // how long the worker took to mesh it
}

type offset struct{ dx, dz int32 }

type streamer struct {
	jobs     chan meshJob
	results  chan meshResult
	inflight map[world.ChunkPos]bool
	version  map[world.ChunkPos]uint32 // bumped when a column's blocks change, to drop stale meshes
	pool     sync.Pool                 // *meshData
	local    mesher                    // for edits, meshed at once on the game thread
	order    []offset                  // columns around the camera, nearest first
	orderFor int
	started  bool
	frame    int
}

// drawStats are what the debug screen shows about the last frame.
type drawStats struct {
	drawn, culled, quads, queued, loaded int
}

func newStreamer() *streamer {
	s := &streamer{inflight: map[world.ChunkPos]bool{}, version: map[world.ChunkPos]uint32{}}
	s.pool.New = func() any { return &meshData{} }
	return s
}

// visibleOrder lists the column offsets within a view distance, nearest first.
func (s *streamer) visibleOrder(d int) []offset {
	if s.orderFor == d && s.order != nil {
		return s.order
	}
	s.order = s.order[:0]
	for dx := -d; dx <= d; dx++ {
		for dz := -d; dz <= d; dz++ {
			s.order = append(s.order, offset{int32(dx), int32(dz)})
		}
	}
	sort.SliceStable(s.order, func(i, j int) bool {
		a, b := s.order[i], s.order[j]
		return a.dx*a.dx+a.dz*a.dz < b.dx*b.dx+b.dz*b.dz
	})
	s.orderFor = d
	return s.order
}

func (g *game) startWorkers() {
	s := g.stream
	if s.started {
		return
	}
	s.started = true
	n := max(1, min(runtime.NumCPU()-1, 3))
	s.jobs = make(chan meshJob, 2*n)
	s.results = make(chan meshResult, 4*n)
	for i := 0; i < n; i++ {
		go func() {
			m := &mesher{}
			for j := range s.jobs {
				md := s.pool.Get().(*meshData)
				t0 := time.Now()
				m.mesh(j.w, j.cp, j.res, j.fancy, md)
				s.results <- meshResult{j.w, j.cp, j.ver, md, time.Since(t0)}
			}
		}()
	}
}

// upload replaces a column's GPU mesh with freshly built geometry.
func (g *game) upload(cp world.ChunkPos, md *meshData) {
	if old := g.meshes[cp]; old != nil {
		old.free()
	}
	m := uploadMesh(&g.solidArenas, md.solid, &md.solidF)
	m.minY, m.maxY = md.minY, md.maxY
	m.vis = md.vis
	if len(md.water) > 0 {
		m.water = uploadMesh(&g.waterArenas, md.water, &md.waterF)
		m.water.minY, m.water.maxY = md.minY, md.maxY
	}
	g.meshes[cp] = m
}

// updateMeshes keeps the columns around the camera meshed: it takes finished meshes from the
// workers within an upload budget, remeshes edited columns at once, queues what is missing nearest
// first, and lets go of what is far away.
func (g *game) updateMeshes() {
	s := g.stream
	g.startWorkers()
	s.frame++
	cx, cz := int32(math.Floor(float64(g.pos[0])))>>4, int32(math.Floor(float64(g.pos[2])))>>4
	d := g.viewDist()

	// Edits show on the next frame: those columns are meshed here, not queued.
	tEdit := time.Now()
	for cp := range g.dirty {
		s.version[cp]++
		s.local.mesh(g.world, cp, g.res, g.opts.Fancy, &g.edit)
		g.upload(cp, &g.edit)
		delete(g.dirty, cp)
	}
	cpuT.edit.add(since(tEdit))

	// Finished meshes: upload a few a frame, so loading never costs a long frame.
	tUp := time.Now()
	budget := tUp.Add(3 * time.Millisecond)
	for done := false; !done; {
		select {
		case r := <-s.results:
			cpuT.mesh.add(float32(r.dur.Nanoseconds()) / 1e6)
			delete(s.inflight, r.cp)
			if r.w == g.world && r.ver == s.version[r.cp] {
				g.upload(r.cp, r.md)
			}
			s.pool.Put(r.md)
			if time.Now().After(budget) && g.opt.Screenshot == "" {
				done = true
			}
		default:
			done = true
		}
	}

	cpuT.upload.add(since(tUp))

	// Queue what is missing, nearest first, until the workers have enough to do.
	tQueue := time.Now()
	queued := 0
	for _, o := range s.visibleOrder(d) {
		cp := world.ChunkPos{X: cx + o.dx, Z: cz + o.dz}
		if g.meshes[cp] != nil || s.inflight[cp] || g.net != nil && !g.net.received[cp] {
			continue // a server's column is meshed once it has arrived
		}
		select {
		case s.jobs <- meshJob{g.res, g.world, cp, s.version[cp], g.opts.Fancy}:
			s.inflight[cp] = true
		default:
			queued++ // the queue is full: the rest waits for a later frame
		}
	}
	g.stats.queued = queued + len(s.inflight)

	if s.frame%30 == 0 {
		for cp, m := range g.meshes {
			if abs(cp.X-cx) > int32(d)+2 || abs(cp.Z-cz) > int32(d)+2 {
				m.free()
				delete(g.meshes, cp)
			}
		}
	}
	if s.frame%300 == 0 && g.net == nil { // a server drops its own columns; far away leave memory; edits not yet saved stay
		g.world.Evict(func(cp world.ChunkPos) bool { return abs(cp.X-cx) <= int32(d)+4 && abs(cp.Z-cz) <= int32(d)+4 })
	}
	cpuT.queue.add(since(tQueue))
}

// waitMeshes blocks until every column in view is meshed: for screenshots and benchmarks.
func (g *game) waitMeshes() {
	cx, cz := int32(math.Floor(float64(g.pos[0])))>>4, int32(math.Floor(float64(g.pos[2])))>>4
	for tries := 0; tries < 20000; tries++ {
		g.updateMeshes()
		all := true
		for _, o := range g.stream.visibleOrder(g.viewDist()) {
			cp := world.ChunkPos{X: cx + o.dx, Z: cz + o.dz}
			if g.meshes[cp] == nil && (g.net == nil || g.net.received[cp]) { // a server sends only what it has in view
				all = false
				break
			}
		}
		if all {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// remeshAll drops every mesh, so columns are built again (after a graphics setting changes).
func (g *game) remeshAll() {
	for cp, m := range g.meshes {
		m.free()
		delete(g.meshes, cp)
		g.stream.version[cp]++
	}
}

// frustum is the six planes of the view volume, from a projection x view matrix, pointing inwards.
type frustum [6][4]float32

func newFrustum(m *mat4) (f frustum) {
	row := func(k int) [4]float32 { return [4]float32{m[k], m[4+k], m[8+k], m[12+k]} }
	r0, r1, r2, r3 := row(0), row(1), row(2), row(3)
	for i := 0; i < 4; i++ {
		f[0][i], f[1][i] = r3[i]+r0[i], r3[i]-r0[i] // left, right
		f[2][i], f[3][i] = r3[i]+r1[i], r3[i]-r1[i] // bottom, top
		f[4][i], f[5][i] = r3[i]+r2[i], r3[i]-r2[i] // near, far
	}
	return f
}

// sees reports whether a box may be inside the volume.
func (f *frustum) sees(x0, y0, z0, x1, y1, z1 float32) bool {
	for _, p := range f {
		x, y, z := x0, y0, z0 // the box corner furthest along the plane's normal
		if p[0] > 0 {
			x = x1
		}
		if p[1] > 0 {
			y = y1
		}
		if p[2] > 0 {
			z = z1
		}
		if p[0]*x+p[1]*y+p[2]*z+p[3] < 0 {
			return false
		}
	}
	return true
}

// visScratch is the cave-culling traversal's working memory, kept between frames.
type visScratch struct {
	stamp []uint32     // per section of the grid: the frame it was reached in
	masks []uint16     // per column of the grid: the sections to draw
	grid  []*chunkMesh // per column of the grid: its mesh, or nil
	queue []visNode
	frame uint32
}

type visNode struct {
	x, z int32 // column, in grid coordinates
	sy   int8
	last int8  // the direction the section was entered by; -1 for the start
	dirs uint8 // every direction travelled to get here: never turn back
}

var opposite = [6]int{dUp, dDown, dSouth, dNorth, dEast, dWest}
var dirStep = [6][3]int32{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}

// cullSections is RenderGlobal.setupTerrain: starting at the section the camera is in, it walks to
// neighbouring sections inside the view frustum, going on through a section only from the face it
// was entered by to a face its see-through blocks join, and never back the way it came. What is not
// reached cannot be seen: caves behind rock, the ground under a hill. The reached sections are left in
// vs.masks, one bit a section for each column of the grid (x, z relative to the camera column).
func (g *game) cullSections(fr *frustum, cx, cz int32, camY float32, d int) {
	vs := &g.vis
	n := 2*d + 1
	if len(vs.masks) != n*n {
		vs.masks = make([]uint16, n*n)
		vs.grid = make([]*chunkMesh, n*n)
		vs.stamp = make([]uint32, n*n*16)
		vs.frame = 0
	}
	vs.frame++
	if vs.frame == 0 { // the counter wrapped: start the stamps over
		clear(vs.stamp)
		vs.frame = 1
	}
	clear(vs.masks)
	col := func(x, z int32) int { return int(z-cz+int32(d))*n + int(x-cx+int32(d)) }
	for dz := -d; dz <= d; dz++ {
		for dx := -d; dx <= d; dx++ {
			vs.grid[col(cx+int32(dx), cz+int32(dz))] = g.meshes[world.ChunkPos{X: cx + int32(dx), Z: cz + int32(dz)}]
		}
	}
	camSy := min(max(int(math.Floor(float64(camY)))>>4, 0), 15)
	start := vs.grid[col(cx, cz)]
	if start == nil { // the camera's own column is not meshed yet: draw what is there, as a plain frustum test
		for dz := -d; dz <= d; dz++ {
			for dx := -d; dx <= d; dx++ {
				c := col(cx+int32(dx), cz+int32(dz))
				if m := vs.grid[c]; m != nil {
					x, z := float32((cx+int32(dx))*16), float32((cz+int32(dz))*16)
					if fr.sees(x, m.minY, z, x+16, m.maxY+1, z+16) {
						vs.masks[c] = 0xFFFF
					}
				}
			}
		}
		return
	}
	vs.queue = append(vs.queue[:0], visNode{cx, cz, int8(camSy), -1, 0})
	vs.stamp[col(cx, cz)*16+camSy] = vs.frame
	for head := 0; head < len(vs.queue); head++ {
		nd := vs.queue[head]
		c := col(nd.x, nd.z)
		vs.masks[c] |= 1 << uint(nd.sy)
		m := vs.grid[c]
		for dir := 0; dir < 6; dir++ {
			if nd.dirs&(1<<uint(opposite[dir])) != 0 {
				continue
			}
			if nd.last >= 0 && m.vis[nd.sy]&pairBit[opposite[nd.last]][dir] == 0 {
				continue
			}
			nx, ny, nz := nd.x+dirStep[dir][0], int32(nd.sy)+dirStep[dir][1], nd.z+dirStep[dir][2]
			if ny < 0 || ny > 15 || nx < cx-int32(d) || nx > cx+int32(d) || nz < cz-int32(d) || nz > cz+int32(d) {
				continue
			}
			nc := col(nx, nz)
			if vs.grid[nc] == nil || vs.stamp[nc*16+int(ny)] == vs.frame {
				continue
			}
			x0, y0, z0 := float32(nx*16), float32(ny*16), float32(nz*16)
			near := abs(nx-cx) <= 1 && abs(nz-cz) <= 1 && abs(int32(camSy)-ny) <= 1
			if !near && !fr.sees(x0, y0, z0, x0+16, y0+16, z0+16) {
				continue
			}
			vs.stamp[nc*16+int(ny)] = vs.frame
			vs.queue = append(vs.queue, visNode{nx, nz, int8(ny), int8(dir), nd.dirs | 1<<uint(dir)})
		}
	}
	// The sections around the camera are always drawn: no turn of the head may show a hole there.
	for dz := int32(-1); dz <= 1; dz++ {
		for dx := int32(-1); dx <= 1; dx++ {
			if x, z := cx+dx, cz+dz; abs(x-cx) <= int32(d) && abs(z-cz) <= int32(d) && vs.grid[col(x, z)] != nil {
				for sy := max(camSy-1, 0); sy <= min(camSy+1, 15); sy++ {
					vs.masks[col(x, z)] |= 1 << uint(sy)
				}
			}
		}
	}
}

// drawChunks draws every visible section: solid geometry nearest first, which saves overdraw, then
// water farthest first, so blending composes.
func (g *game) drawChunks(mvp *mat4) {
	fr := newFrustum(mvp)
	cx, cz := int32(math.Floor(float64(g.pos[0])))>>4, int32(math.Floor(float64(g.pos[2])))>>4
	d := g.viewDist()
	order := g.stream.visibleOrder(d)
	st := &g.stats
	st.drawn, st.culled, st.quads = 0, 0, 0
	t0 := time.Now()
	g.cullSections(&fr, cx, cz, float32(g.pos[1]), d)
	cpuT.cull.add(since(t0))
	t0 = time.Now()
	vs := &g.vis
	n := 2*d + 1
	vis := g.visible[:0]
	for _, o := range order {
		c := int(int32(d)+o.dz)*n + int(int32(d)+o.dx)
		m, mask := vs.grid[c], vs.masks[c]
		if m == nil {
			continue
		}
		if mask == 0 {
			st.culled++
			continue
		}
		m.mask = mask
		vis = append(vis, m)
		st.drawn++
		st.quads += int(m.queue(mask, g.camPos))
	}
	g.visible = vis
	st.loaded = len(g.meshes)
	cpuT.collect.add(since(t0))

	// The GPU's clock starts here, not at the cull: its wait for the CPU is not drawing.
	gpuWorld.begin()
	defer gpuWorld.end()
	t0 = time.Now()
	g.solidArenas.flush()
	cpuT.solid.add(since(t0))
	gpuWorld.mark()

	t0 = time.Now()
	g.drawItems()
	g.drawMobs()
	cpuT.ents.add(since(t0))
	gpuWorld.mark()

	// Water last, blended, and without writing depth so what is under it still shows.
	t0 = time.Now()
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.DepthMask(false)
	gl.Uniform1f(g.uAlpha, 0.7)
	for i := len(vis) - 1; i >= 0; i-- {
		if vis[i].water != nil {
			vis[i].water.queue(vis[i].mask, g.camPos)
		}
	}
	g.waterArenas.flush()
	gl.DepthMask(true)
	gl.Disable(gl.BLEND)
	gl.Uniform1f(g.uAlpha, 1)
	cpuT.water.add(since(t0))
}

// drawItems draws the dropped items: blocks as small cubes, other items as flat pictures.
func (g *game) drawItems() {
	if len(g.items) == 0 && (g.net == nil || len(g.net.pickups) == 0) {
		return
	}
	blocks, flat := g.buildItemGeometry(g.partial)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
	g.entBlocks.draw(blocks)
	gl.BindTexture(gl.TEXTURE_2D, g.hud.itemTex)
	g.entFlat.draw(flat)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
}
