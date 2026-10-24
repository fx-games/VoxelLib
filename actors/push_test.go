package actors

import (
	"math"
	"testing"
)

// TestPush: applyEntityCollision runs once per loop that reaches a pair:
// two living actors push each other twice a tick, a living actor and a
// barrel once (the barrel's loop takes barrels only); actors closer than
// 0.01 are left alone.
func TestPush(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	mk := func(id string, x, z float64, boat bool) *Actor {
		a := New(id, id, x, 1, z)
		a.Body.Width, a.Body.Height, a.Health = 0.6, 1.8, 20
		if boat {
			a.Body.Width, a.Body.Height, a.Boat = 1, 1, &Boat{}
		}
		m.Add(a)
		return a
	}
	a, b := mk("a", 0.5, 0.5, false), mk("b", 1, 0.5, false)
	c, d := mk("c", 5.5, 0.5, false), mk("d", 6, 0.5, true)
	e, f := mk("e", -5.5, 0.5, false), mk("f", -5.495, 0.5, false)
	m.push()
	one := 0.5 / float64(float32(math.Sqrt(0.5))) * 1 * 0.05000000074505806
	if a.Body.VX != -2*one || b.Body.VX != 2*one || c.Body.VX != -one || d.Body.VX != one || e.Body.VX != 0 || f.Body.VX != 0 {
		t.Fatalf("pushes: living %v %v, living/barrel %v %v, stacked %v %v; one is %v",
			a.Body.VX, b.Body.VX, c.Body.VX, d.Body.VX, e.Body.VX, f.Body.VX, one)
	}
}

// TestUnpushed: an Unpushed actor pushes the other but keeps its motion;
// Pathing follows the path; HurtKind is the kind inside a Ward only.
func TestUnpushed(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	a, b := New("a", "a", 0.5, 1, 0.5), New("b", "b", 1, 1, 0.5)
	for _, x := range []*Actor{a, b} {
		x.Body.Width, x.Body.Height, x.Health = 0.6, 1.8, 20
		m.Add(x)
	}
	a.Unpushed = true
	m.push()
	if a.Body.VX != 0 || b.Body.VX <= 0 {
		t.Fatalf("unpushed %v, pushed %v", a.Body.VX, b.Body.VX)
	}
	var k HurtKind = -1
	a.Ward = func(_, _ *Actor, f float64) float64 { k = m.HurtKind(); return f }
	m.HurtAs(HurtFire, a, nil, 1, false)
	if k != HurtFire || m.HurtKind() != HurtOther || a.Pathing() {
		t.Fatalf("kind %v after %v pathing %v", k, m.HurtKind(), a.Pathing())
	}
}
