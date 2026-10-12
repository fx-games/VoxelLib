package character

import (
	"math"
	"testing"
)

// grid is a test world: solid below y=0, water in [waterMin, waterMax) when set.
type grid struct {
	waterTop int
	solid    func(x, y, z int) bool
}

func (g grid) Solid(x, y, z int) bool {
	if g.solid != nil {
		return g.solid(x, y, z)
	}
	return y < 0
}

func (g grid) Liquid(x, y, z int) Liquid {
	if y >= 0 && y < g.waterTop {
		return LiquidWater
	}
	return LiquidNone
}

func run(c *Character, w World, in Input, ticks int) {
	for i := 0; i < ticks; i++ {
		c.Tick(w, in)
	}
}

func speed(c *Character) float64 { return math.Hypot(c.X-c.PrevX, c.Z-c.PrevZ) }

func TestWalkSpeedMatchesMinecraft(t *testing.T) {
	c := New(0.5, 0, 0.5)
	run(c, grid{}, Input{}, 5) // settle on the ground
	run(c, grid{}, Input{Forward: 1}, 60)
	if s := speed(c) * 20; math.Abs(s-4.317) > 0.05 {
		t.Fatalf("walk speed %.3f b/s, want 4.317", s)
	}
	run(c, grid{}, Input{Forward: 1, Sprint: true}, 60)
	if s := speed(c) * 20; math.Abs(s-5.612) > 0.05 {
		t.Fatalf("sprint speed %.3f b/s, want 5.612", s)
	}
}

func TestFlySpeedMatchesMinecraft(t *testing.T) {
	c := New(0.5, 10, 0.5)
	c.Flying = true
	run(c, grid{}, Input{Forward: 1}, 100)
	if s := speed(c) * 20; math.Abs(s-10.89) > 0.1 {
		t.Fatalf("fly speed %.2f b/s, want 10.89", s)
	}
	run(c, grid{}, Input{Forward: 1, Sprint: true}, 100)
	if s := speed(c) * 20; math.Abs(s-21.78) > 0.2 {
		t.Fatalf("sprint-fly speed %.2f b/s, want 21.78", s)
	}
}

func TestWaterSinkSwimUp(t *testing.T) {
	w := grid{waterTop: 20}
	c := New(0.5, 10, 0.5)
	run(c, w, Input{}, 40)
	if !c.InWater || c.EyeIn != LiquidWater {
		t.Fatalf("not underwater: inWater=%v eye=%v", c.InWater, c.EyeIn)
	}
	if c.Y >= 10 || c.Y < 8 {
		t.Fatalf("idle swimmer should sink slowly, y=%.2f", c.Y)
	}
	y := c.Y
	run(c, w, Input{Jump: true}, 40)
	if c.Y <= y+2 {
		t.Fatalf("jump in water should rise, y %.2f -> %.2f", y, c.Y)
	}
	y = c.Y
	run(c, w, Input{Sneak: true}, 20)
	if c.Y >= y-1 {
		t.Fatalf("sneak in water should sink, y %.2f -> %.2f", y, c.Y)
	}
}

func TestDiveFollowsLook(t *testing.T) {
	w := grid{waterTop: 40}
	c := New(0.5, 30, 0.5)
	run(c, w, Input{}, 2)
	run(c, w, Input{Forward: 1, Sprint: true, Pitch: -math.Pi / 3}, 40)
	if !c.Swimming {
		t.Fatal("sprinting underwater should swim")
	}
	if c.Y > 26 {
		t.Fatalf("diving down should descend fast, y=%.2f", c.Y)
	}
}

func TestSneakStopsAtEdge(t *testing.T) {
	// a 1-wide platform at x in [0,1)
	w := grid{solid: func(x, y, z int) bool { return y < 0 && x == 0 }}
	c := New(0.5, 0, 0.5)
	run(c, w, Input{}, 3)
	run(c, w, Input{Strafe: 1, Sneak: true}, 60)
	if c.Y < 0 || c.X > 1.4 {
		t.Fatalf("sneaking walked off the edge: x=%.2f y=%.2f", c.X, c.Y)
	}
}
