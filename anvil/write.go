package anvil

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"

	"gocraft/world"
)

// WriteNBT encodes a root compound with the given root name (empty in every Minecraft file). Keys are
// written in sorted order, so the same compound always gives the same bytes. Values take the types
// ReadNBT produces: int8, int16, int32, int64, float32, float64, string, []byte, []int32, []any and
// Compound; a list takes its element type from its first element (an empty one is a list of end tags).
func WriteNBT(w io.Writer, root Compound, name string) error {
	var b bytes.Buffer
	b.WriteByte(10)
	writeString(&b, name)
	if err := writePayload(&b, root); err != nil {
		return err
	}
	_, err := w.Write(b.Bytes())
	return err
}

func writeString(b *bytes.Buffer, s string) {
	var n [2]byte
	binary.BigEndian.PutUint16(n[:], uint16(len(s)))
	b.Write(n[:])
	b.WriteString(s)
}

func tagOf(v any) (byte, error) {
	switch v.(type) {
	case int8:
		return 1, nil
	case int16:
		return 2, nil
	case int32:
		return 3, nil
	case int64:
		return 4, nil
	case float32:
		return 5, nil
	case float64:
		return 6, nil
	case []byte:
		return 7, nil
	case string:
		return 8, nil
	case []any:
		return 9, nil
	case Compound:
		return 10, nil
	case []int32:
		return 11, nil
	}
	return 0, fmt.Errorf("nbt: cannot write a %T", v)
}

func writePayload(b *bytes.Buffer, v any) error {
	be := binary.BigEndian
	var tmp [8]byte
	switch t := v.(type) {
	case int8:
		b.WriteByte(byte(t))
	case int16:
		be.PutUint16(tmp[:], uint16(t))
		b.Write(tmp[:2])
	case int32:
		be.PutUint32(tmp[:], uint32(t))
		b.Write(tmp[:4])
	case int64:
		be.PutUint64(tmp[:], uint64(t))
		b.Write(tmp[:8])
	case float32:
		be.PutUint32(tmp[:], math.Float32bits(t))
		b.Write(tmp[:4])
	case float64:
		be.PutUint64(tmp[:], math.Float64bits(t))
		b.Write(tmp[:8])
	case []byte:
		be.PutUint32(tmp[:], uint32(len(t)))
		b.Write(tmp[:4])
		b.Write(t)
	case string:
		writeString(b, t)
	case []int32:
		be.PutUint32(tmp[:], uint32(len(t)))
		b.Write(tmp[:4])
		for _, x := range t {
			be.PutUint32(tmp[:], uint32(x))
			b.Write(tmp[:4])
		}
	case []any:
		et := byte(0)
		if len(t) > 0 {
			var err error
			if et, err = tagOf(t[0]); err != nil {
				return err
			}
		}
		b.WriteByte(et)
		be.PutUint32(tmp[:], uint32(len(t)))
		b.Write(tmp[:4])
		for _, e := range t {
			if tt, err := tagOf(e); err != nil || tt != et {
				return fmt.Errorf("nbt: a list of %d holds a %T", et, e)
			}
			if err := writePayload(b, e); err != nil {
				return err
			}
		}
	case Compound:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			tt, err := tagOf(t[k])
			if err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			b.WriteByte(tt)
			writeString(b, k)
			if err := writePayload(b, t[k]); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
		b.WriteByte(0)
	default:
		return fmt.Errorf("nbt: cannot write a %T", v)
	}
	return nil
}

// ---- columns ----

func setNibble(a []byte, i int, v byte) {
	if i&1 == 0 {
		a[i>>1] = a[i>>1]&0xF0 | v&15
	} else {
		a[i>>1] = a[i>>1]&0x0F | v<<4
	}
}

