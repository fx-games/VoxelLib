package render

import (
	"regexp"
	"strings"
)

// WGSL ports of the shader pack's programs (see pack.go). The pack's GLSL
// was preprocessed with its default options (shaders.settings) and
// translated line by line, keeping its constants and maths; comments name
// the GLSL program. GL conventions are kept: clip space, NDC and depth as
// in GL (the shadow and gbuffer depth equal GL window depth), render target
// coordinates with t up (glUV flips v) and gl_FragCoord with y up
// (glFrag). Uploaded textures (blocks, noisetex) need no flip: GL t and
// WebGPU v both start at the first uploaded row.

// packCommonWGSL is shared by every pack program: the uniform block
// (packUniforms) and OptiFine's built-ins. pu is declared by the program.
const packCommonWGSL = `
struct PackU {
	mv: mat4x4f, mvInv: mat4x4f, proj: mat4x4f, projInv: mat4x4f,
	prevMV: mat4x4f, prevProj: mat4x4f, shadowMV: mat4x4f, shadowProj: mat4x4f,
	sunPos: vec4f, moonPos: vec4f, upPos: vec4f, shadowLightPos: vec4f,
	camPos: vec4f, prevCamPos: vec4f, fogColor: vec4f,
	view: vec4f,    // viewWidth, viewHeight, aspectRatio, frameTimeCounter
	weather: vec4f, // rainStrength, wetness, near, far
	eye: vec4f,     // nightVision, blindness, eyeBrightnessSmooth
	eye2: vec4f,    // eyeBrightness, frameTime, celestialAngle
	ints: vec4<i32>, // worldTime, isEyeInWater, frameCounter, lotrTime
}

const PI = 3.1415927;
// shadow distortion (shaders.settings): a = exp(0.05), b = (exp(1)-a)*80/128, k = 1.8
const shadowA = 1.05127108;
const shadowB = 1.04188168;
const shadowK = 1.8;
const shadowMapResolution = 2048.0;
const shadowDistance = 80.0;
// TAA jitter offsets[framemod8]: framemod8 is a custom uniform OptiFine
// 1.7.10 E7 does not set, so it is always 0.
fn taaOffset() -> vec2f { return vec2f(1.0 / 8.0, -3.0 / 8.0); }

fn calcDistortion(worldpos: vec2f) -> f32 {
	return 1.0 / (log(length(worldpos) * shadowB + shadowA) * shadowK);
}
fn gmod(x: f32, y: f32) -> f32 { return x - y * floor(x / y); }
fn luma(c: vec3f) -> f32 { return dot(c, vec3f(0.299, 0.587, 0.114)); }
fn m3(m: mat4x4f) -> mat3x3f { return mat3x3f(m[0].xyz, m[1].xyz, m[2].xyz); }
fn rainStrength() -> f32 { return pu.weather.x; }
fn frameTimeCounter() -> f32 { return pu.view.w; }
fn lotrTime() -> f32 { return f32(pu.ints.w); }
fn texelSize() -> vec2f { return 1.0 / pu.view.xy; }
fn nearP() -> f32 { return pu.weather.z; }
fn farP() -> f32 { return pu.weather.w; }
fn comp() -> f32 { return 1.0 - nearP() / farP() / farP(); }
fn isEyeInWater() -> f32 { return f32(pu.ints.y); }
fn eyeSmoothY() -> f32 { return pu.eye.w; }
fn night() -> f32 { return clamp((lotrTime() - 13000.0) / 300.0, 0.0, 1.0) - clamp((lotrTime() - 22800.0) / 200.0, 0.0, 1.0); }
// GL texture coordinates (t up) of a render target to WebGPU (v down).
fn glUV(t: vec2f) -> vec2f { return vec2f(t.x, 1.0 - t.y); }
// gl_FragCoord.xy from @builtin(position).
fn glFrag(p: vec4f) -> vec2f { return vec2f(p.x, pu.view.y - p.y); }
// float hour, cmpH, cmpH1 and the ToD mix of the lighting shaders.
fn todMix(t: array<vec3f, 7>) -> vec3f {
	var tt = t;
	let hour = max(gmod(lotrTime() / 1000.0 + 2.0, 24.0) - 2.0, 0.0);
	let cmpH = max(-abs(floor(hour) - 6.0) + 6.0, 0.0);
	let cmpH1 = max(-abs(floor(hour) - 5.0) + 6.0, 0.0);
	return mix(tt[i32(cmpH)], tt[i32(cmpH1)], fract(hour));
}
// tr from trCalc = min(abs(lotrTime-vec2(t0,12700.0)),750.0).
fn trOf(t0: f32) -> f32 {
	let trCalc = min(abs(vec2f(lotrTime()) - vec2f(t0, 12700.0)), vec2f(750.0));
	return max(min(trCalc.x, trCalc.y) / 375.0 - 1.0, 0.0);
}
fn ycocg(c: vec3f) -> vec3f {
	return vec3f(0.25 * c.r + 0.5 * c.g + 0.25 * c.b, 0.5 * c.r - 0.5 * c.b + 0.5, -0.25 * c.r + 0.5 * c.g - 0.25 * c.b + 0.5);
}
fn encodeNormal(n: vec3f) -> vec2f {
	let p = sqrt(n.z * 8.0 + 8.0);
	return n.xy / p + 0.5;
}
fn decodeNormal(enc: vec2f) -> vec3f {
	let fenc = enc * 4.0 - 2.0;
	let f = dot(fenc, fenc);
	let g = sqrt(1.0 - f / 4.0);
	return vec3f(fenc * g, 1.0 - f / 2.0);
}
// The checkerboard of gbuffers_terrain/basic: mod(x,2)==mod(y,2).
fn pattern(fc: vec2f) -> bool { return gmod(fc.x, 2.0) == gmod(fc.y, 2.0); }
// ftransform plus gl_Position.xy += offsets[framemod8]*gl_Position.w*texelSize.
fn jitter(p: vec4f) -> vec4f { return vec4f(p.xy + taaOffset() * p.w * texelSize(), p.zw); }
// GL clip (z -w..w) to WebGPU (z 0..w), as the engine's clip().
fn toWGPU(p: vec4f) -> vec4f { return vec4f(p.x, p.y, 0.5 * (p.z + p.w), p.w); }
// lightmap texture coordinates (gl_TextureMatrix[1] * gl_MultiTexCoord1)
// for light levels 0..15: (level*16 + 8) / 256.
fn lmOf(block: f32, sky: f32) -> vec2f { return (vec2f(block, sky) * 16.0 + 8.0) / 256.0; }
`

// packGroupWGSL is group 2 of the gbuffer programs.
const packGroupWGSL = `
@group(2) @binding(0) var<uniform> pu: PackU;
@group(2) @binding(1) var shadowtex0: texture_depth_2d;
@group(2) @binding(2) var shadowCmp: sampler_comparison;
@group(2) @binding(3) var noisetex: texture_2d<f32>;
@group(2) @binding(4) var linRep: sampler;
// shadow2D(shadowtex0, p): p in GL texture coordinates and depth.
fn shadow2D(p: vec3f) -> f32 { return textureSampleCompareLevel(shadowtex0, shadowCmp, glUV(p.xy), p.z); }
`

// packWaveWGSL is gbuffers_terrain.vsh's calcWave/calcMove.
const packWaveWGSL = `
const PI48 = 150.796447372;
fn calcWave(pos: vec3f, fm: f32, mm: f32, ma: f32, f0: f32, f1: f32, f2: f32, f3: f32, f4: f32, f5: f32) -> vec3f {
	let pi2wt = (PI48 * frameTimeCounter()) * 1.0;
	var ret: vec3f;
	let magnitude = sin(pi2wt * fm + pos.x * 0.5 + pos.z * 0.5 + pos.y * 0.5) * mm + ma;
	let d0 = sin(pi2wt * f0);
	let d1 = sin(pi2wt * f1);
	let d2 = sin(pi2wt * f2);
	ret.x = sin(pi2wt * f3 + d0 + d1 - pos.x + pos.z + pos.y) * magnitude;
	ret.z = sin(pi2wt * f4 + d1 + d2 + pos.x - pos.z + pos.y) * magnitude;
	ret.y = sin(pi2wt * f5 + d2 + d0 + pos.z + pos.y - pos.y) * magnitude;
	return ret;
}
fn calcMove(pos: vec3f, f0: f32, f1: f32, f2: f32, f3: f32, f4: f32, f5: f32, amp1: vec3f, amp2: vec3f) -> vec3f {
	let move1 = calcWave(pos, 0.0027, 0.0400, 0.0400, 0.0127, 0.0089, 0.0114, 0.0063, 0.0224, 0.0015) * amp1;
	let move2 = calcWave(pos + move1, 0.0348, 0.0400, 0.0400, f0, f1, f2, f3, f4, f5) * amp2;
	return move1 + move2;
}
`

// Material classes in terrain vertex word 3 bits 24..31
// (mesh.Config.Materials): block.properties' mc_Entity ids.
const (
	PackMatNone      = 0
	PackMatTallgrass = 1  // 10031
	PackMatCrops     = 2  // 10059
	PackMatLeaves    = 3  // 10018
	PackMatVine      = 4  // 10106
	PackMatLily      = 5  // 10111
	PackMatFire      = 6  // 10051
	PackMatLava      = 7  // 10010
	PackMatWater     = 8  // 10008
	PackMatIce       = 9  // 10079
	PackMatWeb       = 10 // 10030
	PackMatDeadbush  = 11 // 10032
	PackMatWart      = 12 // 10115
	PackMatEmissive  = 13 // 10089
)

// PackMaterial is the pack's block.properties (1.7.10 names) for a block
// name. OptiFine E7's parser has no block-state syntax, so the
// double_plant:half= lines are rejected and double plants get no class.
func PackMaterial(name string) uint8 {
	switch name {
	case "minecraft:tallgrass":
		return PackMatTallgrass
	case "minecraft:wheat", "minecraft:sapling", "minecraft:yellow_flower", "minecraft:red_flower", "minecraft:carrots", "minecraft:beetroot", "minecraft:potatoes":
		return PackMatCrops
	case "minecraft:leaves", "minecraft:leaves2":
		return PackMatLeaves
	case "minecraft:vine":
		return PackMatVine
	case "minecraft:waterlily":
		return PackMatLily
	case "minecraft:fire":
		return PackMatFire
	case "minecraft:flowing_lava", "minecraft:lava":
		return PackMatLava
	case "minecraft:flowing_water", "minecraft:water":
		return PackMatWater
	case "minecraft:ice", "minecraft:slime", "minecraft:stained_glass", "minecraft:stained_glass_pane":
		return PackMatIce
	case "minecraft:web":
		return PackMatWeb
	case "minecraft:deadbush":
		return PackMatDeadbush
	case "minecraft:nether_wart":
		return PackMatWart
	case "minecraft:glowstone", "minecraft:torch", "minecraft:redstone_torch", "minecraft:lit_pumpkin", "minecraft:lit_redstone_lamp", "minecraft:beacon":
		return PackMatEmissive
	}
	return PackMatNone
}

// terrainHeaderWGSL is terrainWGSL's uniform block and group 1.
const terrainHeaderWGSL = `
struct U {
	viewProj: mat4x4f,
	origin: vec4f,
	fogColor: vec4f,
	params: vec4f,
	fog: vec4f,
	dynTint: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
@group(1) @binding(2) var layerMap: texture_2d<u32>;
@group(1) @binding(3) var lightmap: texture_2d<f32>;
@group(1) @binding(4) var lightSamp: sampler;
`

// packTerrainDecodeWGSL decodes a terrain vertex like terrainWGSL's vs:
// camera-relative position, tile uv, layer, colour without the face shade
// (OptiFine's shaders mode drops it), light coordinates, face and class.
const packTerrainDecodeWGSL = `
struct TV { pos: vec3f, uv: vec2f, layer: u32, color: vec3f, light: vec2f, n: u32, mat: u32 }
fn decodeTerrain(d: vec4<u32>) -> TV {
	var ao = array<f32, 4>(0.45, 0.65, 0.82, 1.0);
	let w0 = d.x; let w1 = d.y; let w2 = d.z; let w3 = d.w;
	let coarse = vec3f(f32(w0 & 1023u), f32((w0 >> 10u) & 1023u), f32((w0 >> 20u) & 1023u));
	let fine = vec3f(f32((w1 >> 26u) & 3u), f32((w1 >> 28u) & 3u), f32(w1 >> 30u));
	let local = (coarse * 4.0 + fine) * (1.0 / 128.0) - 8.0;
	let si = w2 >> 24u;
	let sec = vec3f(f32(si & 7u), f32((si >> 3u) & 3u), f32((si >> 5u) & 7u)) * 16.0;
	var t: TV;
	t.pos = u.origin.xyz + sec + local;
	t.layer = textureLoad(layerMap, vec2<i32>(i32(w1 & 4095u), 0), 0).r;
	t.uv = vec2f(f32((w1 >> 12u) & 31u), f32((w1 >> 17u) & 31u));
	if ((w1 & (1u << 25u)) != 0u) { t.uv = t.uv * (1.0 / 16.0); }
	t.n = (w1 >> 22u) & 7u;
	let tint = vec3f(f32(w2 & 255u), f32((w2 >> 8u) & 255u), f32((w2 >> 16u) & 255u)) * (1.0 / 255.0);
	let wt = f32((w3 >> 16u) & 255u) * (1.0 / 255.0);
	t.color = (tint + wt * u.dynTint.rgb) * ao[w0 >> 30u];
	t.light = (vec2f(f32((w3 >> 8u) & 255u), f32(w3 & 255u)) + 8.0) * (1.0 / 256.0);
	t.mat = w3 >> 24u;
	return t;
}
fn faceNormal(n: u32) -> vec3f {
	var normals = array<vec3f, 7>(vec3f(0.0, -1.0, 0.0), vec3f(0.0, 1.0, 0.0), vec3f(0.0, 0.0, -1.0),
		vec3f(0.0, 0.0, 1.0), vec3f(-1.0, 0.0, 0.0), vec3f(1.0, 0.0, 0.0), vec3f(0.0, 1.0, 0.0));
	return normals[min(n, 6u)];
}
`

