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
	// a blow: flash and knockback without damage, not again in the window
	a.hurtResist, a.HurtTime = 0, 0
	h, hurt := a.Health, 0
	a.OnHurt = func(_, _ *Actor, n float64) { hurt++ }
	src := New("s", "s", 2.5, 10, 0.5)
	if !m.Blow(a, src) || a.Health != h || a.HurtTime != MaxHurtTime || a.Body.VX >= 0 || hurt != 1 || m.Blow(a, src) {
		t.Fatalf("blow: health %v hurt %d vx %v", a.Health, hurt, a.Body.VX)
	}
	a.OnHurt = nil
	a.hurtResist = 0
	if !m.Hurt(a, nil, 100, true) || !a.Alive() || a.MaxHealth != 40 || a.Health != 40 {
		t.Fatalf("cheat death: alive %v health %v/%v", a.Alive(), a.Health, a.MaxHealth)
	}

	// A fall of more than 3 hurts unless OnLand clears it.
	landed, fell := 0.0, 0.0
	m.FellOn = func(_ *Actor, f float64) { fell = f }
	a.OnLand = func(_ *Actor, f float64) float64 { landed = f; return 0 }
	a.hurtResist = 0
	for range 100 {
		m.Tick()
	}
	if landed <= 3 || fell != landed || a.Health != 40 {
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

// TestHoldAway: a holding chaser closes in but strikes no blow; Away
// walks the actor off to its point before any fighting, then clears.
func TestHoldAway(t *testing.T) {
	f := NewFactions(&Faction{Name: "orcs", Enemies: []string{"men"}}, &Faction{Name: "men"})
	m := NewManager(flat(10), f, nil, 1)
	orc := New("orc", "orc", 0.5, 1, 0.5)
	orc.Faction, orc.Melee = "orcs", Melee{Damage: 3, Cooldown: 20, Hold: true}
	man := New("man", "man", 4.5, 1, 0.5)
	man.Faction, man.Brain = "men", Brain{Mode: Stand, Home: Vec{4.5, 1, 0.5}}
	m.Add(orc)
	m.Add(man)
	hits := 0
	man.OnHurt = func(a, src *Actor, n float64) { hits++ }
	for range 100 {
		m.Tick()
	}
	if hits != 0 || orc.Target != man || orc.Pos().hdist(man.Pos()) > 1.5 {
		t.Fatalf("hold: %d hits, %.1f away", hits, orc.Pos().hdist(man.Pos()))
	}
	orc.Away, orc.AwaySpeed = &Vec{-4.5, 1, 0.5}, 1.5
	for i := 0; orc.Away != nil; i++ {
		if i > 200 {
			t.Fatalf("away: stuck at %v", orc.Pos())
		}
		m.Tick()
	}
	if orc.Pos().hdist(Vec{-4.5, 1, 0.5}) > 0.4 {
		t.Fatalf("away: at %v", orc.Pos())
	}
}
