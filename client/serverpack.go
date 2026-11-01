package client

import (
	"archive/zip"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Resource packs a server offers, after NetHandlerPlayClient.handleResourcePack and ResourcePackRepository
// of 1.8.9. Resource Pack Send (0x48) gives a URL and the SHA-1 of the file; the client answers with
// Resource Pack Status (0x19): accepted, then loaded or failed, or declined. The pack is kept in
// server-resource-packs/<hash>, so the next visit does not download it again, is laid over the player's own
// packs, and is dropped when the player leaves the server. Like the game, the first offer of a server asks
// (GuiYesNo) and the answer is remembered for that server.

// The answers of Resource Pack Status (C19PacketResourcePackStatus.Action).
const (
	packLoaded   = 0 // SUCCESSFULLY_LOADED
	packDeclined = 1 // DECLINED
	packFailed   = 2 // FAILED_DOWNLOAD
	packAccepted = 3 // ACCEPTED
)

const (
	serverPackLimit = 52428800 // the game's download limit: 50 MiB
	serverPackTime  = 60 * time.Second
	serverPackKeep  = 10 // how many cached packs are kept (ResourcePackRepository.deleteOldServerResourcesPacks)
)

// hashName is the file a pack with this hash is cached in: the hash itself when it is a SHA-1, else "legacy".
var hashName = regexp.MustCompile(`^[a-f0-9]{40}$`)

// packOffer is a server's pack the player has not answered yet.
type packOffer struct{ url, hash string }

// serverPackURL says whether the game would download from this URL: http and https only. (The game also
// reads level:// from the saves folder, which a remote server has no business with.)
func serverPackURL(raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	s := strings.ToLower(u.Scheme)
	return (s == "http" || s == "https") && u.Host != ""
}

// serverPackDir is where downloaded packs are kept, next to the folder of the player's own packs.
func (g *game) serverPackDir() string {
	if g.opt.PacksDir != "" {
		return filepath.Join(filepath.Dir(g.opt.PacksDir), "server-resource-packs")
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "gocraft", "server-resource-packs")
	}
	return filepath.Join(os.TempDir(), "gocraft-server-resource-packs")
}

// offlineUUID is the id the game gives a player who is not signed in: the MD5 of "OfflinePlayer:<name>" as a
// version 3 UUID, without dashes.
func offlineUUID(name string) string {
	h := md5.Sum([]byte("OfflinePlayer:" + name))
	h[6] = h[6]&0x0f | 0x30
	h[8] = h[8]&0x3f | 0x80
	return hex.EncodeToString(h[:])
}

// packStatus tells the server what became of its pack.
func (g *game) packStatus(hash string, action int32) {
	if g.net == nil {
		return
	}
	w := newPacket(0x19)
	w.String(hash)
	w.VarInt(action)
	g.net.c.send(w)
}

// offerServerPack is Resource Pack Send: a pack the server recommends.
func (g *game) offerServerPack(url, hash string) {
	n := g.net
	if n == nil {
		return
	}
	if !serverPackURL(url) {
		g.packStatus(hash, packFailed)
		return
	}
	if n.packAsk != nil { // a new offer takes the place of one still waiting
		g.packStatus(n.packAsk.hash, packDeclined)
		n.packAsk = nil
	}
	switch g.opts.ServerPacks[strings.ToLower(n.addr)] {
	case "enabled":
		g.acceptServerPack(url, hash)
	case "disabled":
		g.packStatus(hash, packDeclined)
	default:
		n.packAsk = &packOffer{url, hash}
		g.openScreen(screenServerPack)
	}
}

// answerServerPack is the player's Yes or No on the question. remember keeps the answer for this server.
func (g *game) answerServerPack(yes, remember bool) {
	n := g.net
	if n == nil || n.packAsk == nil {
		g.closeScreen()
		return
	}
	o := n.packAsk
	n.packAsk = nil
	if g.screen == screenServerPack {
		g.closeScreen()
	}
	if remember {
		if g.opts.ServerPacks == nil {
			g.opts.ServerPacks = map[string]string{}
		}
		g.opts.ServerPacks[strings.ToLower(n.addr)] = map[bool]string{true: "enabled", false: "disabled"}[yes]
		if g.opt.OptionsFile != "/dev/null" {
			if err := g.opts.save(); err != nil {
				logf("saving the options: %v", err)
			}
		}
	}
	if yes {
		g.acceptServerPack(o.url, o.hash)
	} else {
		g.packStatus(o.hash, packDeclined)
	}
}

// acceptServerPack answers "accepted" and gets the file, from the cache or the net, without stopping the game.
func (g *game) acceptServerPack(url, hash string) {
	n := g.net
	g.packStatus(hash, packAccepted)
	c := n.c
	dir := g.serverPackDir()
	name := g.net.c.name
	uuid := offlineUUID(name)
	if la := g.onlineLogin(); la != nil {
		name, uuid = la.Session.Name, la.Session.UUID
	}
	go func() {
		path, err := serverPackFetcher(dir, url, hash, name, uuid)
		post := func(f func(*game)) {
			select {
			case c.inbox <- f:
			case <-c.done:
			}
		}
		if err != nil {
			logf("server resource pack: %v", err)
			post(func(g *game) { g.packStatus(hash, packFailed) })
			return
		}
		post(func(g *game) { g.useServerPack(path, hash) })
	}()
}

