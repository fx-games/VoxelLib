package actors

import "math"

// Flight makes an actor a flier: it steers straight at a target block's
// centre by pulling its velocity toward fixed per-axis speeds, its
// vertical speed is damped after each move (which holds it up against
// gravity), and it can sit still. A flier takes no fall damage, is not
// pushed by or pushing others, and has no drive while on the ground.
// Think, the game's behaviour, runs instead of the Brain each tick (with
// the input cleared); it picks targets and calls Steer.
type Flight struct {
	Target  [3]int
	Has     bool    // Target is set
	Time    int     // Steer calls since the target was set
	MaxTime int     // Steer drops the target at this age (0: never)
	Speed   float64 // velocity pull per tick (0..1)
	Pull    Vec     // per-axis speed pulled toward, signed by the target's side
	Forward float64 // forward input while steering
	Hover   float64 // vertical speed factor after each move while not Still
	Still   bool    // sitting: all motion cleared after each move
	Think   func(a *Actor)
}

// SetTarget flies to block (x, y, z) from now on.
func (f *Flight) SetTarget(x, y, z int) { f.Target, f.Has, f.Time = [3]int{x, y, z}, true, 0 }

// Cancel drops the target.
func (f *Flight) Cancel() { f.Has, f.Time = false, 0 }

// Dist2 is the squared distance from p to the target block's centre.
func (f *Flight) Dist2(p Vec) float64 {
	return p.dist2(Vec{float64(f.Target[0]) + 0.5, float64(f.Target[1]) + 0.5, float64(f.Target[2]) + 0.5})
}

// Steer pulls a's velocity toward the target on each axis by Speed, turns
// a to its new motion, sets the forward input and ages the target.
func (a *Actor) Steer() {
	f, b := a.Fly, a.Body
	if f == nil || !f.Has {
		return
	}
	dx := float64(f.Target[0]) + 0.5 - b.X
	dy := float64(f.Target[1]) + 0.5 - b.Y
	dz := float64(f.Target[2]) + 0.5 - b.Z
	b.VX += (sign(dx)*f.Pull.X - b.VX) * f.Speed
	b.VY += (sign(dy)*f.Pull.Y - b.VY) * f.Speed
	b.VZ += (sign(dz)*f.Pull.Z - b.VZ) * f.Speed
	a.Yaw += math.Remainder(math.Atan2(b.VX, -b.VZ)-a.Yaw, 2*math.Pi)
	a.nav.in.Forward = f.Forward
	if f.Time++; f.MaxTime > 0 && f.Time >= f.MaxTime {
		f.Cancel()
	}
}

// sign is Math.signum: -1, 0 or 1.
func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
