package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/nav"
)

// CNPCAI is a CustomNPC's DataAI combat response, the attack tasks
// EntityNPCInterface.setResponse adds: CanSprint EntityAISprintToTarget;
// OnAttack 0 fights (LeapType 1 EntityAILeapAtTarget(0.4), the
// TacticalVariant's task, then EntityAIAttackTarget and with a projectile
// EntityAIRangedAttack), 1 panics (EntityAIPanic(1.2)), 2 retreats
// (EntityAIAvoidTarget), 3 does nothing; before them doorInteractType's
// DoorInteract 0 EntityAIBustDoor (Manager.Doors) and seekShelter's
// FindShelter 0 EntityAIMoveIndoors, 1 EntityAIFindShade (Manager.Sky
// answers their questions). They run as EntityAITasks runs
// them: a task starts on every third tick if no running task of the same
// or a higher priority shares a mutex bit with it, and stops a running
// one of a lower priority that does.
// ponytail: no Surround (EntityAIOrbitTarget) or LeapType 2
// (EntityAIPounceTarget). Distances are from the feet, the server's posY
// (EntityPlayerMP's yOffset is 0).
type CNPCAI struct {
	OnAttack  int
	Tactic    int     // TacticalVariant, EnumNavType ordinal
	Radius    float64 // TacticalRadius
	CanSprint bool
	Leap      int // LeapType
	DirectLOS bool
	// DoorInteract: 0 busts wooden doors in its way (paths through doors,
	// opening none), 1 opens doors (Actor.OpensDoors), 2 neither.
	DoorInteract int
	// FindShelter: 0 indoors at night and in the rain, 1 into the shade by
	// day (and RestrictSun), 2 never.
	FindShelter int
}

// EnumNavType ordinals.
const (
	navDodge   = 1
	navHitNRun = 3
	navAmbush  = 4
	navStalk   = 5
)

// The tasks in priority order (taskCount order in setResponse).
const (
	cnDoor = iota
	cnShelter
	cnSprint
	cnLeap
	cnResp
	cnAttack
	cnTasks
)

// The response task kinds.
const (
	respNone = iota
	respPanic
	respAvoid
	respDodgeShoot
	respZigZag
	respAmbush
	respStalk
)

// cnpcState is the tasks' state: which run, EntityAITasks.tickCount, and
// the response task's fields.
type cnpcState struct {
	on       [cnTasks]bool
	tick     int
	inited   bool
	t        *Actor  // targetEntity / closestLivingEntity
	leap     *Actor  // EntityAILeapAtTarget.leapTarget
	goal     Vec     // the position the task walks to
	health   float64 // EntityAIAvoidTarget.health
	shelter  Vec     // EntityAIAmbushTarget.shelterX/Y/Z
	move     *Vec    // EntityAIStalkTarget.movePosition
	delay    int     // ambush and stalk delay
	wait     int     // stalk tick
	override bool    // stalk overRide
	halted   bool    // the attack's reset cleared the path (kept to walk on)
	home     Vec     // EntityAIMoveIndoors / EntityAIFindShade.shelterX/Y/Z
	door     nav.Pos // EntityAIDoorInteract.entityPosX/Y/Z
	// attacking is the isAttacking flag, reset counts CombatHandler's
	// combatResetTimer.
	attacking bool
	reset     int
	bust     int     // EntityAIBustDoor.breakingTime
	crack    int     // its field_75358_j, the last crack stage sent
}

// init is the tasks' state as made (field_75358_j starts at -1).
func (st *cnpcState) init(a *Actor) { st.inited, st.health, st.crack = true, a.Health, -1 }

func (a *Actor) cnpcProj() bool { return a.Ranged != nil && a.Ranged.AI == RangedCNPC }

func (a *Actor) cnpcResp() int {
	c := a.CNPC
	switch c.OnAttack {
	case 1:
		return respPanic
	case 2:
		return respAvoid
	case 0:
		switch c.Tactic {
		case navDodge:
			if a.cnpcProj() && a.Ranged.UseRangeMelee != 2 {
				return respDodgeShoot
			}
			return respZigZag
		case navHitNRun:
			return respAvoid
		case navAmbush:
			return respAmbush
		case navStalk:
			return respStalk
		}
	}
	return respNone
}

