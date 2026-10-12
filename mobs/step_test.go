package mobs

import (
	"math/rand"
	"testing"
)

// step is flat ground at y<0 with a one-block ledge from z<=-3.
type step struct{}

func (step) Solid(x, y, z int) bool { return y < 0 || (y == 0 && z <= -3) }

func TestWalkerClimbsStep(t *testing.T) {
	for _, w := range []float64{0.6, 1.0, 1.4} {
		m := New(Walker, 0.5, 0, 0.5, w, 1.9)
		m.tx, m.tz, m.hasTarget, m.timer = 0.5, -8, true, 400
		rnd := rand.New(rand.NewSource(1))
		for i := 0; i < 200 && m.Z > -5; i++ {
			m.Tick(step{}, rnd)
			m.tx, m.tz, m.hasTarget = 0.5, -8, true
		}
		if m.Z > -5 || m.Y < 1 {
			t.Errorf("width %.1f: stuck at z=%.2f y=%.2f", w, m.Z, m.Y)
		}
	}
}
