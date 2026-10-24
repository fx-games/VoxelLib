package actors

import "testing"

// TestCNPCAttacking: CombatHandler's isAttacking starts with a target and
// ends on the 42nd tick without a valid one; a hurt restarts the count.
func TestCNPCAttacking(t *testing.T) {
	m, a, tg := cnpcPair(t, CNPCAI{OnAttack: 3}, 6.5)
	if m.Tick(); !a.CNPCAttacking() {
		t.Fatal("not attacking with a target")
	}
	tg.Health = 0
	for range 30 {
		m.Tick()
	}
	m.Hurt(a, nil, 1, false)
	for range 41 {
		m.Tick()
	}
	if !a.CNPCAttacking() {
		t.Fatal("stopped 41 ticks after the hurt")
	}
	if m.Tick(); a.CNPCAttacking() {
		t.Fatal("still attacking 42 ticks after the hurt")
	}
}
