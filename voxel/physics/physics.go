// Package physics provides block raycasting against a voxel.World (block
// picking). Box movement and collision live in voxel/collide, which has no
// dependency on the world type.
package physics

import (
	"math"

	"github.com/memmaker/voxellib/voxel"
)

// Hit describes a raycast result.
type Hit struct {
	Block voxel.BlockPos
	Face  voxel.Face // face of Block that was hit
	Dist  float64
	State voxel.StateID
}

// Raycast walks the voxel grid (Amanatides & Woo DDA) from origin along dir
// (need not be normalised) up to maxDist, returning the first block for
// which pick returns true. A nil pick selects any non-air, non-liquid block.
func Raycast(w *voxel.World, ox, oy, oz, dx, dy, dz, maxDist float64, pick func(*voxel.StateInfo) bool) (Hit, bool) {
	l := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if l == 0 {
		return Hit{}, false
	}
	dx, dy, dz = dx/l, dy/l, dz/l
	if pick == nil {
		pick = func(s *voxel.StateInfo) bool { return s.Shape != voxel.ShapeNone && s.Shape != voxel.ShapeLiquid }
	}
	x, y, z := int(math.Floor(ox)), int(math.Floor(oy)), int(math.Floor(oz))
	step := func(d float64) int {
		if d > 0 {
			return 1
		}
		if d < 0 {
			return -1
		}
		return 0
	}
	sx, sy, sz := step(dx), step(dy), step(dz)
	tDelta := func(d float64) float64 {
		if d == 0 {
			return math.Inf(1)
		}
		return math.Abs(1 / d)
	}
	tMax := func(o float64, c int, d float64) float64 {
		if d > 0 {
			return (float64(c+1) - o) / d
		}
		if d < 0 {
			return (o - float64(c)) / -d
		}
		return math.Inf(1)
	}
	tdx, tdy, tdz := tDelta(dx), tDelta(dy), tDelta(dz)
	tmx, tmy, tmz := tMax(ox, x, dx), tMax(oy, y, dy), tMax(oz, z, dz)
	face := voxel.FaceUp
	t := 0.0
	table := w.Reg.Table()
	w.RLock()
	defer w.RUnlock()
	for t <= maxDist {
		id := w.BlockUnlocked(x, y, z)
		if int(id) < len(table) && pick(&table[id]) {
			return Hit{voxel.BlockPos{X: x, Y: y, Z: z}, face, t, id}, true
		}
		switch {
		case tmx < tmy && tmx < tmz:
			x += sx
			t = tmx
			tmx += tdx
			face = voxel.FaceWest
			if sx < 0 {
				face = voxel.FaceEast
			}
		case tmy < tmz:
			y += sy
			t = tmy
			tmy += tdy
			face = voxel.FaceDown
			if sy < 0 {
				face = voxel.FaceUp
			}
		default:
			z += sz
			t = tmz
			tmz += tdz
			face = voxel.FaceNorth
			if sz < 0 {
				face = voxel.FaceSouth
			}
		}
	}
	return Hit{}, false
}
