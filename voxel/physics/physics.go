// Package physics provides block raycasting against a voxel.World (block
// picking). Box movement and collision live in voxel/collide, which has no
// dependency on the world type.
package physics

import (
	"math"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
)

// Hit describes a raycast result.
type Hit struct {
	Block voxel.BlockPos
	Face  voxel.Face // face of Block that was hit
	Dist  float64
	State voxel.StateID
	// Box is the world box the ray hit (RaycastBox); zero for Raycast.
	Box collide.AABB
}

// Blocks is the block source RaycastBox reads; *voxel.World is one.
type Blocks interface {
	Block(x, y, z int) voxel.StateID
}

// BoxFunc appends to buf a block's ray boxes in world coordinates; none
// when the ray passes through the block (Minecraft: canCollideCheck, then
// the bounds collisionRayTrace tests, several for stairs).
type BoxFunc func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB

// RaycastBox is Raycast against each block's own boxes
// (World.rayTraceBlocks): the grid walk visits the cells in order and a
// cell is hit where the ray first enters one of its boxes. Hit.Dist is
// the distance to that point, Hit.Face the box face entered and Hit.Box
// the box.
func RaycastBox(w Blocks, ox, oy, oz, dx, dy, dz, maxDist float64, boxes BoxFunc) (Hit, bool) {
	l := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if l == 0 {
		return Hit{}, false
	}
	ux, uy, uz := dx/l, dy/l, dz/l
	var out Hit
	var buf []collide.AABB
	// No world lock around the walk: boxes may read neighbours.
	ok := Walk(w.Block, ox, oy, oz, ux, uy, uz, maxDist, func(x, y, z int, id voxel.StateID, _ voxel.Face, _ float64) bool {
		buf = boxes(x, y, z, id, buf[:0])
		hit := false
		for _, b := range buf {
			t, f, in := rayBox(ox, oy, oz, ux, uy, uz, b)
			if in && t <= maxDist && (!hit || t < out.Dist) {
				out, hit = Hit{voxel.BlockPos{X: x, Y: y, Z: z}, f, t, id, b}, true
			}
		}
		return hit
	})
	return out, ok
}

// rayBox is the slab test of the unit ray against b: the entry distance and
// face (the exit if the origin is inside, as collisionRayTrace).
func rayBox(ox, oy, oz, dx, dy, dz float64, b collide.AABB) (float64, voxel.Face, bool) {
	tmin, tmax := math.Inf(-1), math.Inf(1)
	var fmin, fmax voxel.Face
	for i, s := range [3][4]float64{{ox, dx, b.MinX, b.MaxX}, {oy, dy, b.MinY, b.MaxY}, {oz, dz, b.MinZ, b.MaxZ}} {
		if s[1] == 0 {
			if s[0] < s[2] || s[0] > s[3] {
				return 0, 0, false
			}
			continue
		}
		t0, t1 := (s[2]-s[0])/s[1], (s[3]-s[0])/s[1]
		lo, hi := faces[i][0], faces[i][1] // faces of the min and max planes
		if t0 > t1 {
			t0, t1, lo, hi = t1, t0, hi, lo
		}
		if t0 > tmin {
			tmin, fmin = t0, lo
		}
		if t1 < tmax {
			tmax, fmax = t1, hi
		}
	}
	if tmin > tmax || tmax < 0 {
		return 0, 0, false
	}
	if tmin < 0 {
		return tmax, fmax, true
	}
	return tmin, fmin, true
}

// faces are the faces of a box's min and max plane per axis.
var faces = [3][2]voxel.Face{{voxel.FaceWest, voxel.FaceEast}, {voxel.FaceDown, voxel.FaceUp}, {voxel.FaceNorth, voxel.FaceSouth}}

// Raycast walks the voxel grid (Amanatides & Woo DDA) from origin along dir
// (need not be normalised) up to maxDist, returning the first block for
// which pick returns true. A nil pick selects any non-air, non-liquid block.
func Raycast(w *voxel.World, ox, oy, oz, dx, dy, dz, maxDist float64, pick func(*voxel.StateInfo) bool) (Hit, bool) {
	l := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if l == 0 {
		return Hit{}, false
	}
	if pick == nil {
		pick = func(s *voxel.StateInfo) bool { return s.Shape != voxel.ShapeNone && s.Shape != voxel.ShapeLiquid }
	}
	table := w.Reg.Table()
	var out Hit
	w.RLock()
	defer w.RUnlock()
	ok := Walk(w.BlockUnlocked, ox, oy, oz, dx/l, dy/l, dz/l, maxDist, func(x, y, z int, id voxel.StateID, f voxel.Face, t float64) bool {
		if int(id) < len(table) && pick(&table[id]) {
			out = Hit{Block: voxel.BlockPos{X: x, Y: y, Z: z}, Face: f, Dist: t, State: id}
			return true
		}
		return false
	})
	return out, ok
}

// Walk visits the grid cells along the unit ray (Amanatides & Woo DDA) up
// to maxDist, with the face entered and the distance to it, until visit
// returns true. block may be any grid's lookup, or nil when visit does not
// need the state (id is then 0).
func Walk(block func(x, y, z int) voxel.StateID, ox, oy, oz, dx, dy, dz, maxDist float64, visit func(x, y, z int, id voxel.StateID, f voxel.Face, t float64) bool) bool {
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
	for t <= maxDist {
		var id voxel.StateID
		if block != nil {
			id = block(x, y, z)
		}
		if visit(x, y, z, id, face, t) {
			return true
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
	return false
}
