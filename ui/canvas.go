// Package ui is a native 2D GUI layer in the style of Minecraft 1.7.10:
// bitmap fonts (FontRenderer), sprites, widgets, a screen manager, container
// views, dialogs, a quest log, a world map and the in-game HUD. Everything is
// drawn in GUI pixels (Minecraft's scaled resolution) onto a Canvas; textures
// come from the game's resource packs (an fs.FS with the assets/<domain>/...
// layout). The package is pure Go: voxellib/ui/uiapp draws it with the GPU
// and plugs it into app; SoftCanvas renders to an image for tests.
//
// Colours are Minecraft's 0xAARRGGBB ints.
package ui

import (
	"image"
	"math"
)

// Texture is an uploaded image.
type Texture interface{ Size() (w, h int) }

// Quad is one textured, vertically shaded rectangle in GUI pixels.
type Quad struct {
	Tex            Texture // nil: plain colour
	X0, Y0, X1, Y1 float32
	U0, V0, U1, V1 float32 // normalized; beyond 1 repeats
	Skew           float32 // top edge shifted +Skew, bottom -Skew (italics)
	Top, Bottom    uint32  // 0xAARRGGBB, multiplied with the texture
	// Rot turns the placed quad by Rot degrees around RX, RY
	// (glRotatef about z: clockwise on the y-down screen).
	Rot, RX, RY float32
}

// Canvas receives the GUI's draw calls in order.
type Canvas interface {
	NewTexture(img image.Image) Texture
	Quad(q Quad)
	// SetScissor clips later quads to r (GUI px); an empty r turns it off.
	SetScissor(r image.Rectangle)
	// SetInvert switches to colour-inverting blend (crosshair) or back.
	SetInvert(on bool)
}

// ScaledResolution is Minecraft 1.7.10's GUI scale: guiScale 0 is auto,
// 1..3 small, normal, large; the factor grows while the GUI stays at least
// 320x240, and the unicode font forces an even factor. It returns the GUI
// size (rounded up) and the factor.
func ScaledResolution(fbW, fbH, guiScale int, unicode bool) (w, h, scale int) {
	scale = 1
	k := guiScale
	if k == 0 {
		k = 1000
	}
	for scale < k && fbW/(scale+1) >= 320 && fbH/(scale+1) >= 240 {
		scale++
	}
	if unicode && scale%2 != 0 && scale != 1 {
		scale--
	}
	w = int(math.Ceil(float64(fbW) / float64(scale)))
	h = int(math.Ceil(float64(fbH) / float64(scale)))
	return w, h, scale
}

// DrawRect is Gui.drawRect: a filled rectangle (corners in any order).
func DrawRect(c Canvas, x0, y0, x1, y1 int, color uint32) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	c.Quad(Quad{X0: float32(x0), Y0: float32(y0), X1: float32(x1), Y1: float32(y1), Top: color, Bottom: color})
}

// DrawGradientRect is Gui.drawGradientRect: top colour to bottom colour.
func DrawGradientRect(c Canvas, x0, y0, x1, y1 int, top, bottom uint32) {
	c.Quad(Quad{X0: float32(x0), Y0: float32(y0), X1: float32(x1), Y1: float32(y1), Top: top, Bottom: bottom})
}

// DrawTexturedModalRect is Gui.drawTexturedModalRect: a w x h part at u,v of
// a GUI sheet addressed in 256x256 units whatever its real size (HD packs).
func DrawTexturedModalRect(c Canvas, t Texture, x, y, u, v, w, h int) {
	DrawTexRect(c, t, x, y, u, v, w, h, 256, 256, 0xFFFFFFFF)
}

// DrawTexRect draws the w x h part at u,v of t, with t addressed as a
// sheet of tw x th units (Gui.func_146110_a), tinted by color.
func DrawTexRect(c Canvas, t Texture, x, y, u, v, w, h int, tw, th float32, color uint32) {
	DrawTexScaled(c, t, float32(x), float32(y), float32(w), float32(h), float32(u), float32(v), float32(w), float32(h), tw, th, color)
}

// DrawTexScaled draws the sw x sh part at u,v (sheet units tw x th) into
// the w x h rectangle at x,y.
func DrawTexScaled(c Canvas, t Texture, x, y, w, h, u, v, sw, sh, tw, th float32, color uint32) {
	c.Quad(Quad{Tex: t, X0: x, Y0: y, X1: x + w, Y1: y + h,
		U0: u / tw, V0: v / th, U1: (u + sw) / tw, V1: (v + sh) / th, Top: color, Bottom: color})
}

// Sprite is a part of a texture sheet. TW, TH are the sheet's size in the
// units U..H use (256 for Minecraft GUI sheets; 0 means the real size).
type Sprite struct {
	Tex                            Texture
	U, V, W, H                     float32
	TW, TH                         float32
	Color                          uint32  // tint; 0 means white
	SliceL, SliceT, SliceR, SliceB float32 // nine-slice borders in sheet units (0: plain stretch)
}

func (s *Sprite) sheet() (float32, float32) {
	if s.TW > 0 {
		return s.TW, s.TH
	}
	w, h := s.Tex.Size()
	return float32(w), float32(h)
}

func (s *Sprite) color() uint32 {
	if s.Color == 0 {
		return 0xFFFFFFFF
	}
	return s.Color
}

// Draw draws the sprite at its own size.
func (s *Sprite) Draw(c Canvas, x, y float32) { s.DrawSized(c, x, y, s.W, s.H) }

// DrawSized draws the sprite into w x h; with slice borders the corners keep
// their size, the edges and the middle stretch (a nine-slice).
func (s *Sprite) DrawSized(c Canvas, x, y, w, h float32) {
	if s == nil || s.Tex == nil {
		return
	}
	tw, th := s.sheet()
	col := s.color()
	l, t, r, b := s.SliceL, s.SliceT, s.SliceR, s.SliceB
	if l+t+r+b == 0 {
		DrawTexScaled(c, s.Tex, x, y, w, h, s.U, s.V, s.W, s.H, tw, th, col)
		return
	}
	xs := [4]float32{x, x + l, x + w - r, x + w}
	ys := [4]float32{y, y + t, y + h - b, y + h}
	us := [4]float32{s.U, s.U + l, s.U + s.W - r, s.U + s.W}
	vs := [4]float32{s.V, s.V + t, s.V + s.H - b, s.V + s.H}
	for j := 0; j < 3; j++ {
		for i := 0; i < 3; i++ {
			if xs[i+1] <= xs[i] || ys[j+1] <= ys[j] {
				continue
			}
			DrawTexScaled(c, s.Tex, xs[i], ys[j], xs[i+1]-xs[i], ys[j+1]-ys[j], us[i], vs[j], us[i+1]-us[i], vs[j+1]-vs[j], tw, th, col)
		}
	}
}
