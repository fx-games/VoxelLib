package ui

import "image"

// QuestEntry is one quest in the log.
type QuestEntry struct {
	Category   string
	Title      string
	Text       string
	Objectives []string // "Talk to X (1/1)"; § codes allowed
	Done       bool
}

// QuestLogScreen lists quests by category on the left and shows the chosen
// one's text and objectives on the right, on a panel (nine-slice sprite;
// default vanilla demo_background.png), the text scrolling with the wheel.
type QuestLogScreen struct {
	Title      string // lang key or text
	Quests     func() []QuestEntry
	Background *Sprite
	// OnTrack, when set, adds a button to track the selected quest.
	OnTrack  func(q QuestEntry)
	selected int
	listOff  int
	textOff  int
	x, y     int
	w, h     int
	rows     []questRow
	track    *Button
	quests   []QuestEntry
}

type questRow struct {
	header string
	quest  int
}

const questListW = 110

func (s *QuestLogScreen) Init(ctx *Context) {
	s.w, s.h = min(320, ctx.W-8), min(220, ctx.H-8)
	s.x, s.y = (ctx.W-s.w)/2, (ctx.H-s.h)/2
	s.track = nil
	if s.OnTrack != nil {
		s.track = NewButton(s.x+s.w-80-8, s.y+s.h-28, 80, ctx.T("quest.track"), func() {
			if s.selected < len(s.quests) {
				s.OnTrack(s.quests[s.selected])
			}
		})
	}
}

func (s *QuestLogScreen) Update() {}

func (s *QuestLogScreen) refresh() {
	s.quests = nil
	if s.Quests != nil {
		s.quests = s.Quests()
	}
	s.rows = s.rows[:0]
	last := "\x00"
	for i, q := range s.quests {
		if q.Category != last {
			last = q.Category
			s.rows = append(s.rows, questRow{header: q.Category, quest: -1})
		}
		s.rows = append(s.rows, questRow{quest: i})
	}
	if s.selected >= len(s.quests) {
		s.selected = 0
	}
}

func (s *QuestLogScreen) listRows(ctx *Context) int { return (s.h - 30) / (ctx.Font.Height + 1) }

func (s *QuestLogScreen) Draw(ctx *Context, mx, my int, partial float32) {
	s.refresh()
	c, f := ctx.Canvas, ctx.Font
	ctx.DrawBackground()
	bg := s.Background
	if bg == nil {
		bg = &Sprite{Tex: ctx.Tex("textures/gui/demo_background.png"), W: 248, H: 166, TW: 256, TH: 256, SliceL: 4, SliceT: 4, SliceR: 4, SliceB: 4}
	}
	bg.DrawSized(c, float32(s.x), float32(s.y), float32(s.w), float32(s.h))
	f.Draw(c, ctx.T(s.Title), s.x+8, s.y+7, 0x404040, false)
	DrawRect(c, s.x+8+questListW, s.y+18, s.x+9+questListW, s.y+s.h-8, 0xFF8B8B8B)
	lh := f.Height + 1
	y := s.y + 20
	n := s.listRows(ctx)
	s.listOff = min(max(s.listOff, 0), max(len(s.rows)-n, 0))
	for _, r := range s.rows[s.listOff:min(len(s.rows), s.listOff+n)] {
		if r.quest < 0 {
			f.Draw(c, f.TrimToWidth(ctx.T(r.header), questListW, false), s.x+8, y, 0x404040, false)
		} else {
			q := s.quests[r.quest]
			col := uint32(0x202020)
			switch {
			case r.quest == s.selected:
				col = 0xFFFFFF
				DrawRect(c, s.x+8, y-1, s.x+8+questListW-2, y+lh-1, 0xFF6060A0)
			case inside(mx, my, s.x+8, y-1, questListW, lh):
				col = 0x2020C0
			case q.Done:
				col = 0x607060
			}
			f.Draw(c, f.TrimToWidth(ctx.T(q.Title), questListW-8, false), s.x+12, y, col, false)
		}
		y += lh
	}
	if s.selected >= len(s.quests) {
		return
	}
	q := s.quests[s.selected]
	tx, tw := s.x+16+questListW, s.w-questListW-26
	var lines []string
	lines = append(lines, "§l"+ctx.T(q.Title), "")
	lines = append(lines, f.Wrap(ctx.T(q.Text), tw)...)
	if len(q.Objectives) > 0 {
		lines = append(lines, "")
		for _, o := range q.Objectives {
			lines = append(lines, f.Wrap("- "+ctx.T(o), tw)...)
		}
	}
	bottom := s.y + s.h - 10
	if s.track != nil {
		bottom = s.track.Y - 4
	}
	vis := (bottom - (s.y + 20)) / f.Height
	s.textOff = min(max(s.textOff, 0), max(len(lines)-vis, 0))
	c.SetScissor(image.Rect(tx, s.y+18, tx+tw+4, bottom))
	y = s.y + 20
	for _, l := range lines[s.textOff:] {
		if y >= bottom {
			break
		}
		f.Draw(c, l, tx, y, 0x202020, false)
		y += f.Height
	}
	c.SetScissor(image.Rectangle{})
	if s.track != nil {
		s.track.Draw(ctx, mx, my)
	}
}

func (s *QuestLogScreen) Input(ctx *Context, ev Event) bool {
	s.refresh()
	switch ev.Type {
	case MousePress:
		if s.track != nil && s.track.Input(ctx, ev) {
			return true
		}
		lh := ctx.Font.Height + 1
		if ev.X >= s.x+8 && ev.X < s.x+8+questListW && ev.Y >= s.y+19 {
			i := (ev.Y-s.y-19)/lh + s.listOff
			if i < len(s.rows) && s.rows[i].quest >= 0 {
				s.selected, s.textOff = s.rows[i].quest, 0
				ctx.Play("gui.button.press")
			}
		}
		return true
	case MouseScroll:
		d := -1
		if ev.DY < 0 {
			d = 1
		}
		if ev.X < s.x+8+questListW {
			s.listOff += d
		} else {
			s.textOff += d
		}
		return true
	case KeyPress:
		switch ev.Key {
		case KeyDown:
			s.selected = min(s.selected+1, max(len(s.quests)-1, 0))
			s.textOff = 0
		case KeyUp:
			s.selected = max(s.selected-1, 0)
			s.textOff = 0
		default:
			return false
		}
		return true
	}
	return false
}
