package client

import (
	"bufio"
	"bytes"
	"testing"

	"gocraft/proto"
	"gocraft/world"
)

// sent is one packet the client put on the wire.
type sent struct {
	id int32
	r  *proto.Reader
}

func drain(t *testing.T, c *netClient) []sent {
	t.Helper()
	var out []sent
	for len(c.out) > 0 {
		id, r, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(<-c.out)))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, sent{id, r})
	}
	return out
}

// clickSent is a Click Window as the server reads it.
type clickSent struct {
	win           int8
	slot          int16
	button        byte
	action        int16
	mode          byte
	id            int16
	n             byte
	dmg           int16
	tag           []byte
	hasItem       bool
	remainingTail int
}

func readClicks(t *testing.T, c *netClient) []clickSent {
	t.Helper()
	var out []clickSent
	for _, p := range drain(t, c) {
		if p.id != 0x0E {
			continue
		}
		r := p.r
		k := clickSent{win: int8(r.Byte()), slot: r.Short(), button: r.Byte(), action: r.Short(), mode: r.Byte()}
		if k.id = r.Short(); k.id >= 0 {
			k.hasItem, k.n, k.dmg = true, r.Byte(), r.Short()
		}
		k.tag = append([]byte(nil), r.Bytes(r.Len())...)
		out = append(out, k)
	}
	return out
}

func netPlayerGame(t *testing.T) (*game, *netClient) {
	g, c := windowGame(t)
	g.net.received[world.ChunkPos{X: 0, Z: 0}] = true
	g.net.joined = true
	return g, c
}

// Opening the player's window says so ("open inventory" is a Client Status 2); closing it sends Close Window 0 and
// leaves what was in the grid and on the mouse for the server to throw.
func TestPlayersWindowOnAServerOpensAndCloses(t *testing.T) {
	g, c := netPlayerGame(t)
	g.openContainer(contPlayer)
	ps := drain(t, c)
	if len(ps) != 1 || ps[0].id != 0x16 || ps[0].r.VarInt() != 2 {
		t.Fatalf("opening sent %v", ps)
	}
	g.cont.grid[0] = slot{ID: planks, N: 3}
	g.cursor = slot{ID: diamond, N: 2}
	g.closeContainer()
	ps = drain(t, c)
	if len(ps) != 1 || ps[0].id != 0x0D || ps[0].r.Byte() != 0 {
		t.Fatalf("closing sent %v", ps)
	}
	if len(g.items) != 0 || !g.cursor.empty() || g.screen == screenContainer {
		t.Fatalf("%d items thrown, cursor %v, screen %v: the server throws what a closed window held", len(g.items), g.cursor, g.screen)
	}
}

// Every click is sent with the window's slot number, the next transaction number and the stack the click was
// about as the game saw it, tag and all, and the tag moves with the stack.
func TestPlayersWindowClicksAreSentAsTheGameSendsThem(t *testing.T) {
	g, c := netPlayerGame(t)
	tag := []byte{10, 0, 0, 0}
	g.inv[0] = slot{ID: stick, N: 5} // the hotbar's first slot is window slot 36
	g.net.extra[36] = itemExtra{nbt: tag}
	g.openContainer(contPlayer)
	drain(t, c)

	g.clickSlot(inv(g, 0), false, false)
	if !g.inv[0].empty() || g.cursor != (slot{ID: stick, N: 5}) || !bytes.Equal(g.cursorX.nbt, tag) || g.net.extra[36].nbt != nil {
		t.Fatalf("picked up: slot %v cursor %v tag %v left behind %v", g.inv[0], g.cursor, g.cursorX.nbt, g.net.extra[36].nbt)
	}
	g.clickSlot(inv(g, 12), true, false) // right click on an empty slot: one goes down
	if g.inv[12] != (slot{ID: stick, N: 1}) || g.cursor.N != 4 || !bytes.Equal(g.net.extra[12].nbt, tag) {
		t.Fatalf("one down: slot %v cursor %v tag %v", g.inv[12], g.cursor, g.net.extra[12].nbt)
	}
	ks := readClicks(t, c)
	if len(ks) != 2 {
		t.Fatalf("%d clicks sent", len(ks))
	}
	if k := ks[0]; k.win != 0 || k.slot != 36 || k.button != 0 || k.action != 1 || k.mode != 0 || k.id != stick || k.n != 5 || !bytes.Equal(k.tag, tag) {
		t.Fatalf("first click %+v", k)
	}
	if k := ks[1]; k.win != 0 || k.slot != 12 || k.button != 1 || k.action != 2 || k.mode != 0 || k.hasItem {
		t.Fatalf("second click %+v: the slot was empty, so no item", k)
	}
}

