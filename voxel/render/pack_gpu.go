package render

import (
	"time"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/memmaker/voxellib/voxel"
)

// The shader pack's frame (see pack.go), the GPU side of OptiFine's
// Shaders: Renderer.Render with Options.Pack set draws the shadow map,
// then opens the gbuffer pass (dev.pk); the world draws that follow
// record into it with the pack's programs (setPipe). The first depth
// clear (the hand) or screen-space draw ends the world: translucent
// terrain, depthtex1/2, and at the first screen-space draw (packResolve)
// the composite passes and final write the frame. Without Options.Pack
// none of this runs: dev.pk stays nil and every hook is one nil check.

// Gbuffer render targets: colortex0..7 (gcolor, gdepth, gnormal,
// composite, gaux1..4) with the pack's formats. WebGPU can't render to
// RGBA16 (unorm) or, on Metal, R11F_G11F_B10F, so those are RGBA16F:
// more precision and range than the pack's, never less.
var packFormats = [8]gputypes.TextureFormat{
	gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA16Float, gputypes.TextureFormatRGB10A2Unorm,
	gputypes.TextureFormatRGBA16Float, gputypes.TextureFormatRGBA16Float, gputypes.TextureFormatRGBA16Float,
	gputypes.TextureFormatRGBA16Float, gputypes.TextureFormatRGBA16Float,
}

// packMips: gaux1 (colortex4) and gaux4 (colortex7) are mipmapped
// (composite1 reads gaux1 at LOD 1, composite3 colortex7 at LOD 2).
var packMips = [8]int{1, 1, 1, 1, 2, 1, 1, 3}

// packGbuffer is the gbuffer pass's attachments, in fragment output
// location order: DRAWBUFFERS 0, 1, 2, 5, 6, 7.
var packGbuffer = [6]int{0, 1, 2, 5, 6, 7}

type packTarget struct {
	tex  *wgpu.Texture
	view *wgpu.TextureView   // every mip level (sampling)
	mips []*wgpu.TextureView // one level each (attachments, mip sources)
}

func newPackTarget(label string, w, h, mips int, f gputypes.TextureFormat) packTarget {
	t, err := dev.device.CreateTexture(&wgpu.TextureDescriptor{
		Label: label, Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
		MipLevelCount: uint32(mips), SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: f,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		panic(err)
	}
	view := func(base, n int) *wgpu.TextureView {
		v, err := dev.device.CreateTextureView(t, &wgpu.TextureViewDescriptor{Label: label, Format: f,
			Dimension: gputypes.TextureViewDimension2D, Aspect: gputypes.TextureAspectAll,
			BaseMipLevel: uint32(base), MipLevelCount: uint32(n), ArrayLayerCount: 1})
		if err != nil {
			panic(err)
		}
		return v
	}
	pt := packTarget{tex: t, view: view(0, mips)}
	for i := range mips {
		pt.mips = append(pt.mips, view(i, 1))
	}
	return pt
}

func (t *packTarget) release() {
	if t.tex == nil {
		return
	}
	for _, v := range t.mips {
		v.Release()
	}
	t.view.Release()
	t.tex.Release()
	*t = packTarget{}
}

// packVariant names a pack pipeline made from an engine pipeline.
type packVariant struct {
	pl   *wgpu.RenderPipeline
	hand bool
}

type packGPU struct {
	w, h int
	st   packState
	last time.Time
	u    packUniforms
	ubuf *wgpu.Buffer

	a, b                       [8]packTarget // ping-pong sides; the gbuffers write a
	depth                      [3]packTarget // depthtex0 (attachment), depthtex1 (no translucent), depthtex2 (no hand)
	shadow, shadow1, shadowCol packTarget    // shadowtex0, shadowtex1 (opaque only), shadowcolor0
	// R8 attachments the depth copies render into with no writes: gogpu
	// rejects a fragment stage without a colour target.
	dummy, dummyShadow packTarget
	noise              texture2D

	linClp, linRep, cmp                                      *wgpu.Sampler
	g2Layout, emptyLayout, compLayout, mipLayout, copyLayout *wgpu.BindGroupLayout
	g2, g2Shadow, empty                                      *wgpu.BindGroup
	comp                                                     [6]*wgpu.BindGroup
	mipB4, mipB7a, mipB7b                                    *wgpu.BindGroup
	copyDepth, copyShadow                                    *wgpu.BindGroup

	modules                       map[string]*wgpu.ShaderModule
	variants                      map[packVariant]*wgpu.RenderPipeline
	comps                         [6]*wgpu.RenderPipeline
	mip, copy, shadowOp, shadowTr *wgpu.RenderPipeline

	// this frame
	r           *Renderer
	hand, ended bool
	vp          mgl32.Mat4
	fog         Fog
	fogCol      mgl32.Vec3
	trans       []drawBatch // translucent terrain, drawn at worldEnd
	shadowU     ubuf
}

// pack is created on the first pack frame and kept.
var pack *packGPU

func (pk *packGPU) module(label, src string) *wgpu.ShaderModule {
	if m := pk.modules[src]; m != nil {
		return m
	}
	m, err := dev.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: label, WGSL: packStore(src, "")})
	if err != nil {
		panic(err)
	}
	pk.modules[src] = m
	return m
}

