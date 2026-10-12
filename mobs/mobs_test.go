package mobs

import (
	"math"
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

var floor = collide.SolidsFunc(func(x, y, z int) bool { return y < 0 })

func TestWalkerWanders(t *testing.T) {
	m := New(Walker, 0.5, 3, 0.5, 0.6, 0.8)
	rnd := rand.New(rand.NewSource(1))
	seen := map[State]bool{}
	maxD := 0.0
	for i := 0; i < 4000; i++ {
		m.Tick(floor, rnd)
		seen[m.State] = true
		maxD = math.Max(maxD, math.Hypot(m.X-m.HomeX, m.Z-m.HomeZ))
		if m.Y < -0.001 {
			t.Fatal("fell through the floor")
		}
	}
	if !seen[Walk] || !seen[Idle] || !seen[Trick] || maxD < 2 || maxD > m.Range+2 {
		t.Fatalf("states %v, max distance %.1f", seen, maxD)
	}
}

func TestFlyerStaysAirborne(t *testing.T) {
	m := New(Flyer, 0.5, 2, 0.5, 0.5, 0.5)
	rnd := rand.New(rand.NewSource(2))
	for i := 0; i < 2000; i++ {
		m.Tick(floor, rnd)
	}
	if m.Y < 1 || m.Y > m.HomeY+7 {
		t.Fatalf("flyer at y=%.1f", m.Y)
	}
}
