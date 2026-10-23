package render

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The WGSL ports of the GLSL shaders in shaders_gl.go and the *_gl.go files.
// Uniform blocks hold only mat4x4f and vec4f members (see ubuf).

// Light colours come from the lightmap (Options.Lightmap): sampled by
// terrain, looked up on the CPU (Renderer.LightColor) for the other passes.
const wgslLight = `
// Linear fog (GL_LINEAR) between fog.x and fog.y.
fn fogFactor(dist: f32, fog: vec4f) -> f32 {
	return clamp((dist - fog.x) / max(fog.y - fog.x, 1e-4), 0.0, 1.0);
}
`

const terrainWGSL = `
struct U {
	viewProj: mat4x4f, // rotation-only view * projection (camera-relative)
	origin: vec4f,     // region origin minus camera position
	fogColor: vec4f,
	params: vec4f,     // daylight, block brightness, alpha cut
	fog: vec4f,        // start, end
	dynTint: vec4f,    // added to the tint by the vertex's tint weight
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
@group(1) @binding(2) var layerMap: texture_2d<u32>; // base layer -> current animation frame layer
@group(1) @binding(3) var lightmap: texture_2d<f32>;  // x = block, y = sky light
@group(1) @binding(4) var lightSamp: sampler;

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
	// position: 1/128 block, biased by 8 blocks; high 10 bits in w0, low 2 in w1 (mesh.VertexWords)
	let coarse = vec3f(f32(w0 & 1023u), f32((w0 >> 10u) & 1023u), f32((w0 >> 20u) & 1023u));
	let fine = vec3f(f32((w1 >> 26u) & 3u), f32((w1 >> 28u) & 3u), f32(w1 >> 30u));
	let local = (coarse * 4.0 + fine) * (1.0 / 128.0) - 8.0;
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
	let wt = f32((w3 >> 16u) & 255u) * (1.0 / 255.0);
	o.color = (tint + wt * u.dynTint.rgb) * shade[n] * ao[w0 >> 30u];
	// Light is level*16 (0..240); Minecraft samples (l + 8) / 256.
	o.light = (vec2f(f32((w3 >> 8u) & 255u), f32(w3 & 255u)) + 8.0) * (1.0 / 256.0);
	o.dist = length(pos.xz); // cylindrical fog like Minecraft
	return o;
}

@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	let c = textureSample(tex, samp, i.uv, i.layer);
	if (c.a < u.params.z) { discard; }
	let col = c.rgb * i.color * textureSampleLevel(lightmap, lightSamp, i.light, 0.0).rgb;
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

var modelWGSL = `
struct U {
	viewProj: mat4x4f,
	model: mat4x4f,
	light: vec4f,    // lightmap colour (Renderer.LightColor)
	fogColor: vec4f, // rgb, fade
	fog: vec4f,      // start, end, alpha cutoff, emissive
	color: vec4f,    // texture multiplier (glColor4f)
	overlay: vec4f,  // rgb mixed over the lit result by a (hurt flash)
	bones: array<mat4x4f, ` + maxBonesWGSL + `>,
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
	let c = textureSample(tex, samp, i.uv) * u.color;
	if (c.a <= u.fog.z) { discard; }
	// Interleaved gradient noise: a stable screen-space dither for fading.
	let fade = u.fogColor.a;
	if (fade > 0.0 && fract(52.9829189 * fract(dot(i.pos.xy, vec2f(0.06711056, 0.00583715)))) < fade) { discard; }
	var light = i.shade * u.light.rgb;
	if (u.fog.w > 0.5) { light = vec3f(1.0); }
	let col = mix(c.rgb * light, u.overlay.rgb * light, u.overlay.a);
	return vec4f(mix(col, u.fogColor.rgb, fogFactor(i.dist, u.fog)), c.a);
}
`

const itemWGSL = `
struct U {
	mvp: mat4x4f,
	model: mat4x4f,  // model rotation+scale (camera-relative world space)
	light: vec4f,    // lightmap colour
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
	let col = c.rgb * i.shade * u.light.rgb;
	return vec4f(mix(col, u.fogColor.rgb, fogFactor(i.dist, u.fog)), 1.0);
}
`

const particleWGSL = `
struct U {
	viewProj: mat4x4f,
	right: vec4f,
	up: vec4f,
	params: vec4f,   // _, _, additive
	fogColor: vec4f,
	fog: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var atlas: texture_2d<f32>; // 16 x 16 sprite cells
@group(1) @binding(1) var samp: sampler;
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) color: vec4f,
	@location(1) uv: vec2f,
	@location(2) dist: f32,
	@location(3) sprite: f32,
}
` + wgslLight + `
@vertex
fn vs(@location(0) corner: vec2f, @location(1) ipos: vec3f, @location(2) size: f32,
	@location(3) color: vec4f, @location(4) sprite: vec2f, @location(5) rect: vec4f) -> VOut {
	let p = ipos + (u.right.xyz * corner.x + u.up.xyz * corner.y) * size;
	var o: VOut;
	o.pos = u.viewProj * vec4f(p, 1.0);
	o.color = color;
	let k = vec2f(corner.x + 0.5, 0.5 - corner.y);
	// EntityFX.renderParticle: cell (i % 16, i / 16), 0.0624375 wide.
	let cell = max(sprite.x - 1.0, 0.0);
	let c0 = vec2f(cell % 16.0, floor(cell / 16.0)) / 16.0;
	o.uv = c0 + k * 0.0624375;
	o.sprite = sprite.x;
	if (sprite.y > 0.5) { // a sheet's rectangle (block and item particles)
		o.uv = mix(rect.xy, rect.zw, k);
		o.sprite = 1.0;
	}
	o.dist = length(p);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	var c = i.color;
	let t = textureSample(atlas, samp, i.uv);
	if (i.sprite > 0.5) {
		c = c * t;
	}
	if (c.a <= 0.003921569) { // EffectRenderer's alpha test
		discard;
	}
	let f = fogFactor(i.dist, u.fog);
	if (u.params.z > 0.5) {
		return vec4f(c.rgb * c.a * (1.0 - f), 1.0);
	}
	return vec4f(mix(c.rgb, u.fogColor.rgb, f), c.a); // lit on the CPU
}
`

const skyListWGSL = `
struct U {
	mvp: mat4x4f,
	model: mat4x4f,  // to camera-relative world space (fog distance)
	fogColor: vec4f,
	fog: vec4f,      // start, end, enabled, alpha test
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) color: vec4f,
	@location(2) dist: f32,
}
` + wgslLight + `
@vertex
fn vs(@location(0) pos: vec3f, @location(1) uv: vec2f, @location(2) color: vec4f) -> VOut {
	var o: VOut;
	o.pos = u.mvp * vec4f(pos, 1.0);
	o.uv = uv;
	o.color = color;
	o.dist = length((u.model * vec4f(pos, 1.0)).xyz);
	return o;
}
@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	var c = textureSample(tex, samp, i.uv) * i.color;
	if (u.fog.w > 0.0 && c.a <= u.fog.w) { discard; }
	if (u.fog.z > 0.5) { c = vec4f(mix(c.rgb, u.fogColor.rgb, fogFactor(i.dist, u.fog)), c.a); }
	return c;
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
	skyAlpha, skyAdd            *wgpu.RenderPipeline
	outline, crosshair          *wgpu.RenderPipeline
	model, item                 *wgpu.RenderPipeline
	modelAlpha, modelAdd        *wgpu.RenderPipeline
	particleAlpha, particleAdd  *wgpu.RenderPipeline
	overlay                     *wgpu.RenderPipeline
	blockItem, blockDamage      *wgpu.RenderPipeline
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
	blendSrcAlphaOne = &gputypes.BlendState{ // glBlendFunc(GL_SRC_ALPHA, GL_ONE)
		Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorSrcAlpha, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
		Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorSrcAlpha, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd},
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
		texEntry(2, gputypes.ShaderStageVertex, gputypes.TextureSampleTypeUint, gputypes.TextureViewDimension2D),
		texEntry(3, frag, gputypes.TextureSampleTypeFloat, gputypes.TextureViewDimension2D), samplerEntry(4))
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
			attr(3, gputypes.VertexFormatUint16x4, 32), attr(4, gputypes.VertexFormatFloat32x4, 40)}}}
	itemBuf := []gputypes.VertexBufferLayout{{ArrayStride: 32, StepMode: gputypes.VertexStepModeVertex,
		Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x3, 0), attr(1, gputypes.VertexFormatFloat32x2, 12), attr(2, gputypes.VertexFormatFloat32x3, 20)}}}
	particleBuf := []gputypes.VertexBufferLayout{
		{ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x2, 0)}},
		{ArrayStride: particleStride * 4, StepMode: gputypes.VertexStepModeInstance, Attributes: []gputypes.VertexAttribute{
			attr(1, gputypes.VertexFormatFloat32x3, 0), attr(2, gputypes.VertexFormatFloat32, 12),
			attr(3, gputypes.VertexFormatFloat32x4, 16), attr(4, gputypes.VertexFormatFloat32x2, 32), attr(5, gputypes.VertexFormatFloat32x4, 40)}},
	}

	skyBuf := []gputypes.VertexBufferLayout{{ArrayStride: 36, StepMode: gputypes.VertexStepModeVertex,
		Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x3, 0), attr(1, gputypes.VertexFormatFloat32x2, 12), attr(2, gputypes.VertexFormatFloat32x4, 20)}}}
	tri, lines := gputypes.PrimitiveTopologyTriangleList, gputypes.PrimitiveTopologyLineList
	back, none := gputypes.CullModeBack, gputypes.CullModeNone
	specs := []struct {
		dst  **wgpu.RenderPipeline
		spec pipeSpec
	}{
		{&ps.terrainOpaque, pipeSpec{"terrain", terrainWGSL, ps.terrainTex, terrainBuf, tri, back, nil, true, true, false}},
		{&ps.terrainTrans, pipeSpec{"terrain translucent", terrainWGSL, ps.terrainTex, terrainBuf, tri, back, blendAlpha, true, false, false}},
		{&ps.sky, pipeSpec{"sky", skyWGSL, nil, nil, tri, back, nil, true, false, false}},
		{&ps.skyAlpha, pipeSpec{"sky list", skyListWGSL, ps.tex2D, skyBuf, tri, none, blendAlpha, true, false, false}},
		{&ps.skyAdd, pipeSpec{"sky list additive", skyListWGSL, ps.tex2D, skyBuf, tri, none, blendSrcAlphaOne, true, false, false}},
		{&ps.outline, pipeSpec{"outline", lineWGSL, nil, lineBuf, lines, none, blendAlpha, true, true, false}},
		{&ps.crosshair, pipeSpec{"crosshair", lineWGSL, nil, lineBuf, lines, none, blendInvert, false, false, false}},
		{&ps.model, pipeSpec{"model", modelWGSL, ps.tex2D, modelBuf, tri, none, nil, true, true, false}},
		{&ps.modelAlpha, pipeSpec{"model alpha", modelWGSL, ps.tex2D, modelBuf, tri, none, blendAlpha, true, false, false}},
		{&ps.modelAdd, pipeSpec{"model additive", modelWGSL, ps.tex2D, modelBuf, tri, none, blendAdd, true, false, false}},
		{&ps.item, pipeSpec{"item", itemWGSL, ps.texArray, itemBuf, tri, none, nil, true, true, false}},
		{&ps.particleAlpha, pipeSpec{"particles", particleWGSL, ps.tex2D, particleBuf, tri, none, blendAlpha, true, false, false}},
		{&ps.particleAdd, pipeSpec{"particles additive", particleWGSL, ps.tex2D, particleBuf, tri, none, blendAdd, true, false, false}},
		{&ps.overlay, pipeSpec{"overlay", overlayWGSL, ps.tex2D, nil, gputypes.PrimitiveTopologyTriangleStrip, none, blendAlpha, false, false, false}},
		{&ps.blockItem, pipeSpec{"block item", blockWGSL, ps.terrainTex, terrainBuf, tri, back, blendAlpha, true, true, false}},
		{&ps.blockDamage, pipeSpec{"block damage", blockWGSL, ps.terrainTex, terrainBuf, tri, back, blendDamage, true, true, false}},
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
		if s.dst == &ps.blockDamage { // glPolygonOffset(-3, -3)
			depth.DepthBias, depth.DepthBiasSlopeScale = -3, -3
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
