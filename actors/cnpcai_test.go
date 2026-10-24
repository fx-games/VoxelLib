package actors

import (
	"math"
	"testing"
)

func cnpcPair(t *testing.T, ai CNPCAI, tx float64) (*Manager, *Actor, *Actor) {
	t.Helper()
	f := NewFactions(&Faction{Name: "orcs", Enemies: []string{"men"}}, &Faction{Name: "men"})
	m := NewManager(flat(40), f, nil, 3)
	a := New("npc", "npc", 0.5, 1, 0.5)
	a.Faction, a.CNPC, a.Melee.Damage = "orcs", &ai, 3
	tg := New("man", "man", tx, 1, 0.5)
	tg.Faction, tg.Health, tg.MaxHealth = "men", 500, 500
	tg.Brain = Brain{Mode: Stand, Home: tg.Pos()}
	m.Add(a)
	m.Add(tg)
	a.AggroRange = 0
	for range 5 { // settle on the ground
		m.Tick()
	}
	a.AggroRange, a.Target, a.cn = 16, tg, cnpcState{}
	return m, a, tg
}

// TestCNPCAI: EntityAITasks order and mutexes over setResponse's tasks:
// the sprint (PASSIVE) stops the attack (dropping the target) at the next
// task tick, and holds while it moves; the leap jumps 0.4 at a target 2
// to 4 away; a panicking or idle (OnAttack 3) NPC never strikes; HitNRun
// only runs once hurt; an ambusher hides in cover, gives the target back
// when it stops; the stalk sees whether it is looked at.
func TestCNPCAI(t *testing.T) {
	m, a, tg := cnpcPair(t, CNPCAI{CanSprint: true, Radius: 8}, 14.5)
	m.Tick() // a task tick: the attack starts and walks
	if !a.cn.on[cnAttack] || a.cn.on[cnSprint] || !a.pathing() {
		t.Fatalf("attack %v sprint %v", a.cn.on[cnAttack], a.cn.on[cnSprint])
	}
	m.Tick()
	m.Tick()
	m.Tick() // next task tick: the sprint takes over
	if !a.cn.on[cnSprint] || a.cn.on[cnAttack] || !a.Sprinting || a.Target != nil {
		t.Fatalf("sprint %v attack %v target %v", a.cn.on[cnSprint], a.cn.on[cnAttack], a.Target)
	}
	for i := 0; i < 40 && a.Sprinting; i++ {
		m.Tick()
	}
	if a.Sprinting || a.cn.on[cnSprint] {
		t.Fatal("still sprinting standing")
	}

	m, a, _ = cnpcPair(t, CNPCAI{Leap: 1}, 3.5)
	jumped := false
	for range 120 {
		a.Target = m.byID["man"]
		if !jumped {
			a.Body.X, a.Body.VX = a.Target.Body.X-3, 0 // stays 3 away
		}
		vy := a.Body.VY
		m.Tick()
		if a.cn.on[cnLeap] && !jumped {
			jumped = true
			if a.Body.VY == vy || a.Body.VX <= 0 {
				t.Fatalf("leap: vy %v vx %v", a.Body.VY, a.Body.VX)
			}
		}
	}
	if !jumped {
		t.Fatal("never leapt")
	}

	for _, on := range []int{1, 3} {
		m, a, tg = cnpcPair(t, CNPCAI{OnAttack: on}, 1.5)
		for range 60 {
			a.Target = tg
			m.Tick()
		}
		if tg.Health != 500 || a.cn.on[cnAttack] {
			t.Fatalf("OnAttack %d struck", on)
		}
	}
	m, a, tg = cnpcPair(t, CNPCAI{OnAttack: 1}, 1.5)
	m.Tick()
	if !a.cn.on[cnResp] || a.rs.goalMul != 1.2 || !a.rs.goalSet {
		t.Fatal("no panic")
	}

	// HitNRun: holds while unhurt, runs once hurt
	m, a, tg = cnpcPair(t, CNPCAI{Tactic: navHitNRun, Radius: 2}, 6.5)
	m.Tick()
	if a.cn.on[cnResp] || !a.cn.on[cnAttack] {
		t.Fatal("ran unhurt")
	}
	a.Health--
	m.Tick()
	m.Tick()
	m.Tick()
	if !a.cn.on[cnResp] || a.cn.on[cnAttack] || tg.Pos().dist2(a.rs.goal) < tg.Pos().dist2(a.Pos()) {
		t.Fatalf("no run: resp %v goal %v", a.cn.on[cnResp], a.rs.goal)
	}

	// Ambush: a wall at x 5 hides the cells behind it
	m, a, tg = cnpcPair(t, CNPCAI{Tactic: navAmbush, Radius: 4}, 14.5)
	m.Opaque = func(x, y, z int) bool { return y == 0 }
	m.RayHits = func(from, to Vec) bool { return to.X < 3 && to.Z < 0 }
	a.cn.shelter = Vec{99, 99, 99} // a spot sharing a coordinate with the last is skipped
	m.Tick()
	if !a.cn.on[cnResp] || a.cn.shelter.Z >= 0 || a.cn.shelter.X >= 3 || a.cn.on[cnAttack] {
		t.Fatalf("no ambush: %v %v", a.cn.on[cnResp], a.cn.shelter)
	}
	a.Body.X = tg.Body.X - 2 // within the radius and seen: it stops
	m.Tick()
	if a.cn.on[cnResp] || a.Target != tg || a.cn.delay != 60 {
		t.Fatalf("ambush on %v target %v delay %d", a.cn.on[cnResp], a.Target, a.cn.delay)
	}

	// the stalker in front of the target's look, then behind it
	tg.Yaw, tg.Pitch = yawTo(tg.Pos(), a.Pos()), 0
	if m.lookingAway(a, tg) {
		t.Fatal("looked at, looking away")
	}
	tg.Yaw += math.Pi
	if !m.lookingAway(a, tg) {
		t.Fatal("looking away, not seen so")
	}
}
