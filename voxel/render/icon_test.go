package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The icon's centre lands on the asked screen point and its extent on the
// asked size.
func TestIconView(t *testing.T) {
	const aspect = 16.0 / 9
	lo, hi := mgl32.Vec3{0, 0, 0}, mgl32.Vec3{1, 2, 1}
	view, _ := IconView(0.5, -0.25, 0.4, aspect, lo, hi, 0.7, 0.4)
	proj := (&Renderer{Icons: true}).handProj(aspect)
	at := func(p mgl32.Vec3) mgl32.Vec3 { return proj.Mul4(view).Mul4x1(p.Vec4(1)).Vec3() }
	c := at(lo.Add(hi).Mul(0.5))
	if math.Abs(float64(c[0]-0.5)) > 1e-4 || math.Abs(float64(c[1]+0.25)) > 1e-4 {
		t.Fatalf("centre at %v, want 0.5,-0.25", c)
	}
	a, b := view.Mul4x1(lo.Vec4(1)), view.Mul4x1(hi.Vec4(1))
	if d := a.Vec3().Sub(b.Vec3()).Len(); math.Abs(float64(d-0.4)) > 1e-4 {
		t.Fatalf("extent %v, want 0.4", d)
	}
}
