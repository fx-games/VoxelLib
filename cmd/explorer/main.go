// Command explorer is a first-person demo of voxellib on the app shell:
// reference-like terrain, walking, flying and swimming, block breaking and
// placing, extruded 2D item sprites to pick up, creatures, a crossbow.
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
//	items.go   item sprites, drops, pickup, held item
//	creatures.go  imported animated models (app entities) and their behaviour
//	import.go  Roblox-style and Blockbench-style asset imports
//	combat.go, crossbow.go, torches.go, signs.go, help.go: demo features
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/memmaker/voxellib/app"
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
	itemDir, signFont, pets, modelPack, species, anim, hold string
	numItems, numMobs                                       int
	night, fly, lineup, help                                bool
}

func main() {
	var (
		o        options
		cfg      app.Config
		worldDir = flag.String("world", "saves/demo", "world directory to load/save ('' = memory only)")
		readOnly = flag.Bool("readonly", false, "never write to the world directory")
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
	flag.StringVar(&o.itemDir, "items", "assets/items", "directory of item sprite PNGs ('' = no items)")
	flag.IntVar(&o.numItems, "item-count", 40, "items scattered around the spawn")
	flag.StringVar(&o.pets, "pets", filepath.Join(os.Getenv("HOME"), "Downloads/CubePetsVol2_Free_v1.0.zip"), "Roblox-style model pack (dir/zip/glb) to populate the world with ('' = none)")
	flag.StringVar(&o.modelPack, "modelpack", "assets/cleannrooster", "Blockbench asset pack (Blockbench models, animations, item textures) ('' = none)")
	flag.IntVar(&o.numMobs, "mob-count", 24, "creatures spawned around the player")
	flag.StringVar(&cfg.Debug.Shot, "shot", "", "save a screenshot PNG at tick -shot-at and quit")
	flag.IntVar(&cfg.Debug.ShotAt, "shot-at", 200, "tick for -shot")
	flag.BoolVar(&o.night, "night", false, "start at night")
	flag.BoolVar(&o.fly, "fly", false, "start flying")
	flag.StringVar(&o.hold, "hold", "sword_01", "item held at start")
	flag.StringVar(&cfg.Debug.Edits, "edits", "", "debug: 'dx,dy,dz,block;...' edits relative to the player, applied once terrain is ready")
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
		menu    = flag.Bool("menu", true, "open the main menu at start (default not with -shot or -bench; never with -host or -connect)")
	)
	flag.BoolVar(&cfg.Fullscreen, "fullscreen", false, "fullscreen on the primary monitor at its desktop (native) resolution")
	flag.StringVar(&cfg.VSync, "vsync", "on", "frame pacing: on (vsync), limit (no vsync, sleep to max(display rate, 120Hz)), off (unlimited)")
	flag.Parse()
	mp.MapsDir = *mapsDir

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	cfg.Title = "voxellib explorer (F1 help)"
	if *startPos != "" {
		var p [3]float64
		fmt.Sscanf(*startPos, "%f,%f,%f", &p[0], &p[1], &p[2])
		cfg.Start = &p
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
	g := &game{App: a, opt: o, playerName: *name, skins: loadSkins(*skinDir)}
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
		Draw:          g.draw,
		BlockInteract: g.blockInteract,
		BreakStrength: g.breakStrength,
		HeldBlock:     func() voxel.StateID { return g.hotbar[min(a.Player.Selected, len(g.hotbar)-1)] },
		Status:        g.status,
		Ride:          g.ride,
		BlockBroken:   g.blockBroken,
		BlockPlaced:   g.blockPlaced,
	}
	a.HUD = g
	g.setupUI()
	if g.ui != nil {
		a.HUD = hudStack{g, g.ui}
		g.registerSkins()
	}
	switch {
	case *host:
		g.autoNet = func() error { return g.hostGame("", 0, *hostGen, mp.Deathmatch) }
	case *connect != "":
		addr := *connect
		if !strings.Contains(addr, ":") {
			addr = fmt.Sprintf("%s:%d", addr, mp.DefaultPort)
		}
		g.autoNet = func() error { return g.joinGame(addr) }
	case *menu && (cfg.Debug.Shot == "" && cfg.Debug.Bench == 0 || flagSet("menu")):
		g.menuAtStart = true
	}
	g.bindKeys()
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

// game is the explorer's state on top of the app shell.
type game struct {
	*app.App
	opt     options
	hotbar  []voxel.StateID
	signs   *signtext.Renderer
	torches torchFX

	itemsState
	creaturesState
	crossbowState
	combatState
	help     *render.Overlay // F1 help screen
	showHelp bool
	debugMsg string // shown in the title (K animation preview)

	// Multiplayer (multiplayer.go, menu.go).
	ui          *uiapp.UI
	net         *netGame // nil: single player
	sp          *spWorld // the single-player world to return to
	playerName  string
	skins       []skin
	skinIdx     int
	pendingNote string       // why the last game ended, for the main menu
	autoNet     func() error // -host / -connect, run in the first session
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

// start sets up a session: items, creatures, weapons (the app calls it
// once the engine and the player exist).
func (g *game) start() {
	o := &g.opt
	g.Player.Flying = o.fly
	g.Player.HotbarSize = len(g.hotbar)
	if g.net != nil {
		o = &options{hold: "", signFont: o.signFont, modelPack: o.modelPack} // no items, creatures or pets
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
	g.itemsState = itemsState{held: o.hold, itemsLeft: o.numItems}
	g.creaturesState = creaturesState{spawnLeft: o.numMobs, mobRand: rand.New(rand.NewSource(7)),
		lineup: o.lineup, lineupAnim: o.anim}
	g.crossbowState = crossbowState{bowModel: -1, arrowModel: -1} // set by setupCrossbow
	g.combatState = combatState{swordModel: -1}                   // set by setupCombat
	g.torches = torchFX{}
	if o.itemDir != "" {
		if err := g.loadItems(o.itemDir); err != nil {
			log.Printf("items disabled: %v", err)
		}
	}
	if o.signFont != "none" {
		if err := g.loadSignFont(o.signFont); err != nil {
			log.Printf("sign text disabled: %v", err)
		}
	}
	if o.pets != "" {
		g.importPets(o.pets)
	}
	if o.modelPack != "" {
		g.importBlockbenchPack(o.modelPack)
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

// autoShot reports whether -autoturn fires this tick: 5 and 15 s into
// the turning.
func (g *game) autoShot() bool {
	if g.Config.Debug.AutoTurn <= 0 {
		return false
	}
	t := g.Tick - g.AutoTurnTick()
	return t == 5*app.TickRate || t == 15*app.TickRate
}

func (g *game) tick() {
	if g.net == nil && g.autoShot() {
		g.fire()
	}
	if g.net != nil {
		g.tickItems()
		g.tickTorches()
		g.tickNet()
		g.tickPlayerHealth() // the hurt flash fades
		return
	}
	if g.crossbowOn() && g.Input.Down(app.ActAttack) {
		g.fire() // held down: repeat at the reload rate
	}
	g.tickItems()
	g.tickCreatures()
	g.tickArrows()
	g.tickTorches()
	g.tickPlayerHealth()
}

func (g *game) render(alpha float64, _ *render.Options) {
	g.animStep()
	if g.net != nil {
		g.netFrame()
	}
}

// draw adds the explorer's world passes after the terrain and creatures;
// the held item (which clears depth) comes last.
func (g *game) draw(alpha float64) {
	g.drawSigns()
	g.drawWeapons()
	if g.net != nil {
		g.drawNet(alpha)
	} else {
		g.drawArrows(alpha)
	}
	g.drawTorches(alpha)
	g.drawItems(alpha)
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

func (g *game) status() string {
	name := strings.TrimPrefix(g.Eng.World.Reg.Info(g.hotbar[min(g.Player.Selected, len(g.hotbar)-1)]).Name, resloc.Default+":")
	held := g.held
	if held == "" {
		held = "-"
	}
	s := fmt.Sprintf("[%d] %s | hand %s", g.Player.Selected+1, name, held)
	if g.debugMsg != "" {
		s += " | anim " + g.debugMsg
	}
	return s
}
