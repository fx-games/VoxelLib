//go:build !wgpu

package render

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/platform/gl"
)

const itemVS = `#version 330 core
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aUV;
layout(location = 2) in vec3 aNormal;
uniform mat4 uMVP;
uniform mat4 uModel; // model rotation+scale (camera-relative world space)
out vec3 vUV;
out float vShade;
out float vDist;
void main() {
	vec4 wp = uModel * vec4(aPos, 1.0);
	gl_Position = uMVP * vec4(aPos, 1.0);
	vUV = aUV;
	vec3 n = normalize(mat3(uModel) * aNormal);
	// Minecraft-like fixed directional shading: top bright, bottom dark.
	vShade = 0.7 + 0.3 * n.y + 0.1 * abs(n.x);
	vDist = length(wp.xz);
}
`

const itemFS = `#version 330 core
in vec3 vUV;
in float vShade;
in float vDist;
uniform sampler2DArray uTex;
uniform float uLayer;
uniform vec2 uLight; // sky, block in 0..1
uniform float uDaylight;
uniform vec3 uFogColor;
uniform vec2 uFog;
out vec4 fragColor;
float curve(float l) { return mix(0.04, 1.0, l / (4.0 - 3.0 * l)); }
// blockCurve is curve for block light, raised toward Minecraft's bright
// gamma (1-(1-l)^4) by uBlockBright.
uniform float uBlockBright;
float blockCurve(float l) { return mix(curve(l), mix(0.04, 1.0, 1.0 - pow(1.0 - l, 4.0)), uBlockBright); }
void main() {
	vec4 c = texture(uTex, vec3(vUV.xy, uLayer));
	if (c.a < 0.5) discard;
	float sky = curve(uLight.x) * uDaylight;
	float blk = blockCurve(uLight.y);
	vec3 light = max(max(vec3(sky) * vec3(0.95, 0.97, 1.0), vec3(blk) * vec3(1.0, 0.86, 0.66)), vec3(0.04));
	vec3 col = c.rgb * vShade * light;
	float f = clamp((vDist - uFog.x) / (uFog.y - uFog.x), 0.0, 1.0);
	fragColor = vec4(mix(col, uFogColor, f * f), 1.0);
}
`

type gpuItemRenderer struct {
	prog *program
	vao  uint32
	vbo  uint32
	tex  uint32
}

func (ir *ItemRenderer) initGPU(verts []float32, size int, layers [][]byte) error {
	prog, err := newProgram(itemVS, itemFS)
	if err != nil {
		return err
	}
	g := &ir.gpu
	g.prog = prog
	gl.GenVertexArrays(1, &g.vao)
	gl.BindVertexArray(g.vao)
	gl.GenBuffers(1, &g.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.vbo)
	if len(verts) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STATIC_DRAW)
	}
	stride := int32(8 * 4)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, stride, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, stride, 3*4)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 3, gl.FLOAT, false, stride, 5*4)
	gl.BindVertexArray(0)

	gl.GenTextures(1, &g.tex)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, g.tex)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAX_LEVEL, 0)
	gl.TexImage3D(gl.TEXTURE_2D_ARRAY, 0, gl.RGBA8, int32(size), int32(size), int32(max(len(layers), 1)), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	for i, px := range layers {
		gl.TexSubImage3D(gl.TEXTURE_2D_ARRAY, 0, 0, 0, int32(i), int32(size), int32(size), 1, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	}
	return nil
}

func (ir *ItemRenderer) begin(fog Fog) {
	p := ir.gpu.prog
	p.use()
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.Uniform1f(p.loc("uDaylight"), ir.r.Opt.Daylight)
	gl.Uniform1f(p.loc("uBlockBright"), ir.r.Opt.BlockBrightness)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &fog.Color[0])
	gl.Uniform2f(p.loc("uFog"), fog.Start, fog.End)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, ir.gpu.tex)
	gl.BindVertexArray(ir.gpu.vao)
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE) // slabs are thin; cheap to draw both sides
}

func (ir *ItemRenderer) drawRange(i int, mvp, model mgl32.Mat4, sky, block uint8) {
	p := ir.gpu.prog
	gl.UniformMatrix4fv(p.loc("uMVP"), 1, false, &mvp[0])
	gl.UniformMatrix4fv(p.loc("uModel"), 1, false, &model[0])
	gl.Uniform1f(p.loc("uLayer"), float32(i))
	gl.Uniform2f(p.loc("uLight"), float32(sky)/15, float32(block)/15)
	gl.DrawArrays(gl.TRIANGLES, ir.ranges[i][0], ir.ranges[i][1])
}

func (ir *ItemRenderer) end() {
	gl.BindVertexArray(0)
	gl.Enable(gl.CULL_FACE)
}

// Delete frees GL resources.
func (ir *ItemRenderer) Delete() {
	gl.DeleteBuffers(1, &ir.gpu.vbo)
	gl.DeleteVertexArrays(1, &ir.gpu.vao)
	gl.DeleteTextures(1, &ir.gpu.tex)
	gl.DeleteProgram(ir.gpu.prog.id)
}
