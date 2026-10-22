// Command explorer is a first-person demo of voxellib: Minecraft-like
// terrain, walking, flying and swimming, block breaking and placing, and
// extruded 2D item sprites to pick up.
//
//	./run.sh                                   # build and run (repo root)
//	go run ./cmd/explorer                      # generated world saved to saves/demo
//	go run ./cmd/explorer -gen flat -world ''  # flat test world, not saved
//	go run ./cmd/explorer -world ~/path/to/mc/world -pack client.jar
//	go run ./cmd/explorer -map path/to/voxelmap -pos 251,95,-350
//
// See README.md for controls and debug keys. Files:
//
//	main.go    flags, window and engine setup
//	player.go  character controller glue, block breaking/placing
//	items.go   item sprites, drops, pickup, held item
//	creatures.go  imported animated models and their behaviour
//	import.go  Roblox-style and Minecraft-style asset imports
//	input.go   keyboard/mouse, debug keys
//	frame.go   main loop, frame stats, screenshots
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
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/mcres"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/voxelmap"
	"github.com/memmaker/voxellib/worldgen"
	"github.com/memmaker/voxellib/worldgen/flat"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

func init() { runtime.LockOSThread() }

// mapHotbar is the hotbar of -map worlds (pre-1.13 block keys).
var mapHotbar = []string{"minecraft:grass#0", "minecraft:dirt#0", "minecraft:stone#0", "minecraft:cobblestone#0",
	"minecraft:planks#0", "minecraft:log#0", "minecraft:glass#0", "minecraft:brick_block#0", "minecraft:glowstone#0"}

var hotbarNames = []string{
	"grass_block[snowy=false]", "dirt", "stone", "cobblestone", "oak_planks", "oak_log[axis=y]",
	"glass", "bricks", "glowstone", "oak_leaves[distance=1,persistent=true,waterlogged=false]",
	"sand", "stone_bricks", "water[level=0]", "lava[level=0]",
}

