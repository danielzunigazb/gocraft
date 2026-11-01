package server

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"gocraft/proto"
)

// The server's commands: a line of chat that starts with "/". Every player may use them (an offline server has no
// operators). They are the ones a small server wants, not the game's whole set.

// commandHelp lists them, for /help.
var commandHelp = []string{
	"/help: this list",
	"/list: who is online",
	"/say <message>: say something to everyone",
	"/me <action>: do something",
	"/msg <player> <message>: a private message",
	"/tp <x> <y> <z> or /tp <player>: go somewhere (~ is relative)",
	"/time set <day|noon|night|midnight|ticks> or /time add <ticks>",
	"/give <player> <item> [count] [damage]: an item id or name",
	"/kill [player]: survival only",
	"/seed: what the world was made from",
	"/save-all: write the world now",
}

// itemNames are the names /give knows besides numeric ids.
var itemNames = map[string]int16{
	"stone": 1, "grass": 2, "dirt": 3, "cobblestone": 4, "planks": 5, "bedrock": 7, "sand": 12, "gravel": 13,
	"gold_ore": 14, "iron_ore": 15, "coal_ore": 16, "log": 17, "leaves": 18, "glass": 20, "lapis_block": 22, "wool": 35,
	"gold_block": 41, "iron_block": 42, "tnt": 46, "bookshelf": 47, "obsidian": 49, "torch": 50, "chest": 54,
	"diamond_ore": 56, "diamond_block": 57, "crafting_table": 58, "furnace": 61, "ladder": 65, "iron_shovel": 256,
	"iron_pickaxe": 257, "iron_axe": 258, "flint_and_steel": 259, "apple": 260, "bow": 261, "arrow": 262, "coal": 263,
	"diamond": 264, "iron_ingot": 265, "gold_ingot": 266, "iron_sword": 267, "wooden_sword": 268, "stone_sword": 272,
	"diamond_sword": 276, "diamond_shovel": 277, "diamond_pickaxe": 278, "diamond_axe": 279, "stick": 280,
	"golden_sword": 283, "bread": 297, "iron_helmet": 306, "iron_chestplate": 307, "iron_leggings": 308, "iron_boots": 309,
	"diamond_helmet": 310, "diamond_chestplate": 311, "diamond_leggings": 312, "diamond_boots": 313, "porkchop": 319,
	"golden_apple": 322, "water_bucket": 326, "lava_bucket": 327, "ender_pearl": 368, "experience_bottle": 384,
	"cooked_beef": 364, "fishing_rod": 346,
}

func (p *Player) tell(msg string) { p.send(chatPacket(msg)) }

// runCommand runs a line of chat that starts with "/".
func (s *Server) runCommand(p *Player, line string) {
	f := strings.Fields(strings.TrimPrefix(line, "/"))
	if len(f) == 0 {
		return
	}
	name, args := strings.ToLower(strings.TrimPrefix(f[0], "minecraft:")), f[1:]
	switch name {
	case "help", "?":
		for _, l := range commandHelp {
			p.tell(l)
		}
	case "list":
		var names []string
		for _, o := range s.inWorld() {
			names = append(names, o.name)
		}
		sort.Strings(names)
		p.tell(fmt.Sprintf("There are %d of a max of %d players online: %s", len(names), s.cfg.MaxPlayers, strings.Join(names, ", ")))
	case "say":
		if len(args) == 0 {
			p.tell("Usage: /say <message>")
			return
		}
		s.Broadcast(chatPacket("[" + p.name + "] " + strings.Join(args, " ")))
	case "me":
		if len(args) == 0 {
			p.tell("Usage: /me <action>")
			return
		}
		s.Broadcast(chatPacket("* " + p.name + " " + strings.Join(args, " ")))
	case "msg", "tell", "w":
		if len(args) < 2 {
			p.tell("Usage: /msg <player> <message>")
			return
		}
		to := s.playerByName(args[0])
		if to == nil {
			p.tell("No player was found")
			return
		}
		msg := strings.Join(args[1:], " ")
		to.tell(p.name + " whispers to you: " + msg)
		p.tell("You whisper to " + to.name + ": " + msg)
	case "tp", "teleport":
		s.cmdTeleport(p, args)
	case "time":
		s.cmdTime(p, args)
	case "give":
		s.cmdGive(p, args)
	case "kill":
		s.cmdKill(p, args)
	case "seed":
		p.tell(fmt.Sprintf("Seed: %d", s.cfg.Seed))
	case "save-all":
		if s.store == nil {
			p.tell("This server does not keep a world (start it with -save)")
			return
		}
		if err := s.Save(); err != nil {
			p.tell("Saving failed: " + err.Error())
			return
		}
		p.tell("Saved the world")
	default:
		p.tell("Unknown command: /" + name + ". Type /help for a list")
	}
}

func (s *Server) playerByName(name string) *Player {
	for _, o := range s.inWorld() {
		if strings.EqualFold(o.name, name) {
			return o
		}
	}
	return nil
}

// coord reads a coordinate: a number, or ~ and ~n for relative to where the player is.
func coord(arg string, here float64) (float64, bool) {
	if strings.HasPrefix(arg, "~") {
		if arg == "~" {
			return here, true
		}
		d, err := strconv.ParseFloat(arg[1:], 64)
		return here + d, err == nil && finite(here+d)
	}
	v, err := strconv.ParseFloat(arg, 64)
	return v, err == nil && finite(v)
}

