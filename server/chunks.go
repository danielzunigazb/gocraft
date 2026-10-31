package server

import (
	"gocraft/proto"
	"gocraft/world"
)

// ChunkPacket is Chunk Data (0x21) for a whole column. The sections come as one array of block
// states, then one of block light, then one of sky light, then the 256 biomes, as 1.8 lays it out.
func ChunkPacket(w *world.World, cp world.ChunkPos) []byte {
	c := w.Snapshot(cp)
	var mask uint16
	var data []byte
	sections := 0
	for s := 0; s < 16; s++ {
		blocks := c.Blocks[s*4096 : s*4096+4096]
		empty := true
		for _, b := range blocks {
			if b != 0 {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		mask |= 1 << s
		sections++
		for _, b := range blocks {
			data = append(data, byte(b), byte(b>>8))
		}
	}
	if c.Light == nil {
		data = append(data, make([]byte, 2048*sections)...) // block light: dark
		for i := 0; i < 2048*sections; i++ {
			data = append(data, 0xFF) // sky light: full
		}
	} else { // a generated column: the light it was made with, block light first, then sky light
		for _, sky := range []bool{false, true} {
			for s := 0; s < 16; s++ {
				if mask&(1<<s) == 0 {
					continue
				}
				for i := 0; i < 4096; i += 2 {
					lo, hi := c.Light[s*4096+i], c.Light[s*4096+i+1]
					if sky {
						lo, hi = lo>>4, hi>>4
					} else {
						lo, hi = lo&15, hi&15
					}
					data = append(data, lo|hi<<4)
				}
			}
		}
	}
	if c.Biomes != nil {
		data = append(data, c.Biomes...)
	} else {
		for i := 0; i < 256; i++ {
			data = append(data, 1) // plains
		}
	}

	p := proto.NewPacket(0x21)
	p.Int(cp.X)
	p.Int(cp.Z)
	p.Bool(true)
	p.UShort(mask)
	p.VarInt(int32(len(data)))
	p.Bytes(data)
	return p.Frame()
}

// UnloadPacket tells the client to drop a column: a ground-up chunk with no sections and no data.
func UnloadPacket(cp world.ChunkPos) []byte {
	p := proto.NewPacket(0x21)
	p.Int(cp.X)
	p.Int(cp.Z)
	p.Bool(true)
	p.UShort(0)
	p.VarInt(0)
	return p.Frame()
}

// BreakEffectPacket is Effect 2001: the others see the pieces of a broken block and hear it (the one who
// broke it makes them itself). The data is the game's state id: the block id plus its meta times 4096.
func BreakEffectPacket(x, y, z int32, state uint16) []byte {
	p := proto.NewPacket(0x28)
	p.Int(2001)
	p.Long(proto.PackPos(x, y, z))
	p.Int(int32(state>>4) | int32(state&15)<<12)
	p.Bool(false)
	return p.Frame()
}

func BlockChangePacket(x, y, z int32, state uint16) []byte {
	p := proto.NewPacket(0x23)
	p.Long(proto.PackPos(x, y, z))
	p.VarInt(int32(state))
	return p.Frame()
}
