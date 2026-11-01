// Package client is the game window: OpenGL rendering of the world, a free camera, and breaking
// and placing blocks. It reads its textures from the player's own 1.8.9 client jar.
package client

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"

	"gocraft/anvil"
	"gocraft/world"
)

func init() { runtime.LockOSThread() } // GLFW and OpenGL belong to the main thread

type Options struct {
	JarPath      string
	PacksDir     string // where resource packs are
	OptionsFile  string // the 1.8.9 options.txt to start from on the first run
	Bench        int    // seconds to fly a fixed route and print frame statistics, then exit
	BenchWarm    int    // seconds at the start of a benchmark that are not counted (the world is still loading)
	SavesDir     string // the folder of worlds the menu lists
	ViewDistance int
	Width        int
	Height       int
	// Screenshot, when set, renders Frames frames in a hidden window, saves the last one and returns.
	Screenshot string
	Frames     int
	Demo       bool // a few blocks of each kind near the spawn
	// WorldDir is a 1.8.9 save folder to open instead of the flat world.
	WorldDir string
	// Pos overrides the camera ("x,y,z"), to frame a screenshot.
	Pos string
	// Look overrides the view ("yaw,pitch" in degrees).
	Look string
	// Time sets the time of day (ticks) and Hold the held block or item id, for screenshots.
	Time        int64
	Hold        int
	Perspective int    // 0 first person, 1 behind, 2 in front, for screenshots
	Connect     string // a server to join at start, "host:port"
	Dim         int    // -1 or 1: start in the Nether or the End, for tests and screenshots
	Portal      bool   // build and light a Nether portal in front of the camera, for screenshots
	Name        string // the player name for it
	Hand        string // sets the first-person hand for a screenshot: "swing=3,bow=15,eat=10,block"
	Mobs        bool   // a row of every kind of mob in front of the camera, for screenshots
	// Script plays inputs through the real physics before a screenshot, as "forward+jump:20,turn=90,wait:10":
	// actions are forward back left right jump sneak sprint, each held for the given ticks.
	Script string
	// Screen opens a screen for the screenshot: "inventory" (creative only).
	Screen string
}

// defaultHotbar is what a new game starts with: block states (id<<4 | metadata).
var defaultHotbar = [9]uint16{1 << 4, 2 << 4, 3 << 4, 4 << 4, 5 << 4, 12 << 4, 17 << 4, 20 << 4, 45 << 4}

const ()

type game struct {
	opt   Options
	win   *glfw.Window
	world *world.World
	res   *resources

	tex                       uint32
	prog, flat                uint32
	uMVP, uFogC, uFogS, uFogE int32
	uColour, uAlpha           int32
	uDyn, uDynCount           int32
	crossVAO, crossVBO        uint32
	meshes                    map[world.ChunkPos]*chunkMesh
	dirty                     map[world.ChunkPos]bool

	pos                        vec3 // the eye
	feet, prevFeet, vel, spawn vec3
	onGround, flying           bool
	allowFly                   bool // a server lets the player fly without being in creative mode
	abilFlags                  byte // the invulnerable (1) and creative (8) abilities the server last gave: echoed back as the game does
	creative                   bool
	sneaking                   bool
	sprinting, collidedH       bool
	inWeb, wasJump             bool
	wasSneak, wasForward       bool
	jumpTicks                  int
	sprintToggle, flyToggle    int
	sprintTicks                int     // ticks until a sprint runs out: it lasts 600 (EntityPlayerSP.sprintingTicksLeft)
	aiSpeed, jumpFactor        float32 // the speed and air factor the last tick ended with: this tick moves with them
	speedsSet                  bool    // false until a tick has ended: a new player has no speed yet
	inv                        [36]slot
	armor                      [4]slot
	cursor                     slot
	cursorX                    itemExtra // the tag of the stack on the mouse in the player's own window on a server
	wui                        winUI     // what the mouse is doing in a window with slots
	health                     float32
	food, regen                int
	fall                       float32
	mining                     bool
	mineAt                     [3]int32
	mineTicksDone              int
	useDelay, breakDelay       int
	screen                     screenKind
	inGame                     bool
	arrows                     []*arrow
	net                        *netState    // the server this game is a client of; nil in single player
	acct                       account      // the Microsoft account, for online-mode servers
	dimn                       dimState     // the Nether, the End and the portals between them (dimension.go)
	snd                        *soundSystem // the sound output and mixer; nil with sound off
	sndState                   soundState
	parts                      *particleSystem // particles: pieces of broken blocks, torch fire, critical sparks
	be                         beState         // chests and furnaces: what they hold
	chat                       []chatLine
	chatInput                  string
	netMsg                     string // the last thing multiplayer had to say: an error, "Connecting..."
	multiAddr, multiName       string
	multiFocus                 int
	chatSkip                   bool
	bowTicks                   int
	hand                       handState // the first-person hand (firstperson.go)
	perspective                int       // 0 first person, 1 behind, 2 in front
	bodyYaw, prevBodyYaw       float32
	pLimbSwing, pLimbAmount    float32
	swingTicks                 int
	mobs                       []*mob
	entityChunks               map[world.ChunkPos]bool
	blocking                   bool
	hurtResist                 int
	effects                    map[int]potionEffect // the potion effects on the player, by id
	statusBars                 hudState
	maxHP, absorption          float32 // the server's maximum health for the player, and its golden hearts
	xpBar                      float32
	xpLevel                    int
	air                        int // the air supply, 300 down to 0 under water
	hurtTime, deathTime        int // ticks since a blow (10 down to 0) and since the player died, for the camera
	lastDamage                 float32
	mobTex                     map[string]uint32
	skyProg, skyVAO            uint32
	worldTime                  int64
	lightTex                   uint32
	lightmap                   [768]uint8
	flicker                    float32
	packList                   []string
	packsDirty                 bool
	exhaustion, saturation     float32
	foodTimer, eating          int
	cont                       containerState
	partial                    float32
	items                      []*itemEntity
	entBlocks, entFlat         entityMesh
	ticks                      int                    // game ticks run, for what expires after a while
	damage                     map[int32]*blockDamage // cracks of blocks being mined, by who mines them
	crackVerts                 []vertex
	crackMesh                  entityMesh
	orbMesh                    entityMesh // experience orbs, rewritten each frame
	orbVerts                   []vertex
	keyHeld                    func(action string) bool     // tests: which keys are down when there is no window
	signs                      map[[3]int32]*[4][]styledRun // the text of the signs, by block
	entMobs                    entityMesh
	debug                      bool
	frames                     frameStats
	btot                       benchTotals // counters summed over a benchmark run
	benchTimes                 []float32
	stream                     *streamer
	edit                       meshData
	visible                    []*chunkMesh
	solidArenas, waterArenas   arenaSet             // where the columns' quads live on the GPU (arena.go)
	camPos                     vec3                 // the camera, for skipping faces turned away from it
	camDir                     vec3                 // where it looks
	skins                      map[string]*skinSlot // the skins fetched or being fetched, by URL
	skinDone                   chan skinResult      // skins that arrived, to put on the GPU
	walkCap, flyCap            float32              // the walk and fly speeds the server allows (0.1 and 0.05 by default)
	listForced                 bool                 // a screenshot shows the player list without Tab
	titles                     titleState           // the title and action bar a server wrote
	entGroups                  map[string][]vertex  // the vertices of this frame's entities, kept to be reused
	swin                       *serverWindow        // the window a server opened, if one is
	vis                        visScratch           // the cave-culling traversal
	stats                      drawStats
	opts                       options
	rebinding                  string
	optionsFrom                screenKind
	dragging                   string
	lastVSync, vsyncApplied    bool
	autosave                   int
	save                       *anvil.Save
	worldList                  []worldEntry
	worldSel                   int
	glyphs                     [256]int
	uniTex                     map[int]uint32 // unicode font pages on the GPU, by page
	uiClick                    bool
	saves                      string
	invScroll                  int
	creativeList               []slot
	cx, cy                     float64
	lastSpace                  float64
	yaw, pitch                 float32
	sel                        int
	lastX                      float64
	lastY                      float64
	haveCursor                 bool
	fb                         [2]int
	hud                        hud
}

