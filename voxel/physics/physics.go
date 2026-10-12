// Package physics provides block raycasting and AABB-vs-world collision in
// the style of Minecraft's Entity.move: the movement vector is clipped
// against every block box in the swept area, Y first, then X and Z.
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

// AABB is an axis-aligned box in world coordinates.
type AABB struct{ MinX, MinY, MinZ, MaxX, MaxY, MaxZ float64 }

func (a AABB) Offset(x, y, z float64) AABB {
	return AABB{a.MinX + x, a.MinY + y, a.MinZ + z, a.MaxX + x, a.MaxY + y, a.MaxZ + z}
}

// Intersects reports overlap (touching doesn't count).
func (a AABB) Intersects(b AABB) bool {
	return a.MinX < b.MaxX && a.MaxX > b.MinX && a.MinY < b.MaxY && a.MaxY > b.MinY && a.MinZ < b.MaxZ && a.MaxZ > b.MinZ
}

// Expand grows the box toward a movement vector.
func (a AABB) Expand(x, y, z float64) AABB {
	if x < 0 {
		a.MinX += x
	} else {
		a.MaxX += x
	}
	if y < 0 {
		a.MinY += y
	} else {
		a.MaxY += y
	}
	if z < 0 {
		a.MinZ += z
	} else {
		a.MaxZ += z
	}
	return a
}

func clipY(b, a AABB, d float64) float64 {
	if a.MaxX <= b.MinX || a.MinX >= b.MaxX || a.MaxZ <= b.MinZ || a.MinZ >= b.MaxZ {
		return d
	}
	if d > 0 && a.MaxY <= b.MinY {
		d = math.Min(d, b.MinY-a.MaxY)
	} else if d < 0 && a.MinY >= b.MaxY {
		d = math.Max(d, b.MaxY-a.MinY)
	}
	return d
}

func clipX(b, a AABB, d float64) float64 {
	if a.MaxY <= b.MinY || a.MinY >= b.MaxY || a.MaxZ <= b.MinZ || a.MinZ >= b.MaxZ {
		return d
	}
	if d > 0 && a.MaxX <= b.MinX {
		d = math.Min(d, b.MinX-a.MaxX)
	} else if d < 0 && a.MinX >= b.MaxX {
		d = math.Max(d, b.MaxX-a.MinX)
	}
	return d
}

func clipZ(b, a AABB, d float64) float64 {
	if a.MaxX <= b.MinX || a.MinX >= b.MaxX || a.MaxY <= b.MinY || a.MinY >= b.MaxY {
		return d
	}
	if d > 0 && a.MaxZ <= b.MinZ {
		d = math.Min(d, b.MinZ-a.MaxZ)
	} else if d < 0 && a.MinZ >= b.MaxZ {
		d = math.Max(d, b.MaxZ-a.MinZ)
	}
	return d
}

// Colliders returns the block boxes overlapping area. Unloaded chunks are
// solid so nothing falls out of the world while it streams in.
func Colliders(w *voxel.World, area AABB, out []AABB) []AABB {
	table := w.Reg.Table()
	w.RLock()
	defer w.RUnlock()
	for y := int(math.Floor(area.MinY)); y < int(math.Ceil(area.MaxY)); y++ {
		for z := int(math.Floor(area.MinZ)); z < int(math.Ceil(area.MaxZ)); z++ {
			for x := int(math.Floor(area.MinX)); x < int(math.Ceil(area.MaxX)); x++ {
				solid := false
				if w.ChunkUnlocked(voxel.ChunkPos{X: x >> 4, Z: z >> 4}) == nil {
					solid = y >= w.MinY
				} else {
					solid = table[w.BlockUnlocked(x, y, z)].Collides
				}
				if solid {
					fx, fy, fz := float64(x), float64(y), float64(z)
					out = append(out, AABB{fx, fy, fz, fx + 1, fy + 1, fz + 1})
				}
			}
		}
	}
	return out
}

// Move moves box by (dx,dy,dz) against the world and returns the allowed
// movement. Compare with the request to know which axes collided.
func Move(w *voxel.World, box AABB, dx, dy, dz float64) (float64, float64, float64) {
	boxes := Colliders(w, box.Expand(dx, dy, dz), nil)
	for _, b := range boxes {
		dy = clipY(b, box, dy)
	}
	box = box.Offset(0, dy, 0)
	for _, b := range boxes {
		dx = clipX(b, box, dx)
	}
	box = box.Offset(dx, 0, 0)
	for _, b := range boxes {
		dz = clipZ(b, box, dz)
	}
	return dx, dy, dz
}