func (a *Actor) cnpcHas(i int) bool {
	c := a.CNPC
	switch i {
	case cnDoor:
		return c.DoorInteract == 0
	case cnShelter:
		return c.FindShelter == 0 || c.FindShelter == 1
	case cnSprint:
		return c.CanSprint
	case cnLeap:
		return c.OnAttack == 0 && c.Leap == 1
	case cnResp:
		return a.cnpcResp() != respNone
	}
	return c.OnAttack == 0
}

// cnpcMutex is each task's setMutexBits (PASSIVE 1, LOOK 2, PATHING 4).
func (a *Actor) cnpcMutex(i int) int {
	switch i {
	case cnDoor:
		return 0
	case cnShelter, cnSprint:
		return 1
	case cnLeap:
		return 5
	case cnResp:
		switch a.cnpcResp() {
		case respPanic, respDodgeShoot:
			return 1
		}
		return 3
	}
	if a.CNPC.Tactic == navNone || a.rs.override && a.cnpcProj() && a.Ranged.UseRangeMelee == 0 {
		return 4 // navOverride
	}
	return 3
}

// cnpcCanUse is EntityAITasks.canUse (every task is interruptible).
func (a *Actor) cnpcCanUse(i int) bool {
	for j := range cnTasks {
		if j != i && j <= i && a.cn.on[j] && a.cnpcMutex(i)&a.cnpcMutex(j) != 0 {
			return false
		}
	}
	return true
}

// cnpcTasks runs the tasks for one tick (EntityAITasks.onUpdateTasks) and
// then the navigator; it reports whether any task runs (the movement task
// waits then).
func (m *Manager) cnpcTasks(a *Actor) bool {
	st := &a.cn
	if !st.inited {
		st.init(a)
	}
	st.combat(a)
	var start [cnTasks]bool
	if st.tick%3 == 0 {
		for i := range cnTasks {
			if !a.cnpcHas(i) {
				continue
			}
			if st.on[i] {
				if a.cnpcCanUse(i) && m.cnpcCont(a, i) {
					continue
				}
				m.cnpcReset(a, i)
				st.on[i] = false
			}
			if a.cnpcCanUse(i) && m.cnpcShould(a, i) {
				st.on[i], start[i] = true, true
			}
		}
	} else {
		for i := range cnTasks {
			if st.on[i] && !m.cnpcCont(a, i) {
				m.cnpcReset(a, i)
				st.on[i] = false
			}
		}
	}
	st.tick++
	for i := range cnTasks {
		if start[i] {
			m.cnpcStart(a, i)
		}
	}
	busy := false
	for i := range cnTasks {
		if st.on[i] {
			m.cnpcUpdate(a, i)
			busy = true
		}
	}
	if !st.on[cnAttack] || a.cnpcMutex(cnAttack) == 4 { // PathNavigate.onUpdateNavigation
		if a.rs.goalSet {
			m.walk(a)
		} else {
			c := a.carrier()
			c.nav.in = character.Input{Yaw: c.Yaw, Jump: c.Body.InWater && c.CanSwim}
		}
	}
	return busy
}

// combat is CombatHandler.update (run before the tasks, as the previous
// tick's onUpdate ran it after them): attacking from a target until 40
// ticks after the last tick the target was valid (alive and within the
// aggro range); killed, it stops.
// ponytail: a creative player stays a valid target; the game's target
// pick leaves creative players out.
func (st *cnpcState) combat(a *Actor) {
	if !a.Alive() {
		st.attacking, st.reset = false, 0
		return
	}
	t := a.Target
	if t != nil && !st.attacking {
		st.attacking, st.reset = true, 0
	}
	if t != nil && t.Alive() && a.inRange(t, a.AggroRange) {
		st.reset = 0
	} else if st.reset++; st.reset > 41 {
		st.attacking, st.reset = false, 0
	}
}

// CNPCAttacking is a CustomNPC's isAttacking (CombatHandler's flag).
func (a *Actor) CNPCAttacking() bool { return a.cn.attacking }

