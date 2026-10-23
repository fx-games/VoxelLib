package ui

// DialogBlock is one speech block: the speaker's name (drawn left of the
// text, "Name: ") and the text, wrapped to the dialog's width.
type DialogBlock struct {
	Name  string
	Text  string
	Color uint32 // 0: 0xE0E0E0
}

// DialogOption is a reply the player can pick.
type DialogOption struct {
	Text  string
	Color uint32 // 0: 0xE0E0E0
}

// DialogScreen is a conversation in the CustomNPC style: the darkened
// world, the NPC on the left (DrawNPC), the speech blocks with the speaker
// before the first line, paged when longer than the text area (click,
// Space or Enter for the next page), then a line and the options with a
// ">" marker, picked with the mouse or Up/Down + Enter (number keys too).
type DialogScreen struct {
	Blocks  []DialogBlock
	Options []DialogOption
	// OnChoose gets the picked option; the game then calls Say/SetOptions
	// to continue or closes the screen.
	OnChoose func(i int)
	// DrawNPC draws the speaker (feet at x,y).
	DrawNPC   func(ctx *Context, x, y int)
	NoDarken  bool
	TextWidth int // 0: 300
	// OnClose runs when the player leaves with Esc (no option picked).
	OnClose func()

	selected  int
	lines     []dialogLine
	reveal    int // lines shown so far
	rows      int
	left, top int
	optY      int
	width     int
}

type dialogLine struct {
	name  string // set on a block's first line
	text  string
	color uint32
	block int
}

// Say appends a block (the previous ones stay above it) and shows its
// first page.
func (d *DialogScreen) Say(ctx *Context, b DialogBlock) {
	d.Blocks = append(d.Blocks, b)
	shown := len(d.lines)
	d.layout(ctx)
	d.reveal = min(shown+d.rows, len(d.lines))
	d.selected = 0
}

// SetOptions replaces the options.
func (d *DialogScreen) SetOptions(opts []DialogOption) { d.Options, d.selected = opts, 0 }

func (d *DialogScreen) layout(ctx *Context) {
	d.width = d.TextWidth
	if d.width == 0 {
		d.width = 300
	}
	d.width = min(d.width, ctx.W-140)
	d.left = (ctx.W-d.width)/2 + 40
	d.top = 20
	d.lines = d.lines[:0]
	for bi, b := range d.Blocks {
		col := b.Color
		if col == 0 {
			col = 0xE0E0E0
		}
		for i, l := range ctx.Font.Wrap(ctx.T(b.Text), d.width) {
			dl := dialogLine{text: l, color: col, block: bi}
			if i == 0 {
				dl.name = b.Name
			}
			d.lines = append(d.lines, dl)
		}
		d.lines = append(d.lines, dialogLine{block: bi}) // gap between blocks
	}
	if n := len(d.lines); n > 0 {
		d.lines = d.lines[:n-1]
	}
	nOpt := max(len(d.Options), 1)
	d.optY = ctx.H - 12 - nOpt*ctx.Font.Height
	d.rows = max((d.optY-d.top-8)/ctx.Font.Height, 1)
}

func (d *DialogScreen) Init(ctx *Context) {
	shown := d.reveal
	d.layout(ctx)
	if shown == 0 {
		shown = min(d.rows, len(d.lines))
	}
	d.reveal = min(max(shown, 1), len(d.lines))
}

func (d *DialogScreen) Update() {}

func (d *DialogScreen) more() bool { return d.reveal < len(d.lines) }

func (d *DialogScreen) Draw(ctx *Context, mx, my int, partial float32) {
	c, f := ctx.Canvas, ctx.Font
	if !d.NoDarken {
		ctx.DrawBackground()
	}
	if d.DrawNPC != nil {
		d.DrawNPC(ctx, d.left-70, ctx.H-20)
	}
	first := max(d.reveal-d.rows, 0)
	y := d.top
	for _, l := range d.lines[first:d.reveal] {
		if l.name != "" {
			n := l.name + ": "
			f.Draw(c, n, d.left-4-f.Width(n), y, l.color, true)
		}
		f.Draw(c, l.text, d.left, y, l.color, true)
		y += f.Height
	}
	if d.more() {
		if ctx.Ticks/10%2 == 0 {
			f.Draw(c, "▼", d.left+d.width-8, d.optY-f.Height, 0xE0E0E0, true)
		}
		return
	}
	if len(d.Options) == 0 {
		return
	}
	DrawRect(c, d.left-60, d.optY-f.Height/3, d.left+d.width+60, d.optY-f.Height/3+1, 0xFFFFFFFF)
	if i := d.optionAt(mx, my); i >= 0 {
		d.selected = i
	}
	for i, o := range d.Options {
		oy := d.optY + 2 + i*f.Height
		if i == d.selected {
			f.Draw(c, ">", d.left-10, oy, 0xE0E0E0, true)
		}
		col := o.Color
		if col == 0 {
			col = 0xE0E0E0
		}
		f.Draw(c, ctx.T(o.Text), d.left, oy, col, true)
	}
}

func (d *DialogScreen) optionAt(mx, my int) int {
	if d.more() || my < d.optY+2 || mx < d.left-10 || mx > d.left+d.width {
		return -1
	}
	i := (my - d.optY - 2) / 9
	if i >= len(d.Options) {
		return -1
	}
	return i
}

func (d *DialogScreen) choose(ctx *Context, i int) {
	if i < 0 || i >= len(d.Options) {
		return
	}
	ctx.Play("gui.button.press")
	if d.OnChoose != nil {
		d.OnChoose(i)
	}
}

func (d *DialogScreen) Input(ctx *Context, ev Event) bool {
	switch ev.Type {
	case MousePress:
		if d.more() {
			d.reveal = min(d.reveal+d.rows, len(d.lines))
			return true
		}
		d.choose(ctx, d.optionAt(ev.X, ev.Y))
		return true
	case KeyPress:
		switch {
		case ev.Key == KeyEscape:
			if d.OnClose != nil {
				d.OnClose()
			}
			return false
		case d.more() && (ev.Key == KeySpace || ev.Key == KeyEnter || ev.Key == KeyKPEnter):
			d.reveal = min(d.reveal+d.rows, len(d.lines))
		case ev.Key == KeyUp && len(d.Options) > 0:
			d.selected = (d.selected + len(d.Options) - 1) % len(d.Options)
		case ev.Key == KeyDown && len(d.Options) > 0:
			d.selected = (d.selected + 1) % len(d.Options)
		case ev.Key == KeyEnter || ev.Key == KeyKPEnter || ev.Key == KeySpace:
			d.choose(ctx, d.selected)
		case ev.Key >= '1' && ev.Key <= '9' && !d.more():
			d.choose(ctx, ev.Key-'1')
		}
		return true
	case MouseScroll:
		if n := len(d.Options); n > 0 && !d.more() {
			if ev.DY > 0 {
				d.selected = (d.selected + n - 1) % n
			} else {
				d.selected = (d.selected + 1) % n
			}
		}
		return true
	}
	return false
}
