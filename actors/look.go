package actors

import "math"

// Look is how a CustomNPC looks around while idle (EnumStandingType, +1).
type Look uint8

const (
	LookPlayer Look = iota // engine default: watch the nearest player within 8
	LookRotate             // RotateBody: watch the closest within 5, else look around
	LookFixed              // NoRotation: keep Face
	LookStalk              // Stalking: watch the nearest player within 16, else look around
	LookHead               // HeadRotation: look around within ±54° of Face
)

// lookState is the state of CustomNPC's look tasks.
type lookState struct {
	watch     *Actor // EntityAIWatchClosest's target
	watchTime int
	idle      int // EntityAILook: ticks left looking at (x, z)
	x, z      float64
	still     int     // EntityBodyHelper.rotationTickCounter
	lastHead  float64 // EntityBodyHelper.prevRenderYawHead
}

// cnpcLook runs CustomNPC's idle look tasks for one tick:
// EntityAIWatchClosest (EntityLivingBase, 5 blocks; not for NoRotation and
// HeadRotation) above EntityAILook in the LOOK mutex, started on
// EntityAITasks' every-third-tick check; then EntityLookHelper (10° a tick,
// back to the body when not looking) and EntityBodyHelper (a still body
// turns to the head after 10 ticks).
func (m *Manager) cnpcLook(a *Actor) {
	l, b := &a.look, &a.Brain
	if b.Look == LookFixed {
		a.Yaw, a.HeadYaw = b.Face, b.Face
		return
	}
	if l.watch != nil && (!l.watch.Alive() || l.watch.Pos().dist2(a.Pos()) > 25 || l.watchTime <= 0) {
		l.watch = nil
	}
	if l.watch == nil && b.Look != LookHead && m.ticks%3 == 0 && m.Rand.Float64() < 0.002 {
		if t := m.closestLiving(a); t != nil && (m.Sees == nil || m.Sees(a, t)) {
			l.watch, l.watchTime = t, 60+m.Rand.Intn(60)
		}
	}
	want, looking := 0.0, false
	switch {
	case l.watch != nil:
		want, looking = yawTo(a.Pos(), l.watch.Pos()), true
		l.watchTime--
	case b.Look == LookStalk && m.nearestPlayer(a, 16) != nil:
		want, looking = yawTo(a.Pos(), m.nearestPlayer(a, 16).Pos()), true
	default:
		if l.idle == 0 && m.Rand.Float64() < 0.02 {
			ang := 2 * math.Pi * m.Rand.Float64()
			l.x, l.z = math.Sin(ang), -math.Cos(ang)
			if b.Look == LookHead {
				ang = b.Face + (m.Rand.Float64()*0.6-0.3)*math.Pi // ±54°
				l.x, l.z = math.Sin(ang), -math.Cos(ang)
			}
			l.idle = 20 + m.Rand.Intn(20)
		}
		if l.idle > 0 {
			l.idle--
			p := a.Pos()
			want, looking = yawTo(p, Vec{p.X + l.x, p.Y, p.Z + l.z}), true
		}
	}
	if looking {
		a.HeadYaw = turn(a.HeadYaw, want, 10*math.Pi/180)
	} else {
		a.HeadYaw = turn(a.HeadYaw, a.Yaw, 10*math.Pi/180)
	}
	// EntityBodyHelper, standing still
	f := 75.0
	if math.Abs(math.Remainder(a.HeadYaw-l.lastHead, 2*math.Pi)) > 15*math.Pi/180 {
		l.still, l.lastHead = 0, a.HeadYaw
	} else if l.still++; l.still > 10 {
		f = math.Max(1-float64(l.still-10)/10, 0) * 75
	}
	d := math.Remainder(a.Yaw-a.HeadYaw, 2*math.Pi)
	if lim := f * math.Pi / 180; d < -lim {
		d = -lim
	} else if d > lim {
		d = lim
	}
	a.Yaw = a.HeadYaw + d
}

// closestLiving is World.findNearestEntityWithinAABB(EntityLivingBase,
// box grown 5, 3, 5): the closest other living actor in that box.
func (m *Manager) closestLiving(a *Actor) *Actor {
	var t *Actor
	best := math.Inf(1)
	p := a.Pos()
	for _, o := range m.Actors {
		q := o.Pos()
		if o == a || !o.Alive() || math.Abs(q.X-p.X) > 5+a.Body.Width/2+o.Body.Width/2 || math.Abs(q.Z-p.Z) > 5+a.Body.Width/2+o.Body.Width/2 ||
			q.Y > p.Y+a.Body.Height+3 || q.Y+o.Body.Height < p.Y-3 {
			continue
		}
		if d := q.dist2(p); d < best {
			t, best = o, d
		}
	}
	return t
}

// nearestPlayer is World.getClosestPlayerToEntity(a, r).
func (m *Manager) nearestPlayer(a *Actor, r float64) *Actor {
	var t *Actor
	best := r * r
	for _, o := range m.Actors {
		if o.Player && o.Alive() {
			if d := o.Pos().dist2(a.Pos()); d < best {
				t, best = o, d
			}
		}
	}
	return t
}
