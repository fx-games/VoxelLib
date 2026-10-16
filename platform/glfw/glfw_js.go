//go:build js

package glfw

import (
	"image"
	"syscall/js"
)

type (
	Key         int
	Action      int
	ModifierKey int
	MouseButton int
	Hint        int
	InputMode   int
)

const (
	Release Action = 0
	Press   Action = 1
	Repeat  Action = 2

	ModShift   ModifierKey = 1
	ModControl ModifierKey = 2
	ModAlt     ModifierKey = 4
	ModSuper   ModifierKey = 8

	MouseButtonLeft   MouseButton = 0
	MouseButtonRight  MouseButton = 1
	MouseButtonMiddle MouseButton = 2

	True     = 1
	False    = 0
	DontCare = -1

	CursorMode     InputMode = 0x33001
	CursorNormal             = 0x34001
	CursorDisabled           = 0x34003

	Resizable               Hint = 0x20003
	Visible                 Hint = 0x20004
	Decorated               Hint = 0x20005
	DoubleBuffer            Hint = 0x21010
	ContextVersionMajor     Hint = 0x22002
	ContextVersionMinor     Hint = 0x22003
	OpenGLForwardCompatible Hint = 0x22006
	OpenGLDebugContext      Hint = 0x22007
	OpenGLProfile           Hint = 0x22008
	CocoaRetinaFramebuffer  Hint = 0x23001
	OpenGLCoreProfile            = 0x32001
)

const (
	KeySpace        Key = 32
	KeyApostrophe   Key = 39
	KeyComma        Key = 44
	KeyMinus        Key = 45
	KeyPeriod       Key = 46
	KeySlash        Key = 47
	Key0            Key = 48
	Key1            Key = 49
	Key2            Key = 50
	Key3            Key = 51
	Key4            Key = 52
	Key5            Key = 53
	Key6            Key = 54
	Key7            Key = 55
	Key8            Key = 56
	Key9            Key = 57
	KeySemicolon    Key = 59
	KeyEqual        Key = 61
	KeyA            Key = 65
	KeyB            Key = 66
	KeyC            Key = 67
	KeyD            Key = 68
	KeyE            Key = 69
	KeyF            Key = 70
	KeyG            Key = 71
	KeyH            Key = 72
	KeyI            Key = 73
	KeyJ            Key = 74
	KeyK            Key = 75
	KeyL            Key = 76
	KeyM            Key = 77
	KeyN            Key = 78
	KeyO            Key = 79
	KeyP            Key = 80
	KeyQ            Key = 81
	KeyR            Key = 82
	KeyS            Key = 83
	KeyT            Key = 84
	KeyU            Key = 85
	KeyV            Key = 86
	KeyW            Key = 87
	KeyX            Key = 88
	KeyY            Key = 89
	KeyZ            Key = 90
	KeyLeftBracket  Key = 91
	KeyBackslash    Key = 92
	KeyRightBracket Key = 93
	KeyGraveAccent  Key = 96
	KeyEscape       Key = 256
	KeyEnter        Key = 257
	KeyTab          Key = 258
	KeyBackspace    Key = 259
	KeyInsert       Key = 260
	KeyDelete       Key = 261
	KeyRight        Key = 262
	KeyLeft         Key = 263
	KeyDown         Key = 264
	KeyUp           Key = 265
	KeyPageUp       Key = 266
	KeyPageDown     Key = 267
	KeyHome         Key = 268
	KeyEnd          Key = 269
	KeyCapsLock     Key = 280
	KeyF1           Key = 290
	KeyF2           Key = 291
	KeyF3           Key = 292
	KeyF4           Key = 293
	KeyF5           Key = 294
	KeyF6           Key = 295
	KeyF7           Key = 296
	KeyF8           Key = 297
	KeyF9           Key = 298
	KeyF10          Key = 299
	KeyF11          Key = 300
	KeyF12          Key = 301
	KeyKP0          Key = 320
	KeyKP3          Key = 323
	KeyKP9          Key = 329
	KeyKPEnter      Key = 335
	KeyLeftShift    Key = 340
	KeyLeftControl  Key = 341
	KeyLeftAlt      Key = 342
	KeyLeftSuper    Key = 343
	KeyRightShift   Key = 344
	KeyRightControl Key = 345
	KeyRightAlt     Key = 346
	KeyRightSuper   Key = 347
)

