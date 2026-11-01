package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"gocraft/anvil"
)

// Mob heads, after 1.8's TileEntitySkullRenderer, ModelSkeletonHead and ModelHumanoidHead. The skull block (144)
// has no model of its own: Update Block Entity (0x35, action 4) says which head it is (SkullType: 0 skeleton,
// 1 wither skeleton, 2 zombie, 3 player, 4 creeper), how far a standing one is turned (Rot, in sixteenths of a
// turn) and, for a player's, the owner whose skin it wears (the base64 of a JSON that names a texture on
// Mojang's server, as the game's own profile properties do).

const (
	idSkull     = 144 // the block
	idSkullItem = 397 // the item; its damage is the type
)

// The head is one box of 8 pixels at the origin (the game's ModelSkeletonHead, 64 x 32); the player's and the
// zombie's have a second layer a quarter of a pixel larger (ModelHumanoidHead, 64 x 64).
var (
	skullHead  = newModel(64, 32, part(0, 0, 0, box(0, 0, -4, -8, -4, 8, 8, 8)))
	skullHuman = newModel(64, 64, part(0, 0, 0, box(0, 0, -4, -8, -4, 8, 8, 8)), part(0, 0, 0, boxX(32, 0, -4, -8, -4, 8, 8, 8, 0.25)))
)

// skullTextures is the picture of each head type; a player's is the owner's skin, or Steve's.
var skullTextures = [5]string{"skeleton/skeleton", "skeleton/wither_skeleton", "zombie/zombie", "", "creeper/creeper"}

// skullEntity is what the server says about a skull block.
type skullEntity struct {
	typ, rot int
	skin     string // the URL of a player head's skin; empty for the default one
}

// skullSkinURL is the skin a skull owner compound names: Properties.textures[0].Value is the base64 of
// {"textures":{"SKIN":{"url":"..."}}}. Only what Mojang's own domains serve is accepted.
func skullSkinURL(owner anvil.Compound) string {
	props, _ := owner["Properties"].(anvil.Compound)
	list, _ := props["textures"].([]any)
	for _, e := range list {
		c, _ := e.(anvil.Compound)
		v, _ := c["Value"].(string)
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			continue
		}
		var j struct {
			Textures struct {
				Skin struct {
					URL string `json:"url"`
				} `json:"SKIN"`
			} `json:"textures"`
		}
		if json.Unmarshal(raw, &j) == nil && skinAllowed(j.Textures.Skin.URL) {
			return j.Textures.Skin.URL
		}
	}
	return ""
}

// skullItemSkin is the skin of a skull item: its tag's SkullOwner compound (a bare name, which the game
// would look up with Mojang, gives none).
func skullItemSkin(nbt []byte) string {
	if len(nbt) == 0 {
		return ""
	}
	root, err := anvil.ReadNBT(bytes.NewReader(nbt))
	if err != nil {
		return ""
	}
	owner, _ := root["SkullOwner"].(anvil.Compound)
	return skullSkinURL(owner)
}

func num(v any) int {
	switch n := v.(type) {
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	}
	return 0
}

// netBlockEntity is Update Block Entity: only the skull (action 4) is drawn.
func (g *game) netBlockEntity(pos [3]int32, action byte, nbt []byte) {
	if action != 4 {
		return
	}
	root, err := anvil.ReadNBT(bytes.NewReader(nbt))
	if err != nil {
		return
	}
	s := &skullEntity{typ: num(root["SkullType"]), rot: num(root["Rot"]) & 15}
	if s.typ < 0 || s.typ > 4 {
		s.typ = 0
	}
	if owner, ok := root["Owner"].(anvil.Compound); ok {
		s.skin = skullSkinURL(owner)
		g.wantSkinURL(s.skin)
	}
	if g.skulls == nil {
		g.skulls = map[[3]int32]*skullEntity{}
	}
	if len(g.skulls) < 8192 {
		g.skulls[pos] = s
	}
}

