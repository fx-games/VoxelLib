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
