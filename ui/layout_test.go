package ui

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func testContext(t *testing.T) (*Context, *SoftCanvas) {
	c := NewSoftCanvas(854, 480, 2)
	ctx, err := NewContext(c, vanillaFS(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Resize(854, 480)
	return ctx, c
}

func TestMainMenuLayout(t *testing.T) {
	ctx, c := testContext(t)
	if ctx.W != 427 || ctx.H != 240 || ctx.Scale != 2 {
		t.Fatalf("gui size %dx%d@%d, want 427x240@2", ctx.W, ctx.H, ctx.Scale)
	}
	clicked := ""
	m := NewMainMenu(ctx, []MenuItem{{Label: "menu.singleplayer", OnClick: func() { clicked = "sp" }}},
		func() { clicked = "options" }, func() { clicked = "quit" }, nil)
	ctx.Screens.Open(m)
	want := [][4]int{{113, 108, 200, 20}, {113, 192, 98, 20}, {215, 192, 98, 20}}
	for i, w := range want {
		b := m.Widgets[i].(*Button)
		if g := [4]int{b.X, b.Y, b.W, b.H}; g != w {
			t.Errorf("button %d at %v, want %v", i, g, w)
		}
	}
	for _, tc := range []struct {
		x, y int
		want string
	}{{120, 110, "sp"}, {300, 200, "quit"}, {120, 200, "options"}} {
		clicked = ""
		ctx.Screens.Input(Event{Type: MousePress, X: tc.x, Y: tc.y})
		if clicked != tc.want {
			t.Errorf("click %d,%d: %q, want %q", tc.x, tc.y, clicked, tc.want)
		}
	}
	// Smoke: the button face is drawn (not the dark background).
	ctx.Screens.Draw(0, 0, 0)
	p := c.Img.NRGBAAt(2*113+6, 2*108+6)
	if int(p.R)+int(p.G)+int(p.B) < 200 {
		t.Errorf("button face pixel %v looks undrawn", p)
	}
}

func TestContainerSlotHit(t *testing.T) {
	ctx, _ := testContext(t)
	s := &ContainerScreen{Layout: ChestLayout(3, "container.chest")}
	s.Init(ctx)
	l := s.Layout
	if len(l.Slots) != 27+36 {
		t.Fatalf("%d slots", len(l.Slots))
	}
	for _, i := range []int{0, 26, 27, 62} {
		p := l.Slots[i]
		if g := s.slotAt(s.left+p[0]+8, s.top+p[1]+8); g != i {
			t.Errorf("slot %d hit as %d", i, g)
		}
	}
	if g := s.slotAt(s.left+l.Slots[0][0]-3, s.top+l.Slots[0][1]); g != -1 {
		t.Errorf("left of slot 0 hit %d", g)
	}
}

func TestStackFS(t *testing.T) {
	hi := fstest.MapFS{"a.txt": {Data: []byte("hi")}}
	lo := fstest.MapFS{"a.txt": {Data: []byte("lo")}, "b.txt": {Data: []byte("lo")}}
	s := StackFS{hi, lo}
	for name, want := range map[string]string{"a.txt": "hi", "b.txt": "lo"} {
		if b, err := fs.ReadFile(s, name); err != nil || string(b) != want {
			t.Errorf("%s = %q %v, want %q", name, b, err, want)
		}
	}
	if _, err := fs.ReadFile(s, "c.txt"); err == nil {
		t.Error("c.txt found")
	}
}

func TestHUDTitleFade(t *testing.T) {
	h := &HUD{}
	h.Title("T", "", 10, 70, 20)
	for i := 0; i < 100; i++ {
		h.Update()
	}
	if h.tLeft != 0 {
		t.Errorf("title still shown after 100 ticks: %d", h.tLeft)
	}
}
