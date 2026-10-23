package actors

import "testing"

// TestMeleeBonus: Melee.Bonus adds target-dependent damage to each blow;
// OnHit runs after a blow that hurt.
func TestMeleeBonus(t *testing.T) {
	m := NewManager(flat(10), NewFactions(&Faction{Name: "a", Enemies: []string{"b"}}, &Faction{Name: "b"}), nil, 1)
	src := New("src", "x", 0.5, 1, 0.5)
	src.Faction, src.Melee = "a", Melee{Damage: 3, Cooldown: 20,
		Bonus: func(_, t *Actor) float64 {
			if t.Kind == "undead" {
				return 2.5
			}
			return 0
		}}
	hits := 0
	src.Melee.OnHit = func(s, h *Actor) {
		if s == src && h.Health == 100-5.5 {
			hits++
		}
	}
	tg := New("t", "undead", 1.5, 1, 0.5)
	tg.Faction, tg.Health, tg.MaxHealth, tg.KnockbackResist = "b", 100, 100, 1
	tg.Brain = Brain{Mode: Stand, Home: Vec{1.5, 1, 0.5}}
	m.Add(src)
	m.Add(tg)
	for i := 0; i < 25 && tg.Health == 100; i++ {
		m.Tick()
	}
	if tg.Health != 100-5.5 {
		t.Fatalf("health %v, want 94.5", tg.Health)
	}
	if hits != 1 {
		t.Fatalf("OnHit ran %d times", hits)
	}
}