// packTerrainWGSL is gbuffers_terrain (DRAWBUFFERS:01) for the solid and
// cutout layers.
var packTerrainWGSL = terrainHeaderWGSL + packCommonWGSL + packGroupWGSL + packWaveWGSL + packTerrainDecodeWGSL + `
struct GV {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) @interpolate(flat) layer: u32,
	@location(2) color: vec4f,
	@location(3) light: vec2f,
	@location(4) normal: vec4f, // view-space normal, normal.a (material)
	@location(5) @interpolate(flat) flat: u32, // 1: the quad's own normal (no face direction)
	@location(6) vpos: vec3f,
}
@vertex
fn vs(@location(0) d: vec4<u32>) -> GV {
	let t = decodeTerrain(d);
	var o: GV;
	var na = 0.02;
	let m = t.mat;
	var position = t.pos;
	let worldpos = position + pu.camPos.xyz;
	let istopv = t.uv.y < 0.5; // gl_MultiTexCoord0.t < mc_midTexCoord.t
	if (istopv) {
		if (m == 1u) { position += calcMove(worldpos, 0.0041, 0.0070, 0.0044, 0.0038, 0.0063, 0.0000, vec3f(3.0, 1.6, 3.0), vec3f(0.0, 0.0, 0.0)); }
		if (m == 2u) { position += calcMove(worldpos, 0.0041, 0.0070, 0.0044, 0.0038, 0.0240, 0.0000, vec3f(0.8, 0.0, 0.8), vec3f(0.4, 0.0, 0.4)); }
		if (m == 6u) { position += calcMove(worldpos, 0.0105, 0.0096, 0.0087, 0.0063, 0.0097, 0.0156, vec3f(1.2, 0.4, 1.2), vec3f(0.8, 0.8, 0.8)); }
	}
	if (m == 3u) { position += calcMove(worldpos, 0.0040, 0.0064, 0.0043, 0.0035, 0.0037, 0.0041, vec3f(1.0, 0.2, 1.0), vec3f(0.5, 0.1, 0.5)); }
	if (m == 4u) { position += calcMove(worldpos, 0.0040, 0.0064, 0.0043, 0.0035, 0.0037, 0.0041, vec3f(0.5, 1.0, 0.5), vec3f(0.25, 0.5, 0.25)); }
	if (m == 7u) {
		let fy = fract(worldpos.y + 0.001);
		let wave = 0.05 * sin(2.0 * PI * (frameTimeCounter() * 0.2 + worldpos.x / 7.0 + worldpos.z / 13.0))
			+ 0.05 * sin(2.0 * PI * (frameTimeCounter() * 0.15 + worldpos.x / 11.0 + worldpos.z / 5.0));
		position.y += clamp(wave, -fy, 1.0 - fy) * 0.5;
	}
	if (m == 5u) {
		let fy = fract(worldpos.y + 0.001);
		let wave = 0.05 * sin(2.0 * PI * (frameTimeCounter() * 0.8 + worldpos.x / 2.5 + worldpos.z / 5.0))
			+ 0.05 * sin(2.0 * PI * (frameTimeCounter() * 0.6 + worldpos.x / 6.0 + worldpos.z / 12.0));
		position.y += clamp(wave, -fy, 1.0 - fy) * 1.05;
	}
	var color = vec4f(t.color, 1.0);
	if (m == 6u || m == 7u || m == 13u) { na = 0.6; color = vec4f(1.0); }
	if (m == 4u || m == 2u || m == 10u || m == 1u || m == 12u || m == 5u || m == 7u || m == 3u) { na = 0.7; }
	o.pos = jitter(u.viewProj * vec4f(position, 1.0));
	o.uv = t.uv;
	o.layer = t.layer;
	o.color = color;
	o.light = t.light;
	o.normal = vec4f(m3(pu.mv) * faceNormal(t.n), na);
	o.flat = select(0u, 1u, t.n >= 6u);
	o.vpos = (pu.mv * vec4f(position, 1.0)).xyz;
	return o;
}
struct GOut {
	@location(0) c0: vec4f,
	@location(1) c1: vec4f,
}
@fragment
fn fs(i: GV) -> GOut {
	// ShadersTess.calcNormal: faces without a fixed direction (crossed
	// plants) get the quad's own normal, facing the eye on a front face.
	let dn = normalize(cross(dpdy(i.vpos), dpdx(i.vpos)));
	let albedo = textureSample(tex, samp, i.uv, i.layer) * i.color;
	if (albedo.a <= 0.1) { discard; } // glAlphaFunc(GL_GREATER, 0.1)
	var c = vec4f(ycocg(albedo.rgb), albedo.a);
	if (pattern(glFrag(i.pos))) { c.g = c.b; }
	c.b = i.normal.a;
	var n = i.normal.xyz;
	if (i.flat == 1u) { n = dn; }
	var o: GOut;
	o.c0 = c;
	o.c1 = vec4f(encodeNormal(n), i.light);
	return o;
}
`

// packWaterWGSL is gbuffers_water (DRAWBUFFERS:526) for the translucent
// terrain layer. The tangent frame (OptiFine's ShadersTess at_tangent) is
// rebuilt per fragment from screen derivatives, exact for planar quads.
var packWaterWGSL = terrainHeaderWGSL + packCommonWGSL + packGroupWGSL + packTerrainDecodeWGSL + `
struct WV {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) @interpolate(flat) layer: u32,
	@location(2) color: vec4f,
	@location(3) lm: vec2f,
	@location(4) amb: vec4f,      // ambientNdotL
	@location(5) sunl: vec3f,     // getSunlight
	@location(6) vd: vec3f,       // view position / its length (viewVector before the TBN)
	@location(7) worldpos: vec3f,
	@location(8) shadowPos: vec3f, // getShadowPos
	@location(9) diffuse: f32,
	@location(10) vpos: vec3f,
}
fn waterToD() -> array<vec3f, 7> { return array<vec3f, 7>(vec3f(0.58597, 0.16, 0.005), vec3f(0.58597, 0.31, 0.05), vec3f(0.58597, 0.45, 0.16),
	vec3f(0.58597, 0.5, 0.35), vec3f(0.58597, 0.5, 0.36), vec3f(0.58597, 0.5, 0.37), vec3f(0.58597, 0.5, 0.38)); }
fn SunIntensity(zenithAngleCos: f32, sunIntensity: f32, cutoffAngle: f32, steepness: f32) -> f32 {
	return sunIntensity * max(0.0, 1.0 - exp(-((cutoffAngle - acos(zenithAngleCos)) / steepness)));
}
@vertex
fn vs(@location(0) d: vec4<u32>) -> WV {
	let t = decodeTerrain(d);
	var o: WV;
	let normal = normalize(m3(pu.mv) * faceNormal(t.n));
	var position = t.pos;
	o.worldpos = position + pu.camPos.xyz;
	let worldpos = o.worldpos;
	o.color = vec4f(t.color, 1.0);
	o.uv = t.uv;
	o.layer = t.layer;
	let lmcoord = t.light;
	o.lm = lmcoord;
	o.diffuse = clamp(dot(normal, normalize(pu.shadowLightPos.xyz)), 0.0, 1.0);
	let diffuse = o.diffuse;
	o.shadowPos = vec3f(0.0);
	if (diffuse > 0.001) {
		var shadowpos = position;
		shadowpos = m3(pu.shadowMV) * shadowpos + pu.shadowMV[3].xyz;
		shadowpos = vec3f(pu.shadowProj[0].x, pu.shadowProj[1].y, pu.shadowProj[2].z) * shadowpos + pu.shadowProj[3].xyz;
		let distortion = calcDistortion(shadowpos.xy);
		shadowpos = vec3f(shadowpos.xy * distortion, shadowpos.z);
		if (abs(shadowpos.x) < 1.0 - 1.5 / shadowMapResolution && abs(shadowpos.y) < 1.0 - 1.5 / shadowMapResolution && abs(shadowpos.z) < 6.0) {
			let threshMul = max(2048.0 / shadowMapResolution * shadowDistance / 128.0, 0.95);
			let distortThresh = (sqrt(1.0 - diffuse * diffuse) / diffuse + 0.7) / distortion;
			let bias = distortThresh / 6000.0 * threshMul;
			shadowpos = shadowpos * vec3f(0.5, 0.5, 0.5 / 6.0) + vec3f(0.5, 0.5, 0.5);
			shadowpos.z -= bias;
			o.shadowPos = shadowpos;
		}
	}
	var ambA = 0.0;
	var iswater = 1.0;
	if (t.mat == 8u) {
		ambA = 1.0;
		iswater = 0.0;
		let fy = fract(worldpos.y + 0.001);
		let wave = 0.05 * sin(2.0 * PI * (frameTimeCounter() * 0.8 + worldpos.x / 2.5 + worldpos.z / 5.0))
			+ 0.05 * sin(2.0 * PI * (frameTimeCounter() * 0.6 + worldpos.x / 6.0 + worldpos.z / 12.0));
		position.y += clamp(wave, -fy, 1.0 - fy) * 0.65;
	}
	if (t.mat == 9u) { ambA = 0.5; }
	o.pos = jitter(pu.proj * pu.mv * vec4f(position, 1.0));
	o.pos = toWGPU(o.pos);
	var sunlight = todMix(waterToD());
	var torch_lightmap = 16.0 - min(15.0, (lmcoord.x - 0.5 / 16.0) * 16.0 * 16.0 / 15.0);
	let fallof1 = clamp(1.0 - pow(torch_lightmap / 16.0, 4.0), 0.0, 1.0);
	torch_lightmap = fallof1 * fallof1 / (torch_lightmap * torch_lightmap + 1.0);
	torch_lightmap *= iswater;
	let emissiveLightC = vec3f(1.5, 0.42, 0.045) * torch_lightmap * 0.2;
	let sunVec = normalize(pu.sunPos.xyz);
	let upVec = normalize(pu.upPos.xyz);
	var visibility = vec2f(dot(sunVec, upVec), dot(-sunVec, upVec));
	let cutoffAngle = 1.608;
	let steepness = 1.5;
	let cosSunUpAngle = dot(sunVec, upVec) * 0.95 + 0.05;
	let sunE = SunIntensity(cosSunUpAngle, 1000.0, cutoffAngle, steepness);
	let NdotL = dot(normal, sunVec);
	let NdotU = dot(normal, upVec);
	let tr = trOf(23000.0);
	visibility = pow(clamp(visibility + 0.15, vec2f(0.0), vec2f(0.3)) / 0.3, vec2f(4.4));
	sunlight = sunlight / luma(sunlight) * sunE * 0.0075 * 0.075 * 3.0 * visibility.x;
	let rain = rainStrength();
	let skyL = max(lmcoord.y - 2.0 / 16.0, 0.0) * 1.14285714286;
	let SkyL2 = skyL * skyL;
	let skyc2 = mix(1.0, SkyL2, skyL);
	var bounced = vec4f(NdotL, NdotL, NdotL, NdotU) * vec4f(-0.14 * skyL * skyL, 0.33, 0.7, 0.1) + vec4f(0.6, 0.66, 0.7, 0.25);
	bounced *= vec4f(skyc2, skyc2, visibility.x - tr * visibility.x, 0.8);
	var ambientC = mix(vec3f(0.3, 0.5, 1.1), vec3f(0.08, 0.1, 0.1), rain) * length(sunlight) * bounced.w;
	ambientC += 0.25 * sunlight * (bounced.x + bounced.z) * (0.03 + tr * 0.17) / 0.4 * (1.0 - rain * 0.98) + length(sunlight) * 0.2 * (1.0 - rain * 0.9);
	ambientC += sunlight * (NdotL * 0.5 + 0.45) * visibility.x * (1.0 - tr) * (1.0 - tr) * 4.0 * (1.0 - rain * 0.98);
	let moonlight = vec3f(0.0024, 0.00432, 0.0078);
	let moon_ambient = (moonlight * 2.0 + moonlight * bounced.y) * (4.0 - rain * 0.95) * 0.2;
	let moonC = (moon_ambient * visibility.y) * SkyL2 * (0.03 * 0.65 + tr * 0.17 * 0.65);
	let minlight = select(0.00008, 0.075, pu.eye.x > 0.9);
	o.amb = vec4f(ambientC * SkyL2 * 0.3 + moonC + emissiveLightC + minlight, ambA);
	sunlight = mix(sunlight, moonlight * (1.0 - rain * 0.9), visibility.y);
	o.sunl = sunlight;
	let mvpos = (pu.mv * vec4f(t.pos, 1.0)).xyz; // gl_ModelViewMatrix * gl_Vertex: before the wave
	o.vd = mvpos / length(mvpos);
	o.vpos = (pu.mv * vec4f(position, 1.0)).xyz;
	return o;
}
fn rmatrix(rad: f32) -> mat2x2f {
	return mat2x2f(vec2f(cos(rad), -sin(rad)), vec2f(sin(rad), cos(rad)));
}
fn noise(c: vec2f) -> f32 { return textureSampleLevel(noisetex, linRep, c, 0.0).x; }
fn calcWaves(coordIn: vec2f, iswater: f32) -> f32 {
	let movement = abs(vec2f(0.0, -frameTimeCounter() * 0.31365 * iswater));
	let coord = coordIn * 0.262144;
	var coord0 = coord * rmatrix(1.0) - movement * 4.0;
	coord0.y *= 3.0;
	var coord1 = coord * rmatrix(0.5) - movement * 1.5;
	coord1.y *= 3.0;
	var coord2 = coord + movement * 0.5;
	coord2.y *= 3.0;
	var wave = 1.0 - noise(coord0 * 0.005) * 10.0;
	wave += noise(coord1 * 0.010416) * 7.0;
	wave += sqrt(noise(coord2 * 0.045) * 6.5) * 1.33;
	wave *= 0.0157;
	return wave;
}
fn calcBump(coord: vec2f, iswater: f32) -> vec3f {
	let deltaPos = vec2f(0.25, 0.0);
	let h0 = calcWaves(coord, iswater);
	let h1 = calcWaves(coord + deltaPos.xy, iswater);
	let h2 = calcWaves(coord - deltaPos.xy, iswater);
	let h3 = calcWaves(coord + deltaPos.yx, iswater);
	let h4 = calcWaves(coord - deltaPos.yx, iswater);
	let xDelta = ((h1 - h0) + (h0 - h2));
	let yDelta = ((h3 - h0) + (h0 - h4));
	return vec3f(vec2f(xDelta, yDelta) * 0.5, 0.5);
}
struct WOut {
	@location(2) c2: vec4f,
	@location(3) c5: vec4f,
	@location(4) c6: vec4f,
}
@fragment
fn fs(i: WV) -> WOut {
	// tbnMatrix from ShadersTess: n the quad normal, t along +u, the
	// binormal cross(t, n) signed toward +v.
	let dp1 = dpdx(i.vpos);
	let dp2 = dpdy(i.vpos);
	let duv1 = dpdx(i.uv);
	let duv2 = dpdy(i.uv);
	let n = normalize(cross(dp2, dp1));
	let det = duv1.x * duv2.y - duv2.x * duv1.y;
	let dPdu = (dp1 * duv2.y - dp2 * duv1.y) / det;
	let dPdv = (dp2 * duv1.x - dp1 * duv2.x) / det;
	let tg = normalize(dPdu);
	let cr = cross(tg, n);
	let bs = select(1.0, -1.0, dot(dPdv, cr) < 0.0); // a let: naga's MSL drops the parens of a select operand
	let bn = normalize(cr * bs);
	let viewVector = vec2f(dot(tg, i.vd), dot(bn, i.vd)) * 8.25;
	var albedo = textureSample(tex, samp, i.uv, i.layer) * i.color;

	let iswater = clamp(i.amb.a * 2.0 - 1.0, 0.0, 1.0);
	albedo = vec4f(pow(albedo.rgb, vec3f(2.2)), albedo.a);
	let texvis = 0.11;
	if (iswater > 0.9) { albedo = vec4f(0.0, 0.175, 0.2, albedo.a); }
	var waterpos = i.worldpos;
	// calcParallax
	let getwave = calcWaves(waterpos.xz - waterpos.y, iswater);
	waterpos = vec3f(waterpos.x + getwave * viewVector.x, waterpos.y, waterpos.z + getwave * viewVector.y);
	let bump = calcBump(waterpos.xz - waterpos.y, iswater);
	let newnormal = normalize(bump.x * tg + bump.y * bn + bump.z * n);
	var NdotL = i.diffuse;
	let sp = i.shadowPos;
	if (NdotL > 0.001 && (abs(sp.x) < 1.0 - 1.5 / shadowMapResolution && abs(sp.y) < 1.0 - 1.5 / shadowMapResolution && abs(sp.z) < 6.0) && rainStrength() < 0.9) {
		NdotL *= shadow2D(sp);
	}
	NdotL *= (1.0 - iswater);
	NdotL *= (1.0 - rainStrength());
	let sunlight = i.sunl * NdotL * (1.0 - rainStrength() * 0.99) * 0.85;
	let fColor = albedo.rgb * (sunlight + i.amb.rgb);
	var alpha = mix(albedo.a, texvis, iswater);
	if (iswater > 0.9) { alpha *= 6.0; }
	// encode
	var vis = i.lm.y;
	let a = i.amb.a;
	if (a > 0.9) { vis = vis * 0.25; }
	if (a > 0.4 && a < 0.6) { vis = vis * 0.25 + 0.25; }
	if (a < 0.1) { vis = vis * 0.25 + 0.5; }
	var o: WOut;
	o.c5 = vec4f(fColor, alpha);
	o.c2 = vec4f(encodeNormal(newnormal), vis, 1.0);
	o.c6 = vec4f(normalize(albedo.rgb + 0.00001), alpha);
	return o;
}
`

