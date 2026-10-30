package client

import (
	"container/heap"
	"math"
)

// Path finding for mobs that hunt: an A* search over the blocks a mob can stand in, after what
// PathFinder and PathNavigateGround do in 1.8: walk on level ground, step up one block, drop up to
// three, avoid water, lava, fire and cactus, and give up after a number of nodes with the path to the
// closest point reached.

type node struct{ x, y, z int32 }

const (
	pathMaxNodes = 400
	pathRange    = 24 // blocks from the mob that the search stays within
)

type pathItem struct {
	n    node
	f, g float32
	idx  int
}

type pathHeap []*pathItem

func (h pathHeap) Len() int           { return len(h) }
func (h pathHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h pathHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i]; h[i].idx = i; h[j].idx = j }
func (h *pathHeap) Push(x any)        { it := x.(*pathItem); it.idx = len(*h); *h = append(*h, it) }
func (h *pathHeap) Pop() any          { o := *h; it := o[len(o)-1]; *h = o[:len(o)-1]; return it }

// pathSearch caches what is known of the blocks during one search.
type pathSearch struct {
	g     *game
	w, h  float32
	free  map[node]bool
	stand map[node]bool
}

func (s *pathSearch) cellFree(n node) bool {
	if v, ok := s.free[n]; ok {
		return v
	}
	hw, hh := float64(s.w/2), float64(s.h)
	box := aabb{vec3{float64(n.x) + 0.5 - hw + 0.02, float64(n.y) + 0.02, float64(n.z) + 0.5 - hw + 0.02},
		vec3{float64(n.x) + 0.5 + hw - 0.02, float64(n.y) + hh - 0.02, float64(n.z) + 0.5 + hw - 0.02}}
	v := !s.g.anyBlock(box)
	if v { // the blocks it would stand in must not hurt or drown it
		for dy := int32(0); dy < int32(math.Ceil(float64(s.h))); dy++ {
			id := s.g.world.Block(n.x, n.y+dy, n.z) >> 4
			if isFluid(id) || id == 51 || id == 81 || id == 30 {
				v = false
			}
		}
	}
	s.free[n] = v
	return v
}

// solidBelow is whether there is something to stand on under the cell.
func (s *pathSearch) solidBelow(n node) bool {
	box := aabb{vec3{float64(n.x) + 0.3, float64(n.y) - 0.1, float64(n.z) + 0.3}, vec3{float64(n.x) + 0.7, float64(n.y), float64(n.z) + 0.7}}
	if !s.g.anyBlock(box) {
		return false
	}
	id := s.g.world.Block(n.x, n.y-1, n.z) >> 4
	return !isFluid(id) && id != 81 && id != 51
}

func (s *pathSearch) standable(n node) bool {
	if v, ok := s.stand[n]; ok {
		return v
	}
	v := s.cellFree(n) && s.solidBelow(n)
	s.stand[n] = v
	return v
}

// neighbours are the cells a mob can walk to from n, with what each step costs.
func (s *pathSearch) neighbours(n node, out []node, cost []float32) ([]node, []float32) {
	out, cost = out[:0], cost[:0]
	hi := int32(math.Ceil(float64(s.h)))
	for _, d := range [8][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		diag := d[0] != 0 && d[1] != 0
		c := float32(1)
		if diag {
			c = 1.414
			// a diagonal needs both of the cells it cuts across
			if !s.cellFree(node{n.x + d[0], n.y, n.z}) || !s.cellFree(node{n.x, n.y, n.z + d[1]}) {
				continue
			}
		}
		t := node{n.x + d[0], n.y, n.z + d[1]}
		switch {
		case s.standable(t):
			out, cost = append(out, t), append(cost, c)
		case !diag && s.cellFree(node{n.x, n.y + hi, n.z}) && s.standable(node{t.x, t.y + 1, t.z}): // a step up, with room to jump
			out, cost = append(out, node{t.x, t.y + 1, t.z}), append(cost, c+0.5)
		case s.cellFree(t): // a drop of up to three blocks
			for dy := int32(1); dy <= 3; dy++ {
				b := node{t.x, t.y - dy, t.z}
				if s.standable(b) {
					out, cost = append(out, b), append(cost, c+0.3*float32(dy))
					break
				}
				if !s.cellFree(b) {
					break
				}
			}
		}
	}
	return out, cost
}