func Run(opt Options) error {
	if opt.Width == 0 {
		opt.Width, opt.Height = 1280, 720
	}
	opts := loadOptions(opt.OptionsFile)
	res, err := loadResources(opt.JarPath, packPaths(opts.Packs, opt.PacksDir))
	if err != nil {
		return err
	}
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("glfw: %w", err)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	if opt.Screenshot != "" {
		glfw.WindowHint(glfw.Visible, glfw.False)
	}
	win, err := glfw.CreateWindow(opt.Width, opt.Height, "gocraft 1.8.9", nil, nil)
	if err != nil {
		return fmt.Errorf("window: %w", err)
	}
	win.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		return err
	}

	g := &game{opt: opt, win: win, world: world.New(), res: res, creative: true, health: maxHealth, food: maxFood, saturation: 5,
		meshes: map[world.ChunkPos]*chunkMesh{}, dirty: map[world.ChunkPos]bool{}}
	g.world.SetLightRules(res.light)
	g.pos = vec3{0.5, world.SpawnY + 1.62, 0.5}
	g.saves = opt.SavesDir
	g.stream = newStreamer()
	g.opts = opts
	if soundWanted(opt, *noSound, os.Getenv("GOCRAFT_NO_AUDIO")) { // a screenshot or a benchmark makes no noise
		g.snd = newSoundSystem(opt.JarPath, func(c string) float32 { return g.opts.soundVolume(c) })
		defer g.snd.close()
	}
	if err := g.initGL(); err != nil {
		return err
	}
	defer g.closeWorld()
	switch {
	case opt.Connect != "":
		name := opt.Name
		if name == "" {
			name = opts.PlayerName
		}
		if name == "" {
			name = "Player"
		}
		if err := g.connect(opt.Connect, name); err != nil {
			return err
		}
		if opt.Screenshot != "" { // a screenshot shows the server's world, so wait for it to arrive
			for end := time.Now().Add(4 * time.Second); time.Now().Before(end) && g.net != nil; time.Sleep(10 * time.Millisecond) {
				g.pumpNet()
			}
		}
	case opt.WorldDir != "":
		if err := g.loadWorld(opt.WorldDir); err != nil {
			return err
		}
	case opt.Demo || (opt.Screenshot != "" && opt.Screen == ""):
		g.inGame = true // the flat test world
	default:
		g.screen = screenTitle // no world named: start at the title screen, as the game does
	}
	if opt.Demo {
		g.demo()
	}
	if opt.Portal {
		g.demoPortal(opt.Dim)
	} else if opt.Dim != 0 {
		g.debugEnter(opt.Dim)
	}
	if opt.Pos != "" {
		var x, y, z float32
		if _, err := fmt.Sscanf(opt.Pos, "%f,%f,%f", &x, &y, &z); err != nil {
			return fmt.Errorf("-pos wants x,y,z: %w", err)
		}
		g.pos = vec3{float64(x), float64(y), float64(z)}
	}
	if opt.Look != "" {
		var yaw, pitch float32
		if _, err := fmt.Sscanf(opt.Look, "%f,%f", &yaw, &pitch); err != nil {
			return fmt.Errorf("-look wants yaw,pitch: %w", err)
		}
		g.yaw, g.pitch = yaw*math.Pi/180, pitch*math.Pi/180
	}
	if opt.Screenshot == "" {
		g.applyVSync()
		if g.inGame && opt.Bench == 0 { // a benchmark must not grab the mouse
			win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		}
		win.SetCursorPosCallback(g.onCursor)
		win.SetMouseButtonCallback(g.onMouse)
		win.SetKeyCallback(g.onKey)
		win.SetScrollCallback(g.onScroll)
		win.SetCharCallback(g.onCharAny)
	}
	return g.loop()
}

