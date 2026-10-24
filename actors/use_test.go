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
