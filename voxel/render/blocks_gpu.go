package render

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// blockWGSL draws a BlockMesh: mesher vertices (mesh.VertexWords) placed
// by a model matrix, lit like a Minecraft item (two GL lights) or, for
// the breaking overlay, the plain crack texture.
const blockWGSL = `
const damagePull = 0.002;

struct U {
	mvp: mat4x4f,    // mesh local (blocks) -> clip; overlay: camera-relative -> clip
	model: mat4x4f,  // mesh local -> the lights' space (normals, fog distance)
	light: vec4f,    // lightmap colour; w: 1 = breaking overlay
	fogColor: vec4f,
	fog: vec4f,      // start, end, overlay texture layer
	l0: vec4f,       // GL_LIGHT0 / GL_LIGHT1 directions (RenderHelper)
	l1: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
@group(1) @binding(2) var layerMap: texture_2d<u32>;
@group(1) @binding(3) var lightmap: texture_2d<f32>;
@group(1) @binding(4) var lightSamp: sampler;

struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) @interpolate(flat) layer: u32,
	@location(2) color: vec3f,
	@location(3) dist: f32,
}
` + wgslLight + `
@vertex
fn vs(@location(0) d: vec4<u32>) -> VOut {
	var normals = array<vec3f, 6>(vec3f(0.0, -1.0, 0.0), vec3f(0.0, 1.0, 0.0), vec3f(0.0, 0.0, -1.0),
		vec3f(0.0, 0.0, 1.0), vec3f(-1.0, 0.0, 0.0), vec3f(1.0, 0.0, 0.0));
	let w0 = d.x; let w1 = d.y; let w2 = d.z;
	let coarse = vec3f(f32(w0 & 1023u), f32((w0 >> 10u) & 1023u), f32((w0 >> 20u) & 1023u));
	let fine = vec3f(f32((w1 >> 26u) & 3u), f32((w1 >> 28u) & 3u), f32(w1 >> 30u));
	let local = (coarse * 4.0 + fine) * (1.0 / 128.0) - 8.0;
	var o: VOut;
	o.pos = u.mvp * vec4f(local, 1.0);
	if (u.light.w > 0.5) {
		// The overlay's mvp is the camera-relative view-projection and model
		// places the block: pull it towards the eye along its own view ray
		// (same pixels) by damagePull of its distance, so it always wins the
		// depth test against the terrain's copy of the face (glPolygonOffset).
		o.pos = u.mvp * vec4f((u.model * vec4f(local, 1.0)).xyz * (1.0 - damagePull), 1.0);
	}
	o.uv = vec2f(f32((w1 >> 12u) & 31u), f32((w1 >> 17u) & 31u));
	if ((w1 & (1u << 25u)) != 0u) { o.uv = o.uv * (1.0 / 16.0); }
	o.dist = length((u.model * vec4f(local, 1.0)).xz);
	if (u.light.w > 0.5) { // RenderGlobal.drawBlockDamageTexture: colour array and lighting off
		o.layer = u32(u.fog.z);
		o.color = vec3f(1.0);
		return o;
	}
	o.layer = textureLoad(layerMap, vec2<i32>(i32(w1 & 4095u), 0), 0).r;
	let tint = vec3f(f32(w2 & 255u), f32((w2 >> 8u) & 255u), f32((w2 >> 16u) & 255u)) * (1.0 / 255.0);
	let n = (w1 >> 22u) & 7u;
	var shade = 1.0; // faces without one direction (rotated, unshaded boxes)
	if (n < 6u) {
		// RenderHelper.enableStandardItemLighting: ambient 0.4, two diffuse 0.6 lights
		let nn = normalize(mat3x3f(u.model[0].xyz, u.model[1].xyz, u.model[2].xyz) * normals[n]);
		shade = min(1.0, 0.4 + 0.6 * (max(dot(nn, u.l0.xyz), 0.0) + max(dot(nn, u.l1.xyz), 0.0)));
	}
	o.color = tint * shade * u.light.rgb;
	return o;
}

@fragment
fn fs(i: VOut) -> @location(0) vec4f {
	var c = textureSample(tex, samp, i.uv, i.layer);
	if (u.light.w > 0.5) { c.a = c.a * 0.5; } // glColor4f(1, 1, 1, 0.5)
	if (c.a <= 0.1) { discard; }              // glAlphaFunc(GL_GREATER, 0.1)
	return vec4f(mix(c.rgb * i.color, u.fogColor.rgb, fogFactor(i.dist, u.fog)), c.a);
}
`

