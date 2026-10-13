package render

import (
	"image"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// Overlay draws a 2D image over the frame in pixel coordinates, for HUD
// panels such as a help screen. The image is uploaded by SetImage and
// stays on the GPU until replaced.
type Overlay struct {
	prog *program
	vao  uint32
	tex  uint32
	w, h int
}

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

// NewOverlay creates an empty overlay.
func NewOverlay() (*Overlay, error) {
	p, err := newProgram(overlayVS, overlayFS)
	if err != nil {
		return nil, err
	}
	o := &Overlay{prog: p}
	gl.GenVertexArrays(1, &o.vao)
	return o, nil
}

// SetImage uploads the image to draw, into the same texture while the
// size stays (HUDs repaint every frame).
func (o *Overlay) SetImage(img image.Image) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if o.tex != 0 && w == o.w && h == o.h {
		gl.BindTexture(gl.TEXTURE_2D, o.tex)
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(toNRGBARect(img).Pix))
		return
	}
	if o.tex != 0 {
		gl.DeleteTextures(1, &o.tex)
	}
	o.tex = uploadTexture2D(img)
	o.w, o.h = w, h
}

// Size returns the image size in pixels.
func (o *Overlay) Size() (w, h int) { return o.w, o.h }

// Draw blends the image at pixel position x,y (top-left origin), scaled by
// an integer factor, on a framebuffer of fbW x fbH pixels.
func (o *Overlay) Draw(x, y, scale, fbW, fbH int) {
	if o.tex == 0 || fbW == 0 || fbH == 0 {
		return
	}
	nx := func(px int) float32 { return float32(px)/float32(fbW)*2 - 1 }
	ny := func(py int) float32 { return 1 - float32(py)/float32(fbH)*2 }
	o.prog.use()
	gl.Uniform4f(o.prog.loc("uRect"), nx(x), ny(y+o.h*scale), nx(x+o.w*scale), ny(y))
	gl.Uniform1i(o.prog.loc("uTex"), 0)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, o.tex)
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.BindVertexArray(o.vao)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindVertexArray(0)
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
}
