package client

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"gocraft/world"
)

// Block entities that hold items: chests, which keep what is put in them, and furnaces, which cook it.
// A block entity exists from the first time its block is opened (or broken); until then the block is
// empty. Their contents live in the overlay beside the world's edited columns, never in the player's
// own world files.

const (
	idChest, idTrappedChest, idEnderChest = 54, 146, 130
	idFurnace, idLitFurnace               = 61, 62
)

func isChestID(id uint16) bool { return id == idChest || id == idTrappedChest }

func isFurnaceID(id uint16) bool { return id == idFurnace || id == idLitFurnace }

type blockEntity struct {
	kind  byte // 'c' chest, 'f' furnace
	items []slot
	// Furnace state, as TileEntityFurnace keeps it, in ticks.
	burn, burnTotal, cook, cookTotal int
	lid                              float32 // 0 closed .. 1 open, for the picture of a chest
	viewers                          int     // a server's chest: how many players have it open (Block Action 1)
}

type beState struct {
	ents  map[[3]int32]*blockEntity
	chest map[world.ChunkPos]chestCache
	tex   map[string]uint32
	texOf *resources
	// pathBudget is how many A* searches the mobs may still make this tick.
	pathBudget int
	// newMobs are mobs made while the mobs are being ticked (a slime splitting), added after it.
	newMobs []*mob
	// Entities of the world: the columns read from the world file already, and the mobs put away.
	resolved map[world.ChunkPos]bool
	mobStore map[world.ChunkPos][]mobJSON
	// The creative inventory: the chosen tab and each tab's items.
	tab      int
	tabLists [creativeTabCount][]slot
	tabFor   *slot
}

func newBlockEntity(kind byte) *blockEntity {
	be := &blockEntity{kind: kind, cookTotal: furnaceCookTime}
	switch kind {
	case 'c':
		be.items = make([]slot, 27)
	case 'f':
		be.items = make([]slot, 3)
	}
	return be
}

const furnaceCookTime = 200

// ---- smelting ----

type smeltKey struct{ id, dmg uint16 }

const anyDmg = 0xFFFF

// FurnaceRecipes of 1.8.9: what a stack of one smelts into.
var smelting = map[smeltKey]slot{
	{15, anyDmg}:  {ID: 265, N: 1},         // iron ore -> iron ingot
	{14, anyDmg}:  {ID: 266, N: 1},         // gold ore -> gold ingot
	{56, anyDmg}:  {ID: 264, N: 1},         // diamond ore -> diamond
	{12, anyDmg}:  {ID: 20, N: 1},          // sand -> glass
	{4, anyDmg}:   {ID: 1, N: 1},           // cobblestone -> stone
	{82, anyDmg}:  {ID: 172, N: 1},         // clay -> hardened clay
	{81, anyDmg}:  {ID: 351, Dmg: 2, N: 1}, // cactus -> cactus green
	{17, anyDmg}:  {ID: 263, Dmg: 1, N: 1}, // log -> charcoal
	{162, anyDmg}: {ID: 263, Dmg: 1, N: 1}, // log2 -> charcoal
	{87, anyDmg}:  {ID: 405, N: 1},         // netherrack -> nether brick
	{98, 0}:       {ID: 98, Dmg: 2, N: 1},  // stone brick -> cracked
	{19, 1}:       {ID: 19, N: 1},          // wet sponge -> sponge
	{16, anyDmg}:  {ID: 263, N: 1},         // coal ore -> coal
	{21, anyDmg}:  {ID: 351, Dmg: 4, N: 1}, // lapis ore -> lapis lazuli
	{73, anyDmg}:  {ID: 331, N: 1},         // redstone ore -> redstone
	{74, anyDmg}:  {ID: 331, N: 1},         // lit redstone ore
	{129, anyDmg}: {ID: 388, N: 1},         // emerald ore -> emerald
	{153, anyDmg}: {ID: 406, N: 1},         // nether quartz ore -> quartz
	{319, anyDmg}: {ID: 320, N: 1},         // raw porkchop
	{363, anyDmg}: {ID: 364, N: 1},         // raw beef
	{365, anyDmg}: {ID: 366, N: 1},         // raw chicken
	{411, anyDmg}: {ID: 412, N: 1},         // raw rabbit
	{423, anyDmg}: {ID: 424, N: 1},         // raw mutton
	{392, anyDmg}: {ID: 393, N: 1},         // potato -> baked potato
	{337, anyDmg}: {ID: 336, N: 1},         // clay ball -> brick
	{349, 0}:      {ID: 350, N: 1},         // raw fish -> cooked fish
	{349, 1}:      {ID: 350, Dmg: 1, N: 1}, // raw salmon -> cooked salmon
}

