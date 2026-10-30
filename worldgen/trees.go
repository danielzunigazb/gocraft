package worldgen

import "math"

// Trees, after the WorldGen*Tree classes. A tree is chosen by the biome (genBigTreeChance), which
// draws random numbers of its own, then grown at the surface block of a random column.

func (c *popCtx) tree(x, y, z int) {
	r := c.rnd
	b := c.bio
	switch b.Class {
	case clsForest:
		switch {
		case b.Forest == 3:
			if r.IntN(3) > 0 {
				c.canopyTree(x, y, z)
				return
			}
			if r.IntN(5) != 0 {
				c.oakTree(x, y, z, 4, 0, 0, false)
			} else {
				c.birchTree(x, y, z, false)
			}
		case b.Forest != 2 && r.IntN(5) != 0:
			c.oakTree(x, y, z, 4, 0, 0, false)
		default:
			c.birchTree(x, y, z, false)
		}
	case clsTaiga:
		if b.TaigaType != 0 {
			if r.IntN(3) == 0 {
				c.spruceTall(x, y, z)
			} else {
				c.spruceTree(x, y, z)
			}
			return
		}
		if r.IntN(3) == 0 {
			c.spruceTall(x, y, z)
		} else {
			c.spruceTree(x, y, z)
		}
	case clsSwamp:
		c.swampTree(x, y, z)
	case clsHills:
		if r.IntN(3) > 0 {
			c.spruceTall(x, y, z)
		} else if r.IntN(10) == 0 {
			c.bigOak(x, y, z)
		} else {
			c.oakTree(x, y, z, 4, 0, 0, false)
		}
	case clsSavanna:
		if r.IntN(5) > 0 {
			c.acacia(x, y, z)
		} else {
			c.oakTree(x, y, z, 4, 0, 0, false)
		}
	case clsJungle:
		switch {
		case r.IntN(10) == 0:
			c.bigOak(x, y, z)
		case r.IntN(2) == 0:
			c.shrub(x, y, z)
		case r.IntN(3) == 0:
			c.oakTree(x, y, z, 10+int(r.IntN(20)), 3, 3, true) // a tall one in place of the 2 x 2 mega tree
		default:
			c.oakTree(x, y, z, 4+int(r.IntN(7)), 3, 3, true)
		}
	default:
		if r.IntN(10) == 0 {
			c.bigOak(x, y, z)
		} else {
			c.oakTree(x, y, z, 4, 0, 0, false)
		}
	}
}

func treeBlocks(id uint16) bool {
	return id == 0 || isLeavesID(id) || id == 2 || id == 3 || isLogID(id) || id == 6 || id == 106
}

func (c *popCtx) setDirt(x, y, z int) {
	if c.z.id(x, y, z) != 3 {
		c.z.set(x, y, z, stDirt)
	}
}

