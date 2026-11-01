package client

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gocraft/world"
)

// testPackZip is a resource pack that paints the stone block red, and its SHA-1.
func testPackZip(t *testing.T) ([]byte, string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{200, 10, 10, 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	for name, data := range map[string][]byte{
		"pack.mcmeta": []byte(`{"pack":{"pack_format":1,"description":"test"}}`),
		"assets/minecraft/textures/blocks/stone.png": pngBytes.Bytes(),
	} {
		w, _ := zw.Create(name)
		w.Write(data)
	}
	zw.Close()
	sum := sha1.Sum(zb.Bytes())
	return zb.Bytes(), hex.EncodeToString(sum[:])
}

// packServer serves the pack and counts the requests, keeping the headers of the last.
func packServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int32, *http.Header) {
	t.Helper()
	var n atomic.Int32
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		hdr = r.Header.Clone()
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &n, &hdr
}

// redPixels counts the atlas pixels with the colour of the test pack's stone. (The atlas is not laid out the same
// way twice, so it is compared by what is in it.)
func redPixels(g *game) int {
	n := 0
	px := g.res.atlas.img.Pix
	for i := 0; i+3 < len(px); i += 4 {
		if px[i] == 200 && px[i+1] == 10 && px[i+2] == 10 && px[i+3] == 255 {
			n++
		}
	}
	return n
}

// packGame is a game on a server, with the jar to load resources again and an inbox for what the downloads post.
func packGame(t *testing.T) (*game, *netClient) {
	g, c := windowGame(t)
	c.inbox = make(chan func(*game), 16)
	c.done = make(chan struct{})
	c.name = "Steve"
	g.net.addr = "play.example.net"
	g.opt.JarPath = filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "client.jar")
	g.opt.PacksDir = filepath.Join(t.TempDir(), "resourcepacks")
	return g, c
}

func offerPack(t *testing.T, g *game, url, hash string) {
	t.Helper()
	w := newPacket(0x48)
	w.String(url)
	w.String(hash)
	decodeFrame(t, w, nil)(g)
}

// statuses are the Resource Pack Status answers the client sent, as the actions they carry.
func statuses(t *testing.T, c *netClient) []int32 {
	t.Helper()
	var out []int32
	for _, p := range drain(t, c) {
		if p.id == 0x19 {
			p.r.String(40)
			out = append(out, p.r.VarInt())
		}
	}
	return out
}

func awaitPost(t *testing.T, c *netClient, g *game) {
	t.Helper()
	select {
	case f := <-c.inbox:
		f(g)
	case <-time.After(10 * time.Second):
		t.Fatal("the download never finished")
	}
}

