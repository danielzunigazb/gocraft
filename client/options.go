package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// The options use the same file format as 1.8.9's options.txt, so the player's own file can be
// read as the starting point. Changes are written to gocraft's own file, never to theirs.

// keyAction is a control the player can rebind: its id in options.txt ("key_key.<id>"), its label
// and its default as an LWJGL key code (negative numbers are mouse buttons: -100 left, -99 right).
type keyAction struct {
	id, label string
	def       int
}

var keyActions = []keyAction{
	{"forward", "Walk Forwards", 17}, {"left", "Strafe Left", 30}, {"back", "Walk Backwards", 31}, {"right", "Strafe Right", 32},
	{"jump", "Jump", 57}, {"sneak", "Sneak", 42}, {"sprint", "Sprint", 29}, {"inventory", "Open Inventory", 18},
	{"drop", "Drop Item", 16}, {"togglePerspective", "Toggle Perspective", 63}, {"attack", "Attack/Destroy", -100}, {"use", "Use/Place Block", -99}, {"pickItem", "Pick Block", -98},
}

type options struct {
	FOV            float32 // degrees, 30..110
	Sensitivity    float32 // 0..1
	InvertY        bool
	RenderDistance int // chunks, 2..16
	Gamma          float32
	GuiScale       int // 0 is automatic
	VSync          bool
	Fancy          bool               // fancy graphics: leaves keep their inner faces
	DynamicLights  bool               // held and dropped light sources light their surroundings
	Keys           map[string]int     // action id -> LWJGL code
	Packs          []string           // enabled resource packs, lowest priority first
	ViewBobbing    bool               // the view and the hand sway with each step
	Server         string             // the last server joined
	SoundVolume    map[string]float32 // soundCategory_*: master, music, record, weather, block, hostile, neutral, player, ambient, each 0..1
	MSAClientID    string             // the client ID of the Azure application used to sign in (public, not a secret)
	PlayerName     string             // the name used in multiplayer (offline mode)
	packsSet       bool               // the file said which packs, even none
}

func defaultOptions() options {
	o := options{FOV: 70, Sensitivity: 0.5, RenderDistance: 8, VSync: true, DynamicLights: true, ViewBobbing: true, Keys: map[string]int{}, SoundVolume: map[string]float32{}}
	for _, k := range keyActions {
		o.Keys[k.id] = k.def
	}
	return o
}

func clampf(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

// parseOptions reads options.txt text over the given options and returns it.
func parseOptions(r *bufio.Scanner, o options) options {
	for r.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(r.Text()), ":")
		if !ok {
			continue
		}
		f, ferr := strconv.ParseFloat(v, 32)
		n, nerr := strconv.Atoi(v)
		switch k {
		case "fov":
			if ferr == nil {
				o.FOV = clampf(30+float32(f)*80, 30, 110) // stored as 0..1 across 30..110
			}
		case "mouseSensitivity":
			if ferr == nil {
				o.Sensitivity = clampf(float32(f), 0, 1)
			}
		case "invertYMouse":
			o.InvertY = v == "true"
		case "gamma":
			if ferr == nil {
				o.Gamma = clampf(float32(f), 0, 1)
			}
		case "renderDistance":
			if nerr == nil {
				o.RenderDistance = max(2, min(16, n))
			}
		case "guiScale":
			if nerr == nil {
				o.GuiScale = max(0, min(4, n))
			}
		case "enableVsync":
			o.VSync = v == "true"
		case "fancyGraphics":
			o.Fancy = v == "true"
		case "viewBobbing":
			o.ViewBobbing = v == "true"
		case "dynamicLights":
			o.DynamicLights = v == "true"
		case "lastServer":
			o.Server = v
		case "msaClientId":
			o.MSAClientID = v
		case "playerName":
			if validName.MatchString(v) {
				o.PlayerName = v
			}
		case "resourcePacks":
			var names []string
			if json.Unmarshal([]byte(v), &names) == nil {
				o.Packs, o.packsSet = names, true
			}
		default:
			if cat, ok := strings.CutPrefix(k, "soundCategory_"); ok && ferr == nil {
				if o.SoundVolume == nil {
					o.SoundVolume = map[string]float32{}
				}
				o.SoundVolume[cat] = clampf(float32(f), 0, 1)
			}
			if id, ok := strings.CutPrefix(k, "key_key."); ok && nerr == nil {
				if _, known := o.Keys[id]; known {
					o.Keys[id] = n
				}
			}
		}
	}
	return o
}

