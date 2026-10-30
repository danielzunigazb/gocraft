package client

import "testing"

func TestItemNamesComeFromTheLangFile(t *testing.T) {
	g := testGame(t)
	for _, c := range []struct {
		s    slot
		want string
	}{
		{blockSlot(1<<4|1, 1), "Granite"},
		{blockSlot(4<<4, 1), "Cobblestone"},
		{blockSlot(5<<4|2, 1), "Birch Wood Planks"},
		{blockSlot(35<<4|14, 1), "Red Wool"},
		{slot{ID: 276, N: 1}, "Diamond Sword"},
		{slot{ID: 265, N: 1}, "Iron Ingot"},
		{slot{ID: 351, Dmg: 4, N: 1}, "Lapis Lazuli"},
		{slot{ID: 263, Dmg: 1, N: 1}, "Charcoal"},
		{slot{ID: 398, N: 1}, "Carrot on a Stick"},
	} {
		if got := g.displayName(c.s); got != c.want {
			t.Errorf("%v is called %q, want %q", c.s, got, c.want)
		}
	}
}

func TestCreativeTabsSplitTheItems(t *testing.T) {
	g := testGame(t)
	all := g.creativeItems()
	total := 0
	for tab := 0; tab < creativeTabCount; tab++ {
		g.be.tab = tab
		list := g.creativeShown()
		if len(list) == 0 {
			t.Errorf("the %s tab is empty", creativeTabNames[tab])
		}
		for _, s := range list {
			if creativeTabOf(s) != tab {
				t.Fatalf("%v is in tab %d but belongs to %d", s, tab, creativeTabOf(s))
			}
		}
		total += len(list)
	}
	if total != len(all) {
		t.Fatalf("the tabs hold %d items, the game has %d", total, len(all))
	}
	for s, want := range map[slot]int{blockSlot(1<<4, 1): 0, {ID: 276, N: 1}: 7, {ID: 260, N: 1}: 5, {ID: 258, N: 1}: 6, {ID: 331, N: 1}: 2, blockSlot(27<<4, 1): 3} {
		if got := creativeTabOf(s); got != want {
			t.Errorf("%v is in tab %d, want %d", s, got, want)
		}
	}
	// A click on a tab is found where the tabs are drawn.
	for i := 0; i < creativeTabCount; i++ {
		x, y, upper := tabRect(i)
		if upper {
			y += 4
		} else {
			y += 8
		}
		if got := tabAt(x+10, y); got != i {
			t.Errorf("a point on tab %d finds %d", i, got)
		}
	}
}

func TestBlocksDrawAsCubesAndPlantsFlat(t *testing.T) {
	g := testGame(t)
	g.hud.verts = nil
	if !g.icon3D(1<<4, 0, 0, 16) {
		t.Fatal("stone is drawn as a cube")
	}
	if n := len(g.hud.verts) / 8; n != 18 { // three faces, two triangles each
		t.Fatalf("a cube shows 3 faces of 6 vertices, got %d vertices", n)
	}
	if g.icon3D(50<<4, 0, 0, 16) || g.icon3D(38<<4, 0, 0, 16) {
		t.Fatal("a torch and a flower are drawn flat")
	}
	g.hud.verts = nil
	if !g.icon3D(53<<4, 0, 0, 16) || len(g.hud.verts) <= 18*8 {
		t.Fatal("stairs have more faces than a cube")
	}
}