func newPackGPU() *packGPU {
	pk := &packGPU{modules: map[string]*wgpu.ShaderModule{}, variants: map[packVariant]*wgpu.RenderPipeline{}}
	sampler := func(label string, addr gputypes.AddressMode, cmp gputypes.CompareFunction) *wgpu.Sampler {
		s, err := dev.device.CreateSampler(&wgpu.SamplerDescriptor{Label: label,
			AddressModeU: addr, AddressModeV: addr, AddressModeW: addr,
			MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear, MipmapFilter: gputypes.FilterModeNearest,
			LodMaxClamp: 32, Compare: cmp})
		if err != nil {
			panic(err)
		}
		return s
	}
	pk.linClp = sampler("pack linear clamp", gputypes.AddressModeClampToEdge, 0)
	pk.linRep = sampler("pack linear repeat", gputypes.AddressModeRepeat, 0)
	pk.cmp = sampler("pack shadow compare", gputypes.AddressModeClampToEdge, gputypes.CompareFunctionLessEqual)

	vf := gputypes.ShaderStages(gputypes.ShaderStageVertex | gputypes.ShaderStageFragment)
	layout := func(label string, e ...gputypes.BindGroupLayoutEntry) *wgpu.BindGroupLayout {
		l, err := dev.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: label, Entries: e})
		if err != nil {
			panic(err)
		}
		return l
	}
	uni := gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: vf,
		Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: uint64(unsafe.Sizeof(packUniforms{}))}}
	depth := func(b uint32) gputypes.BindGroupLayoutEntry {
		return texEntry(b, vf, gputypes.TextureSampleTypeDepth, gputypes.TextureViewDimension2D)
	}
	color := func(b uint32) gputypes.BindGroupLayoutEntry {
		return texEntry(b, vf, gputypes.TextureSampleTypeFloat, gputypes.TextureViewDimension2D)
	}
	samp := func(b uint32, t gputypes.SamplerBindingType) gputypes.BindGroupLayoutEntry {
		return gputypes.BindGroupLayoutEntry{Binding: b, Visibility: vf, Sampler: &gputypes.SamplerBindingLayout{Type: t}}
	}
	pk.g2Layout = layout("pack group 2", uni, depth(1), samp(2, gputypes.SamplerBindingTypeComparison), color(3), samp(4, gputypes.SamplerBindingTypeFiltering))
	pk.emptyLayout = layout("pack empty")
	ce := []gputypes.BindGroupLayoutEntry{uni}
	for i := uint32(1); i <= 8; i++ {
		ce = append(ce, color(i))
	}
	ce = append(ce, depth(9), depth(10), depth(11), depth(12), depth(13), color(14), color(15),
		samp(16, gputypes.SamplerBindingTypeFiltering), samp(17, gputypes.SamplerBindingTypeFiltering), samp(18, gputypes.SamplerBindingTypeComparison))
	pk.compLayout = layout("pack composite", ce...)
	pk.mipLayout = layout("pack mip", color(0), samp(1, gputypes.SamplerBindingTypeFiltering))
	pk.copyLayout = layout("pack depth copy", depth(0))
	pk.empty = bindGroup("pack empty", pk.emptyLayout)

	pk.ubuf = newBuffer("pack uniforms", wgpu.BufferUsageUniform, nil, int(unsafe.Sizeof(packUniforms{})))
	pk.noise = newTexture("noisetex", 128, 128, 1, 1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2D)
	pk.noise.write(0, 0, 128, 128, 4, hfNoise(128, 128))
	pk.shadow = newPackTarget("shadowtex0", packShadowRes, packShadowRes, 1, gputypes.TextureFormatDepth32Float)
	pk.shadow1 = newPackTarget("shadowtex1", packShadowRes, packShadowRes, 1, gputypes.TextureFormatDepth32Float)
	pk.shadowCol = newPackTarget("shadowcolor0", packShadowRes, packShadowRes, 1, gputypes.TextureFormatRGBA8Unorm)
	pk.dummyShadow = newPackTarget("pack dummy", packShadowRes, packShadowRes, 1, gputypes.TextureFormatR8Unorm)
	pk.copyShadow = bindGroup("pack shadow copy", pk.copyLayout, wgpu.BindGroupEntry{Binding: 0, TextureView: pk.shadow.view})

	pk.mip = pk.pipeline("pack mip", packMipWGSL, []*wgpu.BindGroupLayout{pk.mipLayout}, nil, nil,
		[]gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA16Float, WriteMask: gputypes.ColorWriteMaskAll}}, nil)
	pk.copy = pk.pipeline("pack depth copy", packDepthCopyWGSL, []*wgpu.BindGroupLayout{pk.copyLayout}, nil, nil,
		[]gputypes.ColorTargetState{{Format: gputypes.TextureFormatR8Unorm, WriteMask: gputypes.ColorWriteMaskNone}},
		&wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: true, DepthCompare: gputypes.CompareFunctionAlways})
	f16 := gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA16Float, WriteMask: gputypes.ColorWriteMaskAll}
	for i, src := range []string{packCompositeWGSL, packComposite1WGSL, packComposite2WGSL, packComposite3WGSL, packComposite4WGSL, packFinalWGSL} {
		targets := []gputypes.ColorTargetState{f16}
		switch i {
		case 0:
			targets = append(targets, f16)
		case 5:
			targets[0].Format = dev.format
		}
		if i > 0 && i < 5 {
			src = packStore(src, "storeR11") // packStore is idempotent: module's call finds fs_raw only
		}
		pk.comps[i] = pk.pipeline("pack composite", src, []*wgpu.BindGroupLayout{pk.compLayout}, nil, nil, targets, nil)
	}
	shadowTargets := func(blend *gputypes.BlendState) []gputypes.ColorTargetState {
		return []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, Blend: blend, WriteMask: gputypes.ColorWriteMaskAll}}
	}
	shadowDepth := &wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: true, DepthCompare: gputypes.CompareFunctionLessEqual}
	sp := dev.pipes.specs[dev.pipes.terrainOpaque]
	groups := []*wgpu.BindGroupLayout{dev.uniLayout, dev.pipes.terrainTex, pk.g2Layout}
	pk.shadowOp = pk.pipeline("pack shadow", packShadowWGSL, groups, sp.buffers, nil, shadowTargets(nil), shadowDepth)
	pk.shadowTr = pk.pipeline("pack shadow translucent", packShadowWGSL, groups, sp.buffers, nil, shadowTargets(blendAlpha), shadowDepth)
	return pk
}

