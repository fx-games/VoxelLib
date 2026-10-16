package render

import (
	"github.com/memmaker/voxellib/platform/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/particles"
)

// ParticleRenderer draws particles (package particles) as camera-facing
// squares, one instanced quad each. Normal particles are lit like terrain
// and alpha blended; additive ones glow. Neither writes depth, and alpha
// particles are not sorted (small and short-lived, so it rarely shows).
type ParticleRenderer struct {
	r         *Renderer
	prog      *program
	vao, inst uint32
	buf       []float32
}

const particleStride = 10 // rel pos 3, size 1, rgba 4, light 2

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

// NewParticleRenderer creates the particle renderer.
func NewParticleRenderer(r *Renderer) (*ParticleRenderer, error) {
	p, err := newProgram(particleVS, particleFS)
	if err != nil {
		return nil, err
	}
	pr := &ParticleRenderer{r: r, prog: p}
	gl.GenVertexArrays(1, &pr.vao)
	gl.BindVertexArray(pr.vao)
	var quad uint32
	gl.GenBuffers(1, &quad)
	gl.BindBuffer(gl.ARRAY_BUFFER, quad)
	corners := []float32{-0.5, -0.5, 0.5, -0.5, 0.5, 0.5, -0.5, -0.5, 0.5, 0.5, -0.5, 0.5}
	gl.BufferData(gl.ARRAY_BUFFER, len(corners)*4, gl.Ptr(corners), gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 8, 0)
	gl.GenBuffers(1, &pr.inst)
	gl.BindBuffer(gl.ARRAY_BUFFER, pr.inst)
	for i, n := range []int32{3, 1, 4, 2} {
		off := []uintptr{0, 3, 4, 8}[i] * 4
		loc := uint32(i + 1)
		gl.EnableVertexAttribArray(loc)
		gl.VertexAttribPointerWithOffset(loc, n, gl.FLOAT, false, particleStride*4, off)
		gl.VertexAttribDivisor(loc, 1)
	}
	gl.BindVertexArray(0)
	return pr, nil
}

// Draw renders the particles, interpolated alpha (0..1) between ticks.
// light gives the sky and block light (0..15) at a world position. Call
// after the opaque world and models, with the same camera.
func (pr *ParticleRenderer) Draw(cam *Camera, ps []particles.Particle, alpha float64, light func(x, y, z float64) (sky, block uint8)) {
	if len(ps) == 0 {
		return
	}
	p := pr.prog
	p.use()
	vp := cam.ViewProj()
	f := cam.Forward()
	right := f.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
	up := right.Cross(f).Normalize()
	fog := pr.r.CurrentFog()
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &vp[0])
	gl.Uniform3fv(p.loc("uRight"), 1, &right[0])
	gl.Uniform3fv(p.loc("uUp"), 1, &up[0])
	gl.Uniform1f(p.loc("uDaylight"), pr.r.Opt.Daylight)
	gl.Uniform1f(p.loc("uBlockBright"), pr.r.Opt.BlockBrightness)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &fog.Color[0])
	gl.Uniform2f(p.loc("uFog"), fog.Start, fog.End)
	gl.BindVertexArray(pr.vao)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthMask(false)
	gl.Disable(gl.CULL_FACE)
	gl.Enable(gl.BLEND)
	for _, additive := range []bool{false, true} {
		pr.buf = pr.buf[:0]
		for i := range ps {
			q := &ps[i]
			if q.Additive != additive {
				continue
			}
			x, y, z, size, col := q.Lerp(alpha)
			var sky, blk uint8 = 15, 15
			if !additive && light != nil {
				sky, blk = light(x, y, z)
			}
			pr.buf = append(pr.buf, float32(x-cam.X), float32(y-cam.Y), float32(z-cam.Z), size,
				col[0], col[1], col[2], col[3], float32(sky)/15, float32(blk)/15)
		}
		n := len(pr.buf) / particleStride
		if n == 0 {
			continue
		}
		if additive {
			gl.BlendFunc(gl.ONE, gl.ONE)
			gl.Uniform1i(p.loc("uAdditive"), 1)
		} else {
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
			gl.Uniform1i(p.loc("uAdditive"), 0)
		}
		gl.BindBuffer(gl.ARRAY_BUFFER, pr.inst)
		gl.BufferData(gl.ARRAY_BUFFER, len(pr.buf)*4, gl.Ptr(pr.buf), gl.STREAM_DRAW)
		gl.DrawArraysInstanced(gl.TRIANGLES, 0, 6, int32(n))
	}
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.DepthMask(true)
	gl.BindVertexArray(0)
}
