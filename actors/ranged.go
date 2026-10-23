package actors

import "math"

// Ranged AI kinds (Ranged.AI).
const (
	RangedSkeleton = iota // 1.7.10 EntityAIArrowAttack (the default)
	RangedLOTR            // LOTR LOTREntityAIRangedAttack with LOTREntityNPC's attack modes
	RangedCNPC            // CustomNPC EntityAIRangedAttack with EntityAIAttackTarget
)

// LOTR attack modes (LOTREntityNPC.AttackMode).
const (
	modeIdle = iota
	modeMelee
	modeRanged
)

// rangedState is the running ranged task's state.
type rangedState struct {
	on        bool // the task is executing (resetTask runs when it stops)
	burst     int  // CNPC field_70846_g: shots of the current burst
	mode      int  // LOTR currentAttackMode
	goal      Vec  // the navigator's path end (PathNavigate keeps walking it)
	goalMul   float64
	goalSet   bool
	flee      bool
	navLocked bool // CNPC navOverride: the task leaves the navigator alone
	inited    bool // the task's constructor ran (its timer's first value)
}

// rangedInit gives the timer the task constructor's value: LOTR -1, CNPC
// minDelay/2.
func (a *Actor) rangedInit() {
	if a.rs.inited {
		return
	}
	a.rs.inited = true
	a.rangedCool = -1
	if a.Ranged.AI == RangedCNPC {
		a.rangedCool = a.Ranged.MinInterval / 2
	}
}

func (m *Manager) sees(a, t *Actor) bool {
	if m.Sees == nil {
		return true
	}
	return m.Sees(a, t)
}

// walk keeps a on the path set by setPath, as PathNavigate does between
// AI calls: it reports whether a path is still being walked.
func (m *Manager) walk(a *Actor) bool {
	r := &a.rs
	if !r.goalSet {
		m.stop(a)
		return false
	}
	if m.moveTo(a, r.goal, r.goalMul) {
		r.goalSet = false
	}
	return r.goalSet
}

func (m *Manager) setPath(a *Actor, goal Vec, mul float64) {
	a.rs.goal, a.rs.goalMul, a.rs.goalSet = goal, mul, true
}

func (m *Manager) clearPath(a *Actor) {
	a.rs.goalSet = false
	m.stop(a)
}

// rangedIdle is the tick of an actor without a target: the running ranged
// task stops (resetTask) and LOTR NPCs go back to the idle mode.
func (m *Manager) rangedIdle(a *Actor) {
	r := a.Ranged
	if r == nil {
		return
	}
	if r.AI == RangedLOTR {
		a.rs.mode = modeIdle
	}
	m.rangedReset(a)
}

func (m *Manager) rangedReset(a *Actor) {
	r := a.Ranged
	if !a.rs.on {
		return
	}
	a.rs.on = false
	switch r.AI {
	case RangedLOTR: // LOTREntityAIRangedAttack.resetTask
		a.seeTime, a.rangedCool = 0, -1
	case RangedCNPC: // EntityAIRangedAttack.resetTask: also drops the target
		a.Target = nil
		m.clearPath(a)
		a.seeTime, a.rangedCool = 0, r.MinInterval/2
	}
}

// lotrFight is a LOTR NPC's fight: LOTREntityNPC.onLivingUpdate picks the
// attack mode (melee closer than getMeleeRange, ranged within 0.95 of the
// follow range, else as before), and a class with both tasks swaps them
// with the mode (onAttackModeChange); an archer has the ranged task only.
func (m *Manager) lotrFight(a, t *Actor) {
	r := a.Ranged
	w := a.Body.Width
	mr := 4 + w*w
	if a.Riding != nil {
		mr *= 1.5 // MOUNT_RANGE_BONUS
	}
	d2 := a.Pos().dist2(t.Pos())
	far := a.AggroRange * 0.95
	if d2 >= mr*mr {
		if d2 < far*far {
			a.rs.mode = modeRanged
		}
	} else {
		a.rs.mode = modeMelee
	}
	if r.Switch {
		switch a.rs.mode {
		case modeMelee:
			m.rangedReset(a)
			m.melee(a, t)
			return
		case modeIdle:
			m.rangedReset(a)
			return
		}
	}
	m.lotrRanged(a, t)
}

