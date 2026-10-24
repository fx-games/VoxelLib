package actors

import "testing"

// TestAlsoTargets: AlsoTargets lets an actor pick a target it does not
// see as hostile; without it, a neutral one is never picked.
func TestAlsoTargets(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	o := New("o", "npc", 2.5, 1, 0.5)
	m.Add(a)
	m.Add(o)
	if m.nearestHostile(a, 16) != nil {
		t.Fatal("picked a neutral actor")
	}
	m.AlsoTargets = func(x, y *Actor) bool { return x == a && y == o }
	if m.nearestHostile(a, 16) != o {
		t.Fatal("AlsoTargets ignored")
	}
}
