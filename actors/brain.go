package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
)

// Mode is an actor's standing behaviour when not fighting.
type Mode uint8

const (
	Stand  Mode = iota // stay at Home (walk back if pushed off), look around
	Wander             // stroll around Home within Range
	Follow             // keep up with Leader in formation
	Route              // walk the waypoints
	Guard              // Stand, but fight hostiles within Range of Home and return
	Flee               // keep away from hostiles
)

// RoutePattern is what happens at a route's end.
type RoutePattern uint8

const (
	Once      RoutePattern = iota // stop at the last point
	Loop                          // start again at the first
	BackForth                     // walk back the other way (CustomNPC "backtracking")
)

// Brain is an actor's behaviour, plain data so it saves as it is.
type Brain struct {
	Mode  Mode    `json:"mode"`
	Home  Vec     `json:"home"`
	Range float64 `json:"range,omitempty"` // wander radius, guard leash
	Face  float64 `json:"face"`            // yaw while standing at Home or a route's end
	Look  Look    `json:"look,omitempty"`  // idle looking; 0 = watch the nearest player
	// SpeedMul is the AI speed factor (the reference tryMoveTo speed; 0 = 1).
	SpeedMul float64 `json:"speedMul,omitempty"`

	// Follow: Leader is an actor ID. Offset is the formation slot in the
	// leader's frame (X to its right, Z behind it). The follower stops
	// within StopDist of its slot, runs to catch up beyond CatchUp blocks,
	// and with Defend fights whatever threatens the leader or the company.
	Leader   string  `json:"leader,omitempty"`
	Offset   Vec     `json:"offset"`
	StopDist float64 `json:"stopDist,omitempty"`
	CatchUp  float64 `json:"catchUp,omitempty"`
	Defend   bool    `json:"defend,omitempty"`

	// Route: the waypoints, walked with A* between them. Pause is the
	// ticks to wait at each point. Lead names an actor (the player) the
	// walker guides: it waits when Lead falls LeadGap points behind.
	// Spread moves each point sideways by up to this many blocks (each
	// walker of a group its own side), so a group does not walk single
	// file; points near doors stay exact.
	Points  []Vec        `json:"points,omitempty"`
	Pattern RoutePattern `json:"pattern"`
	Pause   int          `json:"pause,omitempty"`
	Lead    string       `json:"lead,omitempty"`
	LeadGap int          `json:"leadGap,omitempty"`
	Spread  float64      `json:"spread,omitempty"`
	Next    int          `json:"next"`   // index of the point walked to
	Back    bool         `json:"back"`   // walking the route backwards
	Done    bool         `json:"done"`   // a Once route is finished
	LookAt  string       `json:"lookAt"` // actor ID to watch when idle ("" = nearest player)
	Side    float64      `json:"side"`   // this walker's lateral share (-1..1) of Spread
}

// FormationSlot returns the i-th of a group's slots behind a leader: rows
// of three, gap blocks apart, the first row gap behind it.
func FormationSlot(i int, gap float64) Vec {
	row, col := i/3, i%3
	return Vec{X: float64(col-1) * gap, Z: float64(row+1) * gap}
}

func (b *Brain) speed() float64 {
	if b.SpeedMul > 0 {
		return b.SpeedMul
	}
	return 1
}

// think runs the actor's behaviour for one tick: combat first, then the
// mode. It only sets the carrier's input; bodies move in Manager.Tick.
func (m *Manager) think(a *Actor) {
	if f := a.Fly; f != nil {
		a.nav.in = character.Input{}
		if f.Think != nil {
			f.Think(a)
		}
		return
	}
	if a.panic > 0 {
		a.panic--
		if src := a.revenge; src != nil {
			m.moveTo(a, away(a.Pos(), src.Pos(), 6), 1.25)
			return
		}
	}
	if m.fight(a) {
		return
	}
	b := &a.Brain
	switch b.Mode {
	case Stand, Guard:
		if a.Pos().hdist(b.Home) > 1 && !m.moveTo(a, b.Home, b.speed()) {
			return
		}
		m.stop(a)
		m.idleLook(a, b.Face)
	case Wander:
		m.wander(a)
	case Follow:
		m.follow(a)
	case Route:
		m.route(a)
	case Flee:
		if t := m.nearestHostile(a, a.AggroRange); t != nil {
			m.moveTo(a, away(a.Pos(), t.Pos(), 8), 1.25)
			return
		}
		m.stop(a)
		m.idleLook(a, a.Yaw)
	}
}

