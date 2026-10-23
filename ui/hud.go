package ui

import (
	"math"
)

// HUDState is what the in-game overlay shows; the game fills it each tick.
type HUDState struct {
	Hidden bool // F1: nothing but chat

	Hotbar   [9]ItemStack
	Selected int

	// Survival bars; Creative hides them.
	Creative bool
	// Health and MaxHealth in half hearts (the reference's 20 = ten hearts);
	// LastHealth is the health before the latest damage (the flashing
	// part), HurtTime the hurt-resistance ticks left (10..20 flash).
	Health, MaxHealth, LastHealth, Absorption float64
	HurtTime                                  int
	Regen, Poison, Wither, Hardcore           bool
	Food                                      int // 0..20
	Saturation                                float64
	Hunger                                    bool // the hunger effect: green shanks
	Armor                                     int  // 0..20
	Underwater                                bool
	Air                                       int // 0..300
	// XP bar: ShowXP, Progress 0..1, Level.
	ShowXP    bool
	XP        float64
	Level     int
	Crosshair bool
	Mount     *MountBar
	Boss      *BossBar
	// Debug are F3's lines, drawn top left (GuiIngame.renderGameOverlay:
	// 2, 2+10*i, 0xE0E0E0 with shadow); nil: off.
	Debug []string
}

// MountBar is the ridden animal's health (half hearts). A horse
// (JumpBar) shows its jump charge Jump (0..1) instead of the XP bar.
type MountBar struct {
	Health, MaxHealth float64
	JumpBar           bool
	Jump              float64
}

// BossBar is the boss health bar at the top.
type BossBar struct {
	Name   string
	Health float64 // 0..1
}

// HUD is GuiIngameForge's overlay: hotbar, crosshair, boss bar, hearts,
// armour, food, mount hearts, air, XP, the held item's name, the action
// bar, titles and the chat.
type HUD struct {
	State HUDState
	// Overlay draws over the HUD's own parts, below the chat, unless the
	// HUD is hidden (a minimap); nil: none.
	Overlay func(ctx *Context, partial float32)
	// Underlay draws under all of the HUD (where the helmet overlay
	// goes: full-screen tints), unless the HUD is hidden; nil: none.
	Underlay func(ctx *Context, partial float32)
	// ChatWidth is the chat width in GUI px (320); ChatLines how many
	// lines show while the chat is closed (10).
	ChatWidth, ChatLines int
	chatOpen             bool

	ticks                   int
	rand                    javaRandom
	chat                    []chatLine
	chatScroll              int
	highlight               int
	lastSel                 int
	lastName                string
	action                  string
	actionT                 int
	title, sub              string
	tIn, tStay, tOut, tLeft int
}

type chatLine struct {
	text string
	tick int
}

func (h *HUD) chatWidth() int {
	if h.ChatWidth > 0 {
		return h.ChatWidth
	}
	return 320
}

// Update advances the HUD's timers by one tick (20 Hz).
func (h *HUD) Update() {
	h.ticks++
	if h.actionT > 0 {
		h.actionT--
	}
	if h.tLeft > 0 {
		h.tLeft--
	}
	name := ""
	if st := h.State.Hotbar[h.State.Selected%9]; len(st.Tooltip) > 0 {
		name = st.Tooltip[0]
	}
	if h.State.Selected != h.lastSel || name != h.lastName {
		h.lastSel, h.lastName = h.State.Selected, name
		h.highlight = 0
		if name != "" {
			h.highlight = 40
		}
	} else if h.highlight > 0 {
		h.highlight--
	}
}

// Message adds a chat line (§ codes allowed); long lines wrap when drawn.
func (h *HUD) Message(text string) {
	h.chat = append([]chatLine{{text, h.ticks}}, h.chat...)
	if len(h.chat) > 100 {
		h.chat = h.chat[:100]
	}
}

