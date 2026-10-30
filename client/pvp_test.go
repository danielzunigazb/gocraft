package client

import (
	"bufio"
	"gocraft/proto"
	"net"
	"sync"
	"testing"
	"time"

	"gocraft/server"
)

// fighters starts a survival server and joins two real clients to it: Alice at the origin facing
// south, and Bob a block and a half in front of her, both on the ground and seeing each other.
func fighters(t *testing.T) (alice, bob *game, bobSeenByAlice func() *remotePlayer) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go server.New(server.Config{ViewDistance: 2, Survival: true}).Serve(l)
	a, b := testGame(t), testGame(t)
	if err := a.connect(l.Addr().String(), "Alice"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.leaveServer)
	if err := b.connect(l.Addr().String(), "Bob"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.leaveServer)
	pumpUntil(t, "both in view", func() bool { return len(a.net.remote) == 1 && len(b.net.remote) == 1 }, a, b)
	a.creative, b.creative = false, false
	a.feet, a.onGround, a.yaw = vec3{0.5, 4, 0.5}, true, 0
	b.feet, b.onGround, b.yaw = vec3{0.5, 4, 2}, true, 3.14159265
	for _, g := range []*game{a, b} {
		g.pos = g.feet.add(vec3{0, eyeHeight, 0})
		g.tickNet()
	}
	seen := func() *remotePlayer {
		for _, p := range a.net.remote {
			return p
		}
		return nil
	}
	pumpUntil(t, "Alice sees Bob in front of her", func() bool {
		a.tickNet()
		return seen().pos[2] > 1.95
	}, a, b)
	return a, b, seen
}

func TestHittingAPlayerHurtsAndPushesThem(t *testing.T) {
	a, b, bob := fighters(t)
	if !a.attackClick() {
		t.Fatal("the click did not find Bob a block and a half away")
	}
	pumpUntil(t, "Bob's health", func() bool { return b.health == 19 }, a, b)
	// Pushed away from Alice (+z) by 0.4, lifted 0.4 off the ground, in blocks a tick.
	if !near(b.vel[2], 0.4, 1e-3) || !near(b.vel[1], 0.4, 1e-3) || !near(b.vel[0], 0, 1e-3) {
		t.Fatalf("Bob's knockback %v", b.vel)
	}
	// Alice sees him flinch.
	pumpUntil(t, "the flinch", func() bool { return bob().hurt > 0 }, a, b)
}

func TestSprintHitSlowsTheAttacker(t *testing.T) {
	a, b, _ := fighters(t)
	a.inv[0], a.sel = slot{ID: 267, N: 1}, 0 // an iron sword
	a.sprinting = true
	a.vel = vec3{0, 0, 0.3}
	a.tickNet() // the sprint and the sword reach the server
	time.Sleep(100 * time.Millisecond)
	if !a.attackClick() {
		t.Fatal("no hit")
	}
	if a.sprinting || !near(a.vel[2], 0.18, 1e-4) {
		t.Fatalf("the attacker's sprint %v, velocity %v: want the sprint ended and the motion at 60%%", a.sprinting, a.vel)
	}
	pumpUntil(t, "a sword's blow", func() bool { return b.health == 13 }, a, b) // 1+4+2 = 7
	if !near(b.vel[2], 0.9, 1e-3) {                                             // 0.4 away from Alice, 0.5 more for the sprint
		t.Fatalf("sprint knockback %v", b.vel)
	}
}

func TestBlockingOverTheNetwork(t *testing.T) {
	a, b, _ := fighters(t)
	a.inv[0], a.sel = slot{ID: 267, N: 1}, 0
	b.inv[0], b.sel = slot{ID: 268, N: 1}, 0
	b.blocking = true
	a.tickNet()
	b.tickNet()
	time.Sleep(150 * time.Millisecond)
	a.attackClick()
	pumpUntil(t, "a blocked blow", func() bool { return b.health == 16 }, a, b) // (1+7)/2 = 4
}

func TestArmourOverTheNetwork(t *testing.T) {
	a, b, _ := fighters(t)
	a.inv[0], a.sel = slot{ID: 267, N: 1}, 0
	for i, id := range []uint16{310, 311, 312, 313} {
		b.armor[i] = slot{ID: id, N: 1}
	}
	a.tickNet()
	b.tickNet()
	time.Sleep(150 * time.Millisecond)
	a.attackClick()
	pumpUntil(t, "an armoured blow", func() bool { return near(b.health, 18.6, 1e-3) }, a, b) // 7 * 5/25
}

