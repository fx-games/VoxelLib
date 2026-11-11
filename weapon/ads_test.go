package weapon

import (
	"encoding/json"
	"math"
	"testing"
)

// AdsCurve.evaluate: the window, clamping, defaults and camelCase names.
func TestAdsCurve(t *testing.T) {
	var p AdsPath
	if err := json.Unmarshal([]byte(`{"z":{"start":0.2,"end":1,"ease":"easeInOutCubic"},"x":{"ease":"linear","clamp":false}}`), &p); err != nil {
		t.Fatal(err)
	}
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }
	if !p.Valid() || !near(p.Z.At(0.1), 0) || !near(p.Z.At(0.6), 0.5) || !near(p.Y.At(0.3), 0.3) || !near(p.X.At(1.5), 1.5) || !near(p.Z.At(0.4), 4*0.25*0.25*0.25) {
		t.Errorf("path %+v: z(0.6)=%v z(0.4)=%v x(1.5)=%v", p, p.Z.At(0.6), p.Z.At(0.4), p.X.At(1.5))
	}
	var c *AdsFovChannel
	if !near(c.FovProgress(0.5), float32(math.Pow(2, -5))) || !c.Smoothed() {
		t.Errorf("no curve: easeInExpo, smoothed")
	}
}
