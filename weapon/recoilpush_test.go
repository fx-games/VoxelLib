package weapon

import (
	"math"
	"testing"
)

// getFinalStrength: stance order, per-projectile scaling and the clamp.
func TestRecoilPushStrength(t *testing.T) {
	m95 := RecoilPush{Ground: 0.05, Air: 0.25, Sneak: 0.025, Crawl: 0, ProjectileMultiplier: 1}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-12 }
	for _, c := range []struct {
		ground, sneak, crawl bool
		want                 float64
	}{{true, false, false, 0.05}, {false, true, true, 0.25}, {true, true, false, 0.025}, {true, true, true, 0}} {
		if got := m95.Strength(c.ground, c.sneak, c.crawl, 1); !near(got, c.want) {
			t.Errorf("m95 ground %v sneak %v crawl %v: %v, want %v", c.ground, c.sneak, c.crawl, got, c.want)
		}
	}
	db := RecoilPush{Ground: 0.02, Air: 0.1, PerProjectile: true, ProjectileMultiplier: 0.35}
	if got := db.Strength(true, false, false, 6); !near(got, 0.02*6*0.35) {
		t.Errorf("per projectile: %v", got)
	}
	if got := (&RecoilPush{Air: 3, PerProjectile: true, ProjectileMultiplier: 1}).Strength(false, false, false, 2); got != maxRecoilPush {
		t.Errorf("clamp: %v", got)
	}
	if p := (*RecoilPush)(nil).Push([3]float64{0, 0, 1}, true, false, 1); p != ([3]float64{}) {
		t.Errorf("nil push %v", p)
	}
	if p := m95.Push([3]float64{0, 0, 1}, true, false, 1); !near(p[2], -0.05) {
		t.Errorf("push %v, want back along the look", p)
	}
}