// ActionBar shows text above the hotbar for three seconds (the record
// overlay).
func (h *HUD) ActionBar(text string) { h.action, h.actionT = text, 60 }

// Title shows a big title and a subtitle in the middle, fading in, staying
// and fading out over the given ticks (the 1.8 reference's /title; 10, 70, 20).
func (h *HUD) Title(title, sub string, fadeIn, stay, fadeOut int) {
	h.title, h.sub = title, sub
	h.tIn, h.tStay, h.tOut = fadeIn, stay, fadeOut
	h.tLeft = fadeIn + stay + fadeOut
}

// Draw draws the overlay.
func (h *HUD) Draw(ctx *Context, partial float32) {
	st := &h.State
	c, f := ctx.Canvas, ctx.Font
	w, ht := ctx.W, ctx.H
	if !st.Hidden {
		if h.Underlay != nil {
			h.Underlay(ctx, partial)
		}
		h.drawHotbar(ctx)
		icons := ctx.Tex("textures/gui/icons.png")
		if st.Crosshair {
			c.SetInvert(true)
			DrawTexturedModalRect(c, icons, w/2-7, ht/2-7, 0, 0, 16, 16)
			c.SetInvert(false)
		}
		if b := st.Boss; b != nil {
			x := w/2 - 91
			k := int(b.Health * 183)
			DrawTexturedModalRect(c, icons, x, 12, 0, 74, 182, 5)
			DrawTexturedModalRect(c, icons, x, 12, 0, 74, 182, 5)
			if k > 0 {
				DrawTexturedModalRect(c, icons, x, 12, 0, 79, k, 5)
			}
			f.Draw(c, b.Name, w/2-f.Width(b.Name)/2, 2, 0xFFFFFF, true)
		}
		if !st.Creative {
			left, right := 39, 39
			h.drawHealth(ctx, icons, &left)
			h.drawArmor(ctx, icons, &left)
			if st.Mount == nil {
				h.drawFood(ctx, icons, &right)
			} else {
				h.drawMount(ctx, icons, &right)
			}
			h.drawAir(ctx, icons, &right)
			if st.Mount != nil && st.Mount.JumpBar {
				if st.ShowXP {
					h.drawJump(ctx, icons)
				}
			} else if st.ShowXP {
				h.drawXP(ctx, icons)
			}
		}
		if h.highlight > 0 && h.lastName != "" {
			op := min(h.highlight*256/10, 255)
			if op > 0 {
				y := ht - 59
				if st.Creative {
					y += 14
				}
				f.Draw(c, h.lastName, (w-f.Width(h.lastName))/2, y, 0xFFFFFF|uint32(op)<<24, true)
			}
		}
		if h.actionT > 0 {
			a := int((float32(h.actionT) - partial) * 255 / 20)
			if a = min(a, 255); a > 8 {
				f.DrawScaled(c, h.action, float32(w/2), float32(ht-68), 1, float32(-f.Width(h.action)/2), -4, 0xFFFFFF|uint32(a)<<24, false)
			}
		}
		h.drawTitle(ctx, partial)
		for i, l := range st.Debug {
			f.Draw(c, l, 2, 2+10*i, 0xE0E0E0, true)
		}
		if h.Overlay != nil {
			h.Overlay(ctx, partial)
		}
	}
	h.drawChat(ctx)
}

func (h *HUD) drawHotbar(ctx *Context) {
	c := ctx.Canvas
	w, ht := ctx.W, ctx.H
	t := ctx.Tex(widgetsTex)
	DrawTexturedModalRect(c, t, w/2-91, ht-22, 0, 0, 182, 22)
	DrawTexturedModalRect(c, t, w/2-91-1+h.State.Selected*20, ht-22-1, 0, 22, 24, 22)
	for i := 0; i < 9; i++ {
		DrawItem(ctx, h.State.Hotbar[i], w/2-90+i*20+2, ht-16-3)
	}
}

func ceil(f float64) int { return int(math.Ceil(f)) }

