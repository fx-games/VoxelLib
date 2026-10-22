// Package app is a reusable game shell for voxellib games: a native window,
// rebindable input with mouse look, a fixed 20 Hz tick with partial-tick
// interpolation and the render loop, the first-person player (controller,
// camera, hand, reach, block breaking and placing), an entity registry,
// save/load glue for engine save slots, pause and screen/HUD hook points
// for a UI layer, and positional sound hooks.
//
// A game fills Config (window, engine.Config, debug flags) and Hooks (its
// callbacks), then calls Run:
//
//	a, err := app.New(cfg)
//	a.Hooks.Start = func() { ... }          // per session: GPU assets, entities
//	a.Hooks.Tick = func() { ... }           // 20 Hz game rules
//	a.Hooks.Draw = func(alpha float64) {...} // extra world passes
//	err = a.Run()
//
// Everything runs on the main (render) thread; hooks need no locking.
package app

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/worldgen"
)

// TickRate is the game tick rate (Minecraft's 20 Hz).
const TickRate = 20

// TickDur is one tick.
const TickDur = time.Second / TickRate

// Config sets up an App.
type Config struct {
	Title         string
	Width, Height int    // window size, default 1280x720
	VSync         string // "on" (default), "limit" (sleep to max(display, 120 Hz)), "off"
	Engine        engine.Config
	// Start, when set, is where a new game starts (else the world's spawn,
	// else the generator's FindLand near 0,0).
	Start *[3]float64
	// Yaw, Pitch: the start look direction in radians.
	Yaw, Pitch float64
	Audio      bool // open an audio.Mixer (Mixer stays nil if that fails)
	Debug      Debug
}

// Debug holds test and profiling switches.
type Debug struct {
	Shot    string        // save a screenshot PNG here at tick ShotAt and quit; takes no input
	ShotAt  int           // tick for Shot
	Bench   time.Duration // quit after this long and print frame times (runs unfocused)
	AutoFly float64       // fly forward at this many blocks/s (streaming stress test)
	Verbose bool          // print frame stats to stdout
	// Edits: "dx,dy,dz,block;..." block edits relative to the player,
	// applied once the terrain under the player is ready.
	Edits string
}

// Action is what a player does to a block or an entity.
type Action int

const (
	Attack Action = iota // left click: hit, break
	Use                  // right click: open, activate, place
)

// Hooks are the game's callbacks; nil ones are skipped. They run on the
// main thread.
type Hooks struct {
	// Start runs at the start of each session (first run and after
	// NewGame), once the engine, the model renderer and the player exist:
	// load GPU assets, add entities.
	Start func()
	// Load gets the game's state blob from the save slot (nil for a new
	// game), after Start. Save returns it; see App.Save.
	Load func(state []byte) error
	Save func() []byte
	// Tick runs every 20 Hz tick after the player and the entities ticked
	// (not while paused).
	Tick func()
	// Action gets every bound action press (pressed) and release before the
	// app handles it; return true to consume it. A consumed "attack" or
	// "use" press suppresses breaking/placing until the button is released.
	Action func(name string, pressed bool) bool
	// Render runs each frame before the world is drawn: set fog, sky,
	// daylight and the like in opt (the renderer's options). The app has
	// already set the under-water/lava fog.
	Render func(alpha float64, opt *render.Options)
	// Draw runs each frame after the terrain, the block outline and the
	// entities: extra world passes, then the first-person hand last
	// (hands clear depth).
	Draw func(alpha float64)
	// BlockInteract: the player attacked (first click) or used a block
	// within reach. Return true when handled: no breaking/placing follows.
	BlockInteract func(hit physics.Hit, act Action) bool
	// EntityInteract: the player attacked or used an entity within reach.
	EntityInteract func(e *Entity, act Action)
	// BreakTicks is how many ticks of holding attack break a block
	// (0 = at once, <0 = unbreakable). nil: everything breaks at once.
	BreakTicks func(state voxel.StateID) int
	// BlockBroken runs after the player broke a block (now air), e.g. to
	// drop items. A plant standing on it pops too, with its own call.
	BlockBroken func(pos voxel.BlockPos, state voxel.StateID)
	// HeldBlock is the block "use" places (voxel.Air: nothing).
	HeldBlock func() voxel.StateID
	// BlockPlaced runs after the player placed a block, e.g. to use up
	// the item.
	BlockPlaced func(pos voxel.BlockPos, state voxel.StateID)
	// Sound gets the app's sound events ("block.break", "block.place",
	// "block.hit" while breaking, "block.step" on walking) with the block
	// and where; play them with App.Mixer.
	Sound func(event string, state voxel.StateID, x, y, z float64)
	// PauseScreen, when set, is opened by the pause action (Esc). Without
	// it Esc releases the mouse, and a second Esc quits.
	PauseScreen func() Screen
	// Status is appended to the window title.
	Status func() string
}

