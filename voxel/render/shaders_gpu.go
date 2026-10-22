package render

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The WGSL ports of the GLSL shaders in shaders_gl.go and the *_gl.go files.
// Uniform blocks hold only mat4x4f and vec4f members (see ubuf).

const wgslLight = `
// Minecraft's lightmap brightness curve with a small ambient floor.
fn curve(l: f32) -> f32 { return mix(0.04, 1.0, l / (4.0 - 3.0 * l)); }
// blockCurve is curve for block light, raised toward Minecraft's bright
// gamma (1-(1-l)^4) by bright.
fn blockCurve(l: f32, bright: f32) -> f32 {
	return mix(curve(l), mix(0.04, 1.0, 1.0 - pow(1.0 - l, 4.0)), bright);
}
fn lightColor(sky: f32, blk: f32, daylight: f32, bright: f32) -> vec3f {
	let s = curve(clamp(sky, 0.0, 1.0)) * daylight;
	let b = blockCurve(clamp(blk, 0.0, 1.0), bright);
	return max(max(vec3f(s) * vec3f(0.95, 0.97, 1.0), vec3f(b) * vec3f(1.0, 0.86, 0.66)), vec3f(0.04));
}
fn fogFactor(dist: f32, fog: vec4f) -> f32 {
	let f = clamp((dist - fog.x) / (fog.y - fog.x), 0.0, 1.0);
	return f * f;
}
`

const terrainWGSL = `
struct U {
	viewProj: mat4x4f, // rotation-only view * projection (camera-relative)
	origin: vec4f,     // region origin minus camera position
	fogColor: vec4f,
	params: vec4f,     // daylight, block brightness, alpha cut
	fog: vec4f,        // start, end
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
@group(1) @binding(2) var layerMap: texture_2d<u32>; // base layer -> current animation frame layer

struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) @interpolate(flat) layer: u32,
	@location(2) color: vec3f,
	@location(3) light: vec2f,
	@location(4) dist: f32,
}
` + wgslLight + `
@vertex
fn vs(@location(0) d: vec4<u32>) -> VOut {
	var shade = array<f32, 7>(0.5, 1.0, 0.8, 0.8, 0.6, 0.6, 1.0);
	var ao = array<f32, 4>(0.45, 0.65, 0.82, 1.0);
	let w0 = d.x; let w1 = d.y; let w2 = d.z; let w3 = d.w;
	let local = vec3f(f32(w0 & 1023u), f32((w0 >> 10u) & 1023u), f32((w0 >> 20u) & 1023u)) * (1.0 / 32.0);
	let si = w2 >> 24u;
	let sec = vec3f(f32(si & 7u), f32((si >> 3u) & 3u), f32((si >> 5u) & 7u)) * 16.0;
	let pos = u.origin.xyz + sec + local;
	var o: VOut;
	o.pos = u.viewProj * vec4f(pos, 1.0);
	o.layer = textureLoad(layerMap, vec2<i32>(i32(w1 & 4095u), 0), 0).r;
	o.uv = vec2f(f32((w1 >> 12u) & 31u), f32((w1 >> 17u) & 31u));
	if ((w1 & (1u << 25u)) != 0u) { o.uv = o.uv * (1.0 / 16.0); } // box faces: u/v in 1/16 tile
	let n = (w1 >> 22u) & 7u;
	let tint = vec3f(f32(w2 & 255u), f32((w2 >> 8u) & 255u), f32((w2 >> 16u) & 255u)) * (1.0 / 255.0);
	o.color = tint * shade[n] * ao[w0 >> 30u];
	o.light = vec2f(f32(w3 & 255u), f32((w3 >> 8u) & 255u)) * (1.0 / 240.0);
	o.dist = length(pos.xz); // cylindrical fog like Minecraft
	return o;
}

@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let c = textureSample(tex, samp, i.uv, i.layer);
	if (c.a < u.params.z) { discard; }
	let col = c.rgb * i.color * lightColor(i.light.x, i.light.y, u.params.x, u.params.y);
	return vec4f(mix(col, u.fogColor.rgb, fogFactor(i.dist, u.fog)), c.a);
}
`

