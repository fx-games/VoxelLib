package projectile

import (
	"math"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/physics"
)

// HitResult is what a Hitscan ray met first.
type HitResult struct {
	X, Y, Z  float64 // hit point
	Dist     float64 // from the origin, in blocks
	Normal   [3]float64
	Block    bool // a block was hit (Face, BlockPos, Box valid), otherwise Target
	BlockPos voxel.BlockPos
	Face     voxel.Face
	Target   Target // the target hit, nil for a block
	Part     int    // the Shape's part (e.g. bone) hit, -1 for a plain box target
}

// Hitscan casts an instant ray from o along d (any length) and returns the
// nearest of: a block (shaped blocks via boxes, see physics.RaycastBox) or
// one of targets, within maxDist blocks. A Shape is tested at its exact
// surface, any other target by its box; a box the ray starts inside is passed
// through (leave the shooter out of targets anyway). Normal is the surface
// normal for blocks and box targets, and zero for a Shape, which only reports
// a point and a part.
func Hitscan(w physics.Blocks, boxes physics.BoxFunc, o, d [3]float64, maxDist float64, targets []Target) (HitResult, bool) {
	l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
	if l == 0 {
		return HitResult{}, false
	}
	u := [3]float64{d[0] / l, d[1] / l, d[2] / l}
	at := func(t float64) (x, y, z float64) { return o[0] + u[0]*t, o[1] + u[1]*t, o[2] + u[2]*t }

	var best HitResult
	found := false
	if h, ok := physics.RaycastBox(w, o[0], o[1], o[2], u[0], u[1], u[2], maxDist, boxes); ok {
		n := h.Face.Normal()
		best = HitResult{Dist: h.Dist, Normal: [3]float64{float64(n.X), float64(n.Y), float64(n.Z)},
			Block: true, BlockPos: h.Block, Face: h.Face, Part: -1}
		best.X, best.Y, best.Z = at(h.Dist)
		found = true
	}
	limit := maxDist
	if found {
		limit = best.Dist
	}
	for _, t := range targets {
		var r HitResult
		if sh, ok := t.(Shape); ok {
			x1, y1, z1 := at(limit)
			hx, hy, hz, part, hit := sh.Intersect(o[0], o[1], o[2], x1, y1, z1)
			if !hit {
				continue
			}
			r = HitResult{X: hx, Y: hy, Z: hz, Part: part,
				Dist: math.Sqrt((hx-o[0])*(hx-o[0]) + (hy-o[1])*(hy-o[1]) + (hz-o[2])*(hz-o[2]))}
		} else {
			dist, n, hit := collide.RayAABB(o, u, t.Box())
			if !hit || dist == 0 && n == ([3]float64{}) {
				continue // missed, or started inside
			}
			r = HitResult{Dist: dist, Normal: n, Part: -1}
			r.X, r.Y, r.Z = at(dist)
		}
		if r.Dist > limit || (found && r.Dist >= best.Dist) {
			continue
		}
		r.Target = t
		best, found, limit = r, true, r.Dist
	}
	return best, found
}
