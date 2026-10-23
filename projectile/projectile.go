// Package projectile flies arrows the way the reference does: a launch speed,
// gravity and air drag give a ballistic arc; an arrow that hits a block
// sticks in it, and one that hits a target sticks in the target and rides
// along with it. Stuck arrows despawn after a while.
//
// It knows nothing about worlds, models or rendering: terrain is a
// collide.Solids and targets are anything with a box. Tick at 20 Hz.
package projectile

import (
	"math"

	"github.com/go-gl/mathgl/mgl64"

	"github.com/memmaker/voxellib/voxel/collide"
)

// the reference's arrow constants (blocks and ticks).
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
	// Penetration is how deep an arrow's tip sinks past the surface it
	// hits before it sticks: a third of the 0.75-block arrow.
	Penetration = 0.25
	// Length and Radius are the arrow's shaft as other arrows see it: a
	// stuck arrow is a target, so a second shot can split it Robin Hood
	// style and stick in it.
	Length = 0.75
	Radius = 0.06
)

// Target is something arrows can hit and stick in, such as a mob.
type Target interface {
	// Box is the target's current collision box.
	Box() collide.AABB
	// Frame is the target's current position and yaw, used to carry stuck
	// arrows along as it moves and turns.
	Frame() (x, y, z, yaw float64)
}

// Shape is a Target with an exact shape, such as an animated mesh. Arrows
// test it instead of the box: each sub-step's segment from x0,y0,z0 to
// x1,y1,z1 asks for the first point where it meets the shape, and part
// identifies what was hit (e.g. a bone) for the Hit callback.
type Shape interface {
	Target
	Intersect(x0, y0, z0, x1, y1, z1 float64) (x, y, z float64, part int, ok bool)
}

// Jointed is a Shape made of moving parts (bones). An arrow stuck in a part
// rides it: Joint returns the part's current transform from its own space
// to world space.
type Jointed interface {
	Shape
	Joint(part int) mgl64.Mat4
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
	In Target
	// Part is what of a Shape the arrow struck (-1 otherwise).
	Part int
	Age  int // ticks since launch, or since sticking
	// Flown counts ticks since launch and never resets, e.g. to hide a
	// fresh shot near the camera even if it stuck at once.
	Flown int
	gone  bool // removed from the world (arrows stuck in it go too)

	// Offset in the target's frame: position relative to its feet, rotated
	// by -yaw, and the arrow's yaw relative to the target's.
	ox, oy, oz, oyaw float64
	// For a Jointed target: tip and direction in the part's space.
	jointed    bool
	lpos, ldir mgl64.Vec3
}

// World holds the arrows in flight and stuck.
type World struct {
	Arrows  []*Arrow
	grid    grid
	scratch []Target
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
		Yaw: yaw, Pitch: pitch, PrevYaw: yaw, PrevPitch: pitch, Part: -1}
	w.Arrows = append(w.Arrows, a)
	return a
}

// Tick advances every arrow one step and drops expired ones. Stuck arrows
// are targets too, alongside the given ones.
func (w *World) Tick(s collide.Solids, targets []Target) {
	all := append(w.scratch[:0], targets...)
	for _, a := range w.Arrows {
		if a.Stuck {
			all = append(all, a)
		}
	}
	w.scratch = all
	w.grid.build(all)
	live := w.Arrows[:0]
	for _, a := range w.Arrows {
		a.PrevX, a.PrevY, a.PrevZ = a.X, a.Y, a.Z
		a.PrevYaw, a.PrevPitch = a.Yaw, a.Pitch
		a.Age++
		a.Flown++
		if host, ok := a.In.(*Arrow); ok && host.gone {
			a.gone = true // its host expired: it goes with it
			continue
		}
		if a.gone { // discarded
			continue
		}
		switch {
		case a.In != nil:
			a.follow()
		case !a.Stuck:
			w.fly(a, s)
		}
		if !a.gone && !(a.Stuck && a.Age > Life) && a.Y > -1000 {
			live = append(live, a)
		} else {
			a.gone = true
		}
	}
	clear(w.Arrows[len(live):])
	w.Arrows = live
}

