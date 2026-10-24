package ui

import (
	"slices"

	"github.com/memmaker/voxellib/inventory"
)

// GLFWInput turns GLFW callback arguments into Events in GUI pixels. It
// keeps the cursor (X, Y), which every event carries.
type GLFWInput struct{ X, Y int }

// GLFW's actions.
const (
	glfwRelease = 0
	glfwRepeat  = 2
)

func (in *GLFWInput) Key(key, action, mods int) Event {
	t := KeyPress
	if action == glfwRelease {
		t = KeyRelease
	}
	return Event{Type: t, Key: key, Mods: mods, Repeat: action == glfwRepeat, X: in.X, Y: in.Y}
}

func (in *GLFWInput) Char(r rune) Event { return Event{Type: CharTyped, Char: r, X: in.X, Y: in.Y} }

// Button is a mouse button at the cursor last given to Cursor.
func (in *GLFWInput) Button(button, action, mods int) Event {
	t := MousePress
	if action == glfwRelease {
		t = MouseRelease
	}
	return Event{Type: t, Button: button, Mods: mods, X: in.X, Y: in.Y}
}

func (in *GLFWInput) Scroll(dy float64) Event {
	return Event{Type: MouseScroll, DY: dy, X: in.X, Y: in.Y}
}

// Cursor moves the cursor to x, y, given in px units per GUI pixel.
func (in *GLFWInput) Cursor(x, y, px float64) Event {
	px = max(px, 1)
	in.X, in.Y = int(x/px), int(y/px)
	return Event{Type: MouseMove, X: in.X, Y: in.Y}
}

// PlayerGUI runs a player's inventory for a game with its own render
// loop: the HUD hotbar (filled from the player every frame), the
// inventory screen, and the hotbar keys (1-9, the wheel: Player.Hold)
// while no screen is open. The window's GLFW callbacks go straight in;
// each returns whether the GUI took the event.
//
//	pg := ui.NewPlayerGUI(ctx, p, c, item)
//	win.SetKeyCallback(func(_ *glfw.Window, k glfw.Key, _ int, a glfw.Action, m glfw.ModifierKey) { pg.Key(int(k), int(a), int(m)) })
//	// Char, MouseButton, Scroll and CursorPos likewise
//	pg.Update()                       // every tick
//	pg.Draw(fbW, fbH, ww, wh, alpha)  // every frame, after the world
type PlayerGUI struct {
	Ctx       *Context
	HUD       *HUD
	Player    *inventory.Player
	Container *inventory.Container
	// Item is what a stack shows (nil: an empty slot).
	Item func(*inventory.Stack) ItemStack
	// Keys open and close the screen (vanilla's inventory key, E).
	Keys []int
	// SwapKeys swap the held stack with the off hand while no screen is
	// open (vanilla's swap-offhand key, F), for a player with an off hand;
	// OnSwap runs after a swap (the game stops the item in use there).
	SwapKeys []int
	OnSwap   func()
	// The screen's InventoryOptions (Equip is the player's, the off-hand
	// slot shown when the player has one).
	EquipAt                        [][2]int
	NoEquip, NoPreview, NoCrafting bool
	Model                          func(ctx *Context, x, y, mx, my int)
	// OnOpen runs after the screen opened, OnClose after it closed and
	// the cursor stack was dropped (Container.Close).
	OnOpen, OnClose func()
	// In is the cursor, in GUI pixels.
	In GLFWInput

	open bool
}

// NewPlayerGUI wires p's container c to ctx: the screen manager running
// empty closes the inventory.
func NewPlayerGUI(ctx *Context, p *inventory.Player, c *inventory.Container, item func(*inventory.Stack) ItemStack) *PlayerGUI {
	g := &PlayerGUI{Ctx: ctx, HUD: &HUD{}, Player: p, Container: c, Item: item, Keys: []int{KeyE}, SwapKeys: []int{KeyF}}
	ctx.Screens.OnEmpty = g.Close
	return g
}

func (g *PlayerGUI) IsOpen() bool { return g.open }

