package proto

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// Compression, once a server has sent Set Compression with a threshold: every packet after it is
// framed as  length, data length (0 when the body is sent as it is), then the body, which is
// zlib-deflated when its data length is not 0. A negative threshold means compression is off.

// MaxInflated caps what a compressed packet may expand to.
const MaxInflated = 1 << 23

// ReadPacketZ reads one packet from a connection whose compression threshold is set (>= 0).
func ReadPacketZ(br *bufio.Reader, threshold int) (int32, *Reader, error) {
	if threshold < 0 {
		return ReadPacket(br)
	}
	n, err := readVarInt(br)
	if err != nil {
		return 0, nil, err
	}
	if n < 1 || n > MaxPacket {
		return 0, nil, fmt.Errorf("bad packet length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(br, buf); err != nil {
		return 0, nil, err
	}
	head := &Reader{b: buf}
	dataLen := int(head.VarInt())
	if head.err != nil {
		return 0, nil, head.err
	}
	body := head.b
	if dataLen != 0 {
		if dataLen < 0 || dataLen > MaxInflated {
			return 0, nil, fmt.Errorf("bad inflated length %d", dataLen)
		}
		zr, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		defer zr.Close()
		out := make([]byte, dataLen)
		if _, err := io.ReadFull(zr, out); err != nil {
			return 0, nil, fmt.Errorf("inflate: %w", err)
		}
		body = out
	}
	r := &Reader{b: body}
	id := r.VarInt()
	if r.err != nil {
		return 0, nil, r.err
	}
	if Trace != nil {
		Trace("<", id, r.b)
	}
	return id, r, nil
}

func readVarInt(br *bufio.Reader) (uint32, error) {
	var n uint32
	for i := 0; ; i++ {
		if i == 5 {
			return 0, ErrVarInt
		}
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		n |= uint32(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return n, nil
		}
	}
}

// FrameZ returns the packet ready for a connection with the given compression threshold.
func (w *Writer) FrameZ(threshold int) []byte {
	if threshold < 0 {
		return w.Frame()
	}
	traceFrame(w.B)
	var inner Writer
	if len(w.B) >= threshold {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		zw.Write(w.B)
		zw.Close()
		inner.VarInt(int32(len(w.B)))
		inner.Bytes(z.Bytes())
	} else {
		inner.VarInt(0)
		inner.Bytes(w.B)
	}
	var out Writer
	out.VarInt(int32(len(inner.B)))
	return append(out.B, inner.B...)
}
