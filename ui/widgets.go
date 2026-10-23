package ui

import (
	"image"
	"time"
)

// Widget is a control on a screen.
type Widget interface {
	Draw(ctx *Context, mx, my int)
	Input(ctx *Context, ev Event) bool
}

const widgetsTex = "textures/gui/widgets.png"

func inside(x, y, rx, ry, rw, rh int) bool { return x >= rx && y >= ry && x < rx+rw && y < ry+rh }

// Button is GuiButton: the widgets.png button in disabled, normal and
// hover states with a centred, shadowed label.
type Button struct {
	X, Y, W, H int
	Label      string
	Disabled   bool
	Hidden     bool
	Color      uint32 // label colour override (packedFGColour); 0: the reference's
	OnClick    func()
}

// NewButton makes a button; h is 20 as in the reference.
func NewButton(x, y, w int, label string, onClick func()) *Button {
	return &Button{X: x, Y: y, W: w, H: 20, Label: label, OnClick: onClick}
}

// Hovered reports whether the mouse is over the button.
func (b *Button) Hovered(mx, my int) bool { return inside(mx, my, b.X, b.Y, b.W, b.H) }

// drawFrame draws the two button halves for hover state k (0 disabled, 1
// normal, 2 hover).
func drawButtonFrame(ctx *Context, x, y, w, h, k int) {
	t := ctx.Tex(widgetsTex)
	DrawTexturedModalRect(ctx.Canvas, t, x, y, 0, 46+k*20, w/2, h)
	DrawTexturedModalRect(ctx.Canvas, t, x+w/2, y, 200-w/2, 46+k*20, w/2, h)
}

func buttonTextColor(override uint32, disabled, hover bool) uint32 {
	switch {
	case override != 0:
		return override
	case disabled:
		return 0xA0A0A0
	case hover:
		return 0xFFFFA0
	}
	return 0xE0E0E0
}

func (b *Button) Draw(ctx *Context, mx, my int) {
	if b.Hidden {
		return
	}
	hover := b.Hovered(mx, my)
	k := 1
	if b.Disabled {
		k = 0
	} else if hover {
		k = 2
	}
	drawButtonFrame(ctx, b.X, b.Y, b.W, b.H, k)
	ctx.Font.DrawCentered(ctx.Canvas, b.Label, b.X+b.W/2, b.Y+(b.H-8)/2, buttonTextColor(b.Color, b.Disabled, hover))
}

func (b *Button) Input(ctx *Context, ev Event) bool {
	if b.Hidden || b.Disabled || ev.Type != MousePress || ev.Button != 0 || !b.Hovered(ev.X, ev.Y) {
		return false
	}
	ctx.Play("gui.button.press")
	if b.OnClick != nil {
		b.OnClick()
	}
	return true
}

// Slider is GuiOptionSlider: a dark button with a knob; Value is 0..1.
type Slider struct {
	X, Y, W, H int
	Value      float64
	Text       func(v float64) string // label for a value
	OnChange   func(v float64)
	Disabled   bool
	dragging   bool
}

// NewSlider makes a 20 px high slider.
func NewSlider(x, y, w int, v float64, text func(float64) string, onChange func(float64)) *Slider {
	return &Slider{X: x, Y: y, W: w, H: 20, Value: v, Text: text, OnChange: onChange}
}

func (s *Slider) Draw(ctx *Context, mx, my int) {
	hover := inside(mx, my, s.X, s.Y, s.W, s.H)
	drawButtonFrame(ctx, s.X, s.Y, s.W, s.H, 0)
	t := ctx.Tex(widgetsTex)
	kx := s.X + int(float32(s.Value)*float32(s.W-8))
	DrawTexturedModalRect(ctx.Canvas, t, kx, s.Y, 0, 66, 4, 20)
	DrawTexturedModalRect(ctx.Canvas, t, kx+4, s.Y, 196, 66, 4, 20)
	label := ""
	if s.Text != nil {
		label = s.Text(s.Value)
	}
	ctx.Font.DrawCentered(ctx.Canvas, label, s.X+s.W/2, s.Y+(s.H-8)/2, buttonTextColor(0, s.Disabled, hover))
}

