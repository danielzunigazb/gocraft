package client

import (
	"bufio"
	"bytes"
	"strings"
	"sync/atomic"
	"testing"

	"gocraft/proto"
	"gocraft/world"
)

// decode frames w and reads it back as the connection's reader goroutine would.
func decodeFrame(t *testing.T, w *proto.Writer, dim *atomic.Int32) func(*game) {
	t.Helper()
	id, r, err := proto.ReadPacket(bufio.NewReader(bytes.NewReader(w.Frame())))
	if err != nil {
		t.Fatal(err)
	}
	return parsePacketIn(id, r, dim)
}

func joinGame(eid int32, mode byte, dim int8) *proto.Writer {
	w := newPacket(0x01)
	w.Int(eid)
	w.Byte(mode)
	w.Byte(byte(dim))
	w.Byte(2)
	w.Byte(20)
	w.String("flat")
	w.Bool(false)
	return w
}

func respawn(dim int32, mode byte) *proto.Writer {
	w := newPacket(0x07)
	w.Int(dim)
	w.Byte(2)
	w.Byte(mode)
	w.String("default")
	return w
}

// A proxy moves the player to another server with Respawn: the old columns and players go.
func TestRespawnForgetsTheWorld(t *testing.T) {
	g := testGame(t)
	g.net = &netState{c: &netClient{}, received: map[world.ChunkPos]bool{{X: 1, Z: 2}: true}, remote: map[int32]*remotePlayer{7: {eid: 7}}, names: map[string]string{}}
	g.world.Put(world.ChunkPos{X: 1, Z: 2}, &world.Chunk{Light: make([]uint8, 65536)})
	g.creative = true
	var dim atomic.Int32
	decodeFrame(t, respawn(-1, 0), &dim)(g)
	if dim.Load() != -1 || g.dimn.id != dimNether {
		t.Fatalf("dimension %d / %d", dim.Load(), g.dimn.id)
	}
	if len(g.net.received) != 0 || len(g.net.remote) != 0 || g.world.Loaded() != 0 {
		t.Fatal("the old world survived the respawn")
	}
	if g.creative {
		t.Fatal("survival respawn left the player in creative")
	}
	decodeFrame(t, respawn(0, 1), &dim)(g)
	if g.dimn.id != dimOverworld || !g.creative {
		t.Fatalf("back in the Overworld: dim %d creative %v", g.dimn.id, g.creative)
	}
}

