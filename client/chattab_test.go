package client

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"

	"gocraft/proto"
)

func tabAnswer(words ...string) *proto.Writer {
	w := newPacket(0x3A)
	w.VarInt(int32(len(words)))
	for _, s := range words {
		w.String(s)
	}
	return w
}

// Tab asks the server for what follows the line; the answer fills in what all the answers share, or if that
// adds nothing walks through them, one a Tab, printing the list first.
func TestChatTabCompletion(t *testing.T) {
	g, c := windowGame(t)
	g.pos = vec3{0.5, 30, 0.5} // looking at nothing
	g.openChat("/gam")
	drain(t, c)
	g.typingKey(glfw.KeyTab)
	ps := drain(t, c)
	if len(ps) != 1 || ps[0].id != 0x14 || ps[0].r.String(256) != "/gam" || ps[0].r.Bool() {
		t.Fatalf("sent %v", ps)
	}
	decodeFrame(t, tabAnswer("/gamemode", "/gamerule"), nil)(g)
	if g.chatInput != "/game" { // what both start with
		t.Fatalf("line %q, want /game", g.chatInput)
	}
	g.typingKey(glfw.KeyTab)
	drain(t, c)
	decodeFrame(t, tabAnswer("/gamemode", "/gamerule"), nil)(g)
	if g.chatInput != "/gamemode" {
		t.Fatalf("after the walk starts: %q", g.chatInput)
	}
	if len(g.chat) == 0 || plainRuns(g.chat[len(g.chat)-1].runs) != "/gamemode, /gamerule" {
		t.Fatal("the list was not printed")
	}
	g.typingKey(glfw.KeyTab) // now the walk goes on without asking
	if g.chatInput != "/gamerule" || len(drain(t, c)) != 0 {
		t.Fatalf("second answer %q", g.chatInput)
	}
	g.typingKey(glfw.KeyTab)
	if g.chatInput != "/gamemode" {
		t.Fatalf("it wraps: %q", g.chatInput)
	}

	g.openChat("hola Ste") // a word after others: only it is completed
	g.typingKey(glfw.KeyTab)
	drain(t, c)
	decodeFrame(t, tabAnswer("Steve"), nil)(g)
	if g.chatInput != "hola Steve" {
		t.Fatalf("name: %q", g.chatInput)
	}
	g.openChat("x")
	g.typingKey(glfw.KeyTab)
	drain(t, c)
	g.onChar(nil, 'y') // typing ends the wait: a late answer changes nothing
	decodeFrame(t, tabAnswer("xylophone"), nil)(g)
	if g.chatInput != "xy" {
		t.Fatalf("a late answer wrote %q", g.chatInput)
	}
}

func TestCommonPrefix(t *testing.T) {
	for in, want := range map[string]string{"": "", "a": "a", "ab,abc": "ab", "abc,abd,ab": "ab", "x,y": ""} {
		var ss []string
		if in != "" {
			ss = splitComma(in)
		}
		if got := commonPrefix(ss); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out, cur = append(out, cur), ""
		} else {
			cur += string(r)
		}
	}
	return append(out, cur)
}