// oakTree is WorldGenTrees: minimum height, wood and leaf variants, and vines for the jungle.
func (c *popCtx) oakTree(x, y, z, minH int, wood, leaf uint16, vines bool) bool {
	r := c.rnd
	i := int(r.IntN(3)) + minH
	if y < 1 || y+i+1 > 256 {
		return false
	}
	for j := y; j <= y+1+i; j++ {
		k := 1
		if j == y {
			k = 0
		}
		if j >= y+1+i-2 {
			k = 2
		}
		for l := x - k; l <= x+k; l++ {
			for i1 := z - k; i1 <= z+k; i1++ {
				if j >= 0 && j < 256 {
					if !treeBlocks(c.z.id(l, j, i1)) {
						return false
					}
				} else {
					return false
				}
			}
		}
	}
	if below := c.z.id(x, y-1, z); !(below == 2 || below == 3 || below == 60) || y >= 256-i-1 {
		return false
	}
	c.setDirt(x, y-1, z)
	for i3 := y - 3 + i; i3 <= y+i; i3++ {
		i4 := i3 - (y + i)
		j1 := 1 - i4/2
		for k1 := x - j1; k1 <= x+j1; k1++ {
			l1 := k1 - x
			for i2 := z - j1; i2 <= z+j1; i2++ {
				j2 := i2 - z
				if abs(l1) != j1 || abs(j2) != j1 || r.IntN(2) != 0 && i4 != 0 {
					id := c.z.id(k1, i3, i2)
					if id == 0 || isLeavesID(id) || id == 106 {
						c.z.set(k1, i3, i2, stLeaves|leaf)
					}
				}
			}
		}
	}
	for j3 := 0; j3 < i; j3++ {
		id := c.z.id(x, y+j3, z)
		if id == 0 || isLeavesID(id) || id == 106 {
			c.z.set(x, y+j3, z, stLog|wood)
		}
	}
	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// birchTree is WorldGenForest.
func (c *popCtx) birchTree(x, y, z int, tall bool) bool {
	r := c.rnd
	i := int(r.IntN(3)) + 5
	if tall {
		i += int(r.IntN(7))
	}
	if y < 1 || y+i+1 > 256 {
		return false
	}
	for j := y; j <= y+1+i; j++ {
		b0 := 1
		if j == y {
			b0 = 0
		}
		if j >= y+1+i-2 {
			b0 = 2
		}
		for k := x - b0; k <= x+b0; k++ {
			for l := z - b0; l <= z+b0; l++ {
				if j >= 0 && j < 256 {
					if !treeBlocks(c.z.id(k, j, l)) {
						return false
					}
				} else {
					return false
				}
			}
		}
	}
	if below := c.z.id(x, y-1, z); !(below == 2 || below == 3 || below == 60) || y >= 256-i-1 {
		return false
	}
	c.setDirt(x, y-1, z)
	for i2 := y - 3 + i; i2 <= y+i; i2++ {
		k2 := i2 - (y + i)
		l2 := 1 - k2/2
		for i3 := x - l2; i3 <= x+l2; i3++ {
			j1 := i3 - x
			for k1 := z - l2; k1 <= z+l2; k1++ {
				l1 := k1 - z
				if abs(j1) != l2 || abs(l1) != l2 || r.IntN(2) != 0 && k2 != 0 {
					if id := c.z.id(i3, i2, k1); id == 0 || isLeavesID(id) {
						c.z.set(i3, i2, k1, stLeaves|2)
					}
				}
			}
		}
	}
	for j2 := 0; j2 < i; j2++ {
		if id := c.z.id(x, y+j2, z); id == 0 || isLeavesID(id) {
			c.z.set(x, y+j2, z, stLog|2)
		}
	}
	return true
}

// spruceTree is WorldGenTaiga1 (the pointed one), spruceTall WorldGenTaiga2.
func (c *popCtx) spruceTree(x, y, z int) bool {
	r := c.rnd
	i := int(r.IntN(5)) + 7
	j := i - int(r.IntN(2)) - 3
	k := i - j
	l := 1 + int(r.IntN(int32(k+1)))
	if y < 1 || y+i+1 > 256 {
		return false
	}
	for i1 := y; i1 <= y+1+i; i1++ {
		j1 := l
		if i1-y < j {
			j1 = 0
		}
		for k1 := x - j1; k1 <= x+j1; k1++ {
			for l1 := z - j1; l1 <= z+j1; l1++ {
				if i1 >= 0 && i1 < 256 {
					if id := c.z.id(k1, i1, l1); id != 0 && !isLeavesID(id) {
						return false
					}
				} else {
					return false
				}
			}
		}
	}
	if below := c.z.id(x, y-1, z); !(below == 2 || below == 3 || below == 60) || y >= 256-i-1 {
		return false
	}
	c.setDirt(x, y-1, z)
	k2 := 0
	for l2 := y + i; l2 >= y+j; l2-- {
		for j3 := x - k2; j3 <= x+k2; j3++ {
			k3 := j3 - x
			for i2 := z - k2; i2 <= z+k2; i2++ {
				j2 := i2 - z
				if abs(k3) != k2 || abs(j2) != k2 || k2 <= 0 {
					if !fullBlockID(c.z.id(j3, l2, i2)) {
						c.z.set(j3, l2, i2, stLeaves|1)
					}
				}
			}
		}
		if k2 >= 1 && l2 == y+j+1 {
			k2--
		} else if k2 < l {
			k2++
		}
	}
	for i3 := 0; i3 < i-1; i3++ {
		if id := c.z.id(x, y+i3, z); id == 0 || isLeavesID(id) {
			c.z.set(x, y+i3, z, stLog|1)
		}
	}
	return true
}

func (c *popCtx) spruceTall(x, y, z int) bool {
	r := c.rnd
	i := int(r.IntN(4)) + 6
	j := 1 + int(r.IntN(2))
	k := i - j
	l := 2 + int(r.IntN(2))
	if y < 1 || y+i+1 > 256 {
		return false
	}
	for i1 := y; i1 <= y+1+i; i1++ {
		j1 := l
		if i1-y < j {
			j1 = 0
		}
		for k1 := x - j1; k1 <= x+j1; k1++ {
			for l1 := z - j1; l1 <= z+j1; l1++ {
				if i1 >= 0 && i1 < 256 {
					if id := c.z.id(k1, i1, l1); id != 0 && !isLeavesID(id) {
						return false
					}
				} else {
					return false
				}
			}
		}
	}
	if below := c.z.id(x, y-1, z); !(below == 2 || below == 3 || below == 60) || y >= 256-i-1 {
		return false
	}
	c.setDirt(x, y-1, z)
	k2 := int(r.IntN(2))
	l2 := 1
	i3 := 0
	for j3 := 0; j3 <= k; j3++ {
		k3 := y + i - j3
		for l3 := x - k2; l3 <= x+k2; l3++ {
			i4 := l3 - x
			for j4 := z - k2; j4 <= z+k2; j4++ {
				j2 := j4 - z
				if abs(i4) != k2 || abs(j2) != k2 || k2 <= 0 {
					if !fullBlockID(c.z.id(l3, k3, j4)) {
						c.z.set(l3, k3, j4, stLeaves|1)
					}
				}
			}
		}
		if k2 >= l2 {
			k2 = i3
			i3 = 1
			l2++
			if l2 > l {
				l2 = l
			}
		} else {
			k2++
		}
	}
	j3 := int(r.IntN(3))
	for k4 := 0; k4 < i-j3; k4++ {
		if id := c.z.id(x, y+k4, z); id == 0 || isLeavesID(id) {
			c.z.set(x, y+k4, z, stLog|1)
		}
	}
	return true
}

// swampTree is WorldGenSwamp: an oak that wades into water, hung with vines.
func (c *popCtx) swampTree(x, y, z int) bool {
	r := c.rnd
	i := int(r.IntN(4)) + 5
	for isWaterID(c.z.id(x, y-1, z)) {
		y--
	}
	if y < 1 || y+i+1 > 256 {
		return false
	}
	for j := y; j <= y+1+i; j++ {
		k := 1
		if j == y {
			k = 0
		}
		if j >= y+1+i-2 {
			k = 3
		}
		for l := x - k; l <= x+k; l++ {
			for i1 := z - k; i1 <= z+k; i1++ {
				if j >= 0 && j < 256 {
					id := c.z.id(l, j, i1)
					if id != 0 && !isLeavesID(id) {
						if !isWaterID(id) || j > y {
							return false
						}
					}
				} else {
					return false
				}
			}
		}
	}
	if below := c.z.id(x, y-1, z); !(below == 2 || below == 3) || y >= 256-i-1 {
		return false
	}
	c.setDirt(x, y-1, z)
	for k1 := y - 3 + i; k1 <= y+i; k1++ {
		j2 := k1 - (y + i)
		l2 := 2 - j2/2
		for j3 := x - l2; j3 <= x+l2; j3++ {
			k3 := j3 - x
			for i4 := z - l2; i4 <= z+l2; i4++ {
				j4 := i4 - z
				if abs(k3) != l2 || abs(j4) != l2 || r.IntN(2) != 0 && j2 != 0 {
					if !fullBlockID(c.z.id(j3, k1, i4)) {
						c.z.set(j3, k1, i4, stLeaves)
					}
				}
			}
		}
	}
	for l1 := 0; l1 < i; l1++ {
		if id := c.z.id(x, y+l1, z); id == 0 || isLeavesID(id) || isWaterID(id) {
			c.z.set(x, y+l1, z, stLog)
		}
	}
	for i2 := y - 3 + i; i2 <= y+i; i2++ {
		k2 := i2 - (y + i)
		i3 := 2 - k2/2
		for px := x - i3; px <= x+i3; px++ {
			for pz := z - i3; pz <= z+i3; pz++ {
				if !isLeavesID(c.z.id(px, i2, pz)) {
					continue
				}
				for _, d := range [4]struct {
					dx, dz int
					vine   uint16
				}{{-1, 0, 8}, {1, 0, 2}, {0, -1, 1}, {0, 1, 4}} {
					if r.IntN(4) == 0 && c.z.isAir(px+d.dx, i2, pz+d.dz) {
						c.vine(px+d.dx, i2, pz+d.dz, d.vine)
					}
				}
			}
		}
	}
	return true
}

func (c *popCtx) vine(x, y, z int, face uint16) {
	c.z.set(x, y, z, 106<<4|face)
	for n := 4; n > 0; n-- {
		y--
		if !c.z.isAir(x, y, z) {
			break
		}
		c.z.set(x, y, z, 106<<4|face)
	}
}

// acacia: the savanna's bent tree, approximately.
func (c *popCtx) acacia(x, y, z int) bool {
	r := c.rnd
	h := int(r.IntN(3)) + int(r.IntN(3)) + 5
	if y < 1 || y+h+1 > 256 {
		return false
	}
	if below := c.z.id(x, y-1, z); below != 2 && below != 3 {
		return false
	}
	for j := 0; j <= h+1; j++ {
		if !treeBlocks(c.z.id(x, y+j, z)) {
			return false
		}
	}
	c.setDirt(x, y-1, z)
	dir := [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}[r.IntN(4)]
	bend := h - int(r.IntN(4)) - 1
	cx, cz := x, z
	for j := 0; j < h; j++ {
		if j >= bend {
			cx += dir[0]
			cz += dir[1]
		}
		c.z.set(cx, y+j, cz, stLog2)
	}
	top := y + h - 1
	for dy := -1; dy <= 0; dy++ {
		rad := 3 - 2*(dy+1)
		for dx := -rad; dx <= rad; dx++ {
			for dz := -rad; dz <= rad; dz++ {
				if abs(dx)+abs(dz) <= rad+1 && c.z.isAir(cx+dx, top+dy+1, cz+dz) {
					c.z.set(cx+dx, top+dy+1, cz+dz, stLeaves2)
				}
			}
		}
	}
	return true
}

// shrub is the jungle's WorldGenShrub: a log with a tuft of leaves.
func (c *popCtx) shrub(x, y, z int) {
	c.rnd.IntN(1)
	if b := c.z.id(x, y-1, z); b != 2 && b != 3 {
		return
	}
	c.z.set(x, y, z, stLog|3)
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			for dy := 1; dy <= 2; dy++ {
				if abs(dx)+abs(dz) <= 3-dy+1 && c.z.isAir(x+dx, y+dy, z+dz) {
					c.z.set(x+dx, y+dy, z+dz, stLeaves|3)
				}
			}
		}
	}
}

