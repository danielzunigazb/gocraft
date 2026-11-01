package server

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocraft/anvil"
	"gocraft/proto"
	"gocraft/world"
)

// What players change is written as a 1.8.9 world and comes back: the edited column from disk, every other column
// from the generator, and the folder opens as any saved world does.
func TestWorldIsSavedAndReloaded(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{SaveDir: dir, Seed: 1234})
	s.manual = true
	stone := uint16(1 << 4)
	s.setBlock(5, 6, 7, stone) // flat layers stop below y=4: this is air above them
	s.setBlock(100, 5, 100, uint16(4<<4))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"level.dat", filepath.Join("region", "r.0.0.mca")} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s was not written: %v", f, err)
		}
	}
	s2 := New(Config{SaveDir: dir})
	if got := s2.world.Block(5, 6, 7); got != stone {
		t.Fatalf("the edit came back as %#x, want %#x", got, stone)
	}
	if got := s2.world.Block(100, 5, 100); got != 4<<4 {
		t.Fatalf("the second edit came back as %#x", got)
	}
	if s2.world.Block(5, 0, 7) != s.world.Block(5, 0, 7) || s2.world.Block(5, 0, 7) == 0 { // the flat ground, from the generator
		t.Fatal("an unedited block changed")
	}
	// the folder is a world the game can open
	sv, err := anvil.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer sv.Close()
	if sv.Level.Seed != 1234 || sv.Level.Generator != "flat" {
		t.Fatalf("level %+v", sv.Level)
	}
	if c, _ := sv.Chunk(world.ChunkPos{X: 40, Z: 40}); c != nil {
		t.Fatal("a column nobody edited was saved")
	}
	if c, _ := sv.Chunk(world.ChunkPos{X: 0, Z: 0}); c == nil || c.Blocks[6<<8|7<<4|5] != stone {
		t.Fatal("the edited column is not in the saved world")
	}
}

// A second save adds to the region files instead of replacing them, and nothing is written when nothing changed.
func TestSavingAddsToTheFiles(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{SaveDir: dir})
	s.manual = true
	s.setBlock(1, 10, 1, 2<<4)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	region := filepath.Join(dir, "region", "r.0.0.mca")
	before, _ := os.Stat(region)
	time.Sleep(20 * time.Millisecond)
	if err := s.Save(); err != nil { // nothing is dirty
		t.Fatal(err)
	}
	if after, _ := os.Stat(region); !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a save with nothing to write touched the region")
	}
	s.setBlock(40, 10, 40, 3<<4) // another column of the same region
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := New(Config{SaveDir: dir})
	if s2.world.Block(1, 10, 1) != 2<<4 || s2.world.Block(40, 10, 40) != 3<<4 {
		t.Fatal("the first column was lost when the second was added")
	}
}

// The inventory is saved in the game's own numbering: the hotbar is 0-8, the armour 100-103 from the boots up.
func TestPlayerDataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{SaveDir: dir, Survival: true})
	s.manual = true
	p := newPlayer(nil, 1, "Alguien")
	p.x, p.y, p.z, p.yaw, p.pitch = 10.5, 70, -3.25, 90, -15
	p.health, p.held = 13, 4
	p.inv[36+4] = slot{id: 276, count: 1} // the sword in the hotbar
	p.inv[9] = slot{id: 4, count: 64}     // cobblestone in the first slot of the main inventory
	p.inv[5] = slot{id: 310, count: 1}    // a diamond helmet
	p.inv[8] = slot{id: 313, count: 1}    // and boots
	p.inv[7] = slot{id: 312, count: 1, damage: 20}
	if err := s.savePlayer(p); err != nil {
		t.Fatal(err)
	}
	got, ok := s.loadPlayer(p.uuid)
	if !ok {
		t.Fatal("the player was not found")
	}
	if got.pos != [3]float64{10.5, 70, -3.25} || got.yaw != 90 || got.pitch != -15 || got.health != 13 || got.held != 4 {
		t.Fatalf("state %+v", got)
	}
	if got.inv != p.inv {
		t.Fatalf("inventory %v, want %v", got.inv, p.inv)
	}
	if _, ok := s.loadPlayer(offlineUUID("Otro")); ok {
		t.Fatal("a stranger was found")
	}
	// as the game numbers it: the helmet is 103, the boots 100, the hotbar sword slot 4
	c := anvil.ReadPlayer(dir, p.uuid)
	seen := map[int]int{}
	for _, v := range c["Inventory"].([]any) {
		it := v.(anvil.Compound)
		sl, _ := anvil.Number(it["Slot"])
		id, _ := anvil.Number(it["id"])
		seen[int(sl)] = int(id)
	}
	if seen[103] != 310 || seen[100] != 313 || seen[101] != 312 || seen[4] != 276 || seen[9] != 4 {
		t.Fatalf("slots in the file: %v", seen)
	}
	sv, err := anvil.Open(dir) // it needs a level.dat to open: the game's layout keeps players beside it
	if err == nil {
		sv.Close()
	}
}

func rejoin(t *testing.T, addr, name string) (*client, [3]float64) {
	c := dial(t, addr)
	c.handshake(Protocol, 2)
	p := proto.NewPacket(0)
	p.String(name)
	c.send(p)
	c.until(0x02)
	r := c.until(0x08)
	return c, [3]float64{r.Double(), r.Double(), r.Double()}
}

// A player who leaves and comes back stands where they left off and carries what they carried.
func TestPlayerReturnsWhereTheyLeft(t *testing.T) {
	dir := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go New(Config{ViewDistance: 2, SaveDir: dir}).Serve(l)
	addr := l.Addr().String()

	c, first := rejoin(t, addr, "Viajera")
	if first[0] == 20.5 {
		t.Fatal("the spawn is not what this test assumes")
	}
	mv := proto.NewPacket(0x06) // position and look
	mv.Double(20.5)
	mv.Double(60)
	mv.Double(-8.5)
	mv.Float(45)
	mv.Float(10)
	mv.Bool(false)
	c.send(mv)
	cr := proto.NewPacket(0x10) // creative inventory: a diamond sword in the first hotbar slot
	cr.UShort(36)
	cr.UShort(276)
	cr.Byte(1)
	cr.UShort(0)
	cr.Byte(0)
	c.send(cr)
	time.Sleep(200 * time.Millisecond) // the server reads both before the connection goes
	c.nc.Close()
	file := filepath.Join(dir, "playerdata", offlineUUID("Viajera")+".dat")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(file); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the player's data was not written when they left")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c2, back := rejoin(t, addr, "Viajera")
	if back != [3]float64{20.5, 60, -8.5} {
		t.Fatalf("came back at %v, want (20.5, 60, -8.5)", back)
	}
	r := c2.until(0x30) // Window Items of the player's window
	if win, n := r.Byte(), int(r.UShort()); win != 0 || n != 46 {
		t.Fatalf("window %d with %d slots", win, n)
	}
	for i := 0; i < 36; i++ { // the first thirty-six are the crafting and armour slots and the main inventory: empty
		if id := r.Short(); id != -1 {
			t.Fatalf("slot %d holds %d", i, id)
		}
	}
	if id := r.Short(); id != 276 {
		t.Fatalf("the hotbar's first slot holds %d, want the sword", id)
	}
}