func (s *Slider) set(x int) {
	v := float64(x-(s.X+4)) / float64(s.W-8)
	v = min(max(v, 0), 1)
	if v != s.Value {
		s.Value = v
		if s.OnChange != nil {
			s.OnChange(v)
		}
	}
}

func (s *Slider) Input(ctx *Context, ev Event) bool {
	if s.Disabled {
		return false
	}
	switch ev.Type {
	case MousePress:
		if ev.Button == 0 && inside(ev.X, ev.Y, s.X, s.Y, s.W, s.H) {
			s.dragging = true
			s.set(ev.X)
			ctx.Play("gui.button.press")
			return true
		}
	case MouseMove:
		if s.dragging {
			s.set(ev.X)
			return true
		}
	case MouseRelease:
		if s.dragging {
			s.dragging = false
			return true
		}
	}
	return false
}

// TextField is GuiTextField: a black box with a grey border, a blinking
// cursor and horizontal scrolling (no selection).
type TextField struct {
	X, Y, W, H int
	Text       string
	MaxLen     int // 0: 32 like the reference
	Focused    bool
	NoBorder   bool
	OnEnter    func(text string)
	OnChange   func(text string)
	cursor     int // rune index
	scroll     int
	ticks      int
}

// NewTextField makes a text field.
func NewTextField(x, y, w, h int) *TextField { return &TextField{X: x, Y: y, W: w, H: h} }

func (t *TextField) Update() { t.ticks++ }

func (t *TextField) maxLen() int {
	if t.MaxLen > 0 {
		return t.MaxLen
	}
	return 32
}

func (t *TextField) innerWidth() int {
	if t.NoBorder {
		return t.W
	}
	return t.W - 8
}

// SetText replaces the text and puts the cursor at the end.
func (t *TextField) SetText(s string) {
	rs := []rune(s)
	if len(rs) > t.maxLen() {
		rs = rs[:t.maxLen()]
	}
	t.Text = string(rs)
	t.cursor, t.scroll = len(rs), 0
}

func (t *TextField) Draw(ctx *Context, mx, my int) {
	c, f := ctx.Canvas, ctx.Font
	if !t.NoBorder {
		DrawRect(c, t.X-1, t.Y-1, t.X+t.W+1, t.Y+t.H+1, 0xFFA0A0A0)
		DrawRect(c, t.X, t.Y, t.X+t.W, t.Y+t.H, 0xFF000000)
	}
	col := uint32(0xE0E0E0)
	rs := []rune(t.Text)
	t.fixScroll(f)
	j := t.cursor - t.scroll
	s := []rune(f.TrimToWidth(string(rs[t.scroll:]), t.innerWidth(), false))
	inView := j >= 0 && j <= len(s)
	blink := t.Focused && t.ticks/6%2 == 0 && inView
	l, y := t.X, t.Y
	if !t.NoBorder {
		l, y = t.X+4, t.Y+(t.H-8)/2
	}
	j1 := l
	if len(s) > 0 {
		head := s
		if inView {
			head = s[:j]
		}
		j1 = f.Draw(c, string(head), l, y, col, true)
	}
	atEnd := t.cursor < len(rs) || len(rs) >= t.maxLen()
	k1 := j1
	if !inView {
		if j > 0 {
			k1 = l + t.W
		} else {
			k1 = l
		}
	} else if atEnd {
		k1 = j1 - 1
		j1--
	}
	if len(s) > 0 && inView && j < len(s) {
		f.Draw(c, string(s[j:]), j1, y, col, true)
	}
	if blink {
		if atEnd {
			DrawRect(c, k1, y-1, k1+1, y+1+f.Height, 0xFFD0D0D0)
		} else {
			f.Draw(c, "_", k1, y, col, true)
		}
	}
}

func (t *TextField) fixScroll(f *Font) {
	n := len([]rune(t.Text))
	t.scroll = min(max(t.scroll, 0), n)
	if t.cursor < t.scroll {
		t.scroll = t.cursor
	}
	for t.scroll < t.cursor && f.Width(string([]rune(t.Text)[t.scroll:t.cursor])) > t.innerWidth() {
		t.scroll++
	}
}

func (t *TextField) edit(rs []rune) {
	t.Text = string(rs)
	if t.OnChange != nil {
		t.OnChange(t.Text)
	}
}

