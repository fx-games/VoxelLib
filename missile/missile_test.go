package missile

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/physics"
)

// zeroRand: no jitter, signs +1, Float32 0.5.
type zeroRand struct{}

func (zeroRand) Float32() float32     { return 0.5 }
func (zeroRand) IntN(n int) int       { return n - 1 }
func (zeroRand) NormFloat64() float64 { return 0 }

// floor is a world whose blocks below y 0 are solid (state 1).
type floor struct{ sparks map[string]int }

func (floor) Block(x, y, z int) voxel.StateID {
	if y < 0 {
		return 1
	}
	return voxel.Air
}
func (floor) Ray(x0, y0, z0, x1, y1, z1 float64, _ bool) (float64, float64, float64, physics.Hit, bool) {
	if y1 >= 0 || y0 < 0 {
		return 0, 0, 0, physics.Hit{}, false
	}
	t := y0 / (y0 - y1)
	hx, hz := x0+(x1-x0)*t, z0+(z1-z0)*t
	return hx, 0, hz, physics.Hit{Block: voxel.BlockPos{X: int(math.Floor(hx)), Y: -1, Z: int(math.Floor(hz))}, State: 1}, true
}
func (floor) Inside(x, y, z int, px, py, pz float64) bool { return py < 0 && py > -1 }
func (floor) Water(collide.AABB) bool                     { return false }
func (f floor) Spark(name string, x, y, z, vx, vy, vz float64) {
	f.sparks[name]++
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// A level shot: motion along +Z at the speed, then per tick drag 0.99F
// and gravity 0.05F, moving by the motion before the drag.
func TestFlightDragGravity(t *testing.T) {
	w := floor{map[string]int{}}
	m := &Missile{Spec: Arrow, W: 0.5}
	m.Place(0, 10, 0)
	m.Heading(zeroRand{}, 0, 0, 1, 3, 1)
	if !near(m.MZ, 3) || m.MX != 0 || m.MY != 0 || m.Yaw != 0 || m.Pitch != 0 {
		t.Fatalf("heading: %+v", m)
	}
	z, y, mz, my := 0.0, 10.0, 3.0, 0.0
	for range 5 {
		s, fly := m.Begin(w, zeroRand{})
		if !fly || s.Block {
			t.Fatal("hit early")
		}
		m.End(w, 0)
		z, y = z+mz, y+my
		mz, my = mz*float64(float32(0.99)), my*float64(float32(0.99))-float64(float32(0.05))
		if !near(m.Z, z) || !near(m.Y, y) || !near(m.MY, my) || !near(m.MZ, mz) {
			t.Fatalf("got z %v y %v my %v, want %v %v %v", m.Z, m.Y, m.MY, z, y, my)
		}
	}
	if m.Air != 5 || m.Pitch >= 0 {
		t.Errorf("air %d pitch %v", m.Air, m.Pitch)
	}
}

// Straight down into the floor: sticks 0.05 back from the hit, wobbles 7
// ticks, loses its crit, and is gone after Life stuck ticks.
func TestStickAndLife(t *testing.T) {
	w := floor{map[string]int{}}
	m := &Missile{Spec: Arrow, W: 0.5, Crit: true}
	m.Place(0.5, 0.5, 0.5)
	m.MY = -1
	s, _ := m.Begin(w, zeroRand{})
	if !s.Block || !near(s.HY, 0) {
		t.Fatalf("no block hit: %+v", s)
	}
	m.Stick(s)
	if !m.InGround || m.Shake != 7 || m.Crit || !near(m.Y, 0.5+float64(float32(0.05))) {
		t.Fatalf("stick: %+v", m)
	}
	m.End(w, 0) // the stuck missile still moves by the backed-out motion
	if !near(m.Y, float64(float32(0.05))) {
		t.Fatalf("after move y %v", m.Y)
	}
	ticks := 0
	for !m.Dead {
		if _, fly := m.Begin(w, zeroRand{}); fly {
			t.Fatal("stuck missile flew")
		}
		ticks++
	}
	if ticks != 1200 || m.Shake != 0 {
		t.Errorf("gone after %d ticks, shake %d", ticks, m.Shake)
	}
}

func TestHitMaths(t *testing.T) {
	m := &Missile{Spec: Arrow}
	m.MX, m.MZ = 3, 4
	if k := m.Damage(zeroRand{}, 2.2); k != 3 {
		t.Errorf("damage %d", k)
	}
	m.Crit = true // + IntN(3/2+2) = 2 with this rand
	if k := m.Damage(zeroRand{}, 2.2); k != 5 {
		t.Errorf("crit damage %d", k)
	}
	vx, vy, vz := m.Push(2)
	if !near(vx, 3*2*float64(float32(0.6))/5) || vy != 0.1 || !near(vz, 4*2*float64(float32(0.6))/5) {
		t.Errorf("push %v %v %v", vx, vy, vz)
	}
	m.Classic = false
	if vx, _, _ = m.Push(1); vx != 3*0.6/5 {
		t.Errorf("derived push %v", vx)
	}
	m.Bounce()
	if !near(m.MX, -0.3) || m.Yaw != 180 {
		t.Errorf("bounce %v %v", m.MX, m.Yaw)
	}
}

func TestShootAim(t *testing.T) {
	m := &Missile{Spec: Projectile}
	// Looking along +Z level from an eye at 1.62: 0.1 down, 0.16 aside.
	m.Shoot(zeroRand{}, 0, 1.62, 0, 0, 0, 1, 3)
	if !near(m.X, -0.16) || m.Y != 1.62-0.1 || m.Z != 0 || !near(m.MZ, 3) {
		t.Errorf("shoot: %+v", m)
	}
	if !m.Aim(zeroRand{}, 0, 1, 0, 0, 1, 10, 1, 0) || m.Z != 1 {
		t.Fatalf("aim start %v", m.Z)
	}
	// lifted by d*0.2 = 2 over 10: pitch atan(0.2).
	if !near(m.Pitch, float64(float32(math.Atan2(2, 10)*180/math.Pi))) {
		t.Errorf("aim pitch %v", m.Pitch)
	}
	if m.Aim(zeroRand{}, 0, 1, 0, 0, 5, 0, 1, 0) {
		t.Error("aim straight up")
	}
}

func TestSweepPickup(t *testing.T) {
	m := &Missile{Spec: Arrow, W: 0.5}
	m.Place(0, 0, 0)
	s := Step{X1: 0, Y1: 0, Z1: 10}
	far := collide.AABB{MinX: -0.3, MinY: -1, MinZ: 6, MaxX: 0.3, MaxY: 1, MaxZ: 7}
	nearB := collide.AABB{MinX: 0.1, MinY: -1, MinZ: 3, MaxX: 0.7, MaxY: 1, MaxZ: 4} // 0.1 off, inside the 0.3 grow
	miss := collide.AABB{MinX: 1, MinY: -1, MinZ: 2, MaxX: 2, MaxY: 1, MaxZ: 3}
	if i := m.Sweep(s, []collide.AABB{far, miss, nearB}); i != 2 {
		t.Errorf("sweep %d", i)
	}
	m.InGround, m.Shake = true, 1
	p := collide.AABB{MinX: 1.2, MinY: 0, MinZ: -0.3, MaxX: 1.8, MaxY: 1.8, MaxZ: 0.3}
	if m.Pickable(p) {
		t.Error("picked while wobbling")
	}
	m.Shake = 0
	if !m.Pickable(p) {
		t.Error("not picked")
	}
}