// (naga mistakes a call to a function named texture* for a texture
// type constructor.)
// packTexturedLightWGSL is the lighting of gbuffers_textured.vsh and the
// shadowed forward shading of its fsh.
const packTexturedLightWGSL = `
fn toDTextured() -> array<vec3f, 7> { return array<vec3f, 7>(vec3f(0.58597, 0.15, 0.02), vec3f(0.58597, 0.35, 0.09), vec3f(0.58597, 0.5, 0.26),
	vec3f(0.58597, 0.5, 0.35), vec3f(0.58597, 0.5, 0.36), vec3f(0.58597, 0.5, 0.37), vec3f(0.58597, 0.5, 0.38)); }
struct TexLight { amb: vec3f, sun: vec3f, skyL: f32 }
fn lightTextured(normal: vec3f, lmcoord: vec2f, emissiveMul: f32) -> TexLight {
	var torch_lightmap = 16.0 - min(15.0, (lmcoord.x - 0.5 / 16.0) * 16.0 * 16.0 / 15.0);
	let fallof1 = clamp(1.0 - pow(torch_lightmap / 16.0, 4.0), 0.0, 1.0);
	torch_lightmap = fallof1 * fallof1 / (torch_lightmap * torch_lightmap + 1.0);
	let emissiveLightC = vec3f(1.5, 0.42, 0.045) * torch_lightmap * emissiveMul;
	let finalminlight = select(0.002, 0.075, pu.eye.x > 0.9);
	let rain = rainStrength();
	var sunlight = todMix(toDTextured());
	sunlight = mix(sunlight, vec3f(0.01) * sunlight, rain);
	let moonlight = vec3f(0.0024, 0.00432, 0.0078);
	let sunVec = normalize(pu.sunPos.xyz);
	let upVec = normalize(pu.upPos.xyz);
	var visibility = vec2f(dot(sunVec, upVec), dot(-sunVec, upVec));
	let NdotL = dot(normal, sunVec);
	let NdotU = dot(normal, upVec);
	let tr = trOf(23000.0);
	visibility = pow(clamp(visibility + 0.15, vec2f(0.0), vec2f(0.15)) / 0.15, vec2f(4.0));
	let skyL = max(lmcoord.y - 2.0 / 16.0, 0.0) * 1.14285714286;
	let SkyL2 = skyL * skyL;
	let skyc2 = mix(1.0, SkyL2, skyL);
	var bounced = vec4f(NdotL, NdotL, NdotL, NdotU) * vec4f(-0.14 * skyL * skyL, 0.34, 0.7, 0.1) + vec4f(0.6, 0.66, 0.7, 0.25);
	bounced *= vec4f(skyc2, skyc2, visibility.x - tr * visibility.x, 0.8);
	let sun_ambient = bounced.w * (vec3f(0.1, 0.5, 1.1) + rain * vec3f(0.05, -0.27, -0.8)) * 2.3 + 1.7 * sunlight * (sqrt(bounced.w) * bounced.x * 2.4 + bounced.z) * (1.0 - rain * 0.98);
	let moon_ambient = (moonlight * 0.7 + moonlight * bounced.y) * (1.0 - rain * 0.95) * 2.0;
	let amb1 = (sun_ambient * visibility.x + moon_ambient * visibility.y) * SkyL2 * (0.03 + tr * 0.17) * 0.65;
	var o: TexLight;
	o.amb = amb1 + emissiveLightC + finalminlight * 0.5 * min(skyL + 6.0 / 16.0, 9.0 / 16.0) * normalize(amb1 + 0.0001) * 3.0;
	sunlight = mix(sunlight, moonlight * (1.0 - rain * 0.9), visibility.y);
	o.sun = sunlight * 0.9;
	o.skyL = skyL;
	return o;
}
// spidereyes.vsh: ambientNdotL.
fn spiderLight() -> vec3f {
	let finalminlight = select((0.002 + 0.006) * 10.0, 0.025, pu.eye.x > 0.9);
	return vec3f(0.5, 0.0, 1.0) + finalminlight;
}
struct TOut {
	@location(3) c5: vec4f,
	@location(4) c6: vec4f,
}
// fragTextured is gbuffers_textured.fsh for albedo (texture * colour),
// the view normal and the vertex lighting.
fn fragTextured(albedo: vec4f, normal: vec3f, fragCoord: vec4f, sun: vec3f, amb: vec3f, skyL: f32) -> TOut {
	var diffuse = clamp(dot(normalize(pu.shadowLightPos.xyz), normal), 0.0, 1.0);
	let rain = rainStrength();
	if (diffuse > 0.0 && rain < 0.9 && albedo.a > 0.01) {
		let fc = glFrag(fragCoord);
		let fragposition = pu.projInv * (vec4f(fc / pu.view.xy, fragCoord.z, 1.0) * 2.0 - 1.0);
		let fp = fragposition.xyz / fragposition.w;
		var wp = m3(pu.mvInv) * fp + pu.mvInv[3].xyz;
		wp = m3(pu.shadowMV) * wp + pu.shadowMV[3].xyz;
		wp = vec3f(pu.shadowProj[0].x, pu.shadowProj[1].y, pu.shadowProj[2].z) * wp + pu.shadowProj[3].xyz;
		let distortion = calcDistortion(wp.xy);
		let threshMul = max(2048.0 / shadowMapResolution * shadowDistance / 128.0, 0.95);
		let distortThresh = (sqrt(1.0 - diffuse * diffuse) / diffuse + 0.7) / distortion;
		let bias = distortThresh / 6000.0 * threshMul;
		wp = vec3f(wp.xy * distortion, wp.z);
		wp = wp * vec3f(0.5, 0.5, 0.5 / 6.0) + vec3f(0.5, 0.5, 0.5);
		wp.z -= bias;
		diffuse *= shadow2D(wp);
		diffuse *= (1.0 - rain);
		diffuse *= mix(skyL, 1.0, clamp((eyeSmoothY() / 255.0 - 2.0 / 16.0) * 4.0, 0.0, 1.0));
	}
	let finalColor = pow(albedo.rgb, vec3f(2.2)) * (sun * diffuse + amb);
	var o: TOut;
	o.c5 = vec4f(finalColor, albedo.a);
	o.c6 = vec4f(normalize(albedo.rgb + 0.00001), albedo.a);
	return o;
}
`

// packModelWGSL is modelWGSL's vertex input and uniforms drawn with
// gbuffers_textured (gbuffers_entities falls back to it), or with
// gbuffers_spidereyes for an emissive instance. light.w carries the
// light levels (sky + 16*block). HAND_FLAG is replaced per variant: hand
// draws are already in view space.
var packModelWGSL = `
struct U {
	viewProj: mat4x4f,
	model: mat4x4f,
	light: vec4f,
	fogColor: vec4f,
	fog: vec4f,
	color: vec4f,
	overlay: vec4f,
	bones: array<mat4x4f, ` + maxBonesWGSL + `>,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
const HAND = HAND_FLAG;
` + packCommonWGSL + packGroupWGSL + packTexturedLightWGSL + `
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) normal: vec3f,
	@location(2) amb: vec3f,
	@location(3) sun: vec3f,
	@location(4) skyL: f32,
}
@vertex
fn vs(@location(0) pos: vec3f, @location(1) normal: vec3f, @location(2) uv: vec2f,
	@location(3) j: vec4<u32>, @location(4) w: vec4f) -> VOut {
	let skin = u.bones[j.x] * w.x + u.bones[j.y] * w.y + u.bones[j.z] * w.z + u.bones[j.w] * w.w;
	let wp = u.model * skin * vec4f(pos, 1.0);
	var o: VOut;
	o.pos = jitter(u.viewProj * wp);
	if (HAND) { o.pos.z = o.pos.z * 0.1; } // in front of the world (see packHand)
	var n = normalize(m3(u.model) * m3(skin) * normal);
	if (!HAND) { n = normalize(m3(pu.mv) * n); }
	o.normal = n;
	o.uv = uv;
	let lv = u32(u.light.w);
	var tl: TexLight = lightTextured(n, lmOf(f32(lv >> 4u), f32(lv & 15u)), 1.0);
	o.amb = tl.amb;
	o.sun = tl.sun;
	o.skyL = tl.skyL;
	if (u.fog.w > 0.5) { o.amb = spiderLight(); }
	return o;
}
@fragment
fn fs(i: VOut) -> TOut {
	var c = textureSample(tex, samp, i.uv) * u.color;
	if (c.a <= u.fog.z) { discard; }
	let fade = u.fogColor.a;
	if (fade > 0.0 && fract(52.9829189 * fract(dot(i.pos.xy, vec2f(0.06711056, 0.00583715)))) < fade) { discard; }
	if (u.fog.w > 0.5) { // gbuffers_spidereyes
		var o: TOut;
		o.c5 = vec4f(pow(c.rgb, vec3f(2.2)) * i.amb, c.a);
		o.c6 = vec4f(normalize(c.rgb + 0.00001), c.a);
		return o;
	}
	c = vec4f(mix(c.rgb, u.overlay.rgb, u.overlay.a), c.a); // entityColor
	return fragTextured(c, i.normal, i.pos, i.sun, i.amb, i.skyL);
}
`

// packItemWGSL is itemWGSL's input drawn with gbuffers_textured.
var packItemWGSL = `
struct U {
	mvp: mat4x4f,
	model: mat4x4f,
	light: vec4f,
	fogColor: vec4f,
	fog: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
const HAND = HAND_FLAG;
` + packCommonWGSL + packGroupWGSL + packTexturedLightWGSL + `
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) normal: vec3f,
	@location(2) amb: vec3f,
	@location(3) sun: vec3f,
	@location(4) skyL: f32,
}
@vertex
fn vs(@location(0) pos: vec3f, @location(1) uv: vec2f, @location(2) normal: vec3f) -> VOut {
	var o: VOut;
	o.pos = jitter(u.mvp * vec4f(pos, 1.0));
	if (HAND) { o.pos.z = o.pos.z * 0.1; } // in front of the world (see packHand)
	o.uv = uv;
	var n = normalize(m3(u.model) * normal);
	if (!HAND) { n = normalize(m3(pu.mv) * n); }
	o.normal = n;
	let lv = u32(u.light.w);
	var tl: TexLight = lightTextured(n, lmOf(f32(lv >> 4u), f32(lv & 15u)), 1.0);
	o.amb = tl.amb;
	o.sun = tl.sun;
	o.skyL = tl.skyL;
	return o;
}
@fragment
fn fs(i: VOut) -> TOut {
	let c = textureSample(tex, samp, i.uv, i32(u.fogColor.a));
	if (c.a < 0.5) { discard; }
	return fragTextured(c, i.normal, i.pos, i.sun, i.amb, i.skyL);
}
`

// packBlockWGSL is blockWGSL's input (a BlockMesh as an entity) drawn
// with gbuffers_textured; fogColor.w carries the light levels.
var packBlockWGSL = `
struct U {
	mvp: mat4x4f,
	model: mat4x4f,
	light: vec4f,
	fogColor: vec4f,
	fog: vec4f,
	l0: vec4f,
	l1: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
@group(1) @binding(2) var layerMap: texture_2d<u32>;
@group(1) @binding(3) var lightmap: texture_2d<f32>;
@group(1) @binding(4) var lightSamp: sampler;
const HAND = HAND_FLAG;
` + packCommonWGSL + packGroupWGSL + packTexturedLightWGSL + `
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) @interpolate(flat) layer: u32,
	@location(2) color: vec3f,
	@location(3) normal: vec3f,
	@location(4) amb: vec3f,
	@location(5) sun: vec3f,
	@location(6) skyL: f32,
}
fn faceNormal(n: u32) -> vec3f {
	var normals = array<vec3f, 7>(vec3f(0.0, -1.0, 0.0), vec3f(0.0, 1.0, 0.0), vec3f(0.0, 0.0, -1.0),
		vec3f(0.0, 0.0, 1.0), vec3f(-1.0, 0.0, 0.0), vec3f(1.0, 0.0, 0.0), vec3f(0.0, 1.0, 0.0));
	return normals[min(n, 6u)];
}
@vertex
fn vs(@location(0) d: vec4<u32>) -> VOut {
	let w0 = d.x; let w1 = d.y; let w2 = d.z;
	let coarse = vec3f(f32(w0 & 1023u), f32((w0 >> 10u) & 1023u), f32((w0 >> 20u) & 1023u));
	let fine = vec3f(f32((w1 >> 26u) & 3u), f32((w1 >> 28u) & 3u), f32(w1 >> 30u));
	let local = (coarse * 4.0 + fine) * (1.0 / 128.0) - 8.0;
	var o: VOut;
	o.pos = jitter(u.mvp * vec4f(local, 1.0));
	if (HAND) { o.pos.z = o.pos.z * 0.1; } // in front of the world (see packHand)
	o.uv = vec2f(f32((w1 >> 12u) & 31u), f32((w1 >> 17u) & 31u));
	if ((w1 & (1u << 25u)) != 0u) { o.uv = o.uv * (1.0 / 16.0); }
	o.layer = textureLoad(layerMap, vec2<i32>(i32(w1 & 4095u), 0), 0).r;
	o.color = vec3f(f32(w2 & 255u), f32((w2 >> 8u) & 255u), f32((w2 >> 16u) & 255u)) * (1.0 / 255.0);
	var n = normalize(m3(u.model) * faceNormal((w1 >> 22u) & 7u));
	if (!HAND) { n = normalize(m3(pu.mv) * n); }
	o.normal = n;
	let lv = u32(u.fogColor.w);
	var tl: TexLight = lightTextured(n, lmOf(f32(lv >> 4u), f32(lv & 15u)), 1.0);
	o.amb = tl.amb;
	o.sun = tl.sun;
	o.skyL = tl.skyL;
	return o;
}
@fragment
fn fs(i: VOut) -> TOut {
	let c = textureSample(tex, samp, i.uv, i.layer) * vec4f(i.color, 1.0);
	if (c.a <= 0.1) { discard; }
	return fragTextured(c, i.normal, i.pos, i.sun, i.amb, i.skyL);
}
`