func (t *TextField) Input(ctx *Context, ev Event) bool {
	switch ev.Type {
	case MousePress:
		if !inside(ev.X, ev.Y, t.X, t.Y, t.W, t.H) {
			return false
		}
		t.Focused = true
		x := ev.X - t.X
		if !t.NoBorder {
			x -= 4
		}
		rs := []rune(t.Text)
		vis := ctx.Font.TrimToWidth(string(rs[min(t.scroll, len(rs)):]), t.innerWidth(), false)
		t.cursor = min(len([]rune(ctx.Font.TrimToWidth(vis, x, false)))+t.scroll, len(rs))
		return true
	case CharTyped:
		if !t.Focused || ev.Char == '§' || ev.Char < 32 || ev.Char == 127 {
			return t.Focused
		}
		rs := []rune(t.Text)
		if len(rs) >= t.maxLen() {
			return true
		}
		rs = append(rs[:t.cursor], append([]rune{ev.Char}, rs[t.cursor:]...)...)
		t.cursor++
		t.edit(rs)
		return true
	case KeyPress:
		if !t.Focused {
			return false
		}
		rs := []rune(t.Text)
		switch ev.Key {
		case KeyBackspace:
			if t.cursor > 0 {
				t.cursor--
				t.edit(append(rs[:t.cursor:t.cursor], rs[t.cursor+1:]...))
			}
		case KeyDelete:
			if t.cursor < len(rs) {
				t.edit(append(rs[:t.cursor:t.cursor], rs[t.cursor+1:]...))
			}
		case KeyLeft:
			t.cursor = max(t.cursor-1, 0)
		case KeyRight:
			t.cursor = min(t.cursor+1, len(rs))
		case KeyHome:
			t.cursor = 0
		case KeyEnd:
			t.cursor = len(rs)
		case KeyEnter, KeyKPEnter:
			if t.OnEnter != nil {
				t.OnEnter(t.Text)
			}
		case KeyEscape:
			return false
		}
		return true
	}
	return false
}

// ScrollList is GuiSlot: a list of fixed-height rows over a dark tiled
// background between the screen's top and bottom bars, with the reference's
// fades, selection box and scroll bar.
type ScrollList struct {
	Top, Bottom   int // list area rows
	SlotHeight    int
	ListWidth     int // 220 like the reference
	HeaderPadding int
	Count         func() int
	DrawSlot      func(ctx *Context, i, x, y, h, mx, my int)
	Selected      func(i int) bool // draws the selection box
	OnClick       func(i int, double bool)
	scrolled      float64
	dragBar       bool
	dragY         int
	dragStart     float64
	lastClick     time.Time
	lastSlot      int
	width, height int
	scrollBarX    int
}

func (l *ScrollList) layout(ctx *Context) {
	l.width, l.height = ctx.W, ctx.H
	if l.ListWidth == 0 {
		l.ListWidth = 220
	}
	l.scrollBarX = ctx.W/2 + 124
}

func (l *ScrollList) contentHeight() int { return l.Count()*l.SlotHeight + l.HeaderPadding }

func (l *ScrollList) maxScroll() int { return max(0, l.contentHeight()-(l.Bottom-l.Top-4)) }

func (l *ScrollList) clamp() { l.scrolled = min(max(l.scrolled, 0), float64(l.maxScroll())) }

// ScrollTo makes row i visible.
func (l *ScrollList) ScrollTo(i int) {
	y := float64(i*l.SlotHeight + l.HeaderPadding)
	if y < l.scrolled {
		l.scrolled = y
	} else if h := float64(l.Bottom - l.Top - 8); y+float64(l.SlotHeight) > l.scrolled+h {
		l.scrolled = y + float64(l.SlotHeight) - h
	}
	l.clamp()
}

