package extrude

import (
	"image"
	"image/color"
	"testing"
)

func cross(a, b, c [3]float32) [3]float32 {
	u := [3]float32{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v := [3]float32{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	return [3]float32{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
}

func TestExtrude(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	// a 2x1 bar: 6 side faces
	img.Set(3, 4, color.NRGBA{255, 255, 255, 255})
	img.Set(4, 4, color.NRGBA{255, 255, 255, 255})
	m := Extrude(img)
	if got, want := len(m.Verts), (2+6)*6; got != want {
		t.Fatalf("verts %d, want %d", got, want)
	}
	for i := 0; i < len(m.Verts); i += 3 {
		a, b, c := m.Verts[i], m.Verts[i+1], m.Verts[i+2]
		n := cross(a.Pos, b.Pos, c.Pos)
		if d := n[0]*a.Normal[0] + n[1]*a.Normal[1] + n[2]*a.Normal[2]; d <= 0 {
			t.Fatalf("triangle %d winds against its normal %v", i/3, a.Normal)
		}
		for _, v := range []Vertex{a, b, c} {
			for k := 0; k < 3; k++ {
				if v.Pos[k] < -0.5 || v.Pos[k] > 0.5 {
					t.Fatalf("vertex out of unit cube: %v", v.Pos)
				}
			}
		}
	}
}