// packParticleWGSL is particleWGSL's input drawn with gbuffers_textured
// (textured_lit falls back to it). sprite.y carries the sheet flag plus
// 2*(sky + 16*block) light levels.
var packParticleWGSL = `
struct U {
	viewProj: mat4x4f,
	right: vec4f,
	up: vec4f,
	params: vec4f,
	fogColor: vec4f,
	fog: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var atlas: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
` + packCommonWGSL + packGroupWGSL + packTexturedLightWGSL + `
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) color: vec4f,
	@location(1) uv: vec2f,
	@location(2) sprite: f32,
	@location(3) amb: vec3f,
	@location(4) sun: vec3f,
	@location(5) skyL: f32,
}
@vertex
fn vs(@location(0) corner: vec2f, @location(1) ipos: vec3f, @location(2) size: f32,
	@location(3) color: vec4f, @location(4) sprite: vec2f, @location(5) rect: vec4f) -> VOut {
	let p = ipos + (u.right.xyz * corner.x + u.up.xyz * corner.y) * size;
	var o: VOut;
	o.pos = jitter(u.viewProj * vec4f(p, 1.0));
	o.color = color;
	let k = vec2f(corner.x + 0.5, 0.5 - corner.y);
	let cell = max(sprite.x - 1.0, 0.0);
	let c0 = vec2f(cell % 16.0, floor(cell / 16.0)) / 16.0;
	o.uv = c0 + k * 0.0624375;
	o.sprite = sprite.x;
	let code = u32(sprite.y);
	if ((code & 1u) != 0u) {
		o.uv = mix(rect.xy, rect.zw, k);
		o.sprite = 1.0;
	}
	let lv = code >> 1u;
	// No glNormal is set for particles: the last one, facing the eye.
	var tl: TexLight = lightTextured(vec3f(0.0, 0.0, 1.0), lmOf(f32(lv >> 4u), f32(lv & 15u)), 1.0);
	o.amb = tl.amb;
	o.sun = tl.sun;
	o.skyL = tl.skyL;
	return o;
}
@fragment
fn fs(i: VOut) -> TOut {
	var c = i.color;
	let t = textureSample(atlas, samp, i.uv);
	if (i.sprite > 0.5) { c = c * t; }
	if (c.a <= 0.003921569) { discard; }
	return fragTextured(c, vec3f(0.0, 0.0, 1.0), i.pos, i.sun, i.amb, i.skyL);
}
`

// packWeatherWGSL is skyListWGSL's input for the rain and snow, drawn with
// gbuffers_weather (DRAWBUFFERS:7). The vertex colour carries (1, lmcoord.s,
// 1, alpha) in pack mode.
var packWeatherWGSL = `
struct U {
	mvp: mat4x4f,
	model: mat4x4f,
	fogColor: vec4f,
	fog: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d<f32>;
@group(1) @binding(1) var samp: sampler;
` + packCommonWGSL + packGroupWGSL + `
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) color: vec4f,
}
@vertex
fn vs(@location(0) pos: vec3f, @location(1) uv: vec2f, @location(2) color: vec4f) -> VOut {
	var o: VOut;
	o.pos = u.mvp * vec4f(pos, 1.0);
	o.uv = uv;
	o.color = color;
	return o;
}
struct WOut { @location(5) c7: vec4f }
@fragment
fn fs(i: VOut) -> WOut {
	let t = textureSample(tex, samp, i.uv) * vec4f(1.0, 1.0, 1.0, i.color.a);
	if (u.fog.w > 0.0 && t.a <= u.fog.w) { discard; }
	var o: WOut;
	o.c7 = vec4f(vec3f(1.0, i.color.g, 1.0), t.a * length(t.rgb) / 1.732);
	return o;
}
`

// packBasicWGSL is lineWGSL's input (the block outline) drawn with
// gbuffers_basic (DRAWBUFFERS:0).
var packBasicWGSL = `
struct U { mvp: mat4x4f, color: vec4f }
@group(0) @binding(0) var<uniform> u: U;
` + packCommonWGSL + packGroupWGSL + `
@vertex
fn vs(@location(0) p: vec3f) -> @builtin(position) vec4f { return u.mvp * vec4f(p, 1.0); }
@fragment
fn fs(@builtin(position) pos: vec4f) -> @location(0) vec4f {
	var c = vec4f(ycocg(u.color.rgb), u.color.a);
	if (pattern(glFrag(pos))) { c.g = c.b; }
	c.b = 1.0;
	return c;
}
`

// packShadowWGSL is shadow.vsh/fsh for terrain: the distorted shadow
// clip position, the texture colour (water white) to shadowcolor0.
var packShadowWGSL = terrainHeaderWGSL + packCommonWGSL + packGroupWGSL + packTerrainDecodeWGSL + `
struct SV {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) @interpolate(flat) layer: u32,
	@location(2) @interpolate(flat) water: u32,
}
fn calcShadowDistortion(shadowpos: vec2f) -> vec2f {
	let distortion = log(length(shadowpos) * shadowB + shadowA) * shadowK;
	return shadowpos / distortion;
}
@vertex
fn vs(@location(0) d: vec4<u32>) -> SV {
	let t = decodeTerrain(d);
	var p = pu.shadowProj * pu.shadowMV * vec4f(t.pos, 1.0);
	p = vec4f(calcShadowDistortion(p.xy), p.z / 6.0, p.w);
	var o: SV;
	o.pos = toWGPU(p);
	o.uv = t.uv;
	o.layer = t.layer;
	o.water = select(0u, 1u, t.mat == 8u);
	return o;
}
@fragment
fn fs(i: SV) -> @location(0) vec4f {
	var color = textureSample(tex, samp, i.uv, i.layer);
	if (color.a <= 0.1) { discard; }
	if (i.water == 1u) { color = vec4f(1.0, 1.0, 1.0, color.a); }
	return color;
}
`

// packCompositeHeadWGSL declares the composite passes' bindings (one
// layout; each pass binds the ping-pong side it reads) and the
// fullscreen triangle with GL texcoord.
var packCompositeHeadWGSL = packCommonWGSL + `
@group(0) @binding(0) var<uniform> pu: PackU;
@group(0) @binding(1) var colortex0: texture_2d<f32>;
@group(0) @binding(2) var colortex1: texture_2d<f32>;
@group(0) @binding(3) var colortex2: texture_2d<f32>;
@group(0) @binding(4) var colortex3: texture_2d<f32>;
@group(0) @binding(5) var colortex4: texture_2d<f32>;
@group(0) @binding(6) var colortex5: texture_2d<f32>;
@group(0) @binding(7) var colortex6: texture_2d<f32>;
@group(0) @binding(8) var colortex7: texture_2d<f32>;
@group(0) @binding(9) var depthtex0: texture_depth_2d;
@group(0) @binding(10) var depthtex1: texture_depth_2d;
@group(0) @binding(11) var depthtex2: texture_depth_2d;
@group(0) @binding(12) var shadowtex0: texture_depth_2d;
@group(0) @binding(13) var shadowtex1: texture_depth_2d;
@group(0) @binding(14) var shadowcolor0: texture_2d<f32>;
@group(0) @binding(15) var noisetex: texture_2d<f32>;
@group(0) @binding(16) var linClp: sampler;
@group(0) @binding(17) var linRep: sampler;
@group(0) @binding(18) var shadowCmp: sampler_comparison;

struct CV { @builtin(position) pos: vec4f, @location(0) tc: vec2f }
@vertex
fn vs(@builtin(vertex_index) id: u32) -> CV {
	let p = vec2f(f32((id << 1u) & 2u), f32(id & 2u));
	var o: CV;
	o.tc = p;
	o.pos = vec4f(p * 2.0 - 1.0, 0.0, 1.0);
	return o;
}
// texture2D on a render target with GL coordinates (linear, clamped).
fn tex2D(t: texture_2d<f32>, uv: vec2f) -> vec4f { return textureSampleLevel(t, linClp, glUV(uv), 0.0); }
fn tex2DLod(t: texture_2d<f32>, uv: vec2f, lod: f32) -> vec4f { return textureSampleLevel(t, linClp, glUV(uv), lod); }
// texture2D on a depth texture (GL_NEAREST).
fn depthAt(t: texture_depth_2d, uv: vec2f) -> f32 {
	let s = vec2i(textureDimensions(t));
	let p = clamp(vec2i(floor(glUV(uv) * vec2f(s))), vec2i(0), s - 1);
	return textureLoad(t, p, 0);
}
fn shadow2Dt(t: texture_depth_2d, p: vec3f) -> f32 { return textureSampleCompareLevel(t, shadowCmp, glUV(p.xy), p.z); }
fn noiseAt(c: vec2f) -> f32 { return textureSampleLevel(noisetex, linRep, c, 0.0).x; }
fn toScreenSpace(p: vec3f) -> vec3f {
	let iProjDiag = vec4f(pu.projInv[0].x, pu.projInv[1].y, pu.projInv[2].zw);
	let p3 = p * 2.0 - 1.0;
	let fragposition = iProjDiag * p3.xyzz + pu.projInv[3];
	return fragposition.xyz / fragposition.w;
}
fn YCoCg2RGB(cin: vec3f) -> vec3f {
	var c = cin;
	c.y -= 0.5;
	c.z -= 0.5;
	return vec3f(c.r + c.g - c.b, c.r + c.b, c.r - c.g - c.b);
}
// lightPos of composite.vsh: vec4(sunPosition,1)*gbufferProjection.
fn lightPos() -> vec2f {
	var tpos = vec4f(pu.sunPos.xyz, 1.0) * pu.proj;
	tpos = vec4f(tpos.xyz / tpos.w, 1.0);
	let pos1 = tpos.xy / tpos.z;
	return pos1 * 0.5 + 0.5;
}
`

