package client

// Armour on a player: ModelBiped at two sizes (1.0 for the helmet, chestplate and boots, 0.5 for the
// leggings) with the pieces' textures, as LayerArmorBase draws them over the skin.

var armorMaterials = []string{"leather", "chainmail", "iron", "diamond", "gold"}

// armorModel is ModelBiped(size) on a 64 x 32 sheet: head, body, arms and legs (the left ones mirrored).
func armorModel(size float32) *mmodel {
	return newModel(64, 32,
		part(0, 0, 0, boxX(0, 0, -4, -8, -4, 8, 8, 8, size)),
		part(0, 0, 0, boxX(16, 16, -4, 0, -2, 8, 12, 4, size)),
		part(-5, 2, 0, boxX(40, 16, -3, -2, -2, 4, 12, 4, size)),
		part(5, 2, 0, mirrored(boxX(40, 16, -1, -2, -2, 4, 12, 4, size))),
		part(-1.9, 12, 0, boxX(0, 16, -2, 0, -2, 4, 12, 4, size)),
		part(1.9, 12, 0, mirrored(boxX(0, 16, -2, 0, -2, 4, 12, 4, size))),
	)
}

// armorModelThin is ModelSkeleton(size): the same, with the arms and legs two pixels wide, to fit a skeleton's.
func armorModelThin(size float32) *mmodel {
	return newModel(64, 32,
		part(0, 0, 0, boxX(0, 0, -4, -8, -4, 8, 8, 8, size)),
		part(0, 0, 0, boxX(16, 16, -4, 0, -2, 8, 12, 4, size)),
		part(-5, 2, 0, boxX(40, 16, -1, -2, -1, 2, 12, 2, size)),
		part(5, 2, 0, mirrored(boxX(40, 16, -1, -2, -1, 2, 12, 2, size))),
		part(-2, 12, 0, boxX(0, 16, -1, 0, -1, 2, 12, 2, size)),
		part(2, 12, 0, mirrored(boxX(0, 16, -1, 0, -1, 2, 12, 2, size))),
	)
}

var armorOuter, armorInner = armorModel(1.0), armorModel(0.5)
var armorOuterThin, armorInnerThin = armorModelThin(1.0), armorModelThin(0.5)

// armorPart says which parts of the model a piece of armour shows, in its slot: boots 0, leggings 1,
// chestplate 2, helmet 3.
var armorParts = [4][]int{
	{4, 5},    // boots: the legs
	{1, 4, 5}, // leggings: body and legs
	{1, 2, 3}, // chestplate: body and arms
	{0},       // helmet: the head
}

// armorOf is the material and kind of an armour item: 0 leather to 4 gold, and 0 helmet to 3 boots.
func armorOf(id uint16) (material, kind int, ok bool) {
	if id < 298 || id > 317 {
		return 0, 0, false
	}
	return int(id-298) / 4, int(id-298) % 4, true
}

// leatherColour is how leather armour is dyed; undyed is the game's brown.
func leatherColour(ex itemExtra) [3]uint8 {
	c := int32(0xA06540)
	if ex.hasColour {
		c = ex.colour
	}
	return [3]uint8{uint8(c >> 16), uint8(c >> 8), uint8(c)}
}

// armorMasks is which of the model's six parts each slot's piece shows.
var armorMasks = func() (m [4][6]bool) {
	for slot, parts := range armorParts {
		for _, p := range parts {
			m[slot][p] = true
		}
	}
	return
}()

// armorScale says how the armour is scaled about the entity's feet: one scale for all its parts, or (split) the
// head apart from the rest, as ModelBiped.render does for a child. dy shifts the origin to make up the
// translation that comes with a scale.
type armorScale struct {
	split          bool
	thin           bool // a skeleton's: arms and legs two pixels wide
	body, head     float64
	bodyDY, headDY float64
}

// emitArmor adds the armour a player wears, on top of their posed model. pieces go boots to helmet. The
// player's twelve poses give the armour model's six.
func emitArmor(groups map[string][]vertex, pieces [4]slot, extras [4]itemExtra, rots []rot, pos vec3, yaw, tilt float32, tint [3]uint8, light uint8, scale float64) {
	emitArmorScaled(groups, pieces, extras, rots, pos, yaw, tilt, tint, light, armorScale{body: scale})
}

func emitArmorScaled(groups map[string][]vertex, pieces [4]slot, extras [4]itemExtra, rots []rot, pos vec3, yaw, tilt float32, tint [3]uint8, light uint8, sc armorScale) {
	ar := [6]rot{rots[pHead], rots[pBody], rots[pRArm], rots[pLArm], rots[pRLeg], rots[pLLeg]}
	emit := func(key string, model *mmodel, mask []bool, t [3]uint8) {
		if !sc.split {
			groups[key] = emitModelScaled(groups[key], model, ar[:], mask, pos, yaw, tilt, t, light, sc.body)
			return
		}
		var head, rest [6]bool
		for i, on := range mask {
			if i == 0 {
				head[i] = on
			} else {
				rest[i] = on
			}
		}
		groups[key] = emitModelScaled(groups[key], model, ar[:], head[:], pos.add(vec3{0, sc.headDY, 0}), yaw, tilt, t, light, sc.head)
		groups[key] = emitModelScaled(groups[key], model, ar[:], rest[:], pos.add(vec3{0, sc.bodyDY, 0}), yaw, tilt, t, light, sc.body)
	}
	for slotN, it := range pieces {
		mat, _, ok := armorOf(it.ID)
		if !ok || it.empty() {
			continue
		}
		model, key := armorOuter, armorKeys[mat][0]
		if sc.thin {
			model = armorOuterThin
		}
		if slotN == 1 {
			model, key = armorInner, armorKeys[mat][1]
			if sc.thin {
				model = armorInnerThin
			}
		}
		mask := armorMasks[slotN][:]
		t := tint
		if mat == 0 { // leather takes its dye, and then its overlay is drawn undyed
			c := leatherColour(extras[slotN])
			t = [3]uint8{uint8(int(c[0]) * int(tint[0]) / 255), uint8(int(c[1]) * int(tint[1]) / 255), uint8(int(c[2]) * int(tint[2]) / 255)}
		}
		emit(key, model, mask, t)
		if mat == 0 {
			emit(key+"_overlay", model, mask, tint)
		}
	}
}

// armorKeys are the texture names of each material's two layers.
var armorKeys = func() (k [5][2]string) {
	for i, m := range armorMaterials {
		k[i] = [2]string{"armor/" + m + "_layer_1", "armor/" + m + "_layer_2"}
	}
	return
}()
