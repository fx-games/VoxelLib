package character

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
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

// TestUsingItemSlows: EntityPlayerSP scales the input by 0.2 while an
// item is in use and refuses to sprint, so the steady ground speed is a
// fifth of walking (4.317 * 0.2 = 0.863 b/s), sprint key or not.
func TestUsingItemSlows(t *testing.T) {
	for _, sprint := range []bool{false, true} {
		c := New(0.5, 0, 0.5)
		run(c, grid{}, Input{}, 5)
		run(c, grid{}, Input{Forward: 1, Sprint: sprint, Using: true}, 80)
		if s := speed(c) * 20; math.Abs(s-0.8634) > 0.01 {
			t.Errorf("sprint %v: using speed %.4f b/s, want 0.863", sprint, s)
		}
	}
}

// ledge is flat ground with a box of height h covering z < -2 (ahead when
// walking north); h >= 1 is a full block.
type ledge struct {
	grid
	h float64
}

func (l ledge) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	return append(out, collide.AABB{MinX: -10, MinY: 0, MinZ: -10, MaxX: 10, MaxY: l.h, MaxZ: -2})
}

// TestStepUp is Entity.moveEntity's stepHeight 0.5: carpets and slabs are
// walked onto, a full block stops the player.
func TestStepUp(t *testing.T) {
	for _, tc := range []struct {
		h, wantY float64
		past     bool
	}{{0.0625, 0.0625, true}, {0.5, 0.5, true}, {0.5625, 0, false}, {1, 0, false}} {
		c := New(0.5, 0, 0.5)
		run(c, ledge{h: tc.h}, Input{}, 5)
		run(c, ledge{h: tc.h}, Input{Forward: 1}, 40)
		if past := c.Z < -2.5; past != tc.past || math.Abs(c.Y-tc.wantY) > 1e-9 {
			t.Errorf("h %.4f: z %.3f y %.4f, want past %v y %.4f", tc.h, c.Z, c.Y, tc.past, tc.wantY)
		}
	}
}

// TestNoSprint: blindness (NoSprint) keeps a sprint from starting; one
// already running goes on (EntityPlayerSP.onLivingUpdate).
func TestNoSprint(t *testing.T) {
	c := New(0.5, 0, 0.5)
	run(c, grid{}, Input{}, 5)
	c.NoSprint = true
	run(c, grid{}, Input{Forward: 1, Sprint: true}, 60)
	if s := speed(c) * 20; c.Sprinting || math.Abs(s-4.317) > 0.05 {
		t.Fatalf("blind: sprinting %v at %.3f b/s, want a 4.317 walk", c.Sprinting, s)
	}
	c.NoSprint = false
	run(c, grid{}, Input{Forward: 1, Sprint: true}, 1)
	c.NoSprint = true
	run(c, grid{}, Input{Forward: 1, Sprint: true}, 60)
	if s := speed(c) * 20; !c.Sprinting || math.Abs(s-5.612) > 0.05 {
		t.Fatalf("sprint before blindness: sprinting %v at %.3f b/s, want 5.612", c.Sprinting, s)
	}
}
