package app

import (
	"math"
	"time"

	"github.com/memmaker/voxellib/platform/glfw"
)

// Button is a key (a glfw.Key value) or a mouse button (Mouse(b)).
type Button int

// mouseBase puts mouse buttons above every glfw key code.
const mouseBase = 1000

// Mouse is the Button for a mouse button.
func Mouse(b glfw.MouseButton) Button { return Button(mouseBase + int(b)) }

// Key is the Button for a key.
func Key(k glfw.Key) Button { return Button(k) }

// Actions the app itself handles; games add their own names.
const (
	ActForward    = "forward"
	ActBack       = "back"
	ActLeft       = "left"
	ActRight      = "right"
	ActJump       = "jump"
	ActSneak      = "sneak"
	ActSprint     = "sprint"
	ActAttack     = "attack"
	ActUse        = "use"
	ActPause      = "pause"
	ActScreenshot = "screenshot"
	ActStats      = "stats"
	// ActHotbar+"1".."9" select hotbar slots.
	ActHotbar = "hotbar."
)

// DefaultBindings are the reference's default controls (plus F2/F3 for the
// app's screenshot and frame stats).
func DefaultBindings() map[string][]Button {
	b := map[string][]Button{
		ActForward: {Key(glfw.KeyW)}, ActBack: {Key(glfw.KeyS)}, ActLeft: {Key(glfw.KeyA)}, ActRight: {Key(glfw.KeyD)},
		ActJump: {Key(glfw.KeySpace)}, ActSneak: {Key(glfw.KeyLeftShift)}, ActSprint: {Key(glfw.KeyLeftControl)},
		ActAttack: {Mouse(glfw.MouseButtonLeft)}, ActUse: {Mouse(glfw.MouseButtonRight)},
		ActPause: {Key(glfw.KeyEscape)}, ActScreenshot: {Key(glfw.KeyF2)}, ActStats: {Key(glfw.KeyF3)},
	}
	for i := 0; i < 9; i++ {
		b[ActHotbar+string(rune('1'+i))] = []Button{Key(glfw.Key1 + glfw.Key(i))}
	}
	return b
}

// EventType tells what an Event carries.
type EventType int

const (
	KeyEvent    EventType = iota // Key, Action, Mods
	CharEvent                    // Char (text input)
	ButtonEvent                  // Button, Action, Mods, X, Y
	ScrollEvent                  // X, Y: scroll offsets
	CursorEvent                  // X, Y
)

// Event is a raw input event for screens. Positions are framebuffer
// pixels, top-left origin.
type Event struct {
	Type   EventType
	Key    glfw.Key
	Button glfw.MouseButton
	Action glfw.Action
	Mods   glfw.ModifierKey
	Char   rune
	X, Y   float64
}

// Input maps keys and mouse buttons to named actions. Edit Bindings
// freely (a settings screen rebinds by replacing an action's buttons).
type Input struct {
	Bindings    map[string][]Button
	Sensitivity float64 // radians per pixel of mouse motion

	a        *App
	suppress map[string]bool // consumed press, until release
	virt     map[string]bool // actions held by Hold
	lastJump time.Time
	lastX    float64
	lastY    float64
}

func newInput(a *App) *Input {
	return &Input{Bindings: DefaultBindings(), Sensitivity: 0.0025, a: a, suppress: map[string]bool{}}
}

// Bind sets an action's buttons.
func (in *Input) Bind(action string, bs ...Button) { in.Bindings[action] = bs }

// Hold presses (down) or releases an action in-process, as a button bound
// to it would: the press or release is dispatched and Down reports it held
// until released. For scripted play, demos and tests.
func (in *Input) Hold(action string, down bool) {
	if in.virt == nil {
		in.virt = map[string]bool{}
	}
	if in.virt[action] == down {
		return
	}
	in.virt[action] = down
	in.dispatch(action, down)
}

// Down reports whether a button of the action is held (always false while
// the mouse is free) or Hold holds it.
func (in *Input) Down(action string) bool {
	if in.virt[action] {
		return true
	}
	if !in.a.Captured {
		return false
	}
	for _, b := range in.Bindings[action] {
		if in.held(b) {
			return true
		}
	}
	return false
}

func (in *Input) held(b Button) bool {
	w := in.a.Win
	if b >= mouseBase {
		return w.GetMouseButton(glfw.MouseButton(b-mouseBase)) == glfw.Press
	}
	return w.GetKey(glfw.Key(b)) == glfw.Press
}

