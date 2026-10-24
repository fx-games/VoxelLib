package render

import (
	"math"
	"testing"
)

// TestShadowQuads: a 0.5 shadow (lift 0.9) at (0.25, 64, 0.5) over a floor
// at y 63 covers cells x -1..0, z 0 at 64 + 1/64, clipped to the
// square, alpha (1 − 0.9/2)/2 with full brightness; the UVs span 0..1
// over the square. Hovering 0.4 higher, the alpha drops by 0.2/2; with dark
// cells (ground false) nothing is drawn.
func TestShadowQuads(t *testing.T) {
	cam := &Camera{X: 10, Y: 70, Z: -5}
	floor := func(x, y, z int) (float32, bool) { return 1, y == 64 }
	s := Shadow{X: 0.25, Y: 64, Z: 0.5, Lift: 0.9, Size: 0.5, Opaque: 1}
	v := ShadowQuads(nil, cam, s, floor)
	if len(v) != 2*4 { // z 1 clips to nothing
		t.Fatalf("%d verts", len(v))
	}
	for _, p := range v {
		x, y, z := float64(p.Pos[0])+cam.X, float64(p.Pos[1])+cam.Y, float64(p.Pos[2])+cam.Z
		if math.Abs(y-64.015625) > 1e-4 || x < -0.25-1e-5 || x > 0.75+1e-5 || z < -1e-5 || z > 1+1e-5 {
			t.Fatalf("vertex %v %v %v", x, y, z)
		}
		if u := (s.X-x)/2/s.Size + 0.5; math.Abs(float64(p.UV[0])-u) > 1e-5 || p.UV[0] < -1e-5 || p.UV[0] > 1+1e-5 {
			t.Fatalf("uv %v at x %v", p.UV, x)
		}
		if math.Abs(float64(p.Color[3])-0.275) > 1e-6 {
			t.Fatalf("alpha %v", p.Color[3])
		}
	}
	s.Y = 64.4 // centre 65.3: cells 64 and 65; the floor's cell is 1.3 below it
	if v := ShadowQuads(nil, cam, s, floor); len(v) != 8 || math.Abs(float64(v[0].Color[3])-(1-1.3/2)/2) > 1e-6 {
		t.Fatalf("hovering %d %v", len(v), v)
	}
	if v := ShadowQuads(nil, cam, s, func(x, y, z int) (float32, bool) { return 0, false }); len(v) != 0 {
		t.Fatal("dark")
	}
}
