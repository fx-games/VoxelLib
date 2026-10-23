package actors

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
)

// TestBoat: a vanilla boat floats in a pond (water y 1..2), its player
// rider sits 0.5 below posY - 0.3 and paddles it along the look, a fast
// crash into the bank breaks it (dropping the rider), and hits over 40
// damage knock it apart; the barrel's speed step jumps straight to its max.
func TestBoat(t *testing.T) {
	w := flat(20)
	for x := -10; x < 10; x++ {
		for z := -10; z < 10; z++ {
			w.water[nav.Pos{X: x, Y: 1, Z: z}] = true
			w.water[nav.Pos{X: x, Y: 2, Z: z}] = true
		}
	}
	for x := -10; x < 10; x++ { // the bank at z -11, up to y 3
		for y := 1; y < 4; y++ {
			w.solid[nav.Pos{X: x, Y: y, Z: -11}] = true
		}
	}
	m := NewManager(w, nil, nil, 1)
	boat := New("boat", "boat", 0.5, 2.5, 0.5)
	boat.Body.Width, boat.Body.Height, boat.Health = 1.5, 0.6, 1
	boat.Boat = VanillaBoat()
	var how []BoatBreak
	boat.Boat.OnBreak = func(_ *Actor, h BoatBreak) { how = append(how, h) }
	p := New("player", "player", 0.5, 4, 0.5)
	p.Player = true
	m.Add(boat)
	m.Add(p)
	for i := 0; i < 400; i++ {
		m.Tick()
	}
	if b := boat.Body; b.Y < 2.5 || b.Y > 3 || math.Abs(b.VY) > 0.05 {
		t.Fatalf("not afloat: feet %.3f vy %.3f", b.Y, b.VY)
	}
	if !m.Mount(p, boat) || p.Body.Y != boat.Body.Y-0.5 {
		t.Fatalf("seat: rider %.3f boat %.3f, want boat - 0.5", p.Body.Y, boat.Body.Y)
	}
	z0 := boat.Body.Z
	p.Yaw, p.RideInput.Forward = 0, 1 // toward -Z, the bank
	m.Tick()
	if want := -0.07 * 0.98 * 0.05 * 0.99; math.Abs(boat.Body.VZ-want) > 1e-12 || boat.Boat.speed <= 0.07 {
		t.Fatalf("first stroke vz %v, want %v; speed %v", boat.Body.VZ, want, boat.Boat.speed)
	}
	for i := 0; i < 200 && len(how) == 0; i++ {
		m.Tick()
	}
	if len(how) != 1 || how[0] != BoatCrashed || p.Riding != nil || m.Get("boat") != nil || z0-boat.Body.Z < 5 {
		t.Fatalf("crash: breaks %v riding %v left %v moved %.2f", how, p.Riding != nil, m.Get("boat") != nil, z0-boat.Body.Z)
	}

	b2 := New("boat2", "boat", 0.5, 2.7, 3.5)
	b2.Body.Width, b2.Body.Height, b2.Health = 1.5, 0.6, 1
	b2.Boat, how = VanillaBoat(), nil
	b2.Boat.OnBreak = func(_ *Actor, h BoatBreak) { how = append(how, h) }
	m.Add(b2)
	m.HitBoat(b2, 3, false) // 30
	m.Tick()                // 29
	if len(how) != 0 || b2.Boat.Damage != 29 || b2.Boat.Hit != 9 || b2.Boat.Dir != -1 {
		t.Fatalf("hit: damage %v hit %d dir %d", b2.Boat.Damage, b2.Boat.Hit, b2.Boat.Dir)
	}
	m.HitBoat(b2, 2, false) // 49 > 40
	m.Tick()
	if len(how) != 1 || how[0] != BoatBroken || m.Get("boat2") != nil {
		t.Fatalf("knocked apart: %v", how)
	}

	// the barrel's step (max-s)/(max/150) overshoots: max at once
	bar := &Boat{MountedY: 0.5, MinSpeed: 0.04, MaxSpeed: 0.25, Ramp: 0.25 / 150}
	ba := New("barrel", "barrel", 0.5, 2.5, 6.5)
	ba.Body.Width, ba.Body.Height, ba.Health, ba.Boat = 1, 1, 1, bar
	m.Add(ba)
	p.Body.X, p.Body.Y, p.Body.Z = 0.5, 4, 6.5
	m.Mount(p, ba)
	m.Tick()
	if bar.speed != 0.25 || p.Body.Y != ba.Body.Y {
		t.Fatalf("barrel speed %v rider %.3f barrel %.3f", bar.speed, p.Body.Y, ba.Body.Y)
	}

	// a barrel landing from 7.5 on dry ground passes the fall to its
	// rider: ceil(7.5-3) = 5
	m.Dismount(p)
	drop := New("drop", "barrel", 15.5, 8.5, 15.5)
	drop.Body.Width, drop.Body.Height, drop.Health, drop.Boat = 1, 1, 1, &Boat{MountedY: 0.5, MinSpeed: 0.04, MaxSpeed: 0.25, Ramp: 0.25 / 150}
	m.Add(drop)
	p.Health, p.MaxHealth = 20, 20
	m.Mount(p, drop)
	for i := 0; i < 60 && !drop.Body.OnGround; i++ {
		m.Tick()
	}
	if p.Health != 15 {
		t.Fatalf("rider health %v after the barrel's fall, want 15", p.Health)
	}

	// boats push only their own kind: barrel/boat overlapping don't move,
	// two barrels do
	mk := func(id, kind string, x float64) *Actor {
		a := New(id, kind, x, 1, -15.5)
		a.Body.Width, a.Body.Height, a.Health, a.Boat = 1, 1, 1, &Boat{MinSpeed: 0.04, MaxSpeed: 0.25, Ramp: 0.25 / 150}
		m.Add(a)
		return a
	}
	x1, x2 := mk("x1", "barrel", -15.5), mk("x2", "boat", -15)
	y1, y2 := mk("y1", "barrel", 15.5), mk("y2", "barrel", 16)
	m.Tick() // the push's velocity moves them the tick after
	m.Tick()
	if x1.Body.X != -15.5 || x2.Body.X != -15 || y1.Body.X >= 15.5 || y2.Body.X <= 16 {
		t.Fatalf("push: barrel/boat %.3f %.3f, barrels %.3f %.3f", x1.Body.X, x2.Body.X, y1.Body.X, y2.Body.X)
	}
}
