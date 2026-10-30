package server

import (
	"encoding/hex"
	"math"
	"strings"

	"gocraft/proto"
)

// Other players, as 1.8 shows them: a tab-list entry, a spawned player entity, then teleports as
// they move. Positions are fixed point (1/32 block) and angles are 1/256 of a turn, in a byte.

func fixed(v float64) int32  { return int32(math.Floor(v * 32)) }
func angle(deg float32) byte { return byte(int32(math.Floor(float64(deg) * 256 / 360))) }

func uuidBytes(s string) []byte {
	b, _ := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if len(b) != 16 {
		b = make([]byte, 16)
	}
	return b
}

func (p *Player) listPacket(action int32) []byte {
	w := proto.NewPacket(0x38)
	w.VarInt(action)
	w.VarInt(1)
	w.Bytes(uuidBytes(p.uuid))
	if action == 0 {
		w.String(p.name)
		w.VarInt(0) // no skin properties: the client picks the default skin
		w.VarInt(1) // creative
		w.VarInt(0) // ping
		w.Bool(false)
	}
	return w.Frame()
}

func (p *Player) spawnPacket() []byte {
	p.mu.Lock()
	x, y, z, yaw, pitch := p.x, p.y, p.z, p.yaw, p.pitch
	p.mu.Unlock()
	w := proto.NewPacket(0x0C)
	w.VarInt(p.eid)
	w.Bytes(uuidBytes(p.uuid))
	w.Int(fixed(x))
	w.Int(fixed(y))
	w.Int(fixed(z))
	w.Byte(angle(yaw))
	w.Byte(angle(pitch))
	w.UShort(0)  // empty hand
	w.Byte(0x7F) // no entity metadata
	return w.Frame()
}

func (p *Player) teleportPacket(onGround bool) []byte {
	p.mu.Lock()
	x, y, z, yaw, pitch := p.x, p.y, p.z, p.yaw, p.pitch
	p.mu.Unlock()
	w := proto.NewPacket(0x18)
	w.VarInt(p.eid)
	w.Int(fixed(x))
	w.Int(fixed(y))
	w.Int(fixed(z))
	w.Byte(angle(yaw))
	w.Byte(angle(pitch))
	w.Bool(onGround)
	return w.Frame()
}

func (p *Player) headPacket() []byte {
	p.mu.Lock()
	yaw := p.yaw
	p.mu.Unlock()
	w := proto.NewPacket(0x19)
	w.VarInt(p.eid)
	w.Byte(angle(yaw))
	return w.Frame()
}

func (p *Player) destroyPacket() []byte {
	w := proto.NewPacket(0x13)
	w.VarInt(1)
	w.VarInt(p.eid)
	return w.Frame()
}

func (p *Player) swingPacket() []byte {
	w := proto.NewPacket(0x0B)
	w.VarInt(p.eid)
	w.Byte(0)
	return w.Frame()
}

// others are the players in the world besides p.
func (s *Server) others(p *Player) []*Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Player, 0, len(s.players))
	for _, o := range s.players {
		if o != p && o.inWorld() {
			out = append(out, o)
		}
	}
	return out
}

// sendOthers gives p's packets to every other player.
func (s *Server) sendOthers(p *Player, frames ...[]byte) {
	for _, o := range s.others(p) {
		for _, f := range frames {
			o.send(f)
		}
	}
}

// introduce shows the players already here to the newcomer and the newcomer to them.
func (s *Server) introduce(p *Player) {
	for _, o := range s.others(p) {
		p.send(o.listPacket(0))
		p.send(o.spawnPacket())
		p.send(o.headPacket())
	}
	s.sendOthers(p, p.listPacket(0), p.spawnPacket(), p.headPacket())
}
