package projectile

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/weapon"
)

type fakeBlocks struct{}

func (fakeBlocks) Block(x, y, z int) voxel.StateID     { return voxel.StateID(1) }
func (fakeBlocks) Liquid(x, y, z int) character.Liquid { return character.LiquidNone }

// wall: a full block at x=5 only (state ids are ignored, position decides).
func wallBoxes(x, y, z int, _ voxel.StateID, buf []collide.AABB) []collide.AABB {
	if x == 5 && y == 0 && z == 0 {
		buf = append(buf, collide.Block(x, y, z))
	}
	return buf
}

func noBoxes(int, int, int, voxel.StateID, []collide.AABB) []collide.AABB { return nil }

func mk(x0, y0, z0, x1, y1, z1 float64) collide.AABB {
	return collide.AABB{MinX: x0, MinY: y0, MinZ: z0, MaxX: x1, MaxY: y1, MaxZ: z1}
}

func testGun() *weapon.Gun {
	g := &weapon.Gun{}
	b := &g.Bullet
	b.Damage, b.Pellets, b.Speed, b.Gravity, b.Friction, b.Life, b.Pierce, b.Headshot = 10, 1, 200, 0.02, 0.01, 10, 1, 2
	b.TracerInterval = weapon.NoTracer
	b.Falloff = []weapon.Falloff{{Dist: 20, Damage: 10}, {Dist: 40, Damage: 6}}
	return g
}

// TestBulletTrajectory: the flight matches TACZ's tick (move by the
// velocity, then v·(1−friction) and −gravity) in closed form.
func TestBulletTrajectory(t *testing.T) {
	g := testGun()
	b := NewBullet([3]float64{0, 100, 0}, [3]float64{1, 0, 0}, g, 0, false, rand.New(rand.NewSource(1)))
	v0, f, gr := float64(g.Bullet.Speed)/20, float64(g.Bullet.Friction), float64(g.Bullet.Gravity)
	if b.Vel != [3]float64{v0, 0, 0} {
		t.Fatalf("muzzle velocity %v, want %v along x", b.Vel, v0)
	}
	for n := 1; n <= 30; n++ {
		b.Tick(fakeBlocks{}, &BulletCollider{Boxes: noBoxes})
		k := math.Pow(1-f, float64(n))
		x := v0 * (1 - k) / f
		// vy_i = −g·(1−(1−f)^i)/f, y_n = Σ_{i<n} vy_i
		y := 0.0
		for i := range n {
			y -= gr * (1 - math.Pow(1-f, float64(i))) / f
		}
		if math.Abs(b.Pos[0]-x) > 1e-9 || math.Abs(b.Pos[1]-100-y) > 1e-9 {
			t.Fatalf("tick %d: at %v, want x %v y %v", n, b.Pos, x, 100+y)
		}
	}
	// life 10 s: gone when tickCount reaches life−1
	for !b.Gone {
		b.Tick(fakeBlocks{}, nil)
	}
	if b.Age != 199 {
		t.Fatalf("gone at tick %d, want 199", b.Age)
	}
}

// TestBulletPierce: pierce 1 hits the nearest target only; pierce 2 the
// two nearest, nearest first; a block stops the bullet and hides what is behind.
func TestBulletPierce(t *testing.T) {
	g := testGun()
	g.Bullet.Gravity, g.Bullet.Speed = 0, 20*10 // 10 blocks a tick
	targets := []BulletTarget{
		{Box: mk(4, 0, 0, 4.6, 1.8, 1), EyeHeight: 1.62},
		{Box: mk(2, 0, 0, 2.6, 1.8, 1), EyeHeight: 1.62},
		{Box: mk(3, 0, 0, 3.6, 1.8, 1), EyeHeight: 1.62},
		{Box: mk(7, 0, 0, 7.6, 1.8, 1), EyeHeight: 1.62}, // behind the wall at x=5
	}
	run := func(pierce int) []int {
		g.Bullet.Pierce = pierce
		b := NewBullet([3]float64{0.5, 0.5, 0.5}, [3]float64{1, 0, 0}, g, 0, false, rand.New(rand.NewSource(1)))
		var got []int
		b.Tick(fakeBlocks{}, &BulletCollider{Boxes: wallBoxes, Targets: targets, Hit: func(_ *Bullet, h BulletHit) { got = append(got, h.Target) }})
		if !b.Gone {
			t.Fatalf("pierce %d: bullet flies on past the wall", pierce)
		}
		return got
	}
	if got := run(1); len(got) != 1 || got[0] != 1 {
		t.Fatalf("pierce 1 hit %v, want [1]", got)
	}
	if got := run(2); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("pierce 2 hit %v, want [1 2]", got)
	}
	if got := run(5); len(got) != 3 || got[2] != 0 {
		t.Fatalf("pierce 5 hit %v, want [1 2 0] (the wall stops it)", got)
	}
}