// keyByCode maps KeyboardEvent.code to the GLFW key, so bindings stay physical.
var keyByCode = map[string]Key{
	"Space": KeySpace, "Quote": KeyApostrophe, "Comma": KeyComma, "Minus": KeyMinus,
	"Period": KeyPeriod, "Slash": KeySlash, "Semicolon": KeySemicolon, "Equal": KeyEqual,
	"BracketLeft": KeyLeftBracket, "Backslash": KeyBackslash, "BracketRight": KeyRightBracket,
	"Backquote": KeyGraveAccent, "Escape": KeyEscape, "Enter": KeyEnter, "Tab": KeyTab,
	"Backspace": KeyBackspace, "Insert": KeyInsert, "Delete": KeyDelete, "ArrowRight": KeyRight,
	"ArrowLeft": KeyLeft, "ArrowDown": KeyDown, "ArrowUp": KeyUp, "PageUp": KeyPageUp,
	"PageDown": KeyPageDown, "Home": KeyHome, "End": KeyEnd, "CapsLock": KeyCapsLock,
	"NumpadEnter": KeyKPEnter, "ShiftLeft": KeyLeftShift, "ControlLeft": KeyLeftControl,
	"AltLeft": KeyLeftAlt, "MetaLeft": KeyLeftSuper, "ShiftRight": KeyRightShift,
	"ControlRight": KeyRightControl, "AltRight": KeyRightAlt, "MetaRight": KeyRightSuper,
}

