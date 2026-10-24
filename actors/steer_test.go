package actors

import (
	"math"
	"testing"
)

// TestSteer: EntityAIControlledByPlayer turns 5 degrees a tick toward the
// rider, creeps to Max by 1% of the gap, wears the lure past Max/2,
// boosts only above 0.3 Max and stops (speed 0) without the lure.
func TestSteer(t *testing.T) {
	lure, wear := true, 0
	m, p, pig, _ := horseWorld(t, MountStats{RideHeight: 0.675, Tame: true, Steer: &Steer{Max: 0.3,
		Lure: func(*Actor) bool { return lure }, Wear: func(*Actor, *Actor) { wear++ }}})
	pig.Speed = 0.25
	p.Yaw, pig.Yaw = math.Pi/2, 0
	m.Tick()
	s := pig.Mount.Steer
	if math.Abs(pig.Yaw-5*math.Pi/180) > 1e-9 || math.Abs(s.speed-0.003) > 1e-12 {
		t.Fatal("first tick", pig.Yaw, s.speed)
	}
	if m.Boost(pig) {
		t.Fatal("boost under 0.3 Max")
	}
	x0 := pig.Body.X
	for range 199 {
		m.Tick()
	}
	if want := 0.3 * (1 - math.Pow(0.99, 200)); math.Abs(s.speed-want) > 1e-9 || math.Abs(pig.Yaw-math.Pi/2) > 1e-6 || pig.Body.X-x0 < 3 || wear == 0 {
		t.Fatal("steered", s.speed, pig.Yaw, pig.Body.X-x0, wear)
	}
	if !m.Boost(pig) || m.Boost(pig) || s.boostN < 140 || s.boostN > 980 {
		t.Fatal("boost", s.boostN)
	}
	lure = false
	m.Tick() // boosted: still steered without the lure
	if !s.running {
		t.Fatal("boost dropped")
	}
	s.boosted = false
	m.Tick()
	if s.running || s.speed != 0 {
		t.Fatal("reset", s.speed)
	}
}