const skyWGSL = `
struct U {
	invViewProj: mat4x4f,
	fogColor: vec4f,
	skyColor: vec4f,
	sunDir: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
struct VOut { @builtin(position) pos: vec4f, @location(0) ndc: vec2f }
@vertex
fn vs(@builtin(vertex_index) id: u32) -> VOut {
	let p = vec2f(f32((id << 1u) & 2u), f32(id & 2u)) * 2.0 - 1.0;
	var o: VOut;
	o.ndc = p;
	o.pos = vec4f(p, 0.9999, 1.0);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let p = u.invViewProj * vec4f(i.ndc, 1.0, 1.0);
	let dir = normalize(p.xyz / p.w);
	let h = clamp(dir.y, -0.2, 1.0);
	var col = mix(u.fogColor.rgb, u.skyColor.rgb, smoothstep(0.0, 0.45, h));
	let sun = max(dot(dir, u.sunDir.xyz), 0.0);
	col += vec3f(1.0, 0.95, 0.8) * step(0.9985, sun);
	col += vec3f(1.0, 0.8, 0.5) * pow(sun, 64.0) * 0.25;
	return vec4f(col, 1.0);
}
`

const lineWGSL = `
struct U { mvp: mat4x4f, color: vec4f }
@group(0) @binding(0) var<uniform> u: U;
@vertex
fn vs(@location(0) p: vec3f) -> @builtin(position) vec4f { return u.mvp * vec4f(p, 1.0); }
@fragment
fn fs() -> @location(0) vec4f { return u.color; }
`

const modelWGSL = `
struct U {
	viewProj: mat4x4f,
	model: mat4x4f,
	light: vec4f,    // sky, block, daylight, block brightness
	fogColor: vec4f, // rgb, fade
	fog: vec4f,
	bones: array<mat4x4f, 96>,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) shade: f32,
	@location(2) dist: f32,
}
` + wgslLight + `
fn m3(m: mat4x4f) -> mat3x3f { return mat3x3f(m[0].xyz, m[1].xyz, m[2].xyz); }
@vertex
fn vs(@location(0) pos: vec3f, @location(1) normal: vec3f, @location(2) uv: vec2f,
	@location(3) j: vec4<u32>, @location(4) w: vec4f) -> VOut {
	let skin = u.bones[j.x] * w.x + u.bones[j.y] * w.y + u.bones[j.z] * w.z + u.bones[j.w] * w.w;
	let wp = u.model * skin * vec4f(pos, 1.0);
	var o: VOut;
	o.pos = u.viewProj * wp;
	let n = normalize(m3(u.model) * m3(skin) * normal);
	// Minecraft entity lighting: two fixed lights, ambient 0.4.
	let l0 = normalize(vec3f(0.2, 1.0, -0.7));
	let l1 = normalize(vec3f(-0.2, 1.0, 0.7));
	o.shade = min(1.0, 0.4 + 0.6 * (max(dot(n, l0), 0.0) + max(dot(n, l1), 0.0)) * 0.6 + 0.25);
	o.uv = uv;
	o.dist = length(wp.xz);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let c = textureSample(tex, samp, i.uv);
	if (c.a < 0.5) { discard; }
	// Interleaved gradient noise: a stable screen-space dither for fading.
	let fade = u.fogColor.a;
	if (fade > 0.0 && fract(52.9829189 * fract(dot(i.pos.xy, vec2f(0.06711056, 0.00583715)))) < fade) { discard; }
	let col = c.rgb * i.shade * lightColor(u.light.x, u.light.y, u.light.z, u.light.w);
	return vec4f(mix(col, u.fogColor.rgb, fogFactor(i.dist, u.fog)), 1.0);
}
`