// TestBulletHead: the head band is the eye height ± 0.25 above the feet.
func TestBulletHead(t *testing.T) {
	g := testGun()
	g.Bullet.Gravity = 0
	for _, c := range []struct {
		y    float64
		head bool
	}{{1.5, true}, {1.3, false}, {1.86, true}, {0.5, false}} {
		b := NewBullet([3]float64{0, c.y, 0.5}, [3]float64{1, 0, 0}, g, 0, false, rand.New(rand.NewSource(1)))
		var h BulletHit
		b.Tick(fakeBlocks{}, &BulletCollider{Boxes: noBoxes, Targets: []BulletTarget{{Box: mk(3, 0, 0, 3.6, 1.9, 1), EyeHeight: 1.62}},
			Hit: func(_ *Bullet, hit BulletHit) { h = hit }})
		if h.Head != c.head || h.Pos[0] != 3 {
			t.Fatalf("hit at y %v: %+v, want head %v", c.y, h, c.head)
		}
	}
}

// TestBulletDamage: falloff by the distance from the start, 1/pellets per
// pellet, head shot multiplier, 0 past the last pair.
func TestBulletDamage(t *testing.T) {
	g := testGun()
	b := NewBullet([3]float64{}, [3]float64{1, 0, 0}, g, 0, false, rand.New(rand.NewSource(1)))
	for _, c := range []struct {
		x    float64
		head bool
		want float32
	}{{10, false, 10}, {10, true, 20}, {30, false, 6}, {50, false, 0}} {
		if d := b.Damage([3]float64{c.x, 0, 0}, c.head); d != c.want {
			t.Fatalf("at %v head %v: %v, want %v", c.x, c.head, d, c.want)
		}
	}
	g.Bullet.Pellets = 8
	bs := Fire([3]float64{}, [3]float64{0, 0, -1}, g, 2.5, &Shooter{}, rand.New(rand.NewSource(1)))
	if len(bs) != 8 || bs[0].Damage([3]float64{10, 0, 0}, false) != 10.0/8 {
		t.Fatalf("shotgun: %d pellets, %v each", len(bs), bs[0].Damage([3]float64{10, 0, 0}, false))
	}
}

// TestBulletSpread: each pellet's direction is the unit look plus a
// triangle sample in ±0.0172275·inaccuracy per axis, so pellets differ
// and stay within that box; zero inaccuracy flies straight.
func TestBulletSpread(t *testing.T) {
	g := testGun()
	g.Bullet.Pellets = 12
	const inacc = 3
	bs := Fire([3]float64{}, [3]float64{0, 0, -1}, g, inacc, &Shooter{}, rand.New(rand.NewSource(7)))
	speed := float64(g.Bullet.Speed) / 20
	seen := map[[3]float64]bool{}
	for _, b := range bs {
		n := [3]float64{b.Vel[0] / speed, b.Vel[1] / speed, b.Vel[2]/speed + 1}
		for _, c := range n {
			if math.Abs(c) > inaccuracyScale*inacc {
				t.Fatalf("pellet off by %v, max %v", n, inaccuracyScale*inacc)
			}
		}
		seen[b.Vel] = true
	}
	if len(seen) != len(bs) {
		t.Fatalf("pellets share directions: %d of %d distinct", len(seen), len(bs))
	}
}

// TestTracerCount: every (interval+1)th bullet is a tracer, pellets counted.
func TestTracerCount(t *testing.T) {
	g := testGun()
	g.Bullet.TracerInterval = 2
	s := &Shooter{}
	var got []bool
	for range 6 {
		got = append(got, Fire([3]float64{}, [3]float64{0, 0, -1}, g, 0, s, rand.New(rand.NewSource(1)))[0].Tracer)
	}
	if want := []bool{false, false, true, false, false, true}; !slices.Equal(got, want) {
		t.Fatalf("tracers %v, want %v", got, want)
	}
}

// TestHitBox: HitboxHelper's box for a moving player and a mob.
func TestHitBox(t *testing.T) {
	box := mk(0, 0, 0, 0.6, 1.8, 0.6)
	v := [3]float64{0.2, 0, 0}
	if got, want := HitBox(box, v, true, false), mk(-0.4, 0, 0, 0.6+0.2-0.4, 1.8625, 0.6); !near(got, want) {
		t.Fatalf("player: %+v, want %+v", got, want)
	}
	if got, want := HitBox(box, v, false, false), mk(-1, 0, 0, -0.2, 1.8, 0.6); !near(got, want) {
		t.Fatalf("mob: %+v, want %+v", got, want)
	}
}

func near(a, b collide.AABB) bool {
	return math.Abs(a.MinX-b.MinX) < 1e-12 && math.Abs(a.MaxX-b.MaxX) < 1e-12 && math.Abs(a.MinY-b.MinY) < 1e-12 &&
		math.Abs(a.MaxY-b.MaxY) < 1e-12 && math.Abs(a.MinZ-b.MinZ) < 1e-12 && math.Abs(a.MaxZ-b.MaxZ) < 1e-12
}
