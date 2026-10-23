package ui

// MenuItem is one menu button. Labels and titles in this file are lang keys or
// plain text (translated with Context.T when the screen is laid out, so a
// language switch shows at once).
type MenuItem struct {
	Label    string
	OnClick  func()
	Disabled bool
}

// MenuRow is a row of one (200 px) or two (98 px) buttons, Y below the
// menu's origin.
type MenuRow struct {
	Y     int
	Items []MenuItem
}

// MenuScreen is a column of Minecraft menu buttons with an optional logo,
// title and footer: the main menu and the pause menu are made with it.
type MenuScreen struct {
	Base
	Title  string // centred white text at TitleY
	TitleY int
	Logo   *Sprite // drawn centred at y 30
	Rows   []MenuRow
	// Origin gives the rows' origin y for the GUI height.
	Origin func(h int) int
	// Background draws behind everything (a panorama, a map); nil:
	// Context.DrawBackground.
	Background  func(ctx *Context, partial float32)
	FooterLeft  string
	FooterRight string
	// OnLanguage, when set, adds the globe button left of the bottom row.
	OnLanguage func()
	langBtn    *Button
}

func (m *MenuScreen) Init(ctx *Context) {
	m.Widgets = nil
	y0 := ctx.H / 4
	if m.Origin != nil {
		y0 = m.Origin(ctx.H)
	}
	for _, r := range m.Rows {
		y := y0 + r.Y
		switch len(r.Items) {
		case 1:
			it := r.Items[0]
			b := NewButton(ctx.W/2-100, y, 200, ctx.T(it.Label), it.OnClick)
			b.Disabled = it.Disabled
			m.Add(b)
		case 2:
			for i, it := range r.Items {
				b := NewButton(ctx.W/2-100+i*102, y, 98, ctx.T(it.Label), it.OnClick)
				b.Disabled = it.Disabled
				m.Add(b)
			}
		}
	}
	m.langBtn = nil
	if m.OnLanguage != nil && len(m.Rows) > 0 {
		m.langBtn = &Button{X: ctx.W/2 - 124, Y: y0 + m.Rows[len(m.Rows)-1].Y, W: 20, H: 20, OnClick: m.OnLanguage}
	}
}

func (m *MenuScreen) Draw(ctx *Context, mx, my int, partial float32) {
	if m.Background != nil {
		m.Background(ctx, partial)
	} else {
		ctx.DrawBackground()
	}
	if m.Logo != nil {
		m.Logo.Draw(ctx.Canvas, float32(ctx.W/2)-m.Logo.W/2, 30)
	}
	if m.Title != "" {
		ctx.Font.DrawCentered(ctx.Canvas, ctx.T(m.Title), ctx.W/2, m.TitleY, 0xFFFFFF)
	}
	if m.FooterLeft != "" {
		ctx.Font.Draw(ctx.Canvas, m.FooterLeft, 2, ctx.H-10, 0xFFFFFF, true)
	}
	if m.FooterRight != "" {
		ctx.Font.Draw(ctx.Canvas, m.FooterRight, ctx.W-ctx.Font.Width(m.FooterRight)-2, ctx.H-10, 0xFFFFFF, true)
	}
	m.DrawWidgets(ctx, mx, my)
	if b := m.langBtn; b != nil { // GuiButtonLanguage: globe icon at 0,106
		v := 106
		if b.Hovered(mx, my) {
			v += 20
		}
		DrawTexturedModalRect(ctx.Canvas, ctx.Tex(widgetsTex), b.X, b.Y, 0, v, 20, 20)
	}
}

func (m *MenuScreen) Input(ctx *Context, ev Event) bool {
	if m.langBtn != nil && m.langBtn.Input(ctx, ev) {
		return true
	}
	return m.Base.Input(ctx, ev)
}