func (h *HUD) drawHealth(ctx *Context, icons Texture, leftH *int) {
	st, c := &h.State, ctx.Canvas
	highlight := st.HurtTime/3%2 == 1 && st.HurtTime >= 10
	health, last := ceil(st.Health), ceil(st.LastHealth)
	maxH := st.MaxHealth
	if maxH == 0 {
		maxH = 20
	}
	absorb := st.Absorption
	rows := ceil((maxH + absorb) / 2 / 10)
	rowH := max(10-(rows-2), 3)
	h.rand.SetSeed(int64(int32(h.ticks * 312871)))
	left := ctx.W/2 - 91
	top := ctx.H - *leftH
	*leftH += rows * rowH
	if rowH != 10 {
		*leftH += 10 - rowH
	}
	regen := -1
	if st.Regen {
		regen = h.ticks % 25
	}
	tp := 0
	if st.Hardcore {
		tp = 45
	}
	bg := 16
	if highlight {
		bg = 25
	}
	margin := 16
	if st.Poison {
		margin += 36
	} else if st.Wither {
		margin += 72
	}
	absorbLeft := absorb
	for i := ceil((maxH+absorb)/2) - 1; i >= 0; i-- {
		row := ceil(float64(i+1)/10) - 1
		x := left + i%10*8
		y := top - row*rowH
		if health <= 4 {
			y += int(h.rand.NextInt(2))
		}
		if i == regen {
			y -= 2
		}
		DrawTexturedModalRect(c, icons, x, y, bg, tp, 9, 9)
		if highlight {
			if i*2+1 < last {
				DrawTexturedModalRect(c, icons, x, y, margin+54, tp, 9, 9)
			} else if i*2+1 == last {
				DrawTexturedModalRect(c, icons, x, y, margin+63, tp, 9, 9)
			}
		}
		if absorbLeft > 0 {
			if absorbLeft == absorb && math.Mod(absorb, 2) == 1 {
				DrawTexturedModalRect(c, icons, x, y, margin+153, tp, 9, 9)
			} else {
				DrawTexturedModalRect(c, icons, x, y, margin+144, tp, 9, 9)
			}
			absorbLeft -= 2
		} else if i*2+1 < health {
			DrawTexturedModalRect(c, icons, x, y, margin+36, tp, 9, 9)
		} else if i*2+1 == health {
			DrawTexturedModalRect(c, icons, x, y, margin+45, tp, 9, 9)
		}
	}
}

func (h *HUD) drawArmor(ctx *Context, icons Texture, leftH *int) {
	level := h.State.Armor
	left, top := ctx.W/2-91, ctx.H-*leftH
	for i := 1; level > 0 && i < 20; i += 2 {
		switch {
		case i < level:
			DrawTexturedModalRect(ctx.Canvas, icons, left, top, 34, 9, 9, 9)
		case i == level:
			DrawTexturedModalRect(ctx.Canvas, icons, left, top, 25, 9, 9, 9)
		default:
			DrawTexturedModalRect(ctx.Canvas, icons, left, top, 16, 9, 9, 9)
		}
		left += 8
	}
	if level > 0 {
		*leftH += 10
	}
}

func (h *HUD) drawFood(ctx *Context, icons Texture, rightH *int) {
	st := &h.State
	left, top := ctx.W/2+91, ctx.H-*rightH
	*rightH += 10
	for i := 0; i < 10; i++ {
		idx := i*2 + 1
		x, y := left-i*8-9, top
		icon, bgi := 16, 0
		if st.Hunger {
			icon += 36
			bgi = 13
		}
		if st.Saturation <= 0 && h.ticks%(st.Food*3+1) == 0 {
			y = top + int(h.rand.NextInt(3)) - 1
		}
		DrawTexturedModalRect(ctx.Canvas, icons, x, y, 16+bgi*9, 27, 9, 9)
		if idx < st.Food {
			DrawTexturedModalRect(ctx.Canvas, icons, x, y, icon+36, 27, 9, 9)
		} else if idx == st.Food {
			DrawTexturedModalRect(ctx.Canvas, icons, x, y, icon+45, 27, 9, 9)
		}
	}
}