// cnpcFlip is EntityNPCInterface.attackEntityFrom's choice of target when
// src hurts a: one not attacking takes src, one attacking (combat policy
// Flip) turns on src when src is the nearer.
// ponytail: no Brute, Stubborn or Tactical policy (TacticalChance): every
// map NPC has Flip.
func (m *Manager) cnpcFlip(a, src *Actor) {
	if a.Target == nil || a.Pos().dist2(a.Target.Pos()) > a.Pos().dist2(src.Pos()) {
		a.Target = src
	}
}

// pathing is !PathNavigate.noPath. The attack's reset (clearPathEntity)
// only halts the walker: the path it keeps is walked on when the attack
// starts again, as the reference's path is found at once in its
// shouldExecute (a search here takes ticks).
func (a *Actor) pathing() bool { return a.rs.goalSet || a.nav.has && !a.cn.halted }

// inRange is EntityNPCInterface.isInRange: a box of range around a.
func (a *Actor) inRange(t *Actor, r float64) bool {
	return math.Abs(a.Body.Y-t.Body.Y) <= r && math.Abs(a.Body.X-t.Body.X) <= r && math.Abs(a.Body.Z-t.Body.Z) <= r
}

// moving is motionX and motionZ both not 0 (onLivingUpdate zeroes a
// motion below 0.005).
func (a *Actor) moving() bool { return math.Abs(a.Body.VX) >= 0.005 && math.Abs(a.Body.VZ) >= 0.005 }

func (m *Manager) cnpcShould(a *Actor, i int) bool {
	st, c, t := &a.cn, a.CNPC, a.Target
	switch i {
	case cnDoor:
		return m.bustShould(a)
	case cnShelter:
		p, ok := m.shelterShould(a)
		st.home = p
		return ok
	case cnSprint:
		if t == nil || !a.pathing() || !a.Body.OnGround {
			return false
		}
		switch c.OnAttack {
		case 0:
			return !a.inRange(t, 8)
		case 2:
			return a.inRange(t, 7)
		}
		return false
	case cnLeap:
		if t == nil {
			return false
		}
		if d := a.Pos().dist2(t.Pos()); d < 4 || d > 16 || !a.Body.OnGround {
			return false
		}
		st.leap = t
		return m.Rand.Intn(5) == 0
	case cnAttack:
		return t != nil && t.Alive()
	}
	switch a.cnpcResp() {
	case respPanic:
		if t == nil && !a.OnFire() {
			return false
		}
		p, ok := m.randomTarget(a, 5, 4, nil)
		st.goal = p
		return ok
	case respAvoid:
		return m.avoidShould(a)
	case respDodgeShoot:
		if t == nil || !t.Alive() || !a.cnpcProj() || !a.rs.fired {
			return false
		}
		p, ok := m.randomTarget(a, 4, 1, nil)
		st.goal = p
		return ok
	case respZigZag:
		return m.zigZagShould(a)
	case respAmbush:
		if t == nil {
			return false
		}
		if t.Pos().dist2(a.Pos()) < c.Radius*c.Radius || !m.sees(a, t) {
			return false
		}
		if st.delay > 0 {
			st.delay--
			return false
		}
		p, ok := m.hidingSpot(a, t)
		if !ok {
			return false
		}
		st.shelter, st.t = p, t
		return true
	case respStalk:
		return m.stalkShould(a)
	}
	return false
}

func (m *Manager) cnpcCont(a *Actor, i int) bool {
	st, c := &a.cn, a.CNPC
	switch i {
	case cnDoor: // EntityAIBustDoor.continueExecuting
		d := st.door
		dx, dy, dz := a.Body.X-float64(d.X), a.Body.Y-float64(d.Y), a.Body.Z-float64(d.Z)
		_, open := m.W.Door(d.X, d.Y, d.Z)
		return st.bust <= 240 && !open && dx*dx+dy*dy+dz*dz < 4
	case cnShelter:
		return a.pathing()
	case cnSprint:
		return a.Alive() && a.Body.OnGround && a.HurtTime <= 0 && a.moving()
	case cnLeap:
		return !a.Body.OnGround
	case cnAttack:
		t := a.Target
		if t == nil || !t.Alive() {
			return false
		}
		return a.cnpcProj() || a.Pos().dist2(t.Pos()) <= a.AggroRange*a.AggroRange
	}
	switch a.cnpcResp() {
	case respPanic:
		return a.Target != nil && a.rs.goalSet
	case respAvoid, respDodgeShoot:
		return a.rs.goalSet
	case respZigZag:
		return a.rs.goalSet && st.t.Alive() && st.t.Pos().dist2(a.Pos()) > c.Radius*c.Radius
	case respAmbush:
		hide := st.t.Pos().dist2(a.Pos()) > c.Radius*c.Radius
		seen := m.sees(a, st.t)
		return a.rs.goalSet && hide || !seen && (hide || c.DirectLOS)
	case respStalk:
		return m.stalkShould(a) // EntityAIBase.continueExecuting
	}
	return false
}

