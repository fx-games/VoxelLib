// Package nav is stdlib-only NPC pathfinding on a voxel grid (1 block = 1 unit,
// walkers 1x2 cells unless Graph.Size says otherwise).
//
// The A* search with reusable node storage and a generation counter is adapted
// from gruid paths (ISC License) and beefsack/go-astar (MIT License), via the
// author's TerminalAssassin geometry package.
package nav

import "container/heap"

// World reports whether a cell blocks movement.
type World interface{ Solid(x, y, z int) bool }

// Pos is a feet cell: non-solid, with a solid cell below and a non-solid cell above.
// For a walker wider than one cell it is the footprint's min-x, min-z corner.
type Pos struct{ X, Y, Z int }

// Graph is not safe for concurrent use.
type Graph struct {
	W        World
	MaxDrop  int                     // 0 = default 3
	Passable func(from, to Pos) bool // optional per-actor filter; nil = all
	// Door, if set, marks cells holding a door or gate. They count as open
	// space whatever their state (the walker opens them on the way) and
	// never as floor.
	Door func(x, y, z int) bool
	// Water, if set, marks water cells. A feet cell in water is walkable
	// without a floor (the walker swims), and walkers drop into water from
	// any height.
	Water func(x, y, z int) bool
	// Cost, if set, adds a cost (10 = one straight step) for entering p,
	// e.g. for cells other walkers stand in.
	Cost func(p Pos) int
	// Size is the walker's footprint width and height in cells; 0 = 1 and 2.
	// Minecraft's PathFinder: floor(width+1) x floor(height+1), cells from
	// the Pos corner toward +x and +z (SizeOf).
	Size [2]int
	// Closest: a search that cannot reach its goal returns the path to the
	// expanded cell nearest to it instead of none (PathFinder.addToPath
	// returns the closest point), unless that cell is the start.
	Closest bool

	nodes map[Pos]*node
	gen   uint32
	open  nodeHeap

	// cells caches solid/water answers for the cells around one search
	// (W, Door and Water are callbacks into the game and cost far more
	// than a byte lookup; Neighbors asks about the same cells many times).
	cells      []uint8
	cx, cy, cz int // window origin
	caching    bool

	// the search Begin started
	to                 Pos
	maxNodes, expanded int
	start, best        *node // best: expanded node nearest to the goal
}

// Cache window: x and z within cacheR of the search's centre, y within
// cacheH/2 of it.
const (
	cacheR = 48
	cacheW = 2*cacheR + 1
	cacheH = 48
)

const (
	cellSolidKnown uint8 = 1 << iota
	cellSolid
	cellWaterKnown
	cellWater
)

