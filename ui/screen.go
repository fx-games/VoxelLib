package ui

import (
	"image"
	"image/color"
	"io/fs"
	"log"
	"strings"

	"github.com/memmaker/voxellib/i18n"
)

// EventType tells what an Event carries.
type EventType int

const (
	KeyPress     EventType = iota // Key, Mods, Repeat
	KeyRelease                    // Key, Mods
	CharTyped                     // Char
	MousePress                    // Button, X, Y
	MouseRelease                  // Button, X, Y
	MouseMove                     // X, Y
	MouseScroll                   // DY (positive: wheel up), X, Y
)

// Event is an input event in GUI pixels. Key codes are GLFW's.
type Event struct {
	Type   EventType
	Key    int
	Mods   int
	Repeat bool
	Char   rune
	Button int // 0 left, 1 right, 2 middle
	X, Y   int
	DY     float64
}

// GLFW key codes the widgets use.
const (
	KeySpace     = 32
	KeyEscape    = 256
	KeyEnter     = 257
	KeyTab       = 258
	KeyBackspace = 259
	KeyDelete    = 261
	KeyRight     = 262
	KeyLeft      = 263
	KeyDown      = 264
	KeyUp        = 265
	KeyPageUp    = 266
	KeyPageDown  = 267
	KeyHome      = 268
	KeyEnd       = 269
	KeyKPEnter   = 335
	ModShift     = 1
	ModControl   = 2
)

// Screen is one GUI screen (GuiScreen): Init lays it out for ctx.W x ctx.H
// (on open, on return to it and on resize), Update runs at 20 Hz, Draw
// draws it, Input handles an event (true: consumed).
type Screen interface {
	Init(ctx *Context)
	Update()
	Draw(ctx *Context, mx, my int, partial float32)
	Input(ctx *Context, ev Event) bool
}

// Context is what screens and the HUD draw with: the canvas, the font, the
// translations, the game's assets and the current GUI size.
type Context struct {
	Canvas Canvas
	Assets fs.FS // resource pack stack: assets/<domain>/...
	Font   *Font
	Lang   *i18n.Catalog
	// W, H are the GUI size, Scale the framebuffer pixels per GUI pixel.
	W, H, Scale int
	// GUIScale is the setting (0 auto, 1..3); see ScaledResolution.
	GUIScale int
	// InWorld: a world is drawn behind the screens (translucent dark
	// background instead of the options dirt).
	InWorld bool
	// Ticks counts Update calls (animations, cursor blink, chat fade).
	Ticks int
	// Sound plays a sound event ("gui.button.press"); nil = silent.
	Sound func(name string)
	// Screens is the screen stack.
	Screens *Manager

	textures map[string]Texture
}

// NewContext loads the font from assets and makes a context with a screen
// manager.
func NewContext(c Canvas, assets fs.FS, lang *i18n.Catalog) (*Context, error) {
	ctx := &Context{Canvas: c, Assets: assets, Lang: lang, textures: map[string]Texture{}, W: 320, H: 240, Scale: 1}
	f, err := LoadFont(assets, c.NewTexture)
	if err != nil {
		return nil, err
	}
	ctx.Font = f
	ctx.Screens = &Manager{ctx: ctx}
	if lang == nil {
		ctx.Lang = i18n.New()
	}
	return ctx, nil
}

// T translates a key.
func (ctx *Context) T(key string, args ...any) string { return ctx.Lang.T(key, args...) }

// Play plays a sound event.
func (ctx *Context) Play(name string) {
	if ctx.Sound != nil {
		ctx.Sound(name)
	}
}

// AssetPath turns a resource location ("textures/gui/widgets.png",
// "lotr:gui/map.png") into its path in a pack.
func AssetPath(loc string) string {
	dom, p, ok := strings.Cut(loc, ":")
	if !ok {
		dom, p = "minecraft", loc
	}
	return "assets/" + dom + "/" + p
}

// Tex returns the texture at a resource location, loaded once; a missing one
// is Minecraft's magenta and black checker.
func (ctx *Context) Tex(loc string) Texture {
	if t, ok := ctx.textures[loc]; ok {
		return t
	}
	img, err := loadImage(ctx.Assets, AssetPath(loc))
	if err != nil {
		log.Printf("ui: %v", err)
		img = missingTexture()
	}
	t := ctx.Canvas.NewTexture(img)
	ctx.textures[loc] = t
	return t
}

// SetTexture registers an image under a resource location (generated
// images such as item icons or a rendered map).
// A texture already registered there is released.
func (ctx *Context) SetTexture(loc string, img image.Image) Texture {
	release(ctx.textures[loc])
	t := ctx.Canvas.NewTexture(img)
	ctx.textures[loc] = t
	return t
}

// release frees t's GPU resources when its Canvas has any.
func release(t Texture) {
	if r, ok := t.(interface{ Release() }); ok {
		r.Release()
	}
}