func dist3(a, b node) float32 {
	dx, dy, dz := float64(a.x-b.x), float64(a.y-b.y), float64(a.z-b.z)
	return float32(math.Sqrt(dx*dx + dy*dy*0.5 + dz*dz))
}

// findPath searches from the cell a mob stands in to the cell it wants to reach, for a mob of the given
// size. It returns the cells to walk through (not the first), or nil when there is nowhere to go; when
// the goal is out of reach it leads to the closest cell the search got to.
func (g *game) findPath(from, to node, w, h float32) []node {
	s := &pathSearch{g: g, w: w, h: h, free: map[node]bool{}, stand: map[node]bool{}}
	open := &pathHeap{}
	best := map[node]*pathItem{}
	parent := map[node]node{}
	start := &pathItem{n: from, f: dist3(from, to)}
	heap.Push(open, start)
	best[from] = start
	closed := map[node]bool{}
	closest, closestD := from, dist3(from, to)
	var nb []node
	var cost []float32
	for expanded := 0; open.Len() > 0 && expanded < pathMaxNodes; expanded++ {
		cur := heap.Pop(open).(*pathItem)
		if closed[cur.n] {
			continue
		}
		closed[cur.n] = true
		if d := dist3(cur.n, to); d < closestD {
			closest, closestD = cur.n, d
		}
		if cur.n == to {
			closest = to
			break
		}
		nb, cost = s.neighbours(cur.n, nb, cost)
		for i, n := range nb {
			if closed[n] || abs(n.x-from.x) > pathRange || abs(n.z-from.z) > pathRange {
				continue
			}
			ng := cur.g + cost[i]
			if it, ok := best[n]; ok && it.g <= ng {
				continue
			}
			it := &pathItem{n: n, g: ng, f: ng + dist3(n, to)}
			best[n] = it
			parent[n] = cur.n
			heap.Push(open, it)
		}
	}
	if closest == from {
		return nil
	}
	var path []node
	for n := closest; n != from; n = parent[n] {
		path = append(path, n)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// goalCell is the cell to walk to for a target standing at a point: the one under it if it is in the
// air, down to four blocks.
func (g *game) goalCell(p vec3, w, h float32) node {
	s := &pathSearch{g: g, w: w, h: h, free: map[node]bool{}, stand: map[node]bool{}}
	x, y, z := g.cellAt(p[0], p[1]+0.05, p[2])
	n := node{x, y, z}
	for i := 0; i < 5; i++ {
		if s.solidBelow(n) {
			return n
		}
		n.y--
	}
	return node{x, y, z}
}

// pathSteer is where a hunting mob should head: the yaw to turn to and whether to jump. It follows a
// path made with A* and made again every second or when the target has moved on; with a clear way ahead it
// goes straight.
func (g *game) pathSteer(m *mob, goal vec3) (yaw float32, jump bool) {
	direct := faceYaw(goal[0]-m.pos[0], goal[2]-m.pos[2])
	w, h := m.dims()
	dist := float32(math.Hypot(float64(goal[0]-m.pos[0]), float64(goal[2]-m.pos[2])))
	if dist < 2 && math.Abs(float64(goal[1]-m.pos[1])) < 1.5 {
		return direct, m.collidedH && m.onGround
	}
	to := g.goalCell(goal, w, h)
	tgt := [3]int32{to.x, to.y, to.z}
	stale := m.path == nil || g.worldTime-int64(m.pathAt) > 20 || abs(m.pathGoal[0]-tgt[0])+abs(m.pathGoal[2]-tgt[2]) > 2
	if stale && g.be.pathBudget > 0 {
		g.be.pathBudget--
		x, y, z := g.cellAt(m.pos[0], m.pos[1]+0.05, m.pos[2])
		m.path = g.findPath(node{x, y, z}, to, w, h)
		m.pathAt, m.pathStep, m.pathGoal = int(g.worldTime), 0, tgt
	}
	for m.pathStep < len(m.path) {
		n := m.path[m.pathStep]
		dx, dz := float64(n.x)+0.5-m.pos[0], float64(n.z)+0.5-m.pos[2]
		if dx*dx+dz*dz < 0.16 && math.Abs(float64(n.y)-m.pos[1]) < 1.1 {
			m.pathStep++
			continue
		}
		jump = m.onGround && (float64(n.y) > m.pos[1]+0.4 || m.collidedH)
		return faceYaw(dx, dz), jump
	}
	m.path = nil
	return direct, m.collidedH && m.onGround
}
