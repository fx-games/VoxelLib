package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func near32(a, b, eps float32) bool { return float32(math.Abs(float64(a-b))) <= eps }

func TestLotrTime(t *testing.T) {
	// worldTime is getWorldTime()%24000; the pack halves it with int division.
	for _, c := range []struct {
		wt   int64
		want int32
	}{{0, 0}, {1, 0}, {3, 1}, {23999, 11999}, {24000, 0}, {36001, 6000}, {48000*7 + 13000, 6500}} {
		if got := LotrTime(c.wt); got != c.want {
			t.Errorf("LotrTime(%d) = %d, want %d", c.wt, got, c.want)
		}
	}
}

func TestSunPosition(t *testing.T) {
	// Identity view: noon (ca 0) puts the sun straight up, tilted by
	// sunPathRotation -40° about Z after the -90° Y turn.
	sun, moon := packCelestial(mgl32.Ident4(), 0)
	if !near32(sun.Len(), 100, 1e-3) || !near32(sun.Add(moon).Len(), 0, 1e-3) {
		t.Fatalf("sun %v moon %v", sun, moon)
	}
	want := float32(100 * math.Cos(40*math.Pi/180))
	if !near32(sun[1], want, 1e-3) || !near32(sun[0], 0, 1e-3) {
		t.Errorf("noon sun %v, want y %v x 0", sun, want)
	}
	// Midnight (ca 0.5): the sun is below the horizon.
	if s, _ := packCelestial(mgl32.Ident4(), 0.5); s[1] >= 0 {
		t.Errorf("midnight sun %v above horizon", s)
	}
	// ca 0.25 (sunset): the sun sets in the west (-X) for this view.
	if s, _ := packCelestial(mgl32.Ident4(), 0.25); !near32(s[1], 0, 1e-3) || s[0] >= 0 {
		t.Errorf("sunset sun %v", s)
	}
}

func TestSunAngleFromLotrTime(t *testing.T) {
	// LOTR noon (t = 12000 of 48000) is celestial angle 0 → sunAngle 0.25;
	// midnight is 0.5 → sunAngle 0.75 and the moon casts the shadow.
	for _, c := range []struct {
		t        float64
		sa, shad float32
	}{{12000, 0.25, 0.25}, {36000, 0.75, 0.25}} {
		ca := lotrCelestial(c.t)
		sa := SunAngle(ca)
		_, sh := ShadowModelView(ca, 0, 0, 0)
		if !near32(sa, c.sa, 1e-5) || !near32(sh, c.shad, 1e-5) {
			t.Errorf("t %v: ca %v sunAngle %v shadowAngle %v", c.t, ca, sa, sh)
		}
	}
}

// lotrCelestial mirrors LOTRWorldProvider.calculateCelestialAngle
// (VoxelHobbit look.CelestialAngle) for the test.
func lotrCelestial(t float64) float32 {
	d := float32(math.Mod(t, 48000)) / 48000
	d -= 0.25
	if d < 0 {
		d++
	}
	a := 1 - float32((math.Cos(float64(d)*math.Pi)+1)/2)
	return d + (a-d)/3
}

func TestShadowProjection(t *testing.T) {
	// At noon the shadow camera looks straight down along the sun path:
	// the camera position projects to the map centre, a point 10 blocks
	// above it nearer the light (smaller depth).
	mv, _ := ShadowModelView(0, 1, 65, 1) // x%2-1 = 0: no snap offset
	p := ShadowProjection()
	proj := func(v mgl32.Vec3) mgl32.Vec4 { return ShadowDistort(p.Mul4(mv).Mul4x1(v.Vec4(1))) }
	c := proj(mgl32.Vec3{})
	if !near32(c[0], 0, 0.02) || !near32(c[1], 0, 0.02) {
		t.Errorf("camera maps to %v, want centre", c)
	}
	if up := proj(mgl32.Vec3{0, 10, 0}); up[2] >= c[2] {
		t.Errorf("point above camera depth %v >= %v", up[2], c[2])
	}
	// Distortion: centre is magnified by 1/(log(a)·k) = 1/(0.05·1.8),
	// the half-plane edge (80 blocks → 1.0) shrinks to 1/(log(e)·1.8).
	if d := ShadowDistort(mgl32.Vec4{1e-6, 0, 0, 1}); !near32(d[0]/1e-6, 1/(0.05*1.8), 1e-2) {
		t.Errorf("centre magnification %v", d[0]/1e-6)
	}
	if d := ShadowDistort(mgl32.Vec4{1, 0, 0, 1}); !near32(d[0], float32(1/(math.Log(math.Exp(0.05)+(math.E-math.Exp(0.05))*80/128)*1.8)), 1e-5) {
		t.Errorf("edge %v", d[0])
	}
	// Camera translation snaps to the 2-block interval (Java frem keeps
	// the sign): x = -320.25 → -0.25-1.
	m, _ := ShadowModelView(0, -320.25, 0, 0)
	m0, _ := ShadowModelView(0, 0, 0, 0)
	got := m.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Sub(m0.Mul4x1(mgl32.Vec4{-1.25 + 1, 0, 0, 1}))
	if got.Len() > 1e-4 {
		t.Errorf("interval snap off by %v", got)
	}
}

func TestPackSmoothing(t *testing.T) {
	var s packState
	f := &PackFrame{Rain: 0, EyeLight: 15<<20 | 7<<4, Far: 128}
	s.update(f, [3]float64{}, mgl32.Ident4(), mgl32.Ident4(), 100, 50, 0)
	f.Rain = 1
	// wetness < rain uses drynessHalflife 75: after 75*100 ms half way.
	u := s.update(f, [3]float64{}, mgl32.Ident4(), mgl32.Ident4(), 100, 50, 7500)
	if !near32(u.Weather[1], 0.5, 1e-4) {
		t.Errorf("wetness %v, want 0.5", u.Weather[1])
	}
	// eyeBrightness (block*16, sky*16); the fade starts at 0 with halflife 10 (1 s).
	if u.Eye2[0] != 112 || u.Eye2[1] != 240 {
		t.Errorf("eyeBrightness %v", u.Eye2)
	}
	if u.Ints[2] != 2 || !near32(u.View[3], 7.5, 1e-5) {
		t.Errorf("frameCounter %d frameTimeCounter %v", u.Ints[2], u.View[3])
	}
}