// blendDamage is glBlendFunc(GL_DST_COLOR, GL_SRC_COLOR): 2 * src * dst.
var blendDamage = &gputypes.BlendState{
	Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorDst, DstFactor: gputypes.BlendFactorSrc, Operation: gputypes.BlendOperationAdd},
	Alpha: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorZero, Operation: gputypes.BlendOperationAdd},
}

type gpuBlockMesh struct{ vbo *wgpu.Buffer }

func (b *BlockMesh) upload(data []uint32) {
	b.gpu.vbo = newBuffer("block mesh", wgpu.BufferUsageVertex, bytesOf(data), 0)
}

// Delete frees the mesh's GPU buffer.
func (b *BlockMesh) Delete() {
	if b != nil && b.gpu.vbo != nil {
		b.gpu.vbo.Release()
		b.gpu.vbo = nil
	}
}

// gpuDamage holds the crack textures (Renderer.SetDamageTextures).
type gpuDamage struct {
	tex   texture2D
	bg    *wgpu.BindGroup
	bgFor *textureArray
	bgGen int
}

func (r *Renderer) uploadDamage(size int, layers [][]byte) {
	d := &r.damage
	r.releaseDamage()
	d.tex = newTexture("block damage", size, size, len(layers), 1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2DArray)
	for i, px := range layers {
		d.tex.write(0, i, size, size, 4, px)
	}
}

func (r *Renderer) releaseDamage() {
	d := &r.damage
	if d.bg != nil {
		d.bg.Release()
		d.bg = nil
	}
	d.tex.release()
	d.tex = texture2D{}
}

// damageBG is the terrain bind group layout with the crack textures in
// place of the block textures.
func (r *Renderer) damageBG() *wgpu.BindGroup {
	d := &r.damage
	if d.bg == nil || d.bgFor != r.tex || d.bgGen != r.tex.gen {
		if d.bg != nil {
			d.bg.Release()
		}
		d.bg = bindGroup("block damage", dev.pipes.terrainTex,
			wgpu.BindGroupEntry{Binding: 0, TextureView: d.tex.view},
			wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestRep},
			wgpu.BindGroupEntry{Binding: 2, TextureView: r.tex.gpu.layerMap.view},
			wgpu.BindGroupEntry{Binding: 3, TextureView: r.gpu.lightTex.view},
			wgpu.BindGroupEntry{Binding: 4, Sampler: dev.linearClp})
		d.bgFor, d.bgGen = r.tex, r.tex.gen
	}
	return d.bg
}

// drawBlock issues one BlockMesh draw. mvp maps mesh-local blocks to clip
// space, model to the lights' space; damage >= 0 draws the crack layer
// (mvp is then the camera-relative view-projection, model the placement).
func (r *Renderer) drawBlock(b *BlockMesh, mvp, model mgl32.Mat4, lc mgl32.Vec3, fog Fog, l0, l1 mgl32.Vec3, damage int) {
	p := curPass()
	if p == nil || b == nil || b.gpu.vbo == nil || b.quads == 0 {
		return
	}
	pl, bg, mode, layer := dev.pipes.blockItem, r.terrainBG(), float32(0), float32(0)
	if damage >= 0 {
		if r.damage.tex.tex == nil {
			return
		}
		pl, bg, mode, layer = dev.pipes.blockDamage, r.damageBG(), 1, float32(damage)
	}
	p.SetPipeline(pl)
	u := &r.gpu.u
	bindUniforms(p, u.reset().mat(clip(mvp)).mat(model).f(lc[0], lc[1], lc[2], mode).
		f(fog.Color[0], fog.Color[1], fog.Color[2], 0).f(fog.Start, fog.End, layer, 0).
		f(l0[0], l0[1], l0[2], 0).f(l1[0], l1[1], l1[2], 0))
	p.SetBindGroup(1, bg, nil)
	p.SetIndexBuffer(r.gpu.ebo, gputypes.IndexFormatUint32, 0)
	p.SetVertexBuffer(0, b.gpu.vbo, 0)
	dev.draws++
	p.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: uint32(b.quads * 6), InstanceCount: 1})
}
