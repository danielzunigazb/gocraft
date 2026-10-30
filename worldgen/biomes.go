package worldgen

// The 1.8.9 biome table: ids, terrain height parameters, climate, surface blocks and what each
// biome decorates with. Heights are BiomeGenBase's minHeight (the base) and maxHeight (the scale).

// Block states (id<<4 | meta) the generator places.
const (
	stAir        = 0
	stStone      = 1 << 4
	stGranite    = 1<<4 | 1
	stDiorite    = 1<<4 | 3
	stAndesite   = 1<<4 | 5
	stGrass      = 2 << 4
	stDirt       = 3 << 4
	stCoarseDirt = 3<<4 | 1
	stPodzol     = 3<<4 | 2
	stCobble     = 4 << 4
	stPlanks     = 5 << 4
	stBedrock    = 7 << 4
	stFlowWater  = 8 << 4
	stWater      = 9 << 4
	stFlowLava   = 10 << 4
	stLava       = 11 << 4
	stSand       = 12 << 4
	stRedSand    = 12<<4 | 1
	stGravel     = 13 << 4
	stGoldOre    = 14 << 4
	stIronOre    = 15 << 4
	stCoalOre    = 16 << 4
	stLog        = 17 << 4
	stLeaves     = 18 << 4
	stLapisOre   = 21 << 4
	stSandstone  = 24 << 4
	stDeadBush   = 32 << 4
	stYellowFl   = 37 << 4
	stRedFlower  = 38 << 4
	stBrownMush  = 39 << 4
	stRedMush    = 40 << 4
	stMossy      = 48 << 4
	stSpawner    = 52 << 4
	stChest      = 54 << 4
	stDiamondOre = 56 << 4
	stRedstoneOr = 73 << 4
	stSnowLayer  = 78 << 4
	stIce        = 79 << 4
	stSnowBlock  = 80 << 4
	stCactus     = 81 << 4
	stClay       = 82 << 4
	stReeds      = 83 << 4
	stPumpkin    = 86 << 4
	stMycelium   = 110 << 4
	stWaterlily  = 111 << 4
	stEmeraldOre = 129 << 4
	stMonsterEgg = 97 << 4
	stLeaves2    = 161 << 4
	stLog2       = 162 << 4
	stRedSandst  = 179 << 4
	stDoublePlnt = 175 << 4
)

// Classes are BiomeGenBase.getBiomeClass: biomes of one class count as alike when the layers
// decide on edges and hills.
const (
	clsOcean = iota + 1
	clsPlains
	clsDesert
	clsHills
	clsForest
	clsTaiga
	clsSwamp
	clsRiver
	clsHell
	clsSky
	clsSnow
	clsMushroom
	clsBeach
	clsStoneBeach
	clsJungle
	clsSavanna
	clsMesa
)

type tempCat int

const (
	catOcean tempCat = iota
	catCold
	catMedium
	catWarm
)

// Decor is what a biome's decorator places in each chunk (BiomeDecorator's *PerChunk fields).
type Decor struct {
	Trees, Flowers, Grass, DeadBush, Mushrooms, Reeds, Cacti, BigMushrooms, Lilies int
	Sand, Sand2, Clay                                                              int
}

func defaultDecor() Decor { return Decor{Flowers: 2, Grass: 1, Sand: 1, Sand2: 3, Clay: 1} }

type Biome struct {
	ID         int
	Name       string
	MinH, MaxH float32
	Temp, Rain float32
	Top, Fill  uint16
	Class      int
	Snow       bool // setEnableSnow: ice and snow form here
	Hills      int  // BiomeGenHills type: 0 normal, 1 extra trees, 2 mutated
	TaigaType  int  // BiomeGenTaiga type: 0 normal, 1 mega, 2 mega spruce
	Forest     int  // BiomeGenForest type: 0 normal, 1 flower, 2 birch, 3 roofed
	Mutated    bool
	Base       *Biome // the biome a mutation was made from
	Decor      Decor
}

func (b *Biome) tempCategory() tempCat {
	if b.Class == clsOcean {
		return catOcean
	}
	switch {
	case b.Temp < 0.2:
		return catCold
	case b.Temp < 1.0:
		return catMedium
	}
	return catWarm
}

var biomes [256]*Biome

type height struct{ min, max float32 }

var (
	hDefault   = height{0.1, 0.2}
	hShallow   = height{-0.5, 0.0}
	hOcean     = height{-1.0, 0.1}
	hDeepOcean = height{-1.8, 0.1}
	hLowPlains = height{0.125, 0.05}
	hMidPlains = height{0.2, 0.2}
	hLowHills  = height{0.45, 0.3}
	hPlateau   = height{1.5, 0.025}
	hMidHills  = height{1.0, 0.5}
	hShores    = height{0.0, 0.025}
	hRocky     = height{0.1, 0.8}
	hSubmerged = height{-0.2, 0.1}
)