// packCompositeWGSL is composite.vsh/fsh (DRAWBUFFERS:34): deferred
// lighting with the shadow PCF, and the half-resolution sky and god ray
// mask.
var packCompositeWGSL = packCompositeHeadWGSL + `
fn c0ToD() -> array<vec3f, 7> { return array<vec3f, 7>(vec3f(0.58597, 0.16, 0.005), vec3f(0.58597, 0.31, 0.08), vec3f(0.58597, 0.45, 0.16),
	vec3f(0.58597, 0.5, 0.35), vec3f(0.58597, 0.5, 0.36), vec3f(0.58597, 0.5, 0.37), vec3f(0.58597, 0.5, 0.38)); }
struct C0V {
	lightPos: vec2f, sunVec: vec3f, upVec: vec3f, sky1: vec3f, sky2: vec3f, tr: f32,
	sunlight: vec3f, nsunlight: vec3f, rawAvg: vec3f, SdotU: f32, sunVisibility: f32, moonVisibility: f32,
}
fn c0vars() -> C0V {
	var o: C0V;
	let rain = rainStrength();
	o.lightPos = lightPos();
	o.sunVec = normalize(pu.sunPos.xyz);
	o.upVec = normalize(pu.upPos.xyz);
	o.SdotU = dot(o.sunVec, o.upVec);
	o.sunVisibility = pow(clamp(o.SdotU + 0.15, 0.0, 0.15) / 0.15, 4.0);
	o.moonVisibility = pow(clamp(-o.SdotU + 0.15, 0.0, 0.15) / 0.15, 4.0);
	o.sunlight = todMix(c0ToD());
	o.tr = trOf(23000.0);
	let lt = lotrTime();
	let tr2 = clamp(min(min(abs(lt - 23000.0), 750.0), min(abs(lt - 12700.0), 800.0)) / 800.0 - 0.5, 0.0, 1.0) * 2.0;
	let bounced = vec4f(0.5, 0.66, 1.3, 0.27);
	let sun_ambient = bounced.w * (vec3f(0.25, 0.62, 1.32) - rain * vec3f(0.1, 0.47, 1.17)) * (1.0 + rain * 7.0) + o.sunlight * (bounced.x + bounced.z) * (1.0 - rain * 0.95);
	let moonlight = vec3f(0.0016, 0.00288, 0.00448);
	o.rawAvg = (sun_ambient * o.sunVisibility + 8.0 * moonlight * o.moonVisibility) * (0.05 + tr2 * 0.15) * 4.7 + 0.0002;
	let mcosS = max(o.SdotU, 0.0);
	let sunlight04 = pow(o.sunlight, vec3f(0.454));
	let skyMult = max(o.SdotU * 0.1 + 0.1, 0.0) / 0.2 * (1.0 - rain * 0.6) * 0.7;
	o.nsunlight = normalize(pow(mix(sunlight04, 5.0 * sunlight04 * o.sunVisibility * (1.0 - rain * 0.95) + vec3f(0.3, 0.3, 0.35), rain), vec3f(2.2))) * 0.6 * skyMult;
	var sky_color = vec3f(0.05, 0.32, 1.0);
	sky_color = normalize(mix(sky_color, 2.0 * sunlight04 * o.sunVisibility * (1.0 - rain * 0.95) + vec3f(0.3, 0.3, 0.3) * length(sunlight04), rain));
	o.sky1 = sky_color * 0.6 * skyMult;
	o.sky2 = mix(sky_color, mix(o.nsunlight, sky_color, rain * 0.9), 1.0 - max(mcosS - 0.2, 0.0) * 0.5) * 0.6 * skyMult;
	return o;
}
fn tapLocation(sampleNumber: i32, nb: i32, jit: f32) -> vec2f {
	let alpha = (f32(sampleNumber) + jit) / f32(nb);
	let angle = jit * 6.28 + alpha * 4.0 * 6.28;
	return vec2f(cos(angle), sin(angle)) * sqrt(alpha);
}
fn cdist(coord: vec2f) -> f32 {
	let v = abs(coord * 2.0 - 1.0);
	let d = max(v.x, v.y);
	return 1.0 - d * d;
}
fn getnoise(pos: vec2f) -> f32 {
	return fract(sin(dot(pos, vec2f(18.9898, 28.633))) * 4378.5453);
}
fn getSkyColor(fposition: vec3f, v: C0V) -> vec3f {
	let moonlightS = vec3f(0.00575, 0.0105, 0.014);
	let rain = rainStrength();
	let sVector = normalize(fposition);
	let invRain07 = 1.0 - rain * 0.4;
	let cosT = dot(sVector, v.upVec);
	let mCosT = max(cosT, 0.0);
	let absCosT = 1.0 - max(cosT * 0.82 + 0.26, 0.2);
	let cosY = dot(v.sunVec, sVector);
	let Y = acos(cosY);
	let a = -1.0;
	let b = -0.22;
	let c = 8.0;
	let d = -3.5;
	let e = 0.3;
	let L = (1.0 + a * exp(b / (mCosT)));
	let A = 1.0 + e * cosY * cosY;
	let grad1 = mix(v.sky1, v.sky2, absCosT * absCosT);
	let sunscat = max(cosY, 0.0);
	let grad3 = mix(grad1, v.nsunlight * (1.0 - isEyeInWater()), sunscat * sunscat * (1.0 - mCosT) * (0.9 - rain * 0.5 * 0.9) * (clamp(-(v.SdotU) * 4.0 + 3.0, 0.0, 1.0) * 0.65 + 0.35) + 0.1);
	let Y2 = 3.14159265359 - Y;
	let L2 = L * (8.0 * exp(d * Y2) + A);
	let moonlight2 = pow(normalize(moonlightS), vec3f(3.0)) * length(moonlightS);
	let moonlightRain = normalize(vec3f(0.25, 0.3, 0.4)) * length(moonlightS);
	var gradN = mix(moonlightS, moonlight2, 1.0 - L2 / 2.0);
	gradN = mix(gradN, moonlightRain, rain);
	return pow(L * (c * exp(d * Y) + A), invRain07) * v.sunVisibility * length(v.rawAvg) * (0.85 + rain * 0.425) * grad3 + 0.2 * pow(L2 * 1.2 + 1.2, invRain07) * v.moonVisibility * gradN;
}
struct C0Out {
	@location(0) c3: vec4f,
	@location(1) c4: vec4f,
}
@fragment
fn fs(i: CV) -> C0Out {
	let v = c0vars();
	let texcoord = i.tc;
	let fc = glFrag(i.pos);
	let rain = rainStrength();
	let ts = texelSize();
	// hr reads colortex3, cleared each frame and written only by
	// gbuffers_skytextured (with 0): it is 0.
	let hr = vec4f(0.0);
	let Depth = depthAt(depthtex1, texcoord);
	let albedo = tex2D(colortex0, texcoord);
	let land = !(dot(albedo.rgb, vec3f(1.0)) < 0.00000000001 || (Depth > comp()));
	let translucent = albedo.b > 0.69 && albedo.b < 0.71;
	let emissive = albedo.b > 0.59 && albedo.b < 0.61;
	var color = vec3f(albedo.rg, 0.0);
	if (land && dot(albedo.rgb, vec3f(1.0)) > 0.00000000001) {
		let a0 = tex2D(colortex0, texcoord + vec2f(ts.x, 0.0)).rg;
		let a1 = tex2D(colortex0, texcoord - vec2f(ts.x, 0.0)).rg;
		let a2 = tex2D(colortex0, texcoord + vec2f(0.0, ts.y)).rg;
		let a3 = tex2D(colortex0, texcoord - vec2f(0.0, ts.y)).rg;
		let lumas = vec4f(a0.x, a1.x, a2.x, a3.x);
		let chromas = vec4f(a0.y, a1.y, a2.y, a3.y);
		let THRESH = vec4f(30.0 / 255.0);
		var w = 1.0 - step(THRESH, abs(lumas - color.x));
		var W = dot(w, vec4f(1.0));
		w.x = select(w.x, 1.0, W == 0.0);
		W = select(W, 1.0, W == 0.0);
		color.b = dot(w, chromas) / W;
		if (pattern(fc)) { color = color.rbg; }
		color = YCoCg2RGB(color);
		color = pow(color, vec3f(2.2));
		let normal = tex2D(colortex2, texcoord).xyz;
		let iswater = normal.z < 0.2499 && dot(normal, normal) > 0.0;
		let isice = normal.z > 0.2499 && normal.z < 0.4999 && dot(normal, normal) > 0.0;
		let isnsun = (iswater || isice) || ((!iswater || !isice) && isEyeInWater() == 1.0);
		let newTC = fc * ts;
		let TAAfragpos = toScreenSpace(vec3f(newTC - taaOffset() * ts * 0.5, depthAt(depthtex1, newTC)));
		let gd = tex2D(colortex1, texcoord);
		let normalT = decodeNormal(gd.xy);
		let ao = 1.0;
		var torch_lightmap = 16.0 - min(15.0, (gd.z - 0.5 / 16.0) * 16.0 * 16.0 / 15.0);
		let fallof1 = clamp(1.0 - pow(torch_lightmap / 16.0, 4.0), 0.0, 1.0);
		torch_lightmap = fallof1 * fallof1 / (torch_lightmap * torch_lightmap + 1.0);
		let c_emitted = dot((color.rgb), vec3f(1.0, 0.6, 0.4)) / 2.0;
		let emitted = select(0.0, clamp(c_emitted * c_emitted, 0.0, 1.0) * torch_lightmap, emissive);
		let emissiveLightC = vec3f(1.5, 0.42, 0.045);
		let NdotL = dot(normalT, v.sunVec);
		let NdotU = dot(normalT, v.upVec);
		let moonlight = vec3f(0.5, 0.9, 1.8) * 0.003;
		let visibility = vec2f(v.sunVisibility, v.moonVisibility);
		let skyL = max(gd.w - 2.0 / 16.0, 0.0) * 1.14285714286;
		let SkyL2 = skyL * skyL;
		let skyc2 = mix(1.0, SkyL2, skyL);
		var bounced = vec4f(NdotL, NdotL, NdotL, NdotU) * vec4f(-0.14 * skyL * skyL, 0.33, 0.7, 0.1) + vec4f(0.6, 0.66, 0.7, 0.25);
		bounced *= vec4f(skyc2, skyc2, visibility.x - v.tr * visibility.x, 0.8);
		let sun_ambient = bounced.w * (vec3f(0.1, 0.5, 1.1) * 2.4 + rain * 2.3 * vec3f(0.05, -0.33, -0.9)) + 1.6 * v.sunlight * (sqrt(bounced.w) * bounced.x * 2.4 + bounced.z) * (1.0 - rain * 0.99);
		let moon_ambient = (moonlight * 0.7 + moonlight * bounced.y) * 4.0;
		let LightC = mix(v.sunlight, moonlight, v.moonVisibility) * (1.0 - rain * 0.99);
		let amb1 = (sun_ambient * visibility.x + moon_ambient * visibility.y) * SkyL2 * (0.03 * 0.65 + v.tr * 0.17 * 0.65);
		let finalminlight = select(0.002, 0.15, pu.eye.x > 0.9);
		let ambientC = ao * amb1 + emissiveLightC * (emitted * 15.0 * color + torch_lightmap * ao) * 0.66 + ao * finalminlight * min(skyL + 6.0 / 16.0, 9.0 / 16.0) * normalize(amb1 + 0.0001);
		let diffuse = select(clamp(dot(normalT, normalize(pu.shadowLightPos.xyz)) * 1.02 - 0.02, 0.0, 1.0), 0.75, translucent);
		var finalShading = vec3f(diffuse);
		var noise = fract(0.75487765 * fc.x + 0.56984026 * fc.y);
		noise = fract(frameTimeCounter() * 16.0 + noise);
		if (diffuse > 0.001) {
			var shadowpos = m3(pu.mvInv) * TAAfragpos.xyz + pu.mvInv[3].xyz;
			shadowpos = m3(pu.shadowMV) * shadowpos + pu.shadowMV[3].xyz;
			shadowpos = vec3f(pu.shadowProj[0].x, pu.shadowProj[1].y, pu.shadowProj[2].z) * shadowpos + pu.shadowProj[3].xyz;
			let distortion = calcDistortion(shadowpos.xy);
			shadowpos = vec3f(shadowpos.xy * distortion, shadowpos.z);
			var shading = vec2f(1.0);
			if (abs(shadowpos.x) < 1.0 - 1.5 / shadowMapResolution && abs(shadowpos.y) < 1.0 - 1.5 / shadowMapResolution && abs(shadowpos.z) < 6.0) {
				let pdepth = 1.412;
				let threshMul = max(2048.0 / shadowMapResolution * shadowDistance / 128.0, 0.95);
				let distortThresh = (sqrt(1.0 - diffuse * diffuse) / diffuse + 0.7) / distortion;
				shadowpos = shadowpos * vec3f(0.5, 0.5, 0.5 / 6.0) + vec3f(0.5, 0.5, 0.5);
				let rdMul = pdepth * distortion * 0.05 * shadowK / shadowMapResolution;
				let bias = select(distortThresh / 6000.0 * threshMul, 0.00014, translucent);
				shading = vec2f(0.0);
				for (var k = 0; k < 10; k++) {
					let offsetS = tapLocation(k, 10, noise);
					let weight = 1.0 + (f32(k) + noise) * rdMul / 10.0 * shadowMapResolution;
					let sp = shadowpos + vec3f(rdMul * offsetS, -bias * weight);
					shading.x += shadow2Dt(shadowtex0, sp) / 10.0;
					shading.y += shadow2Dt(shadowtex1, sp) / 10.0;
				}
			}
			finalShading = tex2D(shadowcolor0, shadowpos.xy).rgb * (shading.y - shading.x) + shading.x;
			finalShading *= diffuse;
			finalShading *= mix(skyL, 1.0, clamp((eyeSmoothY() / 255.0 - 2.0 / 16.0) * 4.0, 0.0, 1.0));
		}
		let nsun = select(1.0, SkyL2 * skyL, isnsun);
		let nsunA = select(1.0, 1.0 / (SkyL2 * skyL * 0.5 + 0.5), isnsun);
		color *= (finalShading * LightC * nsun * 2.15 + ambientC * nsunA * 1.4) * 0.63;
	}
	let ntc = texcoord * 2.0;
	let ntc2 = texcoord * 2.0 - 1.0;
	var c = vec3f(0.0);
	if (ntc.x < 1.0 && ntc.y < 1.0 && ntc.x > 0.0 && ntc.y > 0.0) {
		let depth1 = depthAt(depthtex1, ntc);
		let sky = 0.950 - nearP() / farP() / farP();
		if (depth1 > sky) {
			var fragpos = pu.projInv * (vec4f(ntc, depth1, 1.0) * 2.0 - 1.0);
			fragpos /= fragpos.w;
			c = getSkyColor(fragpos.xyz, v);
		}
	}
	var gr = 0.0;
	if (ntc2.x < 1.0 && ntc2.y < 1.0 && ntc2.x > 0.0 && ntc2.y > 0.0) {
		let deltatexcoord = vec2f(v.lightPos - ntc2) * 0.04;
		var noisetc = ntc2 + deltatexcoord * getnoise(ntc2) + deltatexcoord;
		for (var k = 0; k < 20; k++) {
			let gdepth1 = depthAt(depthtex0, noisetc);
			noisetc += deltatexcoord;
			gr += step(comp(), gdepth1) * cdist(noisetc);
		}
		gr /= 20.0;
	}
	if (!land) { color = hr.rgb; }
	var o: C0Out;
	o.c3 = vec4f(c / 30.0, 1.0);
	o.c4 = vec4f(pow(color / 257.0, vec3f(0.454)), gr);
	return o;
}
`

