// Command explorer is a first-person demo of voxellib on the app shell. It
// starts in zombie defense: a private server (single player) or a LAN game
// where zombies keep coming and TACZ guns and ammunition lie around to be
// picked up. -sandbox (or -world, -map, -bench, -autofly) explores the
// world instead: reference-like terrain, walking, flying and swimming,
// block breaking and placing, animals, every gun.
//
//	./run.sh                                   # build and run (repo root)
//	go run ./cmd/explorer                      # generated world saved to saves/demo
//	go run ./cmd/explorer -gen flat -world ''  # flat test world, not saved
//	go run ./cmd/explorer -world ~/path/to/mc/world -pack client.jar
//	go run ./cmd/explorer -map path/to/voxelmap -pos 251,95,-350
//
// See README.md for controls and debug keys. The window, input, tick loop,
// player and entities are package app; files here:
//
//	main.go    flags, engine config, hooks
//	input.go   the explorer's own keys
//	items.go   item sprites: the pickups
//	creatures.go  the animals (app entities): behaviour and deaths
//	import.go  Roblox-style animal packs
//	zombies.go zombie defense's client side: zombies, pickups
//	inventory.go the player's guns and ammunition as inventory items
//	guns*.go, refit.go  TACZ guns
//	combat.go, torches.go, signs.go, help.go: demo features
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"

	"github.com/memmaker/voxellib/ui/basepack"
	"github.com/memmaker/voxellib/ui/nametag"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/ui/uiapp"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/respack"
	"github.com/memmaker/voxellib/voxel/signtext"
	"github.com/memmaker/voxellib/voxel/voxelmap"
	"github.com/memmaker/voxellib/worldgen/flat"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

func init() { runtime.LockOSThread() }

// mapHotbar is the hotbar of -map worlds (pre-1.13 block keys).
var mapHotbar = []string{resloc.Qualify("grass#0"), resloc.Qualify("dirt#0"), resloc.Qualify("stone#0"), resloc.Qualify("cobblestone#0"),
	resloc.Qualify("planks#0"), resloc.Qualify("log#0"), resloc.Qualify("glass#0"), resloc.Qualify("brick_block#0"), resloc.Qualify("glowstone#0")}

var hotbarNames = []string{
	"grass_block[snowy=false]", "dirt", "stone", "cobblestone", "oak_planks", "oak_log[axis=y]",
	"glass", "bricks", "glowstone", "oak_leaves[distance=1,persistent=true,waterlogged=false]",
	"sand", "stone_bricks", "water[level=0]", "lava[level=0]",
}

type options struct {
	signFont, pets, species, anim, gun, press string
	attach                                    string // -attach: slot=attachment,... on the -gun
	numMobs                                   int
	zombies                                   int // -zombies: stress test (mp.ServerConfig.Zombies)
	night, fly, lineup, help                  bool
	autoplay                                  bool // -autoplay: autoplay.go
	// sandbox explores the world instead of playing zombie defense.
	sandbox bool
	// fullZoom magnifies the whole screen while aiming (TACZ; cheaper)
	// instead of only inside a scope's lens (a second world render).
	fullZoom bool
	// watch is the debug camera on another player (-watch): the angle
	// (radians from their facing, clockwise from above) and distance.
	watch *[2]float64
}

// attachAdapterPrefix marks a slot adapter in -attach (adapter.scope=ID).
const attachAdapterPrefix = "adapter."