// Capture grabs (on) or frees the mouse. Mouse look and actions work only
// while it is grabbed.
func (in *Input) Capture(on bool) {
	w := in.a.Win
	in.a.Captured = on
	if on {
		w.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		if glfw.RawMouseMotionSupported() {
			w.SetInputMode(glfw.RawMouseMotion, glfw.True)
		}
	} else {
		w.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
}

// toScreen sends ev to the top screen; true if a screen is open (the
// world then gets no input).
func (in *Input) toScreen(ev Event) bool {
	s := in.a.TopScreen()
	if s == nil {
		return false
	}
	if !s.HandleInput(ev) && ev.Action == glfw.Press && in.bound(ActPause, ev) {
		in.a.CloseScreen()
	}
	return true
}

func (in *Input) bound(action string, ev Event) bool {
	b := Key(ev.Key)
	if ev.Type == ButtonEvent {
		b = Mouse(ev.Button)
	}
	for _, x := range in.Bindings[action] {
		if x == b {
			return true
		}
	}
	return false
}

// button dispatches a press or release to every action bound to b.
func (in *Input) button(b Button, act glfw.Action) {
	if act == glfw.Repeat {
		return
	}
	for name, bs := range in.Bindings {
		for _, x := range bs {
			if x == b {
				in.dispatch(name, act == glfw.Press)
				break
			}
		}
	}
}

func (in *Input) dispatch(name string, pressed bool) {
	a := in.a
	if !pressed {
		delete(in.suppress, name)
	}
	if h := a.Hooks.Action; h != nil && h(name, pressed) {
		if pressed {
			in.suppress[name] = true
		}
		return
	}
	if !pressed {
		return
	}
	switch name {
	case ActPause:
		switch {
		case a.Hooks.PauseScreen != nil:
			a.OpenScreen(a.Hooks.PauseScreen())
		case a.Captured:
			in.Capture(false)
		default:
			a.Win.SetShouldClose(true)
		}
	case ActJump:
		if p := a.Player; p.CanFly && time.Since(in.lastJump) < 300*time.Millisecond && !p.InWater {
			p.ToggleFly()
		}
		in.lastJump = time.Now()
	case ActAttack:
		a.Player.click(Attack)
	case ActUse:
		a.Player.click(Use)
	case ActScreenshot:
		a.shotReq = true
	case ActStats:
		a.ShowDebug = !a.ShowDebug
	default:
		if len(name) == len(ActHotbar)+1 && name[:len(ActHotbar)] == ActHotbar {
			if i := int(name[len(ActHotbar)] - '1'); i >= 0 && i < a.Player.HotbarSize {
				a.Player.Selected = i
			}
		}
	}
}

// fbPos converts window coordinates to framebuffer pixels.
func (in *Input) fbPos(x, y float64) (float64, float64) {
	ww, wh := in.a.Win.GetSize()
	fw, fh := in.a.Win.GetFramebufferSize()
	if ww == 0 || wh == 0 {
		return x, y
	}
	return x * float64(fw) / float64(ww), y * float64(fh) / float64(wh)
}

// install sets the window's input callbacks.
func (in *Input) install() {
	a, w := in.a, in.a.Win
	w.SetKeyCallback(func(_ *glfw.Window, k glfw.Key, _ int, act glfw.Action, m glfw.ModifierKey) {
		if in.toScreen(Event{Type: KeyEvent, Key: k, Action: act, Mods: m}) {
			return
		}
		in.button(Key(k), act)
	})
	w.SetCharCallback(func(_ *glfw.Window, r rune) {
		in.toScreen(Event{Type: CharEvent, Char: r})
	})
	w.SetMouseButtonCallback(func(_ *glfw.Window, b glfw.MouseButton, act glfw.Action, m glfw.ModifierKey) {
		x, y := in.fbPos(w.GetCursorPos())
		if in.toScreen(Event{Type: ButtonEvent, Button: b, Action: act, Mods: m, X: x, Y: y}) {
			return
		}
		if !a.Captured {
			if act == glfw.Press {
				in.Capture(true)
			}
			return
		}
		in.button(Mouse(b), act)
	})
	w.SetScrollCallback(func(_ *glfw.Window, dx, dy float64) {
		if in.toScreen(Event{Type: ScrollEvent, X: dx, Y: dy}) || !a.Captured {
			return
		}
		p := a.Player
		if n := p.HotbarSize; n > 0 {
			if dy < 0 {
				p.Selected = (p.Selected + 1) % n
			} else if dy > 0 {
				p.Selected = (p.Selected + n - 1) % n
			}
		}
	})
	in.lastX, in.lastY = w.GetCursorPos()
	w.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		dx, dy := x-in.lastX, y-in.lastY
		in.lastX, in.lastY = x, y
		if a.TopScreen() != nil {
			fx, fy := in.fbPos(x, y)
			in.toScreen(Event{Type: CursorEvent, X: fx, Y: fy})
			return
		}
		if !a.Captured {
			return
		}
		a.Cam.Yaw += dx * in.Sensitivity
		a.Cam.Pitch = math.Max(-1.55, math.Min(1.55, a.Cam.Pitch-dy*in.Sensitivity))
	})
}