func missingTexture() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := color.NRGBA{0, 0, 0, 255}
			if (x < 8) != (y < 8) {
				c = color.NRGBA{248, 0, 248, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// Resize sets the GUI size for a framebuffer (ScaledResolution); it
// re-lays out the open screen when the size changed.
func (ctx *Context) Resize(fbW, fbH int) {
	w, h, s := ScaledResolution(fbW, fbH, ctx.GUIScale, ctx.Font.Unicode)
	if w == ctx.W && h == ctx.H && s == ctx.Scale {
		return
	}
	ctx.W, ctx.H, ctx.Scale = w, h, s
	if top := ctx.Screens.Top(); top != nil {
		top.Init(ctx)
	}
}

// DrawBackground is GuiScreen.drawDefaultBackground: a dark gradient over
// the world, else the options background tiled at 32 px, darkened.
func (ctx *Context) DrawBackground() {
	if ctx.InWorld {
		DrawGradientRect(ctx.Canvas, 0, 0, ctx.W, ctx.H, 0xC0101010, 0xD0101010)
		return
	}
	ctx.DrawDirt(0, ctx.H, 0, 0xFF404040)
}

// DrawDirt tiles options_background.png over rows y0..y1 at 32 GUI px a
// tile, scrolled by vOff GUI px, tinted by color (GuiSlot.overlayBackground).
func (ctx *Context) DrawDirt(y0, y1 int, vOff float32, color uint32) {
	ctx.DrawDirtFade(y0, y1, vOff, color, color)
}

// DrawDirtFade is DrawDirt with separate top and bottom colours.
func (ctx *Context) DrawDirtFade(y0, y1 int, vOff float32, top, bottom uint32) {
	t := ctx.Tex("textures/gui/options_background.png")
	ctx.Canvas.Quad(Quad{Tex: t, X0: 0, Y0: float32(y0), X1: float32(ctx.W), Y1: float32(y1),
		U0: 0, V0: (float32(y0) + vOff) / 32, U1: float32(ctx.W) / 32, V1: (float32(y1) + vOff) / 32, Top: top, Bottom: bottom})
}

// Manager is the screen stack. Only the top screen is drawn (as Minecraft
// shows one GuiScreen); Back returns to the one below.
type Manager struct {
	ctx   *Context
	stack []Screen
	// OnEmpty runs when the last screen closed (resume the game).
	OnEmpty func()
}

// Open puts s on top and lays it out.
func (m *Manager) Open(s Screen) {
	m.stack = append(m.stack, s)
	s.Init(m.ctx)
}

// Replace swaps the top screen for s.
func (m *Manager) Replace(s Screen) {
	if len(m.stack) > 0 {
		m.stack = m.stack[:len(m.stack)-1]
	}
	m.Open(s)
}

// Back closes the top screen.
func (m *Manager) Back() {
	if len(m.stack) == 0 {
		return
	}
	m.stack = m.stack[:len(m.stack)-1]
	if top := m.Top(); top != nil {
		top.Init(m.ctx)
	} else if m.OnEmpty != nil {
		m.OnEmpty()
	}
}

// CloseAll closes every screen.
func (m *Manager) CloseAll() {
	if len(m.stack) == 0 {
		return
	}
	m.stack = m.stack[:0]
	if m.OnEmpty != nil {
		m.OnEmpty()
	}
}

// Top returns the top screen or nil.
func (m *Manager) Top() Screen {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

// Len is the number of open screens.
func (m *Manager) Len() int { return len(m.stack) }

// Update ticks the top screen.
func (m *Manager) Update() {
	if top := m.Top(); top != nil {
		top.Update()
	}
}

// Draw draws the top screen.
func (m *Manager) Draw(mx, my int, partial float32) {
	if top := m.Top(); top != nil {
		top.Draw(m.ctx, mx, my, partial)
	}
}

// Input gives ev to the top screen; an unconsumed Escape goes Back.
func (m *Manager) Input(ev Event) bool {
	top := m.Top()
	if top == nil {
		return false
	}
	if top.Input(m.ctx, ev) {
		return true
	}
	if ev.Type == KeyPress && ev.Key == KeyEscape && !ev.Repeat {
		m.Back()
	}
	return true
}

// Base is embedded by screens: a widget list with default Update, Draw
// and Input that pass to the widgets.
type Base struct {
	Widgets []Widget
}

// Add appends widgets.
func (b *Base) Add(ws ...Widget) { b.Widgets = append(b.Widgets, ws...) }

func (b *Base) Update() {
	for _, w := range b.Widgets {
		if u, ok := w.(interface{ Update() }); ok {
			u.Update()
		}
	}
}

// DrawWidgets draws every widget.
func (b *Base) DrawWidgets(ctx *Context, mx, my int) {
	for _, w := range b.Widgets {
		w.Draw(ctx, mx, my)
	}
}

// Input passes ev to the widgets until one takes it. A mouse press first
// takes the focus away from every text field.
func (b *Base) Input(ctx *Context, ev Event) bool {
	if ev.Type == MousePress {
		for _, w := range b.Widgets {
			if tf, ok := w.(*TextField); ok {
				tf.Focused = false
			}
		}
	}
	for _, w := range b.Widgets {
		if w.Input(ctx, ev) {
			return true
		}
	}
	return false
}
