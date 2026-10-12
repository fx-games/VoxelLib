// Package nav is stdlib-only NPC pathfinding on a voxel grid (1 block = 1 unit,
// humanoids 2 blocks tall).
//
// The A* search with reusable node storage and a generation counter is adapted
// from gruid paths (ISC License) and beefsack/go-astar (MIT License), via the
// author's TerminalAssassin geometry package.
package nav

import "container/heap"

// World reports whether a cell blocks movement.
type World interface{ Solid(x, y, z int) bool }

// Pos is a feet cell: non-solid, with a solid cell below and a non-solid cell above.
type Pos struct{ X, Y, Z int }

// Graph is not safe for concurrent use.
type Graph struct {
	W        World
	MaxDrop  int                     // 0 = default 3
	Passable func(from, to Pos) bool // optional per-actor filter; nil = all

	nodes map[Pos]*node
	gen   uint32
	open  nodeHeap
}

type node struct {
	pos    Pos
	g, f   int
	parent *node
	gen    uint32
	closed bool
	idx    int // heap index, -1 if not in heap
}

type nodeHeap []*node

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].idx, h[j].idx = i, j
}
func (h *nodeHeap) Push(x any) { n := x.(*node); n.idx = len(*h); *h = append(*h, n) }
func (h *nodeHeap) Pop() any {
	o := *h
	n := o[len(o)-1]
	*h = o[:len(o)-1]
	n.idx = -1
	return n
}

func (g *Graph) maxDrop() int {
	if g.MaxDrop <= 0 {
		return 3
	}
	return g.MaxDrop
}

// clear reports whether cells y..y+h-1 at (x,z) are non-solid.
func (g *Graph) clear(x, y, z, h int) bool {
	for i := 0; i < h; i++ {
		if g.W.Solid(x, y+i, z) {
			return false
		}
	}
	return true
}

// Walkable reports whether p is a valid feet cell.
func (g *Graph) Walkable(p Pos) bool {
	return g.clear(p.X, p.Y, p.Z, 2) && g.W.Solid(p.X, p.Y-1, p.Z)
}

// Neighbors appends the cells reachable from p in one step to buf[:0].
func (g *Graph) Neighbors(p Pos, buf []Pos) []Pos {
	buf = buf[:0]
	md := g.maxDrop()
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			tx, tz := p.X+dx, p.Z+dz
			// Step up needs 3 clear cells above the current floor.
			canUp := g.clear(p.X, p.Y, p.Z, 3)
			if dx != 0 && dz != 0 {
				// No corner cutting: both orthogonal columns must be clear.
				if !g.clear(p.X+dx, p.Y, p.Z, 2) || !g.clear(p.X, p.Y, p.Z+dz, 2) {
					continue
				}
			}
			var to Pos
			switch {
			case g.W.Solid(tx, p.Y, tz): // wall or step
				if !canUp {
					continue
				}
				if dx != 0 && dz != 0 && (!g.clear(p.X+dx, p.Y+2, p.Z, 1) || !g.clear(p.X, p.Y+2, p.Z+dz, 1)) {
					continue
				}
				to = Pos{tx, p.Y + 1, tz}
				if !g.Walkable(to) {
					continue
				}
			case g.W.Solid(tx, p.Y+1, tz): // low ceiling at target
				continue
			case g.W.Solid(tx, p.Y-1, tz):
				to = Pos{tx, p.Y, tz}
			default: // drop
				ok := false
				for d := 2; d <= md+1; d++ {
					if g.W.Solid(tx, p.Y-d, tz) {
						to, ok = Pos{tx, p.Y - d + 1, tz}, true
						break
					}
				}
				if !ok {
					continue
				}
			}
			if g.Passable == nil || g.Passable(p, to) {
				buf = append(buf, to)
			}
		}
	}
	return buf
}

// Ground finds the nearest walkable feet cell at or below (x,y,z), searching
// down up to 8 blocks and up 1.
func (g *Graph) Ground(x, y, z int) (Pos, bool) {
	if p := (Pos{x, y + 1, z}); g.Walkable(p) {
		return p, true
	}
	for d := 0; d <= 8; d++ {
		if p := (Pos{x, y - d, z}); g.Walkable(p) {
			return p, true
		}
	}
	return Pos{}, false
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func h(a, b Pos) int {
	dx, dz := abs(a.X-b.X), abs(a.Z-b.Z)
	mn, mx := dx, dz
	if mn > mx {
		mn, mx = mx, mn
	}
	return 14*mn + 10*(mx-mn) + 10*abs(a.Y-b.Y)
}

func (g *Graph) get(p Pos) *node {
	n := g.nodes[p]
	if n == nil {
		n = &node{pos: p}
		g.nodes[p] = n
	}
	if n.gen != g.gen {
		*n = node{pos: p, gen: g.gen, g: 1 << 30, idx: -1}
	}
	return n
}

// Path runs A* from..to (both included). maxNodes caps expansions (0 = 20000).
func (g *Graph) Path(from, to Pos, maxNodes int) ([]Pos, bool) {
	if maxNodes <= 0 {
		maxNodes = 20000
	}
	if !g.Walkable(from) || !g.Walkable(to) {
		return nil, false
	}
	if g.nodes == nil {
		g.nodes = map[Pos]*node{}
	}
	g.gen++
	if len(g.nodes) > 200000 { // bound memory of the reuse cache
		g.nodes = map[Pos]*node{}
	}
	g.open = g.open[:0]
	var buf [8]Pos
	s := g.get(from)
	s.g, s.f = 0, h(from, to)
	heap.Push(&g.open, s)
	for exp := 0; g.open.Len() > 0 && exp < maxNodes; exp++ {
		cur := heap.Pop(&g.open).(*node)
		if cur.pos == to {
			var path []Pos
			for n := cur; n != nil; n = n.parent {
				path = append(path, n.pos)
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path, true
		}
		cur.closed = true
		for _, np := range g.Neighbors(cur.pos, buf[:0]) {
			n := g.get(np)
			if n.closed {
				continue
			}
			c := 10
			if np.X != cur.pos.X && np.Z != cur.pos.Z {
				c = 14
			}
			if dy := np.Y - cur.pos.Y; dy > 0 {
				c += 5
			} else if dy < 0 {
				c += 2 * -dy
			}
			if ng := cur.g + c; ng < n.g {
				n.g, n.f, n.parent = ng, ng+h(np, to), cur
				if n.idx >= 0 {
					heap.Fix(&g.open, n.idx)
				} else {
					heap.Push(&g.open, n)
				}
			}
		}
	}
	return nil, false
}
