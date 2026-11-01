package client

import (
	"testing"

	"gocraft/proto"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func openKind(t *testing.T, g *game, id byte, kind string, n byte) {
	w := newPacket(0x2D)
	w.Byte(id)
	w.String(kind)
	w.String(`{"text":"Ventana"}`)
	w.Byte(n)
	decodeFrame(t, w, nil)(g)
}

// The game builds a crafting table, an enchanting table, an anvil and a brewing stand by their kind: the slot
// count in Open Window (0 for the ones that are not an inventory) is not believed there.
func TestSpecialWindowsTakeTheirSlotsFromTheirKind(t *testing.T) {
	for _, d := range []struct {
		kind    string
		sent, n int
	}{
		{"minecraft:crafting_table", 0, 10}, {kindEnchant, 0, 2}, {kindAnvil, 0, 3}, {kindBrewing, 4, 4},
		{"minecraft:chest", 27, 27}, {"minecraft:furnace", 3, 3},
	} {
		g, _ := windowGame(t)
		openKind(t, g, 5, d.kind, byte(d.sent))
		if g.swin == nil || g.swin.n != d.n {
			t.Fatalf("%s sent with %d: n %v, want %d", d.kind, d.sent, g.swin, d.n)
		}
		slots, _ := g.swin.layout()
		if len(slots) != d.n+36 {
			t.Fatalf("%s: %d slots in the layout, want %d", d.kind, len(slots), d.n+36)
		}
	}
}

// The slots of each window are where GuiEnchantment, GuiRepair and GuiBrewingStand put them.
func TestSpecialWindowLayouts(t *testing.T) {
	for _, d := range []struct {
		kind  string
		slots [][2]int
	}{
		{kindEnchant, [][2]int{{15, 47}, {35, 47}}},
		{kindAnvil, [][2]int{{27, 47}, {76, 47}, {134, 47}}},
		{kindBrewing, [][2]int{{56, 46}, {79, 53}, {102, 46}, {79, 17}}},
	} {
		g, _ := windowGame(t)
		openKind(t, g, 5, d.kind, 0)
		slots, h := g.swin.layout()
		if h != 166 {
			t.Fatalf("%s: panel %d high", d.kind, h)
		}
		for i, want := range d.slots {
			if slots[i].x != want[0] || slots[i].y != want[1] {
				t.Fatalf("%s slot %d at (%d, %d), want %v", d.kind, i, slots[i].x, slots[i].y, want)
			}
		}
		if p := slots[len(d.slots)]; p.x != 8 || p.y != 84 { // the player's inventory under them
			t.Fatalf("%s: the inventory starts at (%d, %d)", d.kind, p.x, p.y)
		}
	}
}

// A button of the enchanting table sends Enchant Item with its index, when the player has the levels and the
// lapis for it; a button with nothing to offer sends nothing.
func TestEnchantButtonsSendEnchantItem(t *testing.T) {
	g, c := windowGame(t)
	openKind(t, g, 7, kindEnchant, 0)
	w := g.swin
	w.slots[0] = slot{ID: 276, N: 1}
	w.slots[1] = slot{ID: 351, Dmg: 4, N: 2}
	w.props[0], w.props[1], w.props[2] = 8, 17, 30
	g.xpLevel = 20
	y := func(i int) float32 { return float32(14+19*i) + 5 }
	if !g.windowButton(100, y(1)) { // the second needs 2 lapis and 17 levels
		t.Fatal("the second button took no click")
	}
	sent := drain(t, c)
	if len(sent) != 1 || sent[0].id != 0x11 {
		t.Fatalf("sent %v", sent)
	}
	if win, b := int8(sent[0].r.Byte()), sent[0].r.Byte(); win != 7 || b != 1 {
		t.Fatalf("Enchant Item window %d button %d, want 7 and 1", win, b)
	}
	g.windowButton(100, y(2))    // 3 lapis and 30 levels: not enough of either
	g.windowButton(100, y(0)+60) // under the last button
	g.windowButton(10, y(0))     // beside the buttons
	if sent := drain(t, c); len(sent) != 0 {
		t.Fatalf("an unavailable button sent %d packets", len(sent))
	}
	g.xpLevel = 40 // plenty of levels, but 2 lapis is too few for the third button
	g.windowButton(100, y(2))
	if sent := drain(t, c); len(sent) != 0 {
		t.Fatalf("a button with too little lapis sent %d packets", len(sent))
	}
	g.xpLevel = 20
	g.creative = true // a creative player needs neither
	g.windowButton(100, y(2))
	if sent := drain(t, c); len(sent) != 1 || sent[0].id != 0x11 {
		t.Fatalf("creative sent %v", sent)
	}
	g.swin.kind = kindAnvil
	if g.windowButton(100, y(0)) {
		t.Fatal("an anvil has no enchant buttons")
	}
}

func TestEnchantButtonRegions(t *testing.T) {
	for _, d := range []struct {
		x, y float32
		want int
	}{{60, 14, 0}, {167, 32, 0}, {100, 33, 1}, {100, 52, 2}, {100, 71, -1}, {59, 20, -1}, {168, 20, -1}, {100, 13, -1}} {
		if got := enchantButtonAt(d.x, d.y); got != d.want {
			t.Fatalf("(%v, %v): button %d, want %d", d.x, d.y, got, d.want)
		}
	}
}

// The anvil's name field takes the item's name, sends what is typed as MC|ItemName and sends the item's own name
// as empty.
func TestAnvilNameField(t *testing.T) {
	g, c := windowGame(t)
	g.net.joined = true
	openKind(t, g, 3, kindAnvil, 0)
	if !g.anvilTyping() || !g.typing() {
		t.Fatal("the anvil does not own the keyboard")
	}
	g.anvilChar('x') // no item: the field is disabled
	if g.swin.name != "" || len(drain(t, c)) != 0 {
		t.Fatal("a character went in with no item")
	}
	g.swin.slots[0] = slot{ID: 267, N: 1} // an iron sword
	g.syncAnvilName()
	if g.swin.name != g.displayName(slot{ID: 267, N: 1}) {
		t.Fatalf("name %q", g.swin.name)
	}
	payload := func() string {
		var last string
		for _, p := range drain(t, c) {
			if p.id == 0x17 && p.r.String(32) == "MC|ItemName" {
				last = p.r.String(32767)
			}
		}
		return last
	}
	if got := payload(); got != "" { // the item's own name goes as empty
		t.Fatalf("the item's own name was sent as %q", got)
	}
	g.anvilChar('!')
	if got := payload(); got != g.displayName(slot{ID: 267, N: 1})+"!" {
		t.Fatalf("typed name %q", got)
	}
	g.anvilKey(glfw.KeyBackspace)
	g.anvilKey(glfw.KeyBackspace)
	if got := payload(); got != g.displayName(slot{ID: 267, N: 1})[:len(g.displayName(slot{ID: 267, N: 1}))-1] {
		t.Fatalf("after two backspaces %q", got)
	}
	for i := 0; i < 60; i++ {
		g.anvilChar('a')
	}
	if n := len([]rune(g.swin.name)); n != 40 {
		t.Fatalf("the field holds %d characters, want 40", n)
	}
	g.anvilKey(glfw.KeyEscape)
	if g.swin != nil {
		t.Fatal("Escape did not close the anvil")
	}
}

// The enchantment hint in a tooltip is the game's: the name, the level in numerals and the clue text.
func TestEnchantClue(t *testing.T) {
	g, _ := windowGame(t)
	if got := g.enchantClue(16, 2); got != "Sharpness II . . . ?" {
		t.Fatalf("clue %q", got)
	}
	if got := g.enchantClue(6, 1); got != "Aqua Affinity I . . . ?" {
		t.Fatalf("clue %q", got)
	}
	if got := g.enchantClue(99, 1); got != "" {
		t.Fatalf("an unknown enchantment gave %q", got)
	}
	_ = proto.PackPos
}
