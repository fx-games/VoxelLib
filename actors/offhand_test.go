package actors

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

var testShield = &inventory.Item{Key: "test:shield", MaxStack: 1, MaxDamage: 336, Blocks: inventory.Shield()}

// guarded is a player at the origin facing -z with a shield raised in
// its off hand for ticks ticks, and an attacker at angle deg off its
// facing, 1.5 blocks away.
func guarded(t *testing.T, ticks int, deg float64) (*Manager, *Actor, *Actor, *ItemUse) {
	m := NewManager(flat(20), nil, nil, 1)
	p := New("p", "player", 0.5, 1, 0.5)
	p.Player, p.Health = true, 20
	inv := inventory.NewPlayer(inventory.PlayerSpec{OffHand: true})
	inv.Off = &inventory.Stack{Item: testShield, Count: 1}
	u := &ItemUse{Inv: inv}
	if !u.Guard(p, inventory.OffHand, inv.Off) {
		t.Fatal("the shield is not raised")
	}
	for range ticks {
		u.Hold(p, false)
	}
	r := deg * math.Pi / 180
	z := New("z", "zombie", 0.5+math.Sin(r)*1.5, 1, 0.5-math.Cos(r)*1.5)
	m.Add(p)
	m.Add(z)
	return m, p, z, u
}

// TestShieldBlock: the shield takes nothing for its first 4 ticks, then
// takes a whole blow from within 90° of the facing and none from behind;
// a blow of 4 wears it floor(1+4) = 5.
func TestShieldBlock(t *testing.T) {
	for _, c := range []struct {
		ticks   int
		deg     float64
		blocked bool
	}{{4, 0, false}, {5, 0, true}, {5, 80, true}, {5, -80, true}, {5, 100, false}, {5, 180, false}} {
		m, p, z, u := guarded(t, c.ticks, c.deg)
		m.Hurt(p, z, 4, false)
		if got := p.Health == 20; got != c.blocked {
			t.Errorf("%d ticks at %v°: blocked %v (health %v)", c.ticks, c.deg, got, p.Health)
		}
		if c.blocked && u.Using.Damage != 5 {
			t.Errorf("wear %d", u.Using.Damage)
		}
	}
	m, p, _, _ := guarded(t, 5, 0)
	m.Hurt(p, nil, 4, false) // no source: the angle is pi
	if p.Health != 16 {
		t.Errorf("a sourceless hurt is blocked: %v", p.Health)
	}
}

// TestAxeDisable: a blow from a disabling weapon (an axe's 5 s) puts the
// shield on cooldown for 100 ticks and lowers it.
func TestAxeDisable(t *testing.T) {
	m, p, z, u := guarded(t, 5, 0)
	axe := &inventory.Stack{Item: &inventory.Item{Key: "test:axe", Weapon: &inventory.Weapon{DisableSeconds: inventory.AxeDisableSeconds}}, Count: 1}
	m.Weapon = func(a *Actor) *inventory.Stack {
		if a == z {
			return axe
		}
		return nil
	}
	var ticks int
	u.OnDisabled = func(_ *Actor, _ *inventory.Stack, n int) { ticks = n }
	m.Hurt(p, z, 4, false)
	if u.Using != nil || p.BlockingWith() != nil || ticks != 100 {
		t.Fatalf("not disabled: using %v, ticks %d", u.Using, ticks)
	}
	if u.Guard(p, inventory.OffHand, u.Inv.Off) {
		t.Fatal("raised on cooldown")
	}
	for range 99 {
		u.Inv.Cool.Tick()
	}
	if !u.Inv.Cool.On(u.Inv.Off) {
		t.Fatal("cooldown over after 99 ticks")
	}
	u.Inv.Cool.Tick()
	if u.Inv.Cool.On(u.Inv.Off) {
		t.Fatal("cooldown still on after 100 ticks")
	}
}

var testSword = &inventory.Item{Key: "test:sword", AttackSpeed: -2.4, Sword: true}