// canopyTree is the dark oak, approximated: a 2 x 2 trunk under a flat crown.
func (c *popCtx) canopyTree(x, y, z int) bool {
	r := c.rnd
	h := int(r.IntN(3)) + int(r.IntN(2)) + 6
	if y < 1 || y+h+1 >= 256 {
		return false
	}
	for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		if b := c.z.id(x+d[0], y-1, z+d[1]); b != 2 && b != 3 {
			return false
		}
		for j := 0; j <= h; j++ {
			if !treeBlocks(c.z.id(x+d[0], y+j, z+d[1])) {
				return false
			}
		}
	}
	for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		c.setDirt(x+d[0], y-1, z+d[1])
		for j := 0; j < h; j++ {
			c.z.set(x+d[0], y+j, z+d[1], stLog2|1)
		}
	}
	for dy := 0; dy <= 2; dy++ {
		rad := 3 - dy
		for dx := -rad; dx <= rad+1; dx++ {
			for dz := -rad; dz <= rad+1; dz++ {
				if (dx < -rad+1 && dz < -rad+1) || (dx > rad && dz > rad) {
					continue
				}
				if id := c.z.id(x+dx, y+h-2+dy, z+dz); id == 0 || isLeavesID(id) {
					c.z.set(x+dx, y+h-2+dy, z+dz, stLeaves2|1)
				}
			}
		}
	}
	return true
}