// A shift-click carries the stack that moved, a click outside sends -999 and throws nothing here, and the grid
// and armour are the window's slots 1 to 8.
func TestPlayersWindowShiftOutsideAndGridSlots(t *testing.T) {
	g, c := netPlayerGame(t)
	g.inv[9] = slot{ID: cobble, N: 10}
	g.openContainer(contPlayer)
	drain(t, c)
	g.clickSlot(inv(g, 9), false, true)
	if g.inv[0] != (slot{ID: cobble, N: 10}) || !g.inv[9].empty() {
		t.Fatalf("shift-click: hotbar %v main %v", g.inv[0], g.inv[9])
	}
	g.cursor = slot{ID: diamond, N: 3}
	g.windowClick(g.localMap(), -999, 1, 0)
	if g.cursor.N != 2 || len(g.items) != 0 {
		t.Fatalf("outside: cursor %v, %d items thrown here", g.cursor, len(g.items))
	}
	g.cursor = slot{ID: ironHelmet, N: 1}
	g.clickSlot(slotRef{&g.armor[0], kArmor, 0}, false, false)
	g.cursor = slot{ID: planks, N: 1}
	g.clickSlot(slotRef{&g.cont.grid[3], kGrid, 3}, false, false)
	ks := readClicks(t, c)
	if len(ks) != 4 {
		t.Fatalf("%d clicks", len(ks))
	}
	if k := ks[0]; k.slot != 9 || k.mode != 1 || k.id != cobble || k.n != 10 {
		t.Fatalf("shift %+v", k)
	}
	if k := ks[1]; k.slot != -999 || k.button != 1 || k.hasItem {
		t.Fatalf("outside %+v", k)
	}
	if ks[2].slot != 5 || ks[3].slot != 4 {
		t.Fatalf("armour slot %d (want 5), grid slot %d (want 4)", ks[2].slot, ks[3].slot)
	}
	if g.armor[0].ID != ironHelmet || g.cont.grid[3].ID != planks {
		t.Fatal("the helmet and the plank did not land")
	}
}

// A server that refuses a click answers with the same transaction and then the window's contents and the mouse:
// all of it lands, grid included, and the refusal is answered.
func TestPlayersWindowIsRestoredAfterARefusedClick(t *testing.T) {
	g, c := netPlayerGame(t)
	g.inv[0] = slot{ID: stick, N: 5}
	g.openContainer(contPlayer)
	g.clickSlot(inv(g, 0), false, false) // predicted: the sticks are on the mouse
	drain(t, c)
	no := newPacket(0x32)
	no.Byte(0)
	no.UShort(1)
	no.Bool(false)
	decodeFrame(t, no, nil)(g)
	ps := drain(t, c)
	if len(ps) != 1 || ps[0].id != 0x0F || ps[0].r.Byte() != 0 || ps[0].r.Short() != 1 || !ps[0].r.Bool() {
		t.Fatalf("answered %v", ps)
	}
	items := newPacket(0x30)
	items.Byte(0)
	items.UShort(45)
	for i := 0; i < 45; i++ {
		switch i {
		case 36:
			items.UShort(stick)
			items.Byte(5)
			items.UShort(0)
			items.Byte(0)
		case 2:
			items.UShort(planks)
			items.Byte(2)
			items.UShort(0)
			items.Byte(0)
		default:
			items.UShort(0xFFFF)
		}
	}
	decodeFrame(t, items, nil)(g)
	cur := newPacket(0x2F)
	cur.Byte(0xFF)
	cur.UShort(0xFFFF)
	cur.UShort(0xFFFF)
	decodeFrame(t, cur, nil)(g)
	if g.inv[0] != (slot{ID: stick, N: 5}) || !g.cursor.empty() || g.cont.grid[1] != (slot{ID: planks, N: 2}) {
		t.Fatalf("restored: hotbar %v cursor %v grid %v", g.inv[0], g.cursor, g.cont.grid)
	}
}

// A server's window with rules the engine knows is predicted, tags included; the server's cursor and slot
// bring it back when it cancelled the click, as a lobby menu does.
func TestServerWindowIsPredictedAndRestored(t *testing.T) {
	g, c := netPlayerGame(t)
	openChest(t, g, 5, `{"text":"Menu"}`, 27)
	w := newPacket(0x2F)
	w.Byte(5)
	w.UShort(4)
	tag := namedItem(w, 399, 1, "§bJugar", "x")
	decodeFrame(t, w, nil)(g)
	g.sendWindowClick(4, 0, 0)
	if g.swin.cursor.ID != 399 || !g.swin.slots[4].empty() || !bytes.Equal(g.swin.cursorX.nbt, tag) {
		t.Fatalf("predicted: cursor %v slot %v tag kept %v", g.swin.cursor, g.swin.slots[4], bytes.Equal(g.swin.cursorX.nbt, tag))
	}
	ks := readClicks(t, c)
	if len(ks) != 1 || ks[0].win != 5 || ks[0].slot != 4 || ks[0].id != 399 || !bytes.Equal(ks[0].tag, tag) {
		t.Fatalf("sent %+v", ks)
	}
	cur := newPacket(0x2F)
	cur.Byte(0xFF)
	cur.UShort(0xFFFF)
	cur.UShort(0xFFFF)
	decodeFrame(t, cur, nil)(g)
	back := newPacket(0x2F)
	back.Byte(5)
	back.UShort(4)
	namedItem(back, 399, 1, "§bJugar", "x")
	decodeFrame(t, back, nil)(g)
	if !g.swin.cursor.empty() || g.swin.slots[4].ID != 399 || plainRuns(g.swin.extra[4].name) != "Jugar" {
		t.Fatalf("restored: cursor %v slot %v name %q", g.swin.cursor, g.swin.slots[4], plainRuns(g.swin.extra[4].name))
	}
}

