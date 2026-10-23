package actors

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

type fireWorld struct {
	*testWorld
	fire, wet bool
}

func (w *fireWorld) Burning(collide.AABB) bool { return w.fire }
func (w *fireWorld) Wet(*Actor) bool           { return w.wet }

// TestFire: a strike sets 8 s of fire (1 damage a second through armour),
// fire blocks catch after fireResistance ticks, water puts it out.
func TestFire(t *testing.T) {
	w := &fireWorld{testWorld: flat(10)}
	m := NewManager(w, NewFactions(&Faction{Name: "f"}), nil, 2)
	a := New("a", "pig", 0.5, 1, 0.5)
	a.Faction, a.Health, a.MaxHealth, a.Armor = "f", 100, 100, 20
	m.Add(a)
	m.Tick()
	if a.fire != -1 || a.OnFire() {
		t.Fatal("fire at rest", a.fire)
	}
	m.StruckByLightning(a)
	if a.fire != 160 || a.Health != 99 { // 5 * 5/25 through full armour
		t.Fatal("strike", a.fire, a.Health)
	}
	for i := 0; i < 200; i++ {
		m.Tick()
	}
	// 160..20: eight unblockable 1s, the first inside the strike's resistance.
	if a.OnFire() || a.Health != 92 {
		t.Fatal("burnt", a.fire, a.Health)
	}
	w.fire = true
	m.Tick()
	if !a.OnFire() || a.fire != 160 {
		t.Fatal("catch from a fire block", a.fire)
	}
	w.fire, w.wet = false, true
	out := 0
	m.OnExtinguish = func(*Actor) { out++ }
	m.Tick()
	if a.OnFire() || out != 1 {
		t.Fatal("water", a.fire, out)
	}
}
