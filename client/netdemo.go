package client

import (
	"fmt"
	"math"
	"net"

	"gocraft/world"
)

// netDemo fills the game with what a lobby shows, for a screenshot (-screenshot -screen netmenu): coloured
// chat, a hologram, and a game selector, with no server behind it.
func (g *game) netDemo(menu bool) {
	g.fb = [2]int{g.opt.Width, g.opt.Height}
	conn, _ := net.Pipe()
	g.net = &netState{c: &netClient{conn: conn, done: make(chan struct{})}, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	for _, js := range []string{
		`{"text":"","extra":[{"text":"Cuenta » ","color":"gold","bold":true},{"text":"¡Inicia sesión! ","color":"green"},{"text":"/login <tucontraseña>","color":"yellow"}]}`,
		`{"text":"§7[§aUni§7] §fRecompensas: Hola §b000danielz§f, tengo §e1 §frecompensas para ti... ven a reclamarlas."}`,
		`{"text":"Posición #4185 (-177) en Victorias de Ranked","color":"white","extra":[{"text":" ¡Ñandú! ","color":"red","bold":true,"underlined":true}]}`,
		`{"text":"§c▬▬▬▬▬▬ §a✔ §4❤ §fLeyenda §e★ §7› §6§l●"}`,
		`{"text":"§c§lSkyWars §7» §fpartida §nlista§r §kabcd §rsin formato"}`,
	} {
		g.addChatRuns(parseChat(js))
	}
	g.net.addEntity(&netEntity{eid: 1, kind: mobArmorStand, pos: vec3{0.5, g.feet[1] + 1.2, 4.5}, showName: true,
		name: legacyRuns("§6§lTOP KILLS §f(semana)", defaultStyle), marker: true})
	g.net.addEntity(&netEntity{eid: 2, kind: mobArmorStand, pos: vec3{0.5, g.feet[1] + 0.9, 4.5}, showName: true,
		name: legacyRuns("§e1. §fHerobrine §7- §a312", defaultStyle), marker: true})
	g.net.addEntity(&netEntity{eid: 3, kind: mobArmorStand, pos: vec3{0.5, g.feet[1] + 0.6, 4.5}, showName: true,
		name: legacyRuns("§e2. §fSteve §7- §a287", defaultStyle), marker: true})
	g.creative = false
	g.health, g.food, g.saturation = 13, 15, 0
	g.absorption, g.maxHP = 6, 20
	g.xpBar, g.xpLevel = 0.6, 12
	g.armor = [4]slot{{ID: 310, N: 1}, {ID: 311, N: 1}, {ID: 312, N: 1}, {ID: 313, N: 1}}
	g.effects = map[int]potionEffect{effRegen: {0, 600}}
	g.hurtResist = 10
	g.statusBars.shownHP, g.statusBars.lastHP = 16, 16
	g.statusBars.blinkUntil = 9
	sb := g.net.board()
	sb.objective("lobby", 0, "§e§lUNIVERSOCRAFT")
	sb.slots[1] = "lobby"
	for i, e := range []string{"§1", "§fRango: §aVIP", "§fCoins: §e1250", "§2", "§fKills: §c312", "§fElo: §b1840", "§3", "§euniversocraft.com"} {
		sb.setScore(e, "lobby", int32(8-i), false)
	}
	if !menu {
		return
	}
	g.openServerWindow(1, "minecraft:chest", `{"text":"§0Selector de juegos"}`, 27)
	w := g.swin
	for i, e := range []struct {
		id   uint16
		name string
		lore []string
	}{{267, "§c§lSkyWars", []string{"§7Lucha por la isla", "§e12 jugando"}}, {355, "§b§lBedWars", []string{"§7Protege tu cama"}}, {276, "§a§lArenaPvP", []string{"§7Ranked y casual"}}} {
		slotN := 10 + 2*i
		w.slots[slotN] = slot{ID: e.id, N: 1}
		ex := itemExtra{name: legacyRuns(e.name, defaultStyle)}
		for _, l := range e.lore {
			ex.lore = append(ex.lore, legacyRuns(l, textStyle{col: chatColours[5], italic: true}))
		}
		w.extra[slotN] = ex
	}
	g.inv[0], g.inv[1] = slot{ID: 345, N: 1}, slot{ID: 264, N: 3}
	_, ph := w.layout()
	sc, left, top := g.windowPanel(ph)
	g.cx, g.cy = float64(left+(62+8)*sc), float64(top+(36+8)*sc) // the mouse rests on the second game
}

// fishDemo puts the player's own hook out in front of them, and another player fishing beside it.
func (g *game) fishDemo() {
	g.net.eid = 7
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	spot := g.feet.add(f.scale(3.5)).add(right.scale(-0.8)).add(vec3{0, 0.1, 0})
	hook := func(eid, owner int32, at vec3) {
		g.net.addEntity(&netEntity{eid: eid, obj: objFishHook, owner: owner, pos: at, prev: at, target: at})
	}
	hook(60, 7, spot)
	at := g.feet.add(f.scale(7)).add(right.scale(-3))
	p := &remotePlayer{eid: 9, name: "Steve", pos: at, prev: at, target: at, yaw: 0}
	g.net.remote[9] = p
	hook(61, 9, at.add(f.scale(4)).add(vec3{0, 0.1, 0}))
	p.armor = [4]slot{{ID: 313, N: 1}, {ID: 312, N: 1}, {ID: 311, N: 1}, {ID: 310, N: 1}} // diamond from boots to helmet
	p.held = slot{ID: 346, N: 1}
	red := itemExtra{colour: 0xCC2222, hasColour: true}
	at2 := g.feet.add(f.scale(5)).add(right.scale(2.5))
	q := &remotePlayer{eid: 10, name: "Alex", pos: at2, prev: at2, target: at2, yaw: 160, tyaw: 160, headYaw: 160, thead: 160, body: 160, prevBody: 160, sneaking: true, using: true, useTicks: 20, held: slot{ID: 261, N: 1},
		armor:  [4]slot{{ID: 301, N: 1}, {ID: 300, N: 1}, {ID: 299, N: 1}, {ID: 298, N: 1}},
		armorX: [4]itemExtra{red, red, red, red}}
	g.net.remote[10] = q
	at3 := g.feet.add(f.scale(4)).add(right.scale(-3.5))
	g.net.remote[11] = &remotePlayer{eid: 11, name: "Notch", pos: at3, prev: at3, target: at3, yaw: 20, tyaw: 20, headYaw: 20, thead: 20, body: 20, prevBody: 20, using: true, held: slot{ID: 267, N: 1},
		armor: [4]slot{{ID: 309, N: 1}, {ID: 308, N: 1}, {ID: 307, N: 1}, {ID: 306, N: 1}}} // iron
}

// heldDemo gives two more players a block and a flat item.
func (g *game) heldDemo() {
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	at := g.feet.add(f.scale(3)).add(right.scale(1.2))
	g.net.remote[12] = &remotePlayer{eid: 12, name: "Dinah", pos: at, prev: at, target: at, yaw: 180, tyaw: 180, headYaw: 180, thead: 180, body: 180, prevBody: 180, held: slot{ID: 1, N: 1}}
	at = g.feet.add(f.scale(3)).add(right.scale(-1.2))
	g.net.remote[13] = &remotePlayer{eid: 13, name: "Eve", pos: at, prev: at, target: at, yaw: 200, tyaw: 200, headYaw: 200, thead: 200, body: 200, prevBody: 200, held: slot{ID: 368, N: 1}}
}

// profileDemo puts three players in a row, seen from the side, with a sword, a rod and a drawn bow.
func (g *game) profileDemo() {
	g.net.eid = 7
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	for i, h := range []remotePlayer{
		{held: slot{ID: 276, N: 1}},
		{held: slot{ID: 346, N: 1}},
		{held: slot{ID: 261, N: 1}, using: true, useTicks: 20},
		{held: slot{ID: 267, N: 1}, using: true},
	} {
		at := g.feet.add(f.scale(3)).add(right.scale(float64(i-1)*1.4 - 0.7))
		p := h
		p.eid, p.name, p.pos, p.prev, p.target = int32(20+i), "", at, at, at
		p.yaw, p.tyaw, p.headYaw, p.thead, p.body, p.prevBody = 90, 90, 90, 90, 90, 90
		g.net.remote[p.eid] = &p
	}
	hook := g.net.remote[21].pos.add(vec3{1.2, 0.1, 0})
	g.net.addEntity(&netEntity{eid: 70, obj: objFishHook, owner: 21, pos: hook, prev: hook, target: hook})
}

// listDemo fills the player list and shows it.
func (g *game) listDemo() {
	g.listForced = true
	g.net.header = legacyRuns("§6§lUNIVERSOCRAFT §7Network\n§fJugadores: §a529", defaultStyle)
	g.net.footer = legacyRuns("§euniversocraft.com §7| §bdiscord.universocraft.com", defaultStyle)
	g.net.tab = map[string]*tabEntry{}
	names := []string{"000danielz", "Herobrine", "Steve", "Alex", "Notch", "jeb_", "Dinnerbone", "Grumm", "xXProXx", "Kratos", "Zeus", "Hades", "Athena", "Apolo", "Ares", "Hera", "Eros", "Gaia", "Nyx", "Pan", "Rea", "Tetis", "Uranio", "Febe", "Hebe", "Iris", "Mnemosine", "Temis", "Cronos", "Hestia"}
	for i, nm := range names {
		e := &tabEntry{uuid: fmt.Sprintf("%032x", i+1), name: nm, ping: []int{12, 80, 160, 320, 700, 1200}[i%6]}
		if i == 1 {
			e.display = legacyRuns("§c[ADMIN] Herobrine", defaultStyle)
		}
		if i == 4 {
			e.gamemode = 3
		}
		g.net.tab[e.uuid] = e
	}
	g.net.sb = nil
	g.net.board().objective("kills", 0, "Kills")
	g.net.board().slots[0] = "kills"
	for i, nm := range names {
		g.net.board().setScore(nm, "kills", int32(i*7%50), false)
	}
	g.net.board().teamUpdate("admin", 0, scoreTeam{prefix: "§c", colour: -1}, []string{"Herobrine"})
}

// itemsDemo lays a row of the server's dropped items in front of the player (blocks, flat items, a big
// stack) and one more on its way to another player who has just taken it.
func (g *game) itemsDemo() {
	g.net.eid = 7
	f := look(g.yaw, 0)
	right := vec3{-f[2], 0, f[0]}
	ground := vec3{0, math.Floor(g.feet[1]) - g.feet[1], 0}
	for i, s := range []slot{{ID: 4, N: 3}, {ID: 264, N: 1}, {ID: 2, N: 64}, {ID: 276, N: 1}, {ID: 35, Dmg: 14, N: 5}, {ID: 50, N: 1}} {
		at := g.feet.add(ground).add(f.scale(3)).add(right.scale(float64(i)*0.7 - 1.75))
		g.netSpawnItem(int32(100+i), at, vec3{}, true)
		e := g.net.netItem(int32(100 + i))
		e.stack, e.age, e.hover = s, 7*i, float32(i)
	}
	at := g.feet.add(ground).add(f.scale(6)).add(right.scale(2))
	g.net.remote[12] = &remotePlayer{eid: 12, name: "Steve", pos: at, prev: at, target: at, yaw: 200, tyaw: 200, headYaw: 200, thead: 200, body: 200, prevBody: 200}
	lay := at.add(right.scale(-2.2))
	g.net.pickups = append(g.net.pickups, &pickupFX{item: itemEntity{net: true, stack: slot{ID: 266, N: 1}, pos: lay, prev: lay}, to: 12, age: 1})
}
