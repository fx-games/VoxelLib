package actors

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
)

// TestPathEnds: an open cell is reached, and one in the air or the ground
// over it (the path ends in its column); a cell walled in all round is
// not (the path stops at the nearest cell outside).
func TestPathEnds(t *testing.T) {
	w := flat(20)
	for x := 8; x <= 12; x++ {
		for z := 8; z <= 12; z++ {
			if x == 8 || x == 12 || z == 8 || z == 12 {
				for y := 1; y <= 3; y++ {
					w.solid[nav.Pos{X: x, Y: y, Z: z}] = true
				}
			}
		}
	}
	m := NewManager(w, nil, nil, 1)
	a := New("a", "a", 0.5, 1, 0.5)
	a.AggroRange = 16
	for _, y := range []float64{1, 5, -3} {
		if !m.PathEnds(a, Vec{X: 5, Y: y, Z: -3}) {
			t.Error("open column not reached at y", y)
		}
	}
	if m.PathEnds(a, Vec{X: 10, Y: 1, Z: 10}) {
		t.Error("walled-in cell reached")
	}
}

// TestPathToward: getPathToEntityLiving is a path when the walker stands
// and can move nearer; none in the air, and none from a one-cell pocket
// (no cell nearer than its own).
func TestPathToward(t *testing.T) {
	w := flat(20)
	for x := 9; x <= 11; x++ { // a one-cell pocket at 10,10
		for z := 9; z <= 11; z++ {
			if x != 10 || z != 10 {
				for y := 1; y <= 3; y++ {
					w.solid[nav.Pos{X: x, Y: y, Z: z}] = true
				}
			}
		}
	}
	m := NewManager(w, nil, nil, 1)
	a := New("a", "a", 0.5, 1, 0.5)
	a.AggroRange = 16
	tg := New("t", "t", 5.5, 1, -3.5)
	a.Body.OnGround = true
	if !m.PathToward(a, tg) {
		t.Error("no path on open ground")
	}
	if a.Body.OnGround = false; m.PathToward(a, tg) {
		t.Error("path from the air (canNavigate)")
	}
	in := New("i", "i", 10.5, 1, 10.5)
	in.AggroRange, in.Body.OnGround = 16, true
	if m.PathToward(in, tg) {
		t.Error("path out of a walled cell")
	}
}