const itemWGSL = `
struct U {
	mvp: mat4x4f,
	model: mat4x4f,  // model rotation+scale (camera-relative world space)
	light: vec4f,    // sky, block, daylight, block brightness
	fogColor: vec4f, // rgb, layer
	fog: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) shade: f32,
	@location(2) dist: f32,
}
` + wgslLight + `
@vertex
fn vs(@location(0) pos: vec3f, @location(1) uv: vec2f, @location(2) normal: vec3f) -> VOut {
	let wp = u.model * vec4f(pos, 1.0);
	var o: VOut;
	o.pos = u.mvp * vec4f(pos, 1.0);
	o.uv = uv;
	let n = normalize(mat3x3f(u.model[0].xyz, u.model[1].xyz, u.model[2].xyz) * normal);
	// Minecraft-like fixed directional shading: top bright, bottom dark.
	o.shade = 0.7 + 0.3 * n.y + 0.1 * abs(n.x);
	o.dist = length(wp.xz);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let c = textureSample(tex, samp, i.uv, i32(u.fogColor.a));
	if (c.a < 0.5) { discard; }
	let col = c.rgb * i.shade * lightColor(u.light.x, u.light.y, u.light.z, u.light.w);
	return vec4f(mix(col, u.fogColor.rgb, fogFactor(i.dist, u.fog)), 1.0);
}
`

const particleWGSL = `
struct U {
	viewProj: mat4x4f,
	right: vec4f,
	up: vec4f,
	params: vec4f,   // daylight, block brightness, additive
	fogColor: vec4f,
	fog: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) color: vec4f,
	@location(1) light: vec2f,
	@location(2) dist: f32,
}
` + wgslLight + `
@vertex
fn vs(@location(0) corner: vec2f, @location(1) ipos: vec3f, @location(2) size: f32,
	@location(3) color: vec4f, @location(4) light: vec2f) -> VOut {
	let p = ipos + (u.right.xyz * corner.x + u.up.xyz * corner.y) * size;
	var o: VOut;
	o.pos = u.viewProj * vec4f(p, 1.0);
	o.color = color;
	o.light = light;
	o.dist = length(p);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let f = fogFactor(i.dist, u.fog);
	if (u.params.z > 0.5) {
		return vec4f(i.color.rgb * i.color.a * (1.0 - f), 1.0);
	}
	let light = lightColor(i.light.x, i.light.y, u.params.x, u.params.y);
	return vec4f(mix(i.color.rgb * light, u.fogColor.rgb, f), i.color.a);
}
`

const overlayWGSL = `
struct U { rect: vec4f } // x0, y0, x1, y1 in NDC
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
struct VOut { @builtin(position) pos: vec4f, @location(0) uv: vec2f }
@vertex
fn vs(@builtin(vertex_index) id: u32) -> VOut {
	let c = vec2f(f32(id & 1u), f32((id >> 1u) & 1u));
	var o: VOut;
	o.uv = vec2f(c.x, 1.0 - c.y);
	o.pos = vec4f(mix(u.rect.xy, u.rect.zw, c), 0.0, 1.0);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f { return textureSample(tex, samp, i.uv); }
`

const blitWGSL = `
@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;
struct VOut { @builtin(position) pos: vec4f, @location(0) uv: vec2f }
@vertex
fn vs(@builtin(vertex_index) id: u32) -> VOut {
	let p = vec2f(f32((id << 1u) & 2u), f32(id & 2u));
	var o: VOut;
	o.uv = vec2f(p.x, 1.0 - p.y);
	o.pos = vec4f(p * 2.0 - 1.0, 0.0, 1.0);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f { return textureSample(tex, samp, i.uv); }
`

// pipelines holds every render pipeline; they are created once in Init.
type pipelines struct {
	terrainTex, tex2D, texArray *wgpu.BindGroupLayout // group 1 layouts

	terrainOpaque, terrainTrans *wgpu.RenderPipeline
	sky                         *wgpu.RenderPipeline
	outline, crosshair          *wgpu.RenderPipeline
	model, item                 *wgpu.RenderPipeline
	particleAlpha, particleAdd  *wgpu.RenderPipeline
	overlay                     *wgpu.RenderPipeline
	blit                        *wgpu.RenderPipeline
}