// ---- the big oak ----

type foliage struct{ x, y, z, base int }

type bigTree struct {
	c            *popCtx
	r            *Random
	bx, by, bz   int
	heightLimit  int
	height       int
	leafDistance int
	nodes        []foliage
}

func (c *popCtx) bigOak(x, y, z int) bool {
	t := &bigTree{c: c, bx: x, by: y, bz: z, leafDistance: 4}
	t.r = NewRandom(c.rnd.Long())
	t.heightLimit = 5 + int(t.r.IntN(12))
	if !t.validLocation() {
		return false
	}
	t.leafNodes()
	for _, f := range t.nodes {
		for i := 0; i < t.leafDistance; i++ {
			t.leafLayer(f.x, f.y+i, f.z, t.leafSize(i))
		}
	}
	t.line(t.bx, t.by, t.bz, t.bx, t.by+t.height, t.bz, stLog)
	for _, f := range t.nodes {
		if float64(f.base-t.by) >= float64(t.heightLimit)*0.2 {
			t.line(t.bx, f.base, t.bz, f.x, f.y, f.z, stLog)
		}
	}
	return true
}

func (t *bigTree) validLocation() bool {
	if b := t.c.z.id(t.bx, t.by-1, t.bz); b != 2 && b != 3 {
		return false
	}
	i := t.check(t.bx, t.by, t.bz, t.bx, t.by+t.heightLimit-1, t.bz)
	if i == -1 {
		return true
	}
	if i < 6 {
		return false
	}
	t.heightLimit = i
	return true
}

