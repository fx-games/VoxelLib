package actors

import "testing"

// TestHomeArea: findRandomTarget keeps a homed walker near its home to
// cells inside the home area, and one far off it to none; a Guard keeps
// its target however far from home.
func TestHomeArea(t *testing.T) {
	m := NewManager(flat(60), nil, nil, 1)
	a := New("a", "npc", 20.5, 1, 20.5)
	m.Add(a)
	a.Brain = Brain{Mode: Stroll, Home: a.Pos(), Homed: true, HomeRange: 3}
	out := false
	for range 200 {
		p, ok := m.findRandomTarget(a, 10, 7)
		if ok && !a.Brain.InHome(ifloor(p.X), ifloor(p.Y), ifloor(p.Z)) {
			t.Fatalf("%v outside the home area", p)
		}
		a.Brain.Homed = false
		if p, _ := m.findRandomTarget(a, 10, 7); !a.Brain.InHome(ifloor(p.X), 0, 0) {
			t.Fatal("no home but outside")
		}
		a.Brain.Homed = true
		a.Brain.HomeRange = 100
		p, _ = m.findRandomTarget(a, 10, 7)
		a.Brain.HomeRange = 3
		out = out || p.hdist(a.Brain.Home) > 4
	}
	if !out {
		t.Fatal("a wide home area held to the small one")
	}
	a.Body.X = 40.5 // 20 off a home of 3: no restriction
	far := false
	for range 50 {
		p, ok := m.findRandomTarget(a, 10, 7)
		far = far || ok && !a.Brain.InHome(ifloor(p.X), ifloor(p.Y), ifloor(p.Z))
	}
	if !far {
		t.Fatal("far from home, still restricted")
	}

	g := New("g", "npc", 30.5, 1, 20.5)
	o := New("o", "npc", 50.5, 1, 20.5)
	g.Melee.Damage, g.AggroRange = 1, 30
	g.Brain = Brain{Mode: Guard, Home: Vec{X: 20.5, Y: 1, Z: 20.5}, Range: 2}
	m.Add(g)
	m.Add(o)
	m.Keeps = func(x, y *Actor) bool { return true }
	g.Target = o
	m.fight(g)
	if g.Target != o {
		t.Fatal("a guard off its home dropped its target")
	}
}

// TestCNPCReturn: EntityAIReturn walks a pushed standing custom NPC with
// ReturnToStart back to its place (never teleporting); without it, it
// stays; a fight notes where it began, and the walk goes back there.
func TestCNPCReturn(t *testing.T) {
	for _, ret := range []bool{true, false} {
		m := NewManager(flat(40), nil, nil, 1)
		a := New("a", "npc", 0.5, 1, 0.5)
		a.CNPC, a.AggroRange = &CNPCAI{Return: ret, FindShelter: 2, OnAttack: 3}, 16
		a.Brain = Brain{Mode: Stand, Home: a.Pos()}
		m.Add(a)
		for range 5 {
			m.Tick()
		}
		a.Body.X = 6.5
		x := a.Body.X
		for range 200 {
			m.Tick()
			if d := a.Body.X - x; d > 0.5 || d < -0.5 {
				t.Fatalf("moved %.2f in a tick", d)
			}
			x = a.Body.X
		}
		if a.homeNear() != ret || a.cn.on[cnReturn] {
			t.Fatalf("return %v: at %.2f, task %v", ret, a.Body.X, a.cn.on[cnReturn])
		}
	}

	m := NewManager(flat(40), nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	a.CNPC = &CNPCAI{Return: true, FindShelter: 2, OnAttack: 3}
	a.Brain = Brain{Mode: Stand, Home: a.Pos()}
	m.Add(a)
	a.Body.X, a.cn.attacking, a.Ticks = 8.5, true, 100 // past the first 40 ticks' interacting
	if m.returnShould(a) || !a.cn.rWas || a.cn.rPre.X != 8.5 {
		t.Fatal("no preAttackPos")
	}
	a.cn.attacking, a.Body.X = false, 12.5
	if !m.returnShould(a) || a.returnEnd().X != 8.5 {
		t.Fatal("not back to where the fight began")
	}
}