var (
	blendAlpha = &gputypes.BlendState{
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorSrcAlpha, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorSrcAlpha, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha, Operation: gputypes.BlendOperationAdd},
	}
	blendAdd = &gputypes.BlendState{
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
	}
	blendInvert = &gputypes.BlendState{
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOneMinusDst, DstFactor: gputypes.BlendFactorZero, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOneMinusDst, DstFactor: gputypes.BlendFactorZero, Operation: gputypes.BlendOperationAdd},
	}
)

// pipeSpec describes one pipeline.
type pipeSpec struct {
	label      string
	wgsl       string
	group1     *wgpu.BindGroupLayout
	buffers    []gputypes.VertexBufferLayout
	topology   gputypes.PrimitiveTopology
	cull       gputypes.CullMode
	blend      *gputypes.BlendState
	depthTest  bool
	depthWrite bool
	blit       bool // group 0 is a texture, no depth buffer
}

func texEntry(binding uint32, vis gputypes.ShaderStages, sample gputypes.TextureSampleType, dim gputypes.TextureViewDimension) gputypes.BindGroupLayoutEntry {
	return gputypes.BindGroupLayoutEntry{Binding: binding, Visibility: vis, Texture: &gputypes.TextureBindingLayout{SampleType: sample, ViewDimension: dim}}
}

func samplerEntry(binding uint32) gputypes.BindGroupLayoutEntry {
	return gputypes.BindGroupLayoutEntry{Binding: binding, Visibility: gputypes.ShaderStageFragment, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}}
}