// App is a running game shell.
type App struct {
	Config Config
	Hooks  Hooks
	Win    *glfw.Window
	Input  *Input
	Mixer  *audio.Mixer // nil without Config.Audio

	// Per session (rebuilt by NewGame):
	Eng      *engine.Engine
	Models   *render.ModelRenderer // shared model renderer (entities, games)
	Cam      render.Camera
	Player   *Player
	Entities Entities
	Terrain  *Terrain // the world as character.World / collide.Solids
	Tick     int      // ticks this session

	screens  []Screen
	HUD      HUD // drawn every frame under the screens (nil = none)
	Captured bool

	quit, restart bool
	shotReq       bool
	frameLimit    time.Duration
	stats         frameStats
	alpha         float64
	sessionStart  time.Time
	readyLogged   bool
}

// New opens the window and the GPU. Call it on the main thread (lock it in
// an init with runtime.LockOSThread).
func New(cfg Config) (*App, error) {
	if cfg.Width == 0 {
		cfg.Width, cfg.Height = 1280, 720
	}
	if cfg.VSync == "" {
		cfg.VSync = "on"
	}
	if err := glfw.Init(); err != nil {
		return nil, err
	}
	render.WindowHints()
	if cfg.automated() {
		// Don't take focus from whatever the user is doing.
		glfw.WindowHint(glfw.Focused, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
	}
	if cfg.Debug.Bench > 0 {
		// On top but unfocused: macOS throttles occluded Metal windows.
		glfw.WindowHint(glfw.Floating, glfw.True)
	}
	win, err := glfw.CreateWindow(cfg.Width, cfg.Height, cfg.Title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return nil, err
	}
	if err := render.Init(win, cfg.VSync == "on"); err != nil {
		glfw.Terminate()
		return nil, err
	}
	a := &App{Config: cfg, Win: win}
	a.Input = newInput(a)
	if cfg.VSync == "limit" {
		hz := 120 // never pace slower than this: 8.3ms frames
		if m := glfw.GetPrimaryMonitor(); m != nil && m.GetVideoMode().RefreshRate > hz {
			hz = m.GetVideoMode().RefreshRate
		}
		a.frameLimit = time.Second / time.Duration(hz)
	}
	if cfg.Audio {
		if a.Mixer, err = audio.New(); err != nil {
			log.Printf("audio disabled: %v", err)
		}
	}
	return a, nil
}

func (c *Config) automated() bool {
	return c.Debug.Shot != "" || c.Debug.AutoFly > 0 || c.Debug.Bench > 0
}

// Run runs sessions until the window closes: each starts the engine, the
// player and the game (Hooks.Start, Hooks.Load), runs the loop, and saves
// on the way out. NewGame ends the session without saving, empties the
// save slot and starts a new one.
func (a *App) Run() error {
	defer glfw.Terminate()
	if a.Mixer != nil {
		defer a.Mixer.Close()
	}
	if a.Config.Debug.Shot == "" { // automated shots take no input at all
		a.Input.install()
	}
	for {
		if err := a.startSession(); err != nil {
			return err
		}
		a.loop()
		restart := a.restart
		a.endSession(!restart)
		if !restart {
			return nil
		}
		if dir := a.Config.Engine.SaveDir; dir != "" && !a.Config.Engine.ReadOnly {
			if err := engine.NewGame(dir); err != nil {
				return err
			}
		}
		a.restart, a.quit = false, false
	}
}

// Quit ends Run after this frame, saving the game.
func (a *App) Quit() { a.quit = true }

// NewGame ends the session without saving, empties the save slot and
// starts over (Hooks.Start, then Hooks.Load(nil)).
func (a *App) NewGame() { a.restart = true }

func (a *App) startSession() error {
	a.sessionStart, a.readyLogged = time.Now(), false
	eng, err := engine.New(a.Config.Engine)
	if err != nil {
		return err
	}
	a.logStartup("engine.New")
	a.Eng, a.Tick, a.Entities = eng, 0, Entities{}
	a.screens = nil
	a.Terrain = &Terrain{eng: eng}
	if a.Models, err = render.NewModelRenderer(eng.Renderer); err != nil {
		eng.Close()
		return err
	}
	a.Cam = render.Camera{FOV: 70 * math.Pi / 180, Near: 0.05, Yaw: a.Config.Yaw, Pitch: a.Config.Pitch}
	a.Player = a.newPlayer()
	if a.Hooks.Start != nil {
		a.Hooks.Start()
		a.logStartup("Hooks.Start")
	}
	st, err := eng.GameState()
	if err != nil {
		log.Printf("load: %v", err)
	}
	game, err := a.decodeSave(st)
	if err != nil {
		log.Printf("load: %v", err)
	}
	if a.Hooks.Load != nil {
		if err := a.Hooks.Load(game); err != nil {
			log.Printf("load: %v", err)
		}
	}
	a.Input.Capture(!a.Config.automated())
	return nil
}

// logStartup logs the time since the session started (Debug.Verbose).
func (a *App) logStartup(step string) {
	if a.Config.Debug.Verbose {
		log.Printf("startup: %s done at %v", step, time.Since(a.sessionStart).Round(time.Millisecond))
	}
}

func (a *App) endSession(save bool) {
	if save {
		if err := a.Save(); err != nil {
			log.Printf("save: %v", err)
		}
	}
	a.Eng.Close() // ponytail: game-owned GPU objects of the old renderer are left to the GC
	a.Eng, a.Models = nil, nil
}

// save is the slot's state blob: the app's player plus the game's own.
type save struct {
	Player playerSave `json:"player"`
	Game   []byte     `json:"game,omitempty"`
}

type playerSave struct {
	X, Y, Z, Yaw, Pitch    float64
	Flying                 bool
	SpawnX, SpawnY, SpawnZ float64
}

// Save saves the game to the save slot now: the world changes, the player
// and Hooks.Save's blob. Without Engine.SaveDir it does nothing.
func (a *App) Save() error {
	c := a.Config.Engine
	if c.SaveDir == "" || c.ReadOnly {
		return nil
	}
	b, err := a.encodeSave()
	if err != nil {
		return err
	}
	return a.Eng.SaveGame(b)
}

func (a *App) encodeSave() ([]byte, error) {
	p := a.Player
	s := save{Player: playerSave{p.X, p.Y, p.Z, a.Cam.Yaw, a.Cam.Pitch, p.Flying, p.SpawnX, p.SpawnY, p.SpawnZ}}
	if a.Hooks.Save != nil {
		s.Game = a.Hooks.Save()
	}
	return json.Marshal(s)
}

// decodeSave restores the player from a blob and returns the game's part
// (nil for a new game).
func (a *App) decodeSave(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var s save
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("save blob: %w", err)
	}
	p, ps := a.Player, s.Player
	p.Teleport(ps.X, ps.Y, ps.Z)
	p.Flying = ps.Flying
	p.SpawnX, p.SpawnY, p.SpawnZ = ps.SpawnX, ps.SpawnY, ps.SpawnZ
	p.findGround, p.spawnKnown = false, true
	a.Cam.Yaw, a.Cam.Pitch = ps.Yaw, ps.Pitch
	return s.Game, nil
}

