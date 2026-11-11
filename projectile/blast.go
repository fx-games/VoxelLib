package projectile

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/weapon"
)

// explosionKnockDivisor is ProjectileExplosion's knockback multiplier
// power·radius/explosionKnockDivisor.
const explosionKnockDivisor = 500

// Blast is ProjectileExplosion.explode on a living entity with box at
// (the fixed hit box): the nearest of the box's 15 sample points with no
// block in between (clear) sets the strength; damage is (1−strength)·power,
// push the velocity it adds (zero without knockback). ok is false out of
// reach.
func Blast(e *weapon.Explosion, at [3]float64, box collide.AABB, clear func(a, b [3]float64) bool) (damage float32, push [3]float64, ok bool) {
	radius := float64(e.Radius) * 2 // explode doubles it
	c := [3]float64{(box.MinX + box.MaxX) / 2, (box.MinY + box.MaxY) / 2, (box.MinZ + box.MaxZ) / 2}
	pts := [15][3]float64{
		{box.MinX, box.MinY, box.MinZ}, {box.MinX, box.MinY, box.MaxZ}, {box.MinX, box.MaxY, box.MinZ},
		{box.MaxX, box.MinY, box.MinZ}, {box.MinX, box.MaxY, box.MaxZ}, {box.MaxX, box.MinY, box.MaxZ},
		{box.MaxX, box.MaxY, box.MinZ}, {box.MaxX, box.MaxY, box.MaxZ},
		{box.MinX, c[1], c[2]}, {box.MaxX, c[1], c[2]}, {c[0], box.MinY, c[2]},
		{c[0], box.MaxY, c[2]}, {c[0], c[1], box.MinZ}, {c[0], c[1], box.MaxZ}, c,
	}
	minDist := radius
	for _, p := range pts {
		if clear(at, p) {
			minDist = min(minDist, math.Sqrt(sq(p[0]-at[0])+sq(p[1]-at[1])+sq(p[2]-at[2])))
		}
	}
	strength := minDist * 2 / radius
	if strength > 1 {
		return 0, push, false
	}
	d := [3]float64{c[0] - at[0], c[1] - at[1], c[2] - at[2]}
	if l := math.Sqrt(sq(d[0]) + sq(d[1]) + sq(d[2])); l != 0 {
		d = [3]float64{d[0] / l, d[1] / l, d[2] / l}
	}
	f := 1 - strength
	if e.Knockback {
		m := f * float64(e.Damage) * radius / explosionKnockDivisor
		push = [3]float64{d[0] * m, d[1] * m, d[2] * m}
	}
	return float32(f) * e.Damage, push, true
}
