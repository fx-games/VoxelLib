package projectile

import (
	"math"

	"github.com/memmaker/voxellib/voxel/physics"
)

// InSight reports whether no block stands between a and b (Minecraft's
// hasLineOfSight, eye to eye).
func InSight(w physics.Blocks, boxes physics.BoxFunc, a, b [3]float64) bool {
	d := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	_, hit := physics.RaycastBox(w, a[0], a[1], a[2], d[0], d[1], d[2], math.Sqrt(d[0]*d[0]+d[1]*d[1]+d[2]*d[2]), boxes)
	return !hit
}
