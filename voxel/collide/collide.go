// Package collide moves axis-aligned boxes through a grid of unit blocks the
// way the reference's Entity.move does: the movement is clipped against every
// block box in the swept area, Y first, then X, then Z.
//
// It knows nothing about worlds, chunks or block types: callers describe the
// grid with the one-method Solids interface. The character controller and
// dropped items both build on it. Bodies indexes things that are not blocks
// (props, doors, mobs) and Mover sweeps boxes through blocks and bodies with
// sliding and step-up, for games that resolve collision by time of impact.
package collide

import (
	"math"
	"sync"
)

// Solids reports whether the unit block at integer coordinates blocks
// movement. Implementations usually treat unloaded terrain as solid so
// nothing falls out of the world while it streams in.
type Solids interface {
	Solid(x, y, z int) bool
}

// Boxer is optionally implemented by a Solids that also has solid boxes
// smaller than a block (props, furniture): Boxes appends those
// overlapping area.
type Boxer interface {
	Boxes(area AABB, out []AABB) []AABB
}

// SolidsFunc adapts a function to Solids.
type SolidsFunc func(x, y, z int) bool

func (f SolidsFunc) Solid(x, y, z int) bool { return f(x, y, z) }

// AABB is an axis-aligned box in world coordinates.
type AABB struct{ MinX, MinY, MinZ, MaxX, MaxY, MaxZ float64 }

// Box returns the box of the given width and height standing at x,y,z
// (feet centre), the usual shape of entities.
func Box(x, y, z, width, height float64) AABB {
	h := width / 2
	return AABB{x - h, y, z - h, x + h, y + height, z + h}
}

// Block returns the unit box of a block.
func Block(x, y, z int) AABB {
	fx, fy, fz := float64(x), float64(y), float64(z)
	return AABB{fx, fy, fz, fx + 1, fy + 1, fz + 1}
}

func (a AABB) Offset(x, y, z float64) AABB {
	return AABB{a.MinX + x, a.MinY + y, a.MinZ + z, a.MaxX + x, a.MaxY + y, a.MaxZ + z}
}

// Grow enlarges the box by the given amount on every side of each axis.
func (a AABB) Grow(x, y, z float64) AABB {
	return AABB{a.MinX - x, a.MinY - y, a.MinZ - z, a.MaxX + x, a.MaxY + y, a.MaxZ + z}
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

// Blocks calls f for every block position the box overlaps.
func (a AABB) Blocks(f func(x, y, z int)) {
	for y := int(math.Floor(a.MinY)); y < int(math.Ceil(a.MaxY)); y++ {
		for z := int(math.Floor(a.MinZ)); z < int(math.Ceil(a.MaxZ)); z++ {
			for x := int(math.Floor(a.MinX)); x < int(math.Ceil(a.MaxX)); x++ {
				f(x, y, z)
			}
		}
	}
}

// Colliders appends the boxes of solid blocks overlapping area, and the
// Boxer's boxes if s is one.
func Colliders(s Solids, area AABB, out []AABB) []AABB {
	area.Blocks(func(x, y, z int) {
		if s.Solid(x, y, z) {
			out = append(out, Block(x, y, z))
		}
	})
	if b, ok := s.(Boxer); ok {
		out = b.Boxes(area, out)
	}
	return out
}

// Fits reports whether the box overlaps nothing solid.
func Fits(s Solids, box AABB) bool {
	for _, b := range Colliders(s, box, nil) {
		if b.Intersects(box) {
			return false
		}
	}
	return true
}

// eps is how far a box may sink into another and still count as touching
// it, not overlapping: a body's box rebuilt from its centre (Box) lands a
// rounding error inside the face it was stopped against.
const eps = 1e-7

func clipY(b, a AABB, d float64) float64 {
	if a.MaxX <= b.MinX+eps || a.MinX >= b.MaxX-eps || a.MaxZ <= b.MinZ+eps || a.MinZ >= b.MaxZ-eps {
		return d
	}
	if d > 0 && a.MaxY <= b.MinY+eps {
		d = math.Min(d, math.Max(0, b.MinY-a.MaxY))
	} else if d < 0 && a.MinY >= b.MaxY-eps {
		d = math.Max(d, math.Min(0, b.MaxY-a.MinY))
	}
	return d
}

func clipX(b, a AABB, d float64) float64 {
	if a.MaxY <= b.MinY+eps || a.MinY >= b.MaxY-eps || a.MaxZ <= b.MinZ+eps || a.MinZ >= b.MaxZ-eps {
		return d
	}
	if d > 0 && a.MaxX <= b.MinX+eps {
		d = math.Min(d, math.Max(0, b.MinX-a.MaxX))
	} else if d < 0 && a.MinX >= b.MaxX-eps {
		d = math.Max(d, math.Min(0, b.MaxX-a.MinX))
	}
	return d
}

func clipZ(b, a AABB, d float64) float64 {
	if a.MaxX <= b.MinX+eps || a.MinX >= b.MaxX-eps || a.MaxY <= b.MinY+eps || a.MinY >= b.MaxY-eps {
		return d
	}
	if d > 0 && a.MaxZ <= b.MinZ+eps {
		d = math.Min(d, math.Max(0, b.MinZ-a.MaxZ))
	} else if d < 0 && a.MinZ >= b.MaxZ-eps {
		d = math.Max(d, math.Min(0, b.MaxZ-a.MinZ))
	}
	return d
}

// ClipY limits a y movement of box by the boxes (calculateYOffset over a list).
func ClipY(boxes []AABB, box AABB, d float64) float64 {
	for _, b := range boxes {
		d = clipY(b, box, d)
	}
	return d
}

// ClipX is ClipY along x.
func ClipX(boxes []AABB, box AABB, d float64) float64 {
	for _, b := range boxes {
		d = clipX(b, box, d)
	}
	return d
}

// ClipZ is ClipY along z.
func ClipZ(boxes []AABB, box AABB, d float64) float64 {
	for _, b := range boxes {
		d = clipZ(b, box, d)
	}
	return d
}

// Move moves box by (dx,dy,dz) against the grid and returns the allowed
// movement. Compare with the request to know which axes collided.
func Move(s Solids, box AABB, dx, dy, dz float64) (float64, float64, float64) {
	bp := movePool.Get().(*[]AABB)
	boxes := Colliders(s, box.Expand(dx, dy, dz), (*bp)[:0])
	dy = ClipY(boxes, box, dy)
	box = box.Offset(0, dy, 0)
	dx = ClipX(boxes, box, dx)
	box = box.Offset(dx, 0, 0)
	dz = ClipZ(boxes, box, dz)
	*bp = boxes[:0]
	movePool.Put(bp)
	return dx, dy, dz
}

// movePool holds Move's box buffers: every body moves every tick, and
// growing a fresh slice each time was most of Move's cost.
var movePool = sync.Pool{New: func() any { return new([]AABB) }}
