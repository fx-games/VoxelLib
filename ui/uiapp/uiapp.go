// Package uiapp runs ui screens and the ui HUD inside an app.App: it draws
// through ui/guicanvas after the world and converts app events
// (framebuffer pixels) to ui events (GUI pixels) with ui.GLFWInput. A game
// with its own loop uses guicanvas and ui.PlayerGUI instead.
//
//	u, _ := uiapp.New(a, packs, lang)
//	a.HUD = u                       // the overlay, every frame
//	a.Hooks.PauseScreen = func() app.Screen { return u.Screen(ui.NewPauseMenu(...)) }
//	u.Open(&ui.DialogScreen{...})   // from game code
//	u.Inventory.Item = func(s *inventory.Stack) ui.ItemStack { ... } // the game's item icons
//
// Inventory is the ui.PlayerGUI of app.Player.Inv: the HUD's hotbar and
// the inventory screen the inventory action (E) opens. The game registers
// its items, sets Inventory.Item and listens on app.Hooks.Held.
package uiapp

import (
	"fmt"
	"io/fs"
	"log"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/ui/guicanvas"
)

// UI is the bridge. It implements app.HUD (draws HUD) and, via Screen,
// app.Screen: all ui screens live in one ui.Manager that is a single entry
// on the app's screen stack, so only the top ui screen is drawn.
type UI struct {
	App *app.App
	Ctx *ui.Context
	HUD *ui.HUD // nil: no overlay
	// Top draws over everything, open screens included (the reference's
	// achievement window, drawn last in the game loop).
	Top func(ctx *ui.Context)
	// Inventory runs app.Player.Inv: the game sets its Item. Its HUD is
	// HUD; what the cursor carries on closing goes back into the inventory.
	Inventory *ui.PlayerGUI

	canvas   *guicanvas.Canvas
	in       ui.GLFWInput
	px       float64 // framebuffer pixels per GUI pixel (resize)
	lastTick int64
	open     bool
}

// InventoryTexture is the inventory screen's background, which assets
// must have (the client jar's).
const InventoryTexture = "textures/gui/container/inventory.png"

// New loads the font from assets (resource pack stack: the client jar)
// and wires the manager to the app's screen stack.
func New(a *app.App, assets fs.FS, lang *i18n.Catalog) (*UI, error) {
	if _, err := fs.Stat(assets, ui.AssetPath(InventoryTexture)); err != nil {
		return nil, fmt.Errorf("uiapp: GUI assets lack %s (give the client jar): %w", InventoryTexture, err)
	}
	u := &UI{App: a, canvas: guicanvas.New()}
	ctx, err := ui.NewContext(u.canvas, assets, lang)
	if err != nil {
		return nil, err
	}
	u.Ctx = ctx
	ctx.InWorld = true
	if ro := &a.Config.Engine.RenderOptions; ro.Glint == nil {
		ro.Glint = ctx.GlintTexture() // the world's glint from the GUI packs
	}
	if a.Win != nil {
		ctx.Clipboard = a.Win.GetClipboardString // the native clipboard (GLFW)
		ctx.SetClipboard = a.Win.SetClipboardString
	}
	// The player (and its inventory) comes with each session: bound on opening.
	pg := ui.NewPlayerGUI(ctx, nil, nil, func(*inventory.Stack) ui.ItemStack { return ui.ItemStack{} })
	pg.Keys = nil // app.ActInventory's bindings open and close it
	pg.NoEquip, pg.NoPreview, pg.NoCrafting = true, true, true
	u.Inventory, u.HUD = pg, pg.HUD
	a.Hooks.InventoryScreen = func() app.Screen {
		if inv := a.Player.Inv; pg.Player != inv {
			pg.Player, pg.Container = inv, playerContainer(inv)
		}
		u.open = true
		pg.Open()
		return u.adapter()
	}
	ctx.Screens.OnEmpty = func() {
		pg.Close()
		if u.open {
			u.open = false
			a.CloseScreen()
		}
	}
	return u, nil
}

// playerContainer is inv's screen without crafting. With no world to drop
// into, what the cursor carries goes back in on closing.
func playerContainer(inv *inventory.Player) *inventory.Container {
	c, _ := inventory.NewPlayerContainer(inv, nil)
	c.Drop = func(s *inventory.Stack) {
		if inv.Add(s); s.Count > 0 {
			log.Printf("uiapp: inventory full, %d %s lost", s.Count, s.Item.Key)
		}
	}
	return c
}