func (m *Manager) cnpcStart(a *Actor, i int) {
	st := &a.cn
	switch i {
	case cnDoor:
		st.bust = 0
		return
	case cnShelter:
		m.setPath(a, st.home, 1)
		return
	case cnSprint:
		a.Sprinting = true
		return
	case cnLeap:
		t, b := st.leap, a.Body
		dx, dz := t.Body.X-b.X, t.Body.Z-b.Z
		f := float64(float32(math.Sqrt(dx*dx + dz*dz)))
		b.VX += dx/f*0.5*0.800000011920929 + b.VX*0.20000000298023224
		b.VZ += dz/f*0.5*0.800000011920929 + b.VZ*0.20000000298023224
		b.VY = float64(float32(0.4))
		return
	case cnAttack:
		st.halted = false
		return
	}
	switch a.cnpcResp() {
	case respPanic, respDodgeShoot, respZigZag:
		m.setPath(a, st.goal, 1.2)
	case respAvoid:
		m.setPath(a, st.goal, 1)
	case respAmbush:
		m.setPath(a, st.shelter, 1.2)
	case respStalk:
		if a.cnpcProj() {
			a.rs.override = true
		}
	}
}

func (m *Manager) cnpcReset(a *Actor, i int) {
	st, c := &a.cn, a.CNPC
	switch i {
	case cnDoor:
		m.Doors.Crack(a, st.door.X, st.door.Y, st.door.Z, -1)
		return
	case cnShelter:
		return
	case cnSprint:
		a.Sprinting = false
		return
	case cnLeap:
		return
	case cnAttack: // EntityAIAttackTarget / EntityAIRangedAttack.resetTask
		a.Target, a.rs.goalSet, st.halted = nil, false, true
		m.rangedReset(a)
		return
	}
	switch a.cnpcResp() {
	case respAvoid:
		st.t, a.Target = nil, nil
	case respAmbush:
		m.clearPath(a)
		if a.Target == nil && st.t != nil {
			a.Target = st.t
		}
		if st.t.Pos().dist2(a.Pos()) < c.Radius*c.Radius {
			st.delay = 60
		}
	case respStalk:
		m.clearPath(a)
		if a.Target == nil && st.t != nil {
			a.Target = st.t
		}
		a.rs.override = false
	}
}

func (m *Manager) cnpcUpdate(a *Actor, i int) {
	st, c := &a.cn, a.CNPC
	switch i {
	case cnDoor: // EntityAIBustDoor.updateTask
		d := st.door
		if m.Rand.Intn(20) == 0 {
			m.Doors.Hit(a, d.X, d.Y, d.Z)
			a.Swing = 0
		}
		st.bust++
		if s := int(float32(st.bust) / 240 * 10); s != st.crack {
			m.Doors.Crack(a, d.X, d.Y, d.Z, s)
			st.crack = s
		}
		if st.bust == 240 {
			m.Doors.Bust(a, d.X, d.Y, d.Z)
		}
		return
	case cnShelter, cnSprint, cnLeap:
		return
	case cnAttack:
		if t := a.Target; t != nil {
			if a.cnpcProj() {
				m.cnpcFight(a, t)
			} else {
				m.melee(a, t)
			}
		}
		return
	}
	switch a.cnpcResp() {
	case respAvoid:
		d2 := a.Pos().dist2(st.t.Pos())
		a.rs.goalMul = 1
		if d2 < 49 {
			a.rs.goalMul = 1.2
		}
		if c.Tactic == navHitNRun {
			if d := float64(float32(math.Sqrt(d2))); d > a.AggroRange || d < c.Radius {
				st.health = a.Health
			}
		}
	case respDodgeShoot:
		if a.Target != nil {
			m.face(a, a.Target)
		}
	case respZigZag, respAmbush:
		m.face(a, st.t)
	case respStalk:
		m.stalkUpdate(a)
	}
}

