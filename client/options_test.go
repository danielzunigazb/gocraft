package client

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func scan(s string) *bufio.Scanner { return bufio.NewScanner(strings.NewReader(s)) }

func TestImportsThePlayersOptions(t *testing.T) {
	path := filepath.Join(os.Getenv("HOME"), "mc-1.8.9-run", "game", "options.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skip("no 1.8.9 options.txt")
	}
	o, ok := loadOptionsFile(path, defaultOptions())
	if !ok {
		t.Fatal("not read")
	}
	if o.FOV != 50 || o.RenderDistance != 10 || o.GuiScale != 2 || o.VSync || o.InvertY {
		t.Fatalf("options %+v", o)
	}
	if math.Abs(float64(o.Sensitivity)-0.40140846) > 1e-6 {
		t.Fatalf("sensitivity %v", o.Sensitivity)
	}
	if o.Keys["forward"] != 17 || o.Keys["sneak"] != 42 || o.Keys["sprint"] != 29 || o.Keys["attack"] != -100 || o.Keys["inventory"] != 18 {
		t.Fatalf("keys %v", o.Keys)
	}
}

func TestOptionsParsingIsDefensive(t *testing.T) {
	o := parseOptions(scan("fov:9\nmouseSensitivity:abc\nrenderDistance:99\nguiScale:-3\nkey_key.forward:xyz\nkey_key.nonsense:5\nnoColon\n\n"), defaultOptions())
	if o.FOV != 110 || o.Sensitivity != 0.5 || o.RenderDistance != 16 || o.GuiScale != 0 || o.Keys["forward"] != 17 {
		t.Fatalf("%+v", o)
	}
	if _, bad := o.Keys["nonsense"]; bad {
		t.Fatal("an unknown key bind was added")
	}
}

func TestOptionsSaveAndReload(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := defaultOptions()
	o.FOV, o.Sensitivity, o.RenderDistance, o.Gamma, o.GuiScale, o.VSync, o.InvertY = 90, 0.25, 5, 0.5, 3, false, true
	o.Keys["jump"] = 28 // enter
	if err := o.save(); err != nil {
		t.Fatal(err)
	}
	got := loadOptions("") // gocraft's own file wins
	if got.FOV != 90 || got.Sensitivity != 0.25 || got.RenderDistance != 5 || got.Gamma != 0.5 || got.GuiScale != 3 || got.VSync || !got.InvertY || got.Keys["jump"] != 28 {
		t.Fatalf("reloaded %+v", got)
	}
}

func TestSensitivityCurveMatchesTheGame(t *testing.T) {
	// 1.8: f = s*0.6+0.2, degrees per pixel = f^3 * 8 * 0.15. Sensitivity 0.4014 is about 0.102 deg/pixel.
	o := defaultOptions()
	o.Sensitivity = 0.40140846
	if d := o.degPerPixel(); d < 0.100 || d > 0.105 {
		t.Fatalf("%v degrees per pixel", d)
	}
	o.Sensitivity = 1
	if d := o.degPerPixel(); math.Abs(float64(d)-0.8*0.8*0.8*8*0.15) > 1e-4 {
		t.Fatalf("hyperspeed %v", d)
	}
}

func TestKeyCodesRoundTrip(t *testing.T) {
	for code, k := range lwjgl {
		if lwjglOf[k] != code {
			t.Fatalf("code %d -> key %d -> %d", code, k, lwjglOf[k])
		}
	}
	for code, want := range map[int]string{-100: "Button 1", -99: "Button 2", 0: "NONE", 57: "SPACE", 17: "W", 42: "LSHIFT", 2: "1", 59: "F1"} {
		if got := keyName(code); got != want {
			t.Errorf("key %d is called %q, want %q", code, got, want)
		}
	}
}

func TestGuiScaleOption(t *testing.T) {
	g := &game{}
	if g.scaleFor(1280, 720) != 3 || g.scaleFor(640, 480) != 2 {
		t.Fatal("auto scale")
	}
	g.opts.GuiScale = 2
	if g.scaleFor(1280, 720) != 2 {
		t.Fatal("the option picks a smaller scale")
	}
	g.opts.GuiScale = 4
	if g.scaleFor(640, 480) != 2 {
		t.Fatal("a scale that does not fit is cut down")
	}
}

func TestMouseButtonsUseTheGamesNumbers(t *testing.T) {
	o := defaultOptions()
	for b, action := range map[glfw.MouseButton]string{glfw.MouseButtonLeft: "attack", glfw.MouseButtonRight: "use", glfw.MouseButtonMiddle: "pickItem"} {
		if o.Keys[action] != mouseCode(b) {
			t.Errorf("button %d is %d, but %s is bound to %d", b, mouseCode(b), action, o.Keys[action])
		}
		if keyName(mouseCode(b)) != fmt.Sprintf("Button %d", int(b)+1) {
			t.Errorf("button %d is called %q", b, keyName(mouseCode(b)))
		}
	}
	if isMouseCode(17) || isMouseCode(0) || !isMouseCode(-98) {
		t.Fatal("isMouseCode")
	}
}

func TestResourcePacksOptionRoundTrips(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := defaultOptions()
	o.Packs = []string{"a pack.zip", "b.zip"}
	if err := o.save(); err != nil {
		t.Fatal(err)
	}
	got := loadOptions("")
	if len(got.Packs) != 2 || got.Packs[0] != "a pack.zip" || got.Packs[1] != "b.zip" {
		t.Fatalf("packs %v", got.Packs)
	}
	// The player's own options.txt names the packs when our file does not say.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	theirs := filepath.Join(t.TempDir(), "options.txt")
	os.WriteFile(theirs, []byte("resourcePacks:[\"mine.zip\"]\n"), 0o644)
	if got := loadOptions(theirs); len(got.Packs) != 1 || got.Packs[0] != "mine.zip" {
		t.Fatalf("imported packs %v", got.Packs)
	}
	// An older file of ours without the key also takes them from theirs.
	os.MkdirAll(filepath.Dir(optionsPath()), 0o755)
	os.WriteFile(optionsPath(), []byte("fov:0.5\n"), 0o644)
	if got := loadOptions(theirs); len(got.Packs) != 1 || got.Packs[0] != "mine.zip" {
		t.Fatalf("packs from an older file %v", got.Packs)
	}
	// But a file that says "none" stays none.
	os.WriteFile(optionsPath(), []byte("resourcePacks:[]\n"), 0o644)
	if got := loadOptions(theirs); len(got.Packs) != 0 {
		t.Fatalf("an explicit empty list was overridden: %v", got.Packs)
	}
}