// pipeline creates a render pipeline (cull none, triangles unless prim).
func (pk *packGPU) pipeline(label, src string, groups []*wgpu.BindGroupLayout, bufs []gputypes.VertexBufferLayout,
	prim *gputypes.PrimitiveState, targets []gputypes.ColorTargetState, depth *wgpu.DepthStencilState) *wgpu.RenderPipeline {
	mod := pk.module(label, src)
	pl, err := dev.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: label, BindGroupLayouts: groups})
	if err != nil {
		panic(err)
	}
	ps := gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, FrontFace: gputypes.FrontFaceCCW, CullMode: gputypes.CullModeNone}
	if prim != nil {
		ps = *prim
	}
	p, err := dev.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: label, Layout: pl,
		Vertex:       wgpu.VertexState{Module: mod, EntryPoint: "vs", Buffers: bufs},
		Primitive:    ps,
		DepthStencil: depth,
		Multisample:  gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		Fragment:     &wgpu.FragmentState{Module: mod, EntryPoint: "fs", Targets: targets},
	})
	if err != nil {
		panic(err)
	}
	return p
}

// resize (re)creates the screen-sized targets and their bind groups.
func (pk *packGPU) resize(w, h int) {
	if w == pk.w && h == pk.h {
		return
	}
	for i := range 8 {
		pk.a[i].release()
		pk.b[i].release()
	}
	for i := range pk.depth {
		pk.depth[i].release()
	}
	for i := range 8 {
		pk.a[i] = newPackTarget("pack target", w, h, packMips[i], packFormats[i])
		pk.b[i] = newPackTarget("pack target", w, h, packMips[i], packFormats[i])
	}
	for i := range pk.depth {
		pk.depth[i] = newPackTarget("pack depth", w, h, 1, gputypes.TextureFormatDepth32Float)
	}
	pk.dummy.release()
	pk.dummy = newPackTarget("pack dummy", w, h, 1, gputypes.TextureFormatR8Unorm)
	pk.w, pk.h = w, h
	ent := func(b uint32, v *wgpu.TextureView) wgpu.BindGroupEntry {
		return wgpu.BindGroupEntry{Binding: b, TextureView: v}
	}
	sam := func(b uint32, s *wgpu.Sampler) wgpu.BindGroupEntry {
		return wgpu.BindGroupEntry{Binding: b, Sampler: s}
	}
	uni := wgpu.BindGroupEntry{Binding: 0, Buffer: pk.ubuf, Size: uint64(unsafe.Sizeof(packUniforms{}))}
	pk.g2 = bindGroup("pack group 2", pk.g2Layout, uni, ent(1, pk.shadow.view), sam(2, pk.cmp), ent(3, pk.noise.view), sam(4, pk.linRep))
	// The shadow pass draws into shadowtex0: bind a depth texture it
	// doesn't write instead (the shadow program never samples it).
	pk.g2Shadow = bindGroup("pack group 2 (shadow)", pk.g2Layout, uni, ent(1, pk.depth[1].view), sam(2, pk.cmp), ent(3, pk.noise.view), sam(4, pk.linRep))
	// colortex sides per pass (OptiFine's flip): b where the pass reads
	// what an earlier pass wrote to the alternate buffer, a otherwise.
	readB := [6][]int{{}, {3, 4}, {}, {7}, {6, 7}, {7}}
	for p := range pk.comp {
		e := []wgpu.BindGroupEntry{uni}
		for i := range 8 {
			v := pk.a[i].view
			for _, j := range readB[p] {
				if j == i {
					v = pk.b[i].view
				}
			}
			e = append(e, ent(uint32(1+i), v))
		}
		e = append(e, ent(9, pk.depth[0].view), ent(10, pk.depth[1].view), ent(11, pk.depth[2].view),
			ent(12, pk.shadow.view), ent(13, pk.shadow1.view), ent(14, pk.shadowCol.view), ent(15, pk.noise.view),
			sam(16, pk.linClp), sam(17, pk.linRep), sam(18, pk.cmp))
		pk.comp[p] = bindGroup("pack composite", pk.compLayout, e...)
	}
	mipBG := func(v *wgpu.TextureView) *wgpu.BindGroup {
		return bindGroup("pack mip", pk.mipLayout, ent(0, v), sam(1, pk.linClp))
	}
	pk.mipB4, pk.mipB7a, pk.mipB7b = mipBG(pk.b[4].mips[0]), mipBG(pk.b[7].mips[0]), mipBG(pk.b[7].mips[1])
	pk.copyDepth = bindGroup("pack depth copy", pk.copyLayout, ent(0, pk.depth[0].view))
}