// bustShould is EntityAIBustDoor.shouldExecute: blocked by something, a
// closed wooden door one up from the path's cells up to two past the next
// within 1.5 (horizontally), else at its own head.
// It needs Griefing.
func (m *Manager) bustShould(a *Actor) bool {
	b, n := a.Body, &a.nav
	if m.Doors == nil || !m.Griefing || !b.HorizontalCollision || n.path == nil || n.i >= len(n.path) {
		return false
	}
	found := false
	for i := 0; i < min(n.i+2, len(n.path)); i++ {
		p := n.path[i]
		dx, dz := b.X-float64(p.X), b.Z-float64(p.Z)
		if dx*dx+dz*dz <= 2.25 && m.Doors.Bustable(p.X, p.Y+1, p.Z) {
			a.cn.door, found = nav.Pos{X: p.X, Y: p.Y + 1, Z: p.Z}, true
			break
		}
	}
	if !found {
		q := nav.Pos{X: int(math.Floor(b.X)), Y: int(math.Floor(b.Y + 1)), Z: int(math.Floor(b.Z))}
		if !m.Doors.Bustable(q.X, q.Y, q.Z) {
			return false
		}
		a.cn.door = q
	}
	_, open := m.W.Door(a.cn.door.X, a.cn.door.Y, a.cn.door.Z)
	return !open
}

// shelterShould is EntityAIMoveIndoors.shouldExecute (FindShelter 0: at
// night or in the rain, unless standing under a roof in light above 8, ten
// tries for such a block 10 around and 3 up or down) and
// EntityAIFindShade's (1: by day under the open sky, ten tries for a block
// under cover darker than brightness 0.5).
func (m *Manager) shelterShould(a *Actor) (Vec, bool) {
	w := m.Sky
	if w == nil {
		return Vec{}, false
	}
	b := a.Body
	x, y, z := int(math.Floor(b.X)), int(b.Y), int(math.Floor(b.Z)) // (int)boundingBox.minY
	ok := func(x, y, z int) bool { return !w.SeesSky(x, y, z) && w.FullLight(x, y, z) > 8 }
	if a.CNPC.FindShelter == 1 {
		if !w.Daytime() || !w.SeesSky(x, y, z) {
			return Vec{}, false
		}
		ok = func(x, y, z int) bool { return !w.SeesSky(x, y, z) && w.Brightness(x, y, z)-0.5 < 0 }
	} else if w.Daytime() && !w.Raining() || ok(x, y, z) {
		return Vec{}, false
	}
	for range 10 {
		i := int(math.Floor(b.X + float64(m.Rand.Intn(20)) - 10))
		j := int(math.Floor(b.Y + float64(m.Rand.Intn(6)) - 3))
		k := int(math.Floor(b.Z + float64(m.Rand.Intn(20)) - 10))
		if ok(i, j, k) {
			return Vec{float64(i), float64(j), float64(k)}, true
		}
	}
	return Vec{}, false
}

// avoidShould is EntityAIAvoidTarget.shouldExecute: the nearest player
// within the aggro range if the target is a player, else the first of the
// target's kind within it (3 up and down); a spot 16 away from it, not
// nearer to it; HitNRun only once hurt since the last time it was far or
// near (melee) or once a burst was shot; and a path that ends there.
func (m *Manager) avoidShould(a *Actor) bool {
	st, c, t := &a.cn, a.CNPC, a.Target
	if t == nil {
		return false
	}
	r := a.AggroRange
	var o *Actor
	if t.Player {
		bd := r * r
		for _, p := range m.Actors {
			if d := p.Pos().dist2(a.Pos()); p.Player && p != a && d < bd {
				o, bd = p, d
			}
		}
	} else {
		box := a.Box().Grow(r, 3, r)
		for _, p := range m.Actors {
			if p.Kind == t.Kind && p.Box().Intersects(box) {
				o = p
				break
			}
		}
	}
	if o == nil || !m.sees(a, o) && c.DirectLOS {
		return false
	}
	p, ok := m.randomTarget(a, 16, 7, &Vec{a.Body.X - o.Body.X, a.Body.Y - o.Body.Y, a.Body.Z - o.Body.Z})
	var hold bool
	if !a.cnpcProj() || a.Ranged.UseRangeMelee == 2 {
		hold = st.health == a.Health
	} else {
		hold = c.OnAttack == 0 && !a.rs.fired
	}
	if !ok || o.Pos().dist2(p) < o.Pos().dist2(a.Pos()) || c.Tactic == navHitNRun && hold || !m.PathEnds(a, p) {
		return false
	}
	st.t, st.goal = o, p
	return true
}