// A window the engine has no rules for (an anvil here) is not predicted: nothing moves until the server says,
// and the click goes out with the slot's stack.
func TestUnknownServerWindowIsNotPredicted(t *testing.T) {
	g, c := netPlayerGame(t)
	g.openServerWindow(3, "minecraft:anvil", `{"text":"Repair"}`, 3)
	g.swin.slots[0] = slot{ID: 267, N: 1}
	g.sendWindowClick(0, 0, 0)
	if g.swin.slots[0].ID != 267 || !g.swin.cursor.empty() {
		t.Fatalf("slot %v cursor %v: moved without rules", g.swin.slots[0], g.swin.cursor)
	}
	ks := readClicks(t, c)
	if len(ks) != 1 || ks[0].win != 3 || ks[0].slot != 0 || ks[0].id != 267 {
		t.Fatalf("sent %+v", ks)
	}
}

// Each window counts its own clicks; the player's window keeps counting between openings.
func TestTransactionNumbersCountPerWindow(t *testing.T) {
	g, c := netPlayerGame(t)
	g.openContainer(contPlayer)
	g.clickSlot(inv(g, 0), false, false)
	g.clickSlot(inv(g, 0), false, false)
	g.closeContainer()
	openChest(t, g, 7, `{"text":"x"}`, 9)
	g.sendWindowClick(0, 0, 0)
	g.closeServerWindow(true)
	g.openContainer(contPlayer)
	g.clickSlot(inv(g, 0), false, false)
	var got []int16
	for _, k := range readClicks(t, c) {
		got = append(got, k.action)
	}
	if len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 1 || got[3] != 3 {
		t.Fatalf("transaction numbers %v, want 1 2 (window 0) 1 (window 7) 3 (window 0 again)", got)
	}
}

// A furnace and a crafting table opened by a server have their own layouts, are predicted by the engine, and
// the server's Window Property drives the furnace's flame and arrow.
func TestServerFurnaceAndCraftingTable(t *testing.T) {
	g, c := netPlayerGame(t)
	g.openServerWindow(4, "minecraft:furnace", `{"text":"Furnace"}`, 3)
	slots, h := g.swin.layout()
	if h != 166 || len(slots) != 39 || slots[0] != (winSlot{56, 17, 0}) || slots[1] != (winSlot{56, 53, 1}) || slots[2] != (winSlot{116, 35, 2}) {
		t.Fatalf("furnace layout: height %d, %d slots, first three %v", h, len(slots), slots[:3])
	}
	prop := func(win int8, id, val uint16) {
		p := newPacket(0x31)
		p.Byte(byte(win))
		p.UShort(id)
		p.UShort(val)
		if f := decodeFrame(t, p, nil); f != nil {
			f(g)
		}
	}
	prop(4, 1, 150)
	prop(4, 2, 300)
	prop(9, 1, 7)  // another window's
	prop(4, 99, 1) // not a property
	if g.swin.props[1] != 150 || g.swin.props[2] != 300 || g.swin.props[3] != 0 {
		t.Fatalf("properties %v", g.swin.props)
	}
	g.inv[0] = slot{ID: ironOre, N: 8} // window slot 3 + 27
	g.sendWindowClick(30, 0, 1)
	if g.swin.slots[0] != (slot{ID: ironOre, N: 8}) || !g.inv[0].empty() {
		t.Fatalf("shift-click: input %v hotbar %v", g.swin.slots[0], g.inv[0])
	}
	ks := readClicks(t, c)
	if len(ks) != 1 || ks[0].win != 4 || ks[0].slot != 30 || ks[0].mode != 1 || ks[0].id != ironOre || ks[0].n != 8 {
		t.Fatalf("sent %+v", ks)
	}

	g.openServerWindow(6, "minecraft:crafting_table", `{"text":"Crafting"}`, 10)
	slots, h = g.swin.layout()
	if h != 166 || slots[0] != (winSlot{124, 35, 0}) || slots[1] != (winSlot{30, 17, 1}) || slots[9] != (winSlot{66, 53, 9}) {
		t.Fatalf("table layout: height %d, %v %v %v", h, slots[0], slots[1], slots[9])
	}
	g.swin.slots[0] = slot{ID: stick, N: 4} // what the server's window contents said the result is
	g.swin.slots[1], g.swin.slots[4] = slot{ID: planks, N: 2}, slot{ID: planks, N: 2}
	g.sendWindowClick(0, 0, 0)
	if g.swin.cursor != (slot{ID: stick, N: 4}) || g.swin.slots[1].N != 1 || g.swin.slots[4].N != 1 || g.swin.slots[0] != (slot{ID: stick, N: 4}) {
		t.Fatalf("crafted: cursor %v grid %v %v result %v", g.swin.cursor, g.swin.slots[1], g.swin.slots[4], g.swin.slots[0])
	}
}