// smeltResult is what the stack in the input slot becomes.
func smeltResult(in slot) (slot, bool) {
	if in.empty() {
		return slot{}, false
	}
	if r, ok := smelting[smeltKey{in.ID, in.Dmg}]; ok {
		return r, true
	}
	if r, ok := smelting[smeltKey{in.ID, anyDmg}]; ok {
		return r, true
	}
	return slot{}, false
}

var woodBlocks = idSet(5, 17, 162, 25, 47, 53, 54, 58, 64, 72, 84, 85, 96, 107, 125, 134, 135, 136, 143, 146, 163, 164,
	183, 184, 185, 186, 187, 188, 189, 190, 191, 192)

// fuelTime is TileEntityFurnace.getItemBurnTime: how many ticks an item burns.
func fuelTime(s slot) int {
	if s.empty() {
		return 0
	}
	id := s.ID
	if id < 256 {
		switch {
		case id == 126: // wooden slab
			return 150
		case id == 173: // block of coal
			return 16000
		case id == 6: // sapling
			return 100
		case woodBlocks[id]:
			return 300
		}
		return 0
	}
	switch id {
	case 268, 269, 270, 271, 290: // wooden sword, shovel, pickaxe, axe, hoe
		return 200
	case 280: // stick
		return 100
	case 263: // coal and charcoal
		return 1600
	case 327: // lava bucket
		return 20000
	case 369: // blaze rod
		return 2400
	}
	return 0
}

func (b *blockEntity) burning() bool { return b.burn > 0 }

// canSmelt is TileEntityFurnace.canSmelt: there is something to cook and room for the result.
func (b *blockEntity) canSmelt() bool {
	res, ok := smeltResult(b.items[0])
	if !ok {
		return false
	}
	out := b.items[2]
	switch {
	case out.empty():
		return true
	case !out.same(res):
		return false
	}
	return int(out.N)+int(res.N) <= int(out.maxStack())
}

func (b *blockEntity) smeltItem() {
	res, _ := smeltResult(b.items[0])
	if b.items[2].empty() {
		b.items[2] = res
	} else {
		b.items[2].N += res.N
	}
	if b.items[0].N--; b.items[0].N <= 0 {
		b.items[0] = slot{}
	}
}

// tickFurnace is TileEntityFurnace.update; it reports whether the burning state changed, which turns
// the block between the plain and the lit furnace.
func (b *blockEntity) tickFurnace() (changed bool) {
	was := b.burning()
	if b.burning() {
		b.burn--
	}
	if b.burning() || !b.items[1].empty() && !b.items[0].empty() {
		if !b.burning() && b.canSmelt() {
			b.burnTotal = fuelTime(b.items[1])
			b.burn = b.burnTotal
			if b.burning() {
				if b.items[1].ID == 327 { // a lava bucket leaves an empty one
					b.items[1] = slot{ID: 325, N: 1}
				} else if b.items[1].N--; b.items[1].N <= 0 {
					b.items[1] = slot{}
				}
			}
		}
		if b.burning() && b.canSmelt() {
			if b.cook++; b.cook == b.cookTotal {
				b.cook, b.cookTotal = 0, furnaceCookTime
				b.smeltItem()
			}
		} else {
			b.cook = 0
		}
	} else if !b.burning() && b.cook > 0 {
		b.cook = max(0, min(b.cook-2, b.cookTotal))
	}
	return was != b.burning()
}

// ---- the registry ----

