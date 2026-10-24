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

// TestGlintPasses: a potion glints its overlay (pass 0) only, other
// multi-pass icons every pass, each pass drawn over the last.
func TestGlintPasses(t *testing.T) {
	px := func(x int, c color.NRGBA) *image.NRGBA {
		img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
		img.SetNRGBA(x, 0, c)
		return img
	}
	overlay, bottle := px(0, color.NRGBA{0, 0, 0, 255}), px(1, color.NRGBA{0, 0, 0, 255})
	glint := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range glint.Pix {
		glint.Pix[i] = 255
	}
	lit := func(img *image.NRGBA, x int) bool { return img.NRGBAAt(x*glintScale, 0).R > 0 }
	pot := glintPasses(glintSrc{[]*image.NRGBA{overlay, bottle}, true}, glint, 0)
	if !lit(pot, 0) || lit(pot, 1) || pot.NRGBAAt(glintScale, 0).A != 255 {
		t.Fatalf("potion: overlay lit %v, bottle lit %v", lit(pot, 0), lit(pot, 1))
	}
	other := glintPasses(glintSrc{[]*image.NRGBA{overlay, bottle}, false}, glint, 0)
	if !lit(other, 0) || !lit(other, 1) {
		t.Fatal("multi-pass: every pass glints")
	}
}