func same(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The first offer asks the player, and says nothing to the server until they answer. Yes accepts, downloads
// and loads the pack over the game's own pictures; the answer is kept, so the next offer is taken without asking.
func TestServerPackIsAskedThenDownloadedAndLaidOver(t *testing.T) {
	body, hash := testPackZip(t)
	srv, hits, hdr := packServer(t, body)
	g, c := packGame(t)
	if redPixels(g) != 0 {
		t.Fatal("the plain game has the pack's colour")
	}

	url := srv.URL + "/a/long/path/to/the/resource/pack/that/is/over/forty/characters.zip" // the old parser read 40 bytes of it
	offerPack(t, g, url, hash)
	if g.screen != screenServerPack || g.net.packAsk == nil {
		t.Fatalf("screen %v, ask %v: the offer should ask", g.screen, g.net.packAsk)
	}
	if st := statuses(t, c); len(st) != 0 {
		t.Fatalf("answered %v before the player did", st)
	}
	g.answerServerPack(true, true)
	if g.screen == screenServerPack {
		t.Fatal("the question is still on screen")
	}
	if st := statuses(t, c); !same(st, []int32{packAccepted}) {
		t.Fatalf("after Yes: %v, want accepted", st)
	}
	awaitPost(t, c, g)
	if st := statuses(t, c); !same(st, []int32{packLoaded}) {
		t.Fatalf("after the download: %v, want loaded", st)
	}
	if redPixels(g) < 256 {
		t.Fatalf("the pack's stone is not in the atlas (%d red pixels)", redPixels(g))
	}
	if got := hdr.Get("X-Minecraft-Version"); got != "1.8.9" || hdr.Get("X-Minecraft-Username") != "Steve" || hdr.Get("X-Minecraft-UUID") != offlineUUID("Steve") || hdr.Get("X-Minecraft-Pack-Format") != "1" {
		t.Fatalf("headers %v", *hdr)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(g.opt.PacksDir), "server-resource-packs", hash)); err != nil {
		t.Fatalf("not kept under its hash: %v", err)
	}
	if g.opts.ServerPacks["play.example.net"] != "enabled" {
		t.Fatalf("answer not remembered: %v", g.opts.ServerPacks)
	}

	// the same server again: no question, accepted at once, and from the cache
	offerPack(t, g, url, hash)
	if g.screen == screenServerPack {
		t.Fatal("asked again")
	}
	if st := statuses(t, c); !same(st, []int32{packAccepted}) {
		t.Fatalf("second offer: %v", st)
	}
	awaitPost(t, c, g)
	if st := statuses(t, c); !same(st, []int32{packLoaded}) {
		t.Fatalf("second load: %v", st)
	}
	if hits.Load() != 1 {
		t.Fatalf("%d downloads: the cached pack should not be fetched again", hits.Load())
	}

	// leaving the server takes the pack off again
	g.dropServerPack()
	if g.serverPack != "" || redPixels(g) != 0 {
		t.Fatal("the server's pack stayed after leaving")
	}
}

func TestServerPackDeclinedIsRememberedAndAnsweredWithoutAsking(t *testing.T) {
	g, c := packGame(t)
	offerPack(t, g, "https://example.com/pack.zip", strings.Repeat("a", 40))
	g.answerServerPack(false, true)
	if st := statuses(t, c); !same(st, []int32{packDeclined}) {
		t.Fatalf("after No: %v", st)
	}
	offerPack(t, g, "https://example.com/pack.zip", strings.Repeat("a", 40))
	if g.screen == screenServerPack {
		t.Fatal("asked again after a No")
	}
	if st := statuses(t, c); !same(st, []int32{packDeclined}) {
		t.Fatalf("second offer: %v", st)
	}
	if g.serverPack != "" {
		t.Fatal("a pack was loaded after a No")
	}
}

// Escape on the question declines, so the server is not left waiting, but does not turn the server off for good.
func TestServerPackEscapeDeclinesOnce(t *testing.T) {
	g, c := packGame(t)
	offerPack(t, g, "https://example.com/pack.zip", strings.Repeat("b", 40))
	g.serverPackKey(256) // glfw.KeyEscape
	if st := statuses(t, c); !same(st, []int32{packDeclined}) {
		t.Fatalf("Escape: %v", st)
	}
	if g.opts.ServerPacks["play.example.net"] != "" {
		t.Fatal("Escape was remembered")
	}
	offerPack(t, g, "https://example.com/pack.zip", strings.Repeat("b", 40))
	if g.screen != screenServerPack {
		t.Fatal("the next offer should ask again")
	}
}

// Only http and https are fetched; the game answers anything else with a failed download.
func TestServerPackOtherSchemesFail(t *testing.T) {
	g, c := packGame(t)
	for _, u := range []string{"file:///etc/passwd", "ftp://example.com/p.zip", "level://world/resources.zip", "not a url", ""} {
		offerPack(t, g, u, strings.Repeat("c", 40))
		if st := statuses(t, c); !same(st, []int32{packFailed}) {
			t.Fatalf("%q: %v, want failed", u, st)
		}
		if g.screen == screenServerPack {
			t.Fatalf("%q was asked about", u)
		}
	}
}