func (g *game) initGL() error {
	var err error
	if g.prog, err = program(worldVS, worldFS); err != nil {
		return err
	}
	if g.flat, err = program(flatVS, flatFS); err != nil {
		return err
	}
	g.uMVP, g.uFogC, g.uFogS, g.uFogE = uniform(g.prog, "mvp"), uniform(g.prog, "fogColor"), uniform(g.prog, "fogStart"), uniform(g.prog, "fogEnd")
	g.uColour = uniform(g.flat, "colour")
	g.uAlpha = uniform(g.prog, "alpha")
	g.uDyn, g.uDynCount = uniform(g.prog, "dyn"), uniform(g.prog, "dynCount")
	gl.UseProgram(g.prog)
	gl.Uniform1i(uniform(g.prog, "atlas"), 0)
	gl.Uniform1i(uniform(g.prog, "lightmap"), 1)

	g.tex = uploadTexture(g.res.atlas.img)

	gl.GenVertexArrays(1, &g.crossVAO)
	gl.GenBuffers(1, &g.crossVBO)
	gl.BindVertexArray(g.crossVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.crossVBO)
	gl.BufferData(gl.ARRAY_BUFFER, 16*4, nil, gl.DYNAMIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 8, 0)

	initQuadIndices()
	gl.Enable(gl.DEPTH_TEST)
	gl.Enable(gl.CULL_FACE) // faces turned away are never drawn: roughly half the work in leaves
	gl.CullFace(gl.BACK)
	gl.FrontFace(gl.CCW)
	gl.GenTextures(1, &g.lightTex)
	gl.ActiveTexture(gl.TEXTURE1)
	gl.BindTexture(gl.TEXTURE_2D, g.lightTex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	var lm [768]uint8 // a plain array: cgo will not take a pointer into the game struct, which holds pointers
	buildLightmap(1, 0, 0, &lm)
	g.lightmap = lm
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGB, 16, 16, 0, gl.RGB, gl.UNSIGNED_BYTE, gl.Ptr(&lm[0]))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.ClearColor(0.75, 0.85, 1, 1)
	return g.initHUD()
}

// demo puts one block of each kind in a row, a short wall and a pillar, so a screenshot shows every texture.
func (g *game) demo() {
	for i, st := range defaultHotbar {
		g.setBlock(int32(i*2-8), 4, 4, st)
	}
	g.setBlock(-5, 4, 1, 50<<4|5) // a torch and a glowstone lamp, to see light spread
	g.setBlock(5, 4, 1, 89<<4)
	for x := int32(-3); x <= 3; x++ {
		for y := int32(4); y < 7; y++ {
			g.setBlock(x, y, 9, 4<<4)
		}
	}
	g.pos = vec3{0.5, 6.5, -5}
	g.yaw, g.pitch = 0, 0.25
}

func (g *game) setBlock(x, y, z int32, state uint16) { g.changeBlock(x, y, z, state, true) }

// changeBlock sets a block; with fx a block that breaks or is placed shows its pieces and is heard. A server's
// block changes do not: the server sends those as Effect 2001 and Sound Effect, as the game has it.
func (g *game) changeBlock(x, y, z int32, state uint16, fx bool) {
	if !world.InRange(y) {
		return
	}
	if state == 0 {
		defer g.portalBroken([3]int32{x, y, z}) // a portal whose frame was broken goes out
	}
	if fx {
		g.blockEffects(x, y, z, g.world.Block(x, y, z), state) // particles (and sound) for a block that breaks or is placed
	}
	if id := state >> 4; g.signs != nil && id != idStandingSign && id != idWallSign { // a sign that is gone takes its text along
		delete(g.signs, [3]int32{x, y, z})
	}
	// Changing a block also changes the light around it: every column the light touched is meshed again.
	for _, c := range g.world.SetBlockLit(x, y, z, state) {
		g.dirty[c] = true
	}
	cp := world.ChunkPos{X: x >> 4, Z: z >> 4}
	// A block on a column's edge changes which faces its neighbour shows.
	if x&15 == 0 {
		g.dirty[world.ChunkPos{X: cp.X - 1, Z: cp.Z}] = true
	}
	if x&15 == 15 {
		g.dirty[world.ChunkPos{X: cp.X + 1, Z: cp.Z}] = true
	}
	if z&15 == 0 {
		g.dirty[world.ChunkPos{X: cp.X, Z: cp.Z - 1}] = true
	}
	if z&15 == 15 {
		g.dirty[world.ChunkPos{X: cp.X, Z: cp.Z + 1}] = true
	}
}

