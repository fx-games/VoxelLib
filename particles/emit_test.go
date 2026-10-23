package particles

import (
	"math"
	"testing"
)

func TestRingTrailFlash(t *testing.T) {
	n := 0
	Ring(1.5, 0.2, 2, func(dx, dz, vx, vz float64) {
		if math.Abs(math.Hypot(dx, dz)-1.5) > 1e-3 || math.Abs(vx-dx*0.2) > 1e-12 {
			t.Fatalf("ring point %v %v %v", dx, dz, vx)
		}
		n++
	})
	if n != 180 {
		t.Errorf("ring %d", n)
	}
	var ys []float64
	Trail(4, 0, 0, 0, 0, 2, 0, 0.2, func(x, y, z, vx, vy, vz float64) {
		ys = append(ys, y)
		if vy != -1.8 {
			t.Errorf("trail vy %v", vy)
		}
	})
	if len(ys) != 4 || ys[3] != 1.5 {
		t.Errorf("trail %v", ys)
	}
	f := Flash{Size0: 16, Shrink: 0.2, Alpha0: 0.9, Fade: 0.15}
	if f.Half(5) != 15 || f.Alpha(1, 0) != 0.9 || f.Alpha(10, 0) != 0 {
		t.Errorf("flash %v %v", f.Half(5), f.Alpha(1, 0))
	}
}

// TestBurst: speeds stay in [lo, hi], three draws per particle.
func TestBurst(t *testing.T) {
	draws, n := 0, 0
	r := func() float32 { draws++; return float32(draws%7) / 7 }
	Burst(r, 20, 0.1, 0.15, func(vx, vy, vz float64) {
		if s := math.Sqrt(vx*vx + vy*vy + vz*vz); s < 0.1-1e-3 || s > 0.15+1e-3 {
			t.Errorf("speed %v", s)
		}
		n++
	})
	if n != 20 || draws != 60 {
		t.Errorf("%d particles, %d draws", n, draws)
	}
}

func TestFlipbookScatter(t *testing.T) {
	if u, v, e, ok := Flipbook(3, 0.5, 7); !ok || u != 0.75 || v != 0.25 || e != 0.24975 { // frame 7
		t.Fatalf("frame %v %v %v %v", u, v, e, ok)
	}
	if _, _, _, ok := Flipbook(9, 0, 8); ok {
		t.Fatal("frame past 15")
	}
	r := []float64{0.75, 0.25, 0, 0, 0.5, 0.5}
	i := 0
	Scatter(func() float64 { i++; return r[i-1] }, 1, 1, 2, 3, 4, func(x, y, z float64) {
		if x != 3 || y != 2 || z != 3 {
			t.Fatalf("scatter %v %v %v", x, y, z)
		}
	})
}