func TestDeathAndRespawnOverTheNetwork(t *testing.T) {
	a, b, bob := fighters(t)
	a.inv[0], a.sel = slot{ID: 276, N: 1}, 0 // a diamond sword: 8 a blow
	a.tickNet()
	time.Sleep(100 * time.Millisecond)
	for hit, want := range []float32{12, 4, 0} {
		a.attackClick()
		pumpUntil(t, "the blow", func() bool { return b.health == want }, a, b)
		if hit < 2 {
			time.Sleep(1100 * time.Millisecond) // out of the invulnerability
		}
	}
	pumpUntil(t, "Bob lying dead for Alice", func() bool { return bob().dead }, a, b)
	if in := b.input(); in != (input{}) {
		t.Fatalf("a dead player's input %v", in)
	}
	// Dead for 1.5 seconds, then full health at the spawn, and a new Bob for Alice.
	pumpUntil(t, "the respawn", func() bool { return b.health == 20 && len(a.net.remote) == 1 && !bob().dead }, a, b)
	if !near(b.feet[0], 0.5, 1e-3) || !near(b.feet[2], 0.5, 1e-3) {
		t.Fatalf("Bob respawned at %v", b.feet)
	}
}

func TestServerKeepsHealthNotTheClient(t *testing.T) {
	g := testGame(t)
	g.net = &netState{}
	g.health = 20
	g.hurt(5) // a fall, say: on a server only its health packet changes it
	g.exhaustion, g.food = 40, 10
	g.tickFood()
	if g.health != 20 || g.food != 10 {
		t.Fatalf("local rules touched the health %v or the food %v on a server", g.health, g.food)
	}
}

// fakeServer logs one client in with the given game mode, optionally saying it is gocraft, and
// returns what the client sends afterwards: packet ids, and the channel of plugin messages.
func fakeServer(t *testing.T, mode byte, brand bool) (addr string, sent func() (ids map[int32]bool, channels map[string]bool)) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var mu sync.Mutex
	ids, channels := map[int32]bool{}, map[string]bool{}
	go func() {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		defer nc.Close()
		br := bufio.NewReader(nc)
		proto.ReadPacket(br)
		proto.ReadPacket(br)
		ok := proto.NewPacket(0x02)
		ok.String("00000000-0000-0000-0000-000000000000")
		ok.String("Eve")
		nc.Write(ok.Frame())
		join := proto.NewPacket(0x01)
		join.Int(1)
		join.Byte(mode)
		join.Byte(0)
		join.Byte(0)
		join.Byte(20)
		join.String("default")
		join.Bool(false)
		nc.Write(join.Frame())
		if brand {
			b := proto.NewPacket(0x3F)
			b.String("MC|Brand")
			b.String("gocraft")
			nc.Write(b.Frame())
		}
		for {
			id, r, err := proto.ReadPacket(br)
			if err != nil {
				return
			}
			mu.Lock()
			ids[id] = true
			if id == 0x17 {
				channels[r.String(20)] = true
			}
			mu.Unlock()
		}
	}()
	return l.Addr().String(), func() (map[int32]bool, map[string]bool) {
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		a, b := map[int32]bool{}, map[string]bool{}
		for k, v := range ids {
			a[k] = v
		}
		for k, v := range channels {
			b[k] = v
		}
		return a, b
	}
}

// A real server may throw out a client that sends what a vanilla one never does, so outside
// creative mode the inventory goes nowhere, except to a server that said it is gocraft's.
func TestInventoryIsOnlySentWhereItBelongs(t *testing.T) {
	cases := []struct {
		name         string
		mode         byte
		brand        bool
		creativeSlot bool // 0x10
		channel      bool // gocraft|inv
	}{
		{"survival on a stranger's server", 0, false, false, false},
		{"survival on a gocraft server", 0, true, false, true},
		{"creative on a stranger's server", 1, false, false, false}, // a stranger hears nothing of ours
		{"creative on a gocraft server", 1, true, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr, sent := fakeServer(t, c.mode, c.brand)
			g := testGame(t)
			if err := g.connect(addr, "Eve"); err != nil {
				t.Fatal(err)
			}
			defer g.leaveServer()
			pumpUntil(t, "the join", func() bool { return g.net.joined && (!c.brand || g.net.gocraft) }, g)
			g.inv[0] = slot{ID: 267, N: 1}
			g.armor[1] = slot{ID: 311, N: 1}
			g.sel = 2
			g.tickNet()
			ids, channels := sent()
			if ids[0x10] != c.creativeSlot {
				t.Errorf("creative inventory packet sent: %v, want %v", ids[0x10], c.creativeSlot)
			}
			if channels["gocraft|inv"] != c.channel {
				t.Errorf("gocraft|inv sent: %v, want %v", channels["gocraft|inv"], c.channel)
			}
			if !ids[0x09] {
				t.Error("the held slot, which every client sends, did not go")
			}
		})
	}
}

// Before the Join Game says what the game mode is, nothing of the inventory goes either.
func TestNoInventoryBeforeTheGameModeIsKnown(t *testing.T) {
	addr, sent := fakeServer(t, 0, false)
	g := testGame(t)
	if err := g.connect(addr, "Eve"); err != nil {
		t.Fatal(err)
	}
	defer g.leaveServer()
	g.creative = true // what connect assumes until the Join Game says otherwise
	g.net.joined = false
	g.tickNet()
	if ids, _ := sent(); ids[0x10] {
		t.Fatal("the inventory went out before the game mode was known")
	}
}