// packComposite1WGSL is composite1.vsh/fsh (DRAWBUFFERS:3): sky, sun,
// moon, stars and volumetric clouds, water refraction, caustics and
// reflections, fog, rain and god rays.
var packComposite1WGSL = packCompositeHeadWGSL + `
const redtint = 1.5;
fn c1ToD() -> array<vec3f, 7> { return array<vec3f, 7>(vec3f(redtint, 0.15, 0.02), vec3f(redtint, 0.35, 0.09), vec3f(redtint, 0.5, 0.26),
	vec3f(redtint, 0.5, 0.35), vec3f(redtint, 0.5, 0.36), vec3f(redtint, 0.5, 0.37), vec3f(redtint, 0.5, 0.38)); }
fn moonlightF() -> vec3f { return vec3f(0.0025, 0.0045, 0.007); }
struct C1V {
	lightPos: vec2f, sunVec: vec3f, upVec: vec3f, lightColor: vec3f, sky1: vec3f, sky2: vec3f,
	nsunlight: vec3f, sunlight: vec3f, rawAvg: vec3f, avgAmbient2: vec3f, cloudColor: vec3f, cloudColor2: vec3f,
	tr: f32, eyeAdapt: f32, SdotU: f32, sunVisibility: f32, moonVisibility: f32,
}
fn c1vars() -> C1V {
	var o: C1V;
	let rain = rainStrength();
	o.lightPos = lightPos();
	o.sunVec = normalize(pu.sunPos.xyz);
	o.upVec = normalize(pu.upPos.xyz);
	o.SdotU = dot(o.sunVec, o.upVec);
	o.sunVisibility = pow(clamp(o.SdotU + 0.15, 0.0, 0.15) / 0.15, 4.0);
	o.moonVisibility = pow(clamp(-o.SdotU + 0.15, 0.0, 0.15) / 0.15, 4.0);
	o.sunlight = todMix(c1ToD());
	let sunlight04 = pow(o.sunlight, vec3f(0.454));
	let eyebright = max(eyeSmoothY() / 255.0 - 0.5 / 16.0, 0.0) * 1.03225806452;
	let SkyL2 = mix(1.0, eyebright * eyebright, eyebright);
	let lt = lotrTime();
	let trCalc = min(abs(vec2f(lt) - vec2f(23050.0, 12700.0)), vec2f(750.0));
	o.tr = max(min(trCalc.x, trCalc.y) / 375.0 - 1.0, 0.0);
	// the vsh then shadows tr with a local for the rest of main()
	let tr = clamp(min(min(abs(lt - 23050.0), 750.0), min(abs(lt - 12700.0), 800.0)) / 800.0 - 0.5, 0.0, 1.0) * 2.0;
	let bounced = vec4f(0.5, 0.66, 1.3, 0.27);
	let sun_ambient = bounced.w * (vec3f(0.25, 0.62, 1.32) - rain * vec3f(0.1, 0.47, 1.17)) * (1.0 + rain * 7.0) + o.sunlight * (bounced.x + bounced.z) * (1.0 - rain * 0.95);
	let moonlight = vec3f(0.0016, 0.00288, 0.00448);
	let moon_ambient = (moonlight + moonlight * eyebright * eyebright * eyebright);
	let tr2 = max(min(trCalc.x, trCalc.y) / 375.0 - 1.0, 0.0);
	let bounced2 = vec4f(0.5 * SkyL2, 0.66 * SkyL2, 0.7, 0.3);
	let sun_ambient2 = bounced2.w * (vec3f(0.25, 0.62, 1.32) - rain * vec3f(0.11, 0.32, 1.07)) + o.sunlight * (bounced2.x + bounced2.z);
	let moon_ambient2 = (moonlight * 3.5);
	o.rawAvg = (sun_ambient * o.sunVisibility + 8.0 * moonlight * o.moonVisibility) * (0.05 + tr * 0.15) * 4.7 + 0.0002;
	var avgAmbient = (sun_ambient2 * o.sunVisibility + moon_ambient2 * o.moonVisibility) * eyebright * eyebright * (0.05 + tr2 * 0.15) * 4.7 + 0.0006;
	let avgAmbient2 = (sun_ambient * o.sunVisibility + 6.0 * moon_ambient * o.moonVisibility) * eyebright * (0.27 + tr * 0.65) + 0.0002;
	let eyeAdapt = log(clamp(luma(avgAmbient), 0.007, 80.0)) / log(2.6) * 0.35;
	o.eyeAdapt = 1.0 / pow(2.6, eyeAdapt) * 1.75;
	avgAmbient /= sqrt(3.0);
	o.avgAmbient2 = avgAmbient2 / sqrt(3.0);
	let truepos = sign(pu.sunPos.z) * 1.0;
	var lightColor = mix(o.sunlight * o.sunVisibility + 0.00001, 12.0 * moonlight * o.moonVisibility + 0.00001, (truepos + 1.0) / 2.0);
	if (length(lightColor) > 0.001) {
		lightColor = mix(lightColor, normalize(vec3f(0.3, 0.3, 0.3)) * pow(normalize(lightColor), vec3f(0.4)) * length(lightColor) * 0.03, rain) * (0.25 + 0.25 * tr);
	}
	o.lightColor = lightColor;
	let mcosS = max(o.SdotU, 0.0);
	let skyMult = max(o.SdotU * 0.1 + 0.1, 0.0) / 0.2 * (1.0 - rain * 0.6) * 0.7;
	o.nsunlight = normalize(pow(mix(sunlight04, 5.0 * sunlight04 * o.sunVisibility * (1.0 - rain * 0.95) + vec3f(0.3, 0.3, 0.35), rain), vec3f(2.2))) * 0.6 * skyMult;
	var sky_color = vec3f(0.15, 0.4, 1.0);
	sky_color = normalize(mix(sky_color, 2.0 * sunlight04 * o.sunVisibility * (1.0 - rain * 0.95) + vec3f(0.3, 0.3, 0.3) * length(sunlight04), rain));
	o.sky1 = sky_color * 0.6 * skyMult;
	o.sky2 = mix(sky_color, mix(o.nsunlight, sky_color, rain * 0.9), 1.0 - max(mcosS - 0.2, 0.0) * 0.5) * 0.6 * skyMult;
	o.cloudColor = sunlight04 * o.sunVisibility * (1.0 - rain * 0.17) * length(o.rawAvg) + o.rawAvg * 0.7 * (1.0 - rain * 0.1) + 2.0 * moonlight * o.moonVisibility * (1.0 - rain * 0.17);
	o.cloudColor2 = 0.1 * o.sunlight * o.sunVisibility * (1.0 - rain * 0.15) * length(o.rawAvg) + 1.5 * length(o.rawAvg) * mix(vec3f(0.15, 0.4, 1.0), vec3f(0.65, 0.65, 0.65), rain) * (1.0 - rain * 0.1) + 2.0 * moonlight * o.moonVisibility * (1.0 - rain * 0.15);
	return o;
}
fn tmult() -> f32 { return mix(min(abs(lotrTime() - 6000.0) / 6000.0, 1.0), 1.0, rainStrength()); }
fn drawSun(fposition: vec3f, color: vec3f, vis: f32, v: C1V) -> vec3f {
	let sVector = normalize(fposition);
	let angle = (1.0 - max(dot(sVector, v.sunVec), 0.0)) * 650.0;
	var sun = exp(-angle * angle * angle);
	sun *= (1.0 - rainStrength() * 1.0) * v.sunVisibility;
	let sunlightB = mix(pow(v.sunlight, vec3f(1.0)) * 44.0, vec3f(0.25, 0.3, 0.4), rainStrength() * 0.8);
	return mix(color, sunlightB, sun * vis);
}
fn drawMoon(fposition: vec3f, color: vec3f, vis: f32, v: C1V) -> vec3f {
	let sVector = normalize(fposition);
	let angle = (1.0 - max(dot(sVector, -v.sunVec), 0.0)) * 2000.0;
	var moon = exp(-angle * angle * angle);
	moon *= (1.0 - rainStrength() * 1.0) * v.moonVisibility;
	let moonlightC = mix(pow(moonlightF() * 40.0, vec3f(1.0)) * 44.0, vec3f(0.25, 0.3, 0.4), rainStrength() * 0.8);
	return mix(color, moonlightC, moon * vis);
}
fn getAirDensity(h: f32) -> f32 { return max(h / 10.0, 6.0); }
fn calcFog(fposition: vec3f) -> f32 {
	let rain = rainStrength();
	let density = 100.0 * (1.0 - rain * 0.115);
	let worldpos = (pu.mvInv * vec4f(fposition, 1.0)).xyz + pu.camPos.xyz;
	let height = mix(getAirDensity(worldpos.y), 6.0, rain);
	let d = length(fposition);
	let tm = tmult();
	return pow(clamp((2.625 + rain * 3.4) / exp(-60.0 / 10.0 / density) * exp(-getAirDensity(pu.camPos.y) / density) * (1.0 - exp(-pow(d, 2.712) * height / density / (6000.0 - tm * tm * 2000.0) / 13.0)) / height, 0.0, 1.0), 1.0 - rain * 0.63) * clamp((eyeSmoothY() / 255.0 - 2.0 / 16.0) * 4.0, 0.0, 1.0);
}
fn getSkyc(fposition: vec3f, v: C1V) -> vec3f {
	let rain = rainStrength();
	let sVector = normalize(fposition);
	let invRain07 = 1.0 - rain * 0.6;
	let cosT = dot(sVector, v.upVec);
	let mCosT = max(cosT, 0.0);
	let absCosT = 1.0 - max(cosT * 0.82 + 0.26, 0.2);
	let cosY = dot(v.sunVec, sVector);
	let Y = acos(cosY);
	let a = -1.0;
	let b = -0.22;
	let c = 8.0;
	let d = -3.5;
	let e = 0.3;
	let L = (1.0 + a * exp(b / (mCosT)));
	let A = 1.0 + e * cosY * cosY;
	let grad1 = mix(v.sky1, v.sky2, absCosT * absCosT);
	let sunscat = max(cosY, 0.0);
	let grad3 = mix(grad1, v.nsunlight, sunscat * sunscat * (1.0 - mCosT) * (0.9 - rain * 0.5 * 0.9) * (clamp(-(v.SdotU) * 4.0 + 3.0, 0.0, 1.0) * 0.65 + 0.35) + 0.1);
	let Y2 = 3.14159265359 - Y;
	let L2 = L * (8.0 * exp(d * Y2) + A);
	let moonlight2 = pow(normalize(moonlightF()), vec3f(3.0)) * length(moonlightF());
	let moonlightRain = normalize(vec3f(0.25, 0.3, 0.4)) * length(moonlightF());
	var gradN = mix(moonlightF(), moonlight2, 1.0 - L2 / 2.0);
	gradN = mix(gradN, moonlightRain, rain);
	return pow(L * (c * exp(d * Y) + A), invRain07) * v.sunVisibility * length(v.rawAvg) * (0.85 + rain * 0.425) * grad3 + 0.2 * pow(L2 * 1.2 + 1.2, invRain07) * v.moonVisibility * gradN;
}
fn nvec3(pos: vec4f) -> vec3f { return pos.xyz / pos.w; }
fn cdist(coord: vec2f) -> f32 { return max(abs(coord.x - 0.5), abs(coord.y - 0.5)) * 2.0; }
fn raytrace(fragposIn: vec3f, skycolor: vec3f, rvectorIn: vec3f, v: C1V) -> vec4f {
	let maxf = 3;
	let ref_ = 0.11;
	let inc = 3.0;
	var color = vec4f(0.0);
	let start = fragposIn;
	var rvector = rvectorIn * 1.2;
	var fragpos = fragposIn + rvector;
	var tvector = rvector;
	var sr = 0;
	for (var i = 0; i < 25; i++) {
		let pos = nvec3(pu.proj * vec4f(fragpos, 1.0)) * 0.5 + 0.5;
		if (pos.x < 0.0 || pos.x > 1.0 || pos.y < 0.0 || pos.y > 1.0 || pos.z < 0.0 || pos.z > 1.0) { break; }
		var fragpos0 = vec3f(pos.xy, depthAt(depthtex1, pos.xy));
		fragpos0 = nvec3(pu.projInv * vec4f(fragpos0 * 2.0 - 1.0, 1.0));
		let err = distance(fragpos, fragpos0);
		if (err < pow(length(rvector), 1.175)) {
			sr++;
			if (sr >= maxf) {
				let land = depthAt(depthtex1, pos.xy) < comp();
				color = pow(tex2DLod(colortex4, pos.xy, 1.0), vec4f(2.2)) * 257.0;
				if (isEyeInWater() == 0.0) {
					if (land) {
						color = vec4f(mix(color.rgb, skycolor * (0.7 + 0.3 * tmult()) * (1.33 - rainStrength() * 0.8), calcFog(fragpos0.xyz)), color.a);
					} else {
						color = vec4f(drawSun(rvector, skycolor, 1.0, v), color.a);
					}
				}
				color.a = clamp(1.0 - pow(cdist(pos.xy), 20.0), 0.0, 1.0);
				break;
			}
			tvector -= rvector;
			rvector *= ref_;
		}
		rvector *= inc;
		tvector += rvector;
		fragpos = start + tvector;
	}
	return color;
}
const maxHeight = 456.0; // 256.0 + 200.0
fn densityAtPos(posIn: vec3f) -> f32 {
	var pos = posIn / 18.0;
	pos = vec3f(pos.x * 0.5, pos.y, pos.z * 0.5);
	let p = floor(pos);
	var f = fract(pos);
	f = (f * f) * (3.0 - 2.0 * f);
	f = sqrt(f);
	let uv = p.xz + f.xz + p.y * 17.0;
	let coord = uv / 64.0;
	let coord2 = uv / 64.0 + 17.0 / 64.0;
	let xy1 = noiseAt(coord);
	let xy2 = noiseAt(coord2);
	return mix(xy1, xy2, f.y);
}
fn cloudPlane(pos: vec3f) -> f32 {
	let center = 256.0 * 0.5 + maxHeight * 0.5;
	let difcenter = maxHeight - center;
	let mult = (pos.y - center) / difcenter;
	let ftc = frameTimeCounter();
	let samplePos = pos * vec3f(0.5, 0.5, 0.5) * 0.35 + ftc * vec3f(0.5, 0.0, 0.5);
	var noise = 0.0;
	var tot = 0.0;
	for (var i = 0; i < 4; i++) {
		let fi = f32(i);
		noise += densityAtPos(samplePos * exp(fi * 1.05) * 0.6 + ftc * fi * vec3f(0.5, 0.0, 0.5) * 0.6) * exp(-fi * 0.8);
		tot += exp(-fi * 0.8);
	}
	return 1.0 - pow(0.4, max(noise / tot - 0.56 - mult * mult * 0.3 + rainStrength() * 0.16, 0.0) * 2.2);
}
fn renderClouds(posIn: vec3f, colorIn: vec3f, cloudIT: i32, fc: vec2f, v: C1V) -> vec3f {
	var color = colorIn;
	var dither = fract(0.75487765 * fc.x + 0.56984026 * fc.y);
	dither = fract(frameTimeCounter() * 256.0 + dither);
	var dV_view = posIn;
	var progress_view = vec3f(0.0);
	let pos = posIn * 2200.0 + pu.camPos.xyz;
	let cam = pu.camPos.xyz;
	let it = f32(cloudIT);
	if (cam.y <= 256.0) {
		let maxHeight2 = min(maxHeight, pos.y);
		dV_view *= -(maxHeight2 - 256.0) / dV_view.y / it;
		progress_view = dV_view * dither + cam + dV_view * (maxHeight2 - cam.y) / (dV_view.y);
		if (pos.y < 256.0) { return color; }
	}
	if (cam.y > 256.0 && cam.y < maxHeight) {
		if (dV_view.y <= 0.0) {
			let maxHeight2 = max(256.0, pos.y);
			dV_view *= abs(maxHeight2 - cam.y) / abs(dV_view.y) / it;
			progress_view = dV_view * dither + cam + dV_view * it;
			dV_view *= -1.0;
		} else if (dV_view.y > 0.0) {
			let maxHeight2 = min(maxHeight, pos.y);
			dV_view *= -abs(maxHeight2 - cam.y) / abs(dV_view.y) / it;
			progress_view = dV_view * dither + cam - dV_view * it;
		}
	}
	if (cam.y >= maxHeight) {
		let maxHeight2 = max(256.0, pos.y);
		dV_view *= -abs(maxHeight2 - maxHeight) / abs(dV_view.y) / it;
		progress_view = dV_view * dither + cam + dV_view * (maxHeight2 - cam.y) / dV_view.y;
		if (pos.y > maxHeight) { return color; }
	}
	let mult = length(dV_view) / 256.0;
	for (var i = 0; i < cloudIT; i++) {
		let cloud = cloudPlane(progress_view) * 40.0;
		let lightsourceVis = pow(clamp(progress_view.y - 256.0, 0.0, 200.0) / 200.0, 2.3);
		color = mix(color, mix(v.cloudColor2 * 0.05, v.cloudColor * 0.15, lightsourceVis), 1.0 - exp(-cloud * mult));
		progress_view += dV_view;
	}
	return color;
}
fn calcStars(pos: vec3f) -> f32 {
	let p = pos * 256.0;
	var flr = floor(p);
	let fr = length((p - flr) - 0.5);
	flr = fract(flr * 443.8975);
	flr += dot(flr, flr.xyz + 19.19);
	let intensity = step(fract((flr.x + flr.y) * flr.z), 0.0025) * (1.0 - rainStrength());
	var stars = clamp((fr - 0.5) / (0.0 - 0.5), 0.0, 1.0);
	stars = stars * stars * (3.0 - 2.0 * stars);
	return stars * intensity;
}
fn rmatrix(rad: f32) -> mat2x2f {
	return mat2x2f(vec2f(cos(rad), -sin(rad)), vec2f(sin(rad), cos(rad)));
}
fn calcWaves(coordIn: vec2f) -> f32 {
	let ftc = frameTimeCounter();
	let movement = abs(vec2f(0.0, -ftc * 0.5));
	let coord = coordIn * 0.4;
	var coord0 = coord * rmatrix(1.0) - movement * 4.5;
	coord0.y *= 3.0;
	var coord1 = coord * rmatrix(0.5) - movement * 1.5;
	coord1.y *= 3.0;
	let coord2 = coord * ftc * 0.02;
	coord0 *= 1.5;
	coord1 *= (1.5 - 0.5);
	var wave = noiseAt(coord0 * 0.005) * 10.0;
	wave -= noiseAt(coord1 * 0.010416) * 7.0;
	wave += 1.0 - sqrt(noiseAt(coord2 * 0.0416) * 6.5) * 1.33;
	wave *= 0.0157;
	return wave;
}
fn calcBump(coord: vec2f) -> vec2f {
	let deltaPos = vec2f(0.25, 0.0);
	let h0 = calcWaves(coord);
	let h1 = calcWaves(coord + deltaPos.xy);
	let h2 = calcWaves(coord - deltaPos.xy);
	let h3 = calcWaves(coord + deltaPos.yx);
	let h4 = calcWaves(coord - deltaPos.yx);
	let xDelta = ((h1 - h0) + (h0 - h2));
	let yDelta = ((h3 - h0) + (h0 - h4));
	return vec2f(xDelta, yDelta) * 0.05;
}
@fragment
fn fs(i: CV) -> @location(0) vec4f {
	let v = c1vars();
	let texcoord = i.tc;
	let fc = glFrag(i.pos);
	let rain = rainStrength();
	let nightV = night();
	let wh = pu.view.xy;
	var c = pow(tex2D(colortex4, texcoord).xyz, vec3f(2.2)) * 257.0;
	let hr = tex2D(colortex3, (floor(texcoord * wh / 2.0) * 2.0 + 1.0) / wh / 2.0).rgb * 30.0;
	let depth0 = depthAt(depthtex0, texcoord);
	var fragpos0 = pu.projInv * (vec4f(texcoord, depth0, 1.0) * 2.0 - 1.0);
	fragpos0 /= fragpos0.w;
	let normalfragpos0 = normalize(fragpos0.xyz);
	let depth1 = depthAt(depthtex1, texcoord);
	var fragpos1 = pu.projInv * (vec4f(texcoord, depth1, 1.0) * 2.0 - 1.0);
	fragpos1 /= fragpos1.w;
	let normalfragpos1 = normalize(fragpos1.xyz);
	let trp = tex2D(colortex6, texcoord);
	let transparency = dot(trp.xyz, trp.xyz) > 0.000001;
	let gn = tex2D(colortex2, texcoord);
	let gd = tex2D(colortex1, texcoord);
	if (gn.z < 0.2499) {
		let normal = decodeNormal(gn.xy);
		let reflective = dot(normal.xyz, normal.xyz) > 0.0;
		let wpos = (pu.mvInv * fragpos0).xz + pu.camPos.xz;
		let refraction = texcoord.xy + calcBump(wpos);
		let skylight = pow(max(gd.w - 1.0 / 16.0, 0.0) * 1.14285714286, 0.5);
		let wposC = (pu.mvInv * fragpos1).xz + pu.camPos.xz;
		var caustics = calcWaves(wposC * 3.0);
		caustics = mix(caustics * 4.0, 1.0, 1.0 - skylight);
		caustics = clamp(caustics * 0.8, -1.0, 0.1);
		caustics *= (1.0 - rain);
		if (transparency && reflective) {
			c = pow(tex2D(colortex4, refraction).xyz, vec3f(2.2) + caustics) * 257.0;
		} else if (isEyeInWater() == 1.0) {
			let underwaterC = vec3f(0.0, 0.0025, 0.0075) * (1.0 - 0.95 * nightV) * (1.0 - 0.95 * rain);
			c = pow(tex2D(colortex4, refraction).xyz, vec3f(2.2) + caustics) * 257.0;
			c = mix(c, underwaterC, 0.75);
		}
	}
	if (depth1 > comp()) {
		let cpos = normalize(pu.mvInv * fragpos1).xyz;
		c = hr.rgb;
		c += calcStars(cpos) * v.moonVisibility;
		c = drawSun(fragpos1.xyz, c, 1.0, v);
		c = drawMoon(fragpos1.xyz, c, 1.0, v);
		let cheight = (256.0 - 32.0);
		if (dot(fragpos1.xyz, v.upVec) > 0.0 || pu.camPos.y > cheight) { c = renderClouds(cpos, c, 6, fc, v); }
	}
	let tm = tmult();
	let fogC = hr.rgb * (0.7 + 0.3 * tm) * (1.33 - rain * 0.67);
	let fogF = calcFog(fragpos0.xyz);
	if (transparency) {
		var normal = gn.xyz;
		var sky = normal.z;
		let iswater = sky < 0.2499;
		let isice = sky > 0.2499 && sky < 0.4999;
		if (iswater) { sky *= 4.0; }
		if (isice) { sky = (sky - 0.25) * 4.0; }
		if (!iswater && !isice) { sky = (sky - 0.5) * 4.0; }
		sky = clamp(sky * 1.2 - 2.0 / 16.0 * 1.2, 0.0, 1.0);
		sky *= sky;
		normal = decodeNormal(normal.xy);
		let reflective = dot(normal.xyz, normal.xyz) > 0.0;
		normal = normalize(normal);
		let iswater2 = f32(iswater);
		let skylight = pow(max(gd.w - 2.0 / 16.0, 0.0) * 1.14285714286, 1.0);
		c = mix(c, fogC, fogF - fogF) / (1.0 + 5.0 * nightV * iswater2 * skylight);
		let finalAc = tex2D(colortex5, texcoord.xy).rgb;
		let alphaT = clamp(length(trp.rgb) * 1.02, 0.0, 1.0);
		c = mix(c, c * (trp.rgb * 0.9999 + 0.0001) * 1.732, alphaT) * (1.0 - alphaT) + finalAc;
		if (reflective) {
			let reflectedVector = reflect(normalfragpos1, normal);
			let hV = normalize(reflectedVector - normalfragpos1);
			let normalDotEye = dot(hV, normalfragpos1);
			let F0 = 0.09;
			var fresnel = pow(clamp(1.0 + normalDotEye, 0.0, 1.0), 4.0);
			fresnel = fresnel + F0 * (1.0 - fresnel);
			var sky_c = getSkyc(reflectedVector * 620.0, v) * 1.7;
			let cloudVector = normalize(pu.mvInv * vec4f(reflect(fragpos1.xyz, normal), 1.0)).xyz;
			sky_c += renderClouds(cloudVector, c, 4, fc, v);
			var reflection = raytrace(fragpos0.xyz, sky_c, reflectedVector, v);
			if (isEyeInWater() == 0.0) {
				sky_c = ((drawSun(reflectedVector, sky_c, 1.0, v) + drawMoon(reflectedVector, sky_c, 1.0, v)) * 0.5) * sky;
			} else {
				sky_c = pow(vec3f(0.25, 0.5, 0.72), vec3f(2.2)) * v.rawAvg * 0.1;
			}
			reflection = vec4f(mix(sky_c, reflection.rgb, reflection.a) * 0.5, reflection.a);
			fresnel *= 0.5 * f32(isice) + 0.5 * iswater2;
			c = mix(c, reflection.rgb, fresnel * 1.5);
		}
	}
	let land = depth0 < comp();
	// The wet-ground reflections need BiomeTemp, a custom uniform OptiFine
	// E7 does not set (always 0): isRaining is always false.
	var foglandC = fogC;
	foglandC.b *= (1.5 - 0.5 * rain);
	if (land) { c = mix(c, foglandC * (1.0 - isEyeInWater()), fogF); }
	if (!land && rain > 0.01) { c = mix(c, fogC, fogF * 0.75); }
	var ufogC = v.sky1.rgb * 0.1;
	ufogC.r *= 0.0;
	ufogC *= (1.0 - 0.85 * nightV);
	if (isEyeInWater() == 1.0) { c = mix(c, ufogC, 1.0 - exp(-length(fragpos0.xyz) / 25.0)); }
	if (isEyeInWater() == 2.0) { c = mix(c, vec3f(1.0, 0.0125, 0.0), 1.0 - exp(-length(fragpos0.xyz))); }
	if (pu.eye.y > 0.9) { c = mix(c, vec3f(0.0), 1.0 - exp(-length(fragpos1.xyz) * 1.125)); }
	let depth2 = depthAt(depthtex2, texcoord);
	let hand = (depth0 < depth1) || !(depth0 < depth2);
	let rainT = tex2D(colortex7, texcoord);
	if (rainT.r > 0.0001 && rain > 0.01 && hand) {
		let rainRGB = 0.25;
		let rainA = rainT.r;
		var torch_lightmap = 6.4 - min(rainT.g / rainT.r * 6.16, 5.6);
		torch_lightmap = 0.1 / torch_lightmap / torch_lightmap - 0.002595;
		var rainC = rainRGB * (pow(max(dot(normalfragpos0, v.sunVec) * 0.1 + 0.9, 0.0), 6.0) * (0.1 + v.tr * 0.9) * pow(v.sunlight, vec3f(0.25)) * v.sunVisibility + pow(max(dot(normalfragpos0, -v.sunVec) * 0.05 + 0.95, 0.0), 6.0) * 48.0 * moonlightF() * v.moonVisibility) * 0.04 + 0.05 * rainRGB * length(v.avgAmbient2);
		rainC += torch_lightmap * vec3f(1.5, 0.42, 0.045);
		c = c * (1.0 - rainA * 0.3) + rainC * 1.5 * rainA;
	}
	let sunpos = abs(dot(normalfragpos0, normalize(pu.sunPos.xyz)));
	let illuminationDecay = pow(sunpos, 30.0) + pow(sunpos, 16.0) * 0.8 + pow(sunpos, 2.0) * 0.125;
	let deltaTextCoord = (v.lightPos - texcoord) * 0.01;
	let textCoord = texcoord * 0.5 + 0.5;
	var gr = 0.0;
	for (var k = 1; k <= 7; k++) {
		gr += tex2DLod(colortex4, textCoord + f32(k) * deltaTextCoord, 1.0).a;
	}
	let grC = v.lightColor * 1.15;
	if (pu.eye.y < 1.0) { c += grC * gr / 7.0 * illuminationDecay * (1.0 - isEyeInWater()); }
	c = (c / 50.0 * pow(v.eyeAdapt, 0.88));
	return vec4f(c, 1.0);
}
`

