package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/nav"
)

// navigator walks an actor along an A* path toward a goal.
type navigator struct {
	path   []nav.Pos
	i      int
	goal   Vec
	has    bool
	age    int // ticks since the path was planned
	check  Vec // position at the last stuck check
	checkT int
	stuck  int       // stuck checks failed in a row
	opened []nav.Pos // doors this actor opened and must close behind it
	in     character.Input
	// pending: a search for from..to is queued or running (Manager.pump);
	// the actor keeps its old path until it ends.
	pending  bool
	from, to nav.Pos
	// detour: ticks left on a detour, taken after the walker stuck twice (a
	// partial plan's dead end behind a hill); tries picks the next of
	// detourAngles. Angle 0 is one long search for the goal itself, walked
	// to its end; the others a leg turned that far away from the goal.
	detour int
	detAng float64
	tries  int
	nodes  int // the search's expansion cap
}

// Planning limits: A* reaches at most planRange blocks ahead (a far goal is
// approached in legs), and a path is replanned every replan ticks while the
// goal moves.
//
// pathBudget caps the A* expansions of all actors in one tick (about 1 ms):
// a search that needs more continues next tick, so one unreachable goal
// (6000 expansions) no longer stalls a tick for 10-30 ms.
const (
	planRange  = 40
	replan     = 40
	pathBudget = 2000
	maxNodes   = 6000
	turnRate   = 30 * math.Pi / 180 // EntityMoveHelper turns at most 30° a tick
)

func (m *Manager) graphFor(a *Actor) *nav.Graph {
	m.setGraph(&m.graph, a)
	return &m.graph
}

// setGraph points g's callbacks at the world as a walker sees it.
func (m *Manager) setGraph(g *nav.Graph, a *Actor) {
	if m.doorFn == nil { // made once: closures made per call would allocate
		m.doorFn = func(x, y, z int) bool { d, _ := m.W.Door(x, y, z); return d }
		m.waterFn = func(x, y, z int) bool { return m.W.Liquid(x, y, z) == character.LiquidWater }
		m.costFn = func(p nav.Pos) int {
			if m.occupied[p] {
				return 30
			}
			return 0
		}
	}
	g.W, g.Door, g.Water, g.Cost = m.W, nil, nil, m.costFn
	g.Size, g.Closest = nav.SizeOf(a.Body.Width, a.Body.Height), true
	if a.OpensDoors {
		g.Door = m.doorFn
	}
	if a.CanSwim {
		g.Water = m.waterFn
	}
}

// ground finds the feet cell (footprint corner) for a point under the
// footprint's centre.
func (m *Manager) ground(g *nav.Graph, v Vec) (nav.Pos, bool) {
	o := float64(g.Size[0]-1) / 2
	return g.Ground(int(math.Floor(v.X-o)), int(math.Floor(v.Y+0.01)), int(math.Floor(v.Z-o)))
}

// start is the search's start for a walker at v: its ground cell, else the
// cell it hangs in (caught in leaves, mid-jump), as PathFinder takes it.
func (m *Manager) start(g *nav.Graph, v Vec) nav.Pos {
	if p, ok := m.ground(g, v); ok {
		return p
	}
	o := float64(g.Size[0]-1) / 2
	return nav.Pos{X: int(math.Floor(v.X - o)), Y: int(math.Floor(v.Y + 0.01)), Z: int(math.Floor(v.Z - o))}
}

// Detours: the long search reaches longRange blocks with longNodes
// expansions (spread over ticks by pathBudget); a turned leg is detourLen
// blocks long and held for detourTicks.
const (
	detourTicks = 200
	longTicks   = 1200 // the long path is walked to its end, at most this long
	detourLen   = 20
	longRange   = 4 * planRange
	longNodes   = 10 * maxNodes
)

var detourAngles = [...]float64{0, 60, -60, 120, -120, 180}

