// Package uiapp runs ui screens and the ui HUD inside an app.App: it draws
// through render.GUIBatch after the world and converts app events
// (framebuffer pixels) to ui events (GUI pixels).
//
//	u, _ := uiapp.New(a, packs, lang)
//	a.HUD = u                       // the overlay, every frame
//	a.Hooks.PauseScreen = func() app.Screen { return u.Screen(ui.NewPauseMenu(...)) }
//	u.Open(&ui.DialogScreen{...})   // from game code
package uiapp

import (
	"image"
	"io/fs"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel/render"
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

	batch    render.GUIBatch
	mx, my   int
	px       float64 // framebuffer pixels per GUI pixel (resize)
	lastTick int64
	open     bool
}

// New loads the font from assets (resource pack stack) and wires the
// manager to the app's screen stack.
func New(a *app.App, assets fs.FS, lang *i18n.Catalog) (*UI, error) {
	u := &UI{App: a, HUD: &ui.HUD{}}
	ctx, err := ui.NewContext((*canvas)(u), assets, lang)
	if err != nil {
		return nil, err
	}
	u.Ctx = ctx
	ctx.InWorld = true
	ctx.Screens.OnEmpty = func() {
		if u.open {
			u.open = false
			a.CloseScreen()
		}
	}
	return u, nil
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
	u.HUD.Draw(u.Ctx, partial)
	if u.Top != nil && u.Ctx.Screens.Len() == 0 {
		u.Top(u.Ctx)
	}
	u.batch.Flush(fbW, fbH, px)
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
	u.Ctx.Screens.Draw(u.mx, u.my, float32(u.App.Alpha()))
	if u.Top != nil {
		u.Top(u.Ctx)
	}
	u.batch.Flush(fbW, fbH, px)
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
	e := ui.Event{X: u.mx, Y: u.my, Mods: int(ev.Mods)}
	switch ev.Type {
	case app.KeyEvent:
		e.Key, e.Repeat = int(ev.Key), ev.Action == glfw.Repeat
		e.Type = ui.KeyPress
		if ev.Action == glfw.Release {
			e.Type = ui.KeyRelease
		}
	case app.CharEvent:
		e.Type, e.Char = ui.CharTyped, ev.Char
	case app.ButtonEvent:
		u.mx, u.my = int(ev.X/sc), int(ev.Y/sc)
		e.X, e.Y, e.Button = u.mx, u.my, int(ev.Button)
		e.Type = ui.MousePress
		if ev.Action == glfw.Release {
			e.Type = ui.MouseRelease
		}
	case app.ScrollEvent:
		if ev.Y == 0 {
			return true
		}
		e.Type, e.DY = ui.MouseScroll, ev.Y
	case app.CursorEvent:
		u.mx, u.my = int(ev.X/sc), int(ev.Y/sc)
		e.Type, e.X, e.Y = ui.MouseMove, u.mx, u.my
	default:
		return false
	}
	return u.Ctx.Screens.Input(e)
}

// canvas implements ui.Canvas over the GUIBatch.
type canvas UI

func (c *canvas) NewTexture(img image.Image) ui.Texture { return render.NewGUITexture(img) }

func (c *canvas) UpdateTexture(t ui.Texture, img image.Image) bool {
	g, ok := t.(*render.GUITexture)
	return ok && g.Update(img)
}

func (c *canvas) Quad(q ui.Quad) {
	t, _ := q.Tex.(*render.GUITexture)
	c.batch.Linear(q.Linear)
	if q.Rot != 0 {
		c.batch.QuadRot(t, q.X0, q.Y0, q.X1, q.Y1, q.U0, q.V0, q.U1, q.V1, q.Skew, q.Top, q.Bottom, q.Rot, q.RX, q.RY)
		return
	}
	c.batch.Quad(t, q.X0, q.Y0, q.X1, q.Y1, q.U0, q.V0, q.U1, q.V1, q.Skew, q.Top, q.Bottom)
}

func (c *canvas) SetScissor(r image.Rectangle) {
	if r.Empty() {
		c.batch.Scissor(0, 0, 0, 0)
		return
	}
	c.batch.Scissor(r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

func (c *canvas) SetInvert(on bool) { c.batch.Invert(on) }

func (c *canvas) NewTarget(w, h int) ui.Texture { return render.NewGUITarget(w, h) }

func (c *canvas) SetTarget(t ui.Texture, clear uint32) {
	g, _ := t.(*render.GUITexture)
	c.batch.Target(g, clear)
}
