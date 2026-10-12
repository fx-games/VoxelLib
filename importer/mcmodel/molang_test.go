package mcmodel

import (
	"math"
	"testing"
)

func TestMolang(t *testing.T) {
	for src, want := range map[string]float64{
		"1+2*3": 7,
		"-math.sin(query.anim_time * 180) * 40 + 17": 17 - math.Sin(0.5*math.Pi)*40,
		"28.58421/2": 14.292105,
		"-40-math.cos(query.anim_time * 180 ) * 10-NaN": -40 - math.Cos(0.5*math.Pi)*10,
		"q.anim_time > 0.25 ? 3 : 4":                    3,
		"math.clamp(5, 0, 2)":                           2,
		"return 2;":                                     2,
		"variable.x + 1":                                1,
	} {
		e, err := ParseMolang(src)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.Eval(&Ctx{AnimTime: 0.5}); math.Abs(got-want) > 1e-6 {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	if _, err := ParseMolang("1 +"); err == nil {
		t.Error("expected error")
	}
}
