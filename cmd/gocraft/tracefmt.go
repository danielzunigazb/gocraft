package main

import (
	"fmt"

	"gocraft/proto"
)

// traceLine is a packet's line in the -netlog file: milliseconds since the start, direction, id, name, size, and what
// it says when it is one that is decoded. tools/pvpreport.py reads these lines.
func traceLine(ms int64, dir string, id int32, payload []byte) string {
	text := ""
	if dir == "<" && (id == 0x02 || id == 0x40 || id == 0x45 || id == 0x3F) || dir == ">" && (id == 0x01 || id == 0x17) {
		text = " " + printable(payload)
	} else if d := decodeTrace(dir, id, payload); d != "" {
		text = " " + d
	}
	return fmt.Sprintf("%6dms %s 0x%02x %-22s %d bytes%s\n", ms, dir, id, packetName(dir, id), len(payload), text)
}

// noteLine is one of the client's own notes in the -netlog file.
func noteLine(ms int64, text string) string { return fmt.Sprintf("%6dms # %s\n", ms, text) }

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
	case dir == ">" && id == 0x07: // digging; status 5 lets go of the item in use
		st := r.Byte()
		x, y, z := proto.UnpackPos(r.Long())
		s = fmt.Sprintf("dig status %d at %d %d %d face %d", st, x, y, z, r.Byte())
	case dir == ">" && id == 0x08: // a click on a block, or the item used in the air (position -1)
		pos, face := r.Long(), r.Byte()
		item := int16(r.UShort())
		if pos == -1 {
			s = fmt.Sprintf("use item %d in the air", item)
		} else {
			x, y, z := proto.UnpackPos(pos)
			s = fmt.Sprintf("click block %d %d %d face %d holding %d", x, y, z, face, item)
		}
	case dir == ">" && id == 0x09:
		s = fmt.Sprintf("held slot %d", r.UShort())
	case dir == "<" && id == 0x1d:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d effect %d amplifier %d for %d ticks", eid, r.Byte(), r.Byte(), r.VarInt())
	case dir == "<" && id == 0x1e:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d effect %d removed", eid, r.Byte())
	case dir == "<" && id == 0x15: // an entity moves by a few 32ths of a block
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d move %+.3f %+.3f %+.3f", eid, float64(int8(r.Byte()))/32, float64(int8(r.Byte()))/32, float64(int8(r.Byte()))/32)
	case dir == "<" && id == 0x17:
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d move %+.3f %+.3f %+.3f", eid, float64(int8(r.Byte()))/32, float64(int8(r.Byte()))/32, float64(int8(r.Byte()))/32)
	case dir == "<" && id == 0x18: // an entity is put somewhere
		eid := r.VarInt()
		s = fmt.Sprintf("entity %d at %.3f %.3f %.3f", eid, float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32)
	case dir == "<" && id == 0x0c: // a player appears
		eid := r.VarInt()
		r.Bytes(16)
		s = fmt.Sprintf("entity %d spawns at %.3f %.3f %.3f", eid, float64(r.Int())/32, float64(r.Int())/32, float64(r.Int())/32)
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
