package client

import (
	"testing"
)

func furnaceWith(in, fuel slot) *blockEntity {
	f := newBlockEntity('f')
	f.items[0], f.items[1] = in, fuel
	return f
}

func TestFurnaceTimings(t *testing.T) {
	f := furnaceWith(slot{ID: 15, N: 3}, slot{ID: 263, N: 1}) // 3 iron ore and a coal
	if !f.tickFurnace() || !f.burning() {
		t.Fatal("the first tick should light the fuel")
	}
	if f.burn != 1600-0 || f.items[1].N != 0 && !f.items[1].empty() {
		t.Fatalf("burn %d fuel %v", f.burn, f.items[1])
	}
	for i := 1; i < 199; i++ { // tick 2..199
		f.tickFurnace()
	}
	if !f.items[2].empty() {
		t.Fatalf("the first ingot came before 200 ticks: %v", f.items[2])
	}
	f.tickFurnace() // the 200th tick of cooking
	if f.items[2].ID != 265 || f.items[2].N != 1 {
		t.Fatalf("after 200 ticks the output is %v", f.items[2])
	}
	// 1600 ticks of coal cook eight items; here three, then the flame goes out at tick 1600.
	for i := 0; i < 400; i++ {
		f.tickFurnace()
	}
	if f.items[2].N != 3 || !f.items[0].empty() {
		t.Fatalf("3 ores should be 3 ingots: out %v in %v", f.items[2], f.items[0])
	}
}

func TestFurnaceFuelAndBurnout(t *testing.T) {
	for _, c := range []struct {
		s    slot
		want int
	}{{blockSlot(5<<4, 1), 300}, {blockSlot(126<<4, 1), 150}, {slot{ID: 280, N: 1}, 100}, {slot{ID: 263, N: 1}, 1600},
		{slot{ID: 327, N: 1}, 20000}, {slot{ID: 369, N: 1}, 2400}, {slot{ID: 268, N: 1}, 200}, {blockSlot(173<<4, 1), 16000}, {slot{ID: 264, N: 1}, 0}} {
		if got := fuelTime(c.s); got != c.want {
			t.Errorf("fuel %v burns %d ticks, want %d", c.s, got, c.want)
		}
	}
	// A stick burns 100 ticks: half an item. The flame goes out and the arrow slips back by two a tick.
	f := furnaceWith(slot{ID: 15, N: 1}, slot{ID: 280, N: 1})
	for i := 0; i < 100; i++ {
		f.tickFurnace()
	}
	if f.cook != 100 || f.burn != 1 {
		t.Fatalf("after 100 ticks of a stick: cook %d burn %d, want 100 and 1 left", f.cook, f.burn)
	}
	changed := f.tickFurnace() // the flame goes out, and with no fuel left the arrow slips back by 2
	if !changed || f.burning() || f.cook != 98 {
		t.Fatalf("tick 101: changed %v burning %v cook %d", changed, f.burning(), f.cook)
	}
	// Lava leaves a bucket.
	l := furnaceWith(slot{ID: 12, N: 64}, slot{ID: 327, N: 1})
	l.tickFurnace()
	if l.items[1].ID != 325 || l.burn != 20000 {
		t.Fatalf("lava bucket %v burn %d", l.items[1], l.burn)
	}
}

func TestFurnaceNeedsRoom(t *testing.T) {
	f := furnaceWith(slot{ID: 15, N: 1}, slot{ID: 263, N: 1})
	f.items[2] = slot{ID: 266, N: 3} // gold in the output: iron cannot join it
	if f.canSmelt() {
		t.Fatal("iron must not go on top of gold")
	}
	f.items[2] = slot{ID: 265, N: 64}
	if f.canSmelt() {
		t.Fatal("a full output stack stops the furnace")
	}
	f.items[2] = slot{ID: 265, N: 63}
	if !f.canSmelt() {
		t.Fatal("room for one more")
	}
}

