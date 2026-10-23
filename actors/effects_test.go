package actors

import "testing"

// TestEffects: poison hurts every 25>>amp ticks of its duration but never
// below 1 health, a weaker effect does not replace a stronger one,
// slowness scales the move speed while it lasts, Refuse turns effects
// down, ClearEffects ends them all, and the colour mixes the effects by level.
func TestEffects(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	m.Effects = []EffectRule{
		{Name: "moveSlowdown", Speed: -0.15000000596046448, Color: 5926017},
		{Name: "poison", Period: 25, Damage: 1, Floor: 1, Color: 5149489},
	}
	a := New("a", "spider", 0.5, 20, 0.5)
	m.Add(a)
	m.AddEffect(a, "poison", 100, 0)
	m.AddEffect(a, "poison", 50, 0) // shorter: kept at 100
	for range 100 {
		a.hurtResist = 0
		m.effectTick(a)
	}
	if a.Health != 16 || a.Effects["poison"] != nil {
		t.Fatalf("poison: health %v, %v", a.Health, a.Effects)
	}
	a.Health = 2
	m.AddEffect(a, "poison", 26, 1) // 12 ticks apart: two hurts due, one lands
	for range 26 {
		a.hurtResist = 0
		m.effectTick(a)
	}
	if a.Health != 1 {
		t.Fatalf("poison floor: health %v", a.Health)
	}

	m.AddEffect(a, "moveSlowdown", 40, 1)
	m.AddEffect(a, "moveSlowdown", 400, 0) // weaker: ignored
	m.AddEffect(a, "poison", 40, 0)
	if v := a.moveSpeed(); v != a.Speed*(1-0.15000000596046448*2) || a.Effects["moveSlowdown"].Ticks != 40 {
		t.Fatalf("slowness: speed %v, %v", v, a.Effects["moveSlowdown"])
	}
	if c := m.EffectColor(a); c != 5667174 {
		t.Fatalf("colour %d", c)
	}
	for range 40 {
		m.effectTick(a)
	}
	if a.moveSpeed() != a.Speed || m.EffectColor(a) != 0 {
		t.Fatalf("after: speed %v colour %d", a.moveSpeed(), m.EffectColor(a))
	}

	m.AddEffect(a, "moveSlowdown", 40, 0)
	m.ClearEffects(a)
	if len(a.Effects) != 0 || a.moveSpeed() != a.Speed {
		t.Fatal("cleared effects remain")
	}

	a.Refuse = func(_ *Actor, e string) bool { return e == "poison" }
	if m.AddEffect(a, "poison", 40, 0) || a.Effects["poison"] != nil {
		t.Fatal("refused effect added")
	}
}
