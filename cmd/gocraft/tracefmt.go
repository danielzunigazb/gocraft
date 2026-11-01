package main

import (
	"fmt"

	"gocraft/proto"
)

// decodeTrace says what is in the packets that matter when the server and the client disagree about where the
// player is or how they move: positions both ways, knockback, blows, sprint and sneak, and health.
func decodeTrace(dir string, id int32, payload []byte) string {
	r := proto.NewReader(payload)
	var s string
	switch {
	case dir == ">" && id == 0x04:
		x, y, z, g := r.Double(), r.Double(), r.Double(), r.Bool()
		s = fmt.Sprintf("pos %.3f %.3f %.3f ground=%v", x, y, z, g)
	case dir == ">" && id == 0x06:
		x, y, z := r.Double(), r.Double(), r.Double()
		yaw, pitch, g := r.Float(), r.Float(), r.Bool()
		s = fmt.Sprintf("pos %.3f %.3f %.3f yaw=%.1f pitch=%.1f ground=%v", x, y, z, yaw, pitch, g)
	case dir == ">" && id == 0x05:
		yaw, pitch, g := r.Float(), r.Float(), r.Bool()
		s = fmt.Sprintf("look yaw=%.1f pitch=%.1f ground=%v", yaw, pitch, g)
	case dir == ">" && id == 0x03:
		s = fmt.Sprintf("ground=%v", r.Bool())
	case dir == "<" && id == 0x08:
		x, y, z := r.Double(), r.Double(), r.Double()
		yaw, pitch, flags := r.Float(), r.Float(), r.Byte()
		s = fmt.Sprintf("TELEPORT to %.3f %.3f %.3f yaw=%.1f pitch=%.1f flags=%#x", x, y, z, yaw, pitch, flags)
	case dir == "<" && id == 0x12:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d velocity %.3f %.3f %.3f", eid, float64(r.Short())/8000, float64(r.Short())/8000, float64(r.Short())/8000)
	case dir == "<" && id == 0x1a:
		eid := r.Int()
		s = fmt.Sprintf("entity %d status %d", eid, r.Byte())
	case dir == "<" && id == 0x0b:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d animation %d", eid, r.Byte())
	case dir == ">" && id == 0x0b:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d action %d", eid, r.VarInt())
	case dir == ">" && id == 0x02:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d use type %d", eid, r.VarInt())
	case dir == "<" && id == 0x06:
		hp := r.Float()
		s = fmt.Sprintf("health %.1f food %d", hp, r.VarInt())
	default:
		return ""
	}
	if r.Err() != nil {
		return ""
	}
	return "[" + s + "]"
}