// away is a spot dist blocks from p, directly away from threat.
func away(p, threat Vec, dist float64) Vec {
	dx, dz := p.X-threat.X, p.Z-threat.Z
	d := math.Max(1e-3, math.Hypot(dx, dz))
	return Vec{p.X + dx/d*dist, p.Y, p.Z + dz/d*dist}
}

// idleLook turns the head toward the actor named in LookAt, else the
// nearest player within 8 blocks (EntityAIWatchClosest), else to face.
func (m *Manager) idleLook(a *Actor, face float64) {
	if a.Brain.Look != LookPlayer && a.Brain.LookAt == "" {
		m.cnpcLook(a)
		return
	}
	var t *Actor
	if a.Brain.LookAt != "" {
		t = m.byID[a.Brain.LookAt]
	} else {
		best := 64.0
		for _, o := range m.Actors {
			if o.Player && o != a {
				if d := o.Pos().dist2(a.Pos()); d < best {
					t, best = o, d
				}
			}
		}
	}
	if t != nil && t.Pos().dist2(a.Pos()) < 64 {
		want := yawTo(a.Pos(), t.Pos())
		a.HeadYaw = turn(a.HeadYaw, want, 10*math.Pi/180)
		// the body follows when the head turns past 75° (EntityBodyHelper)
		if d := math.Remainder(a.HeadYaw-a.Yaw, 2*math.Pi); math.Abs(d) > 75*math.Pi/180 {
			a.Yaw = turn(a.Yaw, want, 10*math.Pi/180)
		}
		return
	}
	a.Yaw = turn(a.Yaw, face, 10*math.Pi/180)
	a.HeadYaw = turn(a.HeadYaw, a.Yaw, 10*math.Pi/180)
}

func (m *Manager) wander(a *Actor) {
	b := &a.Brain
	n := &a.nav
	if n.has {
		if m.moveTo(a, n.goal, b.speed()) || n.stuck >= 3 {
			m.stop(a)
			n.stuck = 0
		}
		return
	}
	m.stop(a)
	m.idleLook(a, a.Yaw)
	if m.Rand.Intn(120) != 0 { // EntityAIWander: a 1 in 120 chance a tick
		return
	}
	r := b.Range
	if r <= 0 {
		r = 10
	}
	ang, d := m.Rand.Float64()*2*math.Pi, m.Rand.Float64()*r
	goal := Vec{b.Home.X + math.Cos(ang)*d, b.Home.Y + 3, b.Home.Z + math.Sin(ang)*d}
	if p, ok := m.ground(m.graphFor(a), goal); ok {
		m.plan(a, center(p))
	}
}

// follow keeps a follower at its slot near the leader, walking the leader's
// track when it is far behind.
func (m *Manager) follow(a *Actor) {
	b := &a.Brain
	l := m.byID[b.Leader]
	if l == nil || !l.Alive() {
		m.stop(a)
		m.idleLook(a, a.Yaw)
		return
	}
	lp := l.carrier().Pos()
	ly := l.Yaw
	// the slot, in the leader's frame: +X right, +Z behind
	sx := lp.X + math.Cos(ly)*b.Offset.X - math.Sin(ly)*b.Offset.Z
	sz := lp.Z + math.Sin(ly)*b.Offset.X + math.Cos(ly)*b.Offset.Z
	slot := Vec{sx, lp.Y, sz}
	stop := b.StopDist
	if stop <= 0 {
		stop = 1
	}
	d := a.Pos().hdist(lp)
	if a.Pos().hdist(slot) < stop || d < 2 {
		m.stop(a)
		m.idleLook(a, l.Yaw)
		return
	}
	mul := b.speed()
	if cu := b.CatchUp; (cu > 0 && d > cu) || (cu == 0 && d > 16) {
		mul *= 1.3
	}
	goal := slot
	if d > 14 { // far behind: walk where the leader walked
		if t := m.trackPoint(l, a.Pos()); t != nil {
			goal = *t
		}
	} else if p, ok := m.ground(m.graphFor(a), slot); !ok || !m.graph.Walkable(p) {
		goal = lp // no floor at the slot: go for the leader
	}
	m.moveTo(a, goal, mul)
}