// skullTexture is the texture name a head is drawn with: its type's, or a player's skin once it is there.
func (g *game) skullTexture(typ int, skin string) string {
	if typ != 3 {
		return skullTextures[typ]
	}
	if skin != "" {
		if s := g.skins[skin]; s != nil && s.state == 1 {
			return skinKey(skin)
		}
	}
	return g.defaultSkinKey(false)
}

// emitSkullModel adds a head whose model origin is at origin, scaled by scale about it, turned about the
// vertical by rot (the model's own rotation) and by the entity's yaw: the head's frame, in which the game
// flips x and y and not z.
func (g *game) emitSkullModel(groups map[string][]vertex, typ int, skin string, rots []rot, origin vec3, yawDeg float32, scale float64, light uint8) {
	m, key := skullHead, g.skullTexture(typ, skin)
	if typ == 2 || typ == 3 {
		m = skullHuman
	}
	if key == "" {
		return
	}
	pos := vec3{origin[0], origin[1] - scale*1.5078125, origin[2]}
	groups[key] = emitModelScaled(groups[key], m, rots, nil, pos, yawDeg, 0, [3]uint8{255, 255, 255}, light, scale)
}

// skullPlace is where a skull block's head goes: the origin in the block, and the model's own turn in degrees
// (TileEntitySkullRenderer.renderSkull). Standing ones turn by Rot sixteenths; on a wall they sit against it.
func skullPlace(pos [3]int32, facing, rot int) (vec3, float32) {
	x, y, z := float64(pos[0]), float64(pos[1]), float64(pos[2])
	switch facing {
	case 2: // north
		return vec3{x + 0.5, y + 0.25, z + 0.74}, 0
	case 3: // south
		return vec3{x + 0.5, y + 0.25, z + 0.26}, 180
	case 4: // west
		return vec3{x + 0.74, y + 0.25, z + 0.5}, 270
	case 5: // east
		return vec3{x + 0.26, y + 0.25, z + 0.5}, 90
	}
	return vec3{x + 0.5, y, z + 0.5}, float32(rot) * 360 / 16
}

// emitSkulls lays out the skull blocks near the camera.
func (g *game) emitSkulls(groups map[string][]vertex) {
	for pos, s := range g.skulls {
		st := g.world.Block(pos[0], pos[1], pos[2])
		if st>>4 != idSkull {
			continue
		}
		c := centre(pos[0], pos[1], pos[2])
		if d := c.sub(g.camPos); d.dot(d) > 64*64 || g.offScreen(c, 1) {
			continue
		}
		origin, deg := skullPlace(pos, int(st&7), s.rot)
		r := rot{y: deg / 57.295776}
		g.emitSkullModel(groups, s.typ, s.skin, []rot{r, r}, origin, 180, 1, g.world.Light(pos[0], pos[1], pos[2]))
	}
}

// headSkull is a skull worn on an armor stand's head: its type (the item's damage) and skin.
type headSkull struct {
	on   bool
	typ  int
	skin string
}

// emitStandHead adds the skull on a stand's head (LayerCustomHead): the model at 1.1875 of its size about the
// head's pivot, turned with the head.
func (g *game) emitStandHead(groups map[string][]vertex, s *standState, pos vec3, yaw float32, small bool, headRot rot, light uint8) {
	if !s.head.on || s.head.typ < 0 || s.head.typ > 4 {
		return
	}
	base, sc := pos, 1.0
	if small {
		base, sc = pos.add(vec3{0, childHeadShift, 0}), childHeadScale
	}
	pivot := vec3{base[0], base[1] + sc*(1.5078125-1.0/16), base[2]} // the head hangs from (0, 1, 0)
	g.emitSkullModel(groups, s.head.typ, s.head.skin, []rot{headRot, headRot}, pivot, yaw, sc*1.1875, light)
}
