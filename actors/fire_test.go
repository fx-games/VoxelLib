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

// TestBurningPanic: EntityAIPanic runs while burning with no revenge
// target: a path to a random cell within 5 at PanicSpeed; a calm animal
// stands.
func TestBurningPanic(t *testing.T) {
	m := NewManager(flat(40), nil, nil, 1)
	a := New("cow", "cow", 0.5, 1, 0.5)
	a.FleeWhenHurt, a.PanicSpeed, a.Speed = true, 2, 0.2
	a.Brain.Mode = Stand
	a.Brain.Home = a.Pos()
	m.Add(a)
	for range 10 {
		m.Tick()
	}
	if a.panicking || a.Pos().hdist(Vec{0.5, 1, 0.5}) > 0.1 {
		t.Fatal("calm cow moved")
	}
	a.SetFire(8)
	moved := false
	for range 30 {
		from := a.Pos()
		m.Tick()
		if a.panicking && !moved {
			if g := a.nav.goal; math.Abs(g.X-from.X) > 5.5 || math.Abs(g.Z-from.Z) > 5.5 {
				t.Fatal("goal beyond 5", g, from)
			}
		}
		moved = moved || a.panicking && a.Pos().hdist(Vec{0.5, 1, 0.5}) > 0.5
	}
	if !moved {
		t.Fatal("burning cow did not panic")
	}
}

// TestPanicEnds: EntityAIPanic asks getAITarget, which lapses 100 ticks
// after the hurt: the cow panics while it lasts and then calms down.
func TestPanicEnds(t *testing.T) {
	m := NewManager(flat(40), nil, nil, 1)
	a := New("cow", "cow", 0.5, 1, 0.5)
	p := New("p", "player", 3.5, 1, 0.5)
	a.FleeWhenHurt, a.Speed = true, 0.2
	a.Brain.Mode = Stand
	a.Brain.Home = a.Pos()
	m.Add(a)
	a.SetRevenge(p)
	started := false
	for range 100 {
		m.Tick()
		started = started || a.panicking
	}
	if !started {
		t.Fatal("hurt cow did not panic")
	}
	for range 40 { // a running panic walks its path to the end
		m.Tick()
	}
	for range 60 {
		if m.Tick(); a.panicking {
			t.Fatal("panicking with the revenge target lapsed")
		}
	}
}