func main() {
	var (
		o        options
		cfg      app.Config
		worldDir = flag.String("world", "saves/demo", "world directory to load/save ('' = memory only)")
		readOnly = flag.Bool("readonly", false, "never write to the world directory")
		jar      = flag.String("jar", filepath.Join(os.Getenv("HOME"), "Library/Application Support/PrismLauncher/libraries/com/mojang/minecraft/1.7.10/minecraft-1.7.10-client.jar"), "Minecraft 1.7.10 client jar (GUI textures, font, language)")
		packs    = flag.String("pack", "", "extra resource packs (dir/zip/jar), comma separated, highest priority first")
		genName  = flag.String("gen", "overworld", "terrain generator for new chunks: overworld | flat")
		seed     = flag.Int64("seed", 1337, "terrain seed")
		dist     = flag.Int("dist", 12, "render distance in chunks")
		startPos = flag.String("pos", "", "start position x,y,z")
		look     = flag.String("look", "", "start yaw,pitch in degrees")
		cpuprof  = flag.String("cpuprofile", "", "write a CPU profile")
		mapDir   = flag.String("map", "", "VoxelMap directory to walk (instead of -world)")
		mapDim   = flag.Int("dim", math.MinInt, "VoxelMap dimension id (default: the map's spawn dimension)")
		mapsDir  = flag.String("maps", mp.MapsDir, "directory of the VoxelMaps multiplayer games can be played on")
		slot     = flag.String("slot", "", "save slot for -map: only your changes are stored there ('' = not saved)")
		scale    = flag.Float64("scale", 1, "render the world at this fraction of the window resolution (HUD stays sharp)")
	)
	flag.StringVar(&o.signFont, "signfont", "", "pixel TTF for sign text ('' = built-in font, 'none' = no sign text)")
	flag.StringVar(&o.pets, "pets", filepath.Join(os.Getenv("HOME"), "Downloads/CubePetsVol2_Free_v1.0.zip"), "Roblox-style model pack (dir/zip/glb) to populate the world with ('' = none)")
	flag.IntVar(&o.numMobs, "mob-count", 24, "animals spawned around the player")
	flag.StringVar(&cfg.Debug.Shot, "shot", "", "save a screenshot PNG at tick -shot-at and quit")
	flag.IntVar(&cfg.Debug.ShotAt, "shot-at", 200, "tick for -shot")
	flag.BoolVar(&o.night, "night", false, "start at night")
	flag.BoolVar(&o.fly, "fly", false, "start flying")
	flag.StringVar(&o.press, "press", "", "debug: actions held down from the start, comma separated (e.g. use,attack: aim and fire)")
	flag.BoolVar(&o.fullZoom, "full-zoom", false, "aiming magnifies the whole screen as TACZ does, instead of a second render inside scope lenses (for slower machines)")
	watch := flag.String("watch", "", "debug: in multiplayer, view the first other player from angle,distance (degrees from their facing, blocks) instead of your eyes")
	flag.StringVar(&o.gun, "gun", "", "debug: gun (a TACZ gun ID, e.g. ak47) given and in hand at start")
	flag.StringVar(&o.attach, "attach", "", "debug: attachments installed on the -gun in order, slot=ID comma separated (e.g. scope=tacz:scope_acog_ta31; an empty ID removes one); adapter.slot=ID selects a slot adapter")
	flag.StringVar(&cfg.Debug.Edits, "edits", "", "debug: 'dx,dy,dz,block;...' edits relative to the player, applied once terrain is ready as blocks the player placed (shared in multiplayer)")
	flag.DurationVar(&cfg.Debug.AutoTurn, "autoturn", 0, "debug: stand and turn round from this long into the game on, firing after 5 and 15 s (benchmark under real conditions)")
	flag.Float64Var(&cfg.Debug.AutoFly, "autofly", 0, "debug: fly forward automatically at this many blocks/s (streaming stress test)")
	flag.BoolVar(&cfg.Debug.Verbose, "verbose", false, "print frame stats to stdout")
	flag.DurationVar(&cfg.Debug.Bench, "bench", 0, "quit after this long and print the mean frame time (runs unfocused, mouse free)")
	flag.BoolVar(&o.lineup, "lineup", false, "debug: line up one standing creature per species in front of the start position")
	flag.StringVar(&o.species, "species", "", "debug: only spawn species whose name contains this")
	flag.StringVar(&o.anim, "anim", "", "debug: force this clip on lined-up creatures")
	flag.BoolVar(&o.help, "help-screen", false, "start with the F1 help screen open")
	var (
		host    = flag.Bool("host", false, "host a LAN multiplayer game at once")
		hostGen = flag.String("host-gen", "overworld", "world of the -host game: overworld | flat | map:<name>")
		connect = flag.String("connect", "", "join the LAN multiplayer game at this address (host[:port]) at once")
		name    = flag.String("name", "", "player name in multiplayer (default: the user name)")
		skinDir = flag.String("skins", "skins", "directory of 64x64 player skin PNGs (cmd/fetchskins)")
		skinSel = flag.String("skin", "", "multiplayer skin (a file name in -skins without .png)")
		menu    = flag.Bool("menu", false, "open the main menu at start instead of playing zombie defense")
	)
	flag.IntVar(&o.zombies, "zombies", 0, "debug: this many more zombies in zombie defense's first wave; the server logs its tick times (single player and -host)")
	flag.BoolVar(&o.autoplay, "autoplay", false, "debug: play zombie defense by itself (fetch guns and ammo, shoot zombies), logging each step")
	flag.BoolVar(&o.sandbox, "sandbox", false, "explore the world (animals, every gun) instead of playing zombie defense; implied by -world, -map, -bench and -autofly")
	flag.BoolVar(&cfg.Fullscreen, "fullscreen", false, "fullscreen on the primary monitor at its desktop (native) resolution")
	flag.StringVar(&cfg.VSync, "vsync", app.DefaultVSync, "frame pacing: on (vsync), limit (no vsync, sleep to max(display rate, 120Hz)), off (unlimited), or a rate in Hz (e.g. 60)")
	logPath := flag.String("log", "", "also write the log and any crash trace to this file")
	flag.Parse()
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(io.MultiWriter(os.Stderr, f))
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
			log.Fatal(err)
		}
	}
	mp.MapsDir = *mapsDir
	o.sandbox = o.sandbox || flagSet("world") || *mapDir != "" || cfg.Debug.Bench > 0 || cfg.Debug.AutoFly > 0

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	cfg.Title = "voxellib explorer (F1 help)"
	// gun sounds; scripted runs (screenshots, benchmarks) stay silent
	cfg.Audio = cfg.Debug.Shot == "" && cfg.Debug.Bench == 0 && cfg.Debug.AutoTurn == 0 && cfg.Debug.AutoFly == 0
	if *startPos != "" {
		var p [3]float64
		fmt.Sscanf(*startPos, "%f,%f,%f", &p[0], &p[1], &p[2])
		cfg.Start = &p
	}
	if *watch != "" {
		var w [2]float64
		if _, err := fmt.Sscanf(*watch, "%f,%f", &w[0], &w[1]); err != nil {
			log.Fatalf("-watch %q: %v", *watch, err)
		}
		w[0] = w[0] * math.Pi / 180
		o.watch = &w
	}
	if *look != "" {
		var yd, pd float64
		fmt.Sscanf(*look, "%f,%f", &yd, &pd)
		cfg.Yaw, cfg.Pitch = yd*math.Pi/180, pd*math.Pi/180
	}

	var paths []string
	if *packs != "" {
		paths = strings.Split(*packs, ",")
	}
	paths = append(paths, "assets/rpg16pack")
	pack, err := respack.Open(paths...)
	if err != nil {
		log.Fatal(err)
	}
	defer pack.Close()

	reg := voxel.NewRegistry()
	cfg.Engine = engine.Config{
		Registry:       reg,
		RenderDistance: *dist,
		WorldDir:       *worldDir,
		ReadOnly:       *readOnly,
		Textures:       pack,
		RenderOptions:  render.Options{Missing: respack.MissingTexture, IsGreyscale: respack.IsGreyscale},
	}
	hotbar := hotbarNames
	if *mapDir != "" {
		m, err := voxelmap.Open(*mapDir, reg)
		if err != nil {
			log.Fatal(err)
		}
		defer m.Close()
		c := &cfg.Engine
		c.Map, c.Dimension, c.SaveDir, c.WorldDir, c.Textures = m, *mapDim, *slot, "", nil
		if *mapDim == math.MinInt {
			c.Dimension = m.Spawn.Dim
		}
		hotbar = mapHotbar
	} else {
		reg.Resolver, reg.Faces = pack.Resolver(), pack.FaceResolver()
		switch *genName {
		case "overworld":
			cfg.Engine.Generator = overworld.New(reg, *seed)
		case "flat":
			cfg.Engine.Generator = flat.New(reg)
		default:
			log.Fatalf("unknown generator %q", *genName)
		}
	}

	a, err := app.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	a.RenderScale = *scale
	g := &game{App: a, opt: o, playerName: *name}
	if g.defaultSkin, g.skins, err = loadSkins(*skinDir); err != nil {
		log.Fatal(err)
	}
	if g.playerName == "" {
		g.playerName = defaultName()
	}
	for i, s := range g.skins {
		if s.name == *skinSel {
			g.skinIdx = i
		}
	}
	for _, n := range hotbar {
		if m := cfg.Engine.Map; m != nil {
			g.hotbar = append(g.hotbar, m.State(n))
		} else {
			g.hotbar = append(g.hotbar, reg.Parse(n))
		}
	}
	a.Hooks = app.Hooks{
		Start:         g.start,
		Tick:          g.tick,
		Action:        g.action,
		Render:        g.render,
		DrawWorld:     g.drawWorld,
		DrawHand:      g.drawHand,
		BlockInteract: g.blockInteract,
		BreakStrength: g.breakStrength,
		HeldBlock:     g.heldBlock,
		Held:          g.heldChanged,
		Status:        g.status,
		Ride:          g.ride,
		BlockBroken:   g.blockBroken,
		BlockPlaced:   g.blockPlaced,
	}
	a.HUD = g
	g.solo = func() error { return g.playSolo(*genName, *seed) }
	if err := g.setupUI(*jar); err != nil {
		log.Fatal(err)
	}
	if g.ui != nil {
		a.HUD = hudStack{g, g.ui}
		g.registerSkins()
	}
	switch {
	case *host:
		g.autoNet = func() error { return g.hostGame("", 0, *hostGen, mp.ZombieDefense) }
	case *connect != "":
		addr := *connect
		if !strings.Contains(addr, ":") {
			addr = fmt.Sprintf("%s:%d", addr, mp.DefaultPort)
		}
		g.autoNet = func() error { return g.joinGame(addr) }
	case *menu:
		g.menuAtStart = true
	case !o.sandbox:
		g.autoNet = g.solo
	}
	g.bindKeys()
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

