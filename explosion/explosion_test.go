package explosion

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

type half struct{}

func (half) Float32() float32 { return 0.5 }

func TestBlocks(t *testing.T) {
	b := Blast{X: 0.5, Y: 0.5, Z: 0.5, Size: 2}
	air := func(x, y, z int) (float32, bool) { return 0, true }
	got := map[[3]int]bool{}
	for _, p := range b.Blocks(half{}, air) {
		got[p] = true
	}
	// strength 2 in air lasts 9 steps of 0.3: reach 2.4 from the centre.
	if !got[[3]int{0, 0, 0}] || !got[[3]int{2, 0, 0}] || got[[3]int{3, 0, 0}] {
		t.Fatalf("air reach wrong: centre %v, x2 %v, x3 %v", got[[3]int{0, 0, 0}], got[[3]int{2, 0, 0}], got[[3]int{3, 0, 0}])
	}
	// a wall of resistance 1200 (obsidian) at x=1 stops every ray.
	wall := func(x, y, z int) (float32, bool) {
		if x == 1 {
			return 1200, false
		}
		return 0, true
	}
	for _, p := range b.Blocks(half{}, wall) {
		if p[0] >= 1 {
			t.Fatalf("blast passed the wall at %v", p)
		}
	}
	// resistance 0.5 (dirt) is broken.
	dirt := func(x, y, z int) (float32, bool) { return 0.5, x != 1 }
	if !contains(b.Blocks(half{}, dirt), [3]int{1, 0, 0}) {
		t.Fatal("dirt at x=1 survived")
	}
}

func contains(ps [][3]int, q [3]int) bool {
	for _, p := range ps {
		if p == q {
			return true
		}
	}
	return false
}

func TestExposureHitKnock(t *testing.T) {
	box := collide.Box(2.5, 0, 0.5, 0.6, 1.8)
	n := 0
	all := Exposure(0.5, 0.5, 0.5, box, func(x0, y0, z0, x1, y1, z1 float64) bool { n++; return true })
	if all != 1 || n != 45 { // 3 x 5 x 3 samples
		t.Fatalf("exposure %v over %d samples", all, n)
	}
	low := Exposure(0.5, 0.5, 0.5, box, func(x0, y0, z0, x1, y1, z1 float64) bool { return y0 < 0.5 })
	if low != float32(18)/45 { // 2 of the 5 rows (y 0 and 0.39) are clear
		t.Fatalf("half exposure %v", low)
	}
	b := Blast{X: 0.5, Y: 0, Z: 0.5, Size: 2}
	dx, _, _, impact, dmg, ok := b.Hit(2.5, 0, 0.5, 0, 1)
	if !ok || dx != 1 || impact != 0.5 || dmg != 13 {
		t.Fatalf("hit %v %v %v %v", ok, dx, impact, dmg)
	}
	if _, _, _, _, _, ok := b.Hit(4.6, 0, 0.5, 0, 1); ok {
		t.Fatal("hit beyond 2*Size")
	}
	if Knock(0.5, 4) != 0.5 || Knock(1, 8) != 0 {
		t.Fatalf("knock %v %v", Knock(0.5, 4), Knock(1, 8))
	}
	if r := b.Reach(); r.MinX != -5 || r.MaxX != 5 {
		t.Fatalf("reach %+v", r)
	}
}

func TestDebris(t *testing.T) {
	b := Blast{X: 0.5, Y: 0.5, Z: 0.5, Size: 2}
	px, _, _, vx, vy, _ := b.Debris(half{}, 2, 0, 0)
	if px != 2.5 || math.Abs(vx-0.25) > 1e-7 || vy != 0 {
		t.Fatalf("debris %v %v %v", px, vx, vy)
	}
	if Pitch(half{}) != 0.7 {
		t.Fatal("pitch")
	}
}