func (g *game) onCursor(_ *glfw.Window, x, y float64) {
	g.cx, g.cy = x, y
	if g.inWindow() {
		g.winMove()
	}
	if g.screen != screenNone {
		return
	}
	if g.haveCursor {
		sens := float64(g.opts.degPerPixel()) * math.Pi / 180
		dy := y - g.lastY
		if g.opts.InvertY {
			dy = -dy
		}
		g.yaw += float32((x - g.lastX) * sens)
		g.pitch += float32(dy * sens)
		const lim = 89.9 * math.Pi / 180
		g.pitch = float32(math.Max(-lim, math.Min(lim, float64(g.pitch))))
	}
	g.lastX, g.lastY, g.haveCursor = x, y, true
}

func (g *game) onKey(w *glfw.Window, key glfw.Key, _ int, action glfw.Action, _ glfw.ModifierKey) {
	if g.newWorldKey(key, action) {
		return
	}
	if g.typing() && (action == glfw.Press || action == glfw.Repeat && key == glfw.KeyBackspace) {
		g.typingKey(key)
		return
	}
	if action != glfw.Press {
		return
	}
	if key == glfw.KeyEscape {
		switch g.screen {
		case screenNone:
			if g.inGame {
				g.openScreen(screenPause)
			} else {
				w.SetShouldClose(true)
			}
		case screenContainer:
			g.closeContainer()
		case screenServerWin:
			g.closeServerWindow(true)
		case screenInventory, screenPause:
			g.closeScreen()
		case screenWorlds:
			g.screen = screenTitle
		case screenOptions:
			g.leaveOptions()
		case screenControls:
			g.rebinding = ""
			g.screen = screenOptions
		case screenPacks:
			g.leavePacks()
		}
		return
	}
	if g.screen.isMenu() {
		return
	}
	if g.rebinding != "" {
		g.bindKey(key)
		return
	}
	if g.net != nil && g.screen == screenNone && (key == glfw.KeyT || key == glfw.KeySlash) {
		if key == glfw.KeySlash {
			g.openChat("/")
		} else {
			g.openChat("")
		}
		g.chatSkip = true // the T that opened the chat is not also typed into it
		return
	}
	if code, ok := lwjglOf[key]; ok && code == g.opts.Keys["inventory"] {
		switch {
		case g.screen == screenContainer:
			g.closeContainer()
		case g.screen == screenServerWin:
			g.closeServerWindow(true)
		case g.screen != screenNone:
			g.closeScreen()
		case g.creative:
			g.openScreen(screenInventory)
		default:
			g.openContainer(contPlayer)
		}
		return
	}
	if code, ok := lwjglOf[key]; ok && code == g.opts.Keys["drop"] && g.screen == screenNone && g.inGame {
		g.dropHeld(g.win != nil && g.win.GetKey(glfw.KeyLeftControl) == glfw.Press)
		return
	}
	if g.screen == screenInventory && key >= glfw.Key1 && key <= glfw.Key9 {
		// Over an item, a number key puts it in that hotbar slot, as in the game.
		if kind, i := g.slotAt(); kind == 'g' {
			g.inv[key-glfw.Key1] = creativeStack(g.creativeShown()[i])
		}
		return
	}
	if g.inWindow() { // over a slot the number keys swap with the hotbar, Q drops, the pick key clones
		if g.winKey(key) || key >= glfw.Key1 && key <= glfw.Key9 { // a number key never changes the held slot here
			return
		}
	}
	if code, ok := lwjglOf[key]; ok && code == g.opts.Keys["togglePerspective"] && g.screen == screenNone {
		g.perspective = (g.perspective + 1) % 3
	}
	if key == glfw.KeyF3 {
		g.debug = !g.debug
	}
	if key == glfw.KeyF7 && g.screen == screenNone {
		g.spawnBot()
	}
	if key == glfw.KeyF8 && g.screen == screenNone && g.net == nil && g.inGame { // a test aid: Overworld, Nether, End
		g.debugCycleDim()
	}
	if key == glfw.KeyF && g.canFly() {
		g.flying = !g.flying
	}
	if key >= glfw.Key1 && key <= glfw.Key9 {
		g.sel = int(key - glfw.Key1)
	}
}

func (g *game) onScroll(_ *glfw.Window, _, dy float64) {
	if g.screen == screenInventory {
		if dy > 0 {
			g.invScroll = max(g.invScroll-1, 0)
		} else if dy < 0 {
			g.invScroll = min(g.invScroll+1, g.maxScroll())
		}
		return
	}
	if dy != 0 {
		step := 1
		if dy > 0 {
			step = -1
		}
		g.sel = (g.sel + step + 9) % 9
	}
}

