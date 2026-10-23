package ui

import (
	"reflect"
	"testing"
)

// TestBookLayout checks GuiScreenBook's numbers at 427x240: the book at
// ((427-192)/2, 2) = (117, 2), Done at width/2-100, 4+192, the page
// buttons at i+120 / i+38, 2+154, page turning and the signing flow.
func TestBookLayout(t *testing.T) {
	ctx, c := testContext(t)
	b := &BookScreen{Pages: []string{"one", "two §ltwo"}}
	ctx.Screens.Open(b)
	if g := [4]int{b.done.X, b.done.Y, b.done.W, b.done.H}; g != [4]int{113, 196, 200, 20} {
		t.Errorf("done %v", g)
	}
	if g := [2]int{b.next.X, b.next.Y}; g != [2]int{237, 156} {
		t.Errorf("next %v", g)
	}
	if g := [2]int{b.prev.X, b.prev.Y}; g != [2]int{155, 156} {
		t.Errorf("prev %v", g)
	}
	if !b.prev.Hidden || b.next.Hidden {
		t.Fatal("page 1 of 2: only next shown")
	}
	ctx.Screens.Input(Event{Type: MousePress, X: 240, Y: 160})
	if b.page != 1 || !b.next.Hidden || b.prev.Hidden {
		t.Fatalf("after next: page %d", b.page)
	}
	ctx.Screens.Input(Event{Type: CharTyped, Char: 'x'}) // signed: read only
	if b.Pages[1] != "two §ltwo" {
		t.Error("written book was edited")
	}
	ctx.Screens.Draw(0, 0, 0)
	// The book image covers (117,2)-(309,194): its cream page is drawn.
	if p := c.Img.NRGBAAt(2*(117+60), 2*(2+120)); int(p.R)+int(p.G)+int(p.B) < 400 {
		t.Errorf("book page pixel %v looks undrawn", p)
	}
	ctx.Screens.Input(Event{Type: MousePress, X: 120, Y: 200}) // Done
	if ctx.Screens.Len() != 0 {
		t.Error("Done did not close")
	}

	// Book and quill: two half buttons, writing, a new page, signing.
	var saved []string
	var title string
	var signed bool
	w := &BookScreen{Unsigned: true, Player: "Bilbo", Save: func(p []string, ti string, s bool) { saved, title, signed = p, ti, s }}
	ctx.Screens.Open(w)
	if g := [2]int{w.sign.X, w.done.X}; g != [2]int{113, 215} || w.sign.W != 98 {
		t.Errorf("sign/done x %v", g)
	}
	for _, r := range "Hi" {
		ctx.Screens.Input(Event{Type: CharTyped, Char: r})
	}
	ctx.Screens.Input(Event{Type: MousePress, X: 240, Y: 160}) // next adds a page
	ctx.Screens.Input(Event{Type: MousePress, X: 240, Y: 160}) // and another
	if w.total != 3 || w.page != 2 {
		t.Fatalf("pages %d, page %d", w.total, w.page)
	}
	ctx.Screens.Input(Event{Type: MousePress, X: 120, Y: 200}) // Sign
	if !w.signing || !w.finalize.Disabled || !w.next.Hidden {
		t.Fatal("sign view")
	}
	for _, r := range "Red Book of Westmarch" {
		ctx.Screens.Input(Event{Type: CharTyped, Char: r})
	}
	if w.title != "Red Book of West" {
		t.Errorf("title %q (16 chars max)", w.title)
	}
	ctx.Screens.Draw(0, 0, 0)
	ctx.Screens.Input(Event{Type: MousePress, X: 120, Y: 200}) // Sign and Close
	if !signed || title != "Red Book of West" || !reflect.DeepEqual(saved, []string{"Hi"}) || ctx.Screens.Len() != 0 {
		t.Errorf("signed %v %q %q", signed, title, saved)
	}
}

// TestBookInsertLimit: a page takes text while its lines with the cursor
// fit 118 px (13 lines of 9 px with the vanilla font).
func TestBookInsertLimit(t *testing.T) {
	ctx, _ := testContext(t)
	w := &BookScreen{Unsigned: true}
	ctx.Screens.Open(w)
	for range 20 {
		ctx.Screens.Input(Event{Type: KeyPress, Key: KeyEnter})
	}
	if n := len([]rune(w.Pages[0])); n != 12 {
		t.Errorf("%d newlines fit, want 12 (13 lines with the cursor)", n)
	}
}
