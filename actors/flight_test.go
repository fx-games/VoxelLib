package actors

import (
	"math"
	"testing"
)

func TestFlightSteerAndHover(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	a := New("b", "bird", 0.5, 5, 0.5)
	a.Body.Width, a.Body.Height = 0.5, 0.5
	thinks := 0
	a.Fly = &Flight{MaxTime: 3, Speed: 0.1, Pull: Vec{0.5, 0.8, 0.5}, Forward: 0.5, Hover: 0.6,
		Think: func(a *Actor) { thinks++; a.Steer() }}
	a.Fly.SetTarget(5, 5, 0) // centre (5.5, 5.5, 0.5): +x, +y, level z
	m.Add(a)

	a.Steer()
	b := a.Body
	if b.VX != 0.05 || math.Abs(b.VY-0.08) > 1e-12 || b.VZ != 0 {
		t.Fatalf("pull %v %v %v, want 0.05 0.08 0", b.VX, b.VY, b.VZ)
	}
	if math.Abs(a.Yaw-math.Pi/2) > 1e-12 || a.nav.in.Forward != 0.5 || a.Fly.Time != 1 {
		t.Fatalf("yaw %v forward %v time %d", a.Yaw, a.nav.in.Forward, a.Fly.Time)
	}

	// One manager tick: Think steers (time 2), the body moves in air:
	// forward 0.5*0.98 at 0.02, then (vy-0.08)*0.98, then vy*0.6.
	vy := b.VY + (0.8-b.VY)*0.1
	m.Tick()
	if thinks != 1 || a.Fly.Time != 2 {
		t.Fatalf("thinks %d time %d", thinks, a.Fly.Time)
	}
	if want := (vy - 0.08) * 0.98 * 0.6; math.Abs(b.VY-want) > 1e-9 {
		t.Fatalf("vy %v, want %v", b.VY, want)
	}
	m.Tick()
	if a.Fly.Has {
		t.Fatal("target kept past MaxTime")
	}

	// Still: motion cleared after the move, no fall damage on landing.
	a.Fly.Still = true
	b.VY = -3
	m.Tick()
	if b.VX != 0 || b.VY != 0 || b.VZ != 0 || a.Health != a.MaxHealth {
		t.Fatalf("still: v %v %v %v health %v", b.VX, b.VY, b.VZ, a.Health)
	}
}
