package ui

import (
	"image"
	"image/draw"
	"math"
)

// SoftCanvas rasterizes the GUI into an image on the CPU (tests, previews):
// nearest sampling, repeat, straight alpha blending, Scale image pixels per
// GUI pixel.
type SoftCanvas struct {
	Img    *image.NRGBA
	Scale  float32
	clip   image.Rectangle
	invert bool
}

type softTex struct{ img *image.NRGBA }

func (t *softTex) Size() (int, int) { return t.img.Rect.Dx(), t.img.Rect.Dy() }

// NewSoftCanvas makes a w x h pixel canvas.
func NewSoftCanvas(w, h int, scale float32) *SoftCanvas {
	return &SoftCanvas{Img: image.NewNRGBA(image.Rect(0, 0, w, h)), Scale: scale}
}

func (s *SoftCanvas) NewTexture(img image.Image) Texture {
	n := image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(n, n.Rect, img, img.Bounds().Min, draw.Src)
	return &softTex{n}
}

func (s *SoftCanvas) SetScissor(r image.Rectangle) {
	if r.Empty() {
		s.clip = image.Rectangle{}
		return
	}
	sc := float64(s.Scale)
	s.clip = image.Rect(int(float64(r.Min.X)*sc), int(float64(r.Min.Y)*sc), int(float64(r.Max.X)*sc), int(float64(r.Max.Y)*sc))
}

func (s *SoftCanvas) SetInvert(on bool) { s.invert = on }

func (s *SoftCanvas) Quad(q Quad) {
	sc := s.Scale
	fx0, fy0, fx1, fy1 := q.X0*sc, q.Y0*sc, q.X1*sc, q.Y1*sc
	skew := q.Skew * sc
	bounds := s.Img.Rect
	if !s.clip.Empty() {
		bounds = bounds.Intersect(s.clip)
	}
	if fy1 <= fy0 || fx1 <= fx0 {
		return
	}
	var tex *image.NRGBA
	if t, ok := q.Tex.(*softTex); ok {
		tex = t.img
	}
	py0 := max(int(math.Ceil(float64(fy0-0.5))), bounds.Min.Y)
	py1 := min(int(math.Ceil(float64(fy1-0.5))), bounds.Max.Y)
	for py := py0; py < py1; py++ {
		cy := float32(py) + 0.5
		ty := (cy - fy0) / (fy1 - fy0)
		off := skew * (1 - 2*ty)
		xl, xr := fx0+off, fx1+off
		col := lerpColor(q.Top, q.Bottom, ty)
		px0 := max(int(math.Ceil(float64(xl-0.5))), bounds.Min.X)
		px1 := min(int(math.Ceil(float64(xr-0.5))), bounds.Max.X)
		for px := px0; px < px1; px++ {
			tx := (float32(px) + 0.5 - xl) / (xr - xl)
			r, g, b, a := col[0], col[1], col[2], col[3]
			if tex != nil {
				u := q.U0 + (q.U1-q.U0)*tx
				v := q.V0 + (q.V1-q.V0)*ty
				w, h := tex.Rect.Dx(), tex.Rect.Dy()
				iu := mod(int(math.Floor(float64(u*float32(w)))), w)
				iv := mod(int(math.Floor(float64(v*float32(h)))), h)
				p := tex.Pix[iv*tex.Stride+iu*4:]
				r, g, b, a = r*float32(p[0])/255, g*float32(p[1])/255, b*float32(p[2])/255, a*float32(p[3])/255
			}
			if a <= 0 {
				continue
			}
			d := s.Img.Pix[py*s.Img.Stride+px*4:]
			dr, dg, db, da := float32(d[0])/255, float32(d[1])/255, float32(d[2])/255, float32(d[3])/255
			if s.invert {
				d[0], d[1], d[2] = u8(r*(1-dr)+dr*(1-r)), u8(g*(1-dg)+dg*(1-g)), u8(b*(1-db)+db*(1-b))
				continue
			}
			oa := a + da*(1-a)
			d[0] = u8((r*a + dr*da*(1-a)) / oa)
			d[1] = u8((g*a + dg*da*(1-a)) / oa)
			d[2] = u8((b*a + db*da*(1-a)) / oa)
			d[3] = u8(oa)
		}
	}
}

func mod(a, n int) int { return ((a % n) + n) % n }

func u8(f float32) uint8 { return uint8(min(max(f, 0), 1)*255 + 0.5) }

func lerpColor(a, b uint32, t float32) [4]float32 {
	var out [4]float32
	for i, sh := range [4]uint{16, 8, 0, 24} {
		ca, cb := float32(a>>sh&255), float32(b>>sh&255)
		out[i] = (ca + (cb-ca)*t) / 255
	}
	return out
}
