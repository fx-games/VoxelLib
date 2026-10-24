package actors

import "testing"

// TestCarry: any actor carries a rider at 0.75 of its height, a carried
// actor carries the next; a mount with a rider takes no other.
func TestCarry(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	a, b, c := New("a", "x", 0.5, 1, 0.5), New("b", "x", 3.5, 1, 0.5), New("c", "x", 5.5, 1, 0.5)
	b.Body.Height, c.Body.Height = 0.5, 0.5
	for _, o := range []*Actor{a, b, c} {
		o.Health, o.MaxHealth = 20, 20
		m.Add(o)
	}
	if !m.Carry(b, a) || b.Riding != a || a.Rider != b || b.Body.Y != a.Body.Y+a.Body.Height*0.75 || b.Body.X != a.Body.X {
		t.Fatalf("b on a at %v", b.Body.Y)
	}
	if m.Carry(c, a) || !m.Carry(c, b) || c.Body.Y != b.Body.Y+0.5*0.75 {
		t.Fatalf("c on b at %v", c.Body.Y)
	}
	if m.Carry(a, a) {
		t.Fatal("carries itself")
	}
	m.Tick()
	if c.Body.Y != b.Body.Y+0.5*0.75 || b.Body.Y != a.Body.Y+a.Body.Height*0.75 {
		t.Fatal("not seated after a tick")
	}
}