// trackPoint is a point of the leader's track a few steps ahead of the
// track point nearest to p.
func (m *Manager) trackPoint(l *Actor, p Vec) *Vec {
	tr := m.tracks[l.ID]
	if len(tr) == 0 {
		return nil
	}
	j, best := 0, math.Inf(1)
	for i, t := range tr {
		if d := t.dist2(p); d < best {
			j, best = i, d
		}
	}
	if best > 256 {
		return nil
	}
	t := tr[min(j+2, len(tr)-1)]
	return &t
}

// route walks the waypoints.
func (m *Manager) route(a *Actor) {
	b := &a.Brain
	if len(b.Points) == 0 || b.Done {
		m.stop(a)
		m.idleLook(a, b.Face)
		return
	}
	if a.routeWait > 0 {
		a.routeWait--
		m.stop(a)
		m.idleLook(a, a.Yaw)
		return
	}
	if b.Lead != "" { // guiding: wait for the one we lead
		if l := m.byID[b.Lead]; l != nil {
			gap := b.LeadGap
			if gap <= 0 {
				gap = 3
			}
			if b.Next > nearestPoint(b.Points, l.Pos())+gap && a.Pos().hdist(l.Pos()) > 8 {
				m.stop(a)
				m.idleLook(a, yawTo(a.Pos(), l.Pos()))
				return
			}
		}
	}
	b.Next = max(0, min(b.Next, len(b.Points)-1))
	p := b.Points[b.Next]
	last := (!b.Back && b.Next == len(b.Points)-1) || (b.Back && b.Next == 0)
	goal := p
	if !last {
		goal = m.spread(a, p)
	}
	reach := 1.5
	if last {
		reach = 0.5
	}
	if a.Pos().hdist(goal) > reach || math.Abs(a.Pos().Y-goal.Y) > 2 {
		m.moveTo(a, goal, b.speed())
		return
	}
	a.routeWait = b.Pause
	switch {
	case !last && !b.Back:
		b.Next++
	case !last:
		b.Next--
	case b.Pattern == Loop:
		b.Next = 0
	case b.Pattern == BackForth:
		b.Back = !b.Back
		if b.Back {
			b.Next = max(0, len(b.Points)-2)
		} else {
			b.Next = min(1, len(b.Points)-1)
		}
	default:
		b.Done = true
		m.stop(a)
	}
}

func nearestPoint(ps []Vec, p Vec) int {
	j, best := 0, math.Inf(1)
	for i, q := range ps {
		if d := q.hdist(p); d < best {
			j, best = i, d
		}
	}
	return j
}

// spread moves a shared waypoint sideways across the walking direction by
// the walker's share of Spread, unless that spot is no floor or a door is
// near.
func (m *Manager) spread(a *Actor, p Vec) Vec {
	s := a.Brain.Spread * a.Brain.Side
	if s == 0 {
		return p
	}
	pos := a.Pos()
	dx, dz := p.X-pos.X, p.Z-pos.Z
	d := math.Hypot(dx, dz)
	if d < 1 {
		return p
	}
	q := Vec{p.X - dz/d*s, p.Y, p.Z + dx/d*s}
	g := m.graphFor(a)
	c, ok := m.ground(g, q)
	if !ok || math.Abs(float64(c.Y)-p.Y) > 1 {
		return p
	}
	if g.Door != nil {
		for x := c.X - 3; x <= c.X+3; x++ {
			for z := c.Z - 3; z <= c.Z+3; z++ {
				for y := c.Y - 1; y <= c.Y+1; y++ {
					if g.Door(x, y, z) {
						return p
					}
				}
			}
		}
	}
	return Vec{q.X, float64(c.Y), q.Z}
}
