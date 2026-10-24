package actors

import "testing"

// TestKnockDiv: a KnockDiv of 4 leaves a quarter of knockBack's motion.
func TestKnockDiv(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	src := New("src", "x", 0.5, 1, 0.5)
	a, b := New("a", "x", 2.5, 1, 0.5), New("b", "x", 2.5, 1, 4.5)
	b.KnockDiv = 4
	for _, o := range []*Actor{src, a, b} {
		o.Health, o.MaxHealth = 20, 20
		m.Add(o)
	}
	m.Hurt(a, src, 1, false)
	b.Body.X, b.Body.Z = a.Body.X, a.Body.Z
	m.Hurt(b, src, 1, false)
	if a.Body.VX == 0 || b.Body.VX != a.Body.VX/4 || b.Body.VY != a.Body.VY/4 {
		t.Fatalf("motion %v,%v vs %v,%v", b.Body.VX, b.Body.VY, a.Body.VX, a.Body.VY)
	}
}
