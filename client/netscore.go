package client

import (
	"sort"
	"strconv"
	"strings"
)

// The scoreboard a server keeps for the player: objectives with scores, shown in the list, in the sidebar or
// under names; and teams, which give names their prefix, suffix and colour. Servers build their lobbies'
// side panels from it. The sidebar is drawn as GuiIngame.renderScoreboard does.

type scoreObjective struct {
	name, display string
	hearts        bool // its type is "hearts": the list shows hearts for the score
}

type scoreTeam struct {
	name, display, prefix, suffix string
	flags                         byte
	tagVisibility                 string
	colour                        int8 // the chat colour index of its members, -1 for none
	players                       map[string]bool
}

type scoreboard struct {
	objectives map[string]*scoreObjective
	scores     map[string]map[string]int32 // objective -> entry -> score
	slots      [19]string                  // the objective shown in each slot: 0 list, 1 sidebar, 2 under names, 3 to 18 the sidebar of a team colour
	teams      map[string]*scoreTeam
	team       map[string]*scoreTeam // entry -> its team
}

func (n *netState) board() *scoreboard {
	if n.sb == nil {
		n.sb = &scoreboard{objectives: map[string]*scoreObjective{}, scores: map[string]map[string]int32{}, teams: map[string]*scoreTeam{}, team: map[string]*scoreTeam{}}
	}
	return n.sb
}

// objective applies Scoreboard Objective: mode 0 creates, 1 removes, 2 renames.
func (sb *scoreboard) objective(name string, mode byte, display string, hearts bool) {
	switch mode {
	case 0, 2:
		if o := sb.objectives[name]; o != nil && mode == 2 {
			o.display, o.hearts = display, hearts
		} else {
			sb.objectives[name] = &scoreObjective{name, display, hearts}
		}
	case 1:
		delete(sb.objectives, name)
		delete(sb.scores, name)
		for i, s := range sb.slots {
			if s == name {
				sb.slots[i] = ""
			}
		}
	}
}

func (sb *scoreboard) setScore(entry, objective string, value int32, remove bool) {
	if remove {
		delete(sb.scores[objective], entry)
		return
	}
	if sb.scores[objective] == nil {
		sb.scores[objective] = map[string]int32{}
	}
	sb.scores[objective][entry] = value
}

// teamUpdate applies Teams: 0 creates, 1 removes, 2 updates, 3 adds players, 4 removes players.
func (sb *scoreboard) teamUpdate(name string, mode byte, t scoreTeam, players []string) {
	switch mode {
	case 1:
		if old := sb.teams[name]; old != nil {
			for p := range old.players {
				delete(sb.team, p)
			}
		}
		delete(sb.teams, name)
		return
	case 0:
		nt := t
		nt.name, nt.players = name, map[string]bool{}
		sb.teams[name] = &nt
	case 2:
		if cur := sb.teams[name]; cur != nil {
			cur.display, cur.prefix, cur.suffix, cur.flags, cur.tagVisibility, cur.colour = t.display, t.prefix, t.suffix, t.flags, t.tagVisibility, t.colour
		}
	}
	cur := sb.teams[name]
	if cur == nil {
		return
	}
	switch mode {
	case 0, 3:
		for _, p := range players {
			if old := sb.team[p]; old != nil {
				delete(old.players, p)
			}
			cur.players[p] = true
			sb.team[p] = cur
		}
	case 4:
		for _, p := range players {
			delete(cur.players, p)
			if sb.team[p] == cur {
				delete(sb.team, p)
			}
		}
	}
}

// format is ScorePlayerTeam.formatPlayerName: the name between its team's prefix and suffix.
func (sb *scoreboard) format(name string) string {
	if sb != nil {
		if t := sb.team[name]; t != nil {
			return t.prefix + name + t.suffix
		}
	}
	return name
}

type scoreLine struct {
	name  string
	value int32
}

// sidebarLines are the entries of an objective in the order the game draws them from the bottom: by score
// and, for equal scores, by name backwards, keeping the last fifteen.
func (sb *scoreboard) sidebarLines(objective string) []scoreLine {
	var ls []scoreLine
	for name, v := range sb.scores[objective] {
		if !strings.HasPrefix(name, "#") {
			ls = append(ls, scoreLine{name, v})
		}
	}
	sort.Slice(ls, func(i, j int) bool {
		if ls[i].value != ls[j].value {
			return ls[i].value < ls[j].value
		}
		return strings.ToLower(ls[i].name) > strings.ToLower(ls[j].name)
	})
	if len(ls) > 15 {
		ls = ls[len(ls)-15:]
	}
	return ls
}

// sidebarObjective is what the sidebar shows this player: their team colour's own panel, else the common one.
func (sb *scoreboard) sidebarObjective(player string) *scoreObjective {
	if t := sb.team[player]; t != nil && t.colour >= 0 && int(t.colour) < 16 {
		if o := sb.objectives[sb.slots[3+int(t.colour)]]; o != nil {
			return o
		}
	}
	return sb.objectives[sb.slots[1]]
}

// drawSidebar draws the sidebar at the right of the screen, as GuiIngame.renderScoreboard does.
func (g *game) drawSidebar(w, h int) {
	n := g.net
	if n == nil || n.sb == nil {
		return
	}
	obj := n.sb.sidebarObjective(n.c.name)
	if obj == nil {
		return
	}
	lines := n.sb.sidebarLines(obj.name)
	g.enableUI()
	s := int(g.scaleFor(w, h))
	sw, sh := (w+s-1)/s, (h+s-1)/s // the scaled resolution, in GUI pixels
	hd := &g.hud
	fs := float32(s)
	rect := func(x1, y1, x2, y2 int, a float32) {
		hd.rect(float32(x1)*fs, float32(y1)*fs, float32(x2-x1)*fs, float32(y2-y1)*fs, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, a})
		hd.flush(hd.white)
	}
	title := legacyRuns(obj.display, defaultStyle)
	type row struct{ name, score []styledRun }
	rows := make([]row, len(lines))
	width := g.runsWidth(title)
	for i, l := range lines {
		rows[i] = row{legacyRuns(n.sb.format(l.name), defaultStyle), legacyRuns("§c"+strconv.Itoa(int(l.value)), defaultStyle)}
		full := append(append(append([]styledRun{}, rows[i].name...), styledRun{": ", defaultStyle}), rows[i].score...)
		width = max(width, g.runsWidth(full))
	}
	const fontH = 9
	j1 := sh/2 + len(lines)*fontH/3
	l1 := sw - width - 3
	for j := 1; j <= len(lines); j++ {
		r := rows[j-1]
		k := j1 - j*fontH
		l := sw - 3 + 2
		rect(l1-2, k, l, k+fontH, 0x50/255.0)
		g.drawRuns(r.name, float32(l1)*fs, float32(k)*fs, fs, 1, false)
		g.drawRuns(r.score, float32(l-g.runsWidth(r.score))*fs, float32(k)*fs, fs, 1, false)
		if j == len(lines) {
			rect(l1-2, k-fontH-1, l, k-1, 0x60/255.0)
			rect(l1-2, k-1, l, k, 0x50/255.0)
			g.drawRuns(title, float32(l1+width/2-g.runsWidth(title)/2)*fs, float32(k-fontH)*fs, fs, 1, false)
		}
	}
	if len(lines) == 0 { // the title of an empty panel is not drawn by the game
		return
	}
}