func steps(dx, dy, dz int) int {
	a, b, c := abs(dx), abs(dy), abs(dz)
	if c > a && c > b {
		return c
	}
	if b > a {
		return b
	}
	return a
}

// check walks the line between two points and returns the step where something blocks it, or -1.
func (t *bigTree) check(x1, y1, z1, x2, y2, z2 int) int {
	dx, dy, dz := x2-x1, y2-y1, z2-z1
	n := steps(dx, dy, dz)
	if n == 0 {
		return -1
	}
	f, f1, f2 := float32(dx)/float32(n), float32(dy)/float32(n), float32(dz)/float32(n)
	for j := 0; j <= n; j++ {
		x := x1 + int(floorD(float64(0.5+float32(j)*f)))
		y := y1 + int(floorD(float64(0.5+float32(j)*f1)))
		z := z1 + int(floorD(float64(0.5+float32(j)*f2)))
		if !treeBlocks(t.c.z.id(x, y, z)) {
			return j
		}
	}
	return -1
}

func (t *bigTree) layerSize(k int) float32 {
	if float32(k) < float32(t.heightLimit)*0.3 {
		return -1
	}
	f := float32(t.heightLimit) / 2
	f1 := f - float32(k)
	f2 := float32(math.Sqrt(float64(f*f - f1*f1)))
	if f1 == 0 {
		f2 = f
	} else if float32(math.Abs(float64(f1))) >= f {
		return 0
	}
	return f2 * 0.5
}

