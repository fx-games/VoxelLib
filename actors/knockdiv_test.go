package actors

import "testing"

// TestKnockDiv: a KnockDiv of 4 leaves a quarter of knockBack's motion.
func TestKnockDiv(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	src := New("src", "x", 0.5, 1, 0.5)
	a, b := New("a", "x", 2.5, 1, 0.5), New("b", "x", 2.5, 1, 4.5)
	b.KnockDiv = 4
	for _, o := range []*Actor{src, a, b} {
		o.Health, o.MaxHealth = 20, 20
		m.Add(o)
	}
	m.Hurt(a, src, 1, false)
	b.Body.X, b.Body.Z = a.Body.X, a.Body.Z
	m.Hurt(b, src, 1, false)
	if a.Body.VX == 0 || b.Body.VX != a.Body.VX/4 || b.Body.VY != a.Body.VY/4 {
		t.Fatalf("motion %v,%v vs %v,%v", b.Body.VX, b.Body.VY, a.Body.VX, a.Body.VY)
	}
}

// TestKnockOwn: a knockBack override pushes KnockPush with a lift of
// 0.2+KnockPush/2 and ignores KnockbackResist; a push of 0 is none.
func TestKnockOwn(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	src := New("src", "x", 0.5, 1, 0.5)
	a, b := New("a", "x", 2.5, 1, 0.5), New("b", "x", 2.5, 1, 4.5)
	a.KnockOwn, a.KnockPush, a.KnockbackResist = true, 0.25, 1
	b.KnockOwn = true
	for _, o := range []*Actor{src, a, b} {
		o.Health, o.MaxHealth = 20, 20
		m.Add(o)
	}
	vx, vy := a.Body.VX, a.Body.VY
	m.Hurt(a, src, 1, false)
	if d := a.Body.VX - (vx/2 + 0.25); d*d > 1e-12 || a.Body.VY != vy/2+0.2+0.125 {
		t.Fatalf("pushed %v,%v", a.Body.VX, a.Body.VY)
	}
	bx, by := b.Body.VX, b.Body.VY
	m.Hurt(b, src, 1, false)
	if b.Body.VX != bx || b.Body.VY != by {
		t.Fatalf("push 0 moved it: %v,%v", b.Body.VX, b.Body.VY)
	}
}

// TestMeleeRangeOverride: Range² plus the target's width, never below the
// default reach; a sprinting mob's blow gets no sprint knockback.
func TestMeleeRangeOverride(t *testing.T) {
	a, b := New("a", "x", 0, 1, 0), New("b", "x", 0, 1, 0)
	def := MeleeReach2(a, b)
	a.Melee.Range = 3
	if got := MeleeReach2(a, b); got != 9+b.Body.Width {
		t.Errorf("range 3: %v", got)
	}
	a.Melee.Range = 0.1
	if got := MeleeReach2(a, b); got != def {
		t.Errorf("range 0.1: %v, want the default %v", got, def)
	}
	m := NewManager(flat(20), nil, nil, 1)
	src, v := New("src", "x", 0.5, 1, 0.5), New("v", "x", 0.5, 1, 1.5)
	for _, o := range []*Actor{src, v} {
		o.Health, o.MaxHealth = 20, 20
		m.Add(o)
	}
	src.Sprinting = true
	vz := v.Body.VZ
	m.Attack(src, v, 1, 0)
	if d := v.Body.VZ - (vz/2 + 0.4); d*d > 1e-12 {
		t.Errorf("a sprinting mob's blow pushed extra: vz %v", v.Body.VZ)
	}
}
