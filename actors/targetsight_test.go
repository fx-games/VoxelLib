package actors

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/nav"
)

// TestTargetSight: a check-sight target task picks only a seen target,
// keeps it 60 ticks unseen and drops it on the 61st, and drops it
// beyond AggroRange (EntityAITarget.continueExecuting).
func TestTargetSight(t *testing.T) {
	m := NewManager(flat(40), nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	o := New("o", "npc", 6.5, 1, 0.5)
	a.Melee.Damage, a.TargetSight, a.AggroRange = 1, true, 16
	m.Add(a)
	m.Add(o)
	seen := false
	m.Sees = func(x, y *Actor) bool { return seen }
	m.AlsoTargets = func(x, y *Actor) bool { return x == a && y == o }
	m.Keeps = m.AlsoTargets
	if m.pickTarget(a) != nil {
		t.Fatal("picked an unseen target")
	}
	seen = true
	if a.Target = m.pickTarget(a); a.Target != o {
		t.Fatal("seen target not picked")
	}
	seen = false
	for i := range 60 {
		if m.fight(a); a.Target != o {
			t.Fatalf("dropped after %d unseen ticks", i+1)
		}
	}
	if m.fight(a); a.Target != nil {
		t.Fatal("kept after 61 unseen ticks")
	}
	seen = true
	a.Target = m.pickTarget(a)
	o.Body.X = a.Body.X + 16.5
	if m.fight(a); a.Target != nil {
		t.Fatal("kept beyond AggroRange")
	}
	a.TargetSight = false // a revenge or game-set target: no such task
	seen = false
	a.Target, o.Body.X = o, a.Body.X+3
	for range 100 {
		m.fight(a)
	}
	if a.Target != o {
		t.Fatal("no check-sight task, yet dropped")
	}
}

// TestTargetNearby: a nearbyOnly task picks only a target a path gets
// within 1.5 blocks of, and keeps that answer for 10 to 14 checks.
func TestTargetNearby(t *testing.T) {
	w := flat(40)
	m := NewManager(w, nil, nil, 1)
	a := New("a", "npc", 0.5, 1, 0.5)
	o := New("o", "npc", 8.5, 1, 0.5)
	a.Melee.Damage, a.TargetNearby, a.AggroRange = 1, true, 16
	a.Body.OnGround = true
	m.Add(a)
	m.Add(o)
	m.AlsoTargets = func(x, y *Actor) bool { return x == a && y == o }
	if m.pickTarget(a) != o {
		t.Fatal("reachable target not picked")
	}
	for x := 5; x <= 11; x++ { // walled in, 3 blocks out, 3 high
		for z := -3; z <= 3; z++ {
			if x == 5 || x == 11 || z == -3 || z == 3 {
				for y := 1; y <= 3; y++ {
					w.solid[nav.Pos{X: x, Y: y, Z: z}] = true
				}
			}
		}
	}
	a.SetPicked(nil) // startExecuting: asks again
	if m.pickTarget(a) != nil {
		t.Fatal("walled-in target picked")
	}
	for x := 5; x <= 11; x++ { // the walls gone, the answer holds a while
		for z := -3; z <= 3; z++ {
			for y := 1; y <= 3; y++ {
				delete(w.solid, nav.Pos{X: x, Y: y, Z: z})
			}
		}
	}
	n := 1
	for ; m.pickTarget(a) == nil; n++ {
		if n > 14 {
			t.Fatal("unreachable answer kept past 14 checks")
		}
	}
	if n < 10 {
		t.Fatalf("answer asked again after %d checks", n)
	}
}