func loadOptionsFile(path string, o options) (options, bool) {
	f, err := os.Open(path)
	if err != nil {
		return o, false
	}
	defer f.Close()
	return parseOptions(bufio.NewScanner(f), o), true
}

func optionsPath() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "gocraft", "options.txt")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gocraft", "options.txt")
}

// loadOptions reads gocraft's own file, or on the first run the player's 1.8.9 options.txt.
func loadOptions(importFrom string) options {
	o := defaultOptions()
	if got, ok := loadOptionsFile(optionsPath(), o); ok {
		if !got.packsSet && importFrom != "" { // an older file of ours: take the packs from the player's own
			if theirs, ok := loadOptionsFile(importFrom, defaultOptions()); ok {
				got.Packs = theirs.Packs
			}
		}
		return got
	}
	if importFrom != "" {
		o, _ = loadOptionsFile(importFrom, o)
	}
	return o
}

func (o options) save() error {
	var b strings.Builder
	fmt.Fprintf(&b, "fov:%g\nmouseSensitivity:%g\ninvertYMouse:%v\ngamma:%g\nrenderDistance:%d\nguiScale:%d\nenableVsync:%v\nfancyGraphics:%v\ndynamicLights:%v\nviewBobbing:%v\n",
		(o.FOV-30)/80, o.Sensitivity, o.InvertY, o.Gamma, o.RenderDistance, o.GuiScale, o.VSync, o.Fancy, o.DynamicLights, o.ViewBobbing)
	packs, _ := json.Marshal(append([]string{}, o.Packs...))
	fmt.Fprintf(&b, "resourcePacks:%s\n", packs)
	if o.Server != "" {
		fmt.Fprintf(&b, "lastServer:%s\n", o.Server)
	}
	if o.MSAClientID != "" {
		fmt.Fprintf(&b, "msaClientId:%s\n", o.MSAClientID)
	}
	if o.PlayerName != "" {
		fmt.Fprintf(&b, "playerName:%s\n", o.PlayerName)
	}
	for _, c := range soundCategories {
		if v, ok := o.SoundVolume[c]; ok {
			fmt.Fprintf(&b, "soundCategory_%s:%g\n", c, v)
		}
	}
	for _, k := range keyActions {
		fmt.Fprintf(&b, "key_key.%s:%d\n", k.id, o.Keys[k.id])
	}
	return writeAtomicString(optionsPath(), b.String())
}

