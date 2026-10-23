package nav

import "testing"

type grid map[Pos]bool

func (m grid) Solid(x, y, z int) bool { return m[Pos{x, y, z}] }

// floor makes a solid floor at y=0 over 0..9 x 0..9 (feet at y=1).
func floor() grid {
	m := grid{}
	for x := 0; x < 10; x++ {
		for z := 0; z < 10; z++ {
			m[Pos{x, 0, z}] = true
		}
	}
	return m
}

func TestPath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m grid, g *Graph)
		to    Pos
		ok    bool
	}{
		{"flat", func(m grid, g *Graph) {}, Pos{5, 1, 0}, true},
		{"step up", func(m grid, g *Graph) { m[Pos{3, 1, 0}], m[Pos{4, 1, 0}], m[Pos{5, 1, 0}] = true, true, true }, Pos{5, 2, 0}, true},
		{"drop 3 ok", func(m grid, g *Graph) {
			m[Pos{4, 0, 0}] = false
			m[Pos{4, -3, 0}] = true
			m[Pos{5, 0, 0}] = false
			m[Pos{5, -3, 0}] = true
		}, Pos{5, -2, 0}, true},
		{"drop 4 rejected", func(m grid, g *Graph) {
			m[Pos{4, 0, 0}] = false
			m[Pos{4, -4, 0}] = true
			m[Pos{5, 0, 0}] = false
			m[Pos{5, -4, 0}] = true
		}, Pos{5, -3, 0}, false},
		{"unreachable", func(m grid, g *Graph) {
			for x := 0; x < 10; x++ {
				m[Pos{x, 1, 1}], m[Pos{x, 2, 1}] = true, true
			}
		}, Pos{5, 1, 3}, false},
		{"door blocked", func(m grid, g *Graph) {
			for x := 0; x < 10; x++ {
				if x != 4 {
					m[Pos{x, 1, 1}], m[Pos{x, 2, 1}] = true, true
				}
			}
			g.Passable = func(from, to Pos) bool { return to != (Pos{4, 1, 1}) }
		}, Pos{4, 1, 3}, false},
	}
	for _, tc := range tests {
		m := floor()
		g := &Graph{W: m}
		tc.setup(m, g)
		p, ok := g.Path(Pos{0, 1, 0}, tc.to, 0)
		if tc.name == "step up" {
			p, ok = g.Path(Pos{1, 1, 0}, tc.to, 0)
		}
		if tc.name == "drop 3 ok" || tc.name == "drop 4 rejected" {
			p, ok = g.Path(Pos{3, 1, 0}, tc.to, 0)
		}
		if ok != tc.ok {
			t.Errorf("%s: ok=%v want %v (path %v)", tc.name, ok, tc.ok, p)
		}
		if ok && (p[0] == tc.to || p[len(p)-1] != tc.to) {
			t.Errorf("%s: bad endpoints %v", tc.name, p)
		}
	}
}

func TestNoCornerCut(t *testing.T) {
	m := floor()
	m[Pos{1, 1, 0}], m[Pos{1, 2, 0}] = true, true
	g := &Graph{W: m}
	for _, n := range g.Neighbors(Pos{0, 1, 1}, nil) {
		if n == (Pos{1, 1, 0}) {
			t.Fatal("diagonal cut corner")
		}
	}
	if len(g.Neighbors(Pos{0, 1, 1}, nil)) == 0 {
		t.Fatal("no neighbours")
	}
}

func TestStepUpLowCeiling(t *testing.T) {
	m := floor()
	m[Pos{3, 1, 0}] = true
	g := &Graph{W: m}
	has := func() bool {
		for _, n := range g.Neighbors(Pos{2, 1, 0}, nil) {
			if n == (Pos{3, 2, 0}) {
				return true
			}
		}
		return false
	}
	if !has() {
		t.Fatal("step up missing")
	}
	m[Pos{2, 3, 0}] = true // only 2 clear cells above current floor
	if has() {
		t.Fatal("step up allowed under low ceiling")
	}
}

// A wall across x=5 with a closed door at z=5: only the door lets through.
func TestDoorPassable(t *testing.T) {
	m := floor()
	door := Pos{5, 1, 5}
	for z := 0; z < 10; z++ {
		m[Pos{5, 1, z}], m[Pos{5, 2, z}], m[Pos{5, 3, z}] = true, true, true
	}
	g := &Graph{W: m}
	if _, ok := g.Path(Pos{1, 1, 1}, Pos{8, 1, 8}, 0); ok {
		t.Fatal("closed wall passed without door support")
	}
	g.Door = func(x, y, z int) bool { return x == door.X && z == door.Z && (y == 1 || y == 2) }
	p, ok := g.Path(Pos{1, 1, 1}, Pos{8, 1, 8}, 0)
	if !ok {
		t.Fatal("no path through the door")
	}
	through := false
	for _, c := range p {
		through = through || c == door
	}
	if !through {
		t.Fatalf("path %v skips the door", p)
	}
	s := g.Smooth(p)
	if len(s) >= len(p) {
		t.Fatalf("smoothing kept %d of %d", len(s), len(p))
	}
	for _, c := range s {
		if c == door {
			return
		}
	}
	// the door cell itself may be skipped only if a neighbour is kept
	for _, c := range s {
		if g.nearDoor(c) {
			return
		}
	}
	t.Fatalf("smoothed path %v lost the door", s)
}

func TestSwimAcross(t *testing.T) {
	m := floor()
	water := map[Pos]bool{}
	for z := 0; z < 10; z++ { // a 3-wide channel at x 4..6, 3 deep
		for x := 4; x <= 6; x++ {
			for y := -2; y <= 0; y++ {
				delete(m, Pos{x, y, z})
				water[Pos{x, y, z}] = true
			}
			m[Pos{x, -3, z}] = true
		}
	}
	g := &Graph{W: m}
	if p, ok := g.Path(Pos{1, 1, 1}, Pos{8, 1, 1}, 0); ok {
		t.Fatalf("crossed a channel without swimming: %v", p)
	}
	g.Water = func(x, y, z int) bool { return water[Pos{x, y, z}] }
	if _, ok := g.Path(Pos{1, 1, 1}, Pos{8, 1, 1}, 0); !ok {
		t.Fatal("no swim path")
	}
}

// TestStepSlices: a search run in slices of one expansion finds the same
// path as Path, and the cell cache never changes an answer.
func TestStepSlices(t *testing.T) {
	m := floor()
	for z := 0; z < 9; z++ { // a wall with a gap at z=9
		m[Pos{5, 1, z}], m[Pos{5, 2, z}] = true, true
	}
	g := &Graph{W: m}
	want, ok := g.Path(Pos{0, 1, 0}, Pos{9, 1, 0}, 0)
	if !ok {
		t.Fatal("no path")
	}
	if !g.Begin(Pos{0, 1, 0}, Pos{9, 1, 0}, 0) {
		t.Fatal("Begin")
	}
	var got []Pos
	steps := 0
	for done := false; !done; steps++ {
		got, done = g.Step(1)
	}
	if steps < 10 || len(got) != len(want) {
		t.Fatalf("sliced: %d steps, path %v, want %v", steps, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("sliced path %v, want %v", got, want)
		}
	}
	// unreachable: ends with no path after maxNodes expansions
	m[Pos{5, 1, 9}], m[Pos{5, 2, 9}] = true, true
	g.Begin(Pos{0, 1, 0}, Pos{9, 1, 0}, 7)
	if p, done := g.Step(100); !done || p != nil || g.Expanded() != 7 {
		t.Fatalf("capped search: path %v done %v expanded %d", p, done, g.Expanded())
	}
}
