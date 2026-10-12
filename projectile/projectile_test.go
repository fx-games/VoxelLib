package projectile

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

// ground is solid below y=0, with a wall at x >= 30.
var ground = collide.SolidsFunc(func(x, y, z int) bool { return y < 0 || x >= 30 })

func TestArcAndStickInGround(t *testing.T) {
	var w World
	a := w.Shoot(0, 1.5, 0, math.Pi/2, 0.1, 1) // east, slightly up, slow
	peak := a.Y
	for i := 0; i < 200 && !a.Stuck; i++ {
		w.Tick(collide.SolidsFunc(func(x, y, z int) bool { return y < 0 }), nil)
		peak = math.Max(peak, a.Y)
	}
	if !a.Stuck {
		t.Fatal("arrow never landed")
	}
	if peak <= 1.6 {
		t.Errorf("no arc: peak %.2f", peak)
	}
	if a.Y > 0 || a.Y < -0.3 {
		t.Errorf("stuck at y=%.2f, want just below the surface", a.Y)
	}
	if a.Pitch >= 0 {
		t.Errorf("stuck arrow should point down, pitch %.2f", a.Pitch)
	}
	x := a.X
	w.Tick(ground, nil)
	if a.X != x {
		t.Error("stuck arrow moved")
	}
}

func TestStickInWallAtFace(t *testing.T) {
	var w World
	a := w.Shoot(25, 5, 0.5, math.Pi/2, 0, Speed)
	for i := 0; i < 20 && !a.Stuck; i++ {
		w.Tick(ground, nil)
	}
	if !a.Stuck || a.X < 30 || a.X > 30.3 {
		t.Errorf("want stuck just inside the wall face x=30, got stuck=%v x=%.3f", a.Stuck, a.X)
	}
}

type dummy struct{ x, y, z, yaw float64 }

func (d *dummy) Box() collide.AABB             { return collide.Box(d.x, d.y, d.z, 0.6, 1.8) }
func (d *dummy) Frame() (x, y, z, yaw float64) { return d.x, d.y, d.z, d.yaw }

func TestStickInTargetAndFollow(t *testing.T) {
	var w World
	d := &dummy{x: 10, y: 0, z: 0.5}
	hits := 0
	w.Hit = func(*Arrow, Target) { hits++ }
	a := w.Shoot(0, 1, 0.5, math.Pi/2, 0, Speed)
	for i := 0; i < 20 && !a.Stuck; i++ {
		w.Tick(ground, []Target{d})
	}
	if a.In != d || hits != 1 {
		t.Fatalf("arrow not stuck in target (in=%v hits=%d)", a.In, hits)
	}
	ox, oy := a.X-d.x, a.Y-d.y
	d.x += 3
	w.Tick(ground, []Target{d})
	if math.Abs(a.X-d.x-ox) > 1e-9 || math.Abs(a.Y-d.y-oy) > 1e-9 {
		t.Errorf("arrow didn't follow the target: off %.3f,%.3f want %.3f,%.3f", a.X-d.x, a.Y-d.y, ox, oy)
	}
	// Turning the target half round swings the arrow to the other side.
	d.yaw = math.Pi
	w.Tick(ground, []Target{d})
	if math.Abs((a.X-d.x)+ox) > 1e-6 {
		t.Errorf("arrow didn't turn with the target: x off %.3f want %.3f", a.X-d.x, -ox)
	}
}

func TestExpires(t *testing.T) {
	var w World
	w.Shoot(0, 0.5, 0, 0, -1.5, 1)
	for i := 0; i < Life+20; i++ {
		w.Tick(ground, nil)
	}
	if len(w.Arrows) != 0 {
		t.Errorf("%d arrows left after their life", len(w.Arrows))
	}
}