func (h *HUD) drawMount(ctx *Context, icons Texture, rightH *int) {
	m := h.State.Mount
	left := ctx.W/2 + 91
	health := ceil(m.Health)
	hearts := min(int(m.MaxHealth+0.5)/2, 30)
	const margin = 52
	for heart := 0; hearts > 0; heart += 20 {
		top := ctx.H - *rightH
		n := min(hearts, 10)
		hearts -= n
		for i := 0; i < n; i++ {
			x := left - i*8 - 9
			DrawTexturedModalRect(ctx.Canvas, icons, x, top, margin, 9, 9, 9)
			if i*2+1+heart < health {
				DrawTexturedModalRect(ctx.Canvas, icons, x, top, margin+36, 9, 9, 9)
			} else if i*2+1+heart == health {
				DrawTexturedModalRect(ctx.Canvas, icons, x, top, margin+45, 9, 9, 9)
			}
		}
		*rightH += 10
	}
}

func (h *HUD) drawAir(ctx *Context, icons Texture, rightH *int) {
	if !h.State.Underwater {
		return
	}
	left, top := ctx.W/2+91, ctx.H-*rightH
	air := float64(h.State.Air)
	full := ceil((air - 2) * 10 / 300)
	partial := ceil(air*10/300) - full
	for i := 0; i < full+partial; i++ {
		u := 25
		if i < full {
			u = 16
		}
		DrawTexturedModalRect(ctx.Canvas, icons, left-i*8-9, top, u, 18, 9, 9)
	}
	*rightH += 10
}

// drawJump is GuiIngameForge.renderJumpBar.
func (h *HUD) drawJump(ctx *Context, icons Texture) {
	left, top := ctx.W/2-91, ctx.H-32+3
	DrawTexturedModalRect(ctx.Canvas, icons, left, top, 0, 84, 182, 5)
	if filled := int(float32(h.State.Mount.Jump) * 183); filled > 0 {
		DrawTexturedModalRect(ctx.Canvas, icons, left, top, 0, 89, filled, 5)
	}
}

func (h *HUD) drawXP(ctx *Context, icons Texture) {
	c, f := ctx.Canvas, ctx.Font
	left := ctx.W/2 - 91
	filled := int(h.State.XP * 183)
	top := ctx.H - 32 + 3
	DrawTexturedModalRect(c, icons, left, top, 0, 64, 182, 5)
	if filled > 0 {
		DrawTexturedModalRect(c, icons, left, top, 0, 69, filled, 5)
	}
	if h.State.Level > 0 {
		s := itoa(h.State.Level)
		x, y := (ctx.W-f.Width(s))/2, ctx.H-31-4
		f.Draw(c, s, x+1, y, 0, false)
		f.Draw(c, s, x-1, y, 0, false)
		f.Draw(c, s, x, y+1, 0, false)
		f.Draw(c, s, x, y-1, 0, false)
		f.Draw(c, s, x, y, 0x80FF20, false)
	}
}