func TestEverySmeltResultIsAnItem(t *testing.T) {
	for k, r := range smelting {
		if r.ID >= 256 {
			if _, ok := itemDefs[r.ID]; !ok {
				t.Errorf("smelting %v gives item %d, which the game does not know", k, r.ID)
			}
		}
		if r.N != 1 {
			t.Errorf("%v makes %d", k, r.N)
		}
	}
	if r, ok := smeltResult(slot{ID: 17, Dmg: 2, N: 1}); !ok || r.ID != 263 || r.Dmg != 1 {
		t.Fatalf("a birch log smelts to charcoal, got %v %v", r, ok)
	}
	if _, ok := smeltResult(slot{ID: 98, Dmg: 1, N: 1}); ok {
		t.Fatal("mossy stone brick does not smelt")
	}
}

func openChestGame(t *testing.T) *game {
	g := testGame(t)
	g.feet = vec3{0.5, 4, 5.5}
	g.setBlock(2, 4, 2, idChest<<4|2)
	return g
}

func TestChestKeepsItsContents(t *testing.T) {
	g := openChestGame(t)
	if !g.openBlockEntity([3]int32{2, 4, 2}) || g.cont.kind != contChest || g.cont.rows != 3 {
		t.Fatalf("the chest did not open: %+v", g.cont.kind)
	}
	g.cont.be.items[4] = slot{ID: 264, N: 5}
	g.closeContainer()
	if !g.openBlockEntity([3]int32{2, 4, 2}) || g.cont.be.items[4].ID != 264 || g.cont.be.items[4].N != 5 {
		t.Fatalf("the diamonds are gone: %v", g.cont.be.items[4])
	}
	// A solid block on top shuts it.
	g.closeContainer()
	g.setBlock(2, 5, 2, 1<<4)
	g.screen = screenNone
	if !g.openBlockEntity([3]int32{2, 4, 2}) || g.screen == screenContainer {
		t.Fatal("a chest with a block above must not open")
	}
}

func TestDoubleChestHasFiftyFourSlotsWestFirst(t *testing.T) {
	g := testGame(t)
	g.setBlock(2, 4, 2, idChest<<4|2)
	g.setBlock(3, 4, 2, idChest<<4|2)
	west := g.beAt([3]int32{2, 4, 2}, true)
	east := g.beAt([3]int32{3, 4, 2}, true)
	west.items[0], east.items[0] = slot{ID: 1, N: 1}, slot{ID: 2, N: 1}
	for _, p := range [][3]int32{{2, 4, 2}, {3, 4, 2}} { // opened from either half, the window is the same
		g.openBlockEntity(p)
		if g.cont.rows != 6 {
			t.Fatalf("rows %d", g.cont.rows)
		}
		slots := g.beSlots()
		if slots[0].ref.p.ID != 1 || slots[27].ref.p.ID != 2 {
			t.Fatalf("from %v: slot 0 is %d and slot 27 is %d, want the west chest first", p, slots[0].ref.p.ID, slots[27].ref.p.ID)
		}
		g.closeContainer()
	}
}

func TestBreakingAChestDropsItsContents(t *testing.T) {
	g := openChestGame(t)
	be := g.beAt([3]int32{2, 4, 2}, true)
	be.items[0], be.items[10] = slot{ID: 264, N: 2}, slot{ID: 265, N: 7}
	g.beBroken([3]int32{2, 4, 2})
	if len(g.items) != 2 || g.be.ents[[3]int32{2, 4, 2}] != nil {
		t.Fatalf("%d item entities, entity left: %v", len(g.items), g.be.ents[[3]int32{2, 4, 2}] != nil)
	}
}

func TestFurnaceBlockLightsAndGoesOut(t *testing.T) {
	g := testGame(t)
	g.setBlock(2, 4, 2, idFurnace<<4|3)
	be := g.beAt([3]int32{2, 4, 2}, true)
	be.items[0], be.items[1] = slot{ID: 12, N: 1}, slot{ID: 280, N: 1} // sand and a stick
	g.tickBlockEntities()
	if st := g.world.Block(2, 4, 2); st != idLitFurnace<<4|3 {
		t.Fatalf("the furnace should light and keep its facing: %d/%d", st>>4, st&15)
	}
	for i := 0; i < 100; i++ {
		g.tickBlockEntities()
	}
	if st := g.world.Block(2, 4, 2); st>>4 != idFurnace || st&15 != 3 {
		t.Fatalf("the furnace should go out after the stick: %d/%d", st>>4, st&15)
	}
}