// zigZagShould is EntityAIZigZagTarget.shouldExecute: a target beyond the
// radius with a path at least that long; a random spot toward the path's
// point at half the radius that is nearer the target than that point.
func (m *Manager) zigZagShould(a *Actor) bool {
	st, c, t := &a.cn, a.CNPC, a.Target
	if t == nil || t.Pos().dist2(a.Pos()) < c.Radius*c.Radius {
		return false
	}
	g := m.graphFor(a)
	path, ok := g.Path(m.start(g, a.Pos()), m.start(g, t.Pos()), maxNodes)
	if !ok || float64(len(path)) < c.Radius {
		return false
	}
	pp := path[int(math.Floor(c.Radius/2))]
	pv := Vec{float64(pp.X), float64(pp.Y), float64(pp.Z)}
	p, ok := m.randomTarget(a, int(c.Radius), 3, &Vec{pv.X - a.Body.X, pv.Y - a.Body.Y, pv.Z - a.Body.Z})
	if !ok || t.Pos().dist2(p) >= t.Pos().dist2(pv) {
		return false
	}
	st.t, st.goal = t, p
	return true
}

// randomTarget is RandomPositionGeneratorAlt.findRandomTargetBlock: of 10
// random cells within xz and y (on dir's side), the one of the highest
// path weight.
func (m *Manager) randomTarget(a *Actor, xz, y int, dir *Vec) (Vec, bool) {
	xz, y = max(xz, 1), max(y, 1)
	best := float32(-99999)
	var p Vec
	found := false
	for range 10 {
		j := m.Rand.Intn(2*xz) - xz
		k := m.Rand.Intn(2*y) - y
		l := m.Rand.Intn(2*xz) - xz
		if dir != nil && float64(j)*dir.X+float64(l)*dir.Z < 0 {
			continue
		}
		rnd := math.Floor
		if m.Rand.Intn(2) == 0 {
			rnd = math.Ceil
		}
		x, yy, z := j+int(rnd(a.Body.X)), k+int(rnd(a.Body.Y)), l+int(rnd(a.Body.Z))
		var w float32
		if m.PathWeight != nil {
			w = m.PathWeight(a, x, yy, z)
		}
		if w > best {
			best, p, found = w, Vec{float64(x), float64(yy), float64(z)}, true
		}
	}
	return p, found
}

// cover reports a spot on an opaque cube under two cells that are not
// that a ray from t's eyes to a's eyes there meets a block on.
func (m *Manager) cover(a, t *Actor, j, k, l float64) bool {
	x, y, z := int(j), int(k), int(l)
	if m.Opaque == nil || m.RayHits == nil || !m.Opaque(x, y, z) || m.Opaque(x, y+1, z) || m.Opaque(x, y+2, z) {
		return false
	}
	return m.RayHits(Vec{t.Body.X, t.Body.Y + t.Body.EyeHeight, t.Body.Z}, Vec{j, k + a.Body.EyeHeight, l})
}

// hidingSpot is EntityAIAmbushTarget.findHidingSpot: in the nearest ring
// up to 8 with one, the last spot in cover of the target (one sharing no
// coordinate with the shelter before); none waits 60.
func (m *Manager) hidingSpot(a, t *Actor) (Vec, bool) {
	st := &a.cn
	minY := a.Box().MinY
	for i := 1; i <= 8; i++ {
		var p Vec
		found := false
		for y := -2; y <= 2; y++ {
			for x := -i; x <= i; x++ {
				for z := -i; z <= i; z++ {
					j := math.Floor(a.Body.X+float64(x)) + 0.5
					k := math.Floor(minY + float64(y))
					l := math.Floor(a.Body.Z+float64(z)) + 0.5
					if m.cover(a, t, j, k, l) && st.shelter.X != j && st.shelter.Y != k && st.shelter.Z != l {
						p, found = Vec{j, k, l}, true
					}
				}
			}
		}
		if found {
			return p, true
		}
	}
	st.delay = 60
	return Vec{}, false
}