// hfNoise is OptiFine's HFNoiseTexture: w x h RGB bytes (here RGBA).
func hfNoise(w, h int) []byte {
	rnd := func(x int32) int32 {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		return x
	}
	px := make([]byte, 0, w*h*4)
	for y := range int32(h) {
		for x := range int32(w) {
			for z := int32(1); z < 4; z++ {
				b := (rnd(x)+rnd(y*19))*rnd(z*23) - z
				px = append(px, byte(int8(rnd(b)%128)))
			}
			px = append(px, 255)
		}
	}
	return px
}

const packDepthCopyWGSL = `
@group(0) @binding(0) var src: texture_depth_2d;
@vertex
fn vs(@builtin(vertex_index) id: u32) -> @builtin(position) vec4f {
	let p = vec2f(f32((id << 1u) & 2u), f32(id & 2u));
	return vec4f(p * 2.0 - 1.0, 0.0, 1.0);
}
@fragment
fn fs(@builtin(position) p: vec4f) -> @builtin(frag_depth) f32 { return textureLoad(src, vec2i(p.xy), 0); }
`

// beginPass starts a render pass on dev.enc.
func beginPass(label string, color []wgpu.RenderPassColorAttachment, depth *wgpu.RenderPassDepthStencilAttachment) *wgpu.RenderPassEncoder {
	p, err := dev.enc.BeginRenderPass(&wgpu.RenderPassDescriptor{Label: label, ColorAttachments: color, DepthStencilAttachment: depth})
	if err != nil {
		panic(err)
	}
	return p
}

