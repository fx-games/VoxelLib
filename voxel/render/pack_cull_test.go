package render

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestCasterHull(t *testing.T) {
	h := convexHull([][2]float32{{0, 0}, {2, 0}, {1, 1}, {2, 2}, {0, 2}})
	if len(h) != 4 {
		t.Fatalf("hull %v", h)
	}
	if !overHull(h, [2]float32{1.5, 1.5}, [2]float32{3, 3}) || overHull(h, [2]float32{3, 0}, [2]float32{4, 1}) {
		t.Error("overHull: box/square overlap wrong")
	}
	tri := convexHull([][2]float32{{0, 0}, {4, 0}, {0, 4}})
	if overHull(tri, [2]float32{2.5, 2.5}, [2]float32{3, 3}) { // in the AABB, past the slanted edge
		t.Error("overHull misses the slanted edge")
	}
	// camera looking north (-z), light straight down
	view := mgl32.LookAtV(mgl32.Vec3{}, mgl32.Vec3{0, 0, -1}, mgl32.Vec3{0, 1, 0})
	proj := mgl32.Perspective(mgl32.DegToRad(70), 16.0/9, 0.05, 512)
	u := packUniforms{ModelViewInv: view.Inv(), ProjInv: proj.Inv(),
		ShadowMV: mgl32.LookAtV(mgl32.Vec3{0, 100, 0}, mgl32.Vec3{}, mgl32.Vec3{0, 0, -1})}
	pk := &packGPU{u: u, hull: casterHull(&u)}
	if !pk.casts(-8, -40, -60) { // ahead, below the view: it shades the ground there
		t.Error("a section ahead casts no shadow")
	}
	if !pk.casts(-8, 30, -30) { // above the view ahead
		t.Error("a section over the view casts no shadow")
	}
	if pk.casts(-8, 0, 40) || pk.casts(-8, 60, 40) { // behind the camera
		t.Error("a section behind the camera casts")
	}
}
