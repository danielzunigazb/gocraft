package client

import (
	"bufio"
	"bytes"
	"gocraft/proto"
	"net"
	"testing"
	"time"

	"gocraft/server"
)

// pumpUntil runs the games' network queues until cond holds or a few seconds pass.
func pumpUntil(t *testing.T, what string, cond func() bool, games ...*game) {
	t.Helper()
	// An upper bound, not a wait: generating a world under the race detector on a loaded machine takes seconds.
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		for _, g := range games {
			g.pumpNet()
		}
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestTwoClientsSeeEachOther(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go server.New(server.Config{ViewDistance: 2}).Serve(l)

	a, b := testGame(t), testGame(t)
	if err := a.connect(l.Addr().String(), "Alice"); err != nil {
		t.Fatal(err)
	}
	defer a.leaveServer()
	pumpUntil(t, "Alice's first columns", func() bool { return len(a.net.received) >= 25 }, a)
	// The flat world: bedrock at y=0 and grass at y=3 under the spawn.
	if got := a.world.Block(0, 0, 0) >> 4; got != 7 {
		t.Fatalf("block at 0,0,0 = %d, want bedrock", got)
	}
	if got := a.world.Block(0, 3, 0) >> 4; got != 2 {
		t.Fatalf("block at 0,3,0 = %d, want grass", got)
	}

	if err := b.connect(l.Addr().String(), "Bob"); err != nil {
		t.Fatal(err)
	}
	defer b.leaveServer()
	pumpUntil(t, "Alice and Bob in each other's view", func() bool { return len(a.net.remote) == 1 && len(b.net.remote) == 1 }, a, b)
	for _, p := range b.net.remote {
		if p.name != "Alice" {
			t.Fatalf("Bob sees %q, want Alice", p.name)
		}
	}

	// Alice walks: Bob sees her move.
	a.feet = vec3{10.5, 4, 3.5}
	a.yaw = 1
	for i := 0; i < 3; i++ {
		a.tickNet()
	}
	pumpUntil(t, "Alice's new position", func() bool {
		for _, p := range b.net.remote {
			if p.target[0] > 10 {
				return true
			}
		}
		return false
	}, a, b)

	// Alice places a block; Bob's world changes. Bob must have the column first.
	pumpUntil(t, "Bob's columns", func() bool { return len(b.net.received) >= 25 }, a, b)
	a.inv[0] = blockSlot(1<<4, 64)
	a.sel = 0
	a.tickNet() // tells the server what Alice holds
	time.Sleep(50 * time.Millisecond)
	a.sendPlace(2, 3, 2, dUp)
	pumpUntil(t, "the block on Bob's side", func() bool { return b.world.Block(2, 4, 2)>>4 == 1 }, a, b)

	// Chat goes to both.
	a.sendChat("hola")
	pumpUntil(t, "chat on Bob's side", func() bool {
		for _, l := range b.chat {
			if l.text == "<Alice> hola" {
				return true
			}
		}
		return false
	}, a, b)

	// Alice leaves: Bob no longer sees her.
	a.leaveServer()
	pumpUntil(t, "Alice gone", func() bool { return len(b.net.remote) == 0 }, b)
}

func TestConnectRefusedShowsReason(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go server.New(server.Config{}).Serve(l)
	g := testGame(t)
	if err := g.connect(l.Addr().String(), "bad name!"); err == nil {
		t.Fatal("an invalid name was accepted")
	}
}

// A server that compresses packets (as real 1.8 servers do), sending its columns in Map Chunk Bulk.
func TestCompressedServerAndBulkChunks(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan int32, 8) // ids of the packets the fake server received after login
	go func() {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		defer nc.Close()
		br := bufio.NewReader(nc)
		proto.ReadPacket(br) // handshake
		proto.ReadPacket(br) // login start
		sc := proto.NewPacket(0x03)
		sc.VarInt(64)
		nc.Write(sc.Frame())
		const thr = 64
		send := func(w *proto.Writer) { nc.Write(w.FrameZ(thr)) }
		ok := proto.NewPacket(0x02)
		ok.String("00000000-0000-0000-0000-000000000000")
		ok.String("Carol")
		send(ok)
		join := proto.NewPacket(0x01)
		join.Int(7)
		join.Byte(0)
		join.Byte(0)
		join.Byte(0)
		join.Byte(20)
		join.String("default")
		join.Bool(false)
		send(join)
		// Two full stone columns, one section each, block light 0 and sky light 15, biomes.
		bulk := proto.NewPacket(0x26)
		bulk.Bool(true)
		bulk.VarInt(2)
		for cx := int32(0); cx < 2; cx++ {
			bulk.Int(cx)
			bulk.Int(0)
			bulk.UShort(1)
		}
		for i := 0; i < 2; i++ {
			for b := 0; b < 4096; b++ {
				bulk.Byte(1 << 4) // stone is state 0x0010, low byte first
				bulk.Byte(0)
			}
			bulk.Bytes(make([]byte, 2048))
			for b := 0; b < 2048; b++ {
				bulk.Byte(0xFF)
			}
			bulk.Bytes(make([]byte, 256))
		}
		send(bulk)
		// Compression is changed in play too: a bigger threshold from now on.
		sc2 := proto.NewPacket(0x46)
		sc2.VarInt(512)
		send(sc2)
		chat := proto.NewPacket(0x02)
		chat.String(`{"text":"hola desde el servidor"}`)
		chat.Byte(0)
		nc.Write(chat.FrameZ(512))
		for {
			id, _, err := proto.ReadPacketZ(br, 64)
			if err != nil {
				return
			}
			got <- id
		}
	}()
	g := testGame(t)
	if err := g.connect(l.Addr().String(), "Carol"); err != nil {
		t.Fatal(err)
	}
	defer g.leaveServer()
	pumpUntil(t, "the bulk columns and the chat line", func() bool {
		return len(g.net.received) == 2 && len(g.chat) == 1
	}, g)
	if st := g.world.Block(5, 3, 5); st>>4 != 1 {
		t.Fatalf("block 5,3,5 = %d, want stone", st>>4)
	}
	if st := g.world.Block(20, 3, 5); st>>4 != 1 {
		t.Fatalf("block 20,3,5 (second column) = %d, want stone", st>>4)
	}
	if g.chat[0].text != "hola desde el servidor" {
		t.Fatalf("chat %q", g.chat[0].text)
	}
	// What the client sends is compressed too, with the threshold the server set: 512 in play.
	g.feet = vec3{1, 4, 1}
	g.tickNet()
	seen := map[int32]bool{} // the brand (0x17) and the settings (0x15) come first, as in the game
	for {
		select {
		case id := <-got:
			seen[id] = true
			if seen[0x17] && seen[0x15] && (seen[0x06] || seen[0x04] || seen[0x10]) {
				return
			}
		case <-time.After(3 * time.Second):
			t.Fatal("the fake server never decoded a packet from the client")
		}
	}
}

