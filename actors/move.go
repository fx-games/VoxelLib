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
}

// Planning limits: A* reaches at most planRange blocks ahead (a far goal is
// approached in legs), and a path is replanned every replan ticks while the
// goal moves.
const (
	planRange = 40
	replan    = 40
	turnRate  = 30 * math.Pi / 180 // EntityMoveHelper turns at most 30° a tick
)

func (m *Manager) graphFor(a *Actor) *nav.Graph {
	g := &m.graph
	g.W = m.W
	g.Door = func(x, y, z int) bool { d, _ := m.W.Door(x, y, z); return d }
	if !a.OpensDoors {
		g.Door = nil
	}
	g.Water = nil
	if a.CanSwim {
		g.Water = func(x, y, z int) bool { return m.W.Liquid(x, y, z) == character.LiquidWater }
	}
	g.Cost = func(p nav.Pos) int {
		if m.occupied[p] {
			return 30
		}
		return 0
	}
	return g
}

// ground finds the feet cell for a point.
func (m *Manager) ground(g *nav.Graph, v Vec) (nav.Pos, bool) {
	return g.Ground(int(math.Floor(v.X)), int(math.Floor(v.Y+0.01)), int(math.Floor(v.Z)))
}

func (m *Manager) plan(a *Actor, goal Vec) {
	c := a.carrier()
	n := &a.nav
	n.goal, n.has, n.age, n.path, n.i = goal, true, 0, nil, 0
	g := m.graphFor(c)
	from, ok1 := m.ground(g, c.Pos())
	leg := goal
	if d := c.Pos().hdist(goal); d > planRange { // a leg toward a far goal
		f := planRange / d
		leg = Vec{c.Body.X + (goal.X-c.Body.X)*f, c.Body.Y + 4, c.Body.Z + (goal.Z-c.Body.Z)*f}
	}
	to, ok2 := m.ground(g, leg)
	if !ok1 || !ok2 {
		return
	}
	if p, ok := g.Path(from, to, 6000); ok {
		n.path = g.Smooth(p)
		n.i = 1
	}
}

func center(p nav.Pos) Vec { return Vec{float64(p.X) + 0.5, float64(p.Y), float64(p.Z) + 0.5} }

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
	if !n.has || (goal.dist2(n.goal) > 2.25 && n.age > 10) || (goal != n.goal && n.age > replan) ||
		(n.path != nil && n.i >= len(n.path) && pos.hdist(goal) > 2) {
		m.plan(a, goal)
	}
	next := goal
	var nextCell *nav.Pos
	if n.path != nil {
		for n.i < len(n.path) {
			cc := center(n.path[n.i])
			if pos.hdist(cc) < 0.5 && math.Abs(pos.Y-cc.Y) < 1.1 {
				n.i++
				continue
			}
			break
		}
		if n.i < len(n.path) {
			next = center(n.path[n.i])
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
		if pos.dist2(n.check) < 0.09 {
			n.stuck++
			n.has = false
			n2.in.Jump = b.OnGround
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
