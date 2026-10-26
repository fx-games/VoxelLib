package actors

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
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

type iceWorld struct{ *testWorld }

func (iceWorld) Slipperiness(x, y, z int) float64 { return 0.98 }

// TestSteerSteps: a steered tick is the task's move then a second with no
// input: from rest, a*f2*f2 with the ground's f2 (0.6*0.91, ice
// 0.98*0.91) and a scaled by 0.16277136/f2^3; a panic (a revenge target)
// stops the steering.
func TestSteerSteps(t *testing.T) {
	run := func(ice bool) (*Manager, *Actor) {
		m, _, pig, _ := horseWorld(t, MountStats{RideHeight: 0.675, Tame: true, Steer: &Steer{Max: 0.3,
			Lure: func(*Actor) bool { return true }}})
		if ice {
			m.W = iceWorld{m.W.(*testWorld)}
		}
		pig.Speed = 0.25
		return m, pig
	}
	for _, ice := range []bool{false, true} {
		m, pig := run(ice)
		lure := false
		pig.Mount.Steer.Lure = func(*Actor) bool { return lure }
		for range 5 { // land
			m.Tick()
		}
		pig.Body.VX, pig.Body.VZ, lure = 0, 0, true
		m.Tick()
		f2, k := 0.546, 1.0
		if ice {
			f := float32(0.98) * 0.91
			f2, k = float64(f), float64(0.16277136/(f*f*f))/(0.16277136/(0.546*0.546*0.546))
		}
		a := 0.003 * 0.98 * pig.Body.MoveSpeed * k
		if v := math.Hypot(pig.Body.VX, pig.Body.VZ); !pig.Body.OnGround || math.Abs(v-a*f2*f2) > 1e-9 {
			t.Fatalf("ice %v: v %v want %v", ice, v, a*f2*f2)
		}
	}

	m, pig := run(false)
	m.Tick()
	pig.FleeWhenHurt = true
	pig.SetRevenge(New("wolf", "wolf", 3, 1, 3))
	m.Tick()
	if s := pig.Mount.Steer; s.running || s.speed != 0 {
		t.Fatal("panic did not stop the steering")
	}
}

type stairWorld struct {
	*testWorld
	stair nav.Pos
}

func (w stairWorld) StairOrSlab(x, y, z int) bool { return w.stair == nav.Pos{X: x, Y: y, Z: z} }
func (w stairWorld) Air(x, y, z int) bool         { return !w.Solid(x, y, z) }

// TestSteerStairJump: a steered mount facing a one-block step jumps,
// but not standing in the air above a slab or stairs (func_151498_a).
func TestSteerStairJump(t *testing.T) {
	for _, stairs := range []bool{false, true} {
		m, p, pig, _ := horseWorld(t, MountStats{RideHeight: 0.675, Tame: true, Steer: &Steer{Max: 0.3,
			Lure: func(*Actor) bool { return true }}})
		w := m.W.(*testWorld)
		w.solid[nav.Pos{X: 1, Y: 1, Z: 0}] = true
		if stairs {
			m.W = stairWorld{w, nav.Pos{X: 0, Y: 0, Z: 0}}
		}
		pig.Yaw, p.Yaw = math.Pi/2, math.Pi/2
		pig.Body.X, pig.Body.Y, pig.Body.Z = 0.5, 1, 0.5
		m.steerInput(pig)
		if pig.Mount.Steer.jump == stairs {
			t.Fatal("stairs", stairs, "jump", pig.Mount.Steer.jump)
		}
	}
}

type stepWorld struct{ step, air map[[3]int]bool }

func (w stepWorld) StairOrSlab(x, y, z int) bool { return w.step[[3]int{x, y, z}] }
func (w stepWorld) Air(x, y, z int) bool         { return w.air[[3]int{x, y, z}] }

// No jumping from a stair or the air above one; anywhere else, and in a
// world that knows no steps, jumping is allowed.
func TestStepJumps(t *testing.T) {
	w := stepWorld{step: map[[3]int]bool{{0, 0, 0}: true}, air: map[[3]int]bool{{0, 1, 0}: true, {1, 1, 0}: true}}
	if StepJumps(w, 0, 0, 0) || StepJumps(w, 0, 1, 0) || !StepJumps(w, 1, 1, 0) || !StepJumps(struct{}{}, 0, 0, 0) {
		t.Fatal("StepJumps")
	}
}
