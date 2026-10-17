//go:build !wgpu

package render

import "github.com/memmaker/voxellib/platform/gl"

const particleVS = `#version 330 core
layout(location = 0) in vec2 aCorner;
layout(location = 1) in vec3 iPos;   // camera-relative
layout(location = 2) in float iSize;
layout(location = 3) in vec4 iColor;
layout(location = 4) in vec2 iLight; // sky, block in 0..1
uniform mat4 uViewProj;
uniform vec3 uRight, uUp;
out vec4 vColor;
out vec2 vLight;
out float vDist;
void main() {
	vec3 p = iPos + (uRight * aCorner.x + uUp * aCorner.y) * iSize;
	gl_Position = uViewProj * vec4(p, 1.0);
	vColor = iColor;
	vLight = iLight;
	vDist = length(p);
}
`

const particleFS = `#version 330 core
in vec4 vColor;
in vec2 vLight;
in float vDist;
uniform float uDaylight;
uniform float uBlockBright;
uniform int uAdditive;
uniform vec3 uFogColor;
uniform vec2 uFog;
out vec4 fragColor;
float curve(float l) { return mix(0.04, 1.0, l / (4.0 - 3.0 * l)); }
float blockCurve(float l) { return mix(curve(l), mix(0.04, 1.0, 1.0 - pow(1.0 - l, 4.0)), uBlockBright); }
void main() {
	float f = clamp((vDist - uFog.x) / (uFog.y - uFog.x), 0.0, 1.0);
	if (uAdditive == 1) {
		fragColor = vec4(vColor.rgb * vColor.a * (1.0 - f * f), 1.0);
		return;
	}
	float sky = curve(vLight.x) * uDaylight;
	float blk = blockCurve(vLight.y);
	vec3 light = max(max(vec3(sky) * vec3(0.95, 0.97, 1.0), vec3(blk) * vec3(1.0, 0.86, 0.66)), vec3(0.04));
	fragColor = vec4(mix(vColor.rgb * light, uFogColor, f * f), vColor.a);
}
`

type gpuParticleRenderer struct {
	prog      *program
	vao, inst uint32
}

func (pr *ParticleRenderer) initGPU() error {
	p, err := newProgram(particleVS, particleFS)
	if err != nil {
		return err
	}
	g := &pr.gpu
	g.prog = p
	gl.GenVertexArrays(1, &g.vao)
	gl.BindVertexArray(g.vao)
	var quad uint32
	gl.GenBuffers(1, &quad)
	gl.BindBuffer(gl.ARRAY_BUFFER, quad)
	gl.BufferData(gl.ARRAY_BUFFER, len(particleCorners)*4, gl.Ptr(particleCorners), gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 8, 0)
	gl.GenBuffers(1, &g.inst)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.inst)
	for i, n := range []int32{3, 1, 4, 2} {
		off := []uintptr{0, 3, 4, 8}[i] * 4
		loc := uint32(i + 1)
		gl.EnableVertexAttribArray(loc)
		gl.VertexAttribPointerWithOffset(loc, n, gl.FLOAT, false, particleStride*4, off)
		gl.VertexAttribDivisor(loc, 1)
	}
	gl.BindVertexArray(0)
	return nil
}

func (pr *ParticleRenderer) begin(pf *particleFrame) {
	p := pr.gpu.prog
	p.use()
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &pf.vp[0])
	gl.Uniform3fv(p.loc("uRight"), 1, &pf.right[0])
	gl.Uniform3fv(p.loc("uUp"), 1, &pf.up[0])
	gl.Uniform1f(p.loc("uDaylight"), pr.r.Opt.Daylight)
	gl.Uniform1f(p.loc("uBlockBright"), pr.r.Opt.BlockBrightness)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &pf.fog.Color[0])
	gl.Uniform2f(p.loc("uFog"), pf.fog.Start, pf.fog.End)
	gl.BindVertexArray(pr.gpu.vao)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthMask(false)
	gl.Disable(gl.CULL_FACE)
	gl.Enable(gl.BLEND)
}

func (pr *ParticleRenderer) drawPass(_ *particleFrame, additive bool, buf []float32) {
	p := pr.gpu.prog
	if additive {
		gl.BlendFunc(gl.ONE, gl.ONE)
		gl.Uniform1i(p.loc("uAdditive"), 1)
	} else {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		gl.Uniform1i(p.loc("uAdditive"), 0)
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, pr.gpu.inst)
	gl.BufferData(gl.ARRAY_BUFFER, len(buf)*4, gl.Ptr(buf), gl.STREAM_DRAW)
	gl.DrawArraysInstanced(gl.TRIANGLES, 0, 6, int32(len(buf)/particleStride))
}

func (pr *ParticleRenderer) end() {
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.DepthMask(true)
	gl.BindVertexArray(0)
}
