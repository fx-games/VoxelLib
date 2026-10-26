package fall

import "testing"

type world map[[3]int]string

func (w world) Block(x, y, z int) string {
	if s, ok := w[[3]int{x, y, z}]; ok {
		return s
	}
	return ""
}
func (w world) SetBlock(x, y, z int, s string) {
	if s == "" {
		delete(w, [3]int{x, y, z})
		return
	}
	w[[3]int{x, y, z}] = s
}
func (w world) Solid(x, y, z int) bool { return y <= 0 || w.Block(x, y, z) != "" }

func TestBalconyFallsAndSettles(t *testing.T) {
	w := world{}
	var cells [][3]int
	for x := 0; x < 3; x++ { // a 3x2 slab at y 6..7
		for y := 6; y <= 7; y++ {
			w.SetBlock(x, y, 0, "planks")
			cells = append(cells, [3]int{x, y, 0})
		}
	}
	w.SetBlock(2, 1, 0, "crate") // something under one end
	p := Detach[string](w, cells, "")
	if len(p.Cells) != 6 || w.Block(0, 6, 0) != "" {
		t.Fatalf("detached %d, left %q", len(p.Cells), w.Block(0, 6, 0))
	}
	ticks := 0
	for !p.Tick(w) {
		if ticks++; ticks > 200 {
			t.Fatal("never landed")
		}
	}
	if p.Drop != 4 { // its bottom row (y 6) rests on the crate (y 1) at y 2
		t.Fatalf("dropped %v, want 4", p.Drop)
	}
	p.Settle(w, "", func(x, y, z int) bool { return x != 0 || y != 2 }) // a body at 0,2
	if w.Block(1, 2, 0) != "planks" || w.Block(1, 3, 0) != "planks" || w.Block(0, 2, 0) != "" {
		t.Fatalf("settled wrong: %v", w)
	}
}
