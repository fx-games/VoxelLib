//go:build !wgpu

package render

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/platform/gl"
)

const modelVS = `#version 330 core
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aNormal;
layout(location = 2) in vec2 aUV;
layout(location = 3) in uvec4 aJoints;
layout(location = 4) in vec4 aWeights;
uniform mat4 uViewProj;
uniform mat4 uModel;
uniform mat4 uBones[96];
out vec2 vUV;
out float vShade;
out float vDist;
void main() {
	mat4 skin = uBones[aJoints.x] * aWeights.x + uBones[aJoints.y] * aWeights.y +
		uBones[aJoints.z] * aWeights.z + uBones[aJoints.w] * aWeights.w;
	vec4 wp = uModel * skin * vec4(aPos, 1.0);
	gl_Position = uViewProj * wp;
	vec3 n = normalize(mat3(uModel) * mat3(skin) * aNormal);
	// Minecraft entity lighting: two fixed lights, ambient 0.4.
	vec3 l0 = normalize(vec3(0.2, 1.0, -0.7)), l1 = normalize(vec3(-0.2, 1.0, 0.7));
	vShade = min(1.0, 0.4 + 0.6 * (max(dot(n, l0), 0.0) + max(dot(n, l1), 0.0)) * 0.6 + 0.25);
	vUV = aUV;
	vDist = length(wp.xz);
}
`

const modelFS = `#version 330 core
in vec2 vUV;
in float vShade;
in float vDist;
uniform sampler2D uTex;
uniform vec2 uLight;
uniform float uDaylight;
uniform vec3 uFogColor;
uniform vec2 uFog;
uniform float uFade;
out vec4 fragColor;
float curve(float l) { return mix(0.04, 1.0, l / (4.0 - 3.0 * l)); }
// blockCurve is curve for block light, raised toward Minecraft's bright
// gamma (1-(1-l)^4) by uBlockBright.
uniform float uBlockBright;
float blockCurve(float l) { return mix(curve(l), mix(0.04, 1.0, 1.0 - pow(1.0 - l, 4.0)), uBlockBright); }
void main() {
	vec4 c = texture(uTex, vUV);
	if (c.a < 0.5) discard;
	// Interleaved gradient noise: a stable screen-space dither for fading.
	if (uFade > 0.0 && fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(0.06711056, 0.00583715)))) < uFade) discard;
	float sky = curve(uLight.x) * uDaylight;
	float blk = blockCurve(uLight.y);
	vec3 light = max(max(vec3(sky) * vec3(0.95, 0.97, 1.0), vec3(blk) * vec3(1.0, 0.86, 0.66)), vec3(0.04));
	vec3 col = c.rgb * vShade * light;
	float f = clamp((vDist - uFog.x) / (uFog.y - uFog.x), 0.0, 1.0);
	fragColor = vec4(mix(col, uFogColor, f * f), 1.0);
}
`

type gpuModelRenderer struct{ prog *program }

type gpuModel struct {
	vao, vbo, ebo uint32
	parts         []gpuPart
	textures      []uint32
}

func (mr *ModelRenderer) initGPU() (err error) {
	mr.gpu.prog, err = newProgram(modelVS, modelFS)
	return err
}

func (mr *ModelRenderer) upload(vb []float32, ib []uint32, texs []image.Image) *gpuModel {
	g := &gpuModel{}
	gl.GenVertexArrays(1, &g.vao)
	gl.BindVertexArray(g.vao)
	gl.GenBuffers(1, &g.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.vbo)
	if len(vb) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(vb)*4, gl.Ptr(vb), gl.STATIC_DRAW)
	}
	gl.GenBuffers(1, &g.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, g.ebo)
	if len(ib) > 0 {
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(ib)*4, gl.Ptr(ib), gl.STATIC_DRAW)
	}
	s := int32(modelStride * 4)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, s, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 3, gl.FLOAT, false, s, 12)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 2, gl.FLOAT, false, s, 24)
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribIPointerWithOffset(3, 4, gl.UNSIGNED_BYTE, s, 32)
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribPointerWithOffset(4, 4, gl.FLOAT, false, s, 36)
	gl.BindVertexArray(0)
	for _, img := range texs {
		g.textures = append(g.textures, uploadTexture2D(img))
	}
	return g
}

func (g *gpuModel) delete() {
	gl.DeleteVertexArrays(1, &g.vao)
	gl.DeleteBuffers(1, &g.vbo)
	gl.DeleteBuffers(1, &g.ebo)
	gl.DeleteTextures(int32(len(g.textures)), &g.textures[0])
}

func uploadTexture2D(img image.Image) uint32 {
	b := img.Bounds()
	px := toNRGBARect(img)
	var id uint32
	gl.GenTextures(1, &id)
	gl.BindTexture(gl.TEXTURE_2D, id)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(b.Dx()), int32(b.Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px.Pix))
	return id
}

func (mr *ModelRenderer) begin(l modelLight) {
	p := mr.gpu.prog
	p.use()
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &l.vp[0])
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.Uniform1f(p.loc("uDaylight"), l.daylight)
	gl.Uniform1f(p.loc("uBlockBright"), l.block)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &l.fog.Color[0])
	gl.Uniform2f(p.loc("uFog"), l.fog.Start, l.fog.End)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE) // mirrored Blockbench cubes flip winding
}

func (mr *ModelRenderer) end() {
	gl.BindVertexArray(0)
	gl.Enable(gl.CULL_FACE)
}

func (mr *ModelRenderer) drawModel(g *gpuModel, model mgl32.Mat4, skin []mgl32.Mat4, sky, block uint8, fade float32) {
	p := mr.gpu.prog
	b := bones(skin)
	gl.Uniform1f(p.loc("uFade"), fade)
	gl.UniformMatrix4fv(p.loc("uModel"), 1, false, &model[0])
	gl.UniformMatrix4fv(p.loc("uBones"), int32(len(b)), false, &b[0][0])
	gl.Uniform2f(p.loc("uLight"), float32(sky)/15, float32(block)/15)
	gl.BindVertexArray(g.vao)
	for _, pt := range g.parts {
		t := 0
		if pt.tex >= 0 && pt.tex < len(g.textures) {
			t = pt.tex
		}
		gl.BindTexture(gl.TEXTURE_2D, g.textures[t])
		gl.DrawElementsWithOffset(gl.TRIANGLES, pt.count, gl.UNSIGNED_INT, uintptr(pt.first)*4)
	}
}

func clearDepth() { gl.Clear(gl.DEPTH_BUFFER_BIT) }
