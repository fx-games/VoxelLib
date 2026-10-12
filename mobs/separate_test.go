package mobs

import "testing"

type noSolids struct{}

func (noSolids) Solid(x, y, z int) bool { return y < 0 }

func TestSeparate(t *testing.T) {
	a := New(Walker, 0, 0, 0, 0.6, 1.8)
	b := New(Walker, 0.1, 0, 0, 0.6, 1.8)
	wall := New(Walker, 5, 0, 0, 0.6, 1.8)
	wall.Static = true
	c := New(Walker, 5.2, 0, 0, 0.6, 1.8)
	ms := []*Mob{a, b, wall, c}
	for i := 0; i < 100; i++ {
		Separate(ms, noSolids{})
	}
	if d := b.X - a.X; d < 0.59 {
		t.Errorf("a and b still overlap: gap %.3f", d)
	}
	if wall.X != 5 {
		t.Errorf("static mob moved to %.3f", wall.X)
	}
	if c.X-wall.X < 0.59 {
		t.Errorf("c not pushed off the static mob: %.3f", c.X)
	}
}
