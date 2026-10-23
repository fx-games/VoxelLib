package actors

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
)

// TestBigWalkerGoalWithoutRoom: a troll-sized walker whose goal lies at the
// bottom of a 1x1 shaft in solid ground (no room for its 2x2 footprint
// anywhere near) still heads there and ends at the shaft, as PathFinder
// keeps the nearest point.
func TestBigWalkerGoalWithoutRoom(t *testing.T) {
	w := flat(20)
	for x := -20; x < 20; x++ {
		for z := -20; z < 20; z++ {
			for y := -20; y < 0; y++ {
				w.solid[nav.Pos{X: x, Y: y, Z: z}] = true
			}
		}
	}
	for z := -20; z < 8; z++ { // a wall in the way, open beyond z 8
		for y := 1; y <= 5; y++ {
			w.solid[nav.Pos{X: 5, Y: y, Z: z}] = true
		}
	}
	for y := -19; y <= 0; y++ { // shaft at x 10, z 0
		delete(w.solid, nav.Pos{X: 10, Y: y, Z: 0})
	}
	m := NewManager(w, nil, nil, 1)
	a := New("troll", "troll", 0.5, 1, 0.5)
	a.Body.Width, a.Body.Height = 1.6, 3.2
	goal := Vec{10.5, -19, 0.5}
	a.Brain = Brain{Mode: Route, Points: []Vec{goal}}
	m.Add(a)
	for range 1200 {
		m.Tick()
	}
	if d := a.Pos().hdist(goal); d > 2.5 {
		t.Fatalf("troll %.1f blocks from a goal it cannot stand on, at %+v", d, a.Pos())
	}
}

// TestBigWalkerStartCell: the search starts at floor(minX) (PathFinder), so
// a troll whose box ends a hair past a ledge it stands on starts on that
// ledge, not in the air beside it.
func TestBigWalkerStartCell(t *testing.T) {
	m := NewManager(flat(4), nil, nil, 1)
	a := New("troll", "troll", 0, 1, 0)
	a.Body.Width, a.Body.Height = 1.6, 3.2
	g := m.graphFor(a)
	if p := m.start(g, Vec{10.79999, 1, 0.8}); p.X != 9 || p.Z != 0 {
		t.Fatalf("start cell %+v, want x 9 z 0 (floor of the min corner)", p)
	}
}
