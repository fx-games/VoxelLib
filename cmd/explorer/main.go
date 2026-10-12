// Command explorer is a first-person demo of voxellib: Minecraft-like
// terrain, walking, flying and swimming, block breaking and placing, and
// extruded 2D item sprites to pick up.
//
//	./run.sh                                   # build and run (repo root)
//	go run ./cmd/explorer                      # generated world saved to saves/demo
//	go run ./cmd/explorer -gen flat -world ''  # flat test world, not saved
//	go run ./cmd/explorer -world ~/path/to/mc/world -pack client.jar
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
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/mcres"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/worldgen"
	"github.com/memmaker/voxellib/worldgen/flat"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

func init() { runtime.LockOSThread() }

var hotbarNames = []string{
	"grass_block[snowy=false]", "dirt", "stone", "cobblestone", "oak_planks", "oak_log[axis=y]",
	"glass", "bricks", "glowstone", "oak_leaves[distance=1,persistent=true,waterlogged=false]",
	"sand", "stone_bricks", "water[level=0]", "lava[level=0]",
}

func main() {
	var (
		worldDir = flag.String("world", "saves/demo", "Minecraft world directory to load/save ('' = memory only)")
		readOnly = flag.Bool("readonly", false, "never write to the world directory")
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
		showHelp = flag.Bool("help-screen", false, "start with the F1 help screen open")
		vsync    = flag.String("vsync", "on", "frame pacing: on (GL vsync), limit (no GL vsync, sleep to max(display rate, 120Hz)), off (unlimited)")
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
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 0)
	win, err := glfw.CreateWindow(1280, 720, "voxellib explorer", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	win.MakeContextCurrent()
	frameLimit := time.Duration(0)
	if *vsync == "on" {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
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
	reg.Resolver = pack.Resolver()
	var generator interface {
		worldgen.Generator
		worldgen.Spawner
	}
	switch *genName {
	case "overworld":
		generator = overworld.New(reg, *seed)
	case "flat":
		generator = flat.New(reg)
	default:
		log.Fatalf("unknown generator %q", *genName)
	}
	eng, err := engine.New(engine.Config{
		Registry:       reg,
		RenderDistance: *dist,
		Generator:      generator,
		WorldDir:       *worldDir,
		ReadOnly:       *readOnly,
		Textures:       pack,
		RenderOptions:  render.Options{Missing: mcres.MissingTexture, IsGreyscale: mcres.IsGreyscale},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	g := &game{eng: eng, win: win, verbose: *verbose, edits: *edits, autofly: *autofly,
		frameLimit: frameLimit}
	g.held, g.itemsLeft = *hold, *numItems
	g.terrain = &terrain{eng: eng}
	for _, n := range hotbarNames {
		g.hotbar = append(g.hotbar, reg.Parse(n))
	}
	g.cam = render.Camera{FOV: 70 * math.Pi / 180, Near: 0.05}
	var x, y, z float64
	if *startPos != "" {
		fmt.Sscanf(*startPos, "%f,%f,%f", &x, &y, &z)
		g.spawnKnown = true
	} else if sx, sy, sz, ok := eng.Spawn(); ok {
		x, y, z = float64(sx)+0.5, float64(sy), float64(sz)+0.5
		g.spawnKnown, g.findGround = true, true
	} else {
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
	g.spawnLeft = *numMobs
	if *pets != "" {
		g.importPets(*pets)
	}
	if *mcAssets != "" {
		g.importMCAssets(*mcAssets)
	}
	g.finishItems()
	if *showHelp {
		g.toggleHelp()
	}
	// -shot runs are automated: never grab the user's mouse.
	g.setupInput(*shot == "")
	g.run(*shot, *shotAt)
}

type game struct {
	eng     *engine.Engine
	win     *glfw.Window
	cam     render.Camera
	terrain *terrain
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

	itemsState
	creaturesState
	help     *render.Overlay // F1 help screen
	showHelp bool
	debugMsg string // shown in the title (K animation preview)

	verbose bool
	edits   string
	autofly float64
	stats   frameStats
	// frameLimit paces frames without GL vsync (0 = unlimited).
	frameLimit time.Duration
}

const tickDur = time.Second / 20
