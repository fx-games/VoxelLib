// Package projectile flies arrows the way Minecraft does: a launch speed,
// gravity and air drag give a ballistic arc; an arrow that hits a block
// sticks in it, and one that hits a target sticks in the target and rides
// along with it. Stuck arrows despawn after a while.
//
// It knows nothing about worlds, models or rendering: terrain is a
// collide.Solids and targets are anything with a box. Tick at 20 Hz.
package projectile

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Minecraft's arrow constants (blocks and ticks).
const (
	Gravity = 0.05 // per tick²
	Drag    = 0.99 // velocity kept per tick in air
	// Speed is a fully drawn crossbow's launch speed (blocks per tick).
	Speed = 3.15
	// Life is how long a stuck arrow stays (1 minute).
	Life = 1200
	// step is the longest sub-step when sweeping, so a fast arrow can't skip
	// through a thin wall or a mob between ticks.
	step = 0.25
)

// Target is something arrows can hit and stick in, such as a mob.
type Target interface {
	// Box is the target's current collision box.
	Box() collide.AABB
	// Frame is the target's current position and yaw, used to carry stuck
	// arrows along as it moves and turns.
	Frame() (x, y, z, yaw float64)
}

// Arrow is one projectile.
type Arrow struct {
	X, Y, Z             float64 // tip position
	VX, VY, VZ          float64 // blocks per tick
	PrevX, PrevY, PrevZ float64
	// Yaw and Pitch orient the arrow along its flight (radians, as the
	// camera: yaw 0 points to -Z, positive pitch points up). Frozen once
	// stuck.
	Yaw, Pitch         float64
	PrevYaw, PrevPitch float64

	Stuck bool
	// In is the target the arrow is stuck in (nil for terrain or flying).
	In  Target
	Age int // ticks since launch, or since sticking

	// Offset in the target's frame: position relative to its feet, rotated
	// by -yaw, and the arrow's yaw relative to the target's.
	ox, oy, oz, oyaw float64
}

// World holds the arrows in flight and stuck.
type World struct {
	Arrows []*Arrow
	// Hit, if set, is called once when an arrow sticks in a target.
	Hit func(a *Arrow, t Target)
}

// Shoot launches an arrow from x,y,z along yaw/pitch at speed (blocks per
// tick). spread adds random inaccuracy in radians from rnd (0 = exact).
func (w *World) Shoot(x, y, z, yaw, pitch, speed float64) *Arrow {
	dx := math.Sin(yaw) * math.Cos(pitch)
	dy := math.Sin(pitch)
	dz := -math.Cos(yaw) * math.Cos(pitch)
	a := &Arrow{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z,
		VX: dx * speed, VY: dy * speed, VZ: dz * speed,
		Yaw: yaw, Pitch: pitch, PrevYaw: yaw, PrevPitch: pitch}
	w.Arrows = append(w.Arrows, a)
	return a
}

// Tick advances every arrow one step and drops expired ones.
func (w *World) Tick(s collide.Solids, targets []Target) {
	live := w.Arrows[:0]
	for _, a := range w.Arrows {
		a.PrevX, a.PrevY, a.PrevZ = a.X, a.Y, a.Z
		a.PrevYaw, a.PrevPitch = a.Yaw, a.Pitch
		a.Age++
		switch {
		case a.In != nil:
			a.follow()
		case !a.Stuck:
			w.fly(a, s, targets)
		}
		if !(a.Stuck && a.Age > Life) && a.Y > -1000 {
			live = append(live, a)
		}
	}
	clear(w.Arrows[len(live):])
	w.Arrows = live
}