func endPassOf(p *wgpu.RenderPassEncoder) {
	if err := p.End(); err != nil {
		panic(err)
	}
}

// copyDepthTo writes src (bound by bg) into dst with a fullscreen pass:
// gogpu's tracker rejects a texture copy from an attachment of the same
// encoder (see ReadFrame).
func (pk *packGPU) copyDepthTo(bg *wgpu.BindGroup, dst, dummy *wgpu.TextureView) {
	p := beginPass("pack depth copy", []wgpu.RenderPassColorAttachment{{View: dummy,
		LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpDiscard}}, &wgpu.RenderPassDepthStencilAttachment{View: dst,
		DepthLoadOp: gputypes.LoadOpClear, DepthClearValue: 1, DepthStoreOp: gputypes.StoreOpStore})
	p.SetPipeline(pk.copy)
	p.SetBindGroup(0, bg, nil)
	dev.draws++
	p.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	endPassOf(p)
}

// gbufferPass opens the gbuffer pass: cleared at the frame's start
// (gcolor to the fog colour, gdepth to 1, gaux4 never), loaded after.
func (pk *packGPU) gbufferPass(clear bool) *wgpu.RenderPassEncoder {
	var col [6]wgpu.RenderPassColorAttachment
	for i, t := range packGbuffer {
		col[i] = wgpu.RenderPassColorAttachment{View: pk.a[t].mips[0], LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
		if clear && t != 7 {
			col[i].LoadOp = gputypes.LoadOpClear
		}
	}
	fc := pk.u.FogColor
	col[0].ClearValue = gputypes.Color{R: float64(fc[0]), G: float64(fc[1]), B: float64(fc[2]), A: 1}
	col[1].ClearValue = gputypes.Color{R: 1, G: 1, B: 1, A: 1}
	depth := &wgpu.RenderPassDepthStencilAttachment{View: pk.depth[0].view, DepthLoadOp: gputypes.LoadOpLoad, DepthStoreOp: gputypes.StoreOpStore}
	if clear {
		depth.DepthLoadOp, depth.DepthClearValue = gputypes.LoadOpClear, 1
	}
	return beginPass("pack gbuffers", col[:], depth)
}

// packFrame starts a pack frame: the uniforms, the shadow pass, and the
// gbuffer pass that the world's draws record into.
func (r *Renderer) packFrame(cam *Camera) {
	if pack == nil {
		pack = newPackGPU()
	}
	pk := pack
	pk.resize(dev.w, dev.h)
	endPass()
	now := time.Now()
	dt := int64(0)
	if !pk.last.IsZero() {
		dt = now.Sub(pk.last).Milliseconds()
	}
	pk.last = now
	f := *r.Opt.Pack
	if f.Far == 0 {
		f.Far = float32(r.Opt.RenderDistance * 16)
	}
	view := mgl32.LookAtV(mgl32.Vec3{}, cam.Forward(), mgl32.Vec3{0, 1, 0})
	if cam.Bob != (mgl32.Mat4{}) {
		view = cam.Bob.Mul4(view)
	}
	proj := mgl32.Perspective(float32(cam.FOV), float32(cam.Aspect), float32(cam.Near), float32(cam.Far))
	pk.u = pk.st.update(&f, [3]float64{cam.X, cam.Y, cam.Z}, view, proj, dev.w, dev.h, dt)
	_, fogCol := r.skyColors()
	pk.u.FogColor = fogCol.Vec4(1)
	writeBuffer(pk.ubuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&pk.u)), unsafe.Sizeof(pk.u)))

	pk.r, pk.hand, pk.ended, pk.trans = r, false, false, nil
	pk.shadowPass(r)
	dev.pass = pk.gbufferPass(true)
	dev.pk = pk
}

