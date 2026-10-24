package actors

import (
	"math"
	"slices"
	"testing"
)

// TestCNPCWander: a custom NPC wanderer walks (never jumping a block in a
// tick) and keeps near its walking range around the start; with
// NPCInteract it walks to another custom NPC, which waits, and both then
// face each other as interacting; one interacting does not wander, and a
// player's interaction stops it.
func TestCNPCWander(t *testing.T) {
	m := NewManager(flat(60), nil, nil, 1)
	a := New("a", "npc", 20.5, 1, 20.5)
	a.CNPC, a.PersistReq = &CNPCAI{OnAttack: 3, FindShelter: 2, StopInteract: true}, true
	a.Brain = Brain{Mode: Wander, Home: a.Pos(), Range: 5, Look: LookRotate}
	m.Add(a)
	moved, far, x, z := false, 0.0, a.Body.X, a.Body.Z
	for range 3000 {
		m.Tick()
		if dx, dz := a.Body.X-x, a.Body.Z-z; dx*dx+dz*dz > 0.25 {
			t.Fatal("moved more than half a block in a tick")
		}
		x, z = a.Body.X, a.Body.Z
		far = max(far, a.Pos().hdist(a.Brain.Home))
		moved = moved || far > 1
	}
	if !moved || far > 6.5 {
		t.Fatalf("wander reach %.2f", far)
	}

	b := New("b", "npc", a.Body.X+4, 1, a.Body.Z)
	b.CNPC, b.PersistReq = &CNPCAI{OnAttack: 3, FindShelter: 2, StopInteract: true}, true
	b.Brain = Brain{Mode: Stand, Home: b.Pos(), Look: LookRotate}
	m.Add(b)
	a.CNPC.NPCInteract = true
	said := 0
	m.CNPCSay = func(sp *Actor, text string, hide bool) {
		if (sp == a || sp == b) && text == "........." && hide {
			said++
		}
	}
	met := false
	for range 6000 {
		m.Tick()
		if len(b.cn.with) == 1 && b.cn.with[0] == a && len(a.cn.with) == 1 && a.cn.with[0] == b && a.Pos().dist2(b.Pos()) < 12 {
			met = true
			break
		}
	}
	if !met || !a.cnpcInteracting() || !b.cnpcInteracting() {
		t.Fatalf("no meeting: %v, a at %.1f %.1f, b at %.1f %.1f", met, a.Body.X, a.Body.Z, b.Body.X, b.Body.Z)
	}
	if said != 1 {
		t.Fatalf("the meeting line said %d times", said)
	}
	if m.wanderShould(a) {
		t.Fatal("wanders while interacting")
	}
	for range 20 {
		m.Tick()
	}
	if d := math.Remainder(yawTo(a.Pos(), b.Pos())-a.HeadYaw, 2*math.Pi); d*d > 0.05 {
		t.Fatalf("not facing the one met (%.2f)", d)
	}

	p := New("p", "player", a.Body.X+1, 1, a.Body.Z)
	p.Player = true
	m.Add(p)
	for range 400 {
		m.Tick()
	}
	a.cn.attacking = false
	m.setPath(a, Vec{a.Body.X + 5, 1, a.Body.Z}, 1)
	m.CNPCInteract(a, p)
	if a.pathing() || !a.cnpcInteracting() || !slices.Contains(a.cn.with, p) {
		t.Fatal("a player's interaction did not stop it")
	}

	// the one met turning unmeetable on the way: the walk goes on
	c := New("c", "npc", 40.5, 1, 40.5)
	c.CNPC, c.PersistReq = &CNPCAI{OnAttack: 3, StopInteract: true, NPCInteract: true}, true
	c.Brain = Brain{Mode: Wander, Home: c.Pos(), Range: 8, Look: LookRotate}
	d := New("d", "npc", 46.5, 1, 40.5)
	d.CNPC, d.PersistReq = &CNPCAI{OnAttack: 3, StopInteract: true}, true
	d.Brain = Brain{Mode: Stand, Home: d.Pos(), Look: LookRotate}
	m.Add(c)
	m.Add(d)
	c.Ticks, d.Ticks = 1000, 1000
	c.cn.near, c.cn.wgoal = d, Vec{46, 1, 40}
	c.cn.on[cnWander] = true
	m.cnpcStart(c, cnWander)
	for range 6 {
		m.Tick()
	}
	d.cn.attacking = true
	for range 6 {
		m.Tick()
	}
	if c.cn.on[cnWander] || c.cn.near != nil {
		t.Fatal("the wander went on with the one met attacking")
	}
	x0 := c.Body.X
	for range 20 {
		m.Tick()
	}
	if c.Body.X-x0 < 1 {
		t.Fatalf("the walk stopped (%.2f)", c.Body.X-x0)
	}
}

