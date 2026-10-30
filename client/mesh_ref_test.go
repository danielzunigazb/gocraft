package client

// The mesher before the optimisation, kept as the reference the fast one is compared against.

import "gocraft/world"

const vertexFloats = 8 // x y z u v r g b

// Blocks whose faces are hidden by a neighbour of the same kind, even though they are not opaque.
func refJoinsSame(id uint16) bool { return id == 20 || id == 95 || id == 79 || id == 165 }

type refMeshData struct {
	solid []float32
	water []float32 // drawn after everything else, blended
}

// The reference had no light in its colours any more than the vertex colours do now: the light rides in the alpha.
func refLightFactor(l uint8) float32 { return 1 }

func refEmit(out []float32, a *atlas, q *quad, x, y, z int32, tint [3]float32, light float32) []float32 {
	var vs [4][vertexFloats]float32
	for i := range vs {
		u, v := a.coord(q.tile, q.uv[i][0], q.uv[i][1])
		vs[i] = [vertexFloats]float32{
			float32(x) + q.pos[i][0]/16, float32(y) + q.pos[i][1]/16, float32(z) + q.pos[i][2]/16,
			u, v, tint[0] * q.shade * light, tint[1] * q.shade * light, tint[2] * q.shade * light,
		}
	}
	for _, i := range [6]int{0, 1, 2, 0, 2, 3} {
		out = append(out, vs[i][:]...)
	}
	return out
}

// fluidQuad builds one face of a fluid block with the given top height (in 1/16).
// refMeshChunk builds the triangles of every visible block face in a column, in world coordinates.
func refMeshChunk(w *world.World, cp world.ChunkPos, res *resources) refMeshData {
	c := w.Snapshot(cp)
	bx, bz := cp.X*16, cp.Z*16
	get := func(x, y, z int32) uint16 {
		if !world.InRange(y) {
			return 0
		}
		if x>>4 == cp.X && z>>4 == cp.Z {
			return c.Blocks[int(y)<<8|int(z&15)<<4|int(x&15)]
		}
		return w.Block(x, y, z)
	}

	lightOf := func(x, y, z int32) uint8 {
		if !world.InRange(y) {
			return world.FullLight
		}
		if x>>4 == cp.X && z>>4 == cp.Z {
			return c.LightAt(x, y, z)
		}
		return w.Light(x, y, z)
	}

	var out refMeshData
	for y := int32(0); y < 256; y++ {
		for z := int32(0); z < 16; z++ {
			for x := int32(0); x < 16; x++ {
				state := c.Blocks[int(y)<<8|int(z)<<4|int(x)]
				id, meta := state>>4, int(state&15)
				if id == 0 {
					continue
				}
				wx, wz := bx+x, bz+z
				biome := c.BiomeAt(wx, wz)
				stAt := func(dx, dy, dz int32) uint16 { return get(wx+dx, y+dy, wz+dz) }
				idAt := func(dx, dy, dz int32) uint16 { return stAt(dx, dy, dz) >> 4 }

				if isFluid(id) {
					out = refMeshFluid(out, res, id, wx, y, wz, idAt, lightOf)
					continue
				}
				file, props := res.stateOf(id, meta, stAt)
				vs := res.pick(file, props)
				if len(vs) == 0 {
					if file == "" {
						continue // no model: chests, signs, beds and the like are not drawn yet
					}
					continue
				}
				v := vs[int(uint32(wx)*3129871^uint32(wz)*116129781^uint32(y))%len(vs)]
				for i := range v.quads {
					q := &v.quads[i]
					if q.cull >= 0 {
						d := dirVec[q.cull]
						n := idAt(d[0], d[1], d[2])
						if res.opaque[n&255] || (n == id && refJoinsSame(id)) {
							continue
						}
					}
					tint := white
					if q.tint >= 0 {
						tint = res.tintFor(id, meta, biome)
					}
					// A face is lit by the air beside it; a block's own cell is dark when it is solid.
					d := dirVec[q.dir]
					l := max(refLightFactor(lightOf(wx, y, wz)), refLightFactor(lightOf(wx+d[0], y+d[1], wz+d[2])))
					out.solid = refEmit(out.solid, res.atlas, q, wx, y, wz, tint, l)
				}
			}
		}
	}
	return out
}

func refMeshFluid(out refMeshData, res *resources, id uint16, x, y, z int32, idAt func(dx, dy, dz int32) uint16, lightOf func(x, y, z int32) uint8) refMeshData {
	water := id == 8 || id == 9
	class := func(n uint16) bool { return (n == 8 || n == 9) == water && isFluid(n) }
	tile, tint := res.lava, white
	if water {
		tile, tint = res.water, tintWater
	}
	top := float32(14)
	if class(idAt(0, 1, 0)) {
		top = 16
	}
	for d := 0; d < 6; d++ {
		v := dirVec[d]
		n := idAt(v[0], v[1], v[2])
		if class(n) || res.opaque[n&255] {
			continue
		}
		q := fluidQuad(d, tile, top)
		l := refLightFactor(lightOf(x+v[0], y+v[1], z+v[2]))
		if d >= dNorth {
			q.uv = [4][2]float32{{0, 0}, {0, top}, {16, top}, {16, 0}}
		}
		if water {
			out.water = refEmit(out.water, res.atlas, &q, x, y, z, tint, l)
		} else {
			out.solid = refEmit(out.solid, res.atlas, &q, x, y, z, tint, l)
		}
	}
	return out
}