// shadowPass draws the terrain within the shadow frustum into shadowtex0
// and shadowcolor0: opaque, then (after the copy to shadowtex1) the
// translucent layer, water white (shadow.fsh).
func (pk *packGPU) shadowPass(r *Renderer) {
	fr := makeFrustum(pk.u.ShadowProj.Mul4(pk.u.ShadowMV))
	bg := r.terrainBG()
	for _, trans := range []bool{false, true} {
		color := []wgpu.RenderPassColorAttachment{{View: pk.shadowCol.view, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}}
		depth := &wgpu.RenderPassDepthStencilAttachment{View: pk.shadow.view, DepthLoadOp: gputypes.LoadOpLoad, DepthStoreOp: gputypes.StoreOpStore}
		pl := pk.shadowTr
		if !trans {
			color[0].LoadOp, color[0].ClearValue = gputypes.LoadOpClear, gputypes.Color{R: 1, G: 1, B: 1, A: 1}
			depth.DepthLoadOp, depth.DepthClearValue = gputypes.LoadOpClear, 1
			pl = pk.shadowOp
		}
		p := beginPass("pack shadow", color, depth)
		p.SetPipeline(pl)
		p.SetBindGroup(1, bg, nil)
		p.SetBindGroup(2, pk.g2Shadow, nil)
		p.SetIndexBuffer(r.gpu.ebo, gputypes.IndexFormatUint32, 0)
		for _, pages := range r.regions {
			for _, reg := range pages {
				first := len(r.counts)
				for _, s := range reg.sections {
					n, off := s.n, s.off
					if trans {
						n, off = s.transN, s.tOff
					}
					if n == 0 || s.empty {
						continue
					}
					o := s.pos.Origin()
					x, y, z := float32(float64(o.X)-r.camX), float32(float64(o.Y)-r.camY), float32(float64(o.Z)-r.camZ)
					if !fr.box(x, y, z, x+16, y+16, z+16) {
						continue
					}
					for q := 0; q < n/4; q += maxQuadsPerDraw {
						r.counts = append(r.counts, int32(min(n/4-q, maxQuadsPerDraw)*6))
						r.bases = append(r.bases, int32(off+q*4))
					}
				}
				if len(r.counts) == first {
					continue
				}
				ro := r.regionOrigin(reg)
				bindUniforms(p, pk.shadowU.reset().mat(mgl32.Ident4()).f(ro[0], ro[1], ro[2], 0).f(0, 0, 0, 0).f(0, 0, 0, 0).f(0, 0, 0, 0).f(0, 0, 0, 0))
				p.SetVertexBuffer(0, reg.gpu.vbo, 0)
				r.multiDraw(p, drawBatch{reg, voxel.LayerSolid, first, len(r.counts) - first})
			}
		}
		endPassOf(p)
		if !trans {
			pk.copyDepthTo(pk.copyShadow, pk.shadow1.view, pk.dummyShadow.view)
		}
	}
}