func (t *bigTree) leafSize(i int) float32 {
	if i >= 0 && i < t.leafDistance {
		if i != 0 && i != t.leafDistance-1 {
			return 3
		}
		return 2
	}
	return -1
}

func (t *bigTree) leafNodes() {
	t.height = int(float64(t.heightLimit) * 0.618)
	if t.height >= t.heightLimit {
		t.height = t.heightLimit - 1
	}
	i := int(1.382 + math.Pow(float64(t.heightLimit)/13, 2))
	if i < 1 {
		i = 1
	}
	j := t.by + t.height
	k := t.heightLimit - t.leafDistance
	t.nodes = []foliage{{t.bx, t.by + k, t.bz, j}}
	for ; k >= 0; k-- {
		f := t.layerSize(k)
		if f < 0 {
			continue
		}
		for l := 0; l < i; l++ {
			d0 := float64(f) * (float64(t.r.Float()) + 0.328)
			d1 := float64(t.r.Float()*2) * math.Pi
			d2 := d0*math.Sin(d1) + 0.5
			d3 := d0*math.Cos(d1) + 0.5
			px := t.bx + int(floorD(d2))
			py := t.by + k - 1
			pz := t.bz + int(floorD(d3))
			if t.check(px, py, pz, px, py+t.leafDistance, pz) == -1 {
				i1, j1 := t.bx-px, t.bz-pz
				d4 := float64(py) - math.Sqrt(float64(i1*i1+j1*j1))*0.381
				k1 := int(d4)
				if d4 > float64(j) {
					k1 = j
				}
				if t.check(t.bx, k1, t.bz, px, py, pz) == -1 {
					t.nodes = append(t.nodes, foliage{px, py, pz, k1})
				}
			}
		}
	}
}

func (t *bigTree) leafLayer(x, y, z int, size float32) {
	i := int(float64(size) + 0.618)
	for j := -i; j <= i; j++ {
		for k := -i; k <= i; k++ {
			if math.Pow(math.Abs(float64(j))+0.5, 2)+math.Pow(math.Abs(float64(k))+0.5, 2) <= float64(size*size) {
				if id := t.c.z.id(x+j, y, z+k); id == 0 || isLeavesID(id) {
					t.c.z.set(x+j, y, z+k, stLeaves)
				}
			}
		}
	}
}

func (t *bigTree) line(x1, y1, z1, x2, y2, z2 int, st uint16) {
	dx, dy, dz := x2-x1, y2-y1, z2-z1
	n := steps(dx, dy, dz)
	if n == 0 {
		t.c.z.set(x1, y1, z1, st)
		return
	}
	f, f1, f2 := float32(dx)/float32(n), float32(dy)/float32(n), float32(dz)/float32(n)
	for j := 0; j <= n; j++ {
		x := x1 + int(floorD(float64(0.5+float32(j)*f)))
		y := y1 + int(floorD(float64(0.5+float32(j)*f1)))
		z := z1 + int(floorD(float64(0.5+float32(j)*f2)))
		axis := uint16(0)
		ax, az := abs(x-x1), abs(z-z1)
		if k := max(ax, az); k > 0 {
			if ax == k {
				axis = 4
			} else if az == k {
				axis = 8
			}
		}
		t.c.z.set(x, y, z, st|axis)
	}
}
