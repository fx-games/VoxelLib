package actors

import "testing"

// TestProtection: the EPF hook knows melee blows from other hurts, and an
// EPF of 1 takes off 1/25.
func TestProtection(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	m.Protection = func(_, _ *Actor, how HurtKind) int {
		if how == HurtMelee {
			return 1
		}
		return 0
	}
	src := New("src", "x", 0.5, 1, 0.5)
	tg := New("t", "y", 1.5, 1, 0.5)
	tg.Health, tg.MaxHealth, tg.KnockbackResist = 100, 100, 1
	m.Add(src)
	m.Add(tg)
	m.Attack(src, tg, 10, 0)
	if d := tg.Health - 90.4; d > 1e-5 || d < -1e-5 {
		t.Fatalf("melee: health %v, want 90.4", tg.Health)
	}
	tg.hurtResist = 0
	m.Hurt(tg, src, 10, false)
	if d := tg.Health - 80.4; d > 1e-5 || d < -1e-5 {
		t.Fatalf("other: health %v, want 80.4", tg.Health)
	}
}

// TestHurting: the LivingHurtEvent hook halves a counted hurt; the window
// still compares the unchanged amount.
func TestHurting(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	m.Hurting = func(_, _ *Actor, n float64) float64 { return n / 2 }
	src := New("src", "x", 0.5, 1, 0.5)
	tg := New("t", "y", 1.5, 1, 0.5)
	tg.Health, tg.MaxHealth = 100, 100
	m.Add(src)
	m.Add(tg)
	m.Hurt(tg, src, 10, true)
	m.Hurt(tg, src, 12, true)
	if tg.Health != 94 {
		t.Fatalf("health %v, want 94", tg.Health)
	}
}
