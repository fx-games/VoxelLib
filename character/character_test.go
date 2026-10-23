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

func TestWalkSpeedMatchesReference(t *testing.T) {
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

func TestFlySpeedMatchesReference(t *testing.T) {
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

type ladderWorld struct{ grid }

func (ladderWorld) Climbable(x, y, z int) bool { return x == 0 && z == 0 && y >= 0 && y < 10 }

// ladder at x=z=0 against a wall at x=1.
func ladder() ladderWorld {
	return ladderWorld{grid{solid: func(x, y, z int) bool { return y < 0 || x == 1 && z == 0 && y >= 0 }}}
}

func TestLadderClimbUp(t *testing.T) {
	c := New(0.5, 2, 0.5)
	run(c, ladder(), Input{Forward: 1, Yaw: math.Pi / 2}, 30) // yaw 90deg walks +X into the wall
	if c.Y < 4 || c.VY <= 0 {
		t.Fatalf("y=%.2f vy=%.2f, want climbing", c.Y, c.VY)
	}
}

func TestLadderSlowFall(t *testing.T) {
	c := New(0.5, 9, 0.5)
	for i := 0; i < 30; i++ {
		y := c.Y
		c.Tick(ladder(), Input{})
		if y-c.Y > 0.15+1e-9 {
			t.Fatalf("fell %.3f in one tick, want at most 0.15", y-c.Y)
		}
	}
	if c.Y > 5 {
		t.Fatalf("y=%.2f, did not fall", c.Y)
	}
}

func TestLadderSneakHolds(t *testing.T) {
	c := New(0.5, 5, 0.5)
	run(c, ladder(), Input{Sneak: true}, 30)
	if math.Abs(c.Y-5) > 0.01 {
		t.Fatalf("y=%.3f, want held at 5", c.Y)
	}
}

func TestQuakeStrafeJumpGainsSpeed(t *testing.T) {
	c := New(0.5, 0, 0.5)
	c.Model = MoveQuake
	w := grid{}
	run(c, w, Input{}, 5)
	run(c, w, Input{Forward: 1}, 40)
	start := speed(c) * 20
	if math.Abs(start-pmSpeed) > 0.1 {
		t.Fatalf("ground speed %.2f, want %.2f", start, pmSpeed)
	}
	// Airborne, look 45 degrees off the travel direction and strafe.
	c.Tick(w, Input{Forward: 1, Jump: true})
	for i := 0; i < 8; i++ {
		c.Tick(w, Input{Forward: 1, Strafe: 1, Yaw: 0.9})
	}
	if c.OnGround {
		t.Fatal("expected to be airborne")
	}
	if s := math.Hypot(c.VX, c.VZ) * 20; s <= start {
		t.Fatalf("air speed %.2f, want more than %.2f", s, start)
	}
}

func TestQuakeJumpNeedsRelease(t *testing.T) {
	c := New(0.5, 0, 0.5)
	c.Model = MoveQuake
	run(c, grid{}, Input{}, 5)
	c.Tick(grid{}, Input{Jump: true})
	if math.Abs(c.VY*20-(pmJumpVelocity-q3Gravity*0.05)) > 0.01 {
		t.Fatalf("jump vy %.3f", c.VY*20)
	}
	run(c, grid{}, Input{Jump: true}, 60) // held: lands, does not rejump
	if !c.OnGround || c.VY != 0 && c.Y > 0.01 {
		t.Fatalf("held jump key rejumped: y=%.2f", c.Y)
	}
}

func TestQuakeFriction(t *testing.T) {
	vx, vz := pmoveSingle(pmSpeed, 0, 0, 0, true, false, 1)
	if vx != 0 || vz != 0 {
		t.Fatal("friction should stop the player")
	}
	vx, _ = pmoveSingle(pmSpeed, 0, 0, 0, false, false, 1)
	if vx != pmSpeed {
		t.Fatal("no friction in the air")
	}
}
