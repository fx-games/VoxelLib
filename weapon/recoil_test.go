package weapon

import (
	"math"
	"testing"
)

// The spline passes through its points, is a line for two, and is natural
// (zero second derivative at the ends).
func TestSpline(t *testing.T) {
	x, y := []float64{0, 0.13, 0.18, 0.48, 0.63}, []float64{0, 1.4, 0.8, -0.175, 0}
	s := newSpline(x, y)
	for i := range x {
		if v := s.at(x[i]); math.Abs(v-y[i]) > 1e-9 {
			t.Fatalf("at %v: %v, want %v", x[i], v, y[i])
		}
	}
	second := func(t0 float64) float64 { const e = 1e-4; return (s.at(t0+e) - 2*s.at(t0) + s.at(t0-e)) / (e * e) }
	if d := second(1e-4); math.Abs(d) > 1 {
		t.Fatalf("not natural at the start: %v", d)
	}
	l := newSpline([]float64{0, 1}, []float64{0, 2})
	if v := l.at(0.25); math.Abs(v-0.5) > 1e-9 {
		t.Fatalf("line: %v", v)
	}
}
