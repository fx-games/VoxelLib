package actors

import "testing"

// TestAttacker: a player's hurt sets attackingPlayer for 100 ticks, nil
// on the tick after; a mob's hurt leaves it, a source AttackerClears
// accepts clears it.
func TestAttacker(t *testing.T) {
	m := NewManager(flat(20), nil, nil, 1)
	p, a := New("p", "player", 0.5, 1, 0.5), New("a", "cow", 2.5, 1, 0.5)
	p.Player, a.Health = true, 1000
	mob, wolf := New("m", "zombie", 4.5, 1, 0.5), New("w", "wolf", 4.5, 1, 2.5)
	for _, x := range []*Actor{p, a, mob, wolf} {
		m.Add(x)
	}
	m.AttackerClears = func(s *Actor) bool { return s == wolf }
	m.Hurt(a, p, 1, false)
	for range 15 {
		m.Tick()
	}
	if m.Hurt(a, mob, 5, false); a.Attacker() != p || a.Revenge() != mob {
		t.Fatalf("after a mob's hurt: attacker %v revenge %v", a.Attacker(), a.Revenge())
	}
	for range 85 {
		m.Tick()
	}
	if a.Attacker() != p {
		t.Fatal("cleared at 100 ticks")
	}
	if m.Tick(); a.Attacker() != nil {
		t.Fatal("kept after 101 ticks")
	}
	for range 15 {
		m.Tick()
	}
	m.Hurt(a, p, 10, false)
	for range 15 {
		m.Tick()
	}
	if m.Hurt(a, wolf, 20, false); a.Attacker() != nil {
		t.Fatal("a wolf's hurt kept it")
	}
}
