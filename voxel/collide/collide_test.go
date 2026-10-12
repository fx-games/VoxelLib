package collide

import "testing"

// floor: everything below y=0 is solid.
var floor = SolidsFunc(func(x, y, z int) bool { return y < 0 })

func TestMoveLandsOnFloor(t *testing.T) {
	box := Box(0.5, 2, 0.5, 0.6, 1.8)
	_, dy, _ := Move(floor, box, 0, -5, 0)
	if dy != -2 {
		t.Fatalf("dy = %v, want -2", dy)
	}
}

func TestMoveBlockedByWall(t *testing.T) {
	wall := SolidsFunc(func(x, y, z int) bool { return x >= 2 })
	box := Box(0.5, 0, 0.5, 0.6, 1.8)
	dx, _, _ := Move(wall, box, 3, 0, 0)
	if want := 2 - 0.8; dx < want-1e-9 || dx > want+1e-9 {
		t.Fatalf("dx = %v, want %v", dx, want)
	}
}

func TestFits(t *testing.T) {
	if Fits(floor, Box(0.5, -0.5, 0.5, 0.6, 1.8)) {
		t.Fatal("box in the floor fits")
	}
	if !Fits(floor, Box(0.5, 0, 0.5, 0.6, 1.8)) {
		t.Fatal("box on the floor doesn't fit")
	}
}
