package ui

import (
	"image"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/memmaker/voxellib/inventory"
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

func TestArmourSlotIcons(t *testing.T) {
	m := InventoryLayout(InventoryOptions{}).SlotIcons
	if len(m) != 4 || m[5] != "textures/items/empty_armor_slot_helmet.png" || m[8] != "textures/items/empty_armor_slot_boots.png" {
		t.Fatal(m)
	}
}

// TestInventoryParts lays out every mix of the parts, with the default
// column and with rings beside a chest slot, and expects every shown
// slot and piece inside the window, no slots overlapping, and the
// shown slots exactly those of the shown parts.
func TestInventoryParts(t *testing.T) {
	ring := inventory.EquipSlot{Name: "ring", Icon: "textures/items/ring.png"}
	five := append([]inventory.EquipSlot{ring}, inventory.DefaultEquip...)
	rogue := []inventory.EquipSlot{ring, ring, {Name: "armor"}, {Name: "amulet"}}
	for _, eq := range []struct {
		slots []inventory.EquipSlot
		at    [][2]int
	}{{five, nil}, {rogue, [][2]int{{0, 1}, {2, 1}, {1, 1}, {1, 0}}}} {
		c, _ := inventory.NewPlayerContainer(inventory.NewPlayer(inventory.PlayerSpec{Equip: eq.slots}), nil)
		for m := range 8 {
			o := InventoryOptions{Equip: eq.slots, EquipAt: eq.at, NoEquip: m&1 != 0, NoPreview: m&2 != 0, NoCrafting: m&4 != 0}
			l := InventoryLayout(o)
			if len(l.Slots) != len(c.Slots) {
				t.Fatalf("%+v: %d layout slots, %d container slots", o, len(l.Slots), len(c.Slots))
			}
			for _, p := range l.Parts {
				if p.X < 0 || p.Y < 0 || p.X+p.W > l.W || p.Y+p.H > l.H {
					t.Fatalf("%+v: piece %+v outside %dx%d", o, p, l.W, l.H)
				}
			}
			for i, a := range l.Slots {
				shown := a != off
				if want := !(i < 5 && o.NoCrafting || i >= 5 && i < 5+len(eq.slots) && o.NoEquip); shown != want {
					t.Fatalf("%+v: slot %d at %v", o, i, a)
				}
				if shown && (a[0] < 7 || a[1] < 7 || a[0]+17 > l.W-7 || a[1]+17 > l.H-6) {
					t.Fatalf("%+v: slot %d %v outside %dx%d", o, i, a, l.W, l.H)
				}
				for j, b := range l.Slots[:i] {
					if shown && a[0] < b[0]+18 && b[0] < a[0]+18 && a[1] < b[1]+18 && b[1] < a[1]+18 {
						t.Fatalf("%+v: slots %d %v and %d %v overlap", o, j, b, i, a)
					}
				}
			}
		}
	}
	l := InventoryLayout(InventoryOptions{Equip: rogue, EquipAt: [][2]int{{0, 1}, {2, 1}, {1, 1}, {1, 0}}, NoCrafting: true})
	// slots 5+d show rogue[3-d]: the amulet over the armour, a ring each side
	am, ar, r1, r0 := l.Slots[5], l.Slots[6], l.Slots[7], l.Slots[8]
	if am != [2]int{ar[0], ar[1] - 18} || r0 != [2]int{ar[0] - 18, ar[1]} || r1 != [2]int{ar[0] + 18, ar[1]} {
		t.Fatalf("rogue equipment at %v", l.Slots[5:9])
	}
}

// TestInventoryPieces draws the default layout's pieces and expects
// inventory.png's 176x166 exactly.
func TestInventoryPieces(t *testing.T) {
	ctx, _ := testContext(t)
	l := InventoryLayout(InventoryOptions{})
	if l.W != 176 || l.H != 166 {
		t.Fatalf("%dx%d", l.W, l.H)
	}
	draw := func(parts []TexPart) *image.NRGBA {
		c := NewSoftCanvas(176, 166, 1)
		for _, p := range parts {
			DrawTexturedModalRect(c, ctx.Tex(l.Texture), p.X, p.Y, p.U, p.V, p.W, p.H)
		}
		return c.Img
	}
	got, want := draw(l.Parts), draw([]TexPart{{0, 0, 0, 0, 176, 166}})
	for y := range 166 {
		for x := range 176 {
			if got.NRGBAAt(x, y) != want.NRGBAAt(x, y) {
				t.Fatalf("%d,%d: %v, inventory.png %v", x, y, got.NRGBAAt(x, y), want.NRGBAAt(x, y))
			}
		}
	}
}

func TestPlayerGUIInput(t *testing.T) {
	ctx, _ := testContext(t)
	p := inventory.NewPlayer(inventory.PlayerSpec{})
	c, _ := inventory.NewPlayerContainer(p, nil)
	g := NewPlayerGUI(ctx, p, c, func(*inventory.Stack) ItemStack { return ItemStack{} })
	var held []int
	p.OnHeld = func(_, n int) { held = append(held, n) }
	g.Key(Key1+4, 1, 0)
	g.Scroll(0, -1)
	g.Scroll(0, 1)
	g.Scroll(0, 1)
	if !slices.Equal(held, []int{4, 5, 4, 3}) {
		t.Fatalf("held %v", held)
	}
	closed := 0
	g.OnClose = func() { closed++ }
	g.Key(KeyE, 1, 0)
	if !g.IsOpen() || ctx.Screens.Len() != 1 || !g.Key(Key1, 1, 0) || p.Held != 3 {
		t.Fatalf("open %v, held %d", g.IsOpen(), p.Held)
	}
	p.Cursor = inventory.New(&inventory.Item{Key: "test:stone"}, 1)
	dropped := 0
	c.Drop = func(*inventory.Stack) { dropped++ }
	g.Key(KeyEscape, 1, 0)
	if g.IsOpen() || ctx.Screens.Len() != 0 || closed != 1 || dropped != 1 || p.Cursor != nil {
		t.Fatalf("escape: open %v closed %d dropped %d", g.IsOpen(), closed, dropped)
	}
}
