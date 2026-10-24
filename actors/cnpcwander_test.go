package actors

import (
	"math"
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
	if a.pathing() || !a.cnpcInteracting() || a.cn.with[0] != p {
		t.Fatal("a player's interaction did not stop it")
	}
}