// cell returns the cache byte for (x,y,z), or nil outside the window or
// when no search runs.
func (g *Graph) cell(x, y, z int) *uint8 {
	if !g.caching {
		return nil
	}
	x, y, z = x-g.cx, y-g.cy, z-g.cz
	if uint(x) >= cacheW || uint(z) >= cacheW || uint(y) >= cacheH {
		return nil
	}
	return &g.cells[(y*cacheW+z)*cacheW+x]
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

// solid is W.Solid with doors open.
func (g *Graph) solid(x, y, z int) bool {
	c := g.cell(x, y, z)
	if c != nil && *c&cellSolidKnown != 0 {
		return *c&cellSolid != 0
	}
	v := g.W.Solid(x, y, z) && (g.Door == nil || !g.Door(x, y, z))
	if c != nil {
		*c |= cellSolidKnown
		if v {
			*c |= cellSolid
		}
	}
	return v
}

func (g *Graph) water(x, y, z int) bool {
	if g.Water == nil {
		return false
	}
	c := g.cell(x, y, z)
	if c != nil && *c&cellWaterKnown != 0 {
		return *c&cellWater != 0
	}
	v := g.Water(x, y, z)
	if c != nil {
		*c |= cellWaterKnown
		if v {
			*c |= cellWater
		}
	}
	return v
}

// SizeOf is Graph.Size for a walker of the given box: floor(w+1), floor(h+1).
func SizeOf(width, height float64) [2]int { return [2]int{int(width + 1), int(height + 1)} }

func (g *Graph) size() (w, h int) {
	if g.Size[0] <= 0 {
		return 1, 2
	}
	return g.Size[0], g.Size[1]
}

// clear reports whether the footprint at (x,z), layers y..y+h-1, is non-solid.
func (g *Graph) clear(x, y, z, h int) bool {
	w, _ := g.size()
	for i := 0; i < h; i++ {
		for a := range w {
			for b := range w {
				if g.solid(x+a, y+i, z+b) {
					return false
				}
			}
		}
	}
	return true
}

// floor reports whether any footprint cell at (x,z) in layer y is solid.
func (g *Graph) floor(x, y, z int) bool { return !g.clear(x, y, z, 1) }

// Walkable reports whether p is a valid feet cell.
func (g *Graph) Walkable(p Pos) bool {
	_, h := g.size()
	return g.clear(p.X, p.Y, p.Z, h) && (g.floor(p.X, p.Y-1, p.Z) || g.water(p.X, p.Y, p.Z))
}

// Neighbors appends the cells reachable from p in one step to buf[:0].
func (g *Graph) Neighbors(p Pos, buf []Pos) []Pos {
	buf = buf[:0]
	md := g.maxDrop()
	_, ht := g.size()
	// Step up needs height+1 clear cells above the current floor.
	canUp := g.clear(p.X, p.Y, p.Z, ht+1)
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			tx, tz := p.X+dx, p.Z+dz
			if dx != 0 && dz != 0 {
				// No corner cutting: both orthogonal columns must be clear.
				if !g.clear(p.X+dx, p.Y, p.Z, ht) || !g.clear(p.X, p.Y, p.Z+dz, ht) {
					continue
				}
			}
			var to Pos
			switch {
			case g.floor(tx, p.Y, tz): // wall or step
				if !canUp {
					continue
				}
				if dx != 0 && dz != 0 && (!g.clear(p.X+dx, p.Y+ht, p.Z, 1) || !g.clear(p.X, p.Y+ht, p.Z+dz, 1)) {
					continue
				}
				to = Pos{tx, p.Y + 1, tz}
				if !g.Walkable(to) {
					continue
				}
			case !g.clear(tx, p.Y+1, tz, ht-1): // low ceiling at target
				continue
			case g.floor(tx, p.Y-1, tz) || g.water(tx, p.Y, tz):
				to = Pos{tx, p.Y, tz}
			default: // drop
				ok, limit := false, md+1
				if g.Water != nil {
					limit = 64
				}
				for d := 2; d <= limit; d++ {
					if g.water(tx, p.Y-d+1, tz) { // any height into water
						to, ok = Pos{tx, p.Y - d + 1, tz}, true
						break
					}
					if g.floor(tx, p.Y-d, tz) {
						to, ok = Pos{tx, p.Y - d + 1, tz}, d <= md+1
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
	if !g.Begin(from, to, maxNodes) {
		return nil, false
	}
	path, _ := g.Step(g.maxNodes)
	return path, path != nil
}

// Begin starts an A* search from..to that Step runs in slices, so a long
// search can spread over several ticks. maxNodes caps expansions (0 =
// 20000). It reports false, ending the search, when either end isn't
// walkable. One search runs per Graph; Begin abandons an unfinished one.
// The world's answers are cached for the whole search: block changes
// made meanwhile may go unseen until the next one.
func (g *Graph) Begin(from, to Pos, maxNodes int) bool {
	if maxNodes <= 0 {
		maxNodes = 20000
	}
	g.caching = false
	g.open = g.open[:0]
	if !g.Walkable(from) || !g.Walkable(to) {
		return false
	}
	if g.cells == nil {
		g.cells = make([]uint8, cacheW*cacheW*cacheH)
	} else {
		clear(g.cells)
	}
	g.cx, g.cy, g.cz = (from.X+to.X)/2-cacheR, (from.Y+to.Y)/2-cacheH/2, (from.Z+to.Z)/2-cacheR
	g.caching = true
	if g.nodes == nil {
		g.nodes = map[Pos]*node{}
	}
	g.gen++
	if len(g.nodes) > 200000 { // bound memory of the reuse cache
		g.nodes = map[Pos]*node{}
	}
	g.to, g.maxNodes, g.expanded = to, maxNodes, 0
	s := g.get(from)
	s.g, s.f = 0, h(from, to)
	g.start, g.best = s, s
	heap.Push(&g.open, s)
	return true
}

// Step expands at most budget nodes of the search Begin started. done
// reports that the search has ended: with the path (from..to) or, when
// path is nil, without one.
func (g *Graph) Step(budget int) (path []Pos, done bool) {
	to := g.to
	var buf [8]Pos
	for ; budget > 0; budget-- {
		if g.open.Len() == 0 || g.expanded >= g.maxNodes {
			g.open = g.open[:0]
			if g.Closest && g.best != g.start {
				return trace(g.best), true
			}
			return nil, true
		}
		g.expanded++
		cur := heap.Pop(&g.open).(*node)
		if cur.pos == to {
			g.open = g.open[:0]
			return trace(cur), true
		}
		if h(cur.pos, to) < h(g.best.pos, to) {
			g.best = cur
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
			if g.Cost != nil {
				c += g.Cost(np)
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
	if g.open.Len() == 0 {
		return g.Step(1) // ended exactly at the budget: report it, with Closest
	}
	return nil, false
}

// trace returns the path from the search's start to n.
func trace(n *node) (path []Pos) {
	for ; n != nil; n = n.parent {
		path = append(path, n.pos)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// Expanded is the number of nodes the current search has expanded.
func (g *Graph) Expanded() int { return g.expanded }

// Smooth drops path cells a walker can skip by heading straight for a later
// one: on the same level, over walkable cells only, keeping every cell next
// to a door (the walker must pass doors square). The ends stay.
func (g *Graph) Smooth(path []Pos) []Pos {
	if len(path) < 3 {
		return path
	}
	out := []Pos{path[0]}
	a := 0
	for i := 2; i < len(path); i++ {
		if !g.straight(path[a], path[i]) || g.nearDoor(path[i-1]) {
			out = append(out, path[i-1])
			a = i - 1
		}
	}
	return append(out, path[len(path)-1])
}

func (g *Graph) nearDoor(p Pos) bool {
	if g.Door == nil {
		return false
	}
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if g.Door(p.X+dx, p.Y, p.Z+dz) {
				return true
			}
		}
	}
	return false
}

// straight reports whether a walker of width < 1 goes from a to b (same Y)
// in a straight line: every cell its centre line and a 0.4 wide band touch
// is walkable, sampled every quarter block.
func (g *Graph) straight(a, b Pos) bool {
	if a.Y != b.Y {
		return false
	}
	dx, dz := float64(b.X-a.X), float64(b.Z-a.Z)
	n := int(4*max(abs(b.X-a.X), abs(b.Z-a.Z))) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x, z := float64(a.X)+0.5+dx*t, float64(a.Z)+0.5+dz*t
		for _, o := range [...][2]float64{{0, 0}, {0.4, 0.4}, {-0.4, 0.4}, {0.4, -0.4}, {-0.4, -0.4}} {
			if !g.Walkable(Pos{ifloor(x + o[0]), a.Y, ifloor(z + o[1])}) {
				return false
			}
		}
	}
	return true
}

func ifloor(v float64) int {
	i := int(v)
	if float64(i) > v {
		i--
	}
	return i
}