// fly sweeps the arrow along its velocity in sub-steps, sticking at the
// first block or target it enters.
func (w *World) fly(a *Arrow, s collide.Solids, targets []Target) {
	d := math.Sqrt(a.VX*a.VX + a.VY*a.VY + a.VZ*a.VZ)
	n := max(1, int(math.Ceil(d/step)))
	sx, sy, sz := a.VX/float64(n), a.VY/float64(n), a.VZ/float64(n)
	for i := 0; i < n; i++ {
		nx, ny, nz := a.X+sx, a.Y+sy, a.Z+sz
		for _, t := range targets {
			if inBox(t.Box(), nx, ny, nz) {
				a.X, a.Y, a.Z = nx, ny, nz
				a.stickIn(t)
				if w.Hit != nil {
					w.Hit(a, t)
				}
				return
			}
		}
		if s.Solid(int(math.Floor(nx)), int(math.Floor(ny)), int(math.Floor(nz))) {
			// Bury the tip a little past the face, like Minecraft's arrows.
			f := surface(a.X, a.Y, a.Z, sx, sy, sz)
			bury := 0.15 / math.Max(1e-9, math.Sqrt(sx*sx+sy*sy+sz*sz))
			f = math.Min(1, f+bury)
			a.X, a.Y, a.Z = a.X+sx*f, a.Y+sy*f, a.Z+sz*f
			a.Stuck, a.Age = true, 0
			a.VX, a.VY, a.VZ = 0, 0, 0
			return
		}
		a.X, a.Y, a.Z = nx, ny, nz
	}
	a.VX *= Drag
	a.VY = a.VY*Drag - Gravity
	a.VZ *= Drag
	a.orient()
}

// orient points the arrow along its velocity.
func (a *Arrow) orient() {
	h := math.Hypot(a.VX, a.VZ)
	if h > 1e-9 || a.VY != 0 {
		a.Yaw = math.Atan2(a.VX, -a.VZ)
		a.Pitch = math.Atan2(a.VY, h)
	}
}

// surface returns the fraction (0..1) of the step from x,y,z along d at
// which the point first crosses a block boundary on any axis: the face of
// the block it is entering.
func surface(x, y, z, dx, dy, dz float64) float64 {
	f := 1.0
	for _, c := range [3][2]float64{{x, dx}, {y, dy}, {z, dz}} {
		p, v := c[0], c[1]
		if v == 0 {
			continue
		}
		var edge float64
		if v > 0 {
			edge = math.Floor(p) + 1
		} else {
			edge = math.Ceil(p) - 1
			if edge == p-1 && p == math.Floor(p) { // exactly on a boundary
				edge = p
			}
		}
		if t := (edge - p) / v; t >= 0 && t < f {
			f = t
		}
	}
	return f
}

func inBox(b collide.AABB, x, y, z float64) bool {
	return x >= b.MinX && x <= b.MaxX && y >= b.MinY && y <= b.MaxY && z >= b.MinZ && z <= b.MaxZ
}

// stickIn records the arrow's pose in t's frame.
func (a *Arrow) stickIn(t Target) {
	a.Stuck, a.In, a.Age = true, t, 0
	a.VX, a.VY, a.VZ = 0, 0, 0
	tx, ty, tz, tyaw := t.Frame()
	dx, dz := a.X-tx, a.Z-tz
	c, s := math.Cos(-tyaw), math.Sin(-tyaw)
	a.ox, a.oy, a.oz = dx*c-dz*s, a.Y-ty, dx*s+dz*c
	a.oyaw = a.Yaw - tyaw
}

// follow moves a stuck arrow with its target.
func (a *Arrow) follow() {
	tx, ty, tz, tyaw := a.In.Frame()
	c, s := math.Cos(tyaw), math.Sin(tyaw)
	a.X, a.Y, a.Z = tx+a.ox*c-a.oz*s, ty+a.oy, tz+a.ox*s+a.oz*c
	a.Yaw = tyaw + a.oyaw
}

// Release unsticks every arrow stuck in t (call when t despawns); they stay
// where they are until they expire.
func (w *World) Release(t Target) {
	for _, a := range w.Arrows {
		if a.In == t {
			a.In = nil
		}
	}
}

// Lerp interpolates position and orientation for rendering.
func (a *Arrow) Lerp(alpha float64) (x, y, z, yaw, pitch float64) {
	dy := math.Remainder(a.Yaw-a.PrevYaw, 2*math.Pi)
	return a.PrevX + (a.X-a.PrevX)*alpha, a.PrevY + (a.Y-a.PrevY)*alpha, a.PrevZ + (a.Z-a.PrevZ)*alpha,
		a.PrevYaw + dy*alpha, a.PrevPitch + (a.Pitch-a.PrevPitch)*alpha
}
