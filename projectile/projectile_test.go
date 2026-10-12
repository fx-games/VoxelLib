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

func TestHeightAndRelease(t *testing.T) {
	var w World
	d := &dummy{x: 10, y: 0, z: 0.5}
	var h float64
	w.Hit = func(a *Arrow, tg Target) { h = a.Height(tg) }
	a := w.Shoot(0, 1.7, 0.5, math.Pi/2, 0, Speed) // head height
	for i := 0; i < 20 && !a.Stuck; i++ {
		w.Tick(ground, []Target{d})
	}
	if h < 0.8 || h > 1 {
		t.Errorf("hit height %.2f, want near the top", h)
	}
	w.Release(d)
	for i := 0; i < 60 && !(a.Stuck && a.In == nil); i++ {
		w.Tick(ground, nil)
	}
	if !a.Stuck || a.Y > 0 || a.Y < -0.3 {
		t.Errorf("released arrow should fall and stick in the ground, stuck=%v y=%.2f", a.Stuck, a.Y)
	}
}

// plate is a Shape: a thin wall at x=px, from y 1 to 2 (part 7), and it
// reports exact surface hits.
type plate struct{ dummy }

func (p *plate) Intersect(x0, y0, z0, x1, y1, z1 float64) (x, y, z float64, part int, ok bool) {
	if (x0-p.x)*(x1-p.x) > 0 || x0 == x1 {
		return 0, 0, 0, 0, false
	}
	f := (p.x - x0) / (x1 - x0)
	y = y0 + (y1-y0)*f
	if y < 1 || y > 2 {
		return 0, 0, 0, 0, false
	}
	return p.x, y, z0 + (z1-z0)*f, 7, true
}

func TestShapeHitExact(t *testing.T) {
	var w World
	p := &plate{dummy{x: 10.1, y: 0, z: 0.5}}
	a := w.Shoot(0, 1.5, 0.5, math.Pi/2, 0, Speed)
	for i := 0; i < 20 && !a.Stuck; i++ {
		w.Tick(ground, []Target{p})
	}
	if a.In != p || a.Part != 7 || math.Abs(a.X-10.1) > 1e-9 {
		t.Fatalf("want stuck exactly on the plate, part 7: in=%v part=%d x=%.4f", a.In, a.Part, a.X)
	}
	// Below the plate: inside the box, but the shape misses.
	b := w.Shoot(0, 0.6, 0.5, math.Pi/2, 0, Speed)
	for i := 0; i < 5; i++ {
		w.Tick(ground, []Target{p})
	}
	if b.In != nil {
		t.Error("arrow that missed the shape stuck in it")
	}
}
