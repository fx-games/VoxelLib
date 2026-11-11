package weapon

import "math/rand"

// Recoil is one shot's camera recoil as TACZ's CameraSetupEvent applies it:
// curves of degrees over seconds since the shot, through (0, 0) and each
// recoil key 30 ms late, at a random value in the key's range, joined by a
// natural cubic spline. The camera follows each frame's change of the
// curves, and the next shot replaces them, so sustained fire climbs.
type Recoil struct{ pitch, yaw spline }

// recoilDelay is TACZ's 30 ms shift of the recoil keys.
const recoilDelay = 0.03

// NewRecoil rolls a shot's recoil, scaled by mod (AimRecoil while aiming).
func (g *Gun) NewRecoil(r *rand.Rand, mod float32) Recoil {
	curve := func(keys []RecoilKey) spline {
		if len(keys) == 0 {
			return spline{}
		}
		t, v := []float64{0}, []float64{0}
		for _, k := range keys {
			t = append(t, float64(k.T)+recoilDelay)
			v = append(v, float64(k.Min+(k.Max-k.Min)*r.Float32())*float64(mod))
		}
		return newSpline(t, v)
	}
	return Recoil{curve(g.Recoil.Pitch), curve(g.Recoil.Yaw)}
}

// At is the recoil (degrees up, degrees left) t seconds after the shot; past
// a curve's end it holds its last value.
func (rc *Recoil) At(t float32) (pitch, yaw float32) {
	return float32(rc.pitch.at(float64(t))), float32(rc.yaw.at(float64(t)))
}

// AimRecoil is TACZ's recoil factor at aiming progress a (0..1) with sight
// magnification zoom: 1 - a + a / min(√zoom, 1.5).
func AimRecoil(a, zoom float32) float32 {
	return 1 - a + a/min(sqrt32(max(zoom, 1)), 1.5)
}

// spline is a natural cubic spline (Apache Commons' SplineInterpolator);
// with two points it is a line.
type spline struct {
	x, a, b, c, d []float64
}

func newSpline(x, y []float64) spline {
	n := len(x) - 1
	s := spline{x: x, a: y, b: make([]float64, n), c: make([]float64, n+1), d: make([]float64, n)}
	h := make([]float64, n)
	for i := range n {
		h[i] = x[i+1] - x[i]
	}
	if n >= 2 {
		mu, z := make([]float64, n), make([]float64, n+1)
		for i := 1; i < n; i++ {
			g := 2*(x[i+1]-x[i-1]) - h[i-1]*mu[i-1]
			mu[i] = h[i] / g
			z[i] = (3*(y[i+1]*h[i-1]-y[i]*(x[i+1]-x[i-1])+y[i-1]*h[i])/(h[i-1]*h[i]) - h[i-1]*z[i-1]) / g
		}
		for j := n - 1; j >= 0; j-- {
			s.c[j] = z[j] - mu[j]*s.c[j+1]
		}
	}
	for j := range n {
		s.b[j] = (y[j+1]-y[j])/h[j] - h[j]*(s.c[j+1]+2*s.c[j])/3
		s.d[j] = (s.c[j+1] - s.c[j]) / (3 * h[j])
	}
	return s
}

func (s *spline) at(t float64) float64 {
	n := len(s.b)
	if n == 0 {
		return 0
	}
	if t >= s.x[n] {
		return s.a[n]
	}
	i := 0
	for i+1 < n && t >= s.x[i+1] {
		i++
	}
	dx := max(t-s.x[i], 0)
	return s.a[i] + dx*(s.b[i]+dx*(s.c[i]+dx*s.d[i]))
}

func sqrt32(f float32) float32 {
	if f <= 0 {
		return 0
	}
	z := f
	for range 20 {
		z = (z + f/z) / 2
	}
	return z
}