// lotrRanged is LOTREntityAIRangedAttack.updateTask.
func (m *Manager) lotrRanged(a, t *Actor) {
	r := a.Ranged
	a.rangedInit()
	a.rs.on = true
	tb := t.Body.Box()
	d2 := a.Pos().dist2(Vec{t.Body.X, tb.MinY, t.Body.Z})
	see := m.sees(a, t)
	if see {
		a.seeTime++
	} else {
		a.seeTime = 0
	}
	rng := r.MaxRange
	switch {
	case d2 > rng*rng:
		m.setPath(a, t.Pos(), r.MoveSpeed)
	case a.Pos().dist2(t.Pos()) < 25:
		if p, ok := m.awayFrom(a, t, 8, 16); ok {
			m.setPath(a, p, 1.5)
		}
	case a.seeTime >= 20:
		m.clearPath(a)
		a.seeTime = 0
	}
	m.walk(a)
	m.face(a, t)
	a.rangedCool--
	f := float64(float32(math.Sqrt(d2)) / float32(rng))
	next := func() int {
		return int(math.Floor(float64(float32(f)*float32(r.MaxInterval-r.MinInterval) + float32(r.MinInterval))))
	}
	switch {
	case a.rangedCool == 0:
		if d2 > rng*rng || !see {
			return
		}
		m.fire(a, t, math.Max(0.1, math.Min(1, f)))
		a.rangedCool = next()
	case a.rangedCool < 0:
		a.rangedCool = next()
	}
}

// awayFrom is LOTREntityAIRangedAttack.findPositionAwayFrom: up to 24
// random cells around a whose squared distance to t lies in (min², max²).
func (m *Manager) awayFrom(a, t *Actor, lo, hi int) (Vec, bool) {
	b := a.Body.Box()
	ax, ay, az := int(math.Floor(a.Body.X)), int(math.Floor(b.MinY)), int(math.Floor(a.Body.Z))
	for range 24 {
		p := Vec{float64(ax - hi + m.Rand.Intn(hi*2+1)), float64(ay - 4 + m.Rand.Intn(9)), float64(az - hi + m.Rand.Intn(hi*2+1))}
		if d := t.Pos().dist2(p); d > float64(lo*lo) && d < float64(hi*hi) {
			return p, true
		}
	}
	return Vec{}, false
}

// cnpcFight runs a CustomNPC's attack tasks (EntityNPCInterface
// .setResponse with a projectile): EntityAIRangedAttack, and
// EntityAIAttackTarget unless RangeAndMelee is 0. RangeAndMelee 1 melees
// within DistanceToMelee and shoots beyond; 2 always runs the melee task
// (which then owns the navigator) and shoots beyond DistanceToMelee.
func (m *Manager) cnpcFight(a, t *Actor) {
	r := a.Ranged
	tb := t.Body.Box()
	d2 := a.Pos().dist2(Vec{t.Body.X, tb.MinY, t.Body.Z})
	dtm2 := r.DistanceToMelee * r.DistanceToMelee
	inMelee := d2 <= dtm2
	shoot := math.Sqrt(a.Pos().dist2(t.Pos())) <= a.AggroRange && !(r.UseRangeMelee >= 1 && inMelee)
	// continueExecuting: shouldExecute or the navigator still has a path.
	if !shoot && !(a.rs.on && a.rs.goalSet) {
		m.rangedReset(a)
	} else {
		a.rs.navLocked = r.Tactic == navNone || (r.UseRangeMelee == 2 && !inMelee)
		m.cnpcRanged(a, t)
	}
	if a.Target == t && r.UseRangeMelee != 0 && (r.UseRangeMelee != 1 || inMelee) {
		m.melee(a, t)
	}
}

const navNone = 6 // EnumNavType.None

// cnpcRanged is CustomNPC EntityAIRangedAttack.updateTask.
func (m *Manager) cnpcRanged(a, t *Actor) {
	r := a.Ranged
	a.rangedInit()
	a.rs.on = true
	m.face(a, t)
	tb := t.Body.Box()
	d2 := a.Pos().dist2(Vec{t.Body.X, tb.MinY, t.Body.Z})
	rng2 := r.MaxRange * r.MaxRange
	see := m.sees(a, t)
	if !a.rs.navLocked && r.DirectLOS {
		if see {
			a.seeTime++
		} else {
			a.seeTime = 0
		}
		v := 5
		if r.Tactic == 0 {
			v = 20
		}
		if d2 <= rng2 && a.seeTime >= v {
			m.clearPath(a)
		} else {
			m.setPath(a, t.Pos(), 1)
		}
	}
	if !a.rs.navLocked {
		m.walk(a)
	}
	a.rangedCool = max(a.rangedCool-1, 0)
	if a.rangedCool > 0 || d2 > rng2 || !(see || r.Indirect == 2) {
		return
	}
	a.rs.burst++
	if a.rs.burst-1 <= r.Burst {
		a.rangedCool = r.FireRate
	} else {
		a.rs.burst = 0
		a.rangedCool = r.MaxInterval - int(math.Floor(float64(m.Rand.Float32()*float32(r.MaxInterval-r.MinInterval))))
	}
	if a.rs.burst > 1 {
		indirect := false
		switch r.Indirect {
		case 1:
			indirect = d2 > rng2/2
		case 2:
			indirect = !see
		}
		f := 0.0
		if indirect {
			f = 1
		}
		m.fire(a, t, f)
	}
}
