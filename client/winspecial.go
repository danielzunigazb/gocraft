package client

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// The windows of 1.8.9 that are more than rows of slots: the enchanting table (GuiEnchantment), the anvil
// (GuiRepair) and the brewing stand (GuiBrewingStand). A server opens them with a kind and the game builds the
// container by that kind, so the slots and drawings here are the game's, not the server's. The numbers are the
// ones in the game's bytecode.

const (
	kindEnchant = "minecraft:enchanting_table"
	kindAnvil   = "minecraft:anvil"
	kindBrewing = "minecraft:brewing_stand"
)

// windowOwnSlots is how many slots a window has of its own. The game builds a crafting table, an enchanting
// table and an anvil by their kind (a server's Open Window says 0 for them, as they are not an inventory),
// so the packet's count is not believed there.
func windowOwnSlots(kind string, n int) int {
	switch kind {
	case "minecraft:crafting_table":
		return 10
	case kindEnchant:
		return 2
	case kindAnvil:
		return 3
	case kindBrewing:
		return 4
	}
	return n
}

// specialLayout is the slots of an enchanting table, anvil or brewing stand, and the panel's height.
func specialLayout(kind string, add func(x, y int), player func(invY, hotY int)) (h int, ok bool) {
	switch kind {
	case kindEnchant:
		add(15, 47)
		add(35, 47)
	case kindAnvil:
		add(27, 47)
		add(76, 47)
		add(134, 47)
	case kindBrewing:
		add(56, 46)
		add(79, 53)
		add(102, 46)
		add(79, 17)
	default:
		return 0, false
	}
	player(84, 142)
	return 166, true
}

func rgb(c uint32) [4]float32 {
	return [4]float32{float32(c>>16&255) / 255, float32(c>>8&255) / 255, float32(c&255) / 255, 1}
}

// enchantName is the language key of the enchantment of an id (Enchantment.getName).
var enchantName = map[int]string{
	0: "protect.all", 1: "protect.fire", 2: "protect.fall", 3: "protect.explosion", 4: "protect.projectile", 5: "oxygen",
	6: "waterWorker", 7: "thorns", 8: "waterWalker", 16: "damage.all", 17: "damage.undead", 18: "damage.arthropods",
	19: "knockback", 20: "fire", 21: "lootBonus", 32: "digging", 33: "untouching", 34: "durability", 35: "lootBonusDigger",
	48: "arrowDamage", 49: "arrowKnockback", 50: "arrowFire", 51: "arrowInfinite", 61: "lootBonusFishing", 62: "fishingSpeed",
}

// roman is the numeral of an enchantment level, as the game's "enchantment.level.N" gives it.
func roman(n int) string {
	if n >= 1 && n <= 10 {
		return [...]string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}[n-1]
	}
	return fmt.Sprint(n)
}

// enchantClue is what an enchanting button's tooltip hints at: "Sharpness II . . . ?".
func (g *game) enchantClue(id, level int) string {
	key, ok := enchantName[id]
	if !ok {
		return ""
	}
	name := g.res.lang["enchantment."+key]
	if name == "" {
		name = key
	}
	if level > 0 {
		name += " " + roman(level)
	}
	return strings.Replace(g.res.lang["container.enchant.clue"], "%s", name, 1)
}

// lapisCount is how much lapis lazuli sits in the enchanting table's second slot.
func (w *serverWindow) lapisCount() int {
	if len(w.slots) > 1 && w.slots[1].ID == 351 && w.slots[1].Dmg == 4 {
		return int(w.slots[1].N)
	}
	return 0
}

// enchantEnabled is GuiEnchantment's test for a button: it shows an enchantment, the player has the levels and
// the lapis for it (a creative player needs neither).
func (g *game) enchantEnabled(w *serverWindow, i int) bool {
	lv := w.props[i]
	if lv <= 0 || len(w.slots) < 1 || w.slots[0].empty() {
		return false
	}
	return g.creative || (w.lapisCount() >= i+1 && g.xpLevel >= lv)
}

// enchantButton is the button of the enchanting table the mouse is over, or -1: a box of 108 x 19 pixels
// from (60, 14), one under the other.
func enchantButtonAt(x, y float32) int {
	for i := 0; i < 3; i++ {
		if x >= 60 && x < 168 && y >= float32(14+19*i) && y < float32(14+19*i+19) {
			return i
		}
	}
	return -1
}

// windowButton is a click on one of the controls of a special window; it reports whether it took the click.
// An enchanting table's button sends Enchant Item (0x11) when the game's container would accept it.
func (g *game) windowButton(x, y float32) bool {
	w := g.swin
	if w == nil || w.kind != kindEnchant {
		return false
	}
	i := enchantButtonAt(x, y)
	if i < 0 || !g.enchantEnabled(w, i) || g.net == nil {
		return i >= 0
	}
	p := newPacket(0x11)
	p.Byte(byte(w.id))
	p.Byte(byte(i))
	g.net.c.send(p)
	return true
}

