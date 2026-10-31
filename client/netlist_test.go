package client

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"gocraft/proto"
)

func tabAdd(name string, uuid byte, gm, ping int, texturesB64, display string) *proto.Writer {
	w := newPacket(0x38)
	w.VarInt(0)
	w.VarInt(1)
	id := make([]byte, 16)
	id[15] = uuid
	w.Bytes(id)
	w.String(name)
	if texturesB64 != "" {
		w.VarInt(1)
		w.String("textures")
		w.String(texturesB64)
		w.Bool(false)
	} else {
		w.VarInt(0)
	}
	w.VarInt(int32(gm))
	w.VarInt(int32(ping))
	w.Bool(display != "")
	if display != "" {
		w.String(display)
	}
	return w
}

func texturesProp(url string, slim bool) string {
	meta := ""
	if slim {
		meta = `,"metadata":{"model":"slim"}`
	}
	return base64.StdEncoding.EncodeToString([]byte(`{"timestamp":1,"textures":{"SKIN":{"url":"` + url + `"` + meta + `}}}`))
}

func TestPlayerListParsing(t *testing.T) {
	g, _ := windowGame(t)
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(tabAdd("Herobrine", 1, 0, 42, texturesProp("http://textures.minecraft.net/texture/abc123", true), `{"text":"§c[ADMIN] Herobrine"}`))
	apply(tabAdd("Steve", 2, 1, 310, "", ""))
	e := g.net.tab[uuidOf(1)]
	if e == nil || e.name != "Herobrine" || e.ping != 42 || e.gamemode != 0 || !e.slim || e.skinURL != "http://textures.minecraft.net/texture/abc123" {
		t.Fatalf("entry %+v", e)
	}
	if got := plainRuns(g.net.tabName(e)); got != "[ADMIN] Herobrine" {
		t.Fatalf("list name %q", got)
	}
	if got := plainRuns(g.net.tabName(g.net.tab[uuidOf(2)])); got != "Steve" {
		t.Fatalf("a player with no display name is %q", got)
	}
	if g.net.names[uuidOf(1)] != "Herobrine" {
		t.Fatal("the name map was not kept for the players that spawn")
	}
	// update the ping, the game mode and the display name; then remove
	up := func(action int32, build func(*proto.Writer)) {
		w := newPacket(0x38)
		w.VarInt(action)
		w.VarInt(1)
		id := make([]byte, 16)
		id[15] = 2
		w.Bytes(id)
		build(w)
		apply(w)
	}
	up(2, func(w *proto.Writer) { w.VarInt(999) })
	up(1, func(w *proto.Writer) { w.VarInt(3) })
	up(3, func(w *proto.Writer) { w.Bool(true); w.String(`{"text":"Esteban","color":"green"}`) })
	if s := g.net.tab[uuidOf(2)]; s.ping != 999 || s.gamemode != 3 || plainRuns(s.display) != "Esteban" {
		t.Fatalf("after the updates %+v", s)
	}
	up(4, func(w *proto.Writer) {})
	if g.net.tab[uuidOf(2)] != nil || g.net.names[uuidOf(2)] != "" {
		t.Fatal("a removed player stayed in the list")
	}
	// header and footer
	hf := newPacket(0x47)
	hf.String(`{"text":"§6UNIVERSOCRAFT"}`)
	hf.String(`{"text":"play.universocraft.com"}`)
	apply(hf)
	if plainRuns(g.net.header) != "UNIVERSOCRAFT" || plainRuns(g.net.footer) != "play.universocraft.com" {
		t.Fatalf("header %q footer %q", plainRuns(g.net.header), plainRuns(g.net.footer))
	}
}

func uuidOf(b byte) string {
	id := make([]byte, 16)
	id[15] = b
	return uuidString(id)
}

// The list: players who are not spectators first, then by team, then by name; at most twenty rows a column,
// at most eighty players; and the ping bars the game gives each latency.
func TestPlayerListOrderAndLayout(t *testing.T) {
	g, _ := windowGame(t)
	n := g.net
	n.tab = map[string]*tabEntry{
		"a": {uuid: "a", name: "Zed", gamemode: 0},
		"b": {uuid: "b", name: "Amy", gamemode: 3},
		"c": {uuid: "c", name: "Bob", gamemode: 0},
		"d": {uuid: "d", name: "Cat", gamemode: 1},
	}
	apply := func(w *proto.Writer) { decodeFrame(t, w, nil)(g) }
	apply(teamPacket("zteam", 0, "", "", -1, "Bob"))
	apply(teamPacket("ateam", 0, "", "", -1, "Cat"))
	var names []string
	for _, e := range n.tabOrder() {
		names = append(names, e.name)
	}
	// non-spectators (no team first: Zed; then ateam: Cat; zteam: Bob), the spectator last
	want := []string{"Zed", "Cat", "Bob", "Amy"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order %v, want %v", names, want)
		}
	}
	for _, c := range []struct{ players, cols, rows int }{{1, 1, 1}, {20, 1, 20}, {21, 2, 11}, {40, 2, 20}, {41, 3, 14}, {80, 4, 20}} {
		if cols, rows := tabLayout(c.players); cols != c.cols || rows != c.rows {
			t.Errorf("%d players: %d x %d, want %d x %d", c.players, cols, rows, c.cols, c.rows)
		}
	}
	for ms, bars := range map[int]int{-1: 5, 0: 0, 149: 0, 150: 1, 299: 1, 300: 2, 599: 2, 600: 3, 999: 3, 1000: 4, 5000: 4} {
		if pingBars(ms) != bars {
			t.Errorf("ping %d shows row %d, want %d", ms, pingBars(ms), bars)
		}
	}
	for i := 0; i < 100; i++ {
		id := string(rune('A'+i/26)) + string(rune('a'+i%26))
		n.tab[id] = &tabEntry{uuid: id, name: id}
	}
	if len(n.tabOrder()) != 80 {
		t.Fatalf("%d players listed, at most 80 are", len(n.tabOrder()))
	}
}

