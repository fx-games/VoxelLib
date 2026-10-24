package guientity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// func_147046_a with mouse offsets dx, dy (Java, float math).
func TestAngles(t *testing.T) {
	for _, d := range [][2]float32{{0, 0}, {30, -20}, {-75, 40}} {
		yo, yaw, pitch := Angles(d[0], d[1])
		jx := float32(math.Atan(float64(d[0] / 40)))
		jy := float32(math.Atan(float64(d[1] / 40)))
		if yo != jx*20 || yaw != jx*40 || pitch != -jy*20 {
			t.Errorf("%v: %v %v %v", d, yo, yaw, pitch)
		}
	}
}

// TestRegion: func_147046_a's entity is clipped only by the screen (GL),
// however far it reaches; the box rounds up to 32 within it.
func TestRegion(t *testing.T) {
	v := func(x, y float32) model.Vertex {
		return model.Vertex{Pos: mgl32.Vec3{x, y, 0}, Weights: [4]float32{1}}
	}
	m := &model.Model{Meshes: []model.Mesh{{Verts: []model.Vertex{v(-150.5, -20), v(10, 230)}}}}
	ds := []Draw{{Model: m, Skin: []mgl32.Mat4{mgl32.Ident4()}}}
	view := mgl32.Translate3D(200, 0, 0)
	if l, tp, r, b, ok := Region(view, ds, 427, 240); !ok || l != 49 || tp != 0 || r != 49+192 || b != 240 {
		t.Fatalf("%d %d %d %d %v", l, tp, r, b, ok)
	}
	if _, _, _, _, ok := Region(mgl32.Translate3D(1000, 0, 0), ds, 427, 240); ok {
		t.Fatal("off-screen draw has a region")
	}
}