// useServerPack lays the downloaded pack over the player's and loads the resources again.
func (g *game) useServerPack(path, hash string) {
	if g.net == nil {
		return
	}
	g.serverPack = path
	if err := g.reloadResources(); err != nil {
		logf("server resource pack: %v", err)
		g.serverPack = ""
		g.reloadResources()
		g.packStatus(hash, packFailed)
		return
	}
	g.packStatus(hash, packLoaded)
}

// dropServerPack takes the server's pack off again, as the game does when the player leaves (clearResourcePack).
func (g *game) dropServerPack() {
	if g.serverPack == "" {
		return
	}
	g.serverPack = ""
	if err := g.reloadResources(); err != nil {
		logf("reloading resources: %v", err)
	}
}

// serverPackFetcher gets the pack's file. A variable so that tests do not need the network.
var serverPackFetcher = fetchServerPack

// fetchServerPack is ResourcePackRepository.downloadResourcePack: a cached file of the right hash is used as it
// is; else the pack is downloaded (50 MiB at most), checked against the hash and kept in dir.
func fetchServerPack(dir, url, hash, name, uuid string) (string, error) {
	hash = strings.ToLower(hash)
	file := "legacy"
	if hashName.MatchString(hash) {
		file = hash
	}
	path := filepath.Join(dir, file)
	if file != "legacy" {
		if sum, err := fileSHA1(path); err == nil {
			if sum == hash {
				now := time.Now()
				os.Chtimes(path, now, now) // the last used are the last to be deleted
				return path, nil
			}
			os.Remove(path) // damaged, or not what it was
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Minecraft-Username", name)
	req.Header.Set("X-Minecraft-UUID", uuid)
	req.Header.Set("X-Minecraft-Version", "1.8.9")
	req.Header.Set("X-Minecraft-Pack-Format", "1")
	resp, err := (&http.Client{Timeout: serverPackTime}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	tmp, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	sum := sha1.New()
	n, err := io.Copy(io.MultiWriter(tmp, sum), io.LimitReader(resp.Body, serverPackLimit+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if n > serverPackLimit {
		return "", errors.New("the pack is over 50 MiB")
	}
	if got := hex.EncodeToString(sum.Sum(nil)); file != "legacy" && got != hash {
		return "", fmt.Errorf("the pack's hash is %s, not the %s the server gave", got, hash)
	}
	if err := checkPackZip(tmp.Name()); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	prunePacks(dir, serverPackKeep)
	return path, nil
}

// checkPackZip refuses what is not a resource pack: a file that is no zip, or a zip without assets.
func checkPackZip(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("not a resource pack: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.HasPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "assets/") || f.Name == "pack.mcmeta" {
			return nil
		}
	}
	return errors.New("not a resource pack: it has no assets")
}

func fileSHA1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// prunePacks keeps the keep most recently used files of dir (the game does the same with its server packs).
func prunePacks(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		path string
		t    time.Time
	}
	var all []f
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), "download-") {
			continue
		}
		if info, err := e.Info(); err == nil {
			all = append(all, f{filepath.Join(dir, e.Name()), info.ModTime()})
		}
	}
	for len(all) > keep {
		old := 0
		for i := range all {
			if all[i].t.Before(all[old].t) {
				old = i
			}
		}
		os.Remove(all[old].path)
		all = append(all[:old], all[old+1:]...)
	}
}

// ---- the question ----

// drawServerPackPrompt is GuiYesNo as handleResourcePack opens it: two lines and Yes and No.
func (g *game) drawServerPackPrompt(w, h int) {
	if g.screen != screenServerPack || g.net == nil || g.net.packAsk == nil {
		return
	}
	g.enableUI()
	hd := &g.hud
	s := float32(g.scaleFor(w, h))
	cx := float32(w) / 2
	hd.rect(0, 0, float32(w), float32(h), 0, 0, 1, 1, 1, 1, [4]float32{0.06, 0.06, 0.06, 0.75})
	hd.flush(hd.white)
	white := [4]float32{1, 1, 1, 1}
	for i, l := range []string{"This server recommends the use of a custom resource pack.", "Do you want to download and install it automatically?"} {
		g.text(l, cx-float32(g.textWidth(l))*s/2, float32(70+i*12)*s, s, white, true)
	}
	left := cx/s - 155
	top := float32(h)/s/6 + 96
	if g.button("Yes", left, top, 150, true) {
		g.answerServerPack(true, true)
	} else if g.button("No", left+160, top, 150, true) {
		g.answerServerPack(false, true)
	}
	g.uiClick = false
}

// serverPackKey is a key pressed on the question. Escape is a No that is not remembered, so the server is
// not left waiting.
func (g *game) serverPackKey(key glfw.Key) {
	if key == glfw.KeyEscape {
		g.answerServerPack(false, false)
	}
}