// setPipe sets pl, or in a pack frame the pack program that replaces it
// (with group 2, and an empty group 1 where pl has none).
func setPipe(p *wgpu.RenderPassEncoder, pl *wgpu.RenderPipeline) {
	if dev.pk == nil {
		p.SetPipeline(pl)
		return
	}
	pk := dev.pk
	sp := dev.pipes.specs[pl]
	src, outs, hand := pk.program(pl)
	if src == "" { // not a world program
		p.SetPipeline(pl)
		return
	}
	key := packVariant{pl, hand && pk.hand}
	v := pk.variants[key]
	if v == nil {
		if hand {
			flag := "false"
			if key.hand {
				flag = "true"
			}
			src = replaceOnce(src, "HAND_FLAG", flag)
		}
		g1 := sp.group1
		if g1 == nil {
			g1 = pk.emptyLayout
		}
		var targets [6]gputypes.ColorTargetState
		for i, t := range packGbuffer {
			targets[i] = gputypes.ColorTargetState{Format: packFormats[t], WriteMask: gputypes.ColorWriteMaskNone}
		}
		for _, o := range outs {
			targets[o].WriteMask, targets[o].Blend = gputypes.ColorWriteMaskAll, sp.blend
		}
		depth := &wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: sp.depthWrite,
			DepthCompare: gputypes.CompareFunctionLessEqual}
		if !sp.depthTest {
			depth.DepthCompare = gputypes.CompareFunctionAlways
		}
		if pl == dev.pipes.terrainTrans { // gbuffers_water writes depth (depthtex0 has it, depthtex1 not)
			depth.DepthWriteEnabled = true
		}
		if pl == dev.pipes.blockDamage {
			depth.DepthBias, depth.DepthBiasSlopeScale = -3, -3
		}
		v = pk.pipeline("pack "+sp.label, src, []*wgpu.BindGroupLayout{dev.uniLayout, g1, pk.g2Layout}, sp.buffers,
			&gputypes.PrimitiveState{Topology: sp.topology, FrontFace: gputypes.FrontFaceCCW, CullMode: sp.cull}, targets[:], depth)
		pk.variants[key] = v
	}
	p.SetPipeline(v)
	if sp.group1 == nil {
		p.SetBindGroup(1, pk.empty, nil)
	}
	p.SetBindGroup(2, pk.g2, nil)
}