// striker is a player with a sword charged for ticks, facing -z at the
// origin on the ground, a target in front, a bystander beside it.
func striker(ticks int) (*Manager, *Actor, *Actor, *Actor, *Charge) {
	m := NewManager(flat(20), nil, nil, 1)
	p := New("p", "player", 0.5, 1, 0.5)
	p.Player, p.Speed, p.Body.OnGround = true, 0.1, true
	a := New("a", "zombie", 0.5, 1, -1)
	b := New("b", "zombie", 1.3, 1, -1)
	a.Health, b.Health = 100, 100
	for _, x := range []*Actor{p, a, b} {
		m.Add(x)
	}
	c := &Charge{}
	main := &inventory.Stack{Item: testSword, Count: 1}
	c.Tick(main) // the item changes: 0
	for range ticks {
		c.Tick(main)
	}
	return m, p, a, b, c
}

// TestAttackStrength: a sword's delay is 12.5 ticks; 5 ticks into it the
// blow is (5.5/12.5)² scaled: 7 × (0.2 + 0.44²·0.8); a blow resets the
// charge; a crit needs full strength.
func TestAttackStrength(t *testing.T) {
	m, p, a, _, c := striker(5)
	if c.Delay() != float32(1/(4+float64(float32(-2.4)))*20) { // 12.500001: the attribute is a double

		t.Fatalf("delay %v", c.Delay())
	}
	m.Strike(p, a, Blow{Damage: 7, Charge: c})
	s := float32(5.5) / c.Delay()
	if want := float64(float32(7) * (0.2 + s*s*0.8)); math.Abs(100-a.Health-want) > 1e-4 || c.Ticker != 0 {
		t.Fatalf("hurt %v, want %v (ticker %d)", 100-a.Health, want, c.Ticker)
	}
	m, p, a, _, c = striker(5)
	p.Body.OnGround = false
	m.Strike(p, a, Blow{Damage: 7, Charge: c, Crit: true})
	if want := float64(float32(7) * (0.2 + s*s*0.8)); math.Abs(100-a.Health-want) > 1e-4 {
		t.Fatalf("a weak blow crits: %v", 100-a.Health)
	}
	m, p, a, _, c = striker(12)
	p.Body.OnGround = false
	m.Strike(p, a, Blow{Damage: 7, Charge: c, Crit: true})
	if math.Abs(100-a.Health-10.5) > 1e-4 {
		t.Fatalf("a full crit: %v", 100-a.Health)
	}
}

// TestSweep: a full sword blow on the ground hits the bystander for 1
// and pushes it 0.4 along the facing; no sweep without a sword, while
// sprinting, or for an ally.
func TestSweep(t *testing.T) {
	var sounds []string
	m, p, a, b, c := striker(12)
	m.Strike(p, a, Blow{Damage: 7, Charge: c, Sweep: true, Sound: func(e string) { sounds = append(sounds, e) }})
	if math.Abs(a.Health-93) > 1e-4 || math.Abs(b.Health-99) > 1e-4 || b.Body.VZ >= 0 || len(sounds) != 1 || sounds[0] != SoundAttackSweep {
		t.Fatalf("sweep: a %v b %v vz %v sounds %v", a.Health, b.Health, b.Body.VZ, sounds)
	}
	for name, f := range map[string]func(p *Actor, bl *Blow){
		"no sword": func(p *Actor, bl *Blow) { bl.Sweep = false },
		"sprint":   func(p *Actor, bl *Blow) { p.Sprinting = true },
		"ally":     func(p *Actor, bl *Blow) { bl.Allied = func(*Actor) bool { return true } },
		"moving":   func(p *Actor, bl *Blow) { p.Body.VX = 0.3 },
	} {
		m, p, a, b, c := striker(12)
		bl := Blow{Damage: 7, Charge: c, Sweep: true}
		f(p, &bl)
		m.Strike(p, a, bl)
		if b.Health != 100 {
			t.Errorf("%s: swept", name)
		}
	}
}
