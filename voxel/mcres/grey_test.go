package mcres

import (
	"image"
	"image/color"
	"testing"
)

func TestIsGreyscale(t *testing.T) {
	img := func(c color.NRGBA) image.Image {
		m := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		m.SetNRGBA(1, 1, c)
		m.SetNRGBA(0, 0, color.NRGBA{128, 128, 128, 255})
		return m
	}
	if !IsGreyscale(img(color.NRGBA{175, 163, 140, 255})) { // Pixel Perfection oak leaves
		t.Fatal("warm grey leaves should be tinted")
	}
	if IsGreyscale(img(color.NRGBA{60, 120, 40, 255})) { // painted green
		t.Fatal("painted green should not be tinted")
	}
}