func replaceOnce(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

// program is the pack program that replaces an engine pipeline: its
// source, the gbuffer outputs it writes (packGbuffer indices) and whether
// it has a HAND_FLAG variant. OptiFine's fallbacks: entities, hand,
// particles and held blocks use gbuffers_textured, the breaking overlay
// gbuffers_terrain, the outline gbuffers_basic, rain and snow
// gbuffers_weather (the only sky list draws in a pack frame).
func (pk *packGPU) program(pl *wgpu.RenderPipeline) (string, []int, bool) {
	ps := &dev.pipes
	switch pl {
	case ps.terrainOpaque:
		return packTerrainWGSL, []int{0, 1}, false
	case ps.terrainTrans:
		return packWaterWGSL, []int{2, 3, 4}, false
	case ps.model, ps.modelAlpha, ps.modelAdd:
		return packModelWGSL, []int{3, 4}, true
	case ps.item:
		return packItemWGSL, []int{3, 4}, true
	case ps.blockItem:
		return packBlockWGSL, []int{3, 4}, true
	case ps.particleAlpha, ps.particleAdd:
		return packParticleWGSL, []int{3, 4}, false
	case ps.blockDamage:
		return packDamageWGSL, []int{0, 1}, false
	case ps.outline:
		return packBasicWGSL, []int{0}, false
	case ps.skyAlpha, ps.skyAdd:
		return packWeatherWGSL, []int{5}, false
	}
	return "", nil, false
}

// packHand is clearDepth in a pack frame: the world ends and later draws
// use the hand programs. The depth buffer is kept (the composites read
// it); the hand programs put their depth in front of the world's.
func packHand() {
	pk := dev.pk
	if !pk.ended {
		pk.worldEnd()
	}
	pk.hand = true
}

// worldEnd copies depthtex1, draws the translucent terrain with
// gbuffers_water, copies depthtex2 and reopens the gbuffer pass.
func (pk *packGPU) worldEnd() {
	endPass()
	pk.copyDepthTo(pk.copyDepth, pk.depth[1].view, pk.dummy.view)
	if len(pk.trans) > 0 {
		dev.pass = pk.gbufferPass(false)
		pk.r.drawBatches(dev.pass, pk.vp, pk.fog, pk.fogCol, pk.trans)
		endPass()
	}
	pk.copyDepthTo(pk.copyDepth, pk.depth[2].view, pk.dummy.view)
	dev.pass = pk.gbufferPass(false)
	pk.ended = true
}

// packResolve ends a pack frame: the composite passes and final into the
// frame's target. A no-op outside a pack frame.
func packResolve() {
	pk := dev.pk
	if pk == nil {
		return
	}
	if !pk.ended {
		pk.worldEnd()
	}
	endPass()
	dev.pk = nil
	full := func(p *wgpu.RenderPassEncoder, pl *wgpu.RenderPipeline, bg *wgpu.BindGroup) {
		p.SetPipeline(pl)
		p.SetBindGroup(0, bg, nil)
		dev.draws++
		p.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		endPassOf(p)
	}
	att := func(vs ...*wgpu.TextureView) []wgpu.RenderPassColorAttachment {
		var c []wgpu.RenderPassColorAttachment
		for _, v := range vs {
			c = append(c, wgpu.RenderPassColorAttachment{View: v, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore})
		}
		return c
	}
	mip := func(bg *wgpu.BindGroup, dst *wgpu.TextureView) {
		full(beginPass("pack mip", att(dst), nil), pk.mip, bg)
	}
	w, h := uint32(pk.w), uint32(pk.h)
	// The bloom passes only fill what final reads: colortex6 at tc*0.25,
	// the lower-left quarter (GL), plus the blur reach of composite4.
	quarter := func(p *wgpu.RenderPassEncoder, pad uint32) *wgpu.RenderPassEncoder {
		qw, qh := min(w/4+pad, w), min(h/4+pad, h)
		p.SetScissorRect(gputypes.ScissorRect{X: 0, Y: h - qh, Width: qw, Height: qh})
		return p
	}
	full(beginPass("composite", att(pk.b[3].mips[0], pk.b[4].mips[0]), nil), pk.comps[0], pk.comp[0])
	mip(pk.mipB4, pk.b[4].mips[1])
	full(beginPass("composite1", att(pk.a[3].mips[0]), nil), pk.comps[1], pk.comp[1])
	full(beginPass("composite2", att(pk.b[7].mips[0]), nil), pk.comps[2], pk.comp[2])
	mip(pk.mipB7a, pk.b[7].mips[1])
	mip(pk.mipB7b, pk.b[7].mips[2])
	full(quarter(beginPass("composite3", att(pk.b[6].mips[0]), nil), 16), pk.comps[3], pk.comp[3])
	full(quarter(beginPass("composite4", att(pk.a[6].mips[0]), nil), 4), pk.comps[4], pk.comp[4])
	target := dev.target
	if dev.capturing {
		target = dev.capTex.view
	}
	full(beginPass("final", att(target), nil), pk.comps[5], pk.comp[5])
	dev.clearNext, dev.clearDepth = false, true
}