// ---- the anvil's name field ----

// anvilTyping is whether the anvil's name field has the keyboard (it does whenever the window is open).
func (g *game) anvilTyping() bool {
	return g.screen == screenServerWin && g.swin != nil && g.swin.kind == kindAnvil
}

// syncAnvilName puts the name of the item in the first slot into the field when the item changes, as
// GuiRepair.sendSlotContents does.
func (g *game) syncAnvilName() {
	w := g.swin
	if w == nil || w.kind != kindAnvil || len(w.slots) < 1 {
		return
	}
	it, ex := w.slots[0], w.extra[0]
	key := fmt.Sprint(it.ID, it.Dmg, it.N, plainRuns(ex.name))
	if key == w.nameFor {
		return
	}
	w.nameFor = key
	if it.empty() {
		w.name = ""
		return
	}
	w.name = plainRuns(ex.name)
	if w.name == "" {
		w.name = g.displayName(it)
	}
	g.renameAnvil()
}

// renameAnvil tells the server the name in the field (MC|ItemName); the item's own name is sent as empty, as
// GuiRepair.renameItem does.
func (g *game) renameAnvil() {
	w := g.swin
	if w == nil || g.net == nil {
		return
	}
	s := w.name
	if len(w.slots) > 0 && !w.slots[0].empty() && len(w.extra[0].name) == 0 && s == g.displayName(w.slots[0]) {
		s = ""
	}
	p := newPacket(0x17)
	p.String("MC|ItemName")
	p.String(s)
	g.net.c.send(p)
}

// anvilChar is a character typed into the name field (up to 40).
func (g *game) anvilChar(r rune) {
	w := g.swin
	if w == nil || len(w.slots) < 1 || w.slots[0].empty() {
		return // the field is disabled until there is an item
	}
	if rs := []rune(w.name); len(rs) < 40 {
		w.name += string(r)
		g.renameAnvil()
	}
}

// anvilKey is a key pressed while the anvil's name field has the keyboard.
func (g *game) anvilKey(key glfw.Key) {
	w := g.swin
	switch key {
	case glfw.KeyBackspace:
		if rs := []rune(w.name); len(rs) > 0 && !w.slots[0].empty() {
			w.name = string(rs[:len(rs)-1])
			g.renameAnvil()
		}
	case glfw.KeyEscape:
		g.closeServerWindow(true)
	}
}

// ---- drawing ----

// drawSpecialBackground draws the panel of a special window and what is on it that is not an item; it returns
// the offset of the title and of the "Inventory" label. mx, my is the mouse.
func (g *game) drawSpecialBackground(win *serverWindow, left, top, s, mx, my float32) (titleX, invLabel int) {
	hd := &g.hud
	one := [4]float32{1, 1, 1, 1}
	switch win.kind {
	case kindEnchant:
		hd.rect(left, top, contW*s, 166*s, 0, 0, contW, 166, 256, 256, one)
		x, y := (mx-left)/s, (my-top)/s
		hover := enchantButtonAt(x, y)
		for i := 0; i < 3; i++ {
			bx, by := left+60*s, top+float32(14+19*i)*s
			if win.props[i] <= 0 { // no enchantment on offer
				hd.rect(bx, by, 108*s, 19*s, 0, 185, 108, 19, 256, 256, one)
				continue
			}
			if !g.enchantEnabled(win, i) {
				hd.rect(bx, by, 108*s, 19*s, 0, 185, 108, 19, 256, 256, one)
				hd.rect(bx+s, by+s, 16*s, 16*s, float32(16*i), 239, 16, 16, 256, 256, one)
				continue
			}
			v := float32(166)
			if hover == i {
				v = 204
			}
			hd.rect(bx, by, 108*s, 19*s, 0, v, 108, 19, 256, 256, one)
			hd.rect(bx+s, by+s, 16*s, 16*s, float32(16*i), 223, 16, 16, 256, 256, one)
		}
		hd.flush(g.guiTexture("enchanting_table"))
		for i := 0; i < 3; i++ { // the level each costs, at the right of its button
			if win.props[i] <= 0 {
				continue
			}
			col := rgb(0x80FF20)
			if !g.enchantEnabled(win, i) {
				col = rgb(0x407F10)
			}
			num := fmt.Sprint(win.props[i])
			w := float32(g.textWidth(num))
			g.text(num, left+(166-w)*s, top+float32(23+19*i)*s, s, col, true)
		}
		return 12, 72
	case kindAnvil:
		hd.rect(left, top, contW*s, 166*s, 0, 0, contW, 166, 256, 256, one)
		has := len(win.slots) > 0 && !win.slots[0].empty()
		v := float32(166 + 16)
		if has {
			v = 166
		}
		hd.rect(left+59*s, top+20*s, 110*s, 16*s, 0, v, 110, 16, 256, 256, one)
		if (has || len(win.slots) > 1 && !win.slots[1].empty()) && len(win.slots) > 2 && win.slots[2].empty() {
			hd.rect(left+99*s, top+45*s, 28*s, 21*s, 176, 0, 28, 21, 256, 256, one) // the cross: nothing comes of it
		}
		hd.flush(g.guiTexture("anvil"))
		g.syncAnvilName()
		if has {
			cursor := ""
			if (time.Now().UnixMilli()/500)%2 == 0 {
				cursor = "_"
			}
			g.text(win.name+cursor, left+62*s, top+26*s, s, [4]float32{1, 1, 1, 1}, true)
		}
		g.drawAnvilCost(win, left, top, s)
		return 60, -1
	case kindBrewing:
		hd.rect(left, top, contW*s, 166*s, 0, 0, contW, 166, 256, 256, one)
		if t := win.props[0]; t > 0 { // GuiBrewingStand: the arrow runs down as the brew goes, the bubbles rise
			k := int(28 * (1 - float32(t)/400))
			if k > 0 {
				hd.rect(left+97*s, top+16*s, 9*s, float32(k)*s, 176, 0, 9, float32(k), 256, 256, one)
			}
			b := [7]int{29, 24, 20, 16, 11, 6, 0}[(t/2)%7]
			if b > 0 {
				hd.rect(left+65*s, top+float32(14+29-b)*s, 12*s, float32(b)*s, 185, float32(29-b), 12, float32(b), 256, 256, one)
			}
		}
		hd.flush(g.guiTexture("brewing_stand"))
		return -1, 72
	}
	return 8, 72
}

