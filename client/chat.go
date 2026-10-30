package client

import (
	"regexp"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Chat and the multiplayer screen: the two places the player types.

type chatLine struct {
	text string      // the line without its colours
	runs []styledRun // and with them
	at   time.Time
}

const (
	chatKeep    = 10 * time.Second // how long a line stays on screen with the chat closed
	chatVisible = 10               // lines shown with it closed
	chatHistory = 100
	chatMax     = 100 // the longest line the protocol takes
)

var validName = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)

// chatWidth is the width of the chat in GUI pixels, as in the game: longer lines wrap.
const chatWidth = 320

// addChat adds a plain line; § codes in it still colour it.
func (g *game) addChat(text string) { g.addChatRuns(legacyRuns(text, defaultStyle)) }

// addChatRuns adds styled text, wrapped to the width of the chat.
func (g *game) addChatRuns(rs []styledRun) {
	now := time.Now()
	if len(rs) == 0 {
		rs = []styledRun{{" ", defaultStyle}}
	}
	for _, l := range g.wrapRuns(rs, chatWidth) {
		g.chat = append(g.chat, chatLine{plainRuns(l), l, now})
	}
	if n := len(g.chat); n > chatHistory {
		g.chat = g.chat[n-chatHistory:]
	}
}

// openChat starts typing a chat line; "/" begins one with the slash in place.
func (g *game) openChat(prefill string) {
	g.chatInput = prefill
	g.openScreen(screenChat)
}

// onChar takes the text the player types into the chat line or the focused field of the multiplayer screen.
func (g *game) onChar(_ *glfw.Window, r rune) {
	if r < 32 || r > 255 || r == 127 {
		return
	}
	if g.chatSkip {
		g.chatSkip = false
		return
	}
	switch g.screen {
	case screenChat:
		if len(g.chatInput) < chatMax {
			g.chatInput += string(r)
		}
	case screenMulti:
		f := &g.multiAddr
		limit := 64
		if g.multiFocus == 1 {
			f, limit = &g.multiName, 16
		}
		if len(*f) < limit {
			*f += string(r)
		}
	}
}

// typing is whether a text entry has the keyboard.
func (g *game) typing() bool { return g.screen == screenChat || g.screen == screenMulti }

// typingKey handles the keys of a text entry; it reports whether the key was one of its own.
func (g *game) typingKey(key glfw.Key) bool {
	switch g.screen {
	case screenChat:
		switch key {
		case glfw.KeyEnter, glfw.KeyKPEnter:
			g.sendChat(g.chatInput)
			g.closeScreen()
		case glfw.KeyBackspace:
			if rs := []rune(g.chatInput); len(rs) > 0 {
				g.chatInput = string(rs[:len(rs)-1])
			}
		case glfw.KeyEscape:
			g.closeScreen()
		}
		return true
	case screenMulti:
		f := &g.multiAddr
		if g.multiFocus == 1 {
			f = &g.multiName
		}
		switch key {
		case glfw.KeyBackspace:
			if rs := []rune(*f); len(rs) > 0 {
				*f = string(rs[:len(rs)-1])
			}
		case glfw.KeyTab:
			g.multiFocus = 1 - g.multiFocus
		case glfw.KeyEnter, glfw.KeyKPEnter:
			g.joinFromScreen()
		case glfw.KeyEscape:
			g.screen = screenTitle
		}
		return true
	}
	return false
}

// joinFromScreen connects to what the multiplayer screen holds and remembers it.
func (g *game) joinFromScreen() {
	addr, name := strings.TrimSpace(g.multiAddr), strings.TrimSpace(g.multiName)
	_, _, signedIn, _, haveClient, use := g.acctView()
	withAccount := signedIn && haveClient && use // the account's own name is used, not the field
	switch {
	case addr == "":
		g.netMsg = "Enter a server address"
		return
	case !withAccount && !validName.MatchString(name):
		g.netMsg = "The name is 1 to 16 letters, digits or _"
		return
	}
	g.opts.Server = addr
	if validName.MatchString(name) {
		g.opts.PlayerName = name
	}
	if g.opt.OptionsFile != "/dev/null" {
		_ = g.opts.save()
	}
	g.netMsg = "Connecting..."
	if err := g.connect(addr, name); err != nil {
		g.netMsg = err.Error()
	}
}

