package actors

import (
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

func TestItemUse(t *testing.T) {
	var u ItemUse
	a := &Actor{}
	st, other := &inventory.Stack{Count: 1}, &inventory.Stack{Count: 1}
	u.Raise(a, st)
	if u.Using != st || u.UseCount != Endless || !a.Blocking {
		t.Fatalf("raise: %+v %v", u, a.Blocking)
	}
	u.Hold(a, true)
	if u.Held() != 1 || u.Begin(st, 5) {
		t.Fatalf("held %d", u.Held())
	}
	if u.Keep(a, other, false) || u.Using != nil || a.Blocking {
		t.Fatal("switching away keeps the use")
	}
	u.Begin(st, 2)
	if u.Tick() || !u.Tick() {
		t.Fatal("tick")
	}
	u.UseCount = 24
	if !u.Crumbs() {
		t.Fatal("crumbs at 24")
	}
}

func TestStrikeSkips(t *testing.T) {
	m := &Manager{}
	called := false
	if m.Strike(&Actor{}, &Actor{}, Blow{Allow: func() bool { called = true; return true }}) || called {
		t.Fatal("a zero blow asks the veto or strikes")
	}
}

// TestCritical: every condition of the falling blow, and its ×1.5 on the
// damage before the enchantment bonus.
func TestCritical(t *testing.T) {
	p := New("p", "player", 0.5, 1, 0.5)
	p.Body.OnGround = false
	if !Critical(p, 1, false) {
		t.Fatal("a falling player does not crit")
	}
	for name, f := range map[string]func(){
		"no fall":  func() {},
		"ground":   func() { p.Body.OnGround = true },
		"ladder":   func() { p.Body.OnLadder = true },
		"water":    func() { p.Body.InWater = true },
		"riding":   func() { p.Riding = &Actor{} },
		"blind":    func() {},
		"sprints!": func() { p.Body.Sprinting = true },
	} {
		p.Body.OnGround, p.Body.OnLadder, p.Body.InWater, p.Riding, p.Body.Sprinting = false, false, false, nil, false
		f()
		fall, blind := float32(1), name == "blind"
		if name == "no fall" {
			fall = 0
		}
		if got, want := Critical(p, fall, blind), name == "sprints!"; got != want {
			t.Errorf("%s: crit %v", name, got)
		}
	}
	for _, c := range []struct {
		crit       bool
		dmg, bonus float64
		want       float32
	}{{false, 4, 1, 5}, {true, 4, 1, 7}, {true, 0, 2, 2}} {
		m := NewManager(flat(20), nil, nil, 1)
		a, tg := New("a", "player", 0.5, 1, 0.5), New("t", "cow", 2, 1, 0.5)
		tg.Health = 100
		m.Add(a)
		m.Add(tg)
		m.Strike(a, tg, Blow{Damage: c.dmg, Bonus: c.bonus, Crit: c.crit})
		if got := float32(100 - tg.Health); got != c.want {
			t.Errorf("%+v: dealt %v", c, got)
		}
	}
}
