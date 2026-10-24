package ui

import (
	"image"
	"image/draw"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/memmaker/voxellib/inventory"
)

// GlintTexture is the default glint texture's resource location
// (RenderItem.RES_ITEM_GLINT), loaded from Context.Assets on first use.
const GlintTexture = "textures/misc/enchanted_item_glint.png"

// glintScale is a glinting copy's size over its icon's: the glint
// changes within a GUI pixel at GUI scales up to 4.
const glintScale = 4

// glintState is a Context's slot glint: the texture, each icon's passes
// and its glinting copy, redrawn when the clock moves.
type glintState struct {
	mu     sync.Mutex
	tex    image.Image
	tried  bool
	images map[Texture]image.Image // SetTexture's images: a sprite's default passes
	srcs   map[*Sprite]glintSrc
	copies map[*Sprite]*glintCopy
}

// glintSrc is an icon's tinted passes; first: only pass 0 glints.
type glintSrc struct {
	passes []*image.NRGBA
	first  bool
}

type glintCopy struct {
	sp  *Sprite
	loc string
	ms  int64
}

// glints is ctx's glint state, made for a Context not from NewContext.
func (ctx *Context) glints() *glintState {
	if ctx.glint == nil {
		ctx.glint = &glintState{}
	}
	return ctx.glint
}

// ItemOf is what a slot shows for s with icon: its count, and the glint
// when s.Glints(). nil or an empty stack shows nothing.
func ItemOf(s *inventory.Stack, icon *Sprite) ItemStack {
	if s == nil || s.Item == nil || s.Count <= 0 {
		return ItemStack{}
	}
	return ItemStack{Icon: icon, Count: s.Count, Glint: s.Glints()}
}

// withGlint adds s's glint to what a game's item func made of it.
func withGlint(st ItemStack, s *inventory.Stack) ItemStack {
	st.Glint = st.Glint || !st.Empty() && s.Glints()
	return st
}

// Sprite is the icon as a slot draws it: glinting when Glint (DrawItem).
func (s ItemStack) Sprite(ctx *Context) *Sprite {
	if s.Glint && s.Icon != nil {
		return ctx.glinted(s.Icon)
	}
	return s.Icon
}

// SetGlintTexture sets the slot glint's texture in place of GlintTexture.
func (ctx *Context) SetGlintTexture(img image.Image) {
	g := ctx.glints()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tex, g.tried = img, true
}

// GlintTexture is the slot glint's texture: SetGlintTexture's, else
// GlintTexture from Assets (loaded once); nil without one.
func (ctx *Context) GlintTexture() image.Image {
	g := ctx.glints()
	g.mu.Lock()
	defer g.mu.Unlock()
	return ctx.glintTexture()
}

// glintTexture is GlintTexture with ctx.glint.mu held.
func (ctx *Context) glintTexture() image.Image {
	g := ctx.glints()
	if !g.tried {
		g.tried = true
		if ctx.Assets != nil {
			img, err := loadImage(ctx.Assets, AssetPath(GlintTexture))
			if err != nil {
				log.Printf("ui: glint: %v", err)
			} else {
				g.tex = img
			}
		}
	}
	return g.tex
}

// SetGlintPasses gives sp's glint its passes, each drawn over the last and
// glinting where it is (renderItemIntoGUI's renderEffect after each pass
// with hasEffect(pass)); first: only pass 0 glints (a potion's overlay).
// No passes: sp never glints (a 3D block icon). A sprite without passes
// glints its SetTexture image, or not at all.
func (ctx *Context) SetGlintPasses(sp *Sprite, passes []*image.NRGBA, first bool) {
	g := ctx.glints()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.srcs == nil {
		g.srcs = map[*Sprite]glintSrc{}
	}
	g.srcs[sp] = glintSrc{passes, first}
}

// glinted is sp with the glint at the system time; sp itself without a
// glint texture or passes.
func (ctx *Context) glinted(sp *Sprite) *Sprite {
	g := ctx.glints()
	g.mu.Lock()
	defer g.mu.Unlock()
	if ctx.glintTexture() == nil {
		return sp
	}
	src, ok := g.srcs[sp]
	if !ok {
		img := g.images[sp.Tex]
		if img == nil {
			return sp
		}
		src = glintSrc{passes: []*image.NRGBA{spriteImage(img, sp)}}
		if g.srcs == nil {
			g.srcs = map[*Sprite]glintSrc{}
		}
		g.srcs[sp] = src
	}
	if len(src.passes) == 0 {
		return sp
	}
	ms := time.Now().UnixMilli()
	c := g.copies[sp]
	if c != nil && c.ms == ms {
		return c.sp
	}
	out := glintPasses(src, g.tex, ms)
	if c != nil {
		c.ms = ms
		if u, ok := c.sp.Tex.(interface{ Update(image.Image) bool }); ok && u.Update(out) {
			return c.sp
		}
	}
	if c == nil {
		if g.copies == nil {
			g.copies = map[*Sprite]*glintCopy{}
		}
		c = &glintCopy{sp: &Sprite{}, loc: "voxellib:glint/" + strconv.Itoa(len(g.copies)), ms: ms}
		g.copies[sp] = c
	}
	t := ctx.setTexture(c.loc, out) // a canvas without Update: a new texture each time
	w, h := t.Size()
	*c.sp = Sprite{Tex: t, W: float32(w), H: float32(h), TW: float32(w), TH: float32(h)}
	return c.sp
}

// spriteImage is sp's region of its texture's image.
func spriteImage(img image.Image, sp *Sprite) *image.NRGBA {
	b := img.Bounds()
	tw, th := sp.sheet()
	kx, ky := float32(b.Dx())/tw, float32(b.Dy())/th
	r := image.Rect(int(sp.U*kx), int(sp.V*ky), int((sp.U+sp.W)*kx), int((sp.V+sp.H)*ky)).Add(b.Min)
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Rect, img, r.Min, draw.Src)
	return out
}

// glintPasses is renderItemIntoGUI's multi-pass glint: each pass drawn
// over the last, then glinting where it is (renderEffect after each pass
// with hasEffect(pass); the glint's blend zeroes the alpha it read).
func glintPasses(src glintSrc, glint image.Image, ms int64) *image.NRGBA {
	if len(src.passes) == 1 {
		return GlintIcon(src.passes[0], glint, ms, glintScale)
	}
	var out *image.NRGBA
	for i, p := range src.passes {
		var l *image.NRGBA
		if i == 0 || !src.first {
			l = GlintIcon(p, glint, ms, glintScale)
		} else {
			l = upscale(p, glintScale)
		}
		if out == nil {
			out = l
		} else {
			draw.Draw(out, out.Rect, l, image.Point{}, draw.Over)
		}
	}
	return out
}

// upscale is img k times larger, nearest.
func upscale(img *image.NRGBA, k int) *image.NRGBA {
	b := img.Rect
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*k, b.Dy()*k))
	for y := range out.Rect.Dy() {
		for x := range out.Rect.Dx() {
			i := img.PixOffset(b.Min.X+x/k, b.Min.Y+y/k)
			copy(out.Pix[out.PixOffset(x, y):], img.Pix[i:i+4])
		}
	}
	return out
}

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