// The server says what the player carries: Window Items fills the hotbar, the main inventory and the
// armour, and an enchanted stack (with its NBT) does not derail the stacks after it.
func TestWindowItemsFillTheInventory(t *testing.T) {
	g := testGame(t)
	g.net = &netState{}
	w := proto.NewPacket(0x30)
	w.Byte(0)
	w.UShort(45)
	for i := 0; i < 45; i++ {
		switch i {
		case 5: // a diamond helmet
			w.UShort(310)
			w.Byte(1)
			w.UShort(0)
			w.Byte(0)
		case 9: // stone
			w.UShort(1)
			w.Byte(64)
			w.UShort(0)
			w.Byte(0)
		case 36: // an enchanted sword: the NBT is {ench:[{id:16s,lvl:2s}]}
			w.UShort(276)
			w.Byte(1)
			w.UShort(0)
			w.Byte(10) // compound, root name ""
			w.UShort(0)
			w.Byte(9) // list "ench" of compounds
			w.UShort(4)
			w.Bytes([]byte("ench"))
			w.Byte(10)
			w.Int(1)
			w.Byte(2)
			w.UShort(2)
			w.Bytes([]byte("id"))
			w.UShort(16)
			w.Byte(2)
			w.UShort(3)
			w.Bytes([]byte("lvl"))
			w.UShort(2)
			w.Byte(0) // end of the list entry
			w.Byte(0) // end of the root
		case 40: // a golden apple after it
			w.UShort(322)
			w.Byte(3)
			w.UShort(1)
			w.Byte(0)
		default:
			w.UShort(0xFFFF)
		}
	}
	id, r, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(w.Frame())))
	if err != nil || id != 0x30 {
		t.Fatal(id, err)
	}
	f := parsePacket(id, r)
	if f == nil {
		t.Fatal("window items were not understood")
	}
	f(g)
	if g.inv[0].ID != 276 || g.inv[4].ID != 322 || g.inv[4].N != 3 || g.inv[4].Dmg != 1 {
		t.Fatalf("hotbar %v %v: the stack after the enchanted sword was lost", g.inv[0], g.inv[4])
	}
	if g.inv[9].ID != 1 || g.inv[9].N != 64 || g.armor[0].ID != 310 {
		t.Fatalf("main %v armour %v", g.inv[9], g.armor[0])
	}
	if !g.inv[1].empty() {
		t.Fatalf("slot 1 should be empty, is %v", g.inv[1])
	}
}

// A new connection starts with nothing in the hands: no blocks of gocraft's own.
func TestJoiningStartsWithAnEmptyInventory(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go server.New(server.Config{ViewDistance: 1}).Serve(l)
	g := testGame(t)
	g.inv[0] = blockSlot(1<<4, 64) // what a single-player game left behind
	if err := g.connect(l.Addr().String(), "Dave"); err != nil {
		t.Fatal(err)
	}
	defer g.leaveServer()
	for i, s := range g.inv {
		if !s.empty() {
			t.Fatalf("slot %d holds %v on joining", i, s)
		}
	}
}
