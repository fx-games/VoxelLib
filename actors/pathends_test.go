package actors

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
)

// TestPathEnds: an open cell is reached; a cell walled in all round is
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
	if !m.PathEnds(a, Vec{X: 5, Y: 1, Z: -3}) {
		t.Error("open cell not reached")
	}
	if m.PathEnds(a, Vec{X: 10, Y: 1, Z: 10}) {
		t.Error("walled-in cell reached")
	}
}
