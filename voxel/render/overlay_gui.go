package render

import (
	"image"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// GUITexture is an image uploaded for GUIBatch (GUI sheets, font pages,
// item icons). It samples nearest and repeats, so UVs past 1 tile (Minecraft's
// options background).
type GUITexture struct {
	tex  texture2D
	bg   *wgpu.BindGroup
	w, h int
}

// NewGUITexture uploads img. Call after Init.
func NewGUITexture(img image.Image) *GUITexture {
	px := toNRGBARect(img)
	w, h := px.Rect.Dx(), px.Rect.Dy()
	t := newTexture("gui", w, h, 1, 1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2D)
	t.write(0, 0, w, h, 4, px.Pix)
	bg := bindGroup("gui texture", dev.pipes.tex2D,
		wgpu.BindGroupEntry{Binding: 0, TextureView: t.view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestRep})
	return &GUITexture{t, bg, w, h}
}

// Size returns the image size in pixels.
func (t *GUITexture) Size() (w, h int) { return t.w, t.h }

// Release frees the GPU texture.
func (t *GUITexture) Release() {
	if t.bg != nil {
		t.bg.Release()
		t.tex.release()
		t.bg = nil
	}
}

// GUIBatch collects 2D quads in GUI pixels (Minecraft's scaled coordinates,
// top-left origin) and draws them in submission order with as few draw calls
// as texture, scissor and blend changes allow. Fill it, then Flush once per
// frame after the world and the hand.
type GUIBatch struct {
	verts []float32 // x, y, u, v, rgba (bits) per vertex
	segs  []guiSeg
	cur   guiSeg
}

type guiSeg struct {
	tex     *GUITexture
	scissor [4]int // GUI px x, y, w, h; w <= 0: none
	invert  bool
	first   int // vertex
}

// Quad adds the rectangle x0,y0-x1,y1 sampling u0,v0-u1,v1 (normalized;
// beyond 1 repeats). The top edge is shifted right by skew and the bottom
// left by skew (italic glyphs). top and bottom are 0xAARRGGBB vertex colours
// multiplied with the texture; t nil draws plain colour.
func (b *GUIBatch) Quad(t *GUITexture, x0, y0, x1, y1, u0, v0, u1, v1, skew float32, top, bottom uint32) {
	if t == nil {
		t = whiteTexture()
	}
	b.state(t, b.cur.scissor, b.cur.invert)
	ct, cb := argbToRGBA(top), argbToRGBA(bottom)
	b.verts = append(b.verts,
		x0+skew, y0, u0, v0, ct, x0-skew, y1, u0, v1, cb, x1+skew, y0, u1, v0, ct,
		x1+skew, y0, u1, v0, ct, x0-skew, y1, u0, v1, cb, x1-skew, y1, u1, v1, cb)
}

// Scissor clips later quads to x,y,w,h in GUI pixels; w <= 0 turns it off.
func (b *GUIBatch) Scissor(x, y, w, h int) { b.state(b.cur.tex, [4]int{x, y, w, h}, b.cur.invert) }

// Invert switches the blend to colour inversion (Minecraft's crosshair:
// ONE_MINUS_DST_COLOR, ONE_MINUS_SRC_COLOR) or back to alpha blending.
func (b *GUIBatch) Invert(on bool) { b.state(b.cur.tex, b.cur.scissor, on) }

func (b *GUIBatch) state(t *GUITexture, sc [4]int, inv bool) {
	if t == b.cur.tex && sc == b.cur.scissor && inv == b.cur.invert {
		return
	}
	n := len(b.verts) / 5
	if n > b.cur.first && b.cur.tex != nil {
		b.segs = append(b.segs, b.cur)
	}
	b.cur = guiSeg{tex: t, scissor: sc, invert: inv, first: n}
}

func argbToRGBA(c uint32) float32 {
	return math.Float32frombits(c>>16&0xff | c&0xff00 | (c&0xff)<<16 | c&0xff000000)
}

// Flush draws everything added since the last Flush into the frame, one GUI
// pixel being scale framebuffer pixels on a fbW x fbH framebuffer, and
// empties the batch.
func (b *GUIBatch) Flush(fbW, fbH int, scale float32) {
	n := len(b.verts) / 5
	if n > b.cur.first && b.cur.tex != nil {
		b.segs = append(b.segs, b.cur)
	}
	segs, verts := b.segs, b.verts
	b.segs, b.verts = b.segs[:0], b.verts[:0]
	b.cur = guiSeg{scissor: b.cur.scissor, invert: b.cur.invert}
	p := curPass()
	if p == nil || len(segs) == 0 || fbW == 0 || fbH == 0 {
		return
	}
	pipes := guiPipelines()
	c, off := dev.vertices.alloc(bytesOf(verts), 0)
	var u ubuf
	bindUniforms(p, u.f(2*scale/float32(fbW), -2*scale/float32(fbH), -1, 1))
	p.SetVertexBuffer(0, c.buf, uint64(off))
	inv := -1
	for i, s := range segs {
		if v := b2i(s.invert); v != inv {
			p.SetPipeline(pipes[v])
			inv = v
		}
		p.SetScissorRect(scissorRect(s.scissor, fbW, fbH, scale))
		p.SetBindGroup(1, s.tex.bg, nil)
		end := n
		if i+1 < len(segs) {
			end = segs[i+1].first
		}
		p.Draw(gputypes.DrawArgs{VertexCount: uint32(end - s.first), InstanceCount: 1, FirstVertex: uint32(s.first)})
	}
	p.SetScissorRect(gputypes.ScissorRect{Width: uint32(fbW), Height: uint32(fbH)})
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scissorRect(s [4]int, fbW, fbH int, scale float32) gputypes.ScissorRect {
	if s[2] <= 0 || s[3] <= 0 {
		return gputypes.ScissorRect{Width: uint32(fbW), Height: uint32(fbH)}
	}
	cl := func(v float32, hi int) int { return min(max(int(v), 0), hi) }
	x0, y0 := cl(float32(s[0])*scale, fbW), cl(float32(s[1])*scale, fbH)
	x1, y1 := cl(float32(s[0]+s[2])*scale, fbW), cl(float32(s[1]+s[3])*scale, fbH)
	return gputypes.ScissorRect{X: uint32(x0), Y: uint32(y0), Width: uint32(x1 - x0), Height: uint32(y1 - y0)}
}

const guiWGSL = `
struct U { xf: vec4f } // scale x, scale y, offset x, offset y: GUI px -> NDC
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
struct VOut { @builtin(position) pos: vec4f, @location(0) uv: vec2f, @location(1) color: vec4f }
@vertex
fn vs(@location(0) p: vec2f, @location(1) uv: vec2f, @location(2) c: vec4f) -> VOut {
	var o: VOut;
	o.pos = vec4f(p * u.xf.xy + u.xf.zw, 0.0, 1.0);
	o.uv = uv;
	o.color = c;
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let c = textureSample(tex, samp, i.uv) * i.color;
	if (c.a <= 0.0) { discard; }
	return c;
}
`

var guiState struct {
	pipes [2]*wgpu.RenderPipeline // alpha, invert
	white *GUITexture
}

func whiteTexture() *GUITexture {
	if guiState.white == nil {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		copy(img.Pix, []byte{255, 255, 255, 255})
		guiState.white = NewGUITexture(img)
	}
	return guiState.white
}

func guiPipelines() [2]*wgpu.RenderPipeline {
	if guiState.pipes[0] != nil {
		return guiState.pipes
	}
	mod, err := dev.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "gui", WGSL: guiWGSL})
	if err != nil {
		panic(err)
	}
	pl, err := dev.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "gui",
		BindGroupLayouts: []*wgpu.BindGroupLayout{dev.uniLayout, dev.pipes.tex2D}})
	if err != nil {
		panic(err)
	}
	invert := &gputypes.BlendState{
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOneMinusDst, DstFactor: gputypes.BlendFactorOneMinusSrc, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorZero, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
	}
	buf := []gputypes.VertexBufferLayout{{ArrayStride: 20, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},
		{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1},
		{Format: gputypes.VertexFormatUnorm8x4, Offset: 16, ShaderLocation: 2}}}}
	for i, blend := range []*gputypes.BlendState{blendAlpha, invert} {
		if guiState.pipes[i], err = dev.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:     "gui",
			Layout:    pl,
			Vertex:    wgpu.VertexState{Module: mod, EntryPoint: "vs", Buffers: buf},
			Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, FrontFace: gputypes.FrontFaceCCW, CullMode: gputypes.CullModeNone},
			DepthStencil: &wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth32Float,
				DepthCompare: gputypes.CompareFunctionAlways},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
			Fragment: &wgpu.FragmentState{Module: mod, EntryPoint: "fs", Targets: []gputypes.ColorTargetState{
				{Format: dev.format, Blend: blend, WriteMask: gputypes.ColorWriteMaskAll}}},
		}); err != nil {
			panic(err)
		}
	}
	return guiState.pipes
}
