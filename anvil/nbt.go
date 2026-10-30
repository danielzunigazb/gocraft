// Package anvil reads Minecraft 1.8.9 worlds: NBT, level.dat and the region files (.mca) of the Anvil format.
package anvil

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Compound is an NBT compound. Its values are int8, int16, int32, int64, float32, float64, string,
// []byte (byte array), []int32 (int array), []any (list) or Compound.
type Compound map[string]any

const maxLen = 1 << 24 // no array or string in a world file is larger than this

// ReadNBT decodes one named root tag, which is a compound in every Minecraft file.
func ReadNBT(r io.Reader) (Compound, error) {
	br := bufio.NewReader(r)
	t, err := br.ReadByte()
	if err != nil {
		return nil, err
	}
	if t != 10 {
		return nil, fmt.Errorf("nbt root is tag %d, not a compound", t)
	}
	if _, err := readString(br); err != nil {
		return nil, err
	}
	v, err := readValue(br, 10, 0)
	if err != nil {
		return nil, err
	}
	return v.(Compound), nil
}

func readString(r *bufio.Reader) (string, error) {
	var n uint16
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return "", err
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return string(b), err
}

func readLen(r *bufio.Reader) (int, error) {
	var n int32
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		return 0, err
	}
	if n < 0 || n > maxLen {
		return 0, errors.New("nbt: bad length")
	}
	return int(n), nil
}

func readValue(r *bufio.Reader, t byte, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("nbt: too deep")
	}
	be := binary.BigEndian
	switch t {
	case 1:
		b, err := r.ReadByte()
		return int8(b), err
	case 2:
		var v int16
		return v, binary.Read(r, be, &v)
	case 3:
		var v int32
		err := binary.Read(r, be, &v)
		return v, err
	case 4:
		var v int64
		err := binary.Read(r, be, &v)
		return v, err
	case 5:
		var v float32
		err := binary.Read(r, be, &v)
		return v, err
	case 6:
		var v float64
		err := binary.Read(r, be, &v)
		return v, err
	case 7:
		n, err := readLen(r)
		if err != nil {
			return nil, err
		}
		b := make([]byte, n)
		_, err = io.ReadFull(r, b)
		return b, err
	case 8:
		return readString(r)
	case 9:
		et, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		n, err := readLen(r)
		if err != nil {
			return nil, err
		}
		l := make([]any, 0, min(n, 1024))
		for i := 0; i < n; i++ {
			v, err := readValue(r, et, depth+1)
			if err != nil {
				return nil, err
			}
			l = append(l, v)
		}
		return l, nil
	case 10:
		c := Compound{}
		for {
			et, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if et == 0 {
				return c, nil
			}
			name, err := readString(r)
			if err != nil {
				return nil, err
			}
			if c[name], err = readValue(r, et, depth+1); err != nil {
				return nil, err
			}
		}
	case 11:
		n, err := readLen(r)
		if err != nil {
			return nil, err
		}
		a := make([]int32, n)
		return a, binary.Read(r, be, a)
	case 12:
		n, err := readLen(r)
		if err != nil {
			return nil, err
		}
		a := make([]int64, n)
		return a, binary.Read(r, be, a)
	}
	return nil, fmt.Errorf("nbt: unknown tag %d", t)
}

// Number reads any numeric NBT value as a float64.
func Number(v any) (float64, bool) {
	switch n := v.(type) {
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, !math.IsNaN(n)
	}
	return 0, false
}