// fly sweeps the arrow along its velocity in sub-steps, sticking at the
// first block or target it enters.
func (w *World) fly(a *Arrow, s collide.Solids) {
	d := math.Sqrt(a.VX*a.VX + a.VY*a.VY + a.VZ*a.VZ)
	n := max(1, int(math.Ceil(d/step)))
	sx, sy, sz := a.VX/float64(n), a.VY/float64(n), a.VZ/float64(n)
	// Unit flight direction, to sink the tip in by Penetration.
	ux, uy, uz := a.VX/math.Max(d, 1e-9), a.VY/math.Max(d, 1e-9), a.VZ/math.Max(d, 1e-9)
	for i := 0; i < n; i++ {
		nx, ny, nz := a.X+sx, a.Y+sy, a.Z+sz
		if t, x, y, z, part := firstHit(w.grid.near(a.X, a.Y, a.Z, nx, ny, nz), a.X, a.Y, a.Z, nx, ny, nz); t != nil {
			if _, exact := t.(Shape); exact {
				x, y, z = x+ux*Penetration, y+uy*Penetration, z+uz*Penetration
			}
			a.X, a.Y, a.Z, a.Part = x, y, z, part
			a.stickIn(t)
			if w.Hit != nil {
				w.Hit(a, t)
			}
			return
		}
		if s.Solid(int(math.Floor(nx)), int(math.Floor(ny)), int(math.Floor(nz))) {
			// Sink the tip in past the face.
			f := surface(a.X, a.Y, a.Z, sx, sy, sz)
			a.X, a.Y, a.Z = a.X+sx*f+ux*Penetration, a.Y+sy*f+uy*Penetration, a.Z+sz*f+uz*Penetration
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

// firstHit finds, among the broad phase's candidates, the target the
// segment meets first: a Shape at its exact surface, any other target when
// the segment enters its box.
func firstHit(targets []Target, x0, y0, z0, x1, y1, z1 float64) (hit Target, x, y, z float64, part int) {
	best := math.Inf(1)
	for _, t := range targets {
		hx, hy, hz, p, ok := x1, y1, z1, -1, false
		if sh, isShape := t.(Shape); isShape {
			hx, hy, hz, p, ok = sh.Intersect(x0, y0, z0, x1, y1, z1)
		} else {
			// Only entering counts: an arrow leaving a box it started in
			// (a shooter's own) passes through.
			b := t.Box()
			ok = inBox(b, x1, y1, z1) && !inBox(b, x0, y0, z0)
		}
		if !ok {
			continue
		}
		if d := (hx-x0)*(hx-x0) + (hy-y0)*(hy-y0) + (hz-z0)*(hz-z0); d < best {
			hit, x, y, z, part, best = t, hx, hy, hz, p, d
		}
	}
	return
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
	if j, ok := t.(Jointed); ok && a.Part >= 0 {
		inv := j.Joint(a.Part).Inv()
		a.jointed = true
		a.lpos = mgl64.TransformCoordinate(mgl64.Vec3{a.X, a.Y, a.Z}, inv)
		a.ldir = mgl64.TransformNormal(dir(a.Yaw, a.Pitch), inv)
	}
}

func dir(yaw, pitch float64) mgl64.Vec3 {
	return mgl64.Vec3{math.Sin(yaw) * math.Cos(pitch), math.Sin(pitch), -math.Cos(yaw) * math.Cos(pitch)}
}

// Attached reports where an arrow stuck in a Jointed target sits in its
// part's space, so a renderer can place it with the part's pose of the
// moment: world tip = Joint(part) * pos, world direction = Joint * dir.
func (a *Arrow) Attached() (part int, pos, dir mgl64.Vec3, ok bool) {
	return a.Part, a.lpos, a.ldir, a.jointed && a.In != nil
}

// Orient returns yaw and pitch pointing along a direction.
func Orient(d mgl64.Vec3) (yaw, pitch float64) {
	return math.Atan2(d[0], -d[2]), math.Atan2(d[1], math.Hypot(d[0], d[2]))
}

// follow moves a stuck arrow with its target.
func (a *Arrow) follow() {
	if j, ok := a.In.(Jointed); ok && a.jointed {
		m := j.Joint(a.Part)
		p := mgl64.TransformCoordinate(a.lpos, m)
		a.X, a.Y, a.Z = p[0], p[1], p[2]
		a.Yaw, a.Pitch = Orient(mgl64.TransformNormal(a.ldir, m))
		return
	}
	tx, ty, tz, tyaw := a.In.Frame()
	c, s := math.Cos(tyaw), math.Sin(tyaw)
	a.X, a.Y, a.Z = tx+a.ox*c-a.oz*s, ty+a.oy, tz+a.ox*s+a.oz*c
	a.Yaw = tyaw + a.oyaw
}

// Release drops every arrow stuck in t (call when t dies or despawns): they
// fall from where they are and stick in whatever they land on.
func (w *World) Release(t Target) {
	for _, a := range w.Arrows {
		if a.In == t {
			a.In, a.Stuck, a.Part, a.jointed = nil, false, -1, false
			a.VX, a.VY, a.VZ = 0, 0, 0
		}
	}
}

// Discard removes the arrow from the world at the end of this tick, e.g.
// from World.Hit when it hit something it shouldn't stick in.
func (a *Arrow) Discard() { a.gone = true }

// Detach removes every arrow stuck in t, and those stuck in them, from the
// world and returns them, e.g. to turn them into pickups when t's body
// disappears.
func (w *World) Detach(t Target) []*Arrow {
	var out []*Arrow
	live := w.Arrows[:0]
	for _, a := range w.Arrows { // hosts precede the arrows stuck in them
		host, inArrow := a.In.(*Arrow)
		if a.In == t || (inArrow && host.gone) {
			a.gone = true
			out = append(out, a)
		} else {
			live = append(live, a)
		}
	}
	clear(w.Arrows[len(live):])
	w.Arrows = live
	return out
}

// Height returns how far up t's box (0 = feet, 1 = top) an arrow struck it,
// for hit-zone checks such as head shots. Call from World.Hit.
func (a *Arrow) Height(t Target) float64 {
	b := t.Box()
	return (a.Y - b.MinY) / math.Max(1e-9, b.MaxY-b.MinY)
}

// Lerp interpolates position and orientation for rendering.
func (a *Arrow) Lerp(alpha float64) (x, y, z, yaw, pitch float64) {
	dy := math.Remainder(a.Yaw-a.PrevYaw, 2*math.Pi)
	return a.PrevX + (a.X-a.PrevX)*alpha, a.PrevY + (a.Y-a.PrevY)*alpha, a.PrevZ + (a.Z-a.PrevZ)*alpha,
		a.PrevYaw + dy*alpha, a.PrevPitch + (a.Pitch-a.PrevPitch)*alpha
}

// An arrow is itself a Jointed target once stuck: its shaft is a thin
// cylinder from the tip back Length along its direction.

func (a *Arrow) tail() (x, y, z float64) {
	d := dir(a.Yaw, a.Pitch)
	return a.X - d[0]*Length, a.Y - d[1]*Length, a.Z - d[2]*Length
}

// Box bounds the shaft.
func (a *Arrow) Box() collide.AABB {
	tx, ty, tz := a.tail()
	return collide.AABB{MinX: min(a.X, tx) - Radius, MinY: min(a.Y, ty) - Radius, MinZ: min(a.Z, tz) - Radius,
		MaxX: max(a.X, tx) + Radius, MaxY: max(a.Y, ty) + Radius, MaxZ: max(a.Z, tz) + Radius}
}

// Frame is the tip and yaw.
func (a *Arrow) Frame() (x, y, z, yaw float64) { return a.X, a.Y, a.Z, a.Yaw }

// Intersect finds where segment p0→p1 first comes within Radius of the
// shaft (closest approach of the two segments).
func (a *Arrow) Intersect(x0, y0, z0, x1, y1, z1 float64) (x, y, z float64, part int, ok bool) {
	tx, ty, tz := a.tail()
	p := mgl64.Vec3{x0, y0, z0}
	u := mgl64.Vec3{x1 - x0, y1 - y0, z1 - z0}
	q := mgl64.Vec3{tx, ty, tz}
	v := mgl64.Vec3{a.X - tx, a.Y - ty, a.Z - tz}
	s, t := closest(p, u, q, v)
	c1, c2 := p.Add(u.Mul(s)), q.Add(v.Mul(t))
	if c1.Sub(c2).Len() > Radius {
		return 0, 0, 0, -1, false
	}
	return c1[0], c1[1], c1[2], 0, true
}

// closest returns the parameters (0..1) of the closest points between
// segments p+s*u and q+t*v.
func closest(p, u, q, v mgl64.Vec3) (s, t float64) {
	w := p.Sub(q)
	a, b, c, d, e := u.Dot(u), u.Dot(v), v.Dot(v), u.Dot(w), v.Dot(w)
	den := a*c - b*b
	clamp := func(x float64) float64 { return math.Max(0, math.Min(1, x)) }
	if den > 1e-12 {
		s = clamp((b*e - c*d) / den)
	}
	if c > 1e-12 {
		t = clamp((b*s + e) / c)
	}
	if a > 1e-12 {
		s = clamp((b*t - d) / a)
	}
	return s, t
}

// Joint is the arrow's frame: origin at the tip, +Z along its flight.
func (a *Arrow) Joint(int) mgl64.Mat4 {
	return mgl64.Translate3D(a.X, a.Y, a.Z).Mul4(mgl64.HomogRotate3DY(math.Pi - a.Yaw)).Mul4(mgl64.HomogRotate3DX(-a.Pitch))
}
