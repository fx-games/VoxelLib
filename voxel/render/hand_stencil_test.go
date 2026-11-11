package render

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// quad is a model of one view-space rectangle x0..x1, y0..y1 at z = -1.
func quad(x0, y0, x1, y1 float32) *model.Model {
	v := func(x, y float32) model.Vertex {
		return model.Vertex{Pos: mgl32.Vec3{x, y, -1}, Normal: mgl32.Vec3{0, 0, 1}, Weights: [4]float32{1}}
	}
	return &model.Model{Name: "quad", Meshes: []model.Mesh{{
		Verts: []model.Vertex{v(x0, y0), v(x1, y0), v(x1, y1), v(x0, y1)}, Indices: []uint32{0, 1, 2, 0, 2, 3}, Texture: -1}}}
}

// TestHandStencil draws a stencil-only mask over the left half, then a
// red quad over the whole view where the stencil is 1: only the left
// half may turn red. A second call must start on a cleared stencil.
func TestHandStencil(t *testing.T) {
	if !gpuOK {
		t.Skip("no GPU device")
	}
	const size = 64
	mr, err := NewModelRenderer(&Renderer{})
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Destroy()
	mask, err := mr.Add(quad(-2, -2, 0, 2))
	if err != nil {
		t.Fatal(err)
	}
	full, err := mr.Add(quad(-2, -2, 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	red := [4]float32{1, 0, 0, 1}
	writeMask := ModelInstance{Model: mask, Emissive: true,
		Stencil: HandStencil{Test: true, Compare: StencilAlways, Ref: 1, Pass: StencilReplace, NoColor: true, NoDepthWrite: true}}
	onMask := ModelInstance{Model: full, Emissive: true, Color: red,
		Stencil: HandStencil{Test: true, Compare: StencilEqual, Ref: 1}}

	CaptureNextFrame()
	BeginFrame(size, size, 0, 0, 0)
	mr.DrawHandInstance(1, mgl32.Ident4(), writeMask, onMask)
	img := ReadFrame(size, size)
	EndFrame(nil)
	if c := img.NRGBAAt(size/4, size/2); c.R < 200 || c.G > 50 {
		t.Errorf("inside the stencil: %v, want red", c)
	}
	if c := img.NRGBAAt(3*size/4, size/2); c.R > 50 {
		t.Errorf("outside the stencil: %v, want black", c)
	}

	// The stencil is cleared per call: alone, onMask draws nothing.
	CaptureNextFrame()
	BeginFrame(size, size, 0, 0, 0)
	mr.DrawHandInstance(1, mgl32.Ident4(), onMask)
	img = ReadFrame(size, size)
	EndFrame(nil)
	if c := img.NRGBAAt(size/4, size/2); c.R > 50 {
		t.Errorf("new call, stencil not cleared: %v, want black", c)
	}
}
