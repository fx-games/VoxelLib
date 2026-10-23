package particles

import "github.com/memmaker/voxellib/fmath"

// Ring emits a ring of particles: one every step degrees on a circle of
// radius r (table sine of the float radians), moving outward at speed
// times the offset. dx and dz are the offsets from the centre.
func Ring(r, speed float64, step int, emit func(dx, dz, vx, vz float64)) {
	for i := 0; i < 360; i += step {
		a := float32(float64(i) * 3.141592653589793 / 180)
		dx, dz := r*float64(fmath.Sin(a)), r*float64(fmath.Cos(a))
		emit(dx, dz, dx*speed, dz*speed)
	}
}

// Trail is the in-flight trail of a fast missile: n particles spread
// along this tick's motion (position + motion*i/n), each moving back
// against it with up lift.
func Trail(n int, x, y, z, mx, my, mz, up float64, emit func(x, y, z, vx, vy, vz float64)) {
	for i := range n {
		f := float64(i) / float64(n)
		emit(x+mx*f, y+my*f, z+mz*f, -mx, -my+up, -mz)
	}
}

// Flash is a billboard flash that shrinks and fades by the tick: half
// size Size0 - age*Shrink, alpha Alpha0 - (age + pt - 1)*Fade.
type Flash struct{ Size0, Shrink, Alpha0, Fade float32 }

// Half is the quad's half size at an age.
func (f Flash) Half(age int) float32 { return f.Size0 - float32(age)*f.Shrink }

// Alpha is the alpha at an age and partial tick, 0 once faded.
func (f Flash) Alpha(age int, pt float32) float32 {
	return max(0, f.Alpha0-(float32(age)+pt-1)*f.Fade)
}
