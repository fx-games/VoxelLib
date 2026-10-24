package particles

import (
	"math/rand"
	"testing"
)

// TestFXMoveBox: EntityFX's 0.2 box stops 0.1 below a ceiling (moveEntity
// clips Y first and zeroes the motion), lands 0.1 above a floor with
// onGround, and slides past a wall block it does not overlap on Y.
func TestFXMoveBox(t *testing.T) {
	s := &System{Rand: rand.New(rand.NewSource(1)), Solid: func(x, y, z int) bool { return y == 10 || y == 4 || (x == 1 && y == 7) }}
	p := FX{X: 0.5, Y: 9.85, Z: 0.5, MY: 0.2, MX: 0.05}
	s.Move(&p)
	if p.Y != 9.9 || p.MY != 0 || p.X != 0.55 || p.OnGround {
		t.Fatalf("ceiling: y %v my %v x %v ground %v", p.Y, p.MY, p.X, p.OnGround)
	}
	p = FX{X: 0.5, Y: 5.15, Z: 0.5, MY: -0.2}
	s.Move(&p)
	if p.Y != 5.1 || !p.OnGround {
		t.Fatalf("floor: y %v ground %v", p.Y, p.OnGround)
	}
	p = FX{X: 0.85, Y: 7.5, Z: 0.5, MX: 0.2}
	s.Move(&p)
	if p.X != 0.9 || p.MX != 0 {
		t.Fatalf("wall: x %v mx %v", p.X, p.MX)
	}
	p = FX{X: 0.85, Y: 6.5, Z: 0.5, MX: 0.2} // below the wall block
	s.Move(&p)
	if p.X != 1.05 {
		t.Fatalf("free: x %v", p.X)
	}
}

// TestPortalFX: EntityPortalFX starts at its position + motion, one block
// up, and closes in on its position over maxAge ticks (40..49), growing from
// nothing to its 0.5..0.7 scale; red 0.9, green 0.3 of its grey.
func TestPortalFX(t *testing.T) {
	s := &System{Rand: rand.New(rand.NewSource(1))}
	p, _ := s.Make("portal", 3, 5, 7, 1, 0.5, -1)
	if p.MaxAge < 40 || p.MaxAge > 49 || p.Scale < 0.5 || p.Scale > 0.7 || p.R != p.B*0.9 || p.G != p.B*0.3 {
		t.Fatalf("make: %+v", p)
	}
	if sz, _ := s.Sprite(&p, 0); sz != 0 {
		t.Fatalf("size at birth %v", sz)
	}
	s.FX = []FX{p}
	s.TickFX()
	if q := s.FX[0]; q.X != 4 || q.Y != 6.5 || q.Z != 6 {
		t.Fatalf("first tick %v %v %v", q.X, q.Y, q.Z)
	}
	for s.FX[0].Age < s.FX[0].MaxAge {
		s.TickFX()
	}
	// The last tick drawn: age maxAge-1, 1 - (-f + 2f^2) of the way out.
	f := float32(p.MaxAge-1) / float32(p.MaxAge)
	v := float64(1 - (-f + f*f*2))
	if q := s.FX[0]; q.X != 3+v || q.Y != 5+0.5*v+float64(1-f) {
		t.Fatalf("last tick %v %v", q.X, q.Y)
	}
	if s.TickFX(); len(s.FX) != 0 {
		t.Fatal("alive after maxAge")
	}
}

// TestWaterFX: EntityBubbleFX keeps setSize(0.02)'s corner (0.09 off its
// spot), rises 0.002 a tick and dies out of water; EntitySplashFX starts
// at rest plus the rain's hop (0.1..0.3 up, texture 20..23), takes the
// given motion when it is level, and falls 0.04 a tick onto the ground.
func TestWaterFX(t *testing.T) {
	wet := true
	s := &System{Rand: rand.New(rand.NewSource(1)), Solid: func(x, y, z int) bool { return y == 4 },
		Water: func(x, y, z int) bool { return wet }}
	p, _ := s.Make("bubble", 0.5, 7.5, 0.5, 0, 0, 0)
	if d := 0.5 - p.X; d < 0.0899 || d > 0.0901 || p.Tex != 32 || p.MaxAge < 8 || p.MaxAge > 40 || p.W != 0.019999999552965164 {
		t.Fatal("bubble", p.X, p.Tex, p.MaxAge, p.W)
	}
	s.FX = []FX{p}
	my := p.MY
	s.TickFX()
	if len(s.FX) != 1 || s.FX[0].MY != (my+0.002)*0.8500000238418579 {
		t.Fatal("bubble rise", s.FX[0].MY)
	}
	wet = false
	s.TickFX()
	if len(s.FX) != 0 {
		t.Fatal("bubble out of water")
	}

	p, _ = s.Make("splash", 0.5, 5.5, 0.5, 0, 0, 0)
	if p.MY < 0.1 || p.MY > 0.3 || p.Tex < 20 || p.Tex > 23 || p.W != 0.009999999776482582 {
		t.Fatal("splash", p.MY, p.Tex)
	}
	if q, _ := s.Make("splash", 0.5, 5.5, 0.5, 0.2, 0, 0); q.MX != 0.2 || q.MY != 0.1 {
		t.Fatal("level splash", q.MX, q.MY)
	}
	p.MaxAge = 100
	s.FX = []FX{p}
	for i := 0; i < 20 && len(s.FX) == 1 && !s.FX[0].OnGround; i++ {
		s.TickFX()
	}
	if len(s.FX) == 1 && s.FX[0].Y != 5.1 {
		t.Fatal("splash landing", s.FX[0].Y)
	}
}

// TestFXWorldSpawnCamera: particles a particle spawns through
// World.spawnParticle (the huge explosion's largeexplode, lava's smoke,
// a drip's splash) pass RenderGlobal's 16-block camera check.
func TestFXWorldSpawnCamera(t *testing.T) {
	for _, d := range []float64{10, 30} {
		s := &System{Rand: rand.New(rand.NewSource(1))}
		s.Cam = [3]float64{d, 0, 0}
		s.FX = append(s.FX, func() FX { q, _ := s.Make("hugeexplosion", 0, 0, 0, 1, 0, 0); return q }())
		for range 3 {
			s.TickFX()
		}
		large := 0
		for _, p := range s.FX {
			if p.Kind == KindLarge {
				large++
			}
		}
		if (d < 16) != (large > 0) {
			t.Fatalf("camera at %v: %d largeexplode", d, large)
		}
	}
}