func (m *Manager) plan(a *Actor, goal Vec) {
	c := a.carrier()
	n := &a.nav
	n.goal, n.has, n.age = goal, true, 0
	g := m.graphFor(c)
	from := m.start(g, c.Pos())
	leg, reach := goal, float64(planRange)
	n.nodes = maxNodes
	d := c.Pos().hdist(goal)
	switch {
	case n.detour > 0 && n.detAng == 0:
		reach, n.nodes = longRange, longNodes
	case n.detour > 0 && d > 0:
		s, co := math.Sincos(math.Atan2(goal.Z-c.Body.Z, goal.X-c.Body.X) + n.detAng*math.Pi/180)
		leg, d = Vec{c.Body.X + co*detourLen, c.Body.Y + 4, c.Body.Z + s*detourLen}, 0
	}
	if d > reach { // a leg toward a far goal
		f := reach / d
		leg = Vec{c.Body.X + (goal.X-c.Body.X)*f, c.Body.Y + 4, c.Body.Z + (goal.Z-c.Body.Z)*f}
	}
	// No footing at the leg for this walker: head for the spot anyway and
	// take the nearest point (Graph.Closest).
	to := m.start(g, leg)
	n.from, n.to = from, to
	if !n.pending { // else the queued or running search takes the new ends
		n.pending = true
		m.queue = append(m.queue, a)
	}
	m.pump()
}

// pump runs queued path searches, one at a time, until this tick's
// expansion budget is spent; an unfinished search continues next tick.
func (m *Manager) pump() {
	for m.budget > 0 {
		if m.searching == nil {
			if len(m.queue) == 0 {
				return
			}
			a := m.queue[0]
			m.queue = m.queue[:copy(m.queue, m.queue[1:])]
			n := &a.nav
			if !n.pending {
				continue
			}
			m.setGraph(&m.search, a.carrier())
			if !m.search.Begin(n.from, n.to, n.nodes) {
				m.finish(a, nil)
				continue
			}
			m.searching = a
		}
		before := m.search.Expanded()
		p, done := m.search.Step(m.budget)
		m.budget -= m.search.Expanded() - before
		if !done {
			return
		}
		a := m.searching
		m.searching = nil
		if p != nil {
			p = m.search.Smooth(p)
		}
		m.finish(a, p)
	}
}

func (m *Manager) finish(a *Actor, p []nav.Pos) {
	n := &a.nav
	if !n.pending {
		return
	}
	n.pending = false
	n.path, n.i = p, 0
	if p != nil {
		n.i = 1
	}
}

// cancelSearch drops a's queued or running search.
func (m *Manager) cancelSearch(a *Actor) {
	a.nav.pending = false
	if m.searching == a {
		m.searching = nil
	}
}

func center(p nav.Pos) Vec { return Vec{float64(p.X) + 0.5, float64(p.Y), float64(p.Z) + 0.5} }

// footCenter is where a walker of a's size stands on path cell p: the
// footprint's centre (PathEntity.getVectorFromIndex: x + (int)(width+1)*0.5).
func footCenter(a *Actor, p nav.Pos) Vec {
	o := float64(nav.SizeOf(a.Body.Width, a.Body.Height)[0]) / 2
	return Vec{float64(p.X) + o, float64(p.Y), float64(p.Z) + o}
}