// packComposite2WGSL is composite2 (DRAWBUFFERS:7): TAA against gaux4.
var packComposite2WGSL = packCompositeHeadWGSL + `
fn toClipSpace3Prev(viewSpacePosition: vec3f) -> vec3f {
	let pp = pu.prevProj;
	return (vec3f(pp[0].x, pp[1].y, pp[2].z) * viewSpacePosition + pp[3].xyz) / -viewSpacePosition.z * 0.5 + 0.5;
}
fn FastCatmulRom(texcoord: vec2f, rtMetrics: vec4f, sharpenAmount: f32) -> vec3f {
	let position = rtMetrics.zw * texcoord;
	let centerPosition = floor(position - 0.5) + 0.5;
	let f = position - centerPosition;
	let f2 = f * f;
	let f3 = f * f2;
	let c = sharpenAmount;
	let w0 = -c * f3 + 2.0 * c * f2 - c * f;
	let w1 = (2.0 - c) * f3 - (3.0 - c) * f2 + 1.0;
	let w2 = -(2.0 - c) * f3 + (3.0 - 2.0 * c) * f2 + c * f;
	let w3 = c * f3 - c * f2;
	let w12 = w1 + w2;
	let tc12 = rtMetrics.xy * (centerPosition + w2 / w12);
	let centerColor = tex2D(colortex7, vec2f(tc12.x, tc12.y)).rgb;
	let tc0 = rtMetrics.xy * (centerPosition - 1.0);
	let tc3 = rtMetrics.xy * (centerPosition + 2.0);
	let color = vec4f(tex2D(colortex7, vec2f(tc12.x, tc0.y)).rgb, 1.0) * (w12.x * w0.y) +
		vec4f(tex2D(colortex7, vec2f(tc0.x, tc12.y)).rgb, 1.0) * (w0.x * w12.y) +
		vec4f(centerColor, 1.0) * (w12.x * w12.y) +
		vec4f(tex2D(colortex7, vec2f(tc3.x, tc12.y)).rgb, 1.0) * (w3.x * w12.y) +
		vec4f(tex2D(colortex7, vec2f(tc12.x, tc3.y)).rgb, 1.0) * (w12.x * w3.y);
	return color.rgb / color.a;
}
fn calcTAA(texcoord: vec2f) -> vec3f {
	let ts = texelSize();
	let wh = pu.view.xy;
	let depth0 = depthAt(depthtex0, texcoord);
	let closestToCamera = vec3f(texcoord, depth0);
	var fragposition = toScreenSpace(closestToCamera);
	fragposition = m3(pu.mvInv) * fragposition + pu.mvInv[3].xyz + (pu.camPos.xyz - pu.prevCamPos.xyz);
	var previousPosition = m3(pu.prevMV) * fragposition + pu.prevMV[3].xyz;
	previousPosition = toClipSpace3Prev(previousPosition);
	previousPosition = vec3f(texcoord + (previousPosition.xy - closestToCamera.xy), previousPosition.z);
	let d = 0.5 - abs(fract(previousPosition.xy * wh - texcoord * wh) - 0.5);
	let rej = dot(d, d) * 0.5;
	if (previousPosition.x < 0.0 || previousPosition.y < 0.0 || previousPosition.x > 1.0 || previousPosition.y > 1.0) {
		return tex2D(colortex3, texcoord).rgb;
	}
	let albedoCurrent0 = tex2D(colortex3, texcoord).rgb;
	let albedoCurrent1 = tex2D(colortex3, texcoord + vec2f(ts.x, ts.y)).rgb;
	let albedoCurrent2 = tex2D(colortex3, texcoord + vec2f(ts.x, -ts.y)).rgb;
	let albedoCurrent3 = tex2D(colortex3, texcoord + vec2f(-ts.x, -ts.y)).rgb;
	let albedoCurrent4 = tex2D(colortex3, texcoord + vec2f(-ts.x, ts.y)).rgb;
	let albedoCurrent5 = tex2D(colortex3, texcoord + vec2f(0.0, ts.y)).rgb;
	let albedoCurrent6 = tex2D(colortex3, texcoord + vec2f(0.0, -ts.y)).rgb;
	let albedoCurrent7 = tex2D(colortex3, texcoord + vec2f(-ts.x, 0.0)).rgb;
	let albedoCurrent8 = tex2D(colortex3, texcoord + vec2f(ts.x, 0.0)).rgb;
	let cMax = max(max(max(albedoCurrent0, albedoCurrent1), albedoCurrent2), max(albedoCurrent3, max(albedoCurrent4, max(albedoCurrent5, max(albedoCurrent6, max(albedoCurrent7, albedoCurrent8))))));
	let cMin = min(min(min(albedoCurrent0, albedoCurrent1), albedoCurrent2), min(albedoCurrent3, min(albedoCurrent4, min(albedoCurrent5, min(albedoCurrent6, min(albedoCurrent7, albedoCurrent8))))));
	let albedoPrev = FastCatmulRom(previousPosition.xy, vec4f(ts, 1.0 / ts), 0.82).xyz;
	let finalcAcc = clamp(albedoPrev, cMin, cMax);
	let lum = dot(albedoPrev, vec3f(0.21, 0.72, 0.07));
	let isclamped = distance(albedoPrev, finalcAcc) / lum;
	var lumDiff2 = distance(albedoPrev, albedoCurrent0) / lum;
	lumDiff2 = 1.0 - clamp(lumDiff2 * lumDiff2, 0.0, 1.0) * 0.75;
	return mix(finalcAcc, albedoCurrent0, clamp(0.1 * lumDiff2 + rej + isclamped + 0.01, 0.0, 1.0));
}
@fragment
fn fs(i: CV) -> @location(0) vec4f { return vec4f(calcTAA(i.tc), 1.0); }
`

// packComposite3WGSL is composite3 (DRAWBUFFERS:6): the horizontal bloom
// blur of colortex7 (mipmapped) into the lower-left quarter. MC_VERSION
// 10710 scales the coordinates by 4: colortex7 is read at LOD 2.
var packComposite3WGSL = packCompositeHeadWGSL + `
@fragment
fn fs(i: CV) -> @location(0) vec4f {
	let newtc = i.tc * 4.0;
	let nSteps = 25;
	let center = 12;
	var blur = vec3f(0.0);
	var tw = 0.0;
	for (var k = 0; k < nSteps; k++) {
		let dist = abs(f32(k) - f32(center)) / f32(center);
		let weight = (exp(-(dist * dist) / 0.28));
		let bsample = tex2DLod(colortex7, (newtc + 2.0 * texelSize() * vec2f(f32(k - center), 0.0)), 2.0).rgb;
		blur += bsample * weight;
		tw += weight;
	}
	blur /= tw;
	blur = clamp(blur, vec3f(0.0), vec3f(1.0));
	return vec4f(blur, 1.0);
}
`