// TestCNPCRoutePause: a custom NPC on a route (EntityAIMovingPath) stands
// while interacting and then walks to the same point.
func TestCNPCRoutePause(t *testing.T) {
	m := NewManager(flat(60), nil, nil, 1)
	a := New("a", "npc", 10.5, 1, 10.5)
	a.CNPC, a.PersistReq = &CNPCAI{OnAttack: 3, StopInteract: true}, true
	a.Brain = Brain{Mode: Route, Points: []Vec{{10.5, 1, 10.5}, {30.5, 1, 10.5}}, Next: 1, Look: LookRotate}
	p := New("p", "player", 10.5, 1, 12.5)
	p.Player = true
	m.Add(a)
	m.Add(p)
	a.Ticks = 1000
	for range 10 {
		m.Tick()
	}
	m.CNPCInteract(a, p)
	for range 39 {
		m.Tick()
	}
	x0 := a.Body.X
	for range 100 {
		m.Tick()
	}
	if math.Abs(a.Body.X-x0) > 0.01 || a.Brain.Next != 1 {
		t.Fatalf("walked while interacting (%.2f, point %d)", a.Body.X-x0, a.Brain.Next)
	}
	for range 100 {
		m.Tick()
	}
	if a.Body.X-x0 < 2 {
		t.Fatal("did not walk on after the interaction")
	}
}

// TestCNPCWorldLines: EntityAIWorldLines rolls 1 in 1800 at each task
// check (every third tick), so about once in 5400 ticks, and the wanderer
// still wanders.
func TestCNPCWorldLines(t *testing.T) {
	m := NewManager(flat(60), nil, nil, 1)
	a := New("a", "npc", 20.5, 1, 20.5)
	a.CNPC, a.PersistReq = &CNPCAI{OnAttack: 3, FindShelter: 2, WorldLines: true}, true
	a.Brain = Brain{Mode: Wander, Home: a.Pos(), Range: 5, Look: LookRotate}
	m.Add(a)
	n := 0
	m.CNPCWorldLine = func(s *Actor) {
		if s == a {
			n++
		}
	}
	far := 0.0
	for range 54000 {
		m.Tick()
		far = max(far, a.Pos().hdist(a.Brain.Home))
	}
	if n < 3 || n > 25 || far < 1 {
		t.Fatalf("%d world lines in 54000 ticks (about 10), wandered %.1f", n, far)
	}
}

// TestCNPCRouteHoldsWorldLines: a custom NPC walking a route says no world
// line while a segment is walked (EntityAIMovingPath, PASSIVE and before
// EntityAIWorldLines, runs); its pauses at the points leave them.
func TestCNPCRouteHoldsWorldLines(t *testing.T) {
	m := NewManager(flat(60), nil, nil, 1)
	a := New("a", "npc", 20.5, 1, 20.5)
	a.CNPC, a.PersistReq = &CNPCAI{OnAttack: 3, FindShelter: 2, WorldLines: true}, true
	a.Brain = Brain{Mode: Route, Points: []Vec{{X: 20.5, Y: 1, Z: 20.5}, {X: 32.5, Y: 1, Z: 20.5}}, Pattern: BackForth, Pause: 100}
	m.Add(a)
	walking, paused := 0, 0
	m.CNPCWorldLine = func(s *Actor) {
		if s != a {
			return
		}
		if a.routeHeld {
			walking++
		} else {
			paused++
		}
	}
	for range 54000 {
		m.Tick()
	}
	if walking != 0 || paused < 1 {
		t.Fatalf("world lines: %d while walking, %d in pauses", walking, paused)
	}
}
