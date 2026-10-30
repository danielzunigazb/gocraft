package client

import (
	"testing"

	"gocraft/proto"
)

func objPacket(name string, mode byte, display string) *proto.Writer {
	w := newPacket(0x3B)
	w.String(name)
	w.Byte(mode)
	if mode != 1 {
		w.String(display)
		w.String("integer")
	}
	return w
}

func scorePacket(entry string, remove bool, obj string, v int32) *proto.Writer {
	w := newPacket(0x3C)
	w.String(entry)
	if remove {
		w.Byte(1)
	} else {
		w.Byte(0)
	}
	w.String(obj)
	if !remove {
		w.VarInt(v)
	}
	return w
}

func teamPacket(name string, mode byte, prefix, suffix string, colour int8, players ...string) *proto.Writer {
	w := newPacket(0x3E)
	w.String(name)
	w.Byte(mode)
	if mode == 0 || mode == 2 {
		w.String(name)
		w.String(prefix)
		w.String(suffix)
		w.Byte(3)
		w.String("always")
		w.Byte(byte(colour))
	}
	if mode == 0 || mode == 3 || mode == 4 {
		w.VarInt(int32(len(players)))
		for _, p := range players {
			w.String(p)
		}
	}
	return w
}

func displayPacket(slot byte, name string) *proto.Writer {
	w := newPacket(0x3D)
	w.Byte(slot)
	w.String(name)
	return w
}

// The sidebar lists the scores from the top down, highest first; equal scores go by name backwards (the
// game's own comparator), only fifteen are kept, and entries starting with # are hidden.
func TestSidebarOrderAndLimit(t *testing.T) {
	g, _ := windowGame(t)
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(objPacket("lobby", 0, "§6§lUNIVERSO"))
	apply(displayPacket(1, "lobby"))
	apply(scorePacket("§1", false, "lobby", 5))
	apply(scorePacket("Kills", false, "lobby", 5))
	apply(scorePacket("Coins", false, "lobby", 9))
	apply(scorePacket("#hidden", false, "lobby", 100))
	sb := g.net.sb
	ls := sb.sidebarLines("lobby")
	// ascending, so the bottom of the panel first; equal scores go by name backwards (§ sorts after K)
	want := []string{"§1", "Kills", "Coins"}
	if len(ls) != 3 {
		t.Fatalf("lines %v", ls)
	}
	for i, w := range want {
		if ls[i].name != w {
			t.Fatalf("line %d is %q, want %q (all: %v)", i, ls[i].name, w, ls)
		}
	}
	for i := 0; i < 20; i++ {
		apply(scorePacket("p"+string(rune('a'+i)), false, "lobby", int32(i+10)))
	}
	if ls := sb.sidebarLines("lobby"); len(ls) != 15 || ls[len(ls)-1].value != 29 || ls[0].value != 15 {
		t.Fatalf("the sidebar keeps %d lines from %d to %d, want the 15 highest", len(ls), ls[0].value, ls[len(ls)-1].value)
	}
	apply(scorePacket("pa", true, "lobby", 0))
	if _, ok := sb.scores["lobby"]["pa"]; ok {
		t.Fatal("a removed score stayed")
	}
	if o := sb.sidebarObjective("me"); o == nil || o.display != "§6§lUNIVERSO" {
		t.Fatalf("sidebar objective %+v", o)
	}
	apply(objPacket("lobby", 1, ""))
	if sb.sidebarObjective("me") != nil {
		t.Fatal("a removed objective is still shown")
	}
}

// Teams give an entry its prefix and suffix, and a colour gives its members a sidebar of their own.
func TestTeamsFormatNamesAndPickTheSidebar(t *testing.T) {
	g, c := windowGame(t)
	c.name = "000danielz"
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(teamPacket("t1", 0, "§c[ADMIN] ", " §7*", -1, "Herobrine"))
	if got := g.net.sb.format("Herobrine"); got != "§c[ADMIN] Herobrine §7*" {
		t.Fatalf("formatted %q", got)
	}
	apply(teamPacket("t1", 4, "", "", 0, "Herobrine"))
	if got := g.net.sb.format("Herobrine"); got != "Herobrine" {
		t.Fatalf("after leaving the team %q", got)
	}
	apply(teamPacket("t1", 3, "", "", 0, "Herobrine"))
	apply(teamPacket("t1", 2, "§a", "", -1))
	if got := g.net.sb.format("Herobrine"); got != "§aHerobrine" {
		t.Fatalf("after an update %q", got)
	}
	apply(objPacket("common", 0, "Comun"))
	apply(objPacket("red", 0, "Equipo rojo"))
	apply(displayPacket(1, "common"))
	apply(displayPacket(3+12, "red")) // chat colour 12 is red
	if o := g.net.sb.sidebarObjective("000danielz"); o.name != "common" {
		t.Fatalf("a player with no team sees %q", o.name)
	}
	apply(teamPacket("reds", 0, "§c", "", 12, "000danielz"))
	if o := g.net.sb.sidebarObjective("000danielz"); o.name != "red" {
		t.Fatalf("a red player sees %q, want their team's sidebar", o.name)
	}
	apply(teamPacket("reds", 1, "", "", 0))
	if o := g.net.sb.sidebarObjective("000danielz"); o.name != "common" {
		t.Fatalf("after the team went, %q", o.name)
	}
}
