package server

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"gocraft/proto"
)

func (c *client) say(msg string) {
	p := proto.NewPacket(0x01)
	p.String(msg)
	c.send(p)
}

// chatText reads chat packets until one contains want, and returns its text.
func (c *client) chatText(want string) string {
	for i := 0; i < 50; i++ {
		var j struct {
			Text string `json:"text"`
		}
		json.Unmarshal([]byte(c.until(0x02).String(32767)), &j)
		if strings.Contains(j.Text, want) {
			return j.Text
		}
	}
	c.t.Fatalf("no chat line with %q", want)
	return ""
}

func TestCommandsTalk(t *testing.T) {
	addr := start(t)
	a, b := login(t, addr, "Alice"), login(t, addr, "Bob")
	a.say("/help")
	a.chatText("/give <player>")
	a.say("/list")
	if got := a.chatText("players online"); !strings.Contains(got, "Alice, Bob") || !strings.Contains(got, "There are 2") {
		t.Fatalf("list: %q", got)
	}
	a.say("/say hola a todos")
	b.chatText("[Alice] hola a todos")
	a.say("/me baila")
	b.chatText("* Alice baila")
	a.say("/msg bob ¿qué tal?")
	b.chatText("Alice whispers to you: ¿qué tal?")
	a.chatText("You whisper to Bob: ¿qué tal?")
	a.say("/msg nadie hola")
	a.chatText("No player was found")
	a.say("/frobnicate")
	a.chatText("Unknown command: /frobnicate")
	a.say("/seed")
	a.chatText("Seed: 0")
	a.say("/save-all")
	a.chatText("does not keep a world")
	a.say("/kill")
	a.chatText("creative")
}

func TestCommandTeleport(t *testing.T) {
	addr := start(t)
	a, b := login(t, addr, "Alice"), login(t, addr, "Bob")
	a.say("/tp 10 70 -4.5")
	r := a.until(0x08)
	if x, y, z := r.Double(), r.Double(), r.Double(); x != 10 || y != 70 || z != -4.5 {
		t.Fatalf("teleported to %v %v %v", x, y, z)
	}
	a.say("/tp ~1 ~ ~-2")
	r = a.until(0x08)
	if x, y, z := r.Double(), r.Double(), r.Double(); x != 11 || y != 70 || z != -6.5 {
		t.Fatalf("relative: %v %v %v", x, y, z)
	}
	b.say("/tp alice")
	r = b.until(0x08)
	if x, y, z := r.Double(), r.Double(), r.Double(); x != 11 || y != 70 || z != -6.5 {
		t.Fatalf("to a player: %v %v %v", x, y, z)
	}
	a.say("/tp 1 2")
	a.chatText("Usage: /tp")
	a.say("/tp a b c")
	a.chatText("not coordinates")
	a.say("/tp 1e99 0 0")
	a.chatText("not coordinates")
}

func TestCommandTime(t *testing.T) {
	addr := start(t)
	a, b := login(t, addr, "Alice"), login(t, addr, "Bob")
	for _, d := range []struct {
		cmd  string
		want int64
	}{{"/time set night", 13000}, {"/time add 1000", 14000}, {"/time set 23000", 23000}, {"/time add 2000", 1000}, {"/time set day", 1000}} {
		a.say(d.cmd)
		for _, c := range []*client{a, b} {
			r := c.until(0x03)
			if age, tm := r.Long(), r.Long(); age != 0 || tm != -d.want {
				t.Fatalf("%s: time %d, want %d (a stopped clock)", d.cmd, tm, -d.want)
			}
		}
	}
	a.say("/time set mañana")
	a.chatText("not a time")
	a.say("/time")
	a.chatText("Usage: /time")
	// a player who joins later is told the time that was set
	c := dial(t, addr)
	c.handshake(Protocol, 2)
	p := proto.NewPacket(0)
	p.String("Carla")
	c.send(p)
	r := c.until(0x03)
	if r.Long(); r.Long() != -1000 {
		t.Fatal("a new player was not given the time that was set")
	}
}