func (g *game) onMouse(_ *glfw.Window, b glfw.MouseButton, action glfw.Action, _ glfw.ModifierKey) {
	if g.inWindow() && g.rebinding == "" { // a window with slots takes the button going down and going up
		if action == glfw.Press || action == glfw.Release {
			g.winMouse(b, action == glfw.Press)
		}
		return
	}
	if action != glfw.Press {
		return
	}
	if g.rebinding != "" && g.screen == screenControls {
		g.opts.Keys[g.rebinding] = mouseCode(b)
		g.rebinding = ""
		return
	}
	if g.screen.isMenu() || g.screen == screenDead {
		g.uiClick = b == glfw.MouseButtonLeft
		return
	}
	if g.screen == screenInventory {
		if b == glfw.MouseButtonLeft {
			g.clickInventory()
		}
		return
	}
	switch code := mouseCode(b); code {
	case g.opts.Keys["attack"]:
		g.swingTicks = 6
		g.sendSwing()
		if g.attackClick() { // a mob in reach takes the click, as in the game
			return
		}
		if g.creative {
			g.breakDelay = breakDelayTicks
			g.breakBlock()
		}
	case g.opts.Keys["use"]:
		g.useDelay = useDelayTicks
		g.useBlock()
	case g.opts.Keys["pickItem"]:
		g.pickBlock()
	}
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (g *game) draw() {
	w, h := g.win.GetFramebufferSize()
	g.fb = [2]int{w, h}
	gl.Viewport(0, 0, int32(w), int32(h))
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	if !g.inGame {
		g.drawMenus()
		return
	}

	camPos, camDir := g.camera()
	g.camPos, g.camDir = camPos, camDir
	g.drawSky(g.opts.FOV*math.Pi/180, w, h, camDir)
	proj := perspective(g.opts.FOV*math.Pi/180, float32(w)/float32(h), 0.05, 1000)
	view := lookAt(camPos, camPos.add(camDir), g.rolledUp(camDir))
	mvp := mul(proj, view)

	gl.UseProgram(g.prog)
	gl.UniformMatrix4fv(g.uMVP, 1, false, &mvp[0])
	gl.Uniform1f(g.uAlpha, 1)
	g.updateLighting()
	far := float32(g.viewDist() * 16)
	fogS, fogE := g.dimFogRange(far)
	gl.Uniform1f(g.uFogS, fogS)
	gl.Uniform1f(g.uFogE, fogE)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, g.tex)
	g.drawChunks(&mvp)
	g.drawFishLines(&mvp)
	g.drawParticles()
	g.drawOrbs()

	g.drawOutline(&mvp)
	g.drawHand(w, h)
	g.drawPortalOverlay(w, h)
	g.drawTags(&mvp, w, h)
	g.drawHUD(w, h)
	g.drawSidebar(w, h)
	g.drawBossBar(w, h)
	g.drawChat(w, h)
	g.drawTabList(w, h)
	g.drawTitles(w, h)
	if g.screen == screenServerWin { // over the chat, as the game's screens are
		g.drawServerWindow(w, h)
	}
	g.drawDeathScreen(w, h)
	g.drawMenus()
}