// game is the explorer's state on top of the app shell.
type game struct {
	*app.App
	opt options
	// watching: the -watch camera views another player this frame
	watching bool
	hotbar   []voxel.StateID
	signs    *signtext.Renderer
	nameTags *nametag.Renderer // other players' names (multiplayer)
	torches  torchFX

	itemsState
	creaturesState
	gunsState
	combatState
	zombieState
	autoplay
	help     *render.Overlay // F1 help screen
	showHelp bool
	debugMsg string // shown in the title (K animation preview)

	// Multiplayer (multiplayer.go, menu.go).
	ui          *uiapp.UI
	net         *netGame // nil: single player
	sp          *spWorld // the single-player world to return to
	playerName  string
	skins       []skin
	defaultSkin skin // the vanilla default (Steve), for players without one
	skinIdx     int
	pendingNote string       // why the last game ended, for the main menu
	autoNet     func() error // -host / -connect, run in the first session
	solo        func() error // starts single player: zombie defense on a private server
	menuAtStart bool
	showScores  bool // Tab held in multiplayer
	wallDraw    [1]render.SkyDraw
}

// flagSet reports whether the flag was given on the command line.
func flagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// defaultName is the login name, trimmed to a player name's 16 characters.
func defaultName() string {
	n := os.Getenv("USER")
	if n == "" {
		n = os.Getenv("USERNAME")
	}
	if n == "" {
		n = "Player"
	}
	return n[:min(len(n), 16)]
}

