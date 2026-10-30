package client

import (
	"fmt"
	"math"
	"os"
	"sort"

	"gocraft/world"
	"gocraft/worldgen"
)

// Nether portals and the End portal, after 1.8's BlockPortal (the frame: BlockPortal.Size),
// ItemFlintAndSteel, Entity's portal counter and Teleporter (finding or making the portal at the
// other end). A frame is an obsidian rectangle 4 to 23 wide and 5 to 23 tall; lighting it fills the
// inside with portal blocks, and standing in them for four seconds (a moment in creative) travels.

const (
	idObsidian  = 49
	idFire      = 51
	idPortal    = 90
	idEndPortal = 119
	idEndFrame  = 120

	portalWait     = 80 // ticks in a portal before survival travels
	portalCooldown = 10 // ticks after arriving during which the portal does not take the player again
)

// portalFree is BlockPortal.Size.func_150857_a: what the inside of a frame may hold.
func portalFree(id uint16) bool { return id == 0 || id == idFire || id == idPortal }

func (g *game) blockAt(x, y, z int32) uint16 { return g.world.Block(x, y, z) }

type vec3i [3]int32

func (a vec3i) add(b vec3i) vec3i               { return vec3i{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a vec3i) mul(n int32) vec3i               { return vec3i{a[0] * n, a[1] * n, a[2] * n} }
func (a vec3i) up(n int32) vec3i                { return vec3i{a[0], a[1] + n, a[2]} }
func (g *game) idAt(p vec3i) uint16             { return g.blockAt(p[0], p[1], p[2]) >> 4 }
func (g *game) idAtOff(p vec3i, d vec3i) uint16 { return g.idAt(p.add(d)) }

// portalFrame is BlockPortal.Size: the frame found around a position along one axis.
type portalFrame struct {
	axisX         bool
	along, back   vec3i // field_150863_d and field_150866_c: EAST and WEST for the x axis, NORTH and SOUTH for z
	near          vec3i // field_150861_f: the bottom corner on the "along" side
	have          bool
	width, height int
	portals       int // portal blocks already inside
}

func (f portalFrame) valid() bool {
	return f.have && f.width >= 2 && f.width <= 21 && f.height >= 3 && f.height <= 21
}

// frameRun is func_180120_a: how many blocks along dir from p are free with obsidian below them,
// counted only if obsidian ends the run.
func (g *game) frameRun(p, dir vec3i) int32 {
	i := int32(0)
	for ; i < 22; i++ {
		b := p.add(dir.mul(i))
		if !portalFree(g.idAt(b)) || g.idAt(b.up(-1)) != idObsidian {
			break
		}
	}
	if g.idAt(p.add(dir.mul(i))) == idObsidian {
		return i
	}
	return 0
}

// newFrame is the constructor of BlockPortal.Size.
func (g *game) newFrame(pos vec3i, axisX bool) portalFrame {
	f := portalFrame{axisX: axisX}
	if axisX {
		f.along, f.back = vec3i{1, 0, 0}, vec3i{-1, 0, 0}
	} else {
		f.along, f.back = vec3i{0, 0, -1}, vec3i{0, 0, 1}
	}
	p := pos
	for p[1] > pos[1]-21 && p[1] > 0 && portalFree(g.idAt(p.up(-1))) {
		p = p.up(-1)
	}
	i := g.frameRun(p, f.along) - 1
	if i >= 0 {
		f.near = p.add(f.along.mul(i))
		f.have = true
		f.width = int(g.frameRun(f.near, f.back))
		if f.width < 2 || f.width > 21 {
			f.have, f.width = false, 0
		}
	}
	if f.have {
		g.frameHeight(&f)
	}
	return f
}

// frameHeight is func_150858_a: how tall the free space is between the two side pillars, and
// whether obsidian closes it at the top.
func (g *game) frameHeight(f *portalFrame) {
	f.height = 0
outer:
	for ; f.height < 21; f.height++ {
		for i := 0; i < f.width; i++ {
			p := f.near.add(f.back.mul(int32(i))).up(int32(f.height))
			id := g.idAt(p)
			if !portalFree(id) {
				break outer
			}
			if id == idPortal {
				f.portals++
			}
			if i == 0 {
				if g.idAt(p.add(f.along)) != idObsidian {
					break outer
				}
			} else if i == f.width-1 {
				if g.idAt(p.add(f.back)) != idObsidian {
					break outer
				}
			}
		}
	}
	for j := 0; j < f.width; j++ {
		if g.idAt(f.near.add(f.back.mul(int32(j))).up(int32(f.height))) != idObsidian {
			f.height = 0
			break
		}
	}
	if f.height < 3 || f.height > 21 {
		f.have, f.width, f.height = false, 0, 0
	}
}

// fillPortal is func_150859_c: the inside of the frame becomes portal blocks, meta 1 for the x axis
// and 2 for z.
func (g *game) fillPortal(f portalFrame) {
	meta := uint16(2)
	if f.axisX {
		meta = 1
	}
	for i := 0; i < f.width; i++ {
		base := f.near.add(f.back.mul(int32(i)))
		for j := 0; j < f.height; j++ {
			p := base.up(int32(j))
			g.setBlock(p[0], p[1], p[2], idPortal<<4|meta)
		}
	}
}

// tryPortal is BlockPortal.func_176548_d: light the frame around pos if there is a whole one.
func (g *game) tryPortal(pos vec3i) bool {
	for _, axisX := range [2]bool{true, false} {
		if f := g.newFrame(pos, axisX); f.valid() && f.portals == 0 {
			g.fillPortal(f)
			return true
		}
	}
	return false
}

// portalBroken is BlockPortal.onNeighborBlockChange for a block that was just removed: the portal
// blocks next to it stay only while the frame they are in is whole and full.
func (g *game) portalBroken(pos [3]int32) {
	for _, d := range dirVec {
		n := vec3i{pos[0] + d[0], pos[1] + d[1], pos[2] + d[2]}
		if g.idAt(n) != idPortal {
			continue
		}
		axisX := g.blockAt(n[0], n[1], n[2])&15 == 1
		f := g.newFrame(n, axisX)
		if f.valid() && f.portals >= f.width*f.height {
			continue
		}
		g.clearPortal(n)
	}
}

// clearPortal removes the portal blocks connected to p.
func (g *game) clearPortal(p vec3i) {
	seen := map[vec3i]bool{p: true}
	queue := []vec3i{p}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		g.setBlock(c[0], c[1], c[2], 0)
		for _, d := range dirVec {
			n := vec3i{c[0] + d[0], c[1] + d[1], c[2] + d[2]}
			if !seen[n] && g.idAt(n) == idPortal {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
}

// ---- flint and steel ----

// tempFire is a flame the player lit that is not on a block that burns for ever: it goes out.
type tempFire struct {
	pos   vec3i
	ticks int
}

// useFlintAndSteel is ItemFlintAndSteel.onItemUse and BlockFire.onBlockAdded: a flame goes where the
// side clicked leads, and if that completes a portal frame the frame lights. It reports whether the
// click was used.
func (g *game) useFlintAndSteel(h rayHit) bool {
	if g.held().ID != 259 || g.held().empty() {
		return false
	}
	d := dirVec[h.side]
	p := vec3i{h.pos[0] + d[0], h.pos[1] + d[1], h.pos[2] + d[2]}
	if p[1] < 0 || p[1] > 255 || g.idAt(p) != 0 {
		return true
	}
	if g.dimn.id <= 0 && g.tryPortal(p) { // BlockFire.onBlockAdded: dimension > 0 is the End, where no portal lights
		g.useDurability(1)
		return true
	}
	g.setBlock(p[0], p[1], p[2], idFire<<4)
	switch g.idAt(p.up(-1)) { // netherrack and the End's bedrock burn for ever
	case 87:
	default:
		g.dimn.fires = append(g.dimn.fires, tempFire{p, 60 + rng.Intn(60)})
	}
	g.useDurability(1)
	return true
}

func (g *game) tickFires() {
	d := &g.dimn
	live := d.fires[:0]
	for _, f := range d.fires {
		if f.ticks--; f.ticks > 0 {
			live = append(live, f)
			continue
		}
		if g.idAt(f.pos) == idFire {
			g.setBlock(f.pos[0], f.pos[1], f.pos[2], 0)
		}
	}
	d.fires = live
}

// ---- standing in a portal ----

// touches reports whether any block of the given id overlaps the player's box, as
// Entity.doBlockCollisions looks: every block cell the box covers, shrunk a thousandth.
func (g *game) touches(id uint16) bool {
	b := playerBox(g.feet)
	x0, x1 := int32(math.Floor(float64(b.min[0])+0.001)), int32(math.Floor(float64(b.max[0])-0.001))
	y0, y1 := int32(math.Floor(float64(b.min[1])+0.001)), int32(math.Floor(float64(b.max[1])-0.001))
	z0, z1 := int32(math.Floor(float64(b.min[2])+0.001)), int32(math.Floor(float64(b.max[2])-0.001))
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			for z := z0; z <= z1; z++ {
				if g.blockAt(x, y, z)>>4 == id {
					return true
				}
			}
		}
	}
	return false
}

// tickPortal runs once a tick: the portal counter (Entity.setInPortal and updatePortal), the purple
// screen (EntityPlayerSP.timeInPortal) and the flames that go out.
func (g *game) tickPortal() {
	g.tickAtlasAnimation()
	if g.net != nil || !g.inGame {
		return
	}
	d := &g.dimn
	g.tickFires()
	d.prevOverlay = d.overlay
	in := g.touches(idPortal)
	if in {
		if d.cooldown > 0 {
			d.cooldown = portalCooldown // standing in it keeps the cooldown from ending
			in = false
		}
	}
	switch {
	case in:
		d.overlay = min(d.overlay+0.0125, 1)
		wait := portalWait
		if g.creative {
			wait = 1
		}
		old := d.portalTicks
		d.portalTicks++
		if old >= wait {
			d.portalTicks, d.cooldown = wait, portalCooldown
			g.travelPortal()
		}
	default:
		if d.overlay > 0 {
			d.overlay = max(d.overlay-0.05, 0)
		}
		if d.portalTicks > 0 {
			d.portalTicks = max(d.portalTicks-4, 0)
		}
	}
	if d.cooldown > 0 && !g.touches(idPortal) {
		d.cooldown--
	}
	if g.touches(idEndPortal) && d.cooldown == 0 {
		d.cooldown = portalCooldown
		g.travelEnd()
	}
}

// ---- travelling ----

// travelPortal takes the player through a Nether portal: Overworld to Nether and back, at one eighth
// the distance in the Nether.
func (g *game) travelPortal() {
	to := dimNether
	x, z := float64(g.feet[0])/8, float64(g.feet[2])/8
	if g.dimn.id == dimNether {
		to = dimOverworld
		x, z = float64(g.feet[0])*8, float64(g.feet[2])*8
	}
	g.enterDimension(to, vec3{x, g.feet[1], z})
}

// travelEnd is the End portal: from the Overworld to the End's platform; from the End back to the
// world's spawn.
func (g *game) travelEnd() {
	if g.dimn.id == dimEnd {
		g.enterDimension(dimOverworld, g.spawn)
		return
	}
	g.enterDimension(dimEnd, vec3{100.5, 50, 0.5})
}

// enterDimension changes the world being played and puts the player down: at a portal for the
// Nether and the Overworld, on the obsidian platform for the End.
func (g *game) enterDimension(to int, at vec3) {
	from := g.dimn.id
	g.switchWorld(to)
	switch {
	case to == dimEnd:
		g.endPlatform(at)
	case from == dimEnd: // out of the End: the spawn point, on the ground
		g.feet = g.freeSpot(at)
		if y, ok := g.surfaceAt(int32(math.Floor(float64(at[0]))), int32(math.Floor(float64(at[2])))); ok {
			g.feet = g.freeSpot(vec3{at[0], y, at[2]})
		}
	default:
		g.placeInPortal(at)
	}
	g.prevFeet, g.vel, g.fall = g.feet, vec3{}, 0
	g.onGround = false
	logf("entered dimension %d at %.1f %.1f %.1f", to, g.feet[0], g.feet[1], g.feet[2])
}

// endPlatform is Teleporter.placeInPortal for the End: a 5 x 5 platform of obsidian with room to
// stand, under the player.
func (g *game) endPlatform(at vec3) {
	x, y, z := int32(math.Floor(float64(at[0]))), int32(math.Floor(float64(at[1])))-1, int32(math.Floor(float64(at[2])))
	for j := int32(-2); j <= 2; j++ {
		for k := int32(-2); k <= 2; k++ {
			for l := int32(-1); l < 3; l++ {
				st := uint16(0)
				if l < 0 {
					st = idObsidian << 4
				}
				g.setBlock(x+k, y+l, z+j, st)
			}
		}
	}
	g.feet = vec3{float64(x) + 0.5, float64(y), float64(z) + 0.5}
}

// worldHeight is WorldProvider.getActualHeight: the Nether is 128 high.
func (g *game) worldHeight() int32 {
	if g.dimn.id == dimNether {
		return 128
	}
	return 256
}

// overlayChunks lists the columns the dimension has saved edits for: where a portal that is not in
// memory could be.
func (g *game) overlayChunks() []world.ChunkPos {
	o, ok := g.dimOverlayOf(g.dimn.id)
	if !ok {
		return nil
	}
	ents, err := os.ReadDir(o.dir)
	if err != nil {
		return nil
	}
	var out []world.ChunkPos
	for _, e := range ents {
		var cp world.ChunkPos
		if n, _ := fmt.Sscanf(e.Name(), "c.%d.%d.bin", &cp.X, &cp.Z); n == 2 {
			out = append(out, cp)
		}
	}
	return out
}

// placeInExistingPortal is Teleporter.placeInExistingPortal: the nearest portal within 128 blocks of
// where the player would arrive. Only columns that exist are looked at: a portal has to have been
// built, and a column nobody ever edited cannot hold one.
func (g *game) placeInExistingPortal(at vec3) bool {
	cands := map[world.ChunkPos]bool{}
	for _, cp := range g.world.LoadedPositions() {
		cands[cp] = true
	}
	for _, cp := range g.overlayChunks() {
		cands[cp] = true
	}
	cx, cz := int32(math.Floor(float64(at[0])))>>4, int32(math.Floor(float64(at[2])))>>4
	var list []world.ChunkPos
	for cp := range cands {
		if cp.X >= cx-9 && cp.X <= cx+9 && cp.Z >= cz-9 && cp.Z <= cz+9 {
			list = append(list, cp)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].X < list[j].X || list[i].X == list[j].X && list[i].Z < list[j].Z })
	best, bestD := vec3i{}, -1.0
	for _, cp := range list {
		c := g.world.Snapshot(cp)
		for y := int32(0); y < g.worldHeight(); y++ {
			for z := int32(0); z < 16; z++ {
				for x := int32(0); x < 16; x++ {
					if c.Blocks[int(y)<<8|int(z)<<4|int(x)]>>4 != idPortal {
						continue
					}
					wx, wz := cp.X*16+x, cp.Z*16+z
					if y > 0 && c.Blocks[int(y-1)<<8|int(z)<<4|int(x)]>>4 == idPortal {
						continue // not the bottom of its column
					}
					if math.Abs(float64(wx)+0.5-float64(at[0])) > 128 || math.Abs(float64(wz)+0.5-float64(at[2])) > 128 {
						continue
					}
					dx, dy, dz := float64(wx)+0.5-float64(at[0]), float64(y)+0.5-float64(at[1]), float64(wz)+0.5-float64(at[2])
					if d := dx*dx + dy*dy + dz*dz; bestD < 0 || d < bestD {
						best, bestD = vec3i{wx, y, wz}, d
					}
				}
			}
		}
	}
	if bestD < 0 {
		return false
	}
	g.feet = vec3{float64(best[0]) + 0.5, float64(best[1]), float64(best[2]) + 0.5}
	return true
}

