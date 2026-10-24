package actors

import "testing"

// TestTargetSight: a check-sight target task picks only a seen target,
// keeps it 60 ticks unseen and drops it on the 61st, and drops it
// beyond AggroRange (EntityAITarget.continueExecuting).
func TestTargetSight(t *testing.T) {
	m := NewManager(flat(40), nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	o := New("o", "npc", 6.5, 1, 0.5)
	a.Melee.Damage, a.TargetSight, a.AggroRange = 1, true, 16
	m.Add(a)
	m.Add(o)
	seen := false
	m.Sees = func(x, y *Actor) bool { return seen }
	m.AlsoTargets = func(x, y *Actor) bool { return x == a && y == o }
	m.Keeps = m.AlsoTargets
	if m.pickTarget(a) != nil {
		t.Fatal("picked an unseen target")
	}
	seen = true
	if a.Target = m.pickTarget(a); a.Target != o {
		t.Fatal("seen target not picked")
	}
	seen = false
	for i := range 60 {
		if m.fight(a); a.Target != o {
			t.Fatalf("dropped after %d unseen ticks", i+1)
		}
	}
	if m.fight(a); a.Target != nil {
		t.Fatal("kept after 61 unseen ticks")
	}
	seen = true
	a.Target = m.pickTarget(a)
	o.Body.X = a.Body.X + 16.5
	if m.fight(a); a.Target != nil {
		t.Fatal("kept beyond AggroRange")
	}
	a.TargetSight = false // a revenge or game-set target: no such task
	seen = false
	a.Target, o.Body.X = o, a.Body.X+3
	for range 100 {
		m.fight(a)
	}
	if a.Target != o {
		t.Fatal("no check-sight task, yet dropped")
	}
}