func (g *game) loop() error {
	if g.opt.Screenshot != "" {
		g.feet = g.freeSpot(g.pos.sub(vec3{0, eyeHeight, 0}))
		g.prevFeet, g.spawn = g.feet, g.feet
		if err := g.playScript(g.opt.Script); err != nil {
			return err
		}
		g.pos = g.feet.add(vec3{0, eyeHeight, 0})
		switch g.opt.Screen {
		case "title":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.screen = screenTitle
		case "worlds":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.worldList, g.screen = scanWorlds(g.saves), screenWorlds
		case "newworld":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.openNewWorld()
		case "pause":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.screen = screenPause
		case "netchat":
			g.netDemo(false)
		case "netlist":
			g.netDemo(false)
			g.listDemo()
		case "netheld":
			g.netDemo(false)
			g.profileDemo()
		case "netfish":
			g.netDemo(false)
			g.fishDemo()
			g.heldDemo()
		case "netmenu":
			g.netDemo(true)
		case "netitems":
			g.netDemo(false)
			g.itemsDemo()
		case "netboss":
			g.netDemo(false)
			g.net.addEntity(&netEntity{eid: 90, kind: mobWither, pos: vec3{0, 30, 20}, name: legacyRuns("§d§lEvento de Halloween §7- §e2:35", defaultStyle), health: 210, healthSet: true})
		case "netmobs":
			g.netDemo(false)
			g.mobsDemo()
		case "netparticles":
			g.netDemo(false)
			g.particlesDemo()
		case "cracks":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.cracksDemo()
		case "signs":
			g.netDemo(false)
			g.signsDemo()
		case "chests", "chest", "bigchest", "furnace":
			g.beScreenshot(g.opt.Screen)
		case "sounds":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.optionsFrom, g.screen, soundPage = screenTitle, screenOptions, true
			g.opts.SoundVolume = map[string]float32{"master": 0.235, "music": 0, "weather": 0}
		case "particles":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.demoParticles()
		case "multi":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.multiAddr, g.multiName, g.screen = "play.example.net:25565", "Player", screenMulti
		case "packs":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.optionsFrom, g.screen = screenTitle, screenPacks
			g.packList = listPacks(g.opt.PacksDir)
		case "options", "controls":
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.optionsFrom, g.screen = screenTitle, screenOptions
			if g.opt.Screen == "controls" {
				g.screen = screenControls
			}
		}
		if g.opt.Time != 0 {
			g.worldTime = g.opt.Time
		}
		g.perspective = g.opt.Perspective
		if g.opt.Mobs {
			order := []string{"Zombie", "Creeper", "Skeleton", "Pig", "Cow", "Sheep", "Chicken", "PvPBot", "Spider", "CaveSpider", "Slime", "Rabbit", "Villager", "Witch", "Squid", "Bat"}
			f := look(g.yaw, 0)
			right := vec3{-f[2], 0, f[0]}
			for i, id := range order {
				at := g.pos.add(f.scale(6 + float64(i/9)*3.5)).add(right.scale(float64(i%9-4) * 1.6))
				y := g.feet[1]
				if sy, ok := g.surfaceAt(int32(math.Floor(float64(at[0]))), int32(math.Floor(float64(at[2])))); ok {
					y = sy
				}
				m := g.newMob(kinds[id], vec3{at[0], y, at[2]}, g.yawDeg()+180)
				m.colour = i * 2
				m.limbSwing, m.limbAmount = float32(i), 0.6
				g.mobs = append(g.mobs, m)
			}
			for i := 0; i < 3; i++ { // arrows in flight, to see them drawn
				a := &arrow{pos: g.pos.add(f.scale(3)).add(right.scale(float64(i)*0.8 - 1)).add(vec3{0, 0.2, 0}), vel: right.scale(1.5).add(vec3{0, -0.15, 0})} // across the view, to show the shaft from the side
				a.prev = a.pos
				g.arrows = append(g.arrows, a)
			}
		}
		if g.opt.Hold != 0 {
			g.inv[0] = slot{ID: uint16(g.opt.Hold), N: 1}
			g.sel = 0
		}
		g.handScreenshot(g.opt.Hand)
		if g.opt.Screen == "survival" || g.opt.Screen == "table" {
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.creative = false
			g.inv[0], g.inv[1], g.inv[2] = slot{ID: 278, Dmg: 100, N: 1}, slot{ID: 280, N: 17}, blockSlot(5<<4, 64)
			g.inv[3], g.inv[4], g.inv[9] = slot{ID: 297, N: 5}, slot{ID: 264, N: 3}, blockSlot(4<<4, 33)
			g.armor[0] = slot{ID: 310, N: 1}
			kind := contPlayer
			if g.opt.Screen == "table" {
				kind = contTable
				g.cont.grid[0], g.cont.grid[3], g.cont.grid[6] = slot{ID: 264, N: 1}, slot{ID: 264, N: 1}, slot{ID: 280, N: 1}
			}
			g.cont.kind = kind
			if kind == contTable {
				g.recompute()
			}
			g.screen, g.cx, g.cy = screenContainer, 640, 360
			g.cursor = slot{ID: 265, N: 7}
		}
		if g.opt.Screen == "drag" { // a stack of 40 painted across four slots, one of them already holding some
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.creative = false
			g.inv[0], g.inv[1], g.inv[2] = slot{ID: 276, N: 1}, slot{ID: 264, N: 7}, slot{ID: 4, N: 3}
			g.inv[10] = slot{ID: 4, N: 5}
			g.openContainer(contPlayer)
			g.cursor = slot{ID: 4, N: 40}
			g.wui.dragging, g.wui.dragSlots = true, []int{10, 11, 12, 13}
			g.screen, g.cx, g.cy = screenContainer, 700, 330
		}
		if g.opt.Screen == "inventory" {
			g.fb = [2]int{g.opt.Width, g.opt.Height}
			g.screen = screenInventory
			sc, left, top := g.panel() // the cursor rests on the third item of the second row
			g.cx, g.cy = float64(left+(invGridX+2*slotSize+8)*sc), float64(top+(invGridY+slotSize+8)*sc)
		}
		n := max(g.opt.Frames, 1)
		for i := 0; i < n; i++ {
			if g.inGame {
				g.waitMeshes() // a screenshot shows every column in view, not a half-loaded world
			}
			g.draw()
			g.win.SwapBuffers()
			glfw.PollEvents()
		}
		return g.saveScreenshot(g.opt.Screenshot)
	}
	if g.inGame && g.save == nil { // the flat world; a loaded one set its feet in applySave
		g.feet = g.freeSpot(g.pos.sub(vec3{0, eyeHeight, 0}))
		g.prevFeet, g.spawn = g.feet, g.feet
	}
	if g.opt.Bench > 0 && g.inGame {
		g.flying = true // the benchmark flies a fixed route
	}
	start := glfw.GetTime()
	last := start
	acc := 0.0
	warmed := g.opt.BenchWarm <= 0
	if g.opt.Bench > 0 && warmed { // no warm-up: count from the first frame
		resetTimers(true)
	}
	for !g.win.ShouldClose() {
		now := glfw.GetTime()
		frame := now - last
		if g.net != nil { // a server hears one movement packet per tick: a slow frame is not caught up in a burst
			acc += math.Min(frame, 1.0/tickRate)
		} else {
			acc += math.Min(frame, 0.25)
		}
		last = now
		g.frames.add(float32(frame))
		if g.opt.Bench > 0 {
			if !warmed && now-start >= float64(g.opt.BenchWarm) { // the loading is over: count from here
				warmed = true
				g.benchTimes, g.frames, g.btot = nil, frameStats{}, benchTotals{}
				resetTimers(true)
			}
			if g.inGame {
				g.yaw += float32(frame) * 0.9
				g.benchTimes = append(g.benchTimes, float32(frame))
				g.frames.record(float32(frame) * 1000)
				g.btot.frame()
			}
			if now-start >= float64(g.opt.Bench) {
				g.printBench()
				break
			}
		}
		for acc >= 1.0/tickRate {
			g.pumpLight()                                                                    // the game takes in what the server sent just before the tick that moves things on
			if g.inGame && (!g.screen.isMenu() || g.net != nil && g.screen == screenPause) { // a menu pauses the game, as in single player; a server's does not wait
				g.tick(g.input())
				if g.autosave++; g.autosave >= 60*tickRate {
					g.autosave = 0
					g.saveWorld()
				}
			}
			acc -= 1.0 / tickRate
		}
		if g.net != nil && time.Since(g.net.pumped) > 100*time.Millisecond { // no tick is running (a menu): do not let packets pile up
			g.pumpLight()
		}
		g.pumpHeavy(4 * time.Millisecond)
		a := float32(acc * tickRate)
		g.partial = a
		if g.inGame {
			g.pos = g.prevFeet.add(g.feet.sub(g.prevFeet).scale(float64(a))).add(vec3{0, float64(g.eyeOffset()), 0})
		}
		rc = renderCounters{}
		t0 := glfw.GetTime()
		if g.inGame {
			g.updateMeshes()
		}
		t1 := glfw.GetTime()
		g.draw()
		g.frames.updateMs, g.frames.drawMs = float32(t1-t0)*1000, float32(glfw.GetTime()-t1)*1000
		g.win.SwapBuffers()
		glfw.PollEvents()
	}
	return nil
}