// ChunkNBT is the NBT root of a column holding the blocks, light and biomes of c. base is the column as
// the world saved it, or nil: what the game does not change (entities, tile entities, inhabited time,
// populated flags) is kept from it, and a column that did not exist is made with none of them.
func ChunkNBT(cp world.ChunkPos, c *world.Chunk, base Compound) Compound {
	level := Compound{}
	if bl, ok := base["Level"].(Compound); ok {
		for k, v := range bl {
			level[k] = v
		}
	}
	level["xPos"], level["zPos"] = cp.X, cp.Z
	if _, ok := level["Entities"]; !ok {
		level["Entities"] = []any{}
	}
	if _, ok := level["TileEntities"]; !ok {
		level["TileEntities"] = []any{}
	}
	if _, ok := level["InhabitedTime"]; !ok {
		level["InhabitedTime"] = int64(0)
	}
	level["TerrainPopulated"] = int8(1)
	level["LightPopulated"] = int8(boolInt(c.Light != nil)) // without light the game works it out when it loads the column
	level["V"] = int8(1)

	var sections []any
	for y := 0; y < 16; y++ {
		blocks := c.Blocks[y*4096 : y*4096+4096]
		empty := true
		for _, b := range blocks {
			if b != 0 {
				empty = false
				break
			}
		}
		if empty {
			continue // a section of air is left out, as the game does
		}
		ids, data := make([]byte, 4096), make([]byte, 2048)
		var add []byte
		for i, b := range blocks {
			id := b >> 4
			ids[i] = byte(id)
			setNibble(data, i, byte(b&15))
			if id > 255 {
				if add == nil {
					add = make([]byte, 2048)
				}
				setNibble(add, i, byte(id>>8))
			}
		}
		sec := Compound{"Y": int8(y), "Blocks": ids, "Data": data}
		if add != nil {
			sec["Add"] = add
		}
		sky, blk := make([]byte, 2048), make([]byte, 2048)
		for i := 0; i < 4096; i++ {
			l := world.FullLight
			if c.Light != nil {
				l = int(c.Light[y*4096+i])
			}
			setNibble(sky, i, byte(l>>4))
			setNibble(blk, i, byte(l&15))
		}
		sec["SkyLight"], sec["BlockLight"] = sky, blk
		sections = append(sections, sec)
	}
	if sections == nil {
		sections = []any{}
	}
	level["Sections"] = sections
	if c.Biomes != nil {
		level["Biomes"] = append([]byte(nil), c.Biomes...)
	} else if _, ok := level["Biomes"]; !ok {
		level["Biomes"] = bytes.Repeat([]byte{1}, 256) // plains
	}
	level["HeightMap"] = heightMap(c)
	return Compound{"Level": level}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// heightMap is, for each column of blocks, the height of the first block above the highest that is not
// air (an approximation of the game's: it counts a block as opaque if it is there at all).
func heightMap(c *world.Chunk) []int32 {
	h := make([]int32, 256)
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			for y := 255; y >= 0; y-- {
				if c.Blocks[y<<8|z<<4|x] != 0 {
					h[z<<4|x] = int32(y + 1)
					break
				}
			}
		}
	}
	return h
}

// ---- regions ----

const sector = 4096

// Region is the content of one .mca file: up to 32 x 32 compressed columns.
type Region struct {
	raw [1024][]byte // the stored column, compression byte first; nil for none
	ts  [1024]uint32
}

func regionIndex(cp world.ChunkPos) int { return int(cp.X&31) + int(cp.Z&31)*32 }

// ReadRegion loads a region file; a missing one is an empty region.
func ReadRegion(b []byte) (*Region, error) {
	r := &Region{}
	if len(b) == 0 {
		return r, nil
	}
	if len(b) < 2*sector {
		return nil, fmt.Errorf("region file of %d bytes has no header", len(b))
	}
	for i := 0; i < 1024; i++ {
		loc := binary.BigEndian.Uint32(b[i*4:])
		r.ts[i] = binary.BigEndian.Uint32(b[sector+i*4:])
		off, n := int(loc>>8)*sector, int(loc&0xFF)*sector
		if loc == 0 || n == 0 {
			continue
		}
		if off+5 > len(b) {
			return nil, fmt.Errorf("column %d lies outside the file", i)
		}
		length := int(binary.BigEndian.Uint32(b[off:]))
		if length < 1 || off+4+length > len(b) {
			return nil, fmt.Errorf("column %d has a bad length %d", i, length)
		}
		r.raw[i] = append([]byte(nil), b[off+4:off+4+length]...)
	}
	return r, nil
}

// Set replaces a column with the given NBT root, compressed with zlib as the game does.
func (r *Region) Set(cp world.ChunkPos, root Compound, modified uint32) error {
	var z bytes.Buffer
	z.WriteByte(2)
	zw := zlib.NewWriter(&z)
	if err := WriteNBT(zw, root, ""); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	i := regionIndex(cp)
	r.raw[i], r.ts[i] = z.Bytes(), modified
	return nil
}

// Bytes lays the region out as a file: the two 4 KiB header tables, then each column in whole sectors.
func (r *Region) Bytes() ([]byte, error) {
	out := make([]byte, 2*sector)
	next := 2
	for i := range r.raw {
		if r.raw[i] == nil {
			continue
		}
		length := 4 + len(r.raw[i])
		sectors := (length + sector - 1) / sector
		if sectors > 255 {
			return nil, fmt.Errorf("column %d needs %d sectors, a region file holds at most 255 per column", i, sectors)
		}
		binary.BigEndian.PutUint32(out[i*4:], uint32(next)<<8|uint32(sectors))
		binary.BigEndian.PutUint32(out[sector+i*4:], r.ts[i])
		var head [4]byte
		binary.BigEndian.PutUint32(head[:], uint32(len(r.raw[i])))
		out = append(out, head[:]...)
		out = append(out, r.raw[i]...)
		out = append(out, make([]byte, sectors*sector-length)...)
		next += sectors
	}
	return out, nil
}
