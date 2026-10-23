package ui

import (
	"math"
	"strconv"
)

// ItemStack is what a slot shows; the game builds it from its item stacks.
type ItemStack struct {
	Icon      *Sprite // nil: empty slot
	Count     int     // shown when > 1
	CountText string  // shown instead of Count when set
	// Damage is the durability bar's fill (damage / max); shown when Bar.
	Damage float64
	Bar    bool
	// Tooltip: the name (with its § rarity colour) then further lines,
	// which are drawn grey like Minecraft's lore.
	Tooltip []string
}

// Empty reports whether the stack shows nothing.
func (s ItemStack) Empty() bool { return s.Icon == nil }

// ContainerModel is the logic behind a container view; the game's
// inventory code implements it. Slot indices follow the layout's Slots.
type ContainerModel interface {
	Slot(i int) ItemStack
	// Cursor is the stack held on the mouse.
	Cursor() ItemStack
	// Click is a click on slot i with button 0 (left) or 1 (right), shift
	// held for a quick move; i is SlotOutside for a click beside the GUI
	// (drop).
	Click(i, button int, shift bool)
	// Drag spreads the cursor stack over slots (button 0: evenly, 1: one
	// each), Minecraft's drag mode.
	Drag(slots []int, button int)
}

// SlotOutside is the slot index of a click outside the container.
const SlotOutside = -999

// TexPart is a part of the background sheet (256-unit coordinates) drawn
// at X,Y in the GUI.
type TexPart struct{ X, Y, U, V, W, H int }

// Label is a text drawn in the container's foreground (0x404040, no shadow).
type Label struct {
	Text     string // lang key or text
	X, Y     int
	Centered bool // centred on the GUI's width
}

// Progress is an animated part: Kind "flame" grows upward (the furnace
// fire, 14 x 12+2), "arrow" grows to the right (the furnace arrow, 24+1 x 16).
type Progress struct {
	Kind  string
	X, Y  int
	U, V  int
	Value func() float64 // 0..1; the fire is drawn only when > 0
}

// ContainerLayout places a container GUI: the sheet, its size, the
// background parts, titles, slot positions (item top-left) and progress.
type ContainerLayout struct {
	Texture  string
	W, H     int
	Parts    []TexPart // empty: the whole W x H at 0,0
	Labels   []Label
	Slots    [][2]int
	Progress []Progress
	// Extra draws over the background (the player model); left, top is
	// the GUI's corner.
	Extra func(ctx *Context, left, top, mx, my int)
}

func playerSlots(slots [][2]int, y int) [][2]int {
	for r := 0; r < 3; r++ {
		for c := 0; c < 9; c++ {
			slots = append(slots, [2]int{8 + c*18, y + r*18})
		}
	}
	for c := 0; c < 9; c++ {
		slots = append(slots, [2]int{8 + c*18, y + 58})
	}
	return slots
}

// InventoryLayout is GuiInventory. Slots as ContainerPlayer: 0 craft
// result, 1-4 craft grid, 5-8 armour (head to feet), 9-35 main, 36-44 hotbar.
func InventoryLayout() ContainerLayout {
	s := [][2]int{{144, 36}}
	for r := 0; r < 2; r++ {
		for c := 0; c < 2; c++ {
			s = append(s, [2]int{88 + c*18, 26 + r*18})
		}
	}
	for i := 0; i < 4; i++ {
		s = append(s, [2]int{8, 8 + i*18})
	}
	return ContainerLayout{Texture: "textures/gui/container/inventory.png", W: 176, H: 166,
		Labels: []Label{{Text: "container.crafting", X: 86, Y: 16}}, Slots: playerSlots(s, 84)}
}

// ChestLayout is GuiChest with rows of 9 (1..6): slots 0..rows*9-1 the
// chest, then 27 main and 9 hotbar. title is the chest's name.
func ChestLayout(rows int, title string) ContainerLayout {
	h := 114 + rows*18
	var s [][2]int
	for r := 0; r < rows; r++ {
		for c := 0; c < 9; c++ {
			s = append(s, [2]int{8 + c*18, 18 + r*18})
		}
	}
	return ContainerLayout{Texture: "textures/gui/container/generic_54.png", W: 176, H: h,
		Parts:  []TexPart{{0, 0, 0, 0, 176, rows*18 + 17}, {0, rows*18 + 17, 0, 126, 176, 96}},
		Labels: []Label{{Text: title, X: 8, Y: 6}, {Text: "container.inventory", X: 8, Y: h - 96 + 2}},
		Slots:  playerSlots(s, 103+(rows-4)*18)}
}

