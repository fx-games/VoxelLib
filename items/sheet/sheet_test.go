package sheet

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestCutSynthetic(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	on := color.NRGBA{255, 0, 0, 255}
	// sprite in cell (1,1)
	for i := 2; i < 6; i++ {
		img.Set(8+i, 8+i, on)
	}
	// "text": two adjacent cells (4,1) and (5,1) -> rejected
	img.Set(34, 10, on)
	img.Set(42, 10, on)
	// divider line across row 3 -> ignored
	for x := 0; x < 64; x++ {
		img.Set(x, 24, on)
	}
	got := Cut(img, Options{Cell: 8, Sections: []Section{{"a", image.Rect(0, 0, 32, 32)}}})
	if len(got) != 1 || got[0].Name != "a_01" || got[0].Rect != image.Rect(8, 8, 16, 16) {
		t.Fatalf("got %+v", got)
	}
}

func TestCutIronchest(t *testing.T) {
	p := filepath.Join(os.Getenv("HOME"), "Downloads/Ironchests_RPG_items_pico8_transparentbg_v1.png")
	f, err := os.Open(p)
	if err != nil {
		t.Skip("sheet not available")
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	got := Cut(img, Ironchest)
	if len(got) != 779 {
		t.Fatalf("got %d sprites, want 779", len(got))
	}
	for _, s := range got {
		if s.Name[:5] == "item_" {
			t.Errorf("%v outside any section", s.Rect)
		}
	}
}
