// Package raster draws model meshes on the CPU the way OpenGL's fixed
// function pipeline draws them with an orthographic projection: flat
// lit, nearest texel sampling, a depth buffer and an alpha test. It is
// for small views that must match Minecraft exactly, such as the player
// in the inventory (GuiInventory.func_147046_a).
package raster

import (
	"image"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// Light is a GL directional light: Dir points towards the light, in the
// frame the skin matrices map to; Diffuse is its GL_DIFFUSE (grey).
type Light struct {
	Dir     mgl32.Vec3
	Diffuse float32
}

// Lighting is GL lighting with GL_COLOR_MATERIAL on ambient and diffuse:
// colour = clamp(Ambient*c + sum Diffuse*max(0, n.L)*c) per channel, with
// c the vertex colour. Without lights (Off) the colour is c.
type Lighting struct {
	Off     bool
	Ambient float32 // GL_LIGHT_MODEL_AMBIENT (grey)
	Lights  []Light
}

// Target is an image with a depth buffer. Depth is GL window depth
// negated: larger is nearer, and a fragment passes with z >= the stored
// value (GL_LEQUAL).
type Target struct {
	Img   *image.NRGBA
	Depth []float32
	// AlphaRef is the alpha test (GL_GREATER AlphaRef); 0 means 0.1.
	AlphaRef float32
	// Blend draws with glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA)
	// over the image (kept non-premultiplied, so it composites like GL over
	// whatever the image is later drawn on); otherwise fragments are opaque.
	Blend bool
}

// NewTarget makes a cleared w x h target.
func NewTarget(w, h int) *Target {
	t := &Target{Img: image.NewNRGBA(image.Rect(0, 0, w, h)), Depth: make([]float32, w*h)}
	t.Clear()
	return t
}

// Clear makes every pixel transparent and as far as possible.
func (t *Target) Clear() {
	clear(t.Img.Pix)
	for i := range t.Depth {
		t.Depth[i] = float32(math.Inf(-1))
	}
}

// Draw draws m's meshes. skin maps each joint's bind-space vertices to
// the lighting frame (the light directions' frame; normals go through
// its inverse transpose and are normalized, as GL_RESCALE_NORMAL does
// for uniform scales); view maps that frame to target pixels (x right,
// y down, pixel centres at +0.5) and z (larger nearer). tex is sampled
// nearest with GL_REPEAT, multiplied by the lit colour (GL_MODULATE);
// fragments with alpha <= 0.1 are dropped (GL_GREATER 0.1) and the rest
// written opaque. Faces are not culled; GL_FLAT shading uses each
// triangle's last vertex. A nil tex samples white.
func (t *Target) Draw(m *model.Model, skin []mgl32.Mat4, view mgl32.Mat4, tex *image.NRGBA, color [4]float32, light Lighting) {
	nm := make([]mgl32.Mat3, len(skin))
	for i, s := range skin {
		nm[i] = s.Mat3().Inv().Transpose()
	}
	type vert struct {
		p  mgl32.Vec3 // target pixels and z
		uv mgl32.Vec2
		c  [3]float32 // lit colour
	}
	for _, mesh := range m.Meshes {
		vs := make([]vert, len(mesh.Verts))
		for i, v := range mesh.Verts {
			var e mgl32.Vec4
			var n mgl32.Vec3
			for k, w := range v.Weights {
				if w == 0 {
					continue
				}
				j := v.Joints[k]
				e = e.Add(skin[j].Mul4x1(v.Pos.Vec4(1)).Mul(w))
				n = n.Add(nm[j].Mul3x1(v.Normal).Mul(w))
			}
			vs[i] = vert{p: view.Mul4x1(e).Vec3(), uv: v.UV, c: lit(n, color, light)}
		}
		for i := 0; i+2 < len(mesh.Indices); i += 3 {
			a, b, c := vs[mesh.Indices[i]], vs[mesh.Indices[i+1]], vs[mesh.Indices[i+2]]
			t.tri([3]mgl32.Vec3{a.p, b.p, c.p}, [3]mgl32.Vec2{a.uv, b.uv, c.uv}, c.c, color[3], tex)
		}
	}
}

// lit is the fixed-function vertex colour for normal n.
func lit(n mgl32.Vec3, c [4]float32, l Lighting) [3]float32 {
	if l.Off {
		return [3]float32{c[0], c[1], c[2]}
	}
	if n.Len() > 0 {
		n = n.Normalize()
	}
	k := l.Ambient
	for _, li := range l.Lights {
		k += li.Diffuse * max(0, n.Dot(li.Dir.Normalize()))
	}
	return [3]float32{min(1, k*c[0]), min(1, k*c[1]), min(1, k*c[2])}
}

// tri fills one triangle at pixel centres with a top-left fill rule.
func (t *Target) tri(p [3]mgl32.Vec3, uv [3]mgl32.Vec2, col [3]float32, alpha float32, tex *image.NRGBA) {
	edge := func(a, b mgl32.Vec3, x, y float32) float32 { return (b[0]-a[0])*(y-a[1]) - (b[1]-a[1])*(x-a[0]) }
	area := edge(p[0], p[1], p[2][0], p[2][1])
	if area == 0 || math.IsNaN(float64(area)) {
		return
	}
	if area < 0 { // either winding: culling is off
		p[1], p[2] = p[2], p[1]
		uv[1], uv[2] = uv[2], uv[1]
		area = -area
	}
	w, h := t.Img.Rect.Dx(), t.Img.Rect.Dy()
	x0 := max(0, int(math.Floor(float64(min(p[0][0], p[1][0], p[2][0])))))
	x1 := min(w-1, int(math.Ceil(float64(max(p[0][0], p[1][0], p[2][0])))))
	y0 := max(0, int(math.Floor(float64(min(p[0][1], p[1][1], p[2][1])))))
	y1 := min(h-1, int(math.Ceil(float64(max(p[0][1], p[1][1], p[2][1])))))
	// Top-left rule (y down, clockwise on screen after the swap): a pixel
	// centre on an edge belongs to the triangle only for top and left edges.
	topLeft := func(a, b mgl32.Vec3) bool {
		dx, dy := b[0]-a[0], b[1]-a[1]
		return dy < 0 || dy == 0 && dx > 0
	}
	in := func(w float32, tl bool) bool { return w > 0 || w == 0 && tl }
	t0, t1, t2 := topLeft(p[1], p[2]), topLeft(p[2], p[0]), topLeft(p[0], p[1])
	var tw, th int
	if tex != nil {
		tw, th = tex.Rect.Dx(), tex.Rect.Dy()
	}
	for y := y0; y <= y1; y++ {
		cy := float32(y) + 0.5
		for x := x0; x <= x1; x++ {
			cx := float32(x) + 0.5
			w0 := edge(p[1], p[2], cx, cy)
			w1 := edge(p[2], p[0], cx, cy)
			w2 := edge(p[0], p[1], cx, cy)
			if !in(w0, t0) || !in(w1, t1) || !in(w2, t2) {
				continue
			}
			l0, l1, l2 := w0/area, w1/area, w2/area
			z := l0*p[0][2] + l1*p[1][2] + l2*p[2][2]
			i := y*w + x
			if z < t.Depth[i] {
				continue
			}
			r, g, b, a := float32(1), float32(1), float32(1), float32(1)
			if tex != nil {
				u := l0*uv[0][0] + l1*uv[1][0] + l2*uv[2][0]
				v := l0*uv[0][1] + l1*uv[1][1] + l2*uv[2][1]
				iu := mod(int(math.Floor(float64(u*float32(tw)))), tw)
				iv := mod(int(math.Floor(float64(v*float32(th)))), th)
				s := tex.Pix[tex.PixOffset(tex.Rect.Min.X+iu, tex.Rect.Min.Y+iv):]
				r, g, b, a = float32(s[0])/255, float32(s[1])/255, float32(s[2])/255, float32(s[3])/255
			}
			ref := t.AlphaRef
			if ref == 0 {
				ref = 0.1
			}
			if a*alpha <= ref {
				continue
			}
			t.Depth[i] = z
			o := t.Img.Pix[y*t.Img.Stride+x*4:]
			if t.Blend {
				t.over(o, r*col[0], g*col[1], b*col[2], a*alpha)
				continue
			}
			o[0], o[1], o[2], o[3] = unorm(r*col[0]), unorm(g*col[1]), unorm(b*col[2]), 255
		}
	}
}

// over composites a fragment over pixel o (non-premultiplied).
func (t *Target) over(o []uint8, r, g, b, a float32) {
	da := float32(o[3]) / 255
	oa := a + da*(1-a)
	if oa <= 0 {
		return
	}
	mix := func(s float32, d uint8) uint8 { return unorm((s*a + float32(d)/255*da*(1-a)) / oa) }
	o[0], o[1], o[2], o[3] = mix(r, o[0]), mix(g, o[1]), mix(b, o[2]), unorm(oa)
}

func mod(a, n int) int { return ((a % n) + n) % n }

// unorm converts to 8 bits as GL does (round to nearest).
func unorm(f float32) uint8 { return uint8(min(1, max(0, f))*255 + 0.5) }
