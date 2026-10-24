package ui

import (
	"image"
	"image/draw"
	"math"
)

// GlintIcon is a slot icon with the enchantment glint at ms
// (getSystemTime), the 1.7.10 reference's RenderItem.renderEffect over a
// 16x16 icon at a slot's x, y: a 20x20 quad at x-2, y-2 drawn with depth
// equal (so on the icon's texels with alpha > 0.1), colour (0.5, 0.25,
// 0.8) and blend (dst alpha, one; alpha zero, zero), its texture
// coordinates u = (f2 + px + 4py)/256, v = py/256 over the quad's pixels,
// f2 = ms%3000/3000*256. That blend zeroes the target's alpha, so the
// second layer (f2 over 4873 ms, -py) adds nothing: one layer shows, on
// a target whose alpha was 1. The glint texture is sampled linearly and
// repeated (its pack metadata's blur). The result is k times the icon's
// size, so the glint keeps its gradient over a scaled GUI.
func GlintIcon(icon, glint image.Image, ms int64, k int) *image.NRGBA {
	ic, gl := asNRGBA(icon), asNRGBA(glint)
	b := ic.Rect
	w, h := b.Dx()*k, b.Dy()*k
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	f2 := float64(ms%3000) / 3000 * 256
	for y := range h {
		py := 2 + (float64(y)+0.5)*16/float64(h)
		for x := range w {
			i := ic.PixOffset(b.Min.X+x/k, b.Min.Y+y/k)
			c := ic.Pix[i : i+4 : i+4]
			o := out.Pix[out.PixOffset(x, y):]
			copy(o, c)
			if float64(c[3])/255 > 0.1 {
				px := 2 + (float64(x)+0.5)*16/float64(w)
				r, g, bl := sampleRepeat(gl, (f2+px+4*py)/256, py/256)
				o[0], o[1], o[2] = addSat(c[0], r*0.5), addSat(c[1], g*0.25), addSat(c[2], bl*0.8)
			}
		}
	}
	return out
}

func asNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	n := image.NewNRGBA(b)
	draw.Draw(n, b, img, b.Min, draw.Src)
	return n
}

func addSat(c uint8, add float64) uint8 {
	return uint8(min(math.Round(float64(c)+add*255), 255))
}

// sampleRepeat is GL_LINEAR with GL_REPEAT at normalized u, v: rgb 0..1.
func sampleRepeat(img *image.NRGBA, u, v float64) (r, g, b float64) {
	bd := img.Rect
	w, h := bd.Dx(), bd.Dy()
	fx, fy := u*float64(w)-0.5, v*float64(h)-0.5
	x0, y0 := math.Floor(fx), math.Floor(fy)
	tx, ty := fx-x0, fy-y0
	for j, wt := range [4]float64{(1 - tx) * (1 - ty), tx * (1 - ty), (1 - tx) * ty, tx * ty} {
		x, y := ((int(x0)+j%2)%w+w)%w, ((int(y0)+j/2)%h+h)%h
		p := img.Pix[img.PixOffset(bd.Min.X+x, bd.Min.Y+y):]
		r, g, b = r+float64(p[0])/255*wt, g+float64(p[1])/255*wt, b+float64(p[2])/255*wt
	}
	return
}
