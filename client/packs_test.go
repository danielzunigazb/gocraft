package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResourcePackOverridesAndScalesTextures(t *testing.T) {
	jar := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "client.jar")
	pack := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "resourcepacks", "bdefault-7-edit-f-8x.zip")
	if _, err := os.Stat(pack); err != nil {
		t.Skip("no 8x pack")
	}
	plain, err := loadResources(jar, nil)
	if err != nil {
		t.Fatal(err)
	}
	with, err := loadResources(jar, []string{pack})
	if err != nil {
		t.Fatal(err)
	}
	if plain.atlas.tile != 16 || with.atlas.tile != 16 {
		t.Fatalf("tile sizes %d and %d: 8x textures are enlarged to the game's 16", plain.atlas.tile, with.atlas.tile)
	}
	if string(plain.atlas.img.Pix) == string(with.atlas.img.Pix) {
		t.Fatal("the pack changed nothing")
	}
	// A stone block still draws and still collides, and models still resolve with the pack on.
	g := testGame(t)
	g.res = with
	if len(with.fast[1<<4].variants) == 0 || !with.opaque[1] {
		t.Fatal("stone is gone with the pack on")
	}
	// A missing pack is skipped, not fatal.
	if _, err := loadResources(jar, []string{filepath.Join(t.TempDir(), "nothing.zip")}); err != nil {
		t.Fatal(err)
	}
	_ = g
}

func TestGlyphWidthsFollowTheFontResolution(t *testing.T) {
	g := testGame(t)
	base := glyphWidths(g.res.gui["ascii"])
	if base['i'] >= base['m'] || base[' '] != 3 {
		t.Fatalf("widths %d %d", base['i'], base['m'])
	}
}
