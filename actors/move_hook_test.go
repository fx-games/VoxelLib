package actors

import "testing"

// An actor with Move is moved by it alone, not by the living physics.
func TestMoveHook(t *testing.T) {
	m := NewManager(flat(8), nil, nil, 1)
	a := New("thing", "item", 0.5, 5, 0.5)
	a.Blocked = true
	calls := 0
	a.Move = func(a *Actor) { calls++; a.Body.Y -= 0.04 }
	m.Add(a)
	m.Tick()
	if calls != 1 || a.Body.Y != 4.96 {
		t.Fatalf("calls %d, y %v", calls, a.Body.Y)
	}
}