func TestCommandGive(t *testing.T) {
	addr := start(t)
	a, b := login(t, addr, "Alice"), login(t, addr, "Bob")
	a.say("/give bob diamond_sword")
	r := b.until(0x2F)
	if win, slotN, id, count, dmg := r.Byte(), int(r.UShort()), r.Short(), r.Byte(), r.Short(); win != 0 || slotN != 36 || id != 276 || count != 1 || dmg != 0 {
		t.Fatalf("set slot: window %d slot %d id %d count %d damage %d", win, slotN, id, count, dmg)
	}
	a.chatText("Gave 1 of diamond_sword to Bob")
	a.say("/give bob minecraft:cobblestone 100")
	// 100 blocks: a stack of 64 in the next hotbar slot and 36 in the one after
	var got [][2]int
	for len(got) < 2 {
		r := b.until(0x2F)
		r.Byte()
		n := int(r.UShort())
		if id := r.Short(); id == 4 {
			got = append(got, [2]int{n, int(r.Byte())})
		}
	}
	if got[0] != [2]int{37, 64} || got[1] != [2]int{38, 36} {
		t.Fatalf("cobblestone went to %v", got)
	}
	a.say("/give bob 4 30") // tops up the 36 to 64 and starts another with 2
	r = b.until(0x2F)
	r.Byte()
	if n, id, count := int(r.UShort()), r.Short(), r.Byte(); n != 38 || id != 4 || count != 64 {
		t.Fatalf("top-up: slot %d id %d count %d", n, id, count)
	}
	a.say("/give bob nosuchitem")
	a.chatText("There is no item called nosuchitem")
	a.say("/give bob stone 0")
	a.chatText("from 1 to 640")
	a.say("/give ghost stone")
	a.chatText("No player was found")
}

// Giving fills stacks first and reports what did not fit.
func TestGiveFillsStacksAndReportsTheRest(t *testing.T) {
	p := newPlayer(nil, 1, "Alguien")
	for i := 9; i < 45; i++ {
		p.inv[i] = slot{id: 1, count: 64}
	}
	p.inv[40] = slot{id: 4, count: 60}
	left, changed := p.give(4, 0, 10) // 4 fill the stack of 60; the other 6 have nowhere to go
	if left != 6 || len(changed) != 1 || changed[0] != 40 || p.inv[40].count != 64 {
		t.Fatalf("left %d changed %v slot %v", left, changed, p.inv[40])
	}
	if left, _ = p.give(4, 0, 6); left != 6 { // the stack is full now, so all 6 are left
		t.Fatalf("a full inventory left %d, want 6", left)
	}
	if left, _ = p.give(4, 5, 3); left != 3 { // another damage value does not stack with it either
		t.Fatalf("left %d", left)
	}
}

// In a survival world /kill kills the named player (or the one who runs it); the health the client is told is 0.
func TestCommandKillInSurvival(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go New(Config{ViewDistance: 2, Survival: true}).Serve(l)
	a, b := login(t, l.Addr().String(), "Alice"), login(t, l.Addr().String(), "Bob")
	a.say("/kill bob")
	for {
		if h := b.until(0x06).Float(); h <= 0 {
			break
		}
	}
	a.say("/kill nadie")
	a.chatText("No player was found")
}

func TestCoordinates(t *testing.T) {
	if v, ok := coord("~2.5", 10); !ok || v != 12.5 {
		t.Fatalf("relative coordinate %v %v", v, ok)
	}
	if v, ok := coord("~", 10); !ok || v != 10 {
		t.Fatalf("a bare ~ is %v", v)
	}
	if _, ok := coord("NaN", 0); ok {
		t.Fatal("NaN was accepted as a coordinate")
	}
	if _, ok := coord("~x", 0); ok {
		t.Fatal("~x was accepted")
	}
}
