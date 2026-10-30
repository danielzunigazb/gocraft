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

var armorOuter, armorInner = armorModel(1.0), armorModel(0.5)

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

// armorRots turns the player's twelve poses into the armour model's six.
func armorRots(r []rot) []rot {
	return []rot{r[pHead], r[pBody], r[pRArm], r[pLArm], r[pRLeg], r[pLLeg]}
}

// emitArmor adds the armour a player wears, on top of their posed model. pieces go boots to helmet.
func emitArmor(groups map[string][]vertex, pieces [4]slot, extras [4]itemExtra, rots []rot, pos vec3, yaw, tilt float32, tint [3]uint8, light uint8) {
	ar := armorRots(rots)
	for slotN, it := range pieces {
		mat, _, ok := armorOf(it.ID)
		if !ok || it.empty() {
			continue
		}
		model, layer := armorOuter, "1"
		if slotN == 1 {
			model, layer = armorInner, "2"
		}
		mask := make([]bool, 6)
		for _, i := range armorParts[slotN] {
			mask[i] = true
		}
		key := "armor/" + armorMaterials[mat] + "_layer_" + layer
		t := tint
		if mat == 0 { // leather takes its dye, and then its overlay is drawn undyed
			c := leatherColour(extras[slotN])
			t = [3]uint8{uint8(int(c[0]) * int(tint[0]) / 255), uint8(int(c[1]) * int(tint[1]) / 255), uint8(int(c[2]) * int(tint[2]) / 255)}
		}
		groups[key] = emitModelParts(groups[key], model, ar, mask, pos, yaw, tilt, t, light)
		if mat == 0 {
			ov := key + "_overlay"
			groups[ov] = emitModelParts(groups[ov], model, ar, mask, pos, yaw, tilt, tint, light)
		}
	}
}