// CraftingLayout is GuiCrafting: 0 result, 1-9 grid, 10-36 main, 37-45
// hotbar.
func CraftingLayout() ContainerLayout {
	s := [][2]int{{124, 35}}
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			s = append(s, [2]int{30 + c*18, 17 + r*18})
		}
	}
	return ContainerLayout{Texture: "textures/gui/container/crafting_table.png", W: 176, H: 166,
		Labels: []Label{{Text: "container.crafting", X: 28, Y: 6}, {Text: "container.inventory", X: 8, Y: 166 - 96 + 2}},
		Slots:  playerSlots(s, 84)}
}

// FurnaceLayout is GuiFurnace: 0 input, 1 fuel, 2 output, 3-29 main,
// 30-38 hotbar; burn and cook are the fire left and the cooking progress
// (0..1).
func FurnaceLayout(burn, cook func() float64) ContainerLayout {
	return ContainerLayout{Texture: "textures/gui/container/furnace.png", W: 176, H: 166,
		Labels:   []Label{{Text: "container.furnace", Y: 6, Centered: true}, {Text: "container.inventory", X: 8, Y: 166 - 96 + 2}},
		Slots:    playerSlots([][2]int{{56, 17}, {56, 53}, {116, 35}}, 84),
		Progress: []Progress{{Kind: "flame", X: 56, Y: 36, U: 176, V: 0, Value: burn}, {Kind: "arrow", X: 79, Y: 34, U: 176, V: 14, Value: cook}}}
}

// ContainerScreen is GuiContainer: a layout over a model, with hover
// highlight, the held stack on the mouse, drag-spreading and tooltips.
type ContainerScreen struct {
	Layout     ContainerLayout
	Model      ContainerModel
	left, top  int
	drag       []int
	dragButton int
	pressed    bool
}

func (s *ContainerScreen) Init(ctx *Context) {
	s.left, s.top = (ctx.W-s.Layout.W)/2, (ctx.H-s.Layout.H)/2
}

func (s *ContainerScreen) Update() {}

func (s *ContainerScreen) slotAt(mx, my int) int {
	for i, p := range s.Layout.Slots {
		x, y := mx-s.left, my-s.top
		if x >= p[0]-1 && x < p[0]+17 && y >= p[1]-1 && y < p[1]+17 {
			return i
		}
	}
	return -1
}

func (s *ContainerScreen) Draw(ctx *Context, mx, my int, partial float32) {
	c := ctx.Canvas
	ctx.DrawBackground()
	l := &s.Layout
	t := ctx.Tex(l.Texture)
	parts := l.Parts
	if len(parts) == 0 {
		parts = []TexPart{{0, 0, 0, 0, l.W, l.H}}
	}
	for _, p := range parts {
		DrawTexturedModalRect(c, t, s.left+p.X, s.top+p.Y, p.U, p.V, p.W, p.H)
	}
	for _, p := range l.Progress {
		if p.Value == nil {
			continue
		}
		v := p.Value()
		switch p.Kind {
		case "flame":
			if v > 0 {
				i := int(v * 12)
				DrawTexturedModalRect(c, t, s.left+p.X, s.top+p.Y+12-i, p.U, p.V+12-i, 14, i+2)
			}
		case "arrow":
			DrawTexturedModalRect(c, t, s.left+p.X, s.top+p.Y, p.U, p.V, int(v*24)+1, 16)
		}
	}
	if l.Extra != nil {
		l.Extra(ctx, s.left, s.top, mx, my)
	}
	hover := s.slotAt(mx, my)
	for i, p := range l.Slots {
		x, y := s.left+p[0], s.top+p[1]
		st := s.Model.Slot(i)
		if s.inDrag(i) && !s.Model.Cursor().Empty() && st.Empty() {
			st = s.Model.Cursor() // preview of the spread
			st.Count, st.CountText = 0, ""
		}
		DrawItem(ctx, st, x, y)
		if i == hover {
			DrawGradientRect(c, x, y, x+16, y+16, 0x80FFFFFF, 0x80FFFFFF)
		}
	}
	for _, lb := range l.Labels {
		txt := ctx.T(lb.Text)
		x := lb.X
		if lb.Centered {
			x = l.W/2 - ctx.Font.Width(txt)/2
		}
		ctx.Font.Draw(c, txt, s.left+x, s.top+lb.Y, 0x404040, false)
	}
	if cur := s.Model.Cursor(); !cur.Empty() {
		DrawItem(ctx, cur, mx-8, my-8)
	} else if hover >= 0 {
		if st := s.Model.Slot(hover); !st.Empty() {
			DrawItemTooltip(ctx, st, mx, my)
		}
	}
}

func (s *ContainerScreen) inDrag(i int) bool {
	if len(s.drag) < 2 {
		return false
	}
	for _, d := range s.drag {
		if d == i {
			return true
		}
	}
	return false
}

