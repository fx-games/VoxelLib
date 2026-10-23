package raster

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// square is a unit quad in z = z facing +z, as two triangles.
func square(z float32) model.Mesh {
	v := func(x, y, u, w float32) model.Vertex {
		return model.Vertex{Pos: mgl32.Vec3{x, y, z}, Normal: mgl32.Vec3{0, 0, 1}, UV: mgl32.Vec2{u, w}, Weights: [4]float32{1}}
	}
	return model.Mesh{Verts: []model.Vertex{v(0, 0, 0, 0), v(1, 0, 1, 0), v(1, 1, 1, 1), v(0, 1, 0, 1)}, Indices: []uint32{0, 1, 2, 0, 2, 3}}
}

func TestDraw(t *testing.T) {
	tex := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	tex.SetNRGBA(0, 0, color.NRGBA{200, 100, 50, 255})
	tex.SetNRGBA(1, 0, color.NRGBA{255, 255, 255, 0}) // cut by the alpha test
	m := &model.Model{Meshes: []model.Mesh{square(0)}}
	tg := NewTarget(8, 8)
	// 4 pixels per unit, offset by 2: covers pixels 2..5 exactly.
	view := mgl32.Translate3D(2, 2, 0).Mul4(mgl32.Scale3D(4, 4, 1))
	l := Lighting{Ambient: 0.4, Lights: []Light{{Dir: mgl32.Vec3{0, 0, 1}, Diffuse: 0.6}}}
	tg.Draw(m, []mgl32.Mat4{mgl32.Ident4()}, view, tex, [4]float32{1, 1, 1, 1}, l)
	n := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := tg.Img.NRGBAAt(x, y)
			inside := x >= 2 && x < 6 && y >= 2 && y < 6
			if c.A != 0 {
				n++
				if !inside || x >= 4 {
					t.Errorf("pixel %d,%d drawn", x, y)
				}
				if c != (color.NRGBA{200, 100, 50, 255}) { // lit 0.4+0.6 = 1
					t.Errorf("pixel %d,%d = %v", x, y, c)
				}
			}
		}
	}
	if n != 8 { // the left half; the right half is alpha 0
		t.Errorf("%d pixels drawn, want 8", n)
	}

	// Half lit (n.L = 0.5): 0.4 + 0.3 = 0.7; a nearer square wins.
	l.Lights[0].Dir = mgl32.Vec3{0, float32(math.Sqrt(3)) / 2, 0.5}
	tg.Draw(&model.Model{Meshes: []model.Mesh{square(1)}}, []mgl32.Mat4{mgl32.Ident4()}, view, nil, [4]float32{1, 1, 1, 1}, l)
	if c := tg.Img.NRGBAAt(2, 2); c != (color.NRGBA{179, 179, 179, 255}) {
		t.Errorf("nearer square %v, want 0.7 grey", c)
	}
	tg.Draw(&model.Model{Meshes: []model.Mesh{square(-1)}}, []mgl32.Mat4{mgl32.Ident4()}, view, nil, [4]float32{0, 0, 0, 1}, l)
	if c := tg.Img.NRGBAAt(2, 2); c.R != 179 {
		t.Errorf("farther square drew over: %v", c)
	}
}