func main() {
	var (
		worldDir = flag.String("world", "saves/demo", "Minecraft world directory to load/save ('' = memory only)")
		readOnly = flag.Bool("readonly", false, "never write to the world directory")
		signFont = flag.String("signfont", "", "pixel TTF for sign text ('' = built-in font, 'none' = no sign text)")
		packs    = flag.String("pack", "", "extra resource packs (dir/zip/jar), comma separated, highest priority first")
		genName  = flag.String("gen", "overworld", "terrain generator for new chunks: overworld | flat")
		seed     = flag.Int64("seed", 1337, "terrain seed")
		dist     = flag.Int("dist", 12, "render distance in chunks")
		itemDir  = flag.String("items", "assets/items", "directory of item sprite PNGs ('' = no items)")
		numItems = flag.Int("item-count", 40, "items scattered around the spawn")
		pets     = flag.String("pets", filepath.Join(os.Getenv("HOME"), "Downloads/CubePetsVol2_Free_v1.0.zip"), "Roblox-style model pack (dir/zip/glb) to populate the world with ('' = none)")
		mcAssets = flag.String("mcassets", "assets/cleannrooster", "Minecraft-style asset pack (Blockbench models, animations, item textures) ('' = none)")
		numMobs  = flag.Int("mob-count", 24, "creatures spawned around the player")
		shot     = flag.String("shot", "", "save a screenshot PNG at tick -shot-at and quit")
		shotAt   = flag.Int("shot-at", 200, "tick for -shot")
		startPos = flag.String("pos", "", "start position x,y,z")
		look     = flag.String("look", "", "start yaw,pitch in degrees")
		night    = flag.Bool("night", false, "start at night")
		fly      = flag.Bool("fly", false, "start flying")
		hold     = flag.String("hold", "sword_01", "item held at start")
		edits    = flag.String("edits", "", "debug: 'dx,dy,dz,block;...' edits relative to the player, applied once terrain is ready")
		autofly  = flag.Float64("autofly", 0, "debug: fly forward automatically at this many blocks/s (streaming stress test)")
		cpuprof  = flag.String("cpuprofile", "", "write a CPU profile")
		verbose  = flag.Bool("verbose", false, "print frame stats to stdout")
		bench    = flag.Duration("bench", 0, "quit after this long and print the mean frame time (runs unfocused, mouse free)")
		lineup   = flag.Bool("lineup", false, "debug: line up one standing creature per species in front of the start position")
		speciesF = flag.String("species", "", "debug: only spawn species whose name contains this")
		animF    = flag.String("anim", "", "debug: force this clip on lined-up creatures")
		showHelp = flag.Bool("help-screen", false, "start with the F1 help screen open")
		mapDir   = flag.String("map", "", "VoxelMap directory to walk (instead of -world)")
		mapDim   = flag.Int("dim", math.MinInt, "VoxelMap dimension id (default: the map's spawn dimension)")
		slot     = flag.String("slot", "", "save slot for -map: only your changes are stored there ('' = not saved)")
		vsync    = flag.String("vsync", "on", "frame pacing: on (vsync), limit (no vsync, sleep to max(display rate, 120Hz)), off (unlimited)")
	)
	flag.Parse()

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()
	render.WindowHints()
	automated := *shot != "" || *autofly > 0 || *bench > 0
	if automated {
		// Automated run: don't take focus from whatever the user is doing.
		glfw.WindowHint(glfw.Focused, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
	}
	if *bench > 0 {
		// On top but unfocused: macOS throttles occluded Metal windows.
		glfw.WindowHint(glfw.Floating, glfw.True)
	}
	win, err := glfw.CreateWindow(1280, 720, "voxellib explorer", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := render.Init(win, *vsync == "on"); err != nil {
		log.Fatal(err)
	}
	frameLimit := time.Duration(0)
	if *vsync != "on" {
		if *vsync == "limit" {
			hz := 120 // never pace slower than this: 8.3ms frames
			if m := glfw.GetPrimaryMonitor(); m != nil && m.GetVideoMode().RefreshRate > hz {
				hz = m.GetVideoMode().RefreshRate
			}
			frameLimit = time.Second / time.Duration(hz)
		}
	}

	var paths []string
	if *packs != "" {
		paths = strings.Split(*packs, ",")
	}
	paths = append(paths, "assets/rpg16pack")
	pack, err := mcres.Open(paths...)
	if err != nil {
		log.Fatal(err)
	}
	defer pack.Close()

	reg := voxel.NewRegistry()
	var generator interface {
		worldgen.Generator
		worldgen.Spawner
	}
	cfg := engine.Config{
		Registry:       reg,
		RenderDistance: *dist,
		WorldDir:       *worldDir,
		ReadOnly:       *readOnly,
		Textures:       pack,
		RenderOptions:  render.Options{Missing: mcres.MissingTexture, IsGreyscale: mcres.IsGreyscale},
	}
	hotbar := hotbarNames
	if *mapDir != "" {
		m, err := voxelmap.Open(*mapDir, reg)
		if err != nil {
			log.Fatal(err)
		}
		defer m.Close()
		cfg.Map, cfg.Dimension, cfg.SaveDir, cfg.WorldDir, cfg.Textures = m, *mapDim, *slot, "", nil
		if *mapDim == math.MinInt {
			cfg.Dimension = m.Spawn.Dim
		}
		hotbar = mapHotbar
		cfg.RenderOptions.Missing = mcres.MissingTexture
	} else {
		reg.Resolver, reg.Faces = pack.Resolver(), pack.FaceResolver()
		switch *genName {
		case "overworld":
			generator = overworld.New(reg, *seed)
		case "flat":
			generator = flat.New(reg)
		default:
			log.Fatalf("unknown generator %q", *genName)
		}
		cfg.Generator = generator
	}
	eng, err := engine.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()
	if cfg.SaveDir != "" && !cfg.ReadOnly {
		defer func() { // a slot keeps only saved games; the explorer has no state of its own
			st, err := eng.GameState()
			if err == nil {
				err = eng.SaveGame(st)
			}
			if err != nil {
				log.Printf("save: %v", err)
			}
		}()
	}

	g := &game{eng: eng, win: win, verbose: *verbose, edits: *edits, autofly: *autofly,
		frameLimit: frameLimit, automated: automated, bench: *bench}
	g.held, g.itemsLeft = *hold, *numItems
	g.terrain = &terrain{eng: eng}
	for _, n := range hotbar {
		if cfg.Map != nil {
			g.hotbar = append(g.hotbar, cfg.Map.State(n))
		} else {
			g.hotbar = append(g.hotbar, reg.Parse(n))
		}
	}
	g.cam = render.Camera{FOV: 70 * math.Pi / 180, Near: 0.05}
	var x, y, z float64
	if *startPos != "" {
		fmt.Sscanf(*startPos, "%f,%f,%f", &x, &y, &z)
		g.spawnKnown = true
	} else if sx, sy, sz, ok := eng.Spawn(); ok {
		x, y, z = float64(sx)+0.5, float64(sy), float64(sz)+0.5
		g.spawnKnown, g.findGround = true, true
	} else if generator != nil {
		lx, lz := generator.FindLand(0, 0)
		x, z = float64(lx)+0.5, float64(lz)+0.5
		g.findGround = true
	}
	g.player = character.New(x, y, z)
	g.player.Flying = *fly
	g.spawnX, g.spawnY, g.spawnZ = x, y, z
	if *look != "" {
		var yd, pd float64
		fmt.Sscanf(*look, "%f,%f", &yd, &pd)
		g.cam.Yaw, g.cam.Pitch = yd*math.Pi/180, pd*math.Pi/180
	}
	if *night {
		eng.Renderer.Opt.Daylight = 0.2
	}
	if *itemDir != "" {
		if err := g.loadItems(*itemDir); err != nil {
			log.Printf("items disabled: %v", err)
		}
	}
	if *signFont != "none" {
		if err := g.loadSignFont(*signFont); err != nil {
			log.Printf("sign text disabled: %v", err)
		}
	}
	g.spawnLeft = *numMobs
	g.mobRand = rand.New(rand.NewSource(7))
	g.bowModel, g.arrowModel = -1, -1 // set by setupCrossbow
	g.swordModel = -1                 // set by setupCombat
	if *pets != "" {
		g.importPets(*pets)
	}
	if *mcAssets != "" {
		g.importMCAssets(*mcAssets)
	}
	g.finishItems()
	g.filterSpecies(*speciesF)
	g.lineup, g.lineupAnim = *lineup, *animF
	if *showHelp {
		g.toggleHelp()
	}
	// -shot runs are automated: never grab the user's mouse or take any
	// keyboard or mouse input.
	if *shot == "" {
		g.setupInput()
	}
	g.run(*shot, *shotAt)
}

type game struct {
	eng     *engine.Engine
	win     *glfw.Window
	cam     render.Camera
	terrain *terrain
	signs   *signText
	torches torchFX
	player  *character.Character

	spawnX, spawnY, spawnZ float64
	findGround             bool // snap to the surface once the chunk is loaded
	spawnKnown             bool
	frozen                 bool // waiting for terrain under the player

	hotbar   []voxel.StateID
	selected int
	captured bool
	lastJump time.Time
	tick     int
	breakCD  int
	placeCD  int
	swing    int // ticks left of the hand swing animation
	walkDist float64
	prevWalk float64 // walkDist last tick, for interpolating the bob

	itemsState
	creaturesState
	crossbowState
	combatState
	help     *render.Overlay // F1 help screen
	showHelp bool
	debugMsg string // shown in the title (K animation preview)
	shotReq  bool   // F2: save a screenshot after this frame

	verbose bool
	edits   string
	autofly float64
	// automated runs never grab the mouse; bench > 0 quits after that long.
	automated bool
	bench     time.Duration
	stats     frameStats
	// frameLimit paces frames without GL vsync (0 = unlimited).
	frameLimit time.Duration
}

const tickDur = time.Second / 20
