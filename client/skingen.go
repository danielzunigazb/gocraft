package client

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// The default look of the player: a dark-armoured wanderer with glowing cyan lines, a purple-to-cyan
// cape and feathered wings. It is drawn here in code onto the game's own skin layout (64x64), cape
// layout (64x32) and a wing sheet, and written to the config folder, where the player can put their
// own skin.png, cape.png or wings.png in its place.

func c(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

var (
	skinTone  = c(224, 172, 140)
	skinShade = c(200, 148, 118)
	hairDark  = c(16, 22, 40)
	hairLight = c(34, 46, 78)
	armour    = c(27, 39, 64)
	armourDk  = c(14, 20, 36)
	armourLt  = c(44, 62, 98)
	cyan      = c(25, 215, 245)
	cyanDim   = c(18, 150, 180)
	magenta   = c(224, 64, 251)
	snow      = c(235, 250, 255)
)

type canvas struct{ img *image.RGBA }

func (cv canvas) rect(x, y, w, h int, col color.RGBA) {
	draw.Draw(cv.img, image.Rect(x, y, x+w, y+h), &image.Uniform{col}, image.Point{}, draw.Src)
}

func (cv canvas) px(x, y int, col color.RGBA) { cv.img.SetRGBA(x, y, col) }

// noise gives a stable speckle so flat colours do not look flat.
func grain(x, y int) float64 {
	return math.Mod(math.Sin(float64(x)*12.9898+float64(y)*78.233)*43758.5453, 1)
}

func tone(col color.RGBA, k float64) color.RGBA {
	f := func(v uint8) uint8 { return uint8(math.Max(0, math.Min(255, float64(v)*k))) }
	return color.RGBA{f(col.R), f(col.G), f(col.B), col.A}
}

// speckled fills a rectangle with a colour and a little variation.
func (cv canvas) speckled(x, y, w, h int, col color.RGBA, amount float64) {
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			cv.px(x+i, y+j, tone(col, 1+(grain(x+i, y+j)-0.5)*amount))
		}
	}
}

// generateSkin draws the 64x64 skin.
func generateSkin() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	cv := canvas{img}

	// Head: skin on every face, hair over the top and back, a fringe over the brow.
	cv.speckled(0, 8, 32, 8, skinTone, 0.06)
	cv.speckled(8, 0, 16, 8, hairDark, 0.2)  // top and bottom
	cv.rect(16, 0, 8, 8, skinShade)          // under the chin
	cv.speckled(24, 8, 8, 8, hairDark, 0.25) // back of the head
	for _, x := range []int{0, 16} {         // sides: hair above the ear, sideburn
		cv.rect(x, 8, 8, 3, hairDark)
		cv.rect(x, 11, 2, 3, hairDark)
	}
	// Front of the face at 8..16 x 8..16.
	cv.rect(8, 8, 8, 3, hairDark)
	cv.px(8, 11, hairDark)
	cv.px(15, 11, hairDark)
	cv.px(9, 10, hairLight)
	cv.px(12, 10, hairLight)
	for _, x := range []int{9, 13} { // glowing eyes
		cv.rect(x, 12, 2, 1, cyan)
		cv.px(x, 13, cyanDim)
		cv.px(x+1, 13, tone(cyan, 0.7))
	}
	cv.rect(11, 14, 2, 1, skinShade) // mouth
	cv.px(8, 15, skinShade)
	cv.px(15, 15, skinShade)

	// Hat layer: swept-back hair and a glowing headband.
	cv.speckled(40, 0, 8, 8, hairLight, 0.3)
	cv.speckled(32, 8, 32, 3, hairDark, 0.3)
	cv.rect(32, 11, 32, 1, cyan)
	for _, x := range []int{32, 48, 56} { // sides and back reach lower
		cv.rect(x, 12, 8, 3, hairDark)
	}
	cv.px(41, 9, hairLight)
	cv.px(44, 10, hairLight)

	// Body: a dark chest with a cyan emblem. Front at 20..28 x 20..32.
	cv.speckled(16, 20, 24, 12, armourDk, 0.08)
	cv.rect(20, 20, 8, 12, armour)
	cv.rect(23, 20, 2, 12, cyanDim) // a stripe down the front
	cv.rect(22, 23, 4, 4, armourDk)
	cv.rect(23, 24, 2, 2, cyan) // the emblem
	cv.px(24, 23, magenta)
	cv.rect(20, 30, 8, 1, cyan) // belt
	cv.rect(32, 20, 8, 12, armourDk)
	cv.rect(35, 20, 2, 12, armour)
	cv.rect(20, 16, 16, 4, armourLt) // top and bottom of the torso
	// Jacket layer over it.
	cv.rect(16, 36, 24, 12, color.RGBA{0, 0, 0, 0})
	cv.rect(20, 36, 8, 12, color.RGBA{0, 0, 0, 0})
	cv.rect(20, 36, 8, 2, armourLt)
	cv.rect(20, 36, 1, 12, cyanDim)
	cv.rect(27, 36, 1, 12, cyanDim)
	cv.rect(32, 36, 8, 3, armourLt)
	cv.rect(20, 46, 8, 2, armourDk)
	cv.rect(20, 47, 8, 1, cyan)

	// Arms: armoured above, bare hands below.
	// Right arm faces (outer, front, inner, back) and its sleeve and hand.
	armFaces := func(x, y int, hand bool) {
		cv.speckled(x, y+4, 16, 12, armour, 0.08)
		cv.rect(x, y+4, 16, 1, armourLt)
		cv.rect(x, y+9, 16, 1, cyan)
		cv.rect(x, y+13, 16, 3, skinTone)
		cv.rect(x, y, 8, 4, armourLt) // top and bottom
		cv.rect(x+8, y, 4, 4, skinShade)
		if hand {
			cv.rect(x, y+12, 16, 1, skinShade)
		}
	}
	armFaces(40, 16, true)
	armFaces(32, 48, true)
	// Sleeves: shoulder plates over the top of each arm.
	for _, a := range [][2]int{{40, 32}, {48, 48}} {
		cv.rect(a[0], a[1]+4, 16, 6, color.RGBA{0, 0, 0, 0})
		cv.rect(a[0], a[1]+4, 16, 2, armourLt)
		cv.rect(a[0], a[1]+6, 16, 1, cyan)
		cv.rect(a[0], a[1], 8, 4, armourLt)
	}

	// Legs: dark trousers with a cyan stripe on the outside, black boots with glowing soles.
	legFaces := func(x, y int) {
		cv.speckled(x, y+4, 16, 12, armourDk, 0.08)
		cv.rect(x, y+4, 4, 12, armour) // outer
		cv.rect(x+1, y+4, 1, 8, cyanDim)
		cv.rect(x, y+12, 16, 4, color.RGBA{10, 12, 20, 255}) // boots
		cv.rect(x, y+11, 16, 1, cyan)
		cv.rect(x, y+15, 16, 1, cyanDim)
		cv.rect(x, y, 8, 4, armourDk)
	}
	legFaces(0, 16)
	legFaces(16, 48)
	// Trouser layers: knee guards.
	for _, a := range [][2]int{{0, 32}, {0, 48}} {
		cv.rect(a[0], a[1]+4, 16, 12, color.RGBA{0, 0, 0, 0})
		cv.rect(a[0], a[1]+8, 16, 2, armourLt)
		cv.rect(a[0], a[1]+10, 16, 1, cyan)
	}
	return img
}

