package client

import (
	"strings"

	"gocraft/proto"
)

// Tab completion in the chat line, after 1.8.9's GuiChat: Tab asks the server for what could follow the text
// before the cursor (Tab-Complete, 0x14), and the answer (0x3A) fills the word in: with what all the answers
// start with, if that is more than the word already is, else with the answers one after the other, each Tab the
// next, after printing the list in the chat.

type chatTab struct {
	waiting bool     // a request is out and its answer will be used
	found   bool     // the answers are being walked
	words   []string // the answers
	index   int      // the one Tab writes next
}

// lastWordStart is where the word under the cursor (the end of the line) begins.
func lastWordStart(s string) int { return strings.LastIndexByte(s, ' ') + 1 }

// chatTabKey is Tab pressed in the chat line.
func (g *game) chatTabKey() {
	t := &g.tab
	if g.net == nil {
		return
	}
	if t.found { // the answers are in: the word written last is replaced by the next
		g.chatInput = g.chatInput[:lastWordStart(g.chatInput)]
		if t.index >= len(t.words) {
			t.index = 0
		}
		g.completeWith()
		return
	}
	if g.chatInput == "" {
		return
	}
	t.words, t.index, t.waiting = nil, 0, true
	p := newPacket(0x14)
	p.String(g.chatInput)
	if h, ok := g.target(); ok {
		p.Bool(true)
		p.Long(proto.PackPos(h.pos[0], h.pos[1], h.pos[2]))
	} else {
		p.Bool(false)
	}
	g.net.c.send(p)
}

// completeWith prints the answers when there are several and writes the next one.
func (g *game) completeWith() {
	t := &g.tab
	if len(t.words) > 1 {
		g.addChat(strings.Join(t.words, ", "))
	}
	g.chatInput += t.words[t.index]
	t.index++
}

// netTabComplete is Tab-Complete's answer (GuiChat.onAutocompleteResponse).
func (g *game) netTabComplete(answers []string) {
	t := &g.tab
	if !t.waiting || g.screen != screenChat {
		return
	}
	t.found, t.words = false, nil
	for _, a := range answers {
		if a != "" {
			t.words = append(t.words, a)
		}
	}
	word := g.chatInput[lastWordStart(g.chatInput):]
	if common := commonPrefix(answers); common != "" && !strings.EqualFold(word, common) {
		g.chatInput = g.chatInput[:lastWordStart(g.chatInput)] + common
	} else if len(t.words) > 0 {
		t.found = true
		g.chatInput = g.chatInput[:lastWordStart(g.chatInput)]
		t.index = 0
		g.completeWith()
	}
}

// commonPrefix is what every string starts with (StringUtils.getCommonPrefix).
func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	p := ss[0]
	for _, s := range ss[1:] {
		i := 0
		for i < len(p) && i < len(s) && p[i] == s[i] {
			i++
		}
		p = p[:i]
	}
	return p
}