// spawn picks a new game's start: Config.Start, the world's spawn, or the
// generator's land near the origin. findGround: snap to the surface once
// loaded; known: the height is meaningful (only rise from it).
func (a *App) spawn() (x, y, z float64, findGround, known bool) {
	if s := a.Config.Start; s != nil {
		return s[0], s[1], s[2], false, true
	}
	if sx, sy, sz, ok := a.Eng.Spawn(); ok {
		return float64(sx) + 0.5, float64(sy), float64(sz) + 0.5, true, true
	}
	if sp, ok := a.Config.Engine.Generator.(worldgen.Spawner); ok {
		lx, lz := sp.FindLand(0, 0)
		return float64(lx) + 0.5, 0, float64(lz) + 0.5, true, false
	}
	return 0.5, 100, 0.5, true, false
}

// Screen is a UI screen (menus, inventories, dialogs) drawn over the world.
// While any is open the game is paused and the mouse is free.
type Screen interface {
	// Update runs every tick, also while paused.
	Update()
	// Draw draws the screen on a framebuffer of fbW x fbH pixels.
	Draw(fbW, fbH int)
	// HandleInput gets every input event while the screen is on top;
	// false lets the app handle it (Esc then closes the screen).
	HandleInput(ev Event) bool
}

// HUD is drawn every frame over the world, under the screens.
type HUD interface {
	Draw(fbW, fbH int)
}

// OpenScreen puts s on top of the screen stack.
func (a *App) OpenScreen(s Screen) {
	a.screens = append(a.screens, s)
	a.Input.Capture(false)
}

// CloseScreen closes the top screen; closing the last one resumes the game.
func (a *App) CloseScreen() {
	if len(a.screens) == 0 {
		return
	}
	a.screens = a.screens[:len(a.screens)-1]
	if len(a.screens) == 0 {
		a.Input.Capture(!a.Config.automated())
	}
}

// TopScreen returns the screen on top, or nil.
func (a *App) TopScreen() Screen {
	if len(a.screens) == 0 {
		return nil
	}
	return a.screens[len(a.screens)-1]
}

// Paused reports whether the world stands still (a screen is open).
func (a *App) Paused() bool { return len(a.screens) > 0 }

// Ready reports whether the terrain around the player has loaded (and the
// player stands on it): spawn things only then.
func (a *App) Ready() bool { return !a.Player.Frozen && !a.Player.findGround }

// Light returns the sky and block light at a world position.
func (a *App) Light(x, y, z float64) (sky, block uint8) {
	return a.Eng.World.Light(int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z)))
}

// Alpha is the current frame's partial tick (0..1).
func (a *App) Alpha() float64 { return a.alpha }

// Screenshot saves the next frame to screenshots/<timestamp>.png with a
// .txt of position and look (as F2).
func (a *App) Screenshot() { a.shotReq = true }