// drawAnvilCost is the "Enchantment Cost: N" box at the bottom right of the anvil (GuiRepair.drawGuiContainerForegroundLayer).
func (g *game) drawAnvilCost(win *serverWindow, left, top, s float32) {
	cost := win.props[0]
	if cost <= 0 {
		return
	}
	col := uint32(0x80FF20)
	show := true
	text := strings.Replace(g.res.lang["container.repair.cost"], "%1$d", fmt.Sprint(cost), 1)
	switch {
	case cost >= 40 && !g.creative:
		text, col = g.res.lang["container.repair.expensive"], 0xFF6060
	case len(win.slots) < 3 || win.slots[2].empty():
		show = false
	case g.xpLevel < cost && !g.creative: // the result cannot be taken
		col = 0xFF6060
	}
	if !show {
		return
	}
	hd := &g.hud
	w := float32(g.textWidth(text))
	x, y := contW-8-w, float32(67)
	hd.rect(left+(x-3)*s, top+(y-2)*s, (contW-7-(x-3))*s, 12*s, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 1})
	hd.rect(left+(x-2)*s, top+(y-1)*s, (contW-8-(x-2))*s, 10*s, 0, 0, 1, 1, 1, 1, [4]float32{0x3A / 255.0, 0x3A / 255.0, 0x3B / 255.0, 1})
	hd.flush(hd.white)
	dark := rgb((col & 0xFCFCFC) >> 2)
	for _, o := range [][2]float32{{0, 1}, {1, 0}, {1, 1}} { // the shadow, as the game draws it by hand
		g.text(text, left+(x+o[0])*s, top+(y+o[1])*s, s, dark, false)
	}
	g.text(text, left+x*s, top+y*s, s, rgb(col), false)
}

// enchantTooltip is the tooltip of an enchanting button the mouse is over: the hint at the enchantment, and
// what it costs in lapis and levels (a creative player is told no cost).
func (g *game) enchantTooltip(win *serverWindow, i int) []string {
	if win.props[i] <= 0 {
		return nil
	}
	clue := g.enchantClue(win.props[4+i], win.props[7+i])
	if clue == "" {
		return nil
	}
	lines := []string{"§f§o" + clue}
	if !g.creative {
		lines = append(lines, "")
		l1 := i + 1
		lapis := g.res.lang["container.enchant.lapis.one"]
		if l1 > 1 {
			lapis = fmt.Sprintf(strings.Replace(g.res.lang["container.enchant.lapis.many"], "%d", "%d", 1), l1)
		}
		lv := g.res.lang["container.enchant.level.one"]
		if l1 > 1 {
			lv = fmt.Sprintf(g.res.lang["container.enchant.level.many"], l1)
		}
		lc, ll := "§7", "§7"
		if win.lapisCount() < l1 {
			lc = "§c"
		}
		if g.xpLevel < l1 {
			ll = "§c"
		}
		lines = append(lines, lc+lapis, ll+lv)
	}
	return lines
}