// packComposite4WGSL is composite4 (DRAWBUFFERS:6): the vertical blur and
// glow.
var packComposite4WGSL = packCompositeHeadWGSL + `
fn c4ToD() -> array<vec3f, 7> { return array<vec3f, 7>(vec3f(0.58597, 0.16, 0.025), vec3f(0.58597, 0.4, 0.2), vec3f(0.58597, 0.52344, 0.24680),
	vec3f(0.58597, 0.55422, 0.34), vec3f(0.58597, 0.57954, 0.38), vec3f(0.58597, 0.58, 0.40), vec3f(0.58597, 0.58, 0.40)); }
fn eyeAdapt4() -> f32 {
	let rain = rainStrength();
	let sunVec = normalize(pu.sunPos.xyz);
	let upVec = normalize(pu.upPos.xyz);
	let SdotU = dot(sunVec, upVec);
	let sunVisibility = pow(clamp(SdotU + 0.15, 0.0, 0.15) / 0.15, 4.0);
	let moonVisibility = pow(clamp(-SdotU + 0.15, 0.0, 0.15) / 0.15, 4.0);
	let sunlight = todMix(c4ToD());
	let eyebright = max(eyeSmoothY() / 255.0 - 0.5 / 16.0, 0.0) * 1.03225806452;
	let SkyL2 = mix(1.0, eyebright * eyebright, eyebright);
	let tr = trOf(23250.0);
	let bounced = vec4f(0.5 * SkyL2, 0.66 * SkyL2, 0.7, 0.3);
	let sun_ambient = bounced.w * (vec3f(0.25, 0.62, 1.32) - rain * vec3f(0.11, 0.32, 1.07)) + sunlight * (bounced.x + bounced.z);
	let moonlight = vec3f(0.0035, 0.0063, 0.0098);
	let avgAmbient = (sun_ambient * sunVisibility + moonlight * moonVisibility) * eyebright * eyebright * (0.05 + tr * 0.15) * 4.7 + 0.0006;
	let e = log(clamp(luma(avgAmbient), 0.007, 80.0)) / log(2.6) * 0.35;
	return 1.0 / pow(2.6, e) * 1.75;
}
@fragment
fn fs(i: CV) -> @location(0) vec4f {
	let nSteps = 17;
	let center = 8;
	var blur = vec3f(0.0);
	var tw = 0.0;
	for (var k = 0; k < nSteps; k++) {
		let dist = abs(f32(k) - f32(center)) / f32(center);
		let weight = (exp(-(dist * dist) / 0.28));
		let bsample = tex2D(colortex6, (i.tc + texelSize() * vec2f(0.0, f32(k - center)))).rgb * 3.0;
		blur += bsample * weight;
		tw += weight;
	}
	blur /= tw;
	let glow = blur * 0.75;
	let overglow = glow * pow(length(glow) * 2.0, 2.8) * 2.0;
	let finalColor = (overglow + glow * 1.15) * (1.0 + isEyeInWater() * 10.0 + (pow(rainStrength(), 3.0) * 7.0 / pow(eyeAdapt4(), 1.0))) * 1.2;
	return vec4f(finalColor, 1.0);
}
`

// packFinalWGSL is final.vsh/fsh: rain lens drops, underwater refraction,
// the sky blur, bloom and the Uncharted 2 tonemap.
var packFinalWGSL = packCompositeHeadWGSL + `
fn noisepattern(pos: vec2f) -> vec2f {
	return vec2f(abs(fract(sin(dot(pos, vec2f(83147.6995379, 125370.887575))))));
}
fn ld(depth: f32) -> f32 {
	return (2.0 * nearP()) / (farP() + nearP() - depth * (farP() - nearP()));
}
fn cblur(tc: vec2f) -> vec3f {
	let pw = 1.0 / pu.view.x;
	let getdist = 1.0 - (exp(-pow(ld(depthAt(depthtex1, tc)) / 256.0 * farP(), 4.0) * 4.0));
	let pcoc = min(getdist * pw * 20.0, pw * 20.0);
	var fast_blur = array<vec2f, 4>(vec2f(0.0, -0.1), vec2f(-0.1, 0.0), vec2f(0.1, 0.0), vec2f(0.0, 0.1));
	var blurC = vec3f(0.0);
	for (var k = 0; k < 4; k++) {
		blurC += tex2D(colortex7, tc + fast_blur[k] * pcoc * vec2f(1.0, pu.view.z)).rgb;
	}
	blurC = blurC / 4.0 * 50.0;
	return blurC;
}
fn distratio(pos: vec2f, pos2: vec2f) -> f32 {
	return distance(pos * vec2f(pu.view.z, 1.0), pos2 * vec2f(pu.view.z, 1.0));
}
fn gen_circular_lens(center: vec2f, size: f32, texcoord: vec2f) -> f32 {
	let dist = distratio(center, texcoord.xy) / size;
	return exp(-dist * dist);
}
fn Uncharted2Tonemap(xIn: vec3f) -> vec3f {
	let x = xIn * 1.0;
	let A = 0.28;
	let B = 0.29;
	let C = 0.10;
	let D = 0.2;
	let E = 0.025;
	let F = 0.35;
	return ((x * (A * x + C * B) + D * E) / (x * (A * x + B) + D * F)) - E / F;
}
@fragment
fn fs(i: CV) -> @location(0) vec4f {
	let texcoord = i.tc;
	let rain = rainStrength();
	let ftc = frameTimeCounter();
	// final.vsh
	let lifetime = 4.0;
	let ftime = ftc * 2.0 / lifetime;
	let drop = vec2f(0.0, fract(ftc / 20.0));
	let rainPos1 = fract((noisepattern(vec2f(-0.94386347 * floor(ftime * 0.5 + 0.25), floor(ftime * 0.5 + 0.25)))) * 0.8 + 0.1 - drop);
	let rainPos2 = fract((noisepattern(vec2f(0.9347 * floor(ftime * 0.5 + 0.5), -0.2533282 * floor(ftime * 0.5 + 0.5)))) * 0.8 + 0.1 - drop);
	let rainPos3 = fract((noisepattern(vec2f(0.785282 * floor(ftime * 0.5 + 0.75), -0.285282 * floor(ftime * 0.5 + 0.75)))) * 0.8 + 0.1 - drop);
	let rainPos4 = fract((noisepattern(vec2f(-0.347 * floor(ftime * 0.5), 0.6847 * floor(ftime * 0.5)))) * 0.8 + 0.1 - drop);
	var weights = vec4f(1.0 - fract((ftime + 0.5) * 0.5), 1.0 - fract((ftime + 1.0) * 0.5), 1.0 - fract((ftime + 1.5) * 0.5), 1.0 - fract(ftime * 0.5));
	weights *= rain * clamp((eyeSmoothY() - 220.0) / 15.0, 0.0, 1.0);
	// final.fsh
	let depth1 = depthAt(depthtex1, texcoord);
	var rainlens = 0.0;
	if (rain > 0.02) {
		rainlens += gen_circular_lens(rainPos1, 0.1, texcoord) * weights.x;
		rainlens += gen_circular_lens(rainPos2, 0.07, texcoord) * weights.y;
		rainlens += gen_circular_lens(rainPos3, 0.086, texcoord) * weights.z;
		rainlens += gen_circular_lens(rainPos4, 0.092, texcoord) * weights.w;
	}
	let fake_refract = vec2f(sin(ftc + texcoord.x * 100.0 + texcoord.y * 50.0), cos(ftc + texcoord.y * 100.0 + texcoord.x * 50.0));
	let ts = texelSize();
	let newTC = clamp(texcoord + fake_refract * 0.01 * (rainlens + isEyeInWater() * 0.2), ts, 1.0 - ts);
	var color = tex2D(colortex7, newTC.xy).rgb * 50.0;
	if (depth1 > comp()) { color = cblur(newTC.xy); }
	color += tex2D(colortex6, texcoord.xy * 0.25).rgb;
	color += rainlens * 0.01;
	let curr = Uncharted2Tonemap(color * 4.7);
	color = pow(curr / Uncharted2Tonemap(vec3f(15.2)), vec3f(1.0 / 2.2));
	return vec4f(color, 1.0);
}
`

// packMipWGSL makes a mip level from the one above (glGenerateMipmap's
// box filter: one bilinear tap at the centre of each 2x2 block).
const packMipWGSL = `
@group(0) @binding(0) var src: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;
struct V { @builtin(position) pos: vec4f, @location(0) uv: vec2f }
@vertex
fn vs(@builtin(vertex_index) id: u32) -> V {
	let p = vec2f(f32((id << 1u) & 2u), f32(id & 2u));
	var o: V;
	o.uv = vec2f(p.x, 1.0 - p.y);
	o.pos = vec4f(p * 2.0 - 1.0, 0.0, 1.0);
	return o;
}
@fragment
fn fs(i: V) -> @location(0) vec4f { return textureSampleLevel(src, samp, i.uv, 0.0); }
`

// packDamageWGSL is the breaking overlay (blockWGSL's damage mode) drawn
// with gbuffers_terrain, the program OptiFine uses for it.
var packDamageWGSL = `
const damagePull = 0.002;
struct U {
	mvp: mat4x4f,
	model: mat4x4f,
	light: vec4f,
	fogColor: vec4f,
	fog: vec4f,
	l0: vec4f,
	l1: vec4f,
}
@group(0) @binding(0) var<uniform> u: U;
@group(1) @binding(0) var tex: texture_2d_array<f32>;
@group(1) @binding(1) var samp: sampler;
@group(1) @binding(2) var layerMap: texture_2d<u32>;
@group(1) @binding(3) var lightmap: texture_2d<f32>;
@group(1) @binding(4) var lightSamp: sampler;
` + packCommonWGSL + packGroupWGSL + `
struct VOut {
	@builtin(position) pos: vec4f,
	@location(0) uv: vec2f,
	@location(1) normal: vec3f,
	@location(2) lm: vec2f,
}
fn faceNormal(n: u32) -> vec3f {
	var normals = array<vec3f, 7>(vec3f(0.0, -1.0, 0.0), vec3f(0.0, 1.0, 0.0), vec3f(0.0, 0.0, -1.0),
		vec3f(0.0, 0.0, 1.0), vec3f(-1.0, 0.0, 0.0), vec3f(1.0, 0.0, 0.0), vec3f(0.0, 1.0, 0.0));
	return normals[min(n, 6u)];
}
@vertex
fn vs(@location(0) d: vec4<u32>) -> VOut {
	let w0 = d.x; let w1 = d.y; let w3 = d.w;
	let coarse = vec3f(f32(w0 & 1023u), f32((w0 >> 10u) & 1023u), f32((w0 >> 20u) & 1023u));
	let fine = vec3f(f32((w1 >> 26u) & 3u), f32((w1 >> 28u) & 3u), f32(w1 >> 30u));
	let local = (coarse * 4.0 + fine) * (1.0 / 128.0) - 8.0;
	var o: VOut;
	o.pos = jitter(u.mvp * vec4f((u.model * vec4f(local, 1.0)).xyz * (1.0 - damagePull), 1.0));
	o.uv = vec2f(f32((w1 >> 12u) & 31u), f32((w1 >> 17u) & 31u));
	if ((w1 & (1u << 25u)) != 0u) { o.uv = o.uv * (1.0 / 16.0); }
	o.normal = m3(pu.mv) * m3(u.model) * faceNormal((w1 >> 22u) & 7u);
	o.lm = (vec2f(f32((w3 >> 8u) & 255u), f32(w3 & 255u)) + 8.0) * (1.0 / 256.0);
	return o;
}
struct GOut {
	@location(0) c0: vec4f,
	@location(1) c1: vec4f,
}
@fragment
fn fs(i: VOut) -> GOut {
	var albedo = textureSample(tex, samp, i.uv, u32(u.fog.z));
	albedo.a *= 0.5; // glColor4f(1, 1, 1, 0.5)
	if (albedo.a <= 0.1) { discard; }
	var c = vec4f(ycocg(albedo.rgb), albedo.a);
	if (pattern(glFrag(i.pos))) { c.g = c.b; }
	c.b = 0.02;
	var o: GOut;
	o.c0 = c;
	o.c1 = vec4f(encodeNormal(normalize(i.normal)), i.lm);
	return o;
}
`

// The pack's buffer formats that WebGPU can't render to are emulated with
// RGBA16Float (packFormats); what they would store is clamped on output:
// R11F_G11F_B10F (composite, gaux2..4 = colortex3, 5..7) has no sign and a
// 65024 maximum, RGBA16 (gdepth, gaux1 = colortex1, 4) is 0..1. No program
// reads those buffers' alpha, so it passes through (it drives blending).
const packStoreWGSL = `
fn storeR11(v: vec4f) -> vec4f { return vec4f(clamp(v.rgb, vec3f(0.0), vec3f(65024.0)), v.a); }
fn storeU16(v: vec4f) -> vec4f { return clamp(v, vec4f(0.0), vec4f(1.0)); }
`

var packFsRe = regexp.MustCompile(`@fragment\s*\nfn fs\(([^)]*)\) -> ([^{]+)\{`)

// packStore wraps a program's fs so its outputs are stored as the pack's
// formats would: struct fields by buffer name, a single @location(0)
// output with single ("storeR11", or "" to leave it).
func packStore(src, single string) string {
	m := packFsRe.FindStringSubmatchIndex(src)
	if m == nil {
		return src
	}
	params, ret := src[m[2]:m[3]], strings.TrimSpace(src[m[4]:m[5]])
	var args []string
	for _, p := range strings.Split(params, ",") {
		if f := strings.Fields(strings.TrimSpace(p)); len(f) > 0 {
			args = append(args, strings.TrimSuffix(f[len(f)-2], ":"))
		}
	}
	call := "fs_raw(" + strings.Join(args, ", ") + ")"
	var body string
	if strings.HasPrefix(ret, "@location") {
		if single == "" {
			return src
		}
		body = "return " + single + "(" + call + ");"
	} else {
		sm := regexp.MustCompile(`struct ` + ret + ` \{([^}]*)\}`).FindStringSubmatch(src)
		if sm == nil {
			panic("packStore: no struct " + ret)
		}
		body = "var o = " + call + ";"
		n := 0
		for _, f := range regexp.MustCompile(`\b(c[0-9]):`).FindAllStringSubmatch(sm[1], -1) {
			fn := map[string]string{"c1": "storeU16", "c4": "storeU16", "c3": "storeR11", "c5": "storeR11", "c6": "storeR11", "c7": "storeR11"}[f[1]]
			if fn != "" {
				body += " o." + f[1] + " = " + fn + "(o." + f[1] + ");"
				n++
			}
		}
		if n == 0 {
			return src
		}
		body += " return o;"
	}
	rawRet := ret
	if i := strings.Index(ret, ")"); strings.HasPrefix(ret, "@location") && i >= 0 {
		rawRet = strings.TrimSpace(ret[i+1:])
	}
	out := src[:m[0]] + "fn fs_raw(" + params + ") -> " + rawRet + " {" + src[m[1]:]
	return out + packStoreWGSL + "@fragment\nfn fs(" + params + ") -> " + ret + " { " + body + " }\n"
}
