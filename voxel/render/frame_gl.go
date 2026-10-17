//go:build !wgpu

package render

import (
	"image"
	"time"

	"github.com/memmaker/voxellib/platform/gl"
	"github.com/memmaker/voxellib/platform/glfw"
)

// Backend names the graphics backend this binary was built with.
const Backend = "gl"

// WindowHints sets the GLFW hints the backend needs; call before
// glfw.CreateWindow.
func WindowHints() {
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 0)
}

// Init binds the backend to a window; call before New.
func Init(win *glfw.Window, vsync bool) error {
	win.MakeContextCurrent()
	if vsync {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
	}
	return nil
}

// BeginFrame clears a w x h framebuffer to the given colour.
func BeginFrame(w, h int, r, g, b float32) {
	gl.Viewport(0, 0, int32(w), int32(h))
	gl.ClearColor(r, g, b, 1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
}

// EndFrame presents the frame.
func EndFrame(win *glfw.Window) { win.SwapBuffers() }

// CaptureNextFrame makes the next frame readable by ReadFrame; GL can
// always read the back buffer.
func CaptureNextFrame() {}

// ReadFrame reads the frame drawn so far into an opaque, top-down image.
func ReadFrame(w, h int) *image.NRGBA {
	px := make([]byte, w*h*4)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(img.Pix[y*w*4:(y+1)*w*4], px[(h-1-y)*w*4:(h-y)*w*4])
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

// GPUTimer measures GPU time with TIME_ELAPSED queries, reading results
// issued 3 frames earlier so the CPU never waits.
type GPUTimer struct {
	tq [4]uint32
	fq int
}

// Measure times f on the GPU and returns the time of an earlier
// measurement (0 until one is available).
func (t *GPUTimer) Measure(f func()) time.Duration {
	if t.tq[0] == 0 {
		gl.GenQueries(int32(len(t.tq)), &t.tq[0])
	}
	t.fq++
	qi := t.fq % len(t.tq)
	var ns uint64
	if t.fq > len(t.tq) {
		gl.GetQueryObjectui64v(t.tq[(qi+1)%len(t.tq)], gl.QUERY_RESULT, &ns)
	}
	gl.BeginQuery(gl.TIME_ELAPSED, t.tq[qi])
	f()
	gl.EndQuery(gl.TIME_ELAPSED)
	return time.Duration(ns)
}
