package actors

import "testing"

// TestZeroHurt: a hurt of 0 is a Blow (flash, knockback, no health lost)
// except to a player; a Ward's negative amount refuses the hurt, its 0
// still blows; HurtKind bits combine.
func TestZeroHurt(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	src := New("s", "s", 2.5, 10, 0.5)
	a := New("a", "a", 0.5, 10, 0.5)
	h := a.Health
	if !m.Hurt(a, src, 0, false) || a.Health != h || a.HurtTime != MaxHurtTime || a.Body.VX >= 0 {
		t.Fatalf("zero: health %v flash %d vx %v", a.Health, a.HurtTime, a.Body.VX)
	}
	p := New("p", "p", 0.5, 10, 0.5)
	p.Player = true
	if m.Hurt(p, src, 0, false) || p.HurtTime != 0 {
		t.Fatal("player took a zero hurt")
	}
	b := New("b", "b", 0.5, 10, 0.5)
	b.Ward = func(_, _ *Actor, f float64) float64 { return -1 }
	if m.Hurt(b, src, 5, false) || b.HurtTime != 0 {
		t.Fatal("refused hurt counted")
	}
	b.Ward = func(_, _ *Actor, f float64) float64 { return 0 }
	if !m.Hurt(b, src, 5, false) || b.Health != h || b.HurtTime != MaxHurtTime {
		t.Fatal("ward 0 is no blow")
	}
	if k := HurtFire | HurtProjectile; !k.Is(HurtFire) || !k.Is(HurtProjectile) || k.Is(HurtMelee) {
		t.Fatal("bits", k)
	}
}
