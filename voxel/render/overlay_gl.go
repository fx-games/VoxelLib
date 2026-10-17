//go:build !wgpu

package render

import "github.com/memmaker/voxellib/platform/gl"

const overlayVS = `#version 330 core
uniform vec4 uRect; // x0, y0, x1, y1 in NDC
out vec2 vUV;
void main() {
	vec2 c = vec2(float(gl_VertexID & 1), float((gl_VertexID >> 1) & 1));
	vUV = vec2(c.x, 1.0 - c.y);
	gl_Position = vec4(mix(uRect.xy, uRect.zw, c), 0.0, 1.0);
}
`

const overlayFS = `#version 330 core
in vec2 vUV;
uniform sampler2D uTex;
out vec4 fragColor;
void main() { fragColor = texture(uTex, vUV); }
`

type gpuOverlay struct {
	prog *program
	vao  uint32
	tex  uint32
}

func (o *Overlay) initGPU() error {
	p, err := newProgram(overlayVS, overlayFS)
	if err != nil {
		return err
	}
	o.gpu.prog = p
	gl.GenVertexArrays(1, &o.gpu.vao)
	return nil
}

func (o *Overlay) hasImage() bool { return o.gpu.tex != 0 }

// setPixels uploads tightly packed RGBA; same means the size is unchanged.
func (o *Overlay) setPixels(px []byte, w, h int, same bool) {
	if o.gpu.tex != 0 && same {
		gl.BindTexture(gl.TEXTURE_2D, o.gpu.tex)
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
		return
	}
	if o.gpu.tex != 0 {
		gl.DeleteTextures(1, &o.gpu.tex)
	}
	gl.GenTextures(1, &o.gpu.tex)
	gl.BindTexture(gl.TEXTURE_2D, o.gpu.tex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(w), int32(h), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
}

func (o *Overlay) drawRect(rect [4]float32) {
	o.gpu.prog.use()
	gl.Uniform4f(o.gpu.prog.loc("uRect"), rect[0], rect[1], rect[2], rect[3])
	gl.Uniform1i(o.gpu.prog.loc("uTex"), 0)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, o.gpu.tex)
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.BindVertexArray(o.gpu.vao)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindVertexArray(0)
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
}
