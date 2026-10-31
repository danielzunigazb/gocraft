package client

import (
	"bufio"
	"bytes"
	"net"
	"testing"
	"time"

	"gocraft/proto"
	"gocraft/world"
)

func chunkFrame(cx, cz int32) []byte {
	data := make([]byte, 8192+2048+2048+256) // one section with sky light and biomes
	w := newPacket(0x21)
	w.Int(cx)
	w.Int(cz)
	w.Bool(true)
	w.UShort(1)
	w.VarInt(int32(len(data)))
	w.Bytes(data)
	return w.Frame()
}

func queueClient() *netClient {
	c := &netClient{inbox: make(chan func(*game), 64), heavy: make(chan heavyMsg, 64), out: make(chan []byte, 64), done: make(chan struct{})}
	c.threshold.Store(-1)
	return c
}

// A proxy switches the player to another server with Respawn: columns read before it belong to a world that is
// gone and are dropped, and columns after it wait until the game has begun the new world.
func TestColumnsOfAnOldWorldDoNotReachTheNewOne(t *testing.T) {
	g := testGame(t)
	c := queueClient()
	g.net = &netState{c: c, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	pr, pw := net.Pipe()
	go c.readLoop(bufio.NewReader(pr))
	defer pr.Close()
	go func() {
		pw.Write(chunkFrame(1, 1)) // the old server's
		pw.Write(respawn(0, 0).Frame())
		pw.Write(chunkFrame(2, 2)) // the new one's
	}()
	deadline := time.Now().Add(2 * time.Second)
	for len(c.inbox) < 1 || len(c.heavy) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the reader queued %d cheap and %d heavy packets", len(c.inbox), len(c.heavy))
		}
		time.Sleep(time.Millisecond)
	}
	// the Respawn has not been applied yet: the old world's column may land in the old world, but the new
	// world's column must wait
	g.pumpHeavy(time.Second)
	if g.net.received[world.ChunkPos{X: 2, Z: 2}] {
		t.Fatal("the new world's column was applied before the game began the new world: the Respawn would erase it")
	}
	g.pumpLight() // the Respawn
	g.pumpHeavy(time.Second)
	if g.world.Loaded() != 1 || !g.net.received[world.ChunkPos{X: 2, Z: 2}] || g.net.received[world.ChunkPos{X: 1, Z: 1}] {
		t.Fatalf("loaded %d, received %v: only the new world's column should be there", g.world.Loaded(), g.net.received)
	}
}

// Columns take their time: the packets that move players are not held up behind them.
func TestHeavyPacketsDoNotHoldUpTheCheapOnes(t *testing.T) {
	g := testGame(t)
	c := queueClient()
	g.net = &netState{c: c, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	slow := 0
	for i := 0; i < 20; i++ {
		c.heavy <- heavyMsg{func(*game) { slow++; time.Sleep(3 * time.Millisecond) }, 0}
	}
	moved := 0
	for i := 0; i < 50; i++ {
		c.inbox <- func(*game) { moved++ }
	}
	g.pumpLight()
	if moved != 50 {
		t.Fatalf("%d of 50 cheap packets applied: a burst of columns delayed them", moved)
	}
	g.pumpHeavy(5 * time.Millisecond)
	if slow == 0 || slow > 4 {
		t.Fatalf("%d heavy packets in a 5 ms budget", slow)
	}
	done := slow
	g.pumpHeavy(time.Second)
	if slow != 20 || done >= 20 {
		t.Fatalf("slow %d after the first pump %d: the rest should follow", slow, done)
	}
}

// Everything waiting to be sent goes out in one write.
func TestWritesAreCoalesced(t *testing.T) {
	pr, pw := net.Pipe()
	c := queueClient()
	c.conn, c.w = pw, pw
	total := 0
	for i := 0; i < 6; i++ {
		f := newPacket(0x04).Frame()
		w := newPacket(0x04)
		w.Double(1)
		w.Double(2)
		w.Double(3)
		w.Bool(true)
		f = w.Frame()
		total += len(f)
		c.out <- f
	}
	go c.writeLoop()
	defer c.close()
	buf := make([]byte, 4096)
	pr.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := pr.Read(buf)
	if err != nil || n != total {
		t.Fatalf("the first read got %d bytes (err %v), want all %d from six packets in one write", n, err, total)
	}
	// and they are still six packets
	br := bufio.NewReader(bytesReader(buf[:n]))
	for i := 0; i < 6; i++ {
		if id, _, err := proto.ReadPacket(br); err != nil || id != 0x04 {
			t.Fatalf("packet %d: id 0x%02x err %v", i, id, err)
		}
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// Between two ticks the angles of another player are interpolated, as their position is: a head turning at
// a steady rate looks steady at any frame rate.
func TestRemoteAnglesAreInterpolatedBetweenTicks(t *testing.T) {
	g := testGame(t)
	g.net = &netState{c: queueClient(), received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	p := &remotePlayer{eid: 1, pos: vec3{5, 5, 5}, prev: vec3{5, 5, 5}, target: vec3{5, 5, 5}}
	g.net.remote[1] = p
	// the server turns the player 60 degrees: it takes three ticks, a third of the way each time
	p.moveTo(p.pos, 60, 30)
	p.thead = 60
	var seen []float32
	for i := 0; i < 3; i++ {
		g.tickRemotes()
		seen = append(seen, p.headYaw)
	}
	want := []float32{20, 40, 60}
	for i, w := range want {
		if d := seen[i] - w; d > 0.01 || d < -0.01 {
			t.Fatalf("tick %d: head yaw %v, want %v (all %v)", i+1, seen[i], w, seen)
		}
	}
	if p.prevHead != 40 {
		t.Fatalf("the angle a tick ago is %v, want 40", p.prevHead)
	}
	g.partial = 0.5
	groups := map[string][]vertex{}
	g.emitRemote(groups, p) // must not panic and must draw
	if skin, _ := g.skinFor(p); len(groups[skin]) == 0 {
		t.Fatal("nothing drawn")
	}
}