func (g *game) beAt(p [3]int32, create bool) *blockEntity {
	if be := g.be.ents[p]; be != nil {
		return be
	}
	if !create {
		return nil
	}
	id := g.world.Block(p[0], p[1], p[2]) >> 4
	var kind byte
	switch {
	case isChestID(id):
		kind = 'c'
	case isFurnaceID(id):
		kind = 'f'
	default:
		return nil
	}
	be := newBlockEntity(kind)
	if g.be.ents == nil {
		g.be.ents = map[[3]int32]*blockEntity{}
	}
	g.be.ents[p] = be
	return be
}

// tickBlockEntities cooks what is in the furnaces.
func (g *game) tickBlockEntities() {
	for p, be := range g.be.ents {
		if be.kind != 'f' {
			continue
		}
		if be.tickFurnace() {
			if st := g.world.Block(p[0], p[1], p[2]); isFurnaceID(st >> 4) {
				id := uint16(idFurnace)
				if be.burning() {
					id = idLitFurnace
				}
				if st>>4 != id {
					g.setBlock(p[0], p[1], p[2], id<<4|st&15)
				}
			}
		}
	}
}

// beBroken is what happens to a chest or furnace that is broken: its contents fall out and it is gone.
func (g *game) beBroken(p [3]int32) {
	be := g.be.ents[p]
	if be == nil {
		return
	}
	for _, s := range be.items {
		g.spawnDrop(float64(p[0]), float64(p[1]), float64(p[2]), s)
	}
	delete(g.be.ents, p)
}

// ---- saving ----

type beJSON struct {
	Kind  string      `json:"kind"`
	Items []stackJSON `json:"items"`
	Burn  int         `json:"burn,omitempty"`
	Total int         `json:"burnTotal,omitempty"`
	Cook  int         `json:"cook,omitempty"`
}

func (o overlay) blockEntitiesPath() string { return filepath.Join(o.dir, "blockentities.json") }

// saveBlockEntities writes every block entity that holds something, beside the edited columns.
func (g *game) saveBlockEntities(o overlay) {
	g.saveMobStore(o)
	out := map[string]beJSON{}
	keys := make([][3]int32, 0, len(g.be.ents))
	for p := range g.be.ents {
		keys = append(keys, p)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i][0] < keys[j][0] || keys[i][0] == keys[j][0] && (keys[i][1] < keys[j][1] || keys[i][1] == keys[j][1] && keys[i][2] < keys[j][2])
	})
	for _, p := range keys {
		be := g.be.ents[p]
		j := beJSON{Kind: string(be.kind), Burn: be.burn, Total: be.burnTotal, Cook: be.cook}
		empty := be.burn == 0 && be.cook == 0
		for _, s := range be.items {
			j.Items = append(j.Items, toJSON(s))
			if !s.empty() {
				empty = false
			}
		}
		if !empty {
			out[strconv.Itoa(int(p[0]))+","+strconv.Itoa(int(p[1]))+","+strconv.Itoa(int(p[2]))] = j
		}
	}
	b, _ := json.Marshal(out)
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		logf("saving block entities: %v", err)
		return
	}
	if err := writeAtomic(o.blockEntitiesPath(), func(w io.Writer) error { _, err := w.Write(b); return err }); err != nil {
		logf("saving block entities: %v", err)
	}
}

// loadBlockEntities reads them back when a world opens.
func (g *game) loadBlockEntities(o overlay) {
	g.be.ents = map[[3]int32]*blockEntity{}
	g.loadMobStore(o) // a world starts with its own mobs and items
	b, err := os.ReadFile(o.blockEntitiesPath())
	if err != nil {
		return
	}
	var in map[string]beJSON
	if json.Unmarshal(b, &in) != nil {
		return
	}
	for k, j := range in {
		var x, y, z int
		if _, err := fmt.Sscanf(k, "%d,%d,%d", &x, &y, &z); err != nil {
			continue
		}
		p := [3]int32{int32(x), int32(y), int32(z)}
		kind := byte('c')
		if j.Kind == "f" {
			kind = 'f'
		}
		be := newBlockEntity(kind)
		for i := range be.items {
			if i < len(j.Items) {
				be.items[i] = slot{j.Items[i].ID, j.Items[i].Dmg, j.Items[i].N}
			}
		}
		be.burn, be.burnTotal, be.cook = j.Burn, j.Total, j.Cook
		g.be.ents[p] = be
	}
}