func (g *game) saveScreenshot(path string) error {
	w, h := g.fb[0], g.fb[1]
	pix := make([]byte, w*h*4)
	gl.ReadBuffer(gl.BACK)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pix))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ { // OpenGL's rows run bottom to top
		copy(img.Pix[y*w*4:(y+1)*w*4], pix[(h-1-y)*w*4:(h-y)*w*4])
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// playScript feeds scripted input to the same tick the interactive loop runs, for tests and screenshots.
func (g *game) playScript(script string) error {
	for _, step := range strings.Split(script, ",") {
		step = strings.TrimSpace(step)
		if step == "" {
			continue
		}
		if deg, ok := strings.CutPrefix(step, "turn="); ok {
			var d float32
			if _, err := fmt.Sscanf(deg, "%f", &d); err != nil {
				return fmt.Errorf("script %q: %w", step, err)
			}
			g.yaw += d * math.Pi / 180
			continue
		}
		acts, ticks, _ := strings.Cut(step, ":")
		n := 1
		if ticks != "" {
			if _, err := fmt.Sscanf(ticks, "%d", &n); err != nil {
				return fmt.Errorf("script %q: %w", step, err)
			}
		}
		var in input
		for _, a := range strings.Split(acts, "+") {
			switch a {
			case "forward":
				in.forward++
			case "back":
				in.forward--
			case "left":
				in.strafe++
			case "right":
				in.strafe--
			case "jump":
				in.jump = true
			case "sneak":
				in.sneak = true
			case "sprint":
				in.sprint = true
			case "wait":
			default:
				return fmt.Errorf("script %q: unknown action %q", step, a)
			}
		}
		for i := 0; i < n; i++ {
			g.tick(in)
		}
		log.Printf("after %s: feet %.2f %.2f %.2f onGround %v", step, g.feet[0], g.feet[1], g.feet[2], g.onGround)
	}
	return nil
}

// input is what the player is pressing: nothing while a screen is open.
func (g *game) input() input {
	if g.opt.Bench > 0 {
		return input{forward: 1, sprint: true}
	}
	if g.screen != screenNone || g.net != nil && g.health <= 0 { // dead on a server: nothing until it respawns us
		return input{}
	}
	return g.readInput()
}

// applySave makes a save the current world: its chunks, game mode, hotbar, health and spawn.
func (g *game) applySave(save *anvil.Save) {
	g.save = save
	g.speedsSet, g.sprintTicks = false, 0
	ov := overlayFor(save.Dir)
	g.loadBlockEntities(ov)
	g.world = world.NewFrom(func(cp world.ChunkPos) *world.Chunk {
		if c, err := ov.loadChunk(cp); err != nil {
			logf("column %v of the saved edits: %v", cp, err)
		} else if c != nil {
			return c // what the player changed wins over the original world
		}
		c, err := save.Chunk(cp)
		if err != nil {
			logf("chunk %v: %v", cp, err)
		}
		return c
	})
	g.world.SetLightRules(g.res.light)
	lv := save.Level
	g.creative = lv.GameType == 1
	g.inv, g.armor, g.cursor = [36]slot{}, [4]slot{}, slot{}
	for i, st := range defaultHotbar {
		g.inv[i] = blockSlot(st, 1)
	}
	g.health, g.food = maxHealth, maxFood
	if !g.creative {
		g.inv = [36]slot{} // survival starts with what the player carries
		for _, it := range lv.Inventory {
			if st := (slot{it.ID, it.Dmg, it.Count}); it.Slot < 36 {
				g.inv[it.Slot] = st
			} else {
				g.armor[it.Slot-100] = st
			}
		}
		if lv.Health > 0 {
			g.health = lv.Health
		}
		if lv.Food > 0 {
			g.food = int(lv.Food)
		}
	}
	g.yaw, g.pitch, g.sel = 0, 0, 0
	g.worldTime = lv.DayTime
	g.vel, g.flying, g.onGround, g.fall = vec3{}, false, false, 0
	g.pos = vec3{float64(lv.SpawnX) + 0.5, float64(lv.SpawnY) + eyeHeight, float64(lv.SpawnZ) + 0.5}
	if lv.HasPlayer {
		p := lv.PlayerPos
		g.pos = vec3{p[0], p[1] + eyeHeight, p[2]}
		g.yaw, g.pitch = lv.PlayerYaw*math.Pi/180, lv.PlayerPitch*math.Pi/180
	} else if y, ok := g.surfaceAt(int32(math.Floor(float64(g.pos[0]))), int32(math.Floor(float64(g.pos[2])))); ok {
		g.pos[1] = y + eyeHeight // the spawn in level.dat may lie underground
	}
	g.feet = g.freeSpot(g.pos.sub(vec3{0, eyeHeight, 0}))
	g.spawn = g.feet
	if p, ok := ov.loadPlayer(); ok && g.opt.Bench == 0 { // pick up where the player left off
		g.feet, g.yaw, g.pitch, g.sel = vec3(p.Pos), p.Yaw, p.Pitch, p.Selected
		g.health, g.food, g.flying = p.Health, p.Food, p.Flying && g.creative
		if p.Time > 0 {
			g.worldTime = p.Time
		}
		g.inv, g.armor = p.inventory()
		g.restoreDim(p.Dim)
		g.feet = g.freeSpot(g.feet)
		g.pos = g.feet.add(vec3{0, eyeHeight, 0})
	}
	g.prevFeet = g.feet
	log.Printf("opened %q at %.1f %.1f %.1f", lv.Name, g.pos[0], g.pos[1], g.pos[2])
}

// loadWorld opens a save folder and starts playing it.
func (g *game) loadWorld(dir string) error {
	if isGeneratedWorld(dir) {
		return g.loadGenerated(dir)
	}
	save, err := anvil.Open(dir)
	if err != nil {
		return err
	}
	g.closeWorld()
	g.applySave(save)
	g.inGame = true
	if g.win != nil && g.opt.Screenshot == "" {
		g.closeScreen()
	}
	return nil
}

// closeWorld leaves the current world: its meshes and its save file go.
func (g *game) closeWorld() {
	g.leaveServer()
	if g.screen == screenContainer {
		g.closeContainer()
	}
	g.saveWorld()
	g.dimReset()
	for cp, m := range g.meshes {
		m.free()
		delete(g.meshes, cp)
	}
	g.dirty = map[world.ChunkPos]bool{}
	if g.save != nil {
		g.save.Close()
		g.save = nil
	}
	g.inGame = false
}

func logf(format string, args ...any) { log.Printf(format, args...) }

// viewDist is the render distance in chunks: -view on the command line, else the option.
func (g *game) viewDist() int {
	if g.opt.ViewDistance > 0 {
		return g.opt.ViewDistance
	}
	return g.opts.RenderDistance
}

func (g *game) applyVSync() {
	if g.win == nil || g.opt.Screenshot != "" {
		return
	}
	if g.vsyncApplied && g.lastVSync == g.opts.VSync {
		return
	}
	if g.opts.VSync {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
	}
	g.lastVSync, g.vsyncApplied = g.opts.VSync, true
}

// bindKey is the key pressed while the Controls screen waits to rebind an action.
func (g *game) bindKey(key glfw.Key) {
	if key == glfw.KeyEscape {
		g.opts.Keys[g.rebinding] = 0
	} else if code, ok := lwjglOf[key]; ok {
		g.opts.Keys[g.rebinding] = code
	} else {
		return // a key 1.8 has no code for: keep waiting
	}
	g.rebinding = ""
}

// keyDown reports whether the key or mouse button bound to an action is held.
func (g *game) keyDown(action string) bool {
	if g.win == nil {
		return g.keyHeld != nil && g.keyHeld(action) // a test holds keys without a window
	}
	code := g.opts.Keys[action]
	if isMouseCode(code) {
		return g.win.GetMouseButton(glfw.MouseButton(code+100)) == glfw.Press
	}
	if k, ok := lwjgl[code]; ok {
		return g.win.GetKey(k) == glfw.Press
	}
	return false
}

// scaleFor is the GUI scale: the option, or the automatic one when it is 0 or does not fit.
func (g *game) scaleFor(w, h int) int {
	auto := autoGuiScale(w, h)
	if g.opts.GuiScale == 0 {
		return auto
	}
	return min(g.opts.GuiScale, auto)
}

// packPaths are the files of the enabled packs, lowest priority first.
func packPaths(names []string, dir string) []string {
	var out []string
	for _, n := range names {
		out = append(out, filepath.Join(dir, n))
	}
	return out
}

// reloadResources reads the jar and the enabled packs again and puts the result on screen: textures
// are uploaded anew and every column is meshed again with the new picture.
func (g *game) reloadResources() error {
	res, err := loadResources(g.opt.JarPath, packPaths(g.opts.Packs, g.opt.PacksDir))
	if err != nil {
		return err
	}
	g.res = res
	g.deleteTextures()
	g.tex = uploadTexture(g.res.atlas.img)
	g.hud.uploadAll(g)
	g.creativeList = nil
	if g.stream != nil {
		g.remeshAll()
	}
	return nil
}

func (g *game) deleteTextures() {
	h := &g.hud
	for _, t := range []*uint32{&g.tex, &h.widgets, &h.icons, &h.tabItems, &h.font, &h.dirt, &h.logo, &h.itemTex, &h.invTex, &h.tableTex} {
		if *t != 0 {
			gl.DeleteTextures(1, t)
			*t = 0
		}
	}
}

// camera is where the view is taken from and which way it looks: from the eyes, or from four
// blocks behind or in front of the player, pulled in where a block is in the way.
func (g *game) camera() (pos, dir vec3) {
	dir = look(g.yaw, g.pitch)
	if g.perspective == 0 {
		return g.pos, dir
	}
	away := dir.scale(-1)
	if g.perspective == 2 {
		away = dir
		dir = dir.scale(-1)
	}
	dist := float32(4)
	if h, ok := g.rayTrace(g.pos, g.pos.add(away.scale(float64(dist)))); ok {
		dist = max(float32(math.Sqrt(float64(h.at.sub(g.pos).dot(h.at.sub(g.pos)))))-0.2, 0.3)
	}
	return g.pos.add(away.scale(float64(dist))), dir
}

// tickBody keeps what the player's model needs: the walking cycle, which way the body faces, and
// the swing of the arm.
func (g *game) tickBody() {
	d := float32(math.Hypot(float64(g.feet[0]-g.prevFeet[0]), float64(g.feet[2]-g.prevFeet[2])))
	step := min(d*4, 1)
	g.pLimbAmount += (step - g.pLimbAmount) * 0.4
	g.pLimbSwing += g.pLimbAmount
	g.prevBodyYaw = g.bodyYaw
	yaw := g.yawDeg()
	if d > 0.0025 {
		g.bodyYaw = turnToward(g.bodyYaw, yaw, 25)
	}
	if diff := float32(math.Mod(float64(yaw-g.bodyYaw)+540, 360)) - 180; diff > 75 {
		g.bodyYaw = yaw - 75
	} else if diff < -75 {
		g.bodyYaw = yaw + 75
	}
	if g.swingTicks > 0 {
		g.swingTicks--
	}
}
