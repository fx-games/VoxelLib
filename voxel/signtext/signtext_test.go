package signtext

import (
	"image"
	"testing"
)

func TestImage(t *testing.T) {
	s, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Image([4]string{}); ok {
		t.Error("blank sign drew text")
	}
	img, ok := s.Image([4]string{"", "HI"})
	if !ok {
		t.Fatal("no text")
	}
	// The ink lies in line 2 (rows 10..19), centred: as much margin left as right.
	minX, maxX := boxW, -1
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < boxW; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				continue
			}
			if y < lineH || y >= 2*lineH {
				t.Fatalf("ink at row %d, outside line 2", y)
			}
			minX, maxX = min(minX, x), max(maxX, x)
		}
	}
	if l, r := minX, boxW-1-maxX; l-r > 2 || r-l > 2 {
		t.Errorf("not centred: %d px left, %d right", l, r)
	}
	// An overlong line is squeezed: ink reaches both ends of the box.
	img, _ = s.Image([4]string{"Project Leader Rafly"})
	if img.NRGBAAt(0, 0).A != 0 || !inkIn(img, 0, 3) || !inkIn(img, boxW-4, boxW) {
		t.Error("overlong line not squeezed to the box")
	}
}

// inkIn reports ink in columns x0..x1 of line 1.
func inkIn(img *image.NRGBA, x0, x1 int) bool {
	for y := range lineH {
		for x := x0; x < x1; x++ {
			if img.NRGBAAt(x, y).A != 0 {
				return true
			}
		}
	}
	return false
}
