package ui

import "fmt"

// MouseButtonBase encodes mouse buttons as key codes: MouseButtonBase+b
// (the app package's encoding).
const MouseButtonBase = 1000

// KeyBinding is one rebindable control. Name and Category are lang keys;
// Key and Default are GLFW key codes or MouseButtonBase+button; 0 is
// unbound.
type KeyBinding struct {
	Name, Category string
	Key, Default   int
}

// KeyName is a key's display name the way the 1.7.10 reference shows LWJGL key
// names ("W", "SPACE", "LSHIFT"), mouse buttons as key.mouseButton.
func KeyName(ctx *Context, k int) string {
	if k >= MouseButtonBase {
		return ctx.T("key.mouseButton", k-MouseButtonBase+1)
	}
	switch {
	case k == 0:
		return "NONE"
	case k >= 'A' && k <= 'Z', k >= '0' && k <= '9':
		return string(rune(k))
	case k >= 290 && k <= 314:
		return fmt.Sprintf("F%d", k-289)
	case k >= 320 && k <= 329:
		return fmt.Sprintf("NUMPAD%d", k-320)
	}
	if n, ok := keyNames[k]; ok {
		return n
	}
	return fmt.Sprintf("KEY_%d", k)
}

var keyNames = map[int]string{
	32: "SPACE", 39: "APOSTROPHE", 44: "COMMA", 45: "MINUS", 46: "PERIOD", 47: "SLASH", 59: "SEMICOLON", 61: "EQUALS",
	91: "LBRACKET", 92: "BACKSLASH", 93: "RBRACKET", 96: "GRAVE", 256: "ESCAPE", 257: "RETURN", 258: "TAB", 259: "BACK",
	260: "INSERT", 261: "DELETE", 262: "RIGHT", 263: "LEFT", 264: "DOWN", 265: "UP", 266: "PRIOR", 267: "NEXT",
	268: "HOME", 269: "END", 280: "CAPITAL", 281: "SCROLL", 282: "NUMLOCK", 283: "SYSRQ", 284: "PAUSE",
	330: "DECIMAL", 331: "DIVIDE", 332: "MULTIPLY", 333: "SUBTRACT", 334: "ADD", 335: "NUMPADENTER", 336: "NUMPADEQUALS",
	340: "LSHIFT", 341: "LCONTROL", 342: "LMENU", 343: "LMETA", 344: "RSHIFT", 345: "RCONTROL", 346: "RMENU", 347: "RMETA",
}

// ControlsScreen is GuiControls: every binding grouped by category, a
// button to rebind (then press a key or click; Esc unbinds), a reset per
// row and "Reset Keys". Settings (mouse sensitivity, invert) go above the
// list in two columns.
type ControlsScreen struct {
	Base
	Bindings []*KeyBinding
	Settings []Setting
	OnChange func(b *KeyBinding)
	list     ScrollList
	rows     []ctrlRow
	waiting  *KeyBinding
	labelW   int
}

type ctrlRow struct {
	cat    string
	b      *KeyBinding
	change *Button
	reset  *Button
}

