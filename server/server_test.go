package server

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"gocraft/proto"
)

type client struct {
	t  *testing.T
	nc net.Conn
	br *bufio.Reader
}

func dial(t *testing.T, addr string) *client {
	nc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	return &client{t: t, nc: nc, br: bufio.NewReader(nc)}
}

func (c *client) send(p *proto.Writer) {
	c.nc.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.nc.Write(p.Frame()); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) handshake(version, next int32) {
	p := proto.NewPacket(0)
	p.VarInt(version)
	p.String("localhost")
	p.UShort(25565)
	p.VarInt(next)
	c.send(p)
}

// until reads packets, skipping others, until one has the id; it returns that packet's fields.
func (c *client) until(id int32) *proto.Reader {
	for {
		c.nc.SetReadDeadline(time.Now().Add(5 * time.Second))
		got, r, err := proto.ReadPacket(c.br)
		if err != nil {
			c.t.Fatalf("waiting for 0x%02x: %v", id, err)
		}
		if got == id {
			return r
		}
	}
}

func start(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go New(Config{ViewDistance: 2}).Serve(l)
	return l.Addr().String()
}

func login(t *testing.T, addr, name string) *client {
	c := dial(t, addr)
	c.handshake(Protocol, 2)
	p := proto.NewPacket(0)
	p.String(name)
	c.send(p)
	r := c.until(0x02)
	if uuid := r.String(36); uuid != offlineUUID(name) {
		t.Fatalf("uuid %q", uuid)
	}
	c.until(0x08) // position and look: the last of the join sequence
	return c
}

func TestStatus(t *testing.T) {
	c := dial(t, start(t))
	c.handshake(Protocol, 1)
	c.send(proto.NewPacket(0))
	var s struct {
		Version struct {
			Name     string
			Protocol int
		}
	}
	if err := json.Unmarshal([]byte(c.until(0).String(32767)), &s); err != nil {
		t.Fatal(err)
	}
	if s.Version.Protocol != 47 || s.Version.Name != "1.8.9" {
		t.Fatalf("status %+v", s)
	}
	ping := proto.NewPacket(1)
	ping.Long(1234)
	c.send(ping)
	if got := c.until(1).Long(); got != 1234 {
		t.Fatalf("pong %d", got)
	}
}

func TestWrongVersionRefused(t *testing.T) {
	c := dial(t, start(t))
	c.handshake(5, 2)
	p := proto.NewPacket(0)
	p.String("Old")
	c.send(p)
	if msg := c.until(0).String(32767); !strings.Contains(msg, "1.8.9") {
		t.Fatalf("reason %q", msg)
	}
}

func TestPlaceBreakAndChat(t *testing.T) {
	addr := start(t)
	a := login(t, addr, "Alice")
	b := login(t, addr, "Bob")

	// Alice puts stone in her hotbar's first slot, then places it on top of the grass at 0,3,0.
	inv := proto.NewPacket(0x10)
	inv.UShort(36)
	inv.UShort(1) // stone
	inv.Byte(1)
	inv.UShort(0)
	inv.Byte(0) // no NBT
	a.send(inv)
	place := proto.NewPacket(0x08)
	place.Long(proto.PackPos(0, 3, 0))
	place.Byte(1) // top face
	place.Bytes([]byte{0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0})
	a.send(place)

	for _, c := range []*client{a, b} {
		r := c.until(0x23)
		x, y, z := proto.UnpackPos(r.Long())
		if st := r.VarInt(); x != 0 || y != 4 || z != 0 || st != 1<<4 {
			t.Fatalf("block change %d,%d,%d = %d", x, y, z, st)
		}
	}

	// Bob breaks it; both see air.
	dig := proto.NewPacket(0x07)
	dig.Byte(0)
	dig.Long(proto.PackPos(0, 4, 0))
	dig.Byte(1)
	b.send(dig)
	for _, c := range []*client{a, b} {
		r := c.until(0x23)
		r.Long()
		if st := r.VarInt(); st != 0 {
			t.Fatalf("after break: %d", st)
		}
	}
	// Alice sees the pieces of Bob's stone (Effect 2001 with the game's state id); Bob makes his own.
	fx := a.until(0x28)
	if id := fx.Int(); id != 2001 {
		t.Fatalf("effect %d, want 2001", id)
	}
	if x, y, z := proto.UnpackPos(fx.Long()); x != 0 || y != 4 || z != 0 {
		t.Fatalf("effect at %d,%d,%d", x, y, z)
	}
	if data := fx.Int(); data != 1 {
		t.Fatalf("effect data %d, want 1 (stone)", data)
	}

	// Bedrock does not break.
	dig = proto.NewPacket(0x07)
	dig.Byte(0)
	dig.Long(proto.PackPos(0, 0, 0))
	dig.Byte(1)
	b.send(dig)
	r := b.until(0x23)
	r.Long()
	if st := r.VarInt(); st != 7<<4 {
		t.Fatalf("bedrock became %d", st)
	}

	chat := proto.NewPacket(0x01)
	chat.String("hello")
	a.send(chat)
	for _, c := range []*client{a, b} {
		for {
			if msg := c.until(0x02).String(32767); strings.Contains(msg, "<Alice> hello") {
				break
			}
		}
	}
}

func TestPositionAndPacking(t *testing.T) {
	for _, p := range [][3]int32{{0, 0, 0}, {-1, 255, -1}, {33554431, 4, -33554432}, {-100, 64, 100}} {
		x, y, z := proto.UnpackPos(proto.PackPos(p[0], p[1], p[2]))
		if x != p[0] || y != p[1] || z != p[2] {
			t.Errorf("%v -> %d,%d,%d", p, x, y, z)
		}
	}
}
