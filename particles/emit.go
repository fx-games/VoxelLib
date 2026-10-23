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

// Burst emits n particles in random directions on the sphere: per
// particle a horizontal angle, a vertical angle (both rnd()*2π) and a
// speed rnd()*(hi-lo)+lo, in that order, with table sines and cosines.
// The caller adds the source's own motion.
func Burst(rnd func() float32, n int, lo, hi float32, emit func(vx, vy, vz float64)) {
	for range n {
		xz := rnd() * fmath.Pi * 2
		y := rnd() * fmath.Pi * 2
		sp := rnd()*(hi-lo) + lo
		emit(float64(fmath.Cos(xz)*fmath.Cos(y)*sp), float64(fmath.Sin(y)*sp), float64(fmath.Sin(xz)*fmath.Cos(y)*sp))
	}
}

// Flipbook is the frame of a 4x4-frame sheet animation over life ticks:
// frame (int)((age+pt)*15/life), its top-left UV (frames run along the
// rows) and the frame edge 0.24975; ok is false past the last frame.
func Flipbook(age int, pt float32, life int) (u, v, edge float32, ok bool) {
	f := int((float32(age) + pt) * 15 / float32(life))
	if f > 15 {
		return 0, 0, 0, false
	}
	return float32(f%4) / 4, float32(f/4) / 4, 0.24975, true
}

// Scatter emits n points around (x, y, z), each axis offset by
// (r-r)*spread with double draws in x, y, z order.
func Scatter(rnd func() float64, n int, x, y, z, spread float64, emit func(x, y, z float64)) {
	for i := 0; i < n; i++ {
		px := x + (rnd()-rnd())*spread
		py := y + (rnd()-rnd())*spread
		pz := z + (rnd()-rnd())*spread
		emit(px, py, pz)
	}
}