// start sets up a session: animals, guns, zombie defense's client side
// (the app calls it once the engine and the player exist).
func (g *game) start() {
	o := &g.opt
	g.Player.Flying = o.fly
	if g.net != nil {
		o = &options{signFont: o.signFont, pets: o.pets, numMobs: o.numMobs, gun: o.gun, attach: o.attach, press: o.press, autoplay: o.autoplay} // the server's time of day
	}
	// the last session's renderers, freed now rather than by the garbage
	// collector while this one draws
	if g.itemRend != nil {
		g.itemRend.Destroy()
	}
	if g.torches.rend != nil {
		g.torches.rend.Destroy()
	}
	g.Eng.Renderer.Opt.Daylight = 1 // after a multiplayer game's
	if o.night {
		g.Eng.Renderer.Opt.Daylight = 0.2
	}
	g.itemsState = itemsState{}
	g.creaturesState = creaturesState{spawnLeft: o.numMobs, mobRand: rand.New(rand.NewSource(7)),
		lineup: o.lineup, lineupAnim: o.anim}
	g.combatState = combatState{health: maxHealth, hudHealth: -1}
	g.gunsState = gunsState{sounds: gunSounds{mixer: g.Mixer}}
	g.setupGuns(g.net == nil)
	g.setupArms()
	g.setupZombies()
	if it := g.itemReg.Get(o.gun); o.gun != "" && it == nil {
		log.Printf("-gun: no gun %q in this pack", o.gun)
	} else if it != nil {
		g.inv().Set(g.inv().Held, inventory.New(it, 1))
		g.heldChanged(g.inv().Current()) // in hand at once, for -attach
	}
	if o.attach != "" && g.gunOn() {
		for _, kv := range strings.Split(o.attach, ",") {
			slot, id, _ := strings.Cut(kv, "=")
			install := g.refit
			if s, ok := strings.CutPrefix(slot, attachAdapterPrefix); ok {
				slot, install = s, g.refitAdapter
			}
			if err := install(tacz.Slot(slot), id); err != nil {
				log.Printf("-attach %s: %v", kv, err)
			}
		}
	}
	for _, a := range strings.Split(o.press, ",") {
		if a != "" {
			g.Input.Hold(a, true)
		}
	}
	g.torches = torchFX{}
	if o.signFont != "none" {
		if err := g.loadSignFont(o.signFont); err != nil {
			log.Printf("sign text disabled: %v", err)
		}
	}
	if nt, err := nametag.New(g.Models, basepack.FS); err != nil {
		log.Printf("name tags disabled: %v", err)
	} else {
		g.nameTags = nt
	}
	if o.pets != "" {
		g.importPets(o.pets)
	}
	g.finishItems()
	g.filterSpecies(o.species)
	if o.help && !g.showHelp {
		g.toggleHelp()
	}
	switch {
	case g.net != nil:
		g.netStart()
	case g.autoNet != nil:
		f := g.autoNet
		g.autoNet = nil
		if err := f(); err != nil {
			log.Printf("mp: %v", err)
			g.pendingNote = err.Error()
			g.openMainMenu()
		}
	case g.menuAtStart || g.pendingNote != "":
		g.menuAtStart = false
		g.openMainMenu()
	}
}