func TestFurnaceWindowRules(t *testing.T) {
	g := testGame(t)
	g.setBlock(2, 4, 2, idFurnace<<4)
	g.openBlockEntity([3]int32{2, 4, 2})
	it := g.cont.be.items
	slots := g.beSlots()
	// Fuel goes only in the fuel slot, and only fuel.
	g.cursor = slot{ID: 264, N: 1} // a diamond
	g.clickSlot(slots[1].ref, false, false)
	if !it[1].empty() {
		t.Fatal("a diamond is not fuel")
	}
	g.cursor = slot{ID: 263, N: 4}
	g.clickSlot(slots[1].ref, false, false)
	if it[1].ID != 263 || it[1].N != 4 {
		t.Fatalf("coal in the fuel slot: %v", it[1])
	}
	// Nothing is put into the output, but it is taken.
	g.cursor = slot{ID: 265, N: 1}
	g.clickSlot(slots[2].ref, false, false)
	if !it[2].empty() {
		t.Fatal("the output slot takes nothing")
	}
	it[2] = slot{ID: 265, N: 3}
	g.cursor = slot{}
	g.clickSlot(slots[2].ref, false, false)
	if g.cursor.ID != 265 || g.cursor.N != 3 || !it[2].empty() {
		t.Fatalf("taking the output: cursor %v out %v", g.cursor, it[2])
	}
	// Shift-click sends ore to the input and coal to the fuel slot.
	g.inv[9], g.inv[10] = slot{ID: 15, N: 5}, slot{ID: 263, N: 2}
	g.clickSlot(slotRef{&g.inv[9], kInv, 9}, false, true)
	g.clickSlot(slotRef{&g.inv[10], kInv, 10}, false, true)
	if it[0].ID != 15 || it[0].N != 5 || it[1].N != 6 {
		t.Fatalf("shift-click: input %v fuel %v", it[0], it[1])
	}
}

func TestBlockEntitiesSurviveTheOverlay(t *testing.T) {
	g := openChestGame(t)
	g.setBlock(5, 4, 5, idFurnace<<4)
	g.beAt([3]int32{2, 4, 2}, true).items[3] = slot{ID: 276, Dmg: 12, N: 1}
	f := g.beAt([3]int32{5, 4, 5}, true)
	f.items[0], f.items[1] = slot{ID: 15, N: 9}, slot{ID: 263, N: 3}
	f.tickFurnace()
	for i := 0; i < 50; i++ {
		f.tickFurnace()
	}
	o := overlay{dir: t.TempDir()}
	g.saveBlockEntities(o)
	h := testGame(t)
	h.loadBlockEntities(o)
	c := h.be.ents[[3]int32{2, 4, 2}]
	if c == nil || c.kind != 'c' || c.items[3].ID != 276 || c.items[3].Dmg != 12 {
		t.Fatalf("the chest came back as %+v", c)
	}
	r := h.be.ents[[3]int32{5, 4, 5}]
	if r == nil || r.kind != 'f' || r.items[0].N != 9 || r.burn != f.burn || r.cook != f.cook || r.burn == 0 {
		t.Fatalf("the furnace came back as %+v, want burn %d cook %d", r, f.burn, f.cook)
	}
}

func TestPlacedChestsFaceThePlayerAndPair(t *testing.T) {
	// A player looking south (yaw 0) places a chest that faces north (2), as in the game.
	for yaw, want := range map[float32]uint16{0: 2, 90: 5, 180: 3, 270: 4, -90: 4} {
		if got := chestFacing(yaw); got != want {
			t.Errorf("yaw %v: facing %d, want %d", yaw, got, want)
		}
	}
	g := testGame(t)
	g.setBlock(2, 4, 2, idChest<<4|2)
	g.setBlock(3, 4, 2, idChest<<4|5) // placed by a player facing west: it would face east
	g.afterPlace(3, 4, 2)
	a, b := g.world.Block(2, 4, 2)&7, g.world.Block(3, 4, 2)&7
	if a != b || (a != 2 && a != 3) {
		t.Fatalf("a pair along x must face north or south together: %d %d", a, b)
	}
}
