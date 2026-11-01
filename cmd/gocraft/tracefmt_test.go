package main

import (
	"math"
	"strings"
	"testing"

	"gocraft/proto"
)

func TestDecodeTrace(t *testing.T) {
	w := proto.NewPacket(0x08)
	w.Double(12.5)
	w.Double(64)
	w.Double(-3.25)
	w.Float(90)
	w.Float(15)
	w.Byte(0x18)
	// A packet is framed with its id; the trace gets what follows it.
	payload := w.B[1:]
	got := decodeTrace("<", 0x08, payload)
	if !strings.Contains(got, "TELEPORT to 12.500 64.000 -3.250") || !strings.Contains(got, "flags=0x18") {
		t.Fatalf("teleport: %q", got)
	}
	c := proto.NewPacket(0x06)
	c.Double(1)
	c.Double(2)
	c.Double(3)
	c.Float(10)
	c.Float(20)
	c.Bool(true)
	if got := decodeTrace(">", 0x06, c.B[1:]); !strings.Contains(got, "pos 1.000 2.000 3.000 yaw=10.0 pitch=20.0 ground=true") {
		t.Fatalf("position: %q", got)
	}
	v := proto.NewPacket(0x12)
	v.VarInt(7)
	for _, s := range []float64{0.4, 0.36, -0.1} {
		v.UShort(uint16(int16(math.Round(s * 8000))))
	}
	if got := decodeTrace("<", 0x12, v.B[1:]); !strings.Contains(got, "entity 7 velocity 0.400 0.360 -0.100") {
		t.Fatalf("velocity: %q", got)
	}
	if got := decodeTrace("<", 0x08, payload[:5]); got != "" {
		t.Fatalf("a truncated packet decoded as %q", got)
	}
	if decodeTrace("<", 0x55, nil) != "" {
		t.Fatal("an unknown packet decoded")
	}
}