func (ps *pipelines) init() error {
	var err error
	layout := func(label string, entries ...gputypes.BindGroupLayoutEntry) *wgpu.BindGroupLayout {
		if err != nil {
			return nil
		}
		var l *wgpu.BindGroupLayout
		l, err = dev.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: label, Entries: entries})
		return l
	}
	frag := gputypes.ShaderStages(gputypes.ShaderStageFragment)
	ps.terrainTex = layout("terrain textures",
		texEntry(0, frag, gputypes.TextureSampleTypeFloat, gputypes.TextureViewDimension2DArray), samplerEntry(1),
		texEntry(2, gputypes.ShaderStageVertex, gputypes.TextureSampleTypeUint, gputypes.TextureViewDimension2D))
	ps.tex2D = layout("texture", texEntry(0, frag, gputypes.TextureSampleTypeFloat, gputypes.TextureViewDimension2D), samplerEntry(1))
	ps.texArray = layout("texture array", texEntry(0, frag, gputypes.TextureSampleTypeFloat, gputypes.TextureViewDimension2DArray), samplerEntry(1))
	if err != nil {
		return err
	}

	attr := func(loc uint32, f gputypes.VertexFormat, off uint64) gputypes.VertexAttribute {
		return gputypes.VertexAttribute{Format: f, Offset: off, ShaderLocation: loc}
	}
	terrainBuf := []gputypes.VertexBufferLayout{{ArrayStride: vertexBytes, StepMode: gputypes.VertexStepModeVertex,
		Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatUint32x4, 0)}}}
	lineBuf := []gputypes.VertexBufferLayout{{ArrayStride: 12, StepMode: gputypes.VertexStepModeVertex,
		Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x3, 0)}}}
	modelBuf := []gputypes.VertexBufferLayout{{ArrayStride: modelStride * 4, StepMode: gputypes.VertexStepModeVertex,
		Attributes: []gputypes.VertexAttribute{
			attr(0, gputypes.VertexFormatFloat32x3, 0), attr(1, gputypes.VertexFormatFloat32x3, 12), attr(2, gputypes.VertexFormatFloat32x2, 24),
			attr(3, gputypes.VertexFormatUint8x4, 32), attr(4, gputypes.VertexFormatFloat32x4, 36)}}}
	itemBuf := []gputypes.VertexBufferLayout{{ArrayStride: 32, StepMode: gputypes.VertexStepModeVertex,
		Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x3, 0), attr(1, gputypes.VertexFormatFloat32x2, 12), attr(2, gputypes.VertexFormatFloat32x3, 20)}}}
	particleBuf := []gputypes.VertexBufferLayout{
		{ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x2, 0)}},
		{ArrayStride: particleStride * 4, StepMode: gputypes.VertexStepModeInstance, Attributes: []gputypes.VertexAttribute{
			attr(1, gputypes.VertexFormatFloat32x3, 0), attr(2, gputypes.VertexFormatFloat32, 12),
			attr(3, gputypes.VertexFormatFloat32x4, 16), attr(4, gputypes.VertexFormatFloat32x2, 32)}},
	}

	tri, lines := gputypes.PrimitiveTopologyTriangleList, gputypes.PrimitiveTopologyLineList
	back, none := gputypes.CullModeBack, gputypes.CullModeNone
	specs := []struct {
		dst  **wgpu.RenderPipeline
		spec pipeSpec
	}{
		{&ps.terrainOpaque, pipeSpec{"terrain", terrainWGSL, ps.terrainTex, terrainBuf, tri, back, nil, true, true, false}},
		{&ps.terrainTrans, pipeSpec{"terrain translucent", terrainWGSL, ps.terrainTex, terrainBuf, tri, back, blendAlpha, true, false, false}},
		{&ps.sky, pipeSpec{"sky", skyWGSL, nil, nil, tri, back, nil, true, false, false}},
		{&ps.outline, pipeSpec{"outline", lineWGSL, nil, lineBuf, lines, none, blendAlpha, true, true, false}},
		{&ps.crosshair, pipeSpec{"crosshair", lineWGSL, nil, lineBuf, lines, none, blendInvert, false, false, false}},
		{&ps.model, pipeSpec{"model", modelWGSL, ps.tex2D, modelBuf, tri, none, nil, true, true, false}},
		{&ps.item, pipeSpec{"item", itemWGSL, ps.texArray, itemBuf, tri, none, nil, true, true, false}},
		{&ps.particleAlpha, pipeSpec{"particles", particleWGSL, nil, particleBuf, tri, none, blendAlpha, true, false, false}},
		{&ps.particleAdd, pipeSpec{"particles additive", particleWGSL, nil, particleBuf, tri, none, blendAdd, true, false, false}},
		{&ps.overlay, pipeSpec{"overlay", overlayWGSL, ps.tex2D, nil, gputypes.PrimitiveTopologyTriangleStrip, none, blendAlpha, false, false, false}},
		{&ps.blit, pipeSpec{"capture blit", blitWGSL, nil, nil, tri, none, nil, false, false, true}},
	}
	modules := map[string]*wgpu.ShaderModule{}
	for _, s := range specs {
		sp := s.spec
		mod := modules[sp.wgsl]
		if mod == nil {
			if mod, err = dev.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: sp.label, WGSL: sp.wgsl}); err != nil {
				return err
			}
			modules[sp.wgsl] = mod
		}
		groups := []*wgpu.BindGroupLayout{dev.uniLayout}
		if sp.group1 != nil {
			groups = append(groups, sp.group1)
		}
		if sp.blit {
			groups = []*wgpu.BindGroupLayout{ps.tex2D}
		}
		pl, err := dev.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: sp.label, BindGroupLayouts: groups})
		if err != nil {
			return err
		}
		depth := &wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: sp.depthWrite,
			DepthCompare: gputypes.CompareFunctionLessEqual}
		if !sp.depthTest {
			depth.DepthCompare = gputypes.CompareFunctionAlways
		}
		if sp.blit {
			depth = nil
		}
		if *s.dst, err = dev.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:        sp.label,
			Layout:       pl,
			Vertex:       wgpu.VertexState{Module: mod, EntryPoint: "vs", Buffers: sp.buffers},
			Primitive:    gputypes.PrimitiveState{Topology: sp.topology, FrontFace: gputypes.FrontFaceCCW, CullMode: sp.cull},
			DepthStencil: depth,
			Multisample:  gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
			Fragment: &wgpu.FragmentState{Module: mod, EntryPoint: "fs", Targets: []gputypes.ColorTargetState{
				{Format: dev.format, Blend: sp.blend, WriteMask: gputypes.ColorWriteMaskAll}}},
		}); err != nil {
			return fmt.Errorf("pipeline %s: %w", sp.label, err)
		}
	}
	return nil
}
