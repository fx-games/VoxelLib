// Command explorer is a first-person demo of voxellib on the app shell:
// Minecraft-like terrain, walking, flying and swimming, block breaking and
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
//	import.go  Roblox-style and Minecraft-style asset imports
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
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/mcres"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/voxelmap"
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

type options struct {
	itemDir, signFont, pets, mcAssets, species, anim, hold string
	numItems, numMobs                                      int
	night, fly, lineup, help                               bool
}

func main() {
	var (
		o        options
		cfg      app.Config
		worldDir = flag.String("world", "saves/demo", "Minecraft world directory to load/save ('' = memory only)")
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
		slot     = flag.String("slot", "", "save slot for -map: only your changes are stored there ('' = not saved)")
	)
	flag.StringVar(&o.signFont, "signfont", "", "pixel TTF for sign text ('' = built-in font, 'none' = no sign text)")
	flag.StringVar(&o.itemDir, "items", "assets/items", "directory of item sprite PNGs ('' = no items)")
	flag.IntVar(&o.numItems, "item-count", 40, "items scattered around the spawn")
	flag.StringVar(&o.pets, "pets", filepath.Join(os.Getenv("HOME"), "Downloads/CubePetsVol2_Free_v1.0.zip"), "Roblox-style model pack (dir/zip/glb) to populate the world with ('' = none)")
	flag.StringVar(&o.mcAssets, "mcassets", "assets/cleannrooster", "Minecraft-style asset pack (Blockbench models, animations, item textures) ('' = none)")
	flag.IntVar(&o.numMobs, "mob-count", 24, "creatures spawned around the player")
	flag.StringVar(&cfg.Debug.Shot, "shot", "", "save a screenshot PNG at tick -shot-at and quit")
	flag.IntVar(&cfg.Debug.ShotAt, "shot-at", 200, "tick for -shot")
	flag.BoolVar(&o.night, "night", false, "start at night")
	flag.BoolVar(&o.fly, "fly", false, "start flying")
	flag.StringVar(&o.hold, "hold", "sword_01", "item held at start")
	flag.StringVar(&cfg.Debug.Edits, "edits", "", "debug: 'dx,dy,dz,block;...' edits relative to the player, applied once terrain is ready")
	flag.Float64Var(&cfg.Debug.AutoFly, "autofly", 0, "debug: fly forward automatically at this many blocks/s (streaming stress test)")
	flag.BoolVar(&cfg.Debug.Verbose, "verbose", false, "print frame stats to stdout")
	flag.DurationVar(&cfg.Debug.Bench, "bench", 0, "quit after this long and print the mean frame time (runs unfocused, mouse free)")
	flag.BoolVar(&o.lineup, "lineup", false, "debug: line up one standing creature per species in front of the start position")
	flag.StringVar(&o.species, "species", "", "debug: only spawn species whose name contains this")
	flag.StringVar(&o.anim, "anim", "", "debug: force this clip on lined-up creatures")
	flag.BoolVar(&o.help, "help-screen", false, "start with the F1 help screen open")
	flag.StringVar(&cfg.VSync, "vsync", "on", "frame pacing: on (vsync), limit (no vsync, sleep to max(display rate, 120Hz)), off (unlimited)")
	flag.Parse()

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
	pack, err := mcres.Open(paths...)
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
		RenderOptions:  render.Options{Missing: mcres.MissingTexture, IsGreyscale: mcres.IsGreyscale},
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
	g := &game{App: a, opt: o}
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
		BreakTicks:    g.breakTicks,
		HeldBlock:     func() voxel.StateID { return g.hotbar[min(a.Player.Selected, len(g.hotbar)-1)] },
		Status:        g.status,
	}
	a.HUD = g
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
	signs   *signText
	torches torchFX

	itemsState
	creaturesState
	crossbowState
	combatState
	help     *render.Overlay // F1 help screen
	showHelp bool
	debugMsg string // shown in the title (K animation preview)
}

// start sets up a session: items, creatures, weapons (the app calls it
// once the engine and the player exist).
func (g *game) start() {
	o := &g.opt
	g.Player.Flying = o.fly
	g.Player.HotbarSize = len(g.hotbar)
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
	if o.mcAssets != "" {
		g.importMCAssets(o.mcAssets)
	}
	g.finishItems()
	g.filterSpecies(o.species)
	if o.help && !g.showHelp {
		g.toggleHelp()
	}
}

func (g *game) tick() {
	if g.crossbowOn() && g.Input.Down(app.ActAttack) {
		g.fire() // held down: repeat at the reload rate
	}
	g.tickItems()
	g.tickCreatures()
	g.tickArrows()
	g.tickTorches()
	g.tickPlayerHealth()
}

func (g *game) render(alpha float64, _ *render.Options) { g.animStep() }

// draw adds the explorer's world passes after the terrain and creatures;
// the held item (which clears depth) comes last.
func (g *game) draw(alpha float64) {
	g.drawSigns()
	g.drawWeapons()
	g.drawArrows(alpha)
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

func (g *game) breakTicks(s voxel.StateID) int {
	if g.Eng.World.Reg.Info(s).Name == "minecraft:bedrock" {
		return -1
	}
	return 0
}

func (g *game) status() string {
	name := strings.TrimPrefix(g.Eng.World.Reg.Info(g.hotbar[min(g.Player.Selected, len(g.hotbar)-1)]).Name, "minecraft:")
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
