package render

import (
	"image"
	"math"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The enchantment glint: a scrolling texture drawn twice over a glinting
// item or model with blend (src colour, one), lighting off, colour
// GlintColor, depth equal, its texture coordinates moved by a texture
// matrix per layer (the 1.7.10 reference's RenderItem / ItemRenderer
// effect pass and RendererLivingEntity's armour pass).

// GlintColor is glColor4f(0.5*0.76, 0.25*0.76, 0.8*0.76, 1).
var GlintColor = [3]float32{0.5 * 0.76, 0.25 * 0.76, 0.8 * 0.76}

// glintAffine maps (u, v, 1) to glint texture coordinates: u' = a[0]u +
// a[1]v + a[2], v' = a[3]u + a[4]v + a[5].
type glintAffine [6]float32

// glintTRS is the GL texture matrix scale(s) * translate(tx, ty) *
// rotate(deg about Z) (rotFirst: translate after the rotation, i.e. the
// calls glScalef, glTranslatef, glRotatef) or scale * rotate * translate
// (glScalef, glRotatef, glTranslatef).
func glintTRS(s, tx, ty, deg float32, rotFirst bool) glintAffine {
	sn, cs := math.Sincos(float64(deg) * math.Pi / 180)
	c, n := float32(cs), float32(sn)
	if rotFirst { // s * (R uv + t)
		return glintAffine{s * c, -s * n, s * tx, s * n, s * c, s * ty}
	}
	// s * R (uv + t)
	return glintAffine{s * c, -s * n, s * (c*tx - n*ty), s * n, s * c, s * (n*tx + c*ty)}
}

// itemGlint is a flat item's layer 0 or 1 at ms (Minecraft.getSystemTime):
// glScalef(0.125), glTranslatef(t, 0, 0), glRotatef(-50 / +10, 0, 0, 1),
// t = ms%3000/3000*8 and -(ms%4873/4873*8). The glint pass's
// renderItemIn2D(0, 0, 1, 1) puts u = x, v = 1 - y on the item's 0..1
// square.
func itemGlint(ms int64, layer int) glintAffine {
	if layer == 0 {
		return glintTRS(0.125, float32(ms%3000)/3000*8, 0, -50, true)
	}
	return glintTRS(0.125, -float32(ms%4873)/4873*8, 0, 10, true)
}

// modelGlint is an armour model's layer k at ticks (ticksExisted +
// partial): glScalef(1/3), glRotatef(30 - 60k, 0, 0, 1), glTranslatef(0,
// ticks*(0.001 + 0.003k)*20, 0), on the model's own texture coordinates.
func modelGlint(ticks float32, k int) glintAffine {
	return glintTRS(0.33333334, 0, ticks*(0.001+float32(k)*0.003)*20, 30-float32(k)*60, false)
}

// glintClock is Minecraft.getSystemTime in milliseconds.
var glintClock = func() int64 { return time.Now().UnixMilli() }

// glintTex is the glint texture's bind group (SetGlintTexture); nil draws
// no glint.
var glintTex struct {
	tex  texture2D
	samp *wgpu.Sampler
	bg   *wgpu.BindGroup
}

// SetGlintTexture sets the glint texture, sampled linearly and repeated
// (its pack metadata's blur).
func (r *Renderer) SetGlintTexture(img image.Image) error {
	g := &glintTex
	if g.bg != nil {
		g.bg.Release()
		g.tex.release()
	}
	if g.samp == nil {
		s, err := dev.device.CreateSampler(&wgpu.SamplerDescriptor{
			Label:        "glint",
			AddressModeU: gputypes.AddressModeRepeat, AddressModeV: gputypes.AddressModeRepeat, AddressModeW: gputypes.AddressModeRepeat,
			MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear, MipmapFilter: gputypes.FilterModeNearest,
			LodMaxClamp: 32,
		})
		if err != nil {
			return err
		}
		g.samp = s
	}
	g.tex = uploadTexture2D(img)
	g.bg = bindGroup("glint", dev.pipes.tex2D,
		wgpu.BindGroupEntry{Binding: 0, TextureView: g.tex.view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: g.samp})
	return nil
}

// glintOn reports whether a glint pass can draw: a texture is set.
func glintOn() bool { return glintTex.bg != nil }

// bindGlint binds the glint texture after setPipe: group 2, or 3 in a
// shader pack frame (gbuffers_armor_glint; group 2 is the pack's).
func bindGlint(p *wgpu.RenderPassEncoder) {
	g := uint32(2)
	if dev.pk != nil {
		g = 3
	}
	p.SetBindGroup(g, glintTex.bg, nil)
}

// blendGlint is glBlendFunc(GL_SRC_COLOR, GL_ONE), the target's alpha kept.
var blendGlint = &gputypes.BlendState{
	Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorSrc, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
	Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorZero, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
}
