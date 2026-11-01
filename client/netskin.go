package client

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Other players' skins. A server that checks accounts names each player's skin in the player list; the game
// fetches the picture from Mojang's texture server (once per skin, in the background) and wears it on that
// player. Everyone else wears the default skin their uuid picks: Steve, or Alex with thin arms.

// skinHost is where a skin may come from: the game itself only fetches from Mojang's own domains.
func skinAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "minecraft.net" || strings.HasSuffix(h, ".minecraft.net") || h == "mojang.com" || strings.HasSuffix(h, ".mojang.com")
}

// skinFetcher gets the picture at a URL. A variable so that tests do not need the network.
var skinFetcher = func(raw string) ([]byte, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("skin: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// skinImage decodes a skin and brings it to the 64 x 64 layout, as ImageBufferDownload does.
func skinImage(b []byte) (*image.RGBA, error) {
	src, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	sb := src.Bounds()
	if sb.Dx() != 64 || (sb.Dy() != 32 && sb.Dy() != 64) {
		return nil, fmt.Errorf("skin: %d x %d is not a skin", sb.Dx(), sb.Dy())
	}
	return upgradeSkin(src), nil
}

// upgradeSkin makes a 64 x 32 skin 64 x 64 (the left leg and arm are the right ones, turned about) and fills the
// opaque areas, as parseUserSkin does.
func upgradeSkin(src image.Image) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	draw.Draw(img, image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()), src, src.Bounds().Min, draw.Src)
	legacy := src.Bounds().Dy() == 32
	copyFlipped := func(x, y, dx, dy, w, h int) {
		for i := 0; i < w; i++ {
			for j := 0; j < h; j++ {
				img.SetRGBA(x+dx+w-1-i, y+dy+j, img.RGBAAt(x+i, y+j))
			}
		}
	}
	if legacy {
		for _, c := range [][6]int{
			{4, 16, 16, 32, 4, 4}, {8, 16, 16, 32, 4, 4}, {0, 20, 24, 32, 4, 12}, {4, 20, 16, 32, 4, 12}, {8, 20, 8, 32, 4, 12}, {12, 20, 16, 32, 4, 12},
			{44, 16, -8, 32, 4, 4}, {48, 16, -8, 32, 4, 4}, {40, 20, 0, 32, 4, 12}, {44, 20, -8, 32, 4, 12}, {48, 20, -16, 32, 4, 12}, {52, 20, -8, 32, 4, 12},
		} {
			copyFlipped(c[0], c[1], c[2], c[3], c[4], c[5])
		}
	}
	opaque := func(x1, y1, x2, y2 int) {
		for x := x1; x < x2; x++ {
			for y := y1; y < y2; y++ {
				p := img.RGBAAt(x, y)
				p.A = 255
				img.SetRGBA(x, y, p)
			}
		}
	}
	opaque(0, 0, 32, 16)
	opaque(0, 16, 64, 32)
	opaque(16, 48, 48, 64)
	return img
}

// skinSlot is a skin the game knows of: being fetched, ready on the GPU, or given up.
type skinSlot struct {
	tex     uint32
	state   int // 0 fetching, 1 ready, 2 failed
	picture *image.RGBA
}

type skinResult struct {
	url string
	img *image.RGBA
}

// wantSkin starts getting the skin a player list entry names, if it is not already.
func (g *game) wantSkin(e *tabEntry) { g.wantSkinURL(e.skinURL) }

// wantSkinURL starts getting the skin at a URL, if it is Mojang's and not already on its way.
func (g *game) wantSkinURL(url string) {
	if url == "" || !skinAllowed(url) {
		return
	}
	if g.skins == nil {
		g.skins = map[string]*skinSlot{}
		g.skinDone = make(chan skinResult, 32)
	}
	if _, ok := g.skins[url]; ok {
		return
	}
	g.skins[url] = &skinSlot{}
	u := url
	go func() {
		b, err := skinFetcher(u)
		var img *image.RGBA
		if err == nil {
			img, err = skinImage(b)
		}
		if err != nil {
			img = nil
		}
		g.skinDone <- skinResult{u, img}
	}()
}

// pumpSkins puts the skins that have arrived on the GPU: it has to be the thread that owns the context.
func (g *game) pumpSkins() {
	g.ensureDefaultSkins()
	for {
		select {
		case r := <-g.skinDone:
			s := g.skins[r.url]
			if s == nil {
				continue
			}
			if r.img == nil {
				s.state = 2
				continue
			}
			s.tex, s.state, s.picture = uploadTexture(r.img), 1, r.img
			g.mobTex[skinKey(r.url)] = s.tex
		default:
			return
		}
	}
}

// skinKey is the name a worn skin goes by among the textures.
func skinKey(u string) string { return "skin/" + u[strings.LastIndex(u, "/")+1:] }

// javaHash is UUID.hashCode of a uuid given as 32 hex digits: what picks Steve or Alex.
func javaHash(uuid string) int32 {
	if len(uuid) != 32 {
		return 0
	}
	var hi, lo uint64
	for _, c := range uuid[:16] {
		hi = hi<<4 | uint64(hexVal(byte(c)))
	}
	for _, c := range uuid[16:] {
		lo = lo<<4 | uint64(hexVal(byte(c)))
	}
	x := hi ^ lo
	return int32(x>>32) ^ int32(uint32(x))
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// defaultSlim is whether a uuid's default skin is Alex's: DefaultPlayerSkin.getDefaultSkin.
func defaultSlim(uuid string) bool { return javaHash(uuid)&1 == 1 }

// skinFor is the texture and the arms a player is drawn with.
func (g *game) skinFor(p *remotePlayer) (key string, slim bool) {
	if e := g.net.tab[p.uuid]; e != nil && e.skinURL != "" {
		if s := g.skins[e.skinURL]; s != nil && s.state == 1 {
			return skinKey(e.skinURL), e.slim
		}
	}
	slim = defaultSlim(p.uuid)
	return g.defaultSkinKey(slim), slim
}

// defaultSkinKey is the name of the default skin among the textures.
func (g *game) defaultSkinKey(slim bool) string {
	if slim {
		return "skin/default/alex"
	}
	return "skin/default/steve"
}

// ensureDefaultSkins uploads Steve and Alex, upgraded to the 64 x 64 layout, the first time they are wanted.
func (g *game) ensureDefaultSkins() {
	for _, name := range []string{"steve", "alex"} {
		key := "skin/default/" + name
		if _, ok := g.mobTex[key]; ok {
			continue
		}
		if src := g.res.entity[name]; src != nil {
			g.mobTex[key] = uploadTexture(upgradeSkin(src))
		} else {
			g.mobTex[key] = g.mobTex["look/skin"]
		}
	}
}

// drawHead draws a player's face (and its hat layer) for the list.
func (g *game) drawHead(e *tabEntry, x, y, s float32) {
	key := g.defaultSkinKey(defaultSlim(e.uuid))
	if sl := g.skins[e.skinURL]; sl != nil && sl.state == 1 {
		key = skinKey(e.skinURL)
	}
	tex := g.mobTex[key]
	hd := &g.hud
	one := [4]float32{1, 1, 1, 1}
	hd.rect(x, y, 8*s, 8*s, 8, 8, 8, 8, 64, 64, one)
	hd.rect(x, y, 8*s, 8*s, 40, 8, 8, 8, 64, 64, one)
	hd.flush(tex)
	gl.Disable(gl.BLEND)
	gl.Enable(gl.BLEND)
}
