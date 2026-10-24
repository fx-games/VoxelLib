package actors

import "testing"

// TestVehicleHooks: a vehicle's Move replaces the floating physics after
// the hit timers, its Push the even push (both of two such vehicles),
// and MoveBody stops on the floor.
func TestVehicleHooks(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	moved, pushes := 0, map[string]string{}
	mk := func(id string, x float64, hooks bool) *Actor {
		a := New(id, id, x, 1, 0.5)
		a.Body.Width, a.Body.Height, a.Health = 0.98, 0.7, 20
		a.Boat = &Boat{Hit: 3, Damage: 5}
		if hooks {
			a.Boat.Move = func(a *Actor) bool { moved++; m.MoveBody(a, 0, -0.5, 0); return false }
			a.Boat.Push = func(a, o *Actor) { pushes[a.ID] = o.ID }
		}
		m.Add(a)
		return a
	}
	c := mk("c", 0.5, true)
	mk("d", 1.2, true)
	l := New("l", "l", 5.5, 1, 0.5)
	l.Body.Width, l.Body.Height, l.Health = 0.6, 1.8, 20
	m.Add(l)
	mk("e", 5.9, true)
	m.Tick()
	if moved != 3 || c.Boat.Hit != 2 || c.Boat.Damage != 4 || c.Body.Y != 1 || !c.Body.OnGround {
		t.Fatalf("move: %d calls, hit %d damage %v, y %v ground %v", moved, c.Boat.Hit, c.Boat.Damage, c.Body.Y, c.Body.OnGround)
	}
	if pushes["c"] != "d" || pushes["d"] != "c" || pushes["e"] != "l" || l.Body.VX != 0 {
		t.Fatalf("pushes %v, living pushed %v", pushes, l.Body.VX)
	}
}