func TestSkinsAreOnlyFetchedFromMojang(t *testing.T) {
	for u, ok := range map[string]bool{
		"http://textures.minecraft.net/texture/abc": true,
		"https://textures.minecraft.net/texture/a":  true,
		"https://skins.mojang.com/x":                true,
		"http://evil.example.com/texture/abc":       false,
		"http://textures.minecraft.net.evil.com/x":  false,
		"file:///etc/passwd":                        false,
		"http://127.0.0.1/skin":                     false,
	} {
		if skinAllowed(u) != ok {
			t.Errorf("%s: allowed %v, want %v", u, skinAllowed(u), ok)
		}
	}
	if u, slim := textureProps(texturesProp("http://textures.minecraft.net/texture/abc", true)); u == "" || !slim {
		t.Fatalf("slim skin read as %q %v", u, slim)
	}
	if u, slim := textureProps(texturesProp("http://textures.minecraft.net/texture/abc", false)); u == "" || slim {
		t.Fatalf("classic skin read as %q %v", u, slim)
	}
	if u, _ := textureProps("not base64 at all!"); u != "" {
		t.Fatal("garbage was taken for a skin")
	}
}

// An old 64 x 32 skin is made 64 x 64: the left leg and arm are the right ones turned about, as the game
// does it, and the body is opaque whatever the picture says.
func TestOldSkinIsUpgraded(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 32))
	red := color.RGBA{255, 0, 0, 255}
	src.SetRGBA(4, 20, red) // the right leg's front face, its first column
	src.SetRGBA(44, 20, color.RGBA{0, 255, 0, 255})
	up := upgradeSkin(src)
	if up.Bounds().Dy() != 64 {
		t.Fatalf("height %d", up.Bounds().Dy())
	}
	if up.RGBAAt(23, 52) != red {
		t.Fatalf("the left leg's front, last column, is %v: the right leg's first column turned about", up.RGBAAt(23, 52))
	}
	if up.RGBAAt(20, 52) == red {
		t.Fatal("the leg was copied without turning it about")
	}
	if got := up.RGBAAt(10, 20); got.A != 255 {
		t.Fatalf("the body is not opaque: %v", got)
	}
	// a 64 x 64 skin is kept as it is, apart from the opaque areas
	full := image.NewRGBA(image.Rect(0, 0, 64, 64))
	full.SetRGBA(40, 40, color.RGBA{1, 2, 3, 128})
	if got := upgradeSkin(full).RGBAAt(40, 40); got != (color.RGBA{1, 2, 3, 128}) {
		t.Fatalf("a new skin's overlay changed: %v", got)
	}
}

// Steve or Alex by uuid, as UUID.hashCode picks them.
func TestDefaultSkinByUUID(t *testing.T) {
	if javaHash("00000000000000000000000000000001") != 1 || !defaultSlim("00000000000000000000000000000001") {
		t.Fatal("uuid 1 should hash to 1 and wear Alex")
	}
	if defaultSlim("00000000000000000000000000000002") {
		t.Fatal("uuid 2 should wear Steve")
	}
	// the high and low halves are mixed in: 1 << 32 hashes to 1 too
	if javaHash("00000001000000000000000000000000") != 1 {
		t.Fatalf("hash %d", javaHash("00000001000000000000000000000000"))
	}
}

// A skin the player list names is fetched once, in the background, and arrives as a 64 x 64 picture.
func TestSkinIsFetchedOnceInTheBackground(t *testing.T) {
	g, _ := windowGame(t)
	g.skinDone = make(chan skinResult, 4)
	old := skinFetcher
	defer func() { skinFetcher = old }()
	calls := 0
	skinFetcher = func(u string) ([]byte, error) {
		calls++
		img := image.NewRGBA(image.Rect(0, 0, 64, 32))
		var b bytes.Buffer
		png.Encode(&b, img)
		return b.Bytes(), nil
	}
	e := &tabEntry{uuid: uuidOf(5), skinURL: "http://textures.minecraft.net/texture/zzz"}
	g.wantSkin(e)
	g.wantSkin(e) // asked for again: not fetched again
	select {
	case r := <-g.skinDone:
		if r.img == nil || r.img.Bounds().Dy() != 64 {
			t.Fatalf("result %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the skin never arrived")
	}
	if calls != 1 {
		t.Fatalf("fetched %d times", calls)
	}
	g.wantSkin(&tabEntry{skinURL: "http://evil.example.com/x"})
	select {
	case r := <-g.skinDone:
		t.Fatalf("a skin from outside Mojang was fetched: %+v", r)
	case <-time.After(50 * time.Millisecond):
	}
}