// Join Game carries the dimension; Change Game State 3 changes the game mode.
func TestJoinGameDimensionAndGameState(t *testing.T) {
	g := testGame(t)
	g.net = &netState{c: &netClient{}, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	var dim atomic.Int32
	decodeFrame(t, joinGame(5, 0x00|0x08, 1), &dim) // hardcore bit set, survival
	if dim.Load() != 1 {
		t.Fatalf("dimension %d", dim.Load())
	}
	w := newPacket(0x2B)
	w.Byte(3)
	w.Float(1)
	decodeFrame(t, w, &dim)(g)
	if !g.creative {
		t.Fatal("game state 3 = 1 should be creative")
	}
	w = newPacket(0x2B)
	w.Byte(3)
	w.Float(0)
	decodeFrame(t, w, &dim)(g)
	if g.creative {
		t.Fatal("game state 3 = 0 should be survival")
	}
}

// Outside the Overworld, Chunk Data carries no sky light, so a column is shorter.
func TestChunkWithoutSkyLight(t *testing.T) {
	data := make([]byte, 8192+2048+256) // one section: blocks, block light, biomes
	w := newPacket(0x21)
	w.Int(3)
	w.Int(4)
	w.Bool(true)
	w.UShort(1)
	w.VarInt(int32(len(data)))
	w.Bytes(data)
	var dim atomic.Int32
	dim.Store(-1)
	if decodeFrame(t, w, &dim) == nil {
		t.Fatal("a Nether column was not understood")
	}
	dim.Store(0)
	if decodeFrame(t, w, &dim) != nil {
		t.Fatal("an Overworld column without sky light should be rejected")
	}
}

// Chat keeps the colours and formats the network sends, in JSON or as § codes, and wraps long lines.
func TestChatKeepsColours(t *testing.T) {
	js := `{"text":"","extra":[{"text":"Cuenta » ","color":"gold","bold":true},{"text":"hola §ac§lverde","color":"red"},{"text":" fin","underlined":true}]}`
	rs := parseChat(js)
	if got := plainRuns(rs); got != "Cuenta » hola cverde fin" {
		t.Fatalf("plain %q", got)
	}
	gold, red, green := chatColours[6], chatColours[12], chatColours[10]
	if rs[0].st.col != gold || !rs[0].st.bold {
		t.Fatalf("first run %+v", rs[0])
	}
	if rs[1].s != "hola " || rs[1].st.col != red || rs[1].st.bold {
		t.Fatalf("second run %+v", rs[1])
	}
	if rs[2].s != "c" || rs[2].st.col != green || rs[2].st.bold { // §a clears the formats
		t.Fatalf("third run %+v", rs[2])
	}
	if rs[3].s != "verde" || !rs[3].st.bold || rs[3].st.col != green {
		t.Fatalf("fourth run %+v", rs[3])
	}
	if last := rs[len(rs)-1]; last.s != " fin" || !last.st.under || last.st.col != defaultStyle.col {
		t.Fatalf("last run %+v", last)
	}
}

func TestChatPacketWrapsAndColours(t *testing.T) {
	g := testGame(t)
	for i := range g.glyphs {
		g.glyphs[i] = 5
	}
	g.net = &netState{c: &netClient{}, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	long := strings.Repeat("palabra ", 30)
	w := newPacket(0x02)
	w.String(`[{"text":"` + long + `","color":"green"}]`)
	w.Byte(0)
	decodeFrame(t, w, nil)(g)
	if len(g.chat) < 3 {
		t.Fatalf("a 240 character line made %d chat lines, want it wrapped", len(g.chat))
	}
	for _, l := range g.chat {
		if g.runsWidth(l.runs) > chatWidth {
			t.Fatalf("line %q is %d wide", l.text, g.runsWidth(l.runs))
		}
		if l.runs[0].st.col != chatColours[10] {
			t.Fatalf("a wrapped line lost its colour: %+v", l.runs[0])
		}
	}
}

func TestGlyphsForAccents(t *testing.T) {
	for _, c := range "áéíóúñÑ¿¡»«üÜ" {
		if id := glyphOf(c); id == '?' {
			t.Errorf("%q has no glyph", c)
		}
	}
	if glyphOf('A') != 'A' || glyphOf('€') != '?' {
		t.Fatal("ASCII is its own cell and what the font lacks is a question mark")
	}
}

func spawnStand(eid int32, x, y, z float64, name string, flags byte) *proto.Writer {
	w := newPacket(0x0F)
	w.VarInt(eid)
	w.Byte(30) // armor stand
	w.Int(int32(x * 32))
	w.Int(int32(y * 32))
	w.Int(int32(z * 32))
	w.Byte(0)
	w.Byte(0)
	w.Byte(0)
	w.UShort(0)
	w.UShort(0)
	w.UShort(0)
	w.Byte(4<<5 | 2) // custom name
	w.String(name)
	w.Byte(0<<5 | 3) // name visible
	w.Byte(1)
	w.Byte(0<<5 | 10) // armor stand flags
	w.Byte(flags)
	w.Byte(0x7F)
	return w
}

// A lobby's holograms are armor stands with a custom name: they are tracked, move, and are renamed.
func TestHologramArmorStands(t *testing.T) {
	g := testGame(t)
	g.net = &netState{c: &netClient{}, received: map[world.ChunkPos]bool{}, remote: map[int32]*remotePlayer{}, names: map[string]string{}}
	decodeFrame(t, spawnStand(9, 10, 64, -3, "§6§lTOP §fKILLS", 16), nil)(g)
	e := g.net.ents[9]
	if e == nil || !e.showName || !e.marker || e.height() != 0 || plainRuns(e.name) != "TOP KILLS" {
		t.Fatalf("entity %+v", e)
	}
	if e.name[0].st.col != chatColours[6] || !e.name[0].st.bold || e.name[1].st.col != chatColours[15] || e.name[1].st.bold {
		t.Fatalf("name runs %+v", e.name)
	}
	if e.pos != (vec3{10, 64, -3}) {
		t.Fatalf("at %v", e.pos)
	}
	mv := newPacket(0x15)
	mv.VarInt(9)
	mv.Byte(32)
	mv.Byte(byte(0xE0)) // -1
	mv.Byte(0)
	mv.Bool(true)
	decodeFrame(t, mv, nil)(g)
	if e.target != (vec3{11, 63, -3}) {
		t.Fatalf("after a relative move heading for %v", e.target)
	}
	md := newPacket(0x1C)
	md.VarInt(9)
	md.Byte(4<<5 | 2)
	md.String("§aNuevo")
	md.Byte(0x7F)
	decodeFrame(t, md, nil)(g)
	if plainRuns(e.name) != "Nuevo" || !e.showName {
		t.Fatalf("renamed to %+v", e)
	}
	de := newPacket(0x13)
	de.VarInt(1)
	de.VarInt(9)
	decodeFrame(t, de, nil)(g)
	if g.net.ents[9] != nil {
		t.Fatal("destroyed entity is still tracked")
	}
	decodeFrame(t, spawnStand(10, 0, 0, 0, "x", 0), nil)(g)
	g.netRespawn(0, 0)
	if len(g.net.ents) != 0 {
		t.Fatal("a respawn keeps the old server's entities")
	}
}

// A point straight ahead lands in the middle of the screen.
func TestProjectPoint(t *testing.T) {
	proj := perspective(70*3.14159265/180, 16.0/9, 0.05, 1000)
	view := lookAt(vec3{0, 0, 0}, vec3{0, 0, 1}, vec3{0, 1, 0})
	mvp := mul(proj, view)
	x, y, d, ok := projectPoint(&mvp, vec3{0, 0, 10}, 1600, 900)
	if !ok || !near(x, 800, 1) || !near(y, 450, 1) || !near(d, 10, 0.01) {
		t.Fatalf("got %v %v depth %v ok %v", x, y, d, ok)
	}
	if _, _, _, ok := projectPoint(&mvp, vec3{0, 0, -10}, 1600, 900); ok {
		t.Fatal("a point behind the camera was projected")
	}
}