func (g *game) tick() {
	if g.net != nil {
		g.sendCmd()
		g.tickGun()       // its shots go to the server, after the command
		g.tickCreatures() // the animals: client-side scenery
		g.tickTorches()
		g.tickNet()
		g.tickPlayerHealth() // the hurt flash fades
		g.tickAutoplay()
		return
	}
	g.tickGun()
	g.tickCreatures()
	g.tickBullets()
	g.tickTorches()
	g.tickPlayerHealth()
}

func (g *game) render(alpha float64, _ *render.Options) {
	g.gunFOV(alpha)
	g.animStep()
	if g.net != nil {
		g.netFrame()
		g.netPose()
		g.watchCam()
	}
	g.fxFrame()
}

// drawWorld adds the explorer's world passes for cam after the terrain
// and creatures (Hooks.DrawWorld: the main view and the scope view).
func (g *game) drawWorld(cam *render.Camera, alpha float64) {
	g.drawSigns(cam)
	if g.net != nil {
		g.drawNet(cam)
	}
	g.drawTorches(cam, alpha)
	g.drawPickups(cam)
	g.drawFX(cam)
	g.drawTracers(cam, alpha)
}

// drawHand draws the held item in first person (Hooks.DrawHand).
func (g *game) drawHand(alpha float64) {
	switch {
	case g.watching:
	case g.gunOn():
		g.drawGunHand(alpha)
	}
}

// Draw is the explorer's app.HUD: hearts, hurt flash, F1 help.
func (g *game) Draw(fbW, fbH int) {
	g.drawHUD(fbW, fbH)
	g.drawHelp(fbW, fbH)
}

// blockInteract opens and closes doors (left or right click).
func (g *game) blockInteract(h physics.Hit, _ app.Action) bool { return g.toggleDoor(h.Block) }

func (g *game) breakStrength(s voxel.StateID) float32 {
	if g.Eng.World.Reg.Info(s).Name == resloc.Qualify("bedrock") {
		return 0
	}
	return float32(math.Inf(1))
}

// heldBlock is the block "use" places (Hooks.HeldBlock): the block
// hotbar's, with an empty hand only.
func (g *game) heldBlock() voxel.StateID {
	if g.inv().Current() != nil {
		return voxel.Air
	}
	return g.hotbar[min(g.inv().Held, len(g.hotbar)-1)]
}

func (g *game) status() string {
	name := "nothing"
	if s := g.inv().Current(); s != nil {
		name = g.itemName(s.Item)
	} else if b := g.heldBlock(); b != voxel.Air {
		name = strings.TrimPrefix(g.Eng.World.Reg.Info(b).Name, resloc.Default+":")
	}
	s := fmt.Sprintf("[%d] %s", g.inv().Held+1, name)
	if g.debugMsg != "" {
		s += " | anim " + g.debugMsg
	}
	return s
}