// Open shows the inventory screen.
func (g *PlayerGUI) Open() {
	if g.open {
		return
	}
	g.open = true
	lay := InventoryLayout(InventoryOptions{Equip: g.Player.EquipSlots(), EquipAt: g.EquipAt,
		NoEquip: g.NoEquip, NoPreview: g.NoPreview, NoCrafting: g.NoCrafting, Model: g.Model,
		OffHand: g.Player.OffSlot() >= 0})
	g.Ctx.Screens.Open(&ContainerScreen{Layout: lay, Model: NewContainerView(g.Container, g.Item, nil)})
	if g.OnOpen != nil {
		g.OnOpen()
	}
}

// Close closes it, dropping what the cursor carries.
func (g *PlayerGUI) Close() {
	if !g.open {
		return
	}
	g.open = false
	g.Ctx.Screens.CloseAll()
	g.Container.Close()
	if g.OnClose != nil {
		g.OnClose()
	}
}

func (g *PlayerGUI) Toggle() {
	if g.open {
		g.Close()
	} else {
		g.Open()
	}
}

// Input routes ev: Keys toggle the screen, an open screen takes
// everything, else 1-9 and the wheel move the held slot.
func (g *PlayerGUI) Input(ev Event) bool {
	if ev.Type == KeyPress && !ev.Repeat && slices.Contains(g.Keys, ev.Key) && (g.open || g.Ctx.Screens.Len() == 0) {
		g.Toggle()
		return true
	}
	if g.Ctx.Screens.Len() > 0 {
		return g.Ctx.Screens.Input(ev)
	}
	switch {
	case ev.Type == KeyPress && ev.Key >= Key1 && ev.Key <= Key1+8:
		g.Player.Hold(ev.Key - Key1)
	case ev.Type == KeyPress && !ev.Repeat && slices.Contains(g.SwapKeys, ev.Key) && g.Player.OffSlot() >= 0:
		if g.Player.SwapHands() && g.OnSwap != nil {
			g.OnSwap()
		}
	case ev.Type == MouseScroll && ev.DY != 0:
		d := 1
		if ev.DY > 0 {
			d = 8 // wheel up: the slot before
		}
		g.Player.Hold((g.Player.Held + d) % 9)
	default:
		return false
	}
	return true
}

// The GLFW callbacks; the cursor is in window points.
func (g *PlayerGUI) Key(key, action, mods int) bool { return g.Input(g.In.Key(key, action, mods)) }
func (g *PlayerGUI) Char(r rune) bool               { return g.Input(g.In.Char(r)) }
func (g *PlayerGUI) Button(button, action, mods int) bool {
	return g.Input(g.In.Button(button, action, mods))
}
func (g *PlayerGUI) Scroll(dx, dy float64) bool { return g.Input(g.In.Scroll(dy)) }
func (g *PlayerGUI) Cursor(x, y float64) bool {
	return g.Input(g.In.Cursor(x, y, float64(g.Ctx.Scale)))
}

// Update is the 20 Hz tick: the HUD's timers and the open screen.
func (g *PlayerGUI) Update() {
	g.Ctx.Ticks++
	g.HUD.Update()
	g.Ctx.Screens.Update()
}

// Draw sizes the GUI for the ww x wh window (points), draws the HUD and
// the open screen, and flushes a canvas that has Flush (guicanvas) onto
// the fbW x fbH framebuffer.
func (g *PlayerGUI) Draw(fbW, fbH, ww, wh int, partial float32) {
	if ww <= 0 || wh <= 0 {
		ww, wh = fbW, fbH
	}
	g.Ctx.Resize(ww, wh)
	st := &g.HUD.State
	for i := range st.Hotbar {
		st.Hotbar[i] = withGlint(g.Item(g.Player.Main[i]), g.Player.Main[i])
	}
	st.Selected = g.Player.Held
	st.OffHand = ItemStack{}
	if g.Player.OffSlot() >= 0 {
		st.OffHand = withGlint(g.Item(g.Player.Off), g.Player.Off)
	}
	g.HUD.Draw(g.Ctx, partial)
	g.Ctx.Screens.Draw(g.In.X, g.In.Y, partial)
	if f, ok := g.Ctx.Canvas.(interface {
		Flush(fbW, fbH int, px float32)
	}); ok {
		f.Flush(fbW, fbH, float32(g.Ctx.Scale)*float32(fbW)/float32(ww))
	}
}