// NewMainMenu lays out a main menu like GuiMainMenu: items (up to three
// full-width buttons) from height/4+48 every 24 px, then Options and Quit
// side by side.
func NewMainMenu(ctx *Context, items []MenuItem, options, quit, language func()) *MenuScreen {
	m := &MenuScreen{Origin: func(h int) int { return h/4 + 48 }, OnLanguage: language}
	for i, it := range items {
		m.Rows = append(m.Rows, MenuRow{Y: i * 24, Items: []MenuItem{it}})
	}
	m.Rows = append(m.Rows, MenuRow{Y: 72 + 12, Items: []MenuItem{
		{Label: "menu.options", OnClick: options}, {Label: "menu.quit", OnClick: quit}}})
	return m
}

// NewPauseMenu lays out GuiIngameMenu: "Back to Game", up to two extra
// buttons in the achievements/stats row, Options, and "Save and Quit to
// Title".
func NewPauseMenu(ctx *Context, resume, options, quit func(), extra ...MenuItem) *MenuScreen {
	m := &MenuScreen{Title: "menu.game", TitleY: 40, Origin: func(h int) int { return h/4 - 16 }}
	m.Rows = append(m.Rows, MenuRow{Y: 24, Items: []MenuItem{{Label: "menu.returnToGame", OnClick: resume}}})
	if len(extra) > 0 {
		m.Rows = append(m.Rows, MenuRow{Y: 48, Items: extra[:min(len(extra), 2)]})
	}
	m.Rows = append(m.Rows,
		MenuRow{Y: 96, Items: []MenuItem{{Label: "menu.options", OnClick: options}}},
		MenuRow{Y: 120, Items: []MenuItem{{Label: "menu.returnToMenu", OnClick: quit}}})
	return m
}

// ConfirmScreen is GuiYesNo: two lines of text and Yes/No buttons.
type ConfirmScreen struct {
	Base
	Line1, Line2 string
	Yes, No      string // button labels; empty: gui.yes / gui.no
	OnResult     func(yes bool)
}

func (s *ConfirmScreen) Init(ctx *Context) {
	s.Widgets = nil
	yes, no := ctx.T(s.Yes), ctx.T(s.No)
	if yes == "" {
		yes = ctx.T("gui.yes")
	}
	if no == "" {
		no = ctx.T("gui.no")
	}
	res := func(v bool) func() {
		return func() {
			ctx.Screens.Back()
			if s.OnResult != nil {
				s.OnResult(v)
			}
		}
	}
	s.Add(NewButton(ctx.W/2-155, ctx.H/6+96, 150, yes, res(true)), NewButton(ctx.W/2-155+160, ctx.H/6+96, 150, no, res(false)))
}

func (s *ConfirmScreen) Draw(ctx *Context, mx, my int, partial float32) {
	ctx.DrawBackground()
	ctx.Font.DrawCentered(ctx.Canvas, ctx.T(s.Line1), ctx.W/2, 70, 0xFFFFFF)
	for i, l := range ctx.Font.Wrap(ctx.T(s.Line2), ctx.W-50) {
		ctx.Font.DrawCentered(ctx.Canvas, l, ctx.W/2, 90+i*ctx.Font.Height, 0xFFFFFF)
	}
	s.DrawWidgets(ctx, mx, my)
}

// Setting is one entry of a SettingsScreen: a slider (Slider, Value,
// SetValue) or a button (OnClick: cycle a value or open a screen). The
// label reads "Label: Text()" like Minecraft's options, or just Label when
// Text is nil.
type Setting struct {
	Label    string
	Slider   bool
	Value    func() float64 // slider position 0..1
	SetValue func(v float64)
	Text     func() string
	OnClick  func()
}

func (s *Setting) caption(ctx *Context) string {
	if s.Text == nil {
		return ctx.T(s.Label)
	}
	return ctx.T(s.Label) + ": " + s.Text()
}

// SettingsScreen is GuiOptions-style: settings in two 150 px columns and a
// Done button.
type SettingsScreen struct {
	Base
	Title    string
	Settings []Setting
	OnDone   func() // after Done/Esc (save the options)
}

