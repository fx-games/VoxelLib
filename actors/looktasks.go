package actors

import "math"

// LookKind is a look task's class.
type LookKind uint8

const (
	LookWatch  LookKind = iota // EntityAIWatchClosest (mutex 2)
	LookWatch2                 // EntityAIWatchClosest2 (mutex 3: also stops for a move)
	LookIdle                   // EntityAILookIdle (mutex 3)
)

// LookTask is one look task of a mob's task list. They share the look
// mutex bit, so one runs at a time; a higher priority (a lower number)
// one may start over a running one. Actor.Looks, in priority order,
// replaces the default idle look.
type LookTask struct {
	Priority int
	Kind     LookKind
	// Of is the watched class (World.findNearestEntityWithinAABB in the
	// box grown Dist, 3, Dist); nil watches players
	// (getClosestPlayerToEntity within Dist).
	Of           func(*Actor) bool
	Dist, Chance float32
}

// lookRun is the running look task's state.
type lookRun struct {
	ran, busy bool // this tick: run from the idle look; fighting
	on        int  // index into Looks + 1 (0: none)
	target    *Actor
	time      int // lookTime / idleTime
	x, z      float64
}

// Watching reports whether a look task holds a's head.
func (a *Actor) Watching() bool { return a.lt.on != 0 }

func (m *Manager) lookShould(a *Actor, t *LookTask) bool {
	if t.Kind == LookIdle {
		return m.Rand.Float32() < 0.02
	}
	if m.Rand.Float32() >= t.Chance {
		return false
	}
	var w *Actor
	if t.Of == nil {
		w = m.nearestPlayer(a, float64(t.Dist))
	} else {
		w = m.nearestOf(a, t)
	}
	a.lt.target = w
	return w != nil
}

// nearestOf is findNearestEntityWithinAABB(class, box grown Dist, 3, Dist).
func (m *Manager) nearestOf(a *Actor, t *LookTask) *Actor {
	bx := a.Box()
	d := float64(t.Dist)
	var best *Actor
	bd := math.Inf(1)
	for _, o := range m.Actors {
		if o == a || !o.Alive() || !t.Of(o) {
			continue
		}
		ob := o.Box()
		if ob.MaxX <= bx.MinX-d || ob.MinX >= bx.MaxX+d || ob.MaxY <= bx.MinY-3 || ob.MinY >= bx.MaxY+3 || ob.MaxZ <= bx.MinZ-d || ob.MinZ >= bx.MaxZ+d {
			continue
		}
		if dd := o.Pos().dist2(a.Pos()); dd < bd {
			best, bd = o, dd
		}
	}
	return best
}

func (m *Manager) lookCont(a *Actor, t *LookTask) bool {
	if t.Kind == LookIdle {
		return a.lt.time >= 0
	}
	w := a.lt.target
	return w != nil && w.Alive() && w.Pos().dist2(a.Pos()) <= float64(t.Dist*t.Dist) && a.lt.time > 0
}

func (m *Manager) lookStart(a *Actor, t *LookTask) {
	if t.Kind == LookIdle {
		d := math.Pi * 2 * m.Rand.Float64()
		a.lt.x, a.lt.z = math.Cos(d), math.Sin(d)
		a.lt.time = 20 + m.Rand.Intn(20)
		return
	}
	a.lt.time = 40 + m.Rand.Intn(40)
}

// lookTasks runs a's look tasks for one tick as EntityAITasks does
// (starts on every third tick, continueExecuting on the others), then
// EntityLookHelper and EntityBodyHelper. busy: a task of a higher
// priority holding the look (attack, panic) runs; moving: a move task
// runs, which resets WatchClosest2 and LookIdle.
func (m *Manager) lookTasks(a *Actor, busy, moving bool) { m.lookRun(a, busy, moving, moving) }

// lookRun is lookTasks with the movement tasks' mutex (gate: only
// WatchClosest runs) apart from the body's motion (moving: the head kept
// within 75 of it).
func (m *Manager) lookRun(a *Actor, busy, gate, moving bool) {
	l := &a.lt
	stop := func() { *l = lookRun{ran: l.ran, busy: l.busy} }
	if busy || a.LookHeld {
		stop()
		return
	}
	if l.on != 0 {
		t := &a.Looks[l.on-1]
		if gate && t.Kind != LookWatch || !m.lookCont(a, t) {
			stop()
		}
	}
	if m.ticks%3 == 0 {
		for i := range a.Looks {
			t := &a.Looks[i]
			if l.on != 0 && t.Priority >= a.Looks[l.on-1].Priority {
				break
			}
			if gate && t.Kind != LookWatch {
				continue
			}
			keep := *l
			if m.lookShould(a, t) {
				*l = lookRun{ran: l.ran, busy: l.busy, on: i + 1, target: l.target}
				m.lookStart(a, t)
				break
			}
			*l = keep
		}
	}
	look, x, y, z := false, 0.0, 0.0, 0.0
	if l.on != 0 {
		p := a.Pos()
		if t := &a.Looks[l.on-1]; t.Kind == LookIdle {
			l.time--
			look, x, y, z = true, p.X+l.x, p.Y+a.Body.EyeHeight, p.Z+l.z
		} else {
			w := l.target
			look, x, y, z = true, w.Body.X, w.Body.Y+w.Body.EyeHeight, w.Body.Z
			l.time--
		}
	}
	if v := a.LookPos; v != nil {
		look, x, y, z = true, v.X, v.Y, v.Z
		a.LookPos = nil
	}
	// EntityLookHelper.onUpdateLook: the pitch from 0 by at most 40, the
	// head by 10; not looking, the head back to the body by 10.
	a.Pitch = 0
	if look {
		e := Vec{a.Body.X, a.Body.Y + a.Body.EyeHeight, a.Body.Z}
		a.Pitch = turn(0, math.Atan2(y-e.Y, math.Hypot(x-e.X, z-e.Z)), 40*math.Pi/180)
		a.HeadYaw = turn(a.HeadYaw, yawTo(e, Vec{x, y, z}), 10*math.Pi/180)
	} else {
		a.HeadYaw = turn(a.HeadYaw, a.Yaw, 10*math.Pi/180)
	}
	if moving { // EntityBodyHelper, moving: the head within 75 of the body
		d := math.Max(-75*math.Pi/180, math.Min(75*math.Pi/180, math.Remainder(a.HeadYaw-a.Yaw, 2*math.Pi)))
		a.HeadYaw = a.Yaw + d
		return
	}
	m.bodyHelper(a)
}