// generateCape draws the 64x32 cape: deep purple at the shoulders to cyan at the hem, with a
// bordered phoenix-like emblem. The outside is at 1..11 x 1..17, the inside at 12..22.
func generateCape() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	cv := canvas{img}
	grad := func(y, h int) color.RGBA {
		t := float64(y) / float64(h-1)
		return color.RGBA{uint8(60 + (25-60)*t), uint8(20 + (200-20)*t), uint8(110 + (240-110)*t), 255}
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 10; x++ {
			col := tone(grad(y, 16), 1+(grain(x, y)-0.5)*0.12)
			cv.px(1+x, 1+y, col)
			cv.px(12+x, 1+y, tone(col, 0.55)) // the inside is darker
		}
	}
	for y := 0; y < 16; y++ { // trim down the edges
		cv.px(1, 1+y, snow)
		cv.px(10, 1+y, snow)
	}
	cv.rect(1, 16, 10, 1, snow)
	cv.rect(1, 1, 10, 1, tone(snow, 0.8))
	// The emblem: a diamond with wings, glowing.
	for dy := -3; dy <= 3; dy++ {
		w := 3 - int(math.Abs(float64(dy)))
		for dx := -w; dx <= w; dx++ {
			cv.px(5+dx, 8+dy, snow)
		}
	}
	cv.px(5, 8, cyan)
	cv.px(5, 7, magenta)
	cv.rect(2, 7, 2, 1, snow)
	cv.rect(7, 7, 2, 1, snow)
	cv.rect(2, 8, 1, 1, tone(snow, 0.8))
	cv.rect(8, 8, 1, 1, tone(snow, 0.8))
	// Top, bottom and sides of the thin cloth.
	cv.rect(1, 0, 10, 1, grad(0, 16))
	cv.rect(11, 0, 10, 1, tone(grad(15, 16), 0.8))
	cv.rect(0, 1, 1, 16, grad(8, 16))
	cv.rect(11, 1, 1, 16, grad(8, 16))
	cv.rect(22, 1, 1, 16, grad(8, 16))
	return img
}

// generateWings draws the wing sheet, 64x32: feathers fading from snow at the root to violet tips,
// edged with a cyan glow.
func generateWings() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	cv := canvas{img}
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			t := float64(y) / 31
			base := color.RGBA{uint8(235 - 90*t), uint8(245 - 120*t), 255, 255}
			if (x/2)%2 == 0 { // vertical feather barbs
				base = tone(base, 0.93)
			}
			if x%8 == 0 || y == 0 { // the feather edges glow
				base = color.RGBA{uint8(30 + 90*t), uint8(210 - 70*t), 245, 255}
			}
			cv.px(x, y, tone(base, 1+(grain(x, y)-0.5)*0.08))
		}
	}
	return img
}

// writePNG saves an image.
func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func readPNG(path string) *image.RGBA {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img
}

// skinsDir is where the player's own skin, cape and wings go.
func skinsDir() string { return filepath.Join(filepath.Dir(optionsPath()), "skins") }

// loadLook reads skin.png, cape.png and wings.png from the skins folder, writing the generated
// defaults there first if a file is missing, so there is something to edit.
func loadLook() (skin, cape, wings *image.RGBA) {
	dir := skinsDir()
	get := func(name string, gen func() *image.RGBA) *image.RGBA {
		path := filepath.Join(dir, name)
		if img := readPNG(path); img != nil {
			return img
		}
		img := gen()
		_ = writePNG(path, img) // best effort: the game runs fine without being able to save it
		return img
	}
	return get("skin.png", generateSkin), get("cape.png", generateCape), get("wings.png", generateWings)
}
