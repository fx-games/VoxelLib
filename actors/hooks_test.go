package actors

import "testing"

// TestHurtHooks: Ward rewrites or drops a hurt before anything else;
// OnDamage sees the pre-armour amount and can cheat death; OnLand
// replaces the fall distance; Blocked stops the brain.
func TestHurtHooks(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	a := New("a", "boss", 0.5, 10, 0.5)
	a.Armor = 12
	m.Add(a)

	a.Ward = func(_, _ *Actor, n float64) float64 { return min(n, 1) }
	var raw float64
	a.OnDamage = func(a, _ *Actor, n float64) {
		raw = n
		if a.Health <= 0 {
			a.MaxHealth *= 2
			a.Health = a.MaxHealth
		}
	}
	if !m.Hurt(a, nil, 30, false) || raw != 1 || a.Health != 20-13.0/25 {
		t.Fatalf("ward: raw %v health %v", raw, a.Health)
	}
	a.Ward = func(_, _ *Actor, n float64) float64 { return 0 }
	if m.Hurt(a, nil, 5, false) {
		t.Fatal("warded hurt counted")
	}
	a.Ward = nil
	a.hurtResist = 0
	if !m.Hurt(a, nil, 100, true) || !a.Alive() || a.MaxHealth != 40 || a.Health != 40 {
		t.Fatalf("cheat death: alive %v health %v/%v", a.Alive(), a.Health, a.MaxHealth)
	}

	// A fall of more than 3 hurts unless OnLand clears it.
	landed := 0.0
	a.OnLand = func(_ *Actor, f float64) float64 { landed = f; return 0 }
	a.hurtResist = 0
	for range 100 {
		m.Tick()
	}
	if landed <= 3 || a.Health != 40 {
		t.Fatalf("land: fall %v health %v", landed, a.Health)
	}

	a.Blocked = true
	a.Brain = Brain{Mode: Wander, Range: 10}
	x, z := a.Body.X, a.Body.Z
	for range 200 {
		m.Tick()
	}
	if a.Body.X != x || a.Body.Z != z {
		t.Fatal("a blocked actor moved")
	}
}