// moveTo steers a (or its mount) one tick toward goal at the speed factor
// mul and reports whether it has arrived (within 0.4 blocks).
func (m *Manager) moveTo(a *Actor, goal Vec, mul float64) bool {
	c := a.carrier()
	n := &a.nav
	pos := c.Pos()
	if pos.hdist(goal) < 0.4 && math.Abs(pos.Y-goal.Y) < 1.5 {
		m.stop(a)
		return true
	}
	n.age++
	if n.detour > 0 {
		n.detour--
		if n.detour == 0 || n.path != nil && n.i >= len(n.path) { // detour walked
			n.detour, n.has = 0, false
		}
	}
	if !n.has || (goal.dist2(n.goal) > 2.25 && n.age > 10) || (goal != n.goal && n.age > replan) ||
		(n.path != nil && n.i >= len(n.path) && pos.hdist(goal) > 2) {
		m.plan(a, goal)
	}
	if n.pending && n.path == nil { // wait for the search rather than walk blind
		c.nav.in = character.Input{Yaw: c.Yaw, Jump: c.Body.InWater && c.CanSwim}
		return false
	}
	next := goal
	var nextCell *nav.Pos
	if n.path != nil {
		for n.i < len(n.path) {
			cc := footCenter(c, n.path[n.i])
			if pos.hdist(cc) < 0.5 && math.Abs(pos.Y-cc.Y) < 1.1 {
				n.i++
				continue
			}
			break
		}
		if n.i < len(n.path) {
			next = footCenter(c, n.path[n.i])
			nextCell = &n.path[n.i]
			if n.i == len(n.path)-1 && goal.hdist(next) < 1 {
				next = goal
			}
		}
	}
	m.openDoors(c, n)
	want := yawTo(pos, next)
	c.Yaw = turn(c.Yaw, want, turnRate)
	if a != c {
		a.Yaw = c.Yaw
	}
	fwd := 1.0
	if math.Abs(math.Remainder(want-c.Yaw, 2*math.Pi)) > math.Pi/2 {
		fwd = 0.1 // turn first
	}
	if d := pos.hdist(next); d < 1 && next == goal {
		fwd *= math.Max(0.3, d)
	}
	b := c.Body
	jump := b.OnGround && b.HorizontalCollision
	if nextCell != nil && b.OnGround && float64(nextCell.Y) > b.Y+0.3 && pos.hdist(next) < 1.6 {
		jump = true
	}
	if b.InWater && c.CanSwim && !(nextCell != nil && float64(nextCell.Y) < b.Y-0.5) {
		jump = true // keep the head up (EntityAISwimming)
	}
	// An AI walker's forward input is its AI speed and its acceleration
	// that times the speed again (EntityMoveHelper + moveFlying), so the
	// ground push is speed²: zombie 0.23 → 2.3 m/s, the player's 0.1 with
	// full input → 4.3 m/s.
	s := c.Speed * mul
	b.MoveSpeed = s * s / 0.98
	n2 := &c.nav
	n2.in = character.Input{Forward: fwd, Yaw: c.Yaw, Jump: jump}
	// Stuck check every 20 ticks: no progress means a new plan, and a hop.
	n.checkT++
	if n.checkT >= 20 {
		if pos.hdist(n.check) < 0.3 { // horizontal: a walker hopping on the spot is stuck too
			n.stuck++
			n.has = false
			n2.in.Jump = b.OnGround
			if n.stuck >= 2 && n.detour == 0 {
				n.detAng, n.detour, n.stuck, n.path = detourAngles[n.tries%len(detourAngles)], detourTicks, 0, nil
				if n.detAng == 0 {
					n.detour = longTicks
				}
				n.tries++
			}
		} else {
			n.stuck = 0
		}
		n.check, n.checkT = pos, 0
	}
	return false
}

// stop leaves the carrier standing.
func (m *Manager) stop(a *Actor) {
	c := a.carrier()
	a.nav.has, a.nav.path = false, nil
	m.cancelSearch(a)
	c.nav.in = character.Input{Yaw: c.Yaw, Jump: c.Body.InWater && c.CanSwim}
}

// openDoors opens closed doors on the next path cells as the walker comes
// near, and remembers them to close behind it.
func (m *Manager) openDoors(c *Actor, n *navigator) {
	if !c.OpensDoors || n.path == nil {
		return
	}
	pos := c.Pos()
	for j := max(0, n.i-1); j < min(len(n.path), n.i+2); j++ {
		p := n.path[j]
		for dy := 0; dy < 2; dy++ {
			q := nav.Pos{X: p.X, Y: p.Y + dy, Z: p.Z}
			door, open := m.W.Door(q.X, q.Y, q.Z)
			if !door || pos.hdist(center(q)) > 2.2 {
				continue
			}
			if !open {
				m.W.SetDoor(q.X, q.Y, q.Z, true)
				n.opened = append(n.opened, q)
			}
			break // one door cell per column
		}
	}
}

// closeDoors shuts doors an actor opened once it is clear of them and
// nobody stands in the doorway.
func (m *Manager) closeDoors(a *Actor) {
	n := &a.nav
	keep := n.opened[:0]
	for _, q := range n.opened {
		c := center(q)
		if a.carrier().Pos().hdist(c) < 2 || m.inDoorway(q) {
			keep = append(keep, q)
			continue
		}
		if _, open := m.W.Door(q.X, q.Y, q.Z); open {
			m.W.SetDoor(q.X, q.Y, q.Z, false)
		}
	}
	n.opened = keep
}

func (m *Manager) inDoorway(q nav.Pos) bool {
	box := center(q)
	for _, o := range m.Actors {
		p := o.Pos()
		if math.Abs(p.X-box.X) < 0.5+o.Body.Width/2 && math.Abs(p.Z-box.Z) < 0.5+o.Body.Width/2 &&
			p.Y < box.Y+2 && p.Y+o.Body.Height > box.Y {
			return true
		}
	}
	return false
}
