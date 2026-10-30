// Package proto reads and writes Minecraft 1.8.9 (protocol 47) packets: VarInt framing and the
// field types the server uses. Compression and encryption are off (offline mode).
package proto

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
)

// MaxPacket caps a serverbound packet's length so a client cannot make us allocate without bound.
const MaxPacket = 1 << 21

var (
	ErrVarInt = errors.New("varint too long")
	ErrShort  = errors.New("packet too short")
)

// ReadPacket reads one framed packet: its id and a Reader over the rest.
func ReadPacket(br *bufio.Reader) (int32, *Reader, error) {
	var n uint32
	for i := 0; ; i++ {
		if i == 5 {
			return 0, nil, ErrVarInt
		}
		b, err := br.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		n |= uint32(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			break
		}
	}
	if n < 1 || n > MaxPacket {
		return 0, nil, fmt.Errorf("bad packet length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(br, buf); err != nil {
		return 0, nil, err
	}
	r := &Reader{b: buf}
	id := r.VarInt()
	if r.err != nil {
		return 0, nil, r.err
	}
	if Trace != nil {
		Trace("<", id, r.b)
	}
	return id, r, nil
}

// Reader decodes fields from one packet. The first error sticks: later reads return zero values,
// so a handler reads everything and checks Err once.
type Reader struct {
	b   []byte
	err error
}

func (r *Reader) Err() error { return r.err }

// Bytes reads n raw bytes (a slice of the packet, not a copy).
func (r *Reader) Bytes(n int) []byte { return r.take(n) }

// Rest is what is left of the packet, without taking it.
func (r *Reader) Rest() []byte { return r.b }

// Len is how many bytes of the packet are left.
func (r *Reader) Len() int { return len(r.b) }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b) {
		r.err = ErrShort
		return nil
	}
	s := r.b[:n]
	r.b = r.b[n:]
	return s
}

func (r *Reader) Byte() byte {
	s := r.take(1)
	if r.err != nil {
		return 0
	}
	return s[0]
}

func (r *Reader) Bool() bool { return r.Byte() != 0 }

func (r *Reader) UShort() uint16 {
	s := r.take(2)
	if r.err != nil {
		return 0
	}
	return uint16(s[0])<<8 | uint16(s[1])
}

func (r *Reader) Short() int16 { return int16(r.UShort()) }

func (r *Reader) Int() int32 {
	s := r.take(4)
	if r.err != nil {
		return 0
	}
	return int32(uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3]))
}

func (r *Reader) Long() int64 {
	s := r.take(8)
	if r.err != nil {
		return 0
	}
	var v uint64
	for _, b := range s {
		v = v<<8 | uint64(b)
	}
	return int64(v)
}

func (r *Reader) Float() float32  { return math.Float32frombits(uint32(r.Int())) }
func (r *Reader) Double() float64 { return math.Float64frombits(uint64(r.Long())) }

func (r *Reader) VarInt() int32 {
	var v uint32
	for i := 0; i < 5; i++ {
		b := r.Byte()
		if r.err != nil {
			return 0
		}
		v |= uint32(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return int32(v)
		}
	}
	r.err = ErrVarInt
	return 0
}

// String reads a VarInt-prefixed UTF-8 string of at most maxChars characters (at most 4 bytes each).
func (r *Reader) String(maxChars int) string {
	n := int(r.VarInt())
	if r.err == nil && (n < 0 || n > maxChars*4) {
		r.err = ErrShort
	}
	s := r.take(n)
	if r.err != nil {
		return ""
	}
	return string(s)
}

// Position unpacks a block position: 26 bits x, 12 bits y, 26 bits z, sign-extended.
func UnpackPos(v int64) (x, y, z int32) {
	return int32(v >> 38), int32((v << 26) >> 52), int32((v << 38) >> 38)
}

func PackPos(x, y, z int32) int64 {
	return int64(x&0x3FFFFFF)<<38 | int64(y&0xFFF)<<26 | int64(z&0x3FFFFFF)
}

// Writer builds one clientbound packet.
type Writer struct{ B []byte }

func NewPacket(id int32) *Writer {
	w := &Writer{}
	w.VarInt(id)
	return w
}

func (w *Writer) Byte(v byte) { w.B = append(w.B, v) }
func (w *Writer) Bool(v bool) {
	if v {
		w.Byte(1)
	} else {
		w.Byte(0)
	}
}
func (w *Writer) UShort(v uint16) { w.B = append(w.B, byte(v>>8), byte(v)) }
func (w *Writer) Int(v int32)     { w.B = append(w.B, byte(v>>24), byte(v>>16), byte(v>>8), byte(v)) }
func (w *Writer) Long(v int64) {
	for s := 56; s >= 0; s -= 8 {
		w.B = append(w.B, byte(v>>s))
	}
}
func (w *Writer) Float(v float32)  { w.Int(int32(math.Float32bits(v))) }
func (w *Writer) Double(v float64) { w.Long(int64(math.Float64bits(v))) }
func (w *Writer) VarInt(v int32) {
	u := uint32(v)
	for u >= 0x80 {
		w.B = append(w.B, byte(u)|0x80)
		u >>= 7
	}
	w.B = append(w.B, byte(u))
}
func (w *Writer) String(s string) {
	w.VarInt(int32(len(s)))
	w.B = append(w.B, s...)
}
func (w *Writer) Bytes(b []byte) { w.B = append(w.B, b...) }

// Frame returns the packet ready for the wire: length prefix, then id and fields.
func (w *Writer) Frame() []byte {
	traceFrame(w.B)
	out := &Writer{B: make([]byte, 0, len(w.B)+5)}
	out.VarInt(int32(len(w.B)))
	return append(out.B, w.B...)
}