func (m *Manager) stalkShould(a *Actor) bool {
	st, c, t := &a.cn, a.CNPC, a.Target
	if t == nil {
		return false
	}
	if st.wait > 0 {
		st.wait--
		return false
	}
	st.t = t
	return t.Pos().dist2(a.Pos()) > c.Radius*c.Radius
}

// stalkUpdate is EntityAIStalkTarget.updateTask: with the navigator idle
// or overridden, it creeps to the covered spot nearest the target while
// the target looks away, else hides in the nearest cover when seen; seen
// while looked at, it moves on (every 60 ticks at most).
func (m *Manager) stalkUpdate(a *Actor) {
	st, t := &a.cn, a.cn.t
	m.face(a, t)
	if !a.rs.goalSet || st.override {
		if m.lookingAway(a, t) {
			if p, ok := m.secluded(a, t, true); ok {
				st.move = &p
				m.setPath(a, p, 1)
				st.override = false
			} else {
				st.wait = 100
			}
		} else if m.sees(a, t) {
			if p, ok := m.secluded(a, t, false); ok {
				st.move = &p
				m.setPath(a, p, 1.33)
				st.override = false
			} else {
				st.wait = 100
			}
		}
	}
	if st.delay > 0 {
		st.delay--
	}
	if !m.lookingAway(a, t) && m.sees(a, t) && st.delay == 0 {
		st.override, st.delay = true, 60
	}
}

// secluded is stalkTarget (nearest: radius 8 down to 1) or hideFromTarget
// (1 up to 8): the first radius with a findSecludedXYZ spot.
func (m *Manager) secluded(a, t *Actor, nearest bool) (Vec, bool) {
	for n := 1; n <= 8; n++ {
		r := n
		if nearest {
			r = 9 - n
		}
		if p, ok := m.secludedXYZ(a, t, r, nearest); ok {
			return p, true
		}
	}
	return Vec{}, false
}

// secludedXYZ is findSecludedXYZ: the last spot in cover of t within r
// (nearest: each no farther from t than the one before) that is not the
// last move's.
func (m *Manager) secludedXYZ(a, t *Actor, r int, nearest bool) (Vec, bool) {
	st := &a.cn
	dist := t.Pos().dist2(a.Pos())
	var u Vec
	if st.move != nil {
		u = *st.move
	}
	var p Vec
	found := false
	minY := a.Box().MinY
	for y := -2; y <= 2; y++ {
		for x := -r; x <= r; x++ {
			for z := -r; z <= r; z++ {
				j := math.Floor(a.Body.X+float64(x)) + 0.5
				k := math.Floor(minY + float64(y))
				l := math.Floor(a.Body.Z+float64(z)) + 0.5
				if !m.cover(a, t, j, k, l) {
					continue
				}
				q := Vec{j, k, l}
				if (!nearest || t.Pos().dist2(q) <= dist) && q != u {
					p, found = q, true
					if nearest {
						dist = t.Pos().dist2(q)
					}
				}
			}
		}
	}
	return p, found
}

// lookingAway: the angle between t's look and the way to a's middle has a
// cosine under 0.6.
func (m *Manager) lookingAway(a, t *Actor) bool {
	cp := math.Cos(t.Pitch)
	lx, ly, lz := math.Sin(t.Yaw)*cp, math.Sin(t.Pitch), -math.Cos(t.Yaw)*cp
	vx := a.Body.X - t.Body.X
	vy := a.Box().MinY + a.Body.Height/2 - (t.Body.Y + t.Body.EyeHeight)
	vz := a.Body.Z - t.Body.Z
	n := math.Sqrt(vx*vx + vy*vy + vz*vz)
	if n < 1e-4 {
		return true // Vec3.normalize of a zero vector is zero
	}
	return (lx*vx+ly*vy+lz*vz)/n < 0.6
}