func (s *ControlsScreen) Init(ctx *Context) {
	s.Widgets = nil
	s.rows, s.labelW = nil, 0
	last := "\x00"
	for _, b := range s.Bindings {
		if b.Category != last {
			last = b.Category
			s.rows = append(s.rows, ctrlRow{cat: b.Category})
		}
		b := b
		r := ctrlRow{b: b}
		r.change = &Button{W: 75, H: 18, OnClick: func() { s.waiting = b }}
		r.reset = &Button{W: 50, H: 18, Label: ctx.T("controls.reset"), OnClick: func() { s.bind(b, b.Default) }}
		s.rows = append(s.rows, r)
		s.labelW = max(s.labelW, ctx.Font.Width(ctx.T(b.Name)))
	}
	s.list = ScrollList{Top: 63, Bottom: ctx.H - 32, SlotHeight: 20, ListWidth: 252,
		Count:    func() int { return len(s.rows) },
		DrawSlot: s.drawRow,
		OnClick:  func(int, bool) {},
	}
	s.list.layout(ctx)
	s.list.scrollBarX += 15
	for i := range s.Settings {
		st := &s.Settings[i]
		x, y := ctx.W/2-155+i%2*160, 18+24*(i>>1)
		if st.Slider {
			v := 0.0
			if st.Value != nil {
				v = st.Value()
			}
			s.Add(NewSlider(x, y, 150, v, func(float64) string { return st.caption(ctx) }, func(v float64) {
				if st.SetValue != nil {
					st.SetValue(v)
				}
			}))
		} else {
			var b *Button
			b = NewButton(x, y, 150, st.caption(ctx), func() {
				if st.OnClick != nil {
					st.OnClick()
				}
				b.Label = st.caption(ctx)
			})
			s.Add(b)
		}
	}
	s.Add(NewButton(ctx.W/2-155, ctx.H-29, 150, ctx.T("gui.done"), ctx.Screens.Back),
		NewButton(ctx.W/2-155+160, ctx.H-29, 150, ctx.T("controls.resetAll"), func() {
			for _, b := range s.Bindings {
				s.bind(b, b.Default)
			}
		}))
}

func (s *ControlsScreen) bind(b *KeyBinding, k int) {
	b.Key = k
	if s.OnChange != nil {
		s.OnChange(b)
	}
}

func (s *ControlsScreen) drawRow(ctx *Context, i, x, y, h, mx, my int) {
	r := &s.rows[i]
	f, c := ctx.Font, ctx.Canvas
	if r.b == nil {
		label := ctx.T(r.cat)
		f.Draw(c, label, ctx.W/2-f.Width(label)/2, y+s.list.SlotHeight-f.Height-1, 0xFFFFFF, false)
		return
	}
	f.Draw(c, ctx.T(r.b.Name), x+90-s.labelW, y+s.list.SlotHeight/2-f.Height/2, 0xFFFFFF, false)
	r.reset.X, r.reset.Y, r.reset.Disabled = x+190, y, r.b.Key == r.b.Default
	r.reset.Draw(ctx, mx, my)
	r.change.X, r.change.Y = x+105, y
	name := KeyName(ctx, r.b.Key)
	conflict := false
	if r.b.Key != 0 {
		for _, o := range s.Bindings {
			if o != r.b && o.Key == r.b.Key {
				conflict = true
			}
		}
	}
	switch {
	case s.waiting == r.b:
		name = "§f> §e" + name + "§f <"
	case conflict:
		name = "§c" + name
	}
	r.change.Label = name
	r.change.Draw(ctx, mx, my)
}

func (s *ControlsScreen) Draw(ctx *Context, mx, my int, partial float32) {
	ctx.DrawBackground()
	s.list.Draw(ctx, mx, my)
	ctx.Font.DrawCentered(ctx.Canvas, ctx.T("controls.title"), ctx.W/2, 8, 0xFFFFFF)
	s.DrawWidgets(ctx, mx, my)
}

func (s *ControlsScreen) Input(ctx *Context, ev Event) bool {
	if s.waiting != nil {
		switch ev.Type {
		case KeyPress:
			k := ev.Key
			if k == KeyEscape {
				k = 0
			}
			s.bind(s.waiting, k)
			s.waiting = nil
			return true
		case MousePress:
			s.bind(s.waiting, MouseButtonBase+ev.Button)
			s.waiting = nil
			return true
		}
	}
	if ev.Type == MousePress && ev.Y >= s.list.Top && ev.Y < s.list.Bottom {
		for i := range s.rows {
			r := &s.rows[i]
			if r.b != nil && (r.change.Input(ctx, ev) || r.reset.Input(ctx, ev)) {
				return true
			}
		}
	}
	if s.list.Input(ctx, ev) {
		return true
	}
	return s.Base.Input(ctx, ev)
}

// ControlsSetting opens a ControlsScreen for bindings.
func ControlsSetting(ctx *Context, bindings []*KeyBinding, onChange func(*KeyBinding), settings ...Setting) Setting {
	return Setting{Label: "options.controls", OnClick: func() {
		ctx.Screens.Open(&ControlsScreen{Bindings: bindings, OnChange: onChange, Settings: settings})
	}}
}
