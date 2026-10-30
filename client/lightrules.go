package client

// What blocks do to light, from 1.8's Block.getLightValue and getLightOpacity.

// blockLum is the light level a block gives out.
var blockLum = func() (t [256]uint8) {
	for id, l := range map[int]uint8{10: 15, 11: 15, 50: 14, 51: 15, 62: 13, 74: 9, 76: 7, 89: 15, 90: 11, 91: 15, 119: 15, 122: 1, 117: 1, 120: 1,
		124: 15, 130: 7, 138: 15, 169: 15, 198: 14} {
		t[id] = l
	}
	return
}()

// lightOf is how much light a held or dropped stack gives off, for dynamic lights.
func lightOf(s slot) uint8 {
	if s.empty() {
		return 0
	}
	if s.isBlock() {
		return blockLum[s.ID&255]
	}
	if s.ID == 327 { // a bucket of lava
		return 15
	}
	return 0
}

// opacity is how much a block dims light passing through it: 15 for a solid block, a little for
// leaves, cobwebs and water, none for air and glass.
func (r *resources) opacity(id uint16) uint8 {
	switch {
	case id == 0 || id > 255:
		return 0
	case r.opaque[id]:
		return 15
	case id == 8 || id == 9 || id == 79:
		return 3
	case id == 18 || id == 161 || id == 30:
		return 1
	}
	if lit, ok := translucentOpaque[id]; ok {
		return lit
	}
	return 0
}

// translucentOpaque are solid-looking blocks the model test does not count as full cubes but that
// still stop all light: slabs and stairs let it through, farmland and soul sand do not.
var translucentOpaque = map[uint16]uint8{60: 15, 88: 15, 110: 15, 2: 15, 3: 15, 12: 15, 13: 15, 82: 15}
