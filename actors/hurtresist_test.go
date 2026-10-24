package actors

import "testing"

// TestHurtResist: a hit sets hurtResistantTime to 20, each tick lowers it.
func TestHurtResist(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	a := New("a", "cow", 0.5, 1, 0.5)
	a.Health = 100
	m.Add(a)
	m.Hurt(a, nil, 1, false)
	if a.HurtResist() != MaxHurtResist {
		t.Fatalf("after a hit: %d", a.HurtResist())
	}
	for range 5 {
		m.Tick()
	}
	if a.HurtResist() != MaxHurtResist-5 {
		t.Fatalf("after 5 ticks: %d", a.HurtResist())
	}
}
