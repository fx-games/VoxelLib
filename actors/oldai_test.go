package actors

import "testing"

// TestOldAI: the pre-task AI keeps a far target and strikes a near one
// only while it sees it (EntityMob.attackEntity).
func TestOldAI(t *testing.T) {
	m := NewManager(flat(80), nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	o := New("o", "npc", 1.5, 1, 0.5)
	a.Melee.Damage, a.Melee.Cooldown, a.AggroRange, a.OldAI = 5, 20, 16, true
	m.Add(a)
	m.Add(o)
	seen := false
	m.Sees = func(x, y *Actor) bool { return seen }
	m.Keeps = func(x, y *Actor) bool { return x == a && y == o }
	a.Target = o
	m.fight(a)
	if o.Health != o.MaxHealth {
		t.Fatal("struck an unseen target")
	}
	seen = true
	m.fight(a)
	if o.Health != o.MaxHealth-5 {
		t.Fatalf("health %v after a seen blow", o.Health)
	}
	o.Body.X = 60.5
	if m.fight(a); a.Target != o {
		t.Fatal("dropped a far target")
	}
}
