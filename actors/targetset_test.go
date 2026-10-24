package actors

import "testing"

// TestTargetSet: a target TargetSet refuses is dropped as it is set, so
// the actor never swings at it; one it accepts is fought.
func TestTargetSet(t *testing.T) {
	m := NewManager(flat(40), nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	o := New("o", "npc", 1.5, 1, 0.5)
	a.Melee.Damage, a.AggroRange = 1, 16
	m.Add(a)
	m.Add(o)
	m.AlsoTargets = func(x, y *Actor) bool { return x == a && y == o }
	m.Keeps = m.AlsoTargets
	ok, hp := false, o.Health
	m.TargetSet = func(x, y *Actor) bool { return ok }
	for range 20 {
		if m.fight(a); a.Target != nil || o.Health != hp {
			t.Fatal("a refused target was kept")
		}
		m.ticks++
	}
	ok = true
	for range 20 {
		m.fight(a)
		m.ticks++
	}
	if a.Target != o || o.Health == hp {
		t.Fatal("an accepted target was not fought")
	}
}
