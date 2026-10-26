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
	proj := mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
	at := func(p mgl32.Vec3) mgl32.Vec3 {
		q := proj.Mul4(view).Mul4x1(p.Vec4(1))
		return q.Vec3().Mul(1 / q[3])
	}
	c := at(lo.Add(hi).Mul(0.5))
	if math.Abs(float64(c[0]-0.5)) > 1e-4 || math.Abs(float64(c[1]+0.25)) > 1e-4 {
		t.Fatalf("centre at %v, want 0.5,-0.25", c)
	}
	// The diagonal is the size, scaled with the icon's distance from the
	// eye (2 straight ahead).
	a, b := view.Mul4x1(lo.Vec4(1)), view.Mul4x1(hi.Vec4(1))
	d := a.Vec3().Sub(b.Vec3()).Len()
	dist := view.Mul4x1(lo.Add(hi).Mul(0.5).Vec4(1)).Vec3().Len()
	want := 0.4 * 2 * float32(math.Tan(35*math.Pi/180)) * dist / 2
	if math.Abs(float64(d-want)) > 1e-4 {
		t.Fatalf("extent %v, want %v", d, want)
	}
}
