package client

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// The player list the server keeps (Player List Item) and shows with Tab (GuiPlayerTabOverlay): names with
// their display names and team colours, ping bars, the score of the list objective, a header and a footer.
// The same packet carries the skin each player wears.

type tabEntry struct {
	uuid, name string
	gamemode   int
	ping       int
	display    []styledRun // the server's own name for them in the list; nil when it has none
	skinURL    string
	slim       bool // Alex's thin arms
}

// textureProps reads the "textures" property of a profile: where the skin is, and the arms it was made for.
func textureProps(b64 string) (url string, slim bool) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", false
	}
	var v struct {
		Textures map[string]struct {
			URL      string            `json:"url"`
			Metadata map[string]string `json:"metadata"`
		} `json:"textures"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	sk := v.Textures["SKIN"]
	return sk.URL, sk.Metadata["model"] == "slim"
}

// parseTabList reads Player List Item into what the game does with it.
func parseTabList(r interface {
	VarInt() int32
	Bytes(int) []byte
	String(int) string
	Bool() bool
	Err() error
}) func(*game) {
	action := r.VarInt()
	n := int(r.VarInt())
	type change struct {
		e       tabEntry
		display bool
		hasDisp bool
	}
	var cs []change
	for i := 0; i < n && i < 1024 && r.Err() == nil; i++ {
		c := change{e: tabEntry{uuid: uuidString(r.Bytes(16))}}
		switch action {
		case 0:
			c.e.name = r.String(64)
			for np := int(r.VarInt()); np > 0 && r.Err() == nil; np-- {
				name, value := r.String(64), r.String(32767)
				if r.Bool() {
					r.String(32767)
				}
				if name == "textures" {
					c.e.skinURL, c.e.slim = textureProps(value)
				}
			}
			c.e.gamemode = int(r.VarInt())
			c.e.ping = int(r.VarInt())
			if r.Bool() {
				c.e.display = parseChat(r.String(32767))
				c.hasDisp = true
			}
		case 1:
			c.e.gamemode = int(r.VarInt())
		case 2:
			c.e.ping = int(r.VarInt())
		case 3:
			if r.Bool() {
				c.e.display = parseChat(r.String(32767))
				c.hasDisp = true
			}
		}
		cs = append(cs, c)
	}
	if r.Err() != nil {
		return nil
	}
	return func(g *game) {
		n := g.net
		if n.tab == nil {
			n.tab = map[string]*tabEntry{}
		}
		for _, c := range cs {
			e := c.e
			switch action {
			case 0:
				cp := e
				n.tab[e.uuid] = &cp
				n.names[e.uuid] = e.name
				for _, p := range n.remote {
					if p.uuid == e.uuid {
						p.name = e.name
					}
				}
				g.wantSkin(&cp)
			case 1:
				if t := n.tab[e.uuid]; t != nil {
					t.gamemode = e.gamemode
				}
			case 2:
				if t := n.tab[e.uuid]; t != nil {
					t.ping = e.ping
				}
			case 3:
				if t := n.tab[e.uuid]; t != nil {
					t.display = e.display
				}
			case 4:
				delete(n.tab, e.uuid)
				delete(n.names, e.uuid)
			}
		}
	}
}

// tabName is what the list calls a player: the server's name for them, else their name between their team's
// prefix and suffix (GuiPlayerTabOverlay.getPlayerName).
func (n *netState) tabName(e *tabEntry) []styledRun {
	if e.display != nil {
		return e.display
	}
	return legacyRuns(n.sb.format(e.name), defaultStyle)
}

// tabOrder is the list in the order it is shown: players who are not spectators first, then by team, then by name.
func (n *netState) tabOrder() []*tabEntry {
	out := make([]*tabEntry, 0, len(n.tab))
	for _, e := range n.tab {
		out = append(out, e)
	}
	teamOf := func(e *tabEntry) string {
		if n.sb != nil {
			if t := n.sb.team[e.name]; t != nil {
				return t.name
			}
		}
		return ""
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.gamemode != 3) != (b.gamemode != 3) {
			return a.gamemode != 3
		}
		if ta, tb := teamOf(a), teamOf(b); ta != tb {
			return ta < tb
		}
		return a.name < b.name
	})
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// tabLayout is how the list is arranged: how many columns, and how many rows in each (at most 20 rows).
func tabLayout(players int) (cols, rows int) {
	cols, rows = 1, players
	for rows > 20 {
		cols++
		rows = (players + cols - 1) / cols
	}
	return
}

// pingBars is which row of the ping icon a latency shows: 0 best to 4 worst, 5 for none.
func pingBars(ms int) int {
	switch {
	case ms < 0:
		return 5
	case ms < 150:
		return 0
	case ms < 300:
		return 1
	case ms < 600:
		return 2
	case ms < 1000:
		return 3
	}
	return 4
}

// listShown is whether Tab is held with a list to show.
func (g *game) listShown() bool {
	if g.listForced && g.net != nil && len(g.net.tab) > 0 {
		return true
	}
	return g.net != nil && g.screen == screenNone && g.win != nil && g.win.GetKey(glfw.KeyTab) == glfw.Press && len(g.net.tab) > 0
}

// drawTabList draws the player list at the top of the screen, as GuiPlayerTabOverlay.renderPlayerlist does.
func (g *game) drawTabList(w, h int) {
	if !g.listShown() {
		return
	}
	n := g.net
	g.enableUI()
	hd := &g.hud
	s := int(g.scaleFor(w, h))
	fs := float32(s)
	sw := (w + s - 1) / s
	rect := func(x1, y1, x2, y2 int, c [4]float32) {
		hd.rect(float32(x1)*fs, float32(y1)*fs, float32(x2-x1)*fs, float32(y2-y1)*fs, 0, 0, 1, 1, 1, 1, c)
		hd.flush(hd.white)
	}
	list := n.tabOrder()
	var listObj *scoreObjective
	if n.sb != nil {
		listObj = n.sb.objectives[n.sb.slots[0]]
	}
	nameW, scoreW := 0, 0
	names := make([][]styledRun, len(list))
	scores := make([][]styledRun, len(list))
	for i, e := range list {
		names[i] = n.tabName(e)
		nameW = max(nameW, g.runsWidth(names[i]))
		if listObj != nil && e.gamemode != 3 {
			scores[i] = legacyRuns("§e"+strconv.Itoa(int(n.sb.scores[listObj.name][e.name])), defaultStyle)
			scoreW = max(scoreW, g.runsWidth(scores[i])+5)
		}
	}
	cols, rows := tabLayout(len(list))
	heads := n.c != nil && n.c.encrypted // heads are drawn on servers that check accounts
	headW := 0
	if heads {
		headW = 9
	}
	colW := min(cols*(headW+nameW+scoreW+13), sw-50) / cols
	left := sw/2 - (colW*cols+(cols-1)*5)/2
	y := 10
	total := colW*cols + (cols-1)*5
	var head, foot [][]styledRun
	if n.header != nil {
		head = g.wrapRuns(n.header, sw-50)
	}
	if n.footer != nil {
		foot = g.wrapRuns(n.footer, sw-50)
	}
	for _, l := range head {
		total = max(total, g.runsWidth(l))
	}
	for _, l := range foot {
		total = max(total, g.runsWidth(l))
	}
	half := [4]float32{0, 0, 0, 0.5}
	if len(head) > 0 {
		rect(sw/2-total/2-1, y-1, sw/2+total/2+1, y+len(head)*9, half)
		for _, l := range head {
			g.drawRuns(l, float32(sw/2-g.runsWidth(l)/2)*fs, float32(y)*fs, fs, 1, true)
			y += 9
		}
		y++
	}
	rect(sw/2-total/2-1, y-1, sw/2+total/2+1, y+rows*9, half)
	for i := range list {
		c, rw := i/rows, i%rows
		x := left + c*colW + c*5
		yy := y + rw*9
		rect(x, yy, x+colW, yy+8, [4]float32{1, 1, 1, 0.125})
		tx := x
		if heads {
			g.drawHead(list[i], float32(x)*fs, float32(yy)*fs, fs)
			tx += 9
		}
		if list[i].gamemode == 3 { // a spectator's name is italic and dim
			dim := make([]styledRun, len(names[i]))
			copy(dim, names[i])
			for k := range dim {
				dim[k].st.italic, dim[k].st.col = true, [3]float32{1, 1, 1}
			}
			g.drawRuns(dim, float32(tx)*fs, float32(yy)*fs, fs, 0.55, true)
		} else {
			g.drawRuns(names[i], float32(tx)*fs, float32(yy)*fs, fs, 1, true)
		}
		if scores[i] != nil {
			g.drawRuns(scores[i], float32(x+colW-12-g.runsWidth(scores[i]))*fs, float32(yy)*fs, fs, 1, true)
		}
		hd.rect(float32(x+colW-11)*fs, float32(yy)*fs, 10*fs, 8*fs, 0, float32(176+pingBars(list[i].ping)*8), 10, 8, 256, 256, [4]float32{1, 1, 1, 1})
		hd.flush(hd.icons)
	}
	y += rows * 9
	if len(foot) > 0 {
		y++
		rect(sw/2-total/2-1, y-1, sw/2+total/2+1, y+len(foot)*9, half)
		for _, l := range foot {
			g.drawRuns(l, float32(sw/2-g.runsWidth(l)/2)*fs, float32(y)*fs, fs, 1, true)
			y += 9
		}
	}
}

var _ = strings.TrimSpace