// placeInPortal is Teleporter.placeInPortal: the existing portal, or a new one made first.
func (g *game) placeInPortal(at vec3) {
	if g.placeInExistingPortal(at) {
		return
	}
	g.makePortal(at)
	if !g.placeInExistingPortal(at) {
		g.feet = g.freeSpot(at)
	}
}

// makePortal is Teleporter.makePortal: the nearest spot within 16 blocks with room for a frame
// (a 4 x 5 frame, 3 deep, with solid ground under it), and failing that a platform of obsidian in
// the open air.
func (g *game) makePortal(at vec3) {
	const radius = 16
	d0 := -1.0
	px, py, pz := int32(math.Floor(float64(at[0]))), int32(math.Floor(float64(at[1]))), int32(math.Floor(float64(at[2])))
	bx, by, bz, bdir := px, py, pz, int32(0)
	r := worldgen.NewRandom(g.worldSeed() ^ int64(g.dimn.id)*0x9E3779B9)
	start := int32(r.IntN(4))
	height := g.worldHeight()
	solid := func(x, y, z int32) bool { return solidForPortal(g.blockAt(x, y, z) >> 4) }
	air := func(x, y, z int32) bool { return g.blockAt(x, y, z) == 0 }

	for pass := 0; pass < 2 && d0 < 0; pass++ {
		for j2 := px - radius; j2 <= px+radius; j2++ {
			d1 := float64(j2) + 0.5 - float64(at[0])
			for l2 := pz - radius; l2 <= pz+radius; l2++ {
				d2 := float64(l2) + 0.5 - float64(at[2])
			column:
				for j3 := height - 1; j3 >= 0; j3-- {
					if !air(j2, j3, l2) {
						continue
					}
					for j3 > 0 && air(j2, j3-1, l2) {
						j3--
					}
					turns := int32(4)
					if pass == 1 {
						turns = 2
					}
					for k3 := start; k3 < start+turns; k3++ {
						l3 := k3 % 2
						i4 := 1 - l3
						if pass == 0 && k3%4 >= 2 {
							l3, i4 = -l3, -i4
						}
						depth := int32(3)
						if pass == 1 {
							depth = 1
						}
						for j4 := int32(0); j4 < depth; j4++ {
							for k4 := int32(0); k4 < 4; k4++ {
								for l4 := int32(-1); l4 < 4; l4++ {
									i5 := j2 + (k4-1)*l3 + j4*i4
									j5 := j3 + l4
									k5 := l2 + (k4-1)*i4 - j4*l3
									if l4 < 0 && !solid(i5, j5, k5) || l4 >= 0 && !air(i5, j5, k5) {
										continue column
									}
								}
							}
						}
						d4 := float64(j3) + 0.5 - float64(at[1])
						if d3 := d1*d1 + d4*d4 + d2*d2; d0 < 0 || d3 < d0 {
							d0, bx, by, bz, bdir = d3, j2, j3, l2, k3%4
							if pass == 1 {
								bdir = k3 % 2
							}
						}
					}
				}
			}
		}
	}

	l6 := bdir % 2
	i7 := 1 - l6
	if bdir%4 >= 2 {
		l6, i7 = -l6, -i7
	}
	if d0 < 0 { // no room anywhere: a platform of obsidian where the portal will stand
		by = max(70, min(by, height-10))
		for j7 := int32(-1); j7 <= 1; j7++ {
			for l7 := int32(1); l7 < 3; l7++ {
				for k8 := int32(-1); k8 < 3; k8++ {
					st := uint16(0)
					if k8 < 0 {
						st = idObsidian << 4
					}
					g.setBlock(bx+(l7-1)*l6+j7*i7, by+k8, bz+(l7-1)*i7-j7*l6, st)
				}
			}
		}
	}
	meta := uint16(2)
	if l6 != 0 {
		meta = 1
	}
	for l8 := int32(0); l8 < 4; l8++ {
		for l9 := int32(-1); l9 < 4; l9++ {
			x, y, z := bx+(l8-1)*l6, by+l9, bz+(l8-1)*i7
			if l8 == 0 || l8 == 3 || l9 == -1 || l9 == 3 {
				g.setBlock(x, y, z, idObsidian<<4)
			} else {
				g.setBlock(x, y, z, idPortal<<4|meta)
			}
		}
	}
}

