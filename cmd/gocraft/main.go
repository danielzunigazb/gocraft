// gocraft is Minecraft 1.8.9 rewritten in Go: the game window. The server is the repository's main package.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"

	"gocraft/client"
	"gocraft/proto"
)

func main() {
	home, _ := os.UserHomeDir()
	jar := flag.String("jar", filepath.Join(home, "mc-1.8.9-run", "client.jar"), "your 1.8.9 client jar (textures are read from it)")
	view := flag.Int("view", 0, "view distance in chunks (default: your option)")
	optionsFile := flag.String("options", filepath.Join(home, "mc-1.8.9-run", "game", "options.txt"), "your 1.8.9 options.txt, read on the first run")
	shot := flag.String("screenshot", "", "render in a hidden window, save a PNG here and exit")
	frames := flag.Int("frames", 3, "frames to render before the screenshot")
	demo := flag.Bool("demo", false, "put a few blocks of each kind near the spawn")
	dir := flag.String("world", "", "a 1.8.9 world folder to open (default: a flat world)")
	pos := flag.String("pos", "", "x,y,z of the camera, with -screenshot")
	look := flag.String("look", "", "yaw,pitch in degrees, with -screenshot")
	script := flag.String("script", "", "scripted input before the screenshot, e.g. forward+sprint:40,jump:1,turn=90")
	screen := flag.String("screen", "", "screen to show in the screenshot: inventory, title, worlds or pause")
	saves := flag.String("saves", filepath.Join(home, "mc-1.8.9-run", "game", "saves"), "folder of your worlds, listed by the menu")
	bench := flag.Int("bench", 0, "fly a fixed route for this many seconds, print frame statistics and exit")
	width := flag.Int("width", 0, "window width (default 1280)")
	height := flag.Int("height", 0, "window height (default 720)")
	packs := flag.String("packs", filepath.Join(home, "mc-1.8.9-run", "game", "resourcepacks"), "folder of your resource packs")
	timeOfDay := flag.Int64("time", 0, "time of day in ticks, with -screenshot (6000 noon, 18000 midnight)")
	hold := flag.Int("hold", 0, "id of the item to hold, with -screenshot")
	benchWarm := flag.Int("benchwarm", 0, "seconds at the start of -bench that are not counted (the world is still loading)")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile of the run to this file (go tool pprof)")
	netlog := flag.String("netlog", "", "write every network packet (direction, id, size, time) to this file, to find why a server refuses the client")
	portal := flag.Bool("portal", false, "build and light a Nether portal in front of the camera (with -dim -1, then go through it): a test aid")
	dim := flag.Int("dim", 0, "start in a dimension: -1 the Nether, 1 the End (a test aid)")
	connect := flag.String("connect", "", "join a server at host:port instead of a world")
	name := flag.String("name", "", "the player name in multiplayer (offline mode)")
	hand := flag.String("hand", "", "first-person hand for a screenshot: swing=3,bow=15,eat=10,block")
	mobs := flag.Bool("mobs", false, "a row of every mob in front of the camera, with -screenshot")
	persp := flag.Int("persp", 0, "camera for the screenshot: 0 first person, 1 behind, 2 in front")
	flag.Parse()
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
	}
	if *netlog != "" {
		f, err := os.Create(*netlog)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		t0 := time.Now()
		var mu sync.Mutex
		proto.Trace = func(dir string, id int32, payload []byte) {
			mu.Lock()
			defer mu.Unlock()
			text := ""
			if dir == "<" && (id == 0x02 || id == 0x40 || id == 0x45 || id == 0x3F) || dir == ">" && (id == 0x01 || id == 0x17) {
				text = " " + printable(payload)
			}
			fmt.Fprintf(f, "%6dms %s 0x%02x %-22s %d bytes%s\n", time.Since(t0).Milliseconds(), dir, id, packetName(dir, id), len(payload), text)
		}
	}
	if err := client.Run(client.Options{JarPath: *jar, ViewDistance: *view, Screenshot: *shot, Frames: *frames, Demo: *demo, WorldDir: *dir, Pos: *pos, Look: *look, Script: *script, Screen: *screen, SavesDir: *saves, OptionsFile: *optionsFile, PacksDir: *packs, Time: *timeOfDay, Mobs: *mobs, Hand: *hand, Connect: *connect, Dim: *dim, Portal: *portal, Name: *name, Perspective: *persp, Hold: *hold, Bench: *bench, BenchWarm: *benchWarm, Width: *width, Height: *height}); err != nil {
		log.Fatal(err)
	}
}

// packetName is 1.8's name for a play packet, for reading the -netlog file.
func packetName(dir string, id int32) string {
	in := map[int32]string{0x00: "keep alive", 0x01: "join game", 0x02: "chat", 0x03: "time", 0x05: "spawn position", 0x06: "health", 0x07: "respawn",
		0x08: "position+look", 0x09: "held item", 0x0B: "animation", 0x0C: "spawn player", 0x0F: "spawn mob", 0x12: "velocity", 0x13: "destroy entities",
		0x14: "entity", 0x15: "rel move", 0x16: "look", 0x17: "rel move+look", 0x18: "teleport", 0x19: "head look", 0x1A: "status", 0x1C: "metadata",
		0x1F: "set experience", 0x20: "properties", 0x21: "chunk", 0x22: "multi block", 0x23: "block change", 0x26: "chunk bulk", 0x2B: "game state",
		0x2D: "open window", 0x2F: "set slot", 0x30: "window items", 0x32: "confirm transaction", 0x38: "tab list", 0x39: "abilities", 0x3A: "tab complete",
		0x3B: "scoreboard objective", 0x3E: "teams", 0x3F: "plugin message", 0x40: "DISCONNECT", 0x45: "title", 0x46: "set compression", 0x48: "resource pack"}
	out := map[int32]string{0x00: "keep alive", 0x01: "chat", 0x02: "use entity", 0x03: "player (ground)", 0x04: "position", 0x05: "look", 0x06: "position+look",
		0x07: "dig", 0x08: "place", 0x09: "held item", 0x0A: "swing", 0x0B: "entity action", 0x0F: "confirm transaction", 0x10: "creative slot", 0x15: "client settings",
		0x16: "client status", 0x17: "plugin message", 0x19: "resource pack status"}
	m := in
	if dir == ">" {
		m = out
	}
	if n, ok := m[id]; ok {
		return n
	}
	return "?"
}

// printable keeps the readable part of a packet body, to show chat and kick messages in the log.
func printable(b []byte) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c >= 32 && c < 127 {
			out = append(out, c)
		} else {
			out = append(out, '.')
		}
	}
	return string(out)
}