func writeAtomicString(path, s string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(s), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// degPerPixel is how far the view turns per pixel of mouse movement, from 1.8's sensitivity curve.
func (o options) degPerPixel() float32 {
	f := o.Sensitivity*0.6 + 0.2
	return f * f * f * 8 * 0.15
}

// lwjgl maps the key codes 1.8 writes in options.txt to GLFW's.
var lwjgl = map[int]glfw.Key{
	1: glfw.KeyEscape, 2: glfw.Key1, 3: glfw.Key2, 4: glfw.Key3, 5: glfw.Key4, 6: glfw.Key5, 7: glfw.Key6, 8: glfw.Key7, 9: glfw.Key8, 10: glfw.Key9, 11: glfw.Key0,
	12: glfw.KeyMinus, 13: glfw.KeyEqual, 14: glfw.KeyBackspace, 15: glfw.KeyTab,
	16: glfw.KeyQ, 17: glfw.KeyW, 18: glfw.KeyE, 19: glfw.KeyR, 20: glfw.KeyT, 21: glfw.KeyY, 22: glfw.KeyU, 23: glfw.KeyI, 24: glfw.KeyO, 25: glfw.KeyP,
	26: glfw.KeyLeftBracket, 27: glfw.KeyRightBracket, 28: glfw.KeyEnter, 29: glfw.KeyLeftControl,
	30: glfw.KeyA, 31: glfw.KeyS, 32: glfw.KeyD, 33: glfw.KeyF, 34: glfw.KeyG, 35: glfw.KeyH, 36: glfw.KeyJ, 37: glfw.KeyK, 38: glfw.KeyL, 39: glfw.KeySemicolon,
	40: glfw.KeyApostrophe, 41: glfw.KeyGraveAccent, 42: glfw.KeyLeftShift, 43: glfw.KeyBackslash,
	44: glfw.KeyZ, 45: glfw.KeyX, 46: glfw.KeyC, 47: glfw.KeyV, 48: glfw.KeyB, 49: glfw.KeyN, 50: glfw.KeyM, 51: glfw.KeyComma, 52: glfw.KeyPeriod, 53: glfw.KeySlash,
	54: glfw.KeyRightShift, 56: glfw.KeyLeftAlt, 57: glfw.KeySpace, 58: glfw.KeyCapsLock,
	59: glfw.KeyF1, 60: glfw.KeyF2, 61: glfw.KeyF3, 62: glfw.KeyF4, 63: glfw.KeyF5, 64: glfw.KeyF6, 65: glfw.KeyF7, 66: glfw.KeyF8, 67: glfw.KeyF9, 68: glfw.KeyF10,
	87: glfw.KeyF11, 88: glfw.KeyF12, 157: glfw.KeyRightControl, 184: glfw.KeyRightAlt,
	199: glfw.KeyHome, 200: glfw.KeyUp, 201: glfw.KeyPageUp, 203: glfw.KeyLeft, 205: glfw.KeyRight, 207: glfw.KeyEnd, 208: glfw.KeyDown, 209: glfw.KeyPageDown,
	210: glfw.KeyInsert, 211: glfw.KeyDelete,
}

var lwjglOf = func() map[glfw.Key]int {
	m := map[glfw.Key]int{}
	for code, k := range lwjgl {
		m[k] = code
	}
	return m
}()

// mouseCode is a mouse button as 1.8 numbers it in options.txt: -100 is the left button, -99 the right.
func mouseCode(b glfw.MouseButton) int { return -100 + int(b) }

func isMouseCode(code int) bool { return code >= -100 && code < 0 }

// keyName is what the Controls screen shows for a key code.
func keyName(code int) string {
	switch code {
	case -100:
		return "Button 1"
	case -99:
		return "Button 2"
	case -98:
		return "Button 3"
	case 0:
		return "NONE"
	}
	if isMouseCode(code) {
		return fmt.Sprintf("Button %d", code+101)
	}
	if k, ok := lwjgl[code]; ok {
		if name, ok := fallbackNames[k]; ok {
			return name
		}
		if k > 32 && k < 127 { // GLFW's printable keys are their ASCII codes
			return string(rune(k))
		}
	}
	return fmt.Sprintf("Key %d", code)
}

var fallbackNames = map[glfw.Key]string{
	glfw.KeySpace: "SPACE", glfw.KeyLeftShift: "LSHIFT", glfw.KeyRightShift: "RSHIFT", glfw.KeyLeftControl: "LCONTROL", glfw.KeyRightControl: "RCONTROL",
	glfw.KeyLeftAlt: "LMENU", glfw.KeyRightAlt: "RMENU", glfw.KeyTab: "TAB", glfw.KeyEnter: "RETURN", glfw.KeyBackspace: "BACK", glfw.KeyEscape: "ESCAPE",
	glfw.KeyUp: "UP", glfw.KeyDown: "DOWN", glfw.KeyLeft: "LEFT", glfw.KeyRight: "RIGHT", glfw.KeyCapsLock: "CAPITAL",
	glfw.KeyF1: "F1", glfw.KeyF2: "F2", glfw.KeyF3: "F3", glfw.KeyF4: "F4", glfw.KeyF5: "F5", glfw.KeyF6: "F6", glfw.KeyF7: "F7", glfw.KeyF8: "F8",
	glfw.KeyF9: "F9", glfw.KeyF10: "F10", glfw.KeyF11: "F11", glfw.KeyF12: "F12", glfw.KeyHome: "HOME", glfw.KeyEnd: "END", glfw.KeyInsert: "INSERT", glfw.KeyDelete: "DELETE",
}