func init() {
	for i := 0; i < 26; i++ {
		keyByCode["Key"+string(rune('A'+i))] = KeyA + Key(i)
	}
	for i := 0; i < 10; i++ {
		keyByCode["Digit"+string(rune('0'+i))] = Key0 + Key(i)
		keyByCode["Numpad"+string(rune('0'+i))] = KeyKP0 + Key(i)
	}
	for i := 1; i <= 12; i++ {
		keyByCode["F"+itoa(i)] = KeyF1 + Key(i-1)
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return "1" + string(rune('0'+i-10))
}

type Cursor struct{}

type VidMode struct {
	Width, Height, RedBits, GreenBits, BlueBits, RefreshRate int
}

// Monitor is the browser window.
type Monitor struct{}

func GetPrimaryMonitor() *Monitor { return &Monitor{} }

func (m *Monitor) GetVideoMode() *VidMode {
	s := js.Global().Get("screen")
	return &VidMode{Width: s.Get("width").Int(), Height: s.Get("height").Int(), RefreshRate: 60}
}
func (m *Monitor) GetVideoModes() []*VidMode { return []*VidMode{m.GetVideoMode()} }
func (m *Monitor) GetPos() (int, int)        { return 0, 0 }
func (m *Monitor) GetWorkarea() (x, y, w, h int) {
	v := m.GetVideoMode()
	return 0, 0, v.Width, v.Height
}
func (m *Monitor) GetContentScale() (float32, float32) { return 1, 1 }

func Init() error                                { return nil }
func Terminate()                                 {}
func WindowHint(Hint, int)                       {}
func SwapInterval(int)                           {}
func DetachCurrentContext()                      {}
func CreateCursor(image.Image, int, int) *Cursor { return &Cursor{} }
func GetTime() float64                           { return js.Global().Get("performance").Call("now").Float() / 1000 }

// Window is the page's <canvas id="canvas">. Browser events are queued as they
// arrive and handed to the callbacks in PollEvents, the way GLFW does it.
type Window struct {
	canvas      js.Value
	w, h        int
	cx, cy      float64
	keys        map[Key]bool
	buttons     map[MouseButton]bool
	queue       []func()
	wantLock    bool
	shouldClose bool
	keyCb       func(*Window, Key, int, Action, ModifierKey)
	posCb       func(*Window, float64, float64)
	buttonCb    func(*Window, MouseButton, Action, ModifierKey)
	scrollCb    func(*Window, float64, float64)
	frame       chan struct{}
	raf         js.Func
	funcs       []js.Func
	presented   bool
}

func mods(e js.Value) ModifierKey {
	var m ModifierKey
	if e.Get("shiftKey").Bool() {
		m |= ModShift
	}
	if e.Get("ctrlKey").Bool() {
		m |= ModControl
	}
	if e.Get("altKey").Bool() {
		m |= ModAlt
	}
	if e.Get("metaKey").Bool() {
		m |= ModSuper
	}
	return m
}

func CreateWindow(width, height int, title string, monitor *Monitor, share *Window) (*Window, error) {
	doc := js.Global().Get("document")
	doc.Set("title", title)
	c := doc.Call("getElementById", "canvas")
	w := &Window{canvas: c, keys: map[Key]bool{}, buttons: map[MouseButton]bool{}, frame: make(chan struct{}, 1)}
	w.w, w.h = c.Get("clientWidth").Int(), c.Get("clientHeight").Int()
	if w.w == 0 || w.h == 0 {
		w.w, w.h = width, height
	}
	c.Set("width", w.w)
	c.Set("height", w.h)
	w.raf = js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case w.frame <- struct{}{}:
		default:
		}
		return nil
	})
	on := func(target js.Value, name string, f func(e js.Value)) {
		fn := js.FuncOf(func(_ js.Value, a []js.Value) any { f(a[0]); return nil })
		w.funcs = append(w.funcs, fn)
		target.Call("addEventListener", name, fn)
	}
	key := func(e js.Value, down bool) {
		k, ok := keyByCode[e.Get("code").String()]
		if !(e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool()) {
			e.Call("preventDefault")
		}
		if !ok {
			return
		}
		action := Release
		if down {
			action = Press
			if e.Get("repeat").Bool() {
				action = Repeat
			}
		}
		m := mods(e)
		w.queue = append(w.queue, func() {
			w.keys[k] = down
			if w.keyCb != nil {
				w.keyCb(w, k, 0, action, m)
			}
		})
	}
	on(doc, "keydown", func(e js.Value) { key(e, true) })
	on(doc, "keyup", func(e js.Value) { key(e, false) })
	on(c, "mousemove", func(e js.Value) {
		var x, y float64
		if w.locked() {
			x, y = e.Get("movementX").Float(), e.Get("movementY").Float()
			w.queue = append(w.queue, func() { w.moveTo(w.cx+x, w.cy+y) })
		} else {
			x, y = e.Get("offsetX").Float(), e.Get("offsetY").Float()
			w.queue = append(w.queue, func() { w.moveTo(x, y) })
		}
	})
	button := func(e js.Value, action Action) {
		b := MouseButton(e.Get("button").Int())
		switch b {
		case 1:
			b = MouseButtonMiddle
		case 2:
			b = MouseButtonRight
		}
		m := mods(e)
		w.queue = append(w.queue, func() {
			w.buttons[b] = action == Press
			if w.buttonCb != nil {
				w.buttonCb(w, b, action, m)
			}
		})
	}
	on(c, "mousedown", func(e js.Value) {
		if w.wantLock && !w.locked() {
			w.lock() // and still a click: where pointer lock is refused, clicks must work anyway
		}
		button(e, Press)
	})
	on(doc, "mouseup", func(e js.Value) { button(e, Release) })
	on(c, "contextmenu", func(e js.Value) { e.Call("preventDefault") })
	on(c, "wheel", func(e js.Value) {
		e.Call("preventDefault")
		dy := e.Get("deltaY").Float()
		if dy == 0 {
			return
		}
		y := 1.0
		if dy > 0 {
			y = -1
		}
		w.queue = append(w.queue, func() {
			if w.scrollCb != nil {
				w.scrollCb(w, 0, y)
			}
		})
	})
	on(js.Global(), "blur", func(js.Value) {
		// Keys held while the tab lost focus never send their keyup.
		w.queue = append(w.queue, func() {
			for k, down := range w.keys {
				if down {
					w.keys[k] = false
					if w.keyCb != nil {
						w.keyCb(w, k, 0, Release, 0)
					}
				}
			}
		})
	})
	return w, nil
}