func def(id int, name string, h height, temp, rain float32, class int) *Biome {
	b := &Biome{ID: id, Name: name, MinH: h.min, MaxH: h.max, Temp: temp, Rain: rain, Top: stGrass, Fill: stDirt, Class: class, Decor: defaultDecor()}
	biomes[id] = b
	return b
}

// mutate is createMutation: a biome that copies another's shape and climate (id + 128).
func mutate(base *Biome) *Biome {
	m := *base
	m.ID = base.ID + 128
	m.Name = base.Name + " M"
	m.Mutated = true
	m.Base = base
	biomes[m.ID] = &m
	return &m
}

func init() {
	def(0, "Ocean", hOcean, 0.5, 0.5, clsOcean)
	plains := def(1, "Plains", hLowPlains, 0.8, 0.4, clsPlains)
	plains.Decor.Trees, plains.Decor.Flowers, plains.Decor.Grass = -999, 4, 10
	desert := def(2, "Desert", hLowPlains, 2.0, 0.0, clsDesert)
	desert.Top, desert.Fill = stSand, stSand
	desert.Decor.Trees, desert.Decor.DeadBush, desert.Decor.Reeds, desert.Decor.Cacti = -999, 2, 50, 10
	def(3, "Extreme Hills", hMidHills, 0.2, 0.3, clsHills)
	forest := def(4, "Forest", hDefault, 0.7, 0.8, clsForest)
	forest.Decor.Trees, forest.Decor.Grass = 10, 2
	taiga := def(5, "Taiga", hMidPlains, 0.25, 0.8, clsTaiga)
	taiga.Decor.Trees, taiga.Decor.Grass, taiga.Decor.Mushrooms = 10, 1, 1
	swamp := def(6, "Swampland", hSubmerged, 0.8, 0.9, clsSwamp)
	swamp.Decor = Decor{Trees: 2, Flowers: 1, DeadBush: 1, Mushrooms: 8, Reeds: 10, Clay: 1, Lilies: 4, Grass: 5}
	def(7, "River", hShallow, 0.5, 0.5, clsRiver)
	def(8, "Hell", height{0.1, 0.2}, 2.0, 0.0, clsHell)
	def(9, "The End", height{0.1, 0.2}, 0.5, 0.5, clsSky)
	fo := def(10, "Frozen Ocean", hOcean, 0.0, 0.5, clsOcean)
	fo.Snow = true
	fr := def(11, "Frozen River", hShallow, 0.0, 0.5, clsRiver)
	fr.Snow = true
	ip := def(12, "Ice Plains", hLowPlains, 0.0, 0.5, clsSnow)
	ip.Snow = true
	im := def(13, "Ice Mountains", hLowHills, 0.0, 0.5, clsSnow)
	im.Snow = true
	mi := def(14, "Mushroom Island", height{0.2, 1.0}, 0.9, 1.0, clsMushroom)
	mi.Top = stMycelium
	mi.Decor = Decor{Trees: -100, Flowers: -100, Grass: -100, Mushrooms: 1, BigMushrooms: 1, Sand: 1, Sand2: 3, Clay: 1}
	ms := def(15, "Mushroom Island Shore", hOcean, 0.9, 1.0, clsMushroom)
	ms.Top = stMycelium
	ms.Decor = mi.Decor
	beach := def(16, "Beach", hShores, 0.8, 0.4, clsBeach)
	beach.Top, beach.Fill = stSand, stSand
	beach.Decor.Trees, beach.Decor.DeadBush, beach.Decor.Reeds, beach.Decor.Cacti = -999, 0, 0, 0
	dh := def(17, "DesertHills", hLowHills, 2.0, 0.0, clsDesert)
	dh.Top, dh.Fill, dh.Decor = stSand, stSand, desert.Decor
	fh := def(18, "ForestHills", hLowHills, 0.7, 0.8, clsForest)
	fh.Decor = forest.Decor
	th := def(19, "TaigaHills", hLowHills, 0.25, 0.8, clsTaiga)
	th.Decor = taiga.Decor
	def(20, "Extreme Hills Edge", height{0.2, 0.8}, 0.2, 0.3, clsHills)
	jungle := def(21, "Jungle", height{0.2, 0.4}, 0.95, 0.9, clsJungle)
	jungle.Decor.Trees, jungle.Decor.Grass, jungle.Decor.Flowers = 50, 25, 4
	jh := def(22, "JungleHills", height{1.8, 0.5}, 0.95, 0.9, clsJungle)
	jh.Decor = jungle.Decor
	je := def(23, "JungleEdge", hDefault, 0.95, 0.8, clsJungle)
	je.Decor.Trees, je.Decor.Grass, je.Decor.Flowers = 2, 25, 4
	def(24, "Deep Ocean", hDeepOcean, 0.5, 0.5, clsOcean)
	sb := def(25, "Stone Beach", hRocky, 0.2, 0.3, clsStoneBeach)
	sb.Top, sb.Fill = stStone, stStone
	sb.Decor.Trees, sb.Decor.DeadBush, sb.Decor.Reeds, sb.Decor.Cacti = -999, 0, 0, 0
	cb := def(26, "Cold Beach", hShores, 0.05, 0.3, clsBeach)
	cb.Top, cb.Fill, cb.Snow = stSand, stSand, true
	cb.Decor = beach.Decor
	bf := def(27, "Birch Forest", hDefault, 0.6, 0.6, clsForest)
	bf.Forest, bf.Decor = 2, forest.Decor
	bfh := def(28, "Birch Forest Hills", hLowHills, 0.6, 0.6, clsForest)
	bfh.Forest, bfh.Decor = 2, forest.Decor
	rf := def(29, "Roofed Forest", hDefault, 0.7, 0.8, clsForest)
	rf.Forest, rf.Decor = 3, forest.Decor
	rf.Decor.Trees = -999
	ct := def(30, "Cold Taiga", hMidPlains, -0.5, 0.4, clsTaiga)
	ct.Snow, ct.Decor = true, taiga.Decor
	cth := def(31, "Cold Taiga Hills", hLowHills, -0.5, 0.4, clsTaiga)
	cth.Snow, cth.Decor = true, taiga.Decor
	mt := def(32, "Mega Taiga", hMidPlains, 0.3, 0.8, clsTaiga)
	mt.TaigaType = 1
	mt.Decor.Trees, mt.Decor.Grass, mt.Decor.DeadBush, mt.Decor.Mushrooms = 10, 7, 1, 3
	mth := def(33, "Mega Taiga Hills", hLowHills, 0.3, 0.8, clsTaiga)
	mth.TaigaType, mth.Decor = 1, mt.Decor
	hp := def(34, "Extreme Hills+", hMidHills, 0.2, 0.3, clsHills)
	hp.Hills = 1
	hp.Decor.Trees = 3
	sv := def(35, "Savanna", hLowPlains, 1.2, 0.0, clsSavanna)
	sv.Decor.Trees, sv.Decor.Flowers, sv.Decor.Grass = 1, 4, 20
	sp := def(36, "Savanna Plateau", hPlateau, 1.0, 0.0, clsSavanna)
	sp.Decor = sv.Decor
	mesa := def(37, "Mesa", hDefault, 2.0, 0.0, clsMesa)
	mesa.Top, mesa.Fill = stRedSand, stRedSand
	mesa.Decor = Decor{Trees: -999, Flowers: 0, DeadBush: 20, Reeds: 3, Cacti: 5, Sand: 1, Sand2: 3, Clay: 1}
	mpf := def(38, "Mesa Plateau F", hPlateau, 2.0, 0.0, clsMesa)
	mpf.Top, mpf.Fill, mpf.Decor = stRedSand, stRedSand, mesa.Decor
	mpf.Decor.Trees = 5
	mp := def(39, "Mesa Plateau", hPlateau, 2.0, 0.0, clsMesa)
	mp.Top, mp.Fill, mp.Decor = stRedSand, stRedSand, mesa.Decor
	mp.Decor.Trees = 5

	// The mutations that exist in 1.8.9.
	for _, id := range []int{1, 2, 3, 4, 5, 6, 12, 21, 23, 27, 28, 29, 30, 32, 33, 34, 35, 36, 37, 38, 39} {
		m := mutate(biomes[id])
		switch id {
		case 3, 34:
			m.Hills = 2
		case 4:
			m.Forest, m.Decor.Trees, m.Decor.Flowers, m.Decor.Grass = 1, 6, 100, 1 // Flower Forest
		case 32, 33:
			m.TaigaType = 2
		case 1:
			m.Decor.Flowers = 4
		}
	}
	biomes[140].Name = "Ice Plains Spikes"
	biomes[132].Name = "Flower Forest"
}

func biomeByID(id int) *Biome {
	if id < 0 || id > 255 {
		return nil
	}
	return biomes[id]
}

// hasMutation is whether BiomeGenBase.getBiome(id+128) exists.
func hasMutation(id int) bool { return id >= 0 && id < 128 && biomes[id+128] != nil }