func (s *ContainerScreen) Input(ctx *Context, ev Event) bool {
	switch ev.Type {
	case MousePress:
		if ev.Button > 1 {
			return false
		}
		i := s.slotAt(ev.X, ev.Y)
		outside := ev.X < s.left || ev.Y < s.top || ev.X >= s.left+s.Layout.W || ev.Y >= s.top+s.Layout.H
		shift := ev.Mods&ModShift != 0
		switch {
		case i < 0 && outside:
			s.Model.Click(SlotOutside, ev.Button, shift)
		case i < 0:
		case s.Model.Cursor().Empty() || shift:
			s.Model.Click(i, ev.Button, shift)
		default: // holding a stack: wait for release, maybe a drag
			s.pressed, s.dragButton, s.drag = true, ev.Button, []int{i}
		}
		return true
	case MouseMove:
		if s.pressed {
			if i := s.slotAt(ev.X, ev.Y); i >= 0 && !s.contains(i) && s.Model.Slot(i).Empty() {
				s.drag = append(s.drag, i)
			}
		}
		return true
	case MouseRelease:
		if s.pressed {
			s.pressed = false
			if len(s.drag) > 1 {
				s.Model.Drag(s.drag, s.dragButton)
			} else if len(s.drag) == 1 {
				s.Model.Click(s.drag[0], s.dragButton, false)
			}
			s.drag = nil
		}
		return true
	}
	return false
}

func (s *ContainerScreen) contains(i int) bool {
	for _, d := range s.drag {
		if d == i {
			return true
		}
	}
	return false
}

// DrawItem draws an item icon at x,y (16x16) with its count and
// durability bar (RenderItem.renderItemOverlayIntoGUI).
func DrawItem(ctx *Context, st ItemStack, x, y int) {
	if st.Empty() {
		return
	}
	c := ctx.Canvas
	st.Icon.DrawSized(c, float32(x), float32(y), 16, 16)
	txt := st.CountText
	if txt == "" && st.Count > 1 {
		txt = strconv.Itoa(st.Count)
	}
	if txt != "" {
		ctx.Font.Draw(c, txt, x+19-2-ctx.Font.Width(txt), y+6+3, 0xFFFFFF, true)
	}
	if st.Bar {
		j1 := int(math.Round(13 - st.Damage*13))
		k := int(math.Round(255 - st.Damage*255))
		col := uint32(255-k)<<16 | uint32(k)<<8
		dark := uint32((255-k)/4)<<16 | 16128
		DrawRect(c, x+2, y+13, x+2+13, y+13+2, 0xFF000000)
		DrawRect(c, x+2, y+13, x+2+12, y+13+1, 0xFF000000|dark)
		DrawRect(c, x+2, y+13, x+2+j1, y+13+1, 0xFF000000|col)
	}
}

// DrawItemTooltip draws a stack's tooltip: the name as given, the rest grey.
func DrawItemTooltip(ctx *Context, st ItemStack, mx, my int) {
	lines := make([]string, len(st.Tooltip))
	for i, l := range st.Tooltip {
		if i > 0 {
			l = "§7" + l
		}
		lines[i] = l
	}
	DrawTooltip(ctx, lines, mx, my)
}

// DrawTooltip is GuiScreen.drawHoveringText: the dark purple-bordered box
// next to the mouse.
func DrawTooltip(ctx *Context, lines []string, mx, my int) {
	if len(lines) == 0 {
		return
	}
	c, f := ctx.Canvas, ctx.Font
	k := 0
	for _, s := range lines {
		k = max(k, f.Width(s))
	}
	x, y := mx+12, my-12
	h := 8
	if len(lines) > 1 {
		h += 2 + (len(lines)-1)*10
	}
	if x+k > ctx.W {
		x -= 28 + k
	}
	if y+h+6 > ctx.H {
		y = ctx.H - h - 6
	}
	const bg = 0xF0100010
	DrawGradientRect(c, x-3, y-4, x+k+3, y-3, bg, bg)
	DrawGradientRect(c, x-3, y+h+3, x+k+3, y+h+4, bg, bg)
	DrawGradientRect(c, x-3, y-3, x+k+3, y+h+3, bg, bg)
	DrawGradientRect(c, x-4, y-3, x-3, y+h+3, bg, bg)
	DrawGradientRect(c, x+k+3, y-3, x+k+4, y+h+3, bg, bg)
	const b1 = 0x505000FF
	const b2 = (b1&0xFEFEFE)>>1 | b1&0xFF000000
	DrawGradientRect(c, x-3, y-3+1, x-3+1, y+h+3-1, b1, b2)
	DrawGradientRect(c, x+k+2, y-3+1, x+k+3, y+h+3-1, b1, b2)
	DrawGradientRect(c, x-3, y-3, x+k+3, y-3+1, b1, b1)
	DrawGradientRect(c, x-3, y+h+2, x+k+3, y+h+3, b2, b2)
	for i, s := range lines {
		f.Draw(c, s, x, y, 0xFFFFFFFF, true)
		if i == 0 {
			y += 2
		}
		y += 10
	}
}
