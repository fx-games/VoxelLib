package actors

import "testing"

// TestPassenger: a passenger's mount walks its own route while the
// passenger, wanting the other way, stays seated on it at its SeatHeight;
// set down, an AI rider lands on top of the mount at its own height again.
func TestPassenger(t *testing.T) {
	m := NewManager(flat(40), nil, nil, 1)
	mt, r := New("mt", "x", 20.5, 1, 20.5), New("r", "x", 20.5, 1, 20.5)
	for _, o := range []*Actor{mt, r} {
		o.Health, o.MaxHealth, o.Speed = 20, 20, 0.25
		m.Add(o)
	}
	r.Passenger, r.SeatHeight = true, r.Body.Height*0.77
	h := r.Body.Height
	mt.Brain = Brain{Mode: Route, Points: []Vec{{X: 30.5, Y: 1, Z: 20.5}}}
	r.Brain = Brain{Mode: Route, Points: []Vec{{X: 10.5, Y: 1, Z: 20.5}}}
	if !m.Carry(r, mt) || r.Body.Height != h*0.77 {
		t.Fatal("not seated", r.Body.Height)
	}
	for i := 0; i < 100; i++ {
		m.Tick()
	}
	if mt.Body.X < 25 || r.Body.X != mt.Body.X || r.Body.Y != mt.Body.Y+mt.Body.Height*0.75 {
		t.Fatalf("mount at %.2f, rider at %.2f %.2f", mt.Body.X, r.Body.X, r.Body.Y)
	}
	m.Dismount(r)
	if r.Body.X != mt.Body.X || r.Body.Y != mt.Body.Y+mt.Body.Height || r.Body.Height != h {
		t.Fatalf("set down at %.2f %.2f, height %.2f", r.Body.X, r.Body.Y, r.Body.Height)
	}
}