// Open shows s, putting the ui layer on the app's stack when needed.
func (u *UI) Open(s ui.Screen) {
	if !u.open {
		u.open = true
		u.App.OpenScreen(u.adapter())
	}
	u.Ctx.Screens.Open(s)
}

// Screen opens s in the manager and returns the app.Screen to push (for
// Hooks.PauseScreen, which pushes the result itself).
func (u *UI) Screen(s ui.Screen) app.Screen {
	u.open = true
	u.Ctx.Screens.Open(s)
	return u.adapter()
}

func (u *UI) adapter() app.Screen { return (*screen)(u) }

// Draw implements app.HUD.
func (u *UI) Draw(fbW, fbH int) {
	if u.HUD == nil {
		return
	}
	px := u.resize(fbW, fbH)
	// The app has no HUD tick; follow its tick counter.
	t := int64(u.App.Tick)
	if u.lastTick == 0 || t < u.lastTick {
		u.lastTick = t
	}
	for ; u.lastTick < t; u.lastTick++ {
		u.HUD.Update()
	}
	partial := float32(u.App.Alpha())
	if u.App.Paused() {
		partial = 0
	}
	u.HUD.State.Crosshair = false // the app draws its own crosshair
	if p := u.App.Player; p != nil {
		u.HUD.ShowPlayer(p.Inv, u.Inventory.Item)
	}
	u.HUD.Draw(u.Ctx, partial)
	if u.Top != nil && u.Ctx.Screens.Len() == 0 {
		u.Top(u.Ctx)
	}
	u.canvas.Flush(fbW, fbH, px)
}

// resize runs ScaledResolution over the window size in points, as LWJGL2's
// Display size is (so the GUI has MC's size on a 2x display), and returns
// the framebuffer pixels per GUI pixel to draw with (Scale * the
// backing scale, sharp at 2x).
func (u *UI) resize(fbW, fbH int) float32 {
	ww, wh := fbW, fbH
	if u.App != nil && u.App.Win != nil {
		if w, h := u.App.Win.GetSize(); w > 0 && h > 0 {
			ww, wh = w, h
		}
	}
	u.Ctx.Resize(ww, wh)
	u.px = float64(u.Ctx.Scale) * float64(fbW) / float64(ww)
	return float32(u.px)
}

type screen UI

func (s *screen) Update() {
	s.Ctx.Ticks++
	s.Ctx.Screens.Update()
	if s.Ctx.Screens.Len() == 0 && s.open {
		s.Ctx.Screens.OnEmpty()
	}
}

func (s *screen) Draw(fbW, fbH int) {
	u := (*UI)(s)
	px := u.resize(fbW, fbH)
	u.Ctx.Screens.Draw(u.in.X, u.in.Y, float32(u.App.Alpha()))
	if u.Top != nil {
		u.Top(u.Ctx)
	}
	u.canvas.Flush(fbW, fbH, px)
}

// PausesGame forwards the top ui screen's PausesGame (GuiScreen's
// doesGuiPauseGame); screens without one pause.
func (s *screen) PausesGame() bool {
	if p, ok := s.Ctx.Screens.Top().(interface{ PausesGame() bool }); ok {
		return p.PausesGame()
	}
	return true
}

func (s *screen) HandleInput(ev app.Event) bool {
	u := (*UI)(s)
	sc := u.px // the events are in framebuffer pixels
	if sc <= 0 {
		sc = float64(max(u.Ctx.Scale, 1))
	}
	if u.Inventory.IsOpen() && ev.Type == app.KeyEvent && ev.Action == glfw.Press && u.App.Input.Bound(app.ActInventory, ev) {
		u.Inventory.Close() // the inventory key closes it too
		return true
	}
	var e ui.Event
	switch ev.Type {
	case app.KeyEvent:
		e = u.in.Key(int(ev.Key), int(ev.Action), int(ev.Mods))
	case app.CharEvent:
		e = u.in.Char(ev.Char)
	case app.ButtonEvent:
		u.in.Cursor(ev.X, ev.Y, sc)
		e = u.in.Button(int(ev.Button), int(ev.Action), int(ev.Mods))
	case app.ScrollEvent:
		if ev.Y == 0 {
			return true
		}
		e = u.in.Scroll(ev.Y)
	case app.CursorEvent:
		e = u.in.Cursor(ev.X, ev.Y, sc)
	default:
		return false
	}
	return u.Inventory.Input(e)
}
