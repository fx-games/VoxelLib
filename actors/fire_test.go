package actors

import (
	"math"
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

// TestPotionDefence: resistance (amplifier+1)*5/25 off after armour, also
// for unblockable damage; fire resistance ignores fire damage while the
// actor keeps burning (EntityLivingBase.attackEntityFrom,
// applyPotionDamageCalculations).
func TestPotionDefence(t *testing.T) {
	for _, c := range []struct {
		res         int
		armor       float64
		unblockable bool
		hit, want   float64
	}{
		{0, 0, false, 10, 90},
		{1, 0, false, 10, 92},    // 10 * 20/25
		{2, 0, false, 10, 94},    // Dwarf ring: resistance II, 10 * 15/25
		{2, 10, false, 10, 96.4}, // 10 * 15/25 armour, then * 15/25
		{2, 10, true, 10, 94},    // magic skips armour, not resistance
		{5, 0, false, 10, 100},   // 25 - 25
	} {
		m := NewManager(flat(10), NewFactions(&Faction{Name: "f"}), nil, 2)
		a := New("a", "pig", 0.5, 1, 0.5)
		a.Faction, a.Health, a.MaxHealth, a.Armor, a.Resistance = "f", 100, 100, c.armor, c.res
		m.Add(a)
		m.Hurt(a, nil, c.hit, c.unblockable)
		if math.Abs(a.Health-c.want) > 1e-5 {
			t.Errorf("%+v: health %v", c, a.Health)
		}
	}
	w := &fireWorld{testWorld: flat(10), fire: true}
	m := NewManager(w, NewFactions(&Faction{Name: "f"}), nil, 2)
	a := New("a", "pig", 0.5, 1, 0.5)
	a.Faction, a.Health, a.MaxHealth, a.FireProof = "f", 100, 100, true
	m.Add(a)
	m.StruckByLightning(a)
	for range 100 {
		m.Tick()
	}
	if !a.OnFire() || a.Health != 100 {
		t.Fatal("fire resistance", a.OnFire(), a.Health)
	}
}
