package client

import (
	"image"
	"image/draw"
	"os"
	"testing"
)

// TestWritePreview draws the generated pictures enlarged, for looking at; it only runs when asked.
func TestWritePreview(t *testing.T) {
	out := os.Getenv("GOCRAFT_PREVIEW")
	if out == "" {
		t.Skip("set GOCRAFT_PREVIEW to a folder")
	}
	up := func(src *image.RGBA, k int) *image.RGBA {
		dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx()*k, src.Bounds().Dy()*k))
		for y := 0; y < dst.Bounds().Dy(); y++ {
			for x := 0; x < dst.Bounds().Dx(); x++ {
				dst.SetRGBA(x, y, src.RGBAAt(x/k, y/k))
			}
		}
		bg := image.NewRGBA(dst.Bounds()) // on a grey checker so transparency is visible
		for y := 0; y < bg.Bounds().Dy(); y++ {
			for x := 0; x < bg.Bounds().Dx(); x++ {
				v := uint8(60)
				if (x/8+y/8)%2 == 0 {
					v = 90
				}
				bg.SetRGBA(x, y, c(v, v, v))
			}
		}
		draw.Draw(bg, bg.Bounds(), dst, image.Point{}, draw.Over)
		return bg
	}
	writePNG(out+"/skin.png", up(generateSkin(), 8))
	writePNG(out+"/cape.png", up(generateCape(), 8))
	writePNG(out+"/wings.png", up(generateWings(), 8))
}
