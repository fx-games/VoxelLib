package collide

import "math"

// Shape is one thing a Mover can run into: a box, or a turned box when OBB is
// set. Body is the Bodies id it belongs to, -1 for a block.
type Shape struct {
	Box  AABB
	OBB  *OBB
	Body int32
}

// Contact is one surface a Mover hit.
type Contact struct {
	T      float64    // when, as a fraction of the leg of movement being swept
	Normal [3]float64 // out of the surface, towards the mover
	Point  [3]float64 // where the mover's box was offset to at the moment of touching
	Body   int32      // -1 for a block
}

// GroundNormal is how upwards a contact normal has to point to count as
// standing on it, roughly 45 degrees.
const GroundNormal = 0.7

// Mover sweeps a box through blocks and bodies by time of impact and slides
// it along what it hits (unlike Move, which clips one axis at a time and only
// knows boxes, it meets turned shapes at their real surface). It is what a
// Quake-style game moves its player, props and actors with.
type Mover struct {
	// Shapes appends everything solid overlapping area (see Bodies.Shapes).
	Shapes func(area AABB, out []Shape) []Shape
	// OnContact, if set, hears every surface hit; returning false ends the
	// move there (a projectile that died on impact).
	OnContact func(c Contact) bool

	buf []Shape
}

// maxPasses is how often one move may be redirected by a surface before the
// rest of it is dropped: a corner can keep producing new surfaces.
const maxPasses = 8

const skin = 0.001 // how far a mover is held off what it touched

var identity = [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func add(a, b vec) vec           { return vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func scale(a vec, s float64) vec { return vec{a[0] * s, a[1] * s, a[2] * s} }

func union(a, b AABB) AABB {
	return AABB{math.Min(a.MinX, b.MinX), math.Min(a.MinY, b.MinY), math.Min(a.MinZ, b.MinZ),
		math.Max(a.MaxX, b.MaxX), math.Max(a.MaxY, b.MaxY), math.Max(a.MaxZ, b.MaxZ)}
}

// Slide moves box by move and returns the offset it got to and the surfaces
// it hit on the way. Boxes that start overlapping something are pushed out
// along the shallowest way. Keep single moves short: the shapes are gathered
// over the whole swept area.
func (m *Mover) Slide(box AABB, move [3]float64) ([3]float64, []Contact) {
	var contacts []Contact
	var pos vec
	target := move
	for pass := 0; pass < maxPasses; pass++ {
		vel := sub(target, pos)
		if vel == (vec{}) {
			break
		}
		prev := box.Offset(pos[0], pos[1], pos[2])
		m.buf = m.Shapes(union(prev, box.Offset(target[0], target[1], target[2])), m.buf[:0])
		best := OBBSweep{T: 1}
		body := int32(-1)
		for _, s := range m.buf {
			b := OBB{Local: s.Box, Axes: identity}
			if s.OBB != nil {
				b = *s.OBB
			}
			if r := SweepAABBVsOBB(prev, b, vel); r.T < best.T {
				best, body = r, s.Body
			}
		}
		if best.T >= 1 {
			pos = target
			break
		}
		touch := add(pos, scale(vel, best.T))
		pos = add(touch, scale(best.Normal, skin))
		c := Contact{T: best.T, Normal: best.Normal, Point: touch, Body: body}
		contacts = append(contacts, c)
		if m.OnContact != nil && !m.OnContact(c) {
			return pos, contacts
		}
		// The rest of the movement, less its part into the surface.
		if nn := dot(best.Normal, best.Normal); nn != 0 {
			rest := scale(vel, 1-best.T)
			target = sub(add(pos, rest), scale(best.Normal, dot(rest, best.Normal)/nn))
		} else {
			target = pos
		}
	}
	return pos, contacts
}

// Wall reports whether any contact is steep enough to be a wall rather than
// a floor or a ceiling.
func Wall(contacts []Contact) bool {
	for _, c := range contacts {
		if math.Abs(c.Normal[1]) < GroundNormal {
			return true
		}
	}
	return false
}

// Grounded reports whether any contact is a floor.
func Grounded(contacts []Contact) bool {
	for _, c := range contacts {
		if c.Normal[1] > GroundNormal {
			return true
		}
	}
	return false
}

// StepSlide is Slide that walks up ledges up to stepHeight, as Quake 3's
// PM_StepSlideMove: a move a wall stopped is tried again from stepHeight
// higher, then dropped back down. The step is taken only if it got further
// along than the blocked move, so a wall too tall to climb never nudges the
// mover upwards. The contacts returned are those of the move taken (for a
// step, its landing).
func (m *Mover) StepSlide(box AABB, move [3]float64, stepHeight float64) ([3]float64, []Contact) {
	blocked, contacts := m.Slide(box, move)
	if stepHeight <= 0 || !Wall(contacts) {
		return blocked, contacts
	}
	risen, _ := m.Slide(box, vec{0, stepHeight, 0})
	along, _ := m.Slide(box.Offset(risen[0], risen[1], risen[2]), move)
	along = add(risen, along)
	down, landing := m.Slide(box.Offset(along[0], along[1], along[2]), vec{0, -stepHeight, 0})
	landed := add(along, down)
	if math.Hypot(landed[0], landed[2]) <= math.Hypot(blocked[0], blocked[2])+skin {
		return blocked, contacts
	}
	return landed, landing
}