func (w *Window) moveTo(x, y float64) {
	w.cx, w.cy = x, y
	if w.posCb != nil {
		w.posCb(w, x, y)
	}
}

func (w *Window) locked() bool {
	return js.Global().Get("document").Get("pointerLockElement").Equal(w.canvas)
}

func (w *Window) lock() {
	p := w.canvas.Call("requestPointerLock")
	if p.Truthy() && p.Get("catch").Truthy() {
		p.Call("catch", js.FuncOf(func(js.Value, []js.Value) any { return nil }))
	}
}

func PollEvents() {
	for _, w := range windows {
		q := w.queue
		w.queue = nil
		for _, f := range q {
			f()
		}
	}
}

var windows []*Window

func (w *Window) MakeContextCurrent() { windows = append(windows, w) }

// SwapBuffers waits for the next animation frame: the browser presents what was
// drawn when control returns to it, and this is where the frame loop yields.
func (w *Window) SwapBuffers() {
	js.Global().Call("requestAnimationFrame", w.raf)
	<-w.frame
	if !w.presented {
		w.presented = true
		if hook := js.Global().Get("__firstFrame"); hook.Type() == js.TypeFunction {
			hook.Invoke() // the page drops its loading message
		}
	}
}

func (w *Window) ShouldClose() bool                            { return w.shouldClose }
func (w *Window) SetShouldClose(v bool)                        { w.shouldClose = v }
func (w *Window) GetSize() (int, int)                          { return w.w, w.h }
func (w *Window) GetFramebufferSize() (int, int)               { return w.w, w.h }
func (w *Window) SetSize(int, int)                             {}
func (w *Window) GetPos() (int, int)                           { return 0, 0 }
func (w *Window) SetPos(int, int)                              {}
func (w *Window) GetMonitor() *Monitor                         { return nil }
func (w *Window) SetMonitor(*Monitor, int, int, int, int, int) {}
func (w *Window) GetAttrib(Hint) int                           { return True }
func (w *Window) SetAttrib(Hint, int)                          {}
func (w *Window) SetCursor(*Cursor)                            {}
func (w *Window) GetCursorPos() (float64, float64)             { return w.cx, w.cy }
func (w *Window) GetKey(k Key) Action {
	if w.keys[k] {
		return Press
	}
	return Release
}

func (w *Window) SetInputMode(mode InputMode, value int) {
	if mode != CursorMode {
		return
	}
	w.wantLock = value == CursorDisabled
	if w.wantLock {
		w.lock() // only succeeds inside a user gesture; mousedown retries
	} else if w.locked() {
		js.Global().Get("document").Call("exitPointerLock")
	}
}

func (w *Window) SetKeyCallback(f func(*Window, Key, int, Action, ModifierKey)) { w.keyCb = f }
func (w *Window) SetCursorPosCallback(f func(*Window, float64, float64))        { w.posCb = f }
func (w *Window) SetMouseButtonCallback(f func(*Window, MouseButton, Action, ModifierKey)) {
	w.buttonCb = f
}
func (w *Window) SetScrollCallback(f func(*Window, float64, float64)) { w.scrollCb = f }
func (w *Window) SetDropCallback(func(*Window, []string))             {}
func (w *Window) SetFramebufferSizeCallback(func(*Window, int, int))  {}

// Added for voxellib. Pointer lock already delivers raw motion.
const (
	RawMouseMotion InputMode = 0x33005
	Samples        Hint      = 0x2100D
	Focused        Hint      = 0x20001
	FocusOnShow    Hint      = 0x2000C
)

func RawMouseMotionSupported() bool { return true }

func (w *Window) GetMouseButton(b MouseButton) Action {
	if w.buttons[b] {
		return Press
	}
	return Release
}
