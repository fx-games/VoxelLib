package particles

import "testing"

func TestFallLandAndDie(t *testing.T) {
	var s System
	floor := func(x, y, z int) bool { return y < 0 }
	s.Emit(0.5, 2, 0.5, Props{Count: 50, Spread: 0.2, VelVar: 2, Gravity: 20, Bounce: 0.3, Life: 2, Size0: 0.1, Size1: 0.1})
	if len(s.P) != 50 {
		t.Fatalf("%d particles", len(s.P))
	}
	for i := 0; i < 30; i++ { // 1.5 s
		s.Tick(0.05, floor)
	}
	for _, p := range s.P {
		if p.Y < 0 {
			t.Fatalf("fell through the floor: y %.2f", p.Y)
		}
		if p.Y > 0.5 {
			t.Fatalf("still in the air after 1.5 s: y %.2f", p.Y)
		}
	}
	for i := 0; i < 20; i++ {
		s.Tick(0.05, floor)
	}
	if len(s.P) != 0 {
		t.Fatalf("%d particles outlived their life", len(s.P))
	}
}