func TestServerPackFailedDownloadIsTold(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	g, c := packGame(t)
	g.opts.ServerPacks = map[string]string{"play.example.net": "enabled"}
	offerPack(t, g, srv.URL+"/gone.zip", strings.Repeat("d", 40))
	if st := statuses(t, c); !same(st, []int32{packAccepted}) {
		t.Fatalf("accepted: %v", st)
	}
	awaitPost(t, c, g)
	if st := statuses(t, c); !same(st, []int32{packFailed}) {
		t.Fatalf("after a 404: %v, want failed", st)
	}
	if g.serverPack != "" {
		t.Fatal("a pack is set after a failure")
	}
}

func TestFetchServerPackChecksTheHashAndTheFile(t *testing.T) {
	body, hash := testPackZip(t)
	srv, hits, _ := packServer(t, body)
	dir := t.TempDir()

	if _, err := fetchServerPack(dir, srv.URL, strings.Repeat("0", 40), "a", "b"); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("a wrong hash was taken: %v", err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Fatalf("a refused download left %d files", len(es))
	}
	path, err := fetchServerPack(dir, srv.URL, strings.ToUpper(hash), "a", "b")
	if err != nil || filepath.Base(path) != hash {
		t.Fatalf("path %q err %v", path, err)
	}
	// a cached file that was damaged is downloaded again
	os.WriteFile(path, []byte("rubbish"), 0o644)
	if _, err := fetchServerPack(dir, srv.URL, hash, "a", "b"); err != nil || hits.Load() != 3 {
		t.Fatalf("damaged cache: err %v, %d requests", err, hits.Load())
	}
	// a hash that is not a SHA-1 is "legacy": always downloaded, not checked
	p1, err := fetchServerPack(dir, srv.URL, "", "a", "b")
	if err != nil || filepath.Base(p1) != "legacy" {
		t.Fatalf("legacy: %q %v", p1, err)
	}
	if _, err := fetchServerPack(dir, srv.URL, "x", "a", "b"); err != nil || hits.Load() != 5 {
		t.Fatalf("legacy twice: err %v, %d requests", err, hits.Load())
	}

	junk, _, _ := packServer(t, []byte("this is not a zip"))
	if _, err := fetchServerPack(t.TempDir(), junk.URL, "", "a", "b"); err == nil {
		t.Fatal("a file that is no zip was taken as a pack")
	}
}

func TestPrunePacksKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, strings.Repeat(string(rune('a'+i)), 40))
		os.WriteFile(p, []byte{1}, 0o644)
		os.Chtimes(p, now.Add(time.Duration(i)*time.Hour), now.Add(time.Duration(i)*time.Hour))
	}
	prunePacks(dir, 2)
	es, _ := os.ReadDir(dir)
	if len(es) != 2 || es[0].Name()[0] != 'd' || es[1].Name()[0] != 'e' {
		t.Fatalf("kept %v", es)
	}
}

func TestOfflineUUIDIsVanillas(t *testing.T) {
	// the id a 1.8.9 client without an account has for "Notch" (UUID.nameUUIDFromBytes("OfflinePlayer:Notch"))
	if got := offlineUUID("Notch"); len(got) != 32 || got[12] != '3' {
		t.Fatalf("%q is not a version 3 uuid", got)
	}
}

func TestServerPackChoiceIsSavedWithTheOptions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := defaultOptions()
	o.ServerPacks = map[string]string{"mc.example.com:25565": "enabled", "other.net": "disabled"}
	if err := o.save(); err != nil {
		t.Fatal(err)
	}
	got := loadOptions("")
	if got.ServerPacks["mc.example.com:25565"] != "enabled" || got.ServerPacks["other.net"] != "disabled" {
		t.Fatalf("read back %v", got.ServerPacks)
	}
}

var _ = world.ChunkPos{}
