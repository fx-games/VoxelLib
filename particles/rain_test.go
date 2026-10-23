package particles

import (
	"math"
	"testing"
)

func TestRainSplashes(t *testing.T) {
	feet := [3]float64{100.5, 64, -20.5}
	ground := func(x, z int) Ground { return Ground{Top: 64, Rains: x != 100} }
	s := RainSplashes(7, 0.5, true, 0, feet, ground, nil)
	if len(s) == 0 || len(s) >= 25 { // 100 * 0.5², minus the dry column
		t.Fatalf("%d splashes", len(s))
	}
	for _, p := range s {
		if math.Abs(p.X-100) > 10 || math.Abs(p.Z+21) > 10 || math.Floor(p.X) == 100 || float32(p.Y) != 64.1 || p.Smoke {
			t.Fatalf("splash %+v", p)
		}
	}
	if again := RainSplashes(7, 0.5, true, 0, feet, ground, nil); len(again) != len(s) || again[0] != s[0] {
		t.Error("not seeded by tick")
	}
	if n := len(RainSplashes(7, 0.5, true, 2, feet, ground, nil)); n != 0 {
		t.Errorf("minimal particles: %d", n)
	}
	air := func(x, z int) Ground { return Ground{Top: 64, Rains: true, Air: true} }
	if n := len(RainSplashes(7, 1, true, 0, feet, air, nil)); n != 0 {
		t.Errorf("splashes on air: %d", n)
	}
}