func (s *Server) cmdTeleport(p *Player, args []string) {
	p.mu.Lock()
	x, y, z, yaw, pitch := p.x, p.y, p.z, p.yaw, p.pitch
	p.mu.Unlock()
	switch len(args) {
	case 1: // to a player
		to := s.playerByName(args[0])
		if to == nil {
			p.tell("No player was found")
			return
		}
		to.mu.Lock()
		x, y, z = to.x, to.y, to.z
		to.mu.Unlock()
	case 3:
		var ok [3]bool
		x, ok[0] = coord(args[0], x)
		y, ok[1] = coord(args[1], y)
		z, ok[2] = coord(args[2], z)
		if !ok[0] || !ok[1] || !ok[2] {
			p.tell("Those are not coordinates")
			return
		}
	default:
		p.tell("Usage: /tp <x> <y> <z> or /tp <player>")
		return
	}
	p.mu.Lock()
	p.x, p.y, p.z = x, y, z
	p.mu.Unlock()
	p.teleport(x, y, z, yaw, pitch)
	p.tell(fmt.Sprintf("Teleported to %.2f, %.2f, %.2f", x, y, z))
}

// timeOf is a time of day by name, or the number of ticks.
func timeOf(arg string) (int64, bool) {
	switch strings.ToLower(arg) {
	case "day":
		return 1000, true
	case "noon":
		return 6000, true
	case "night":
		return 13000, true
	case "midnight":
		return 18000, true
	}
	n, err := strconv.ParseInt(arg, 10, 64)
	return n, err == nil && n >= 0
}

// timePacket is Time Update: the clock is stopped at t (a negative time stops it, as the game does).
func timePacket(t int64) []byte {
	w := proto.NewPacket(0x03)
	w.Long(0)
	if t == 0 {
		t = 1 // -0 would not stop the clock
	}
	w.Long(-t)
	return w.Frame()
}

func (s *Server) cmdTime(p *Player, args []string) {
	if len(args) != 2 || (args[0] != "set" && args[0] != "add") {
		p.tell("Usage: /time set <day|noon|night|midnight|ticks> or /time add <ticks>")
		return
	}
	n, ok := timeOf(args[1])
	if !ok {
		p.tell("That is not a time")
		return
	}
	s.mu.Lock()
	if args[0] == "set" {
		s.clock = n % 24000
	} else {
		s.clock = (s.clock + n) % 24000
	}
	t := s.clock
	s.mu.Unlock()
	s.Broadcast(timePacket(t))
	p.tell(fmt.Sprintf("Set the time to %d", t))
}

// give puts count of an item in a player's inventory: onto a stack of the same that has room, else in the first
// empty slot, the hotbar first. It reports how many did not fit and the slots that changed.
func (p *Player) give(id int16, damage int16, count int) (left int, changed []int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	order := make([]int, 0, 36)
	for i := 36; i < 45; i++ {
		order = append(order, i)
	}
	for i := 9; i < 36; i++ {
		order = append(order, i)
	}
	for _, i := range order { // fill what is there first
		it := &p.inv[i]
		if count > 0 && it.id == id && it.damage == damage && it.count < 64 {
			add := min(count, 64-int(it.count))
			it.count += int8(add)
			count -= add
			changed = append(changed, i)
		}
	}
	for _, i := range order {
		if count <= 0 {
			break
		}
		if it := &p.inv[i]; it.id <= 0 || it.count <= 0 {
			add := min(count, 64)
			*it = slot{id: id, count: int8(add), damage: damage}
			count -= add
			changed = append(changed, i)
		}
	}
	return count, changed
}

// setSlotPacket is Set Slot for the player's own window.
func (p *Player) setSlotPacket(n int) []byte {
	w := proto.NewPacket(0x2F)
	w.Byte(0)
	w.UShort(uint16(n))
	p.mu.Lock()
	it := p.inv[n]
	p.mu.Unlock()
	if it.id <= 0 || it.count <= 0 {
		w.UShort(0xFFFF)
		return w.Frame()
	}
	w.UShort(uint16(it.id))
	w.Byte(byte(it.count))
	w.UShort(uint16(it.damage))
	w.Byte(0)
	return w.Frame()
}

func (s *Server) cmdGive(p *Player, args []string) {
	if len(args) < 2 {
		p.tell("Usage: /give <player> <item> [count] [damage]")
		return
	}
	to := s.playerByName(args[0])
	if to == nil {
		p.tell("No player was found")
		return
	}
	item := strings.TrimPrefix(strings.ToLower(args[1]), "minecraft:")
	id, ok := itemNames[item]
	if n, err := strconv.Atoi(item); err == nil && n > 0 && n < 4096 {
		id, ok = int16(n), true
	}
	if !ok {
		p.tell("There is no item called " + args[1])
		return
	}
	count, damage := 1, 0
	if len(args) > 2 {
		n, err := strconv.Atoi(args[2])
		if err != nil || n < 1 || n > 640 {
			p.tell("The count must be from 1 to 640")
			return
		}
		count = n
	}
	if len(args) > 3 {
		n, err := strconv.Atoi(args[3])
		if err != nil || n < 0 || n > 32767 {
			p.tell("That is not a damage value")
			return
		}
		damage = n
	}
	left, changed := to.give(id, int16(damage), count)
	for _, i := range changed {
		to.send(to.setSlotPacket(i))
	}
	given := count - left
	if given == 0 {
		p.tell(to.name + "'s inventory is full")
		return
	}
	p.tell(fmt.Sprintf("Gave %d of %s to %s", given, item, to.name))
}

func (s *Server) cmdKill(p *Player, args []string) {
	if !s.cfg.Survival {
		p.tell("Nobody can be hurt in a creative world")
		return
	}
	target := p
	if len(args) > 0 {
		if target = s.playerByName(args[0]); target == nil {
			p.tell("No player was found")
			return
		}
	}
	s.hurt(target, float32(math.MaxFloat32/2), nil, true, "was killed", [3]float64{})
}