func itoa(i int) string {
	if i < 0 {
		return "-" + itoa(-i)
	}
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

func (h *HUD) drawTitle(ctx *Context, partial float32) {
	if h.tLeft <= 0 {
		return
	}
	f3 := float32(h.tLeft) - partial
	a := 255
	if h.tLeft > h.tOut+h.tStay && h.tIn > 0 {
		a = int((float32(h.tIn+h.tStay+h.tOut) - f3) * 255 / float32(h.tIn))
	}
	if h.tLeft <= h.tOut && h.tOut > 0 {
		a = int(f3 * 255 / float32(h.tOut))
	}
	a = min(max(a, 0), 255)
	if a <= 8 {
		return
	}
	f, c := ctx.Font, ctx.Canvas
	col := 0xFFFFFF | uint32(a)<<24
	cx, cy := float32(ctx.W/2), float32(ctx.H/2)
	f.DrawScaled(c, h.title, cx, cy, 4, float32(-f.Width(h.title)/2), -10, col, true)
	f.DrawScaled(c, h.sub, cx, cy, 2, float32(-f.Width(h.sub)/2), 5, col, true)
}

// drawChat is GuiNewChat.drawChat: recent lines fade after 10 seconds;
// all of them while the chat is open.
func (h *HUD) drawChat(ctx *Context) {
	c, f := ctx.Canvas, ctx.Font
	maxLines := h.ChatLines
	if maxLines == 0 {
		maxLines = 10
	}
	if h.chatOpen {
		maxLines = 20
	}
	w := h.chatWidth()
	var lines []chatLine
	for _, l := range h.chat {
		wr := f.Wrap(l.text, w)
		for i := len(wr) - 1; i >= 0; i-- {
			lines = append(lines, chatLine{wr[i], l.tick})
		}
	}
	h.chatScroll = min(max(h.chatScroll, 0), max(len(lines)-maxLines, 0))
	if !h.chatOpen {
		h.chatScroll = 0
	}
	baseX, baseY := 2, ctx.H-48+20
	for j := 0; j+h.chatScroll < len(lines) && j < maxLines; j++ {
		l := lines[j+h.chatScroll]
		age := h.ticks - l.tick
		if age >= 200 && !h.chatOpen {
			continue
		}
		d := (1 - float64(age)/200) * 10
		d = math.Min(math.Max(d, 0), 1)
		d *= d
		a := int(255 * d)
		if h.chatOpen {
			a = 255
		}
		if a <= 3 {
			continue
		}
		y := baseY - j*9
		DrawRect(c, baseX, y-9, baseX+w+4, y, uint32(a/2)<<24)
		f.Draw(c, l.text, baseX, y-8, 0xFFFFFF+uint32(a)<<24, true)
	}
}

// ChatScreen is GuiChat: a text line at the bottom; Enter sends.
type ChatScreen struct {
	Base
	HUD    *HUD
	OnSend func(text string)
	Prefix string // initial text ("/" for commands)
	field  *TextField
}

func (s *ChatScreen) Init(ctx *Context) {
	s.Widgets = nil
	old := s.Prefix
	if s.field != nil {
		old = s.field.Text
	}
	s.field = &TextField{X: 4, Y: ctx.H - 12, W: ctx.W - 4, H: 12, MaxLen: 100, NoBorder: true, Focused: true}
	s.field.SetText(old)
	s.field.OnEnter = func(t string) {
		if t != "" && s.OnSend != nil {
			s.OnSend(t)
		}
		ctx.Screens.Back()
	}
	s.Add(s.field)
	if s.HUD != nil {
		s.HUD.chatOpen = true
	}
}

func (s *ChatScreen) Draw(ctx *Context, mx, my int, partial float32) {
	DrawRect(ctx.Canvas, 2, ctx.H-14, ctx.W-2, ctx.H-2, 0x80000000)
	s.field.Draw(ctx, mx, my)
}

func (s *ChatScreen) Input(ctx *Context, ev Event) bool {
	switch {
	case ev.Type == KeyPress && ev.Key == KeyEscape && s.HUD != nil:
		s.HUD.chatOpen = false
		return false
	case ev.Type == MouseScroll && s.HUD != nil:
		if ev.DY > 0 {
			s.HUD.chatScroll++
		} else {
			s.HUD.chatScroll--
		}
		return true
	}
	if ev.Type == KeyPress && (ev.Key == KeyEnter || ev.Key == KeyKPEnter) && s.HUD != nil {
		s.HUD.chatOpen = false
	}
	return s.Base.Input(ctx, ev)
}