func (l *ScrollList) Draw(ctx *Context, mx, my int) {
	l.layout(ctx)
	l.clamp()
	c := ctx.Canvas
	left, right := 0, l.width
	t := ctx.Tex("textures/gui/options_background.png")
	c.Quad(Quad{Tex: t, X0: float32(left), Y0: float32(l.Top), X1: float32(right), Y1: float32(l.Bottom),
		U0: float32(left) / 32, V0: (float32(l.Top) + float32(int(l.scrolled))) / 32, U1: float32(right) / 32, V1: (float32(l.Bottom) + float32(int(l.scrolled))) / 32,
		Top: 0xFF202020, Bottom: 0xFF202020})
	x := l.width/2 - l.ListWidth/2 + 2
	y0 := l.Top + 4 - int(l.scrolled)
	c.SetScissor(image.Rect(0, l.Top, l.width, l.Bottom))
	for i, n := 0, l.Count(); i < n; i++ {
		y := y0 + i*l.SlotHeight + l.HeaderPadding
		h := l.SlotHeight - 4
		if y > l.Bottom || y+h < l.Top {
			continue
		}
		if l.Selected != nil && l.Selected(i) {
			i2 := left + (l.width/2 - l.ListWidth/2)
			j2 := left + l.width/2 + l.ListWidth/2
			DrawRect(c, i2, y-2, j2, y+h+2, 0xFF808080)
			DrawRect(c, i2+1, y-1, j2-1, y+h+1, 0xFF000000)
		}
		if l.DrawSlot != nil {
			l.DrawSlot(ctx, i, x, y, h, mx, my)
		}
	}
	c.SetScissor(image.Rectangle{})
	ctx.DrawDirt(0, l.Top, 0, 0xFF404040)
	ctx.DrawDirt(l.Bottom, l.height, 0, 0xFF404040)
	DrawGradientRect(c, left, l.Top, right, l.Top+4, 0xFF000000, 0x00000000)
	DrawGradientRect(c, left, l.Bottom-4, right, l.Bottom, 0x00000000, 0xFF000000)
	if ms := l.maxScroll(); ms > 0 {
		bx0, bx1 := l.scrollBarX, l.scrollBarX+6
		span := l.Bottom - l.Top
		k2 := span * span / l.contentHeight()
		k2 = min(max(k2, 32), span-8)
		l2 := max(int(l.scrolled)*(span-k2)/ms+l.Top, l.Top)
		DrawRect(c, bx0, l.Top, bx1, l.Bottom, 0xFF000000)
		DrawRect(c, bx0, l2, bx1, l2+k2, 0xFF808080)
		DrawRect(c, bx0, l2, bx1-1, l2+k2-1, 0xFFC0C0C0)
	}
}

func (l *ScrollList) Input(ctx *Context, ev Event) bool {
	l.layout(ctx)
	switch ev.Type {
	case MouseScroll:
		if ev.Y < l.Top || ev.Y >= l.Bottom {
			return false
		}
		d := -1.0
		if ev.DY < 0 {
			d = 1
		}
		l.scrolled += d * float64(l.SlotHeight) / 2
		l.clamp()
		return true
	case MousePress:
		if ev.Button != 0 || ev.Y < l.Top || ev.Y >= l.Bottom {
			return false
		}
		if ev.X >= l.scrollBarX && ev.X < l.scrollBarX+6 {
			l.dragBar, l.dragY, l.dragStart = true, ev.Y, l.scrolled
			return true
		}
		j1 := ev.Y - l.Top - l.HeaderPadding + int(l.scrolled) - 4
		k1 := j1 / l.SlotHeight
		lx0, lx1 := l.width/2-l.ListWidth/2, l.width/2+l.ListWidth/2
		if ev.X >= lx0 && ev.X <= lx1 && k1 >= 0 && j1 >= 0 && k1 < l.Count() {
			double := k1 == l.lastSlot && time.Since(l.lastClick) < 250*time.Millisecond
			l.lastSlot, l.lastClick = k1, time.Now()
			if l.OnClick != nil {
				l.OnClick(k1, double)
			}
			return true
		}
	case MouseMove:
		if l.dragBar {
			span := l.Bottom - l.Top
			k2 := min(max(span*span/max(l.contentHeight(), 1), 32), span-8)
			if span > k2 {
				l.scrolled = l.dragStart + float64(ev.Y-l.dragY)*float64(l.maxScroll())/float64(span-k2)
			}
			l.clamp()
			return true
		}
	case MouseRelease:
		if l.dragBar {
			l.dragBar = false
			return true
		}
	}
	return false
}
