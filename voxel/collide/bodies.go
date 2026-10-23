package collide

import "math"

// Body is something solid that is not a block: a prop, a door, a mob, a
// dropped crate. Box is its collision box in world space; OBB, when set, is
// its exact turned shape and Box should enclose it (the axis-clipping Move and
// the character controller only see Box). Bounds is the extent the index
// lists it under, for picking boxes bigger than the collision box; zero means
// Box. Solid false keeps it listed for queries (picking, triggers) while
// movers pass through it: an open door, an item lying on the floor.
type Body struct {
	Box    AABB
	OBB    *OBB
	Bounds AABB
	Solid  bool
	Data   any // the game's own object
}

func (b *Body) bounds() AABB {
	if b.Bounds == (AABB{}) {
		return b.Box
	}
	return b.Bounds
}

type cell struct{ x, y, z int32 }

// Bodies is a spatial index of bodies on a grid of unit cells. Ids are
// stable until Remove and are reused afterwards. Not safe for concurrent use.
type Bodies struct {
	list  []Body
	live  []bool
	at    [][2]cell // cell range each body is listed in
	free  []int32
	cells map[cell][]int32
	seen  []uint32
	gen   uint32
}

// NewBodies returns an empty index.
func NewBodies() *Bodies { return &Bodies{cells: map[cell][]int32{}} }

func cellRange(a AABB) [2]cell {
	f := func(v float64) int32 { return int32(math.Floor(v)) }
	return [2]cell{{f(a.MinX), f(a.MinY), f(a.MinZ)}, {f(a.MaxX), f(a.MaxY), f(a.MaxZ)}}
}

func (s *Bodies) each(r [2]cell, f func(c cell)) {
	for x := r[0].x; x <= r[1].x; x++ {
		for y := r[0].y; y <= r[1].y; y++ {
			for z := r[0].z; z <= r[1].z; z++ {
				f(cell{x, y, z})
			}
		}
	}
}

func (s *Bodies) index(id int32) {
	s.at[id] = cellRange(s.list[id].bounds())
	s.each(s.at[id], func(c cell) { s.cells[c] = append(s.cells[c], id) })
}

func (s *Bodies) unindex(id int32) {
	s.each(s.at[id], func(c cell) {
		ids := s.cells[c]
		for i, v := range ids {
			if v == id {
				ids[i] = ids[len(ids)-1]
				ids = ids[:len(ids)-1]
				break
			}
		}
		if len(ids) == 0 {
			delete(s.cells, c) // or a falling body leaves a trail of empty cells
		} else {
			s.cells[c] = ids
		}
	})
}

// Add lists a body and returns its id.
func (s *Bodies) Add(b Body) int32 {
	var id int32
	if n := len(s.free); n > 0 {
		id, s.free = s.free[n-1], s.free[:n-1]
		s.list[id], s.live[id] = b, true
	} else {
		id = int32(len(s.list))
		s.list, s.live, s.at, s.seen = append(s.list, b), append(s.live, true), append(s.at, [2]cell{}), append(s.seen, 0)
	}
	s.index(id)
	return id
}

// Set replaces a body, e.g. after it moved or a door opened.
func (s *Bodies) Set(id int32, b Body) {
	s.list[id] = b
	if cellRange(b.bounds()) != s.at[id] {
		s.unindex(id)
		s.index(id)
	}
}

// Remove takes a body out of the index.
func (s *Bodies) Remove(id int32) {
	if id < 0 || int(id) >= len(s.live) || !s.live[id] {
		return
	}
	s.unindex(id)
	s.live[id] = false
	s.list[id] = Body{}
	s.free = append(s.free, id)
}

// Get returns a listed body.
func (s *Bodies) Get(id int32) *Body { return &s.list[id] }

// Cells is how many grid cells hold at least one body.
func (s *Bodies) Cells() int { return len(s.cells) }

// Query calls visit once for every body listed in a cell area reaches into
// (by Bounds; a broad phase, so test the shape yourself).
func (s *Bodies) Query(area AABB, visit func(id int32, b *Body)) {
	s.gen++
	s.each(cellRange(area), func(c cell) {
		for _, id := range s.cells[c] {
			if s.seen[id] != s.gen {
				s.seen[id] = s.gen
				visit(id, &s.list[id])
			}
		}
	})
}

// Boxes appends the boxes of solid bodies overlapping area (collide.Boxer),
// so a World that forwards to it makes Move and the character controller
// collide with bodies too.
func (s *Bodies) Boxes(area AABB, out []AABB) []AABB { return s.BoxesExcept(-1, area, out) }

// BoxesExcept is Boxes leaving out one body, the mover's own.
func (s *Bodies) BoxesExcept(self int32, area AABB, out []AABB) []AABB {
	s.Query(area, func(id int32, b *Body) {
		if id != self && b.Solid && b.Box.Intersects(area) {
			out = append(out, b.Box)
		}
	})
	return out
}

// Shapes is a Mover's shape source over blocks and bodies: the block boxes of
// w (with its Boxer boxes) and every solid body but self that skip (may be
// nil) does not rule out.
func (s *Bodies) Shapes(w Solids, self int32, skip func(id int32, b *Body) bool) func(area AABB, out []Shape) []Shape {
	var boxes []AABB
	return func(area AABB, out []Shape) []Shape {
		if w != nil {
			boxes = Colliders(w, area, boxes[:0])
			for _, b := range boxes {
				out = append(out, Shape{Box: b, Body: -1})
			}
		}
		s.Query(area, func(id int32, b *Body) {
			if id != self && b.Solid && (skip == nil || !skip(id, b)) {
				out = append(out, Shape{Box: b.Box, OBB: b.OBB, Body: id})
			}
		})
		return out
	}
}