// drawChat draws the chat lines over the game, and the entry line when it is open.
func (g *game) drawChat(w, h int) {
	open := g.screen == screenChat
	if len(g.chat) == 0 && !open {
		return
	}
	g.enableUI()
	s := float32(g.scaleFor(w, h))
	hd := &g.hud
	x, base := 2*s, float32(h)-48*s
	lines := g.chat
	n := chatVisible
	if open {
		n = 20
	}
	now := time.Now()
	shown := 0
	for i := len(lines) - 1; i >= 0 && shown < n; i-- {
		l := lines[i]
		age := now.Sub(l.at)
		if !open && age > chatKeep {
			break
		}
		a := float32(1)
		if !open && age > chatKeep-time.Second {
			a = float32(chatKeep-age) / float32(time.Second)
		}
		y := base - float32(shown+1)*9*s
		hd.rect(x-s, y-s, float32(g.runsWidth(l.runs)+2)*s, 9*s, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 0.4 * a})
		hd.flush(hd.white)
		g.drawRuns(l.runs, x, y, s, a, true)
		shown++
	}
	if open {
		y := float32(h) - 14*s
		hd.rect(x-s, y-s, float32(w)-2*x+2*s, 12*s, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 0.5})
		hd.flush(hd.white)
		cursor := ""
		if time.Now().UnixMilli()/500%2 == 0 {
			cursor = "_"
		}
		g.text(g.chatInput+cursor, x, y, s, [4]float32{1, 1, 1, 1}, true)
	}
}

// textField draws one entry box of the multiplayer screen.
func (g *game) textField(label, value string, x, y, w float32, focused bool, idx int) {
	hd := &g.hud
	s := float32(g.scaleFor(g.fb[0], g.fb[1]))
	px, py := x*s, y*s
	mx, my := g.mouse()
	if g.uiClick && mx >= px && mx < px+w*s && my >= py && my < py+20*s {
		g.multiFocus = idx
		focused = true
	}
	g.text(label, px, py-11*s, s, [4]float32{0.63, 0.63, 0.63, 1}, true)
	edge := [4]float32{0.63, 0.63, 0.63, 1}
	if focused {
		edge = [4]float32{1, 1, 1, 1}
	}
	hd.rect(px-s, py-s, w*s+2*s, 22*s, 0, 0, 1, 1, 1, 1, edge)
	hd.rect(px, py, w*s, 20*s, 0, 0, 1, 1, 1, 1, [4]float32{0, 0, 0, 1})
	hd.flush(hd.white)
	cursor := ""
	if focused && time.Now().UnixMilli()/500%2 == 0 {
		cursor = "_"
	}
	g.text(value+cursor, px+4*s, py+6*s, s, [4]float32{0.88, 0.88, 0.88, 1}, true)
}

// drawMulti is the multiplayer screen: a server address, a player name, Join.
func (g *game) drawMulti(cx, s float32) {
	left := cx/s - 100
	g.dirtBackground(0.45)
	g.centredText("Play Multiplayer", cx*1, 20*s, s, [4]float32{1, 1, 1, 1})
	top := float32(g.fb[1])/s/4 + 8
	g.textField("Server address", g.multiAddr, left, top, 200, g.multiFocus == 0, 0)
	g.textField("Player name", g.multiName, left, top+36, 200, g.multiFocus == 1, 1)
	if g.netMsg != "" {
		g.centredText(trimName(g.netMsg, 52), cx, (top+58)*s, s, [4]float32{1, 0.4, 0.4, 1})
	}
	if g.button("Join Server", left, top+70, 200, true) {
		g.joinFromScreen()
	}
	if g.button("Cancel", left, top+92, 200, true) {
		g.screen = screenTitle
	}
	g.drawAccount(cx, s, left, top+118)
}

// drawAccount is the Microsoft account block of the multiplayer screen: who is signed in, the code to
// enter while a sign-in waits, and the buttons.
func (g *game) drawAccount(cx, s, left, top float32) {
	status, code, signedIn, busy, haveClient, use := g.acctView()
	grey := [4]float32{0.63, 0.63, 0.63, 1}
	g.centredText("Microsoft account: "+trimName(status, 40), cx, top*s, s, grey)
	if code != nil {
		g.centredText(code.VerificationURI+"  code "+code.UserCode, cx, (top+11)*s, s, [4]float32{1, 1, 0.4, 1})
	}
	y := top + 20
	switch {
	case signedIn:
		label := "Account: OFF"
		if use {
			label = "Account: ON"
		}
		if g.button(label, left, y, 98, true) { // ON plays with the account's name and proves it to the server
			g.acct.mu.Lock()
			g.acct.use = !g.acct.use
			g.acct.mu.Unlock()
		}
		if g.button("Sign out", left+102, y, 98, true) {
			g.acctSignOut()
		}
	default:
		if g.button("Sign in with Microsoft", left, y, 200, haveClient && !busy) {
			g.acctSignIn()
		}
	}
}
