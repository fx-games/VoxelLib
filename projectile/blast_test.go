package projectile

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/weapon"
)

// rpg7's blast (damage 120, radius 3) on a 0.6×1.8 box: strength is the
// nearest clear sample's distance over the radius; a wall shields it.
func TestBlast(t *testing.T) {
	e := &weapon.Explosion{Damage: 120, Radius: 3, Knockback: true}
	box := collide.AABB{MinX: 1.7, MinY: 0, MinZ: -0.3, MaxX: 2.3, MaxY: 1.8, MaxZ: 0.3}
	open := func(a, b [3]float64) bool { return true }
	dmg, push, ok := Blast(e, [3]float64{0, 0.9, 0}, box, open)
	// nearest point (1.7, 0.9, 0): strength 1.7/3
	if want := float32((1 - 1.7/3) * 120); !ok || math.Abs(float64(dmg-want)) > 1e-3 {
		t.Fatalf("damage %v ok %v, want %v", dmg, ok, want)
	}
	if want := (1 - 1.7/3) * 120 * 6 / 500.0; math.Abs(push[0]-want) > 1e-9 || push[1] != 0 {
		t.Fatalf("push %v, want x %v", push, want)
	}
	if _, _, ok := Blast(e, [3]float64{0, 0.9, 0}, box, func(a, b [3]float64) bool { return false }); ok {
		t.Fatal("blast through a wall")
	}
	if _, _, ok := Blast(e, [3]float64{-2, 0.9, 0}, box, open); ok {
		t.Fatal("blast out of reach")
	}
}

// An explosive round blows up at its fuse (max(delay·20, 1) ticks) and at
// the first target, which it does not pierce.
func TestExplosiveBullet(t *testing.T) {
	g := &weapon.Gun{}
	g.Bullet.Explosion = &weapon.Explosion{Delay: 0.1}
	g.Bullet.Pierce = 5
	b := &Bullet{Gun: g, Vel: [3]float64{1, 0, 0}, Pierce: 5, Life: 100, fuse: 2}
	var at [][3]float64
	c := &BulletCollider{Boxes: noBoxes, Explode: func(_ *Bullet, p [3]float64) { at = append(at, p) }}
	w := fakeBlocks{}
	b.Tick(w, c)
	if b.Tick(w, c); !b.Gone || len(at) != 1 || at[0] != [3]float64{1, 0, 0} {
		t.Fatalf("fuse: gone %v at %v", b.Gone, at)
	}
	b = &Bullet{Gun: g, Vel: [3]float64{1, 0, 0}, Pierce: 5, Life: 100, fuse: 100}
	c.Targets = []BulletTarget{{Box: collide.AABB{MinX: 0.5, MinY: -1, MinZ: -1, MaxX: 0.6, MaxY: 1, MaxZ: 1}}, {Box: collide.AABB{MinX: 0.8, MinY: -1, MinZ: -1, MaxX: 0.9, MaxY: 1, MaxZ: 1}}}
	hits := 0
	c.Hit = func(*Bullet, BulletHit) { hits++ }
	at = nil
	if b.Tick(w, c); hits != 1 || len(at) != 1 || at[0][0] != 0.5 {
		t.Fatalf("hits %d at %v", hits, at)
	}
}
