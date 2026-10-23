package render

import (
	"math"
	"testing"
)

func TestFrustumBox(t *testing.T) {
	c := Camera{X: 1000, Y: 64, Z: -2000, FOV: math.Pi / 2, Aspect: 1, Near: 0.05, Far: 512}
	f := c.Frustum() // yaw 0 looks toward -Z
	if !f.Box(999, 64, -2011, 1001, 66, -2009) {
		t.Fatal("box ahead culled")
	}
	if f.Box(999, 64, -1991, 1001, 66, -1989) {
		t.Fatal("box behind drawn")
	}
	if f.Box(1030, 64, -2011, 1032, 66, -2009) {
		t.Fatal("box far right of a 90° view drawn")
	}
}