// solidForPortal is Material.isSolid for the blocks that can be under a new portal: anything that is
// not air, a liquid, fire or a plant.
func solidForPortal(id uint16) bool {
	switch {
	case id == 0 || isFluid(id) || id == idFire || id == idPortal:
		return false
	case id == 6 || id == 18 || id == 161 || id == 30 || id == 31 || id == 32 || id == 37 || id == 38 || id == 39 || id == 40 || id == 50 ||
		id == 55 || id == 59 || id == 65 || id == 66 || id == 69 || id == 70 || id == 72 || id == 77 || id == 78 || id == 83 || id == 106 || id == 143 || id == 171 || id == 175:
		return false // saplings, leaves, web, plants, torches, wire, rails, buttons, snow, carpet: not solid
	}
	return true
}

// demoPortal builds a Nether portal four blocks in front of the spawn and lights it; with dim -1 the
// player goes through it, which makes (or finds) the portal at the other end.
func (g *game) demoPortal(dim int) {
	g.feet = vec3{0.5, 4, 0.5}
	if y, ok := g.surfaceAt(0, 6); ok {
		g.feet[1] = y
	}
	y := int32(math.Floor(float64(g.feet[1])))
	for i := int32(0); i < 2; i++ {
		g.setBlock(i, y-1, 6, idObsidian<<4)
		g.setBlock(i, y+3, 6, idObsidian<<4)
	}
	for j := int32(0); j < 3; j++ {
		g.setBlock(-1, y+j, 6, idObsidian<<4)
		g.setBlock(2, y+j, 6, idObsidian<<4)
	}
	g.tryPortal(vec3i{0, y, 6})
	if dim == dimNether {
		g.feet = vec3{1, float64(y), 6.5}
		g.travelPortal()
	}
}