func (s *SettingsScreen) Init(ctx *Context) {
	s.Widgets = nil
	for i := range s.Settings {
		st := &s.Settings[i]
		x, y := ctx.W/2-155+i%2*160, ctx.H/6-12+24*(i>>1)
		if st.Slider {
			v := 0.0
			if st.Value != nil {
				v = st.Value()
			}
			s.Add(NewSlider(x, y, 150, v, func(v float64) string { return st.caption(ctx) }, func(v float64) {
				if st.SetValue != nil {
					st.SetValue(v)
				}
			}))
			continue
		}
		var b *Button
		b = NewButton(x, y, 150, st.caption(ctx), func() {
			if st.OnClick != nil {
				st.OnClick()
			}
			b.Label = st.caption(ctx)
		})
		s.Add(b)
	}
	s.Add(NewButton(ctx.W/2-100, ctx.H/6+168, 200, ctx.T("gui.done"), func() { s.done(ctx) }))
}

func (s *SettingsScreen) done(ctx *Context) {
	ctx.Screens.Back()
	if s.OnDone != nil {
		s.OnDone()
	}
}

func (s *SettingsScreen) Draw(ctx *Context, mx, my int, partial float32) {
	ctx.DrawBackground()
	ctx.Font.DrawCentered(ctx.Canvas, ctx.T(s.Title), ctx.W/2, 15, 0xFFFFFF)
	s.DrawWidgets(ctx, mx, my)
}

func (s *SettingsScreen) Input(ctx *Context, ev Event) bool {
	if ev.Type == KeyPress && ev.Key == KeyEscape {
		s.done(ctx)
		return true
	}
	return s.Base.Input(ctx, ev)
}

// GUIScaleSetting cycles Context.GUIScale (auto, small, normal, large).
func GUIScaleSetting(ctx *Context, onChange func(scale int)) Setting {
	names := []string{"options.guiScale.auto", "options.guiScale.small", "options.guiScale.normal", "options.guiScale.large"}
	return Setting{Label: "options.guiScale", Text: func() string { return ctx.T(names[ctx.GUIScale%4]) },
		OnClick: func() {
			ctx.GUIScale = (ctx.GUIScale + 1) % 4
			if onChange != nil {
				onChange(ctx.GUIScale)
			}
		}}
}

// LanguageSetting opens the LanguageScreen.
func LanguageSetting(ctx *Context) Setting {
	return Setting{Label: "options.language", OnClick: func() { ctx.Screens.Open(&LanguageScreen{}) }}
}

// LanguageScreen is GuiLanguage: the loaded languages in a list; a click
// switches the language at once.
type LanguageScreen struct {
	Base
	list  ScrollList
	codes []string
}

func (s *LanguageScreen) Init(ctx *Context) {
	s.Widgets = nil
	s.codes = ctx.Lang.Languages()
	s.list = ScrollList{Top: 32, Bottom: ctx.H - 65 + 4, SlotHeight: 18,
		Count:    func() int { return len(s.codes) },
		Selected: func(i int) bool { return s.codes[i] == ctx.Lang.Language() },
		DrawSlot: func(ctx *Context, i, x, y, h, mx, my int) {
			ctx.Font.DrawCentered(ctx.Canvas, ctx.Lang.Name(s.codes[i]), ctx.W/2, y+1, 0xFFFFFF)
		},
		OnClick: func(i int, double bool) {
			if s.codes[i] != ctx.Lang.Language() {
				ctx.Lang.SetLanguage(s.codes[i])
				s.Init(ctx)
			}
		}}
	for i, c := range s.codes {
		if c == ctx.Lang.Language() {
			s.list.layout(ctx)
			s.list.ScrollTo(i)
		}
	}
	s.Add(&s.list, NewButton(ctx.W/2-155+160, ctx.H-38, 150, ctx.T("gui.done"), ctx.Screens.Back))
}

func (s *LanguageScreen) Draw(ctx *Context, mx, my int, partial float32) {
	ctx.DrawBackground()
	s.DrawWidgets(ctx, mx, my)
	ctx.Font.DrawCentered(ctx.Canvas, ctx.T("options.language"), ctx.W/2, 16, 0xFFFFFF)
	ctx.Font.DrawCentered(ctx.Canvas, "("+ctx.T("options.languageWarning")+")", ctx.W/2, ctx.H-56, 0x808080)
}
