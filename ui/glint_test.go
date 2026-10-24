package ui

import (
	"image"
	"image/color"
	"testing"
)

// TestGlintIcon: the glint adds (0.5, 0.25, 0.8) times the texel at u =
// (f2 + px + 4py)/256 on the opaque texels only, saturating.
func TestGlintIcon(t *testing.T) {
	glint := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			glint.SetNRGBA(x, y, color.NRGBA{uint8(x * 4), 200, 255, 255})
		}
	}
	icon := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	icon.SetNRGBA(0, 0, color.NRGBA{0, 0, 250, 255})
	icon.SetNRGBA(1, 0, color.NRGBA{9, 9, 9, 25}) // alpha <= 0.1: no glint
	out := GlintIcon(icon, glint, 3000, 1)        // f2 = 0
	// texel (0,0): px = py = 2.5, u = 12.5/256: texel x 2.625 -> R 10.5/255
	if c := out.NRGBAAt(0, 0); c != (color.NRGBA{5, 50, 255, 255}) {
		t.Errorf("opaque texel %v", c)
	}
	if c := out.NRGBAAt(1, 0); c != (color.NRGBA{9, 9, 9, 25}) {
		t.Errorf("clear texel %v", c)
	}
	if b := GlintIcon(icon, glint, 0, 4).Bounds(); b.Dx() != 64 {
		t.Errorf("k=4 size %v", b)
	}
}
