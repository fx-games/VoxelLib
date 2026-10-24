package actors

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
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

// TestCNPCFlip: a hurt CustomNPC turns on the one who hurt it when that one
// is nearer than its target (combat policy Flip), else keeps its target;
// one with no target takes the attacker.
func TestCNPCFlip(t *testing.T) {
	m, a, tg := cnpcPair(t, CNPCAI{}, 10.5)
	far := New("far", "man", 20.5, 1, 0.5)
	near := New("near", "man", 3.5, 1, 0.5)
	far.Faction, near.Faction = "men", "men"
	m.Add(far)
	m.Add(near)
	m.Hurt(a, far, 1, false)
	if a.Target != tg {
		t.Fatalf("far attacker taken: %v", a.Target.ID)
	}
	a.hurtResist = 0
	m.Hurt(a, near, 1, false)
	if a.Target != near {
		t.Fatalf("near attacker not taken: %v", a.Target.ID)
	}
	a.Target, a.hurtResist = nil, 0
	m.Hurt(a, far, 1, false)
	if a.Target != far {
		t.Fatal("no target from the attack")
	}
}

// fakeSky: a roof over x >= 5, light 12 under it, dark (brightness 0.1)
// under it past x 8.
type fakeSky struct{ day, rain bool }

func (s fakeSky) SeesSky(x, y, z int) bool  { return x < 5 }
func (s fakeSky) FullLight(x, y, z int) int { return 12 }
func (s fakeSky) Brightness(x, y, z int) float32 {
	if x > 8 {
		return 0.1
	}
	return 0.9
}
func (s fakeSky) Daytime() bool { return s.day }
func (s fakeSky) Raining() bool { return s.rain }

// TestCNPCShelter: FindShelter 1 seeks a dark covered block by day from
// under the open sky; 0 seeks a lit covered block at night, but not when
// under one; a shaded walker's path ends two cells short of the sun.
func TestCNPCShelter(t *testing.T) {
	m, a, _ := cnpcPair(t, CNPCAI{FindShelter: 1}, 30.5)
	m.Sky = fakeSky{day: true}
	a.Target = nil
	for range 200 {
		if p, ok := m.shelterShould(a); ok {
			if p.X <= 8 {
				t.Fatalf("shade at %v", p)
			}
			break
		}
	}
	m.Tick()
	if !a.cn.on[cnShelter] || a.cn.home.X <= 8 {
		t.Fatalf("shade task %v home %v", a.cn.on[cnShelter], a.cn.home)
	}
	m.Sky = fakeSky{}
	if _, ok := m.shelterShould(a); ok {
		t.Fatal("shade by night")
	}
	a.CNPC.FindShelter = 0
	got := false
	for range 50 {
		if p, ok := m.shelterShould(a); ok {
			got = true
			if p.X < 5 {
				t.Fatalf("indoors at %v", p)
			}
		}
	}
	if !got {
		t.Fatal("no shelter at night")
	}
	a.Body.X = 6.5
	if _, ok := m.shelterShould(a); ok {
		t.Fatal("seeks shelter under a roof")
	}
	a.CNPC.FindShelter = 1
	m.Sky = fakeSky{day: true}
	p, cut := m.shade(a, []nav.Pos{{X: 6, Y: 1}, {X: 5, Y: 1}, {X: 4, Y: 1}, {X: 3, Y: 1}})
	if !cut || len(p) != 1 {
		t.Fatalf("cut %v %v", cut, p)
	}
}

type fakeDoors struct {
	w      *testWorld
	stages []int
	hits   int
	busted bool
}

func (d *fakeDoors) Bustable(x, y, z int) bool {
	_, ok := d.w.door[nav.Pos{X: x, Y: y, Z: z}]
	return ok
}
func (d *fakeDoors) Hit(a *Actor, x, y, z int) { d.hits++ }
func (d *fakeDoors) Crack(a *Actor, x, y, z, stage int) {
	d.stages = append(d.stages, stage)
}
func (d *fakeDoors) Bust(a *Actor, x, y, z int) {
	delete(d.w.door, nav.Pos{X: x, Y: y, Z: z})
	delete(d.w.door, nav.Pos{X: x, Y: y - 1, Z: z})
	d.busted = true
}

// TestCNPCBustDoor: DoorInteract 0 walks its path up to a closed door in
// a wall, hits it for 240 ticks (cracks 0..9, then 10) and breaks it
// without ever opening it.
func TestCNPCBustDoor(t *testing.T) {
	m, a, _ := cnpcPair(t, CNPCAI{DoorInteract: 0, FindShelter: 2}, 8.5)
	w := m.W.(*testWorld)
	for z := -40; z < 40; z++ {
		w.solid[nav.Pos{X: 3, Y: 1, Z: z}], w.solid[nav.Pos{X: 3, Y: 2, Z: z}] = true, true
	}
	delete(w.solid, nav.Pos{X: 3, Y: 1})
	delete(w.solid, nav.Pos{X: 3, Y: 2})
	w.door[nav.Pos{X: 3, Y: 1}], w.door[nav.Pos{X: 3, Y: 2}] = false, false
	d := &fakeDoors{w: w}
	m.Doors = d
	a.OpensDoors = false
	for i := 0; i < 600 && !d.busted; i++ {
		m.Tick()
		if w.door[nav.Pos{X: 3, Y: 1}] {
			t.Fatal("opened the door")
		}
	}
	if !d.busted || d.hits == 0 {
		t.Fatalf("busted %v hits %d stages %v", d.busted, d.hits, d.stages)
	}
	if n := len(d.stages); n < 11 || d.stages[0] != 0 || d.stages[n-1] != 10 {
		t.Fatalf("stages %v", d.stages)
	}
}
