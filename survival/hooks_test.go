package survival

import (
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

// The game hooks: a tool-speed factor, armour points and wear negation.
func TestGameHooks(t *testing.T) {
	g := Dig{OnGround: true, ToolFactor: 2}
	if n := breakTicks(stone, inventory.New(ironPick, 1), g); n != 4 { // 6·2 = golden pickaxe speed
		t.Fatalf("tool factor: %d ticks", n)
	}
	if n := breakTicks(stone, nil, g); n != 151 { // a hand's speed of 1 is not multiplied
		t.Fatalf("tool factor by hand: %d ticks", n)
	}

	inventory.ArmorPoints = func(s *inventory.Stack) int { return s.Item.Armor.Points + 1 }
	inventory.WearNegated = func(s *inventory.Stack, n int) int { return n }
	defer func() { inventory.ArmorPoints, inventory.WearNegated = nil, nil }()
	var a [4]*inventory.Stack
	a[2] = inventory.New(armorItem(inventory.Chestplate, 4, 240), 1)
	b := NewPlayer(a[:])
	if b.TotalArmor() != 5 {
		t.Fatalf("armour points hook: %d", b.TotalArmor())
	}
	b.Hurt(MobAttack(nil), 10, Normal) // 5 points → 20 % absorbed
	if d := b.Health - 12; d > 1e-5 || d < -1e-5 {
		t.Fatalf("health %v, want 12", b.Health)
	}
	if a[2].Damage != 0 {
		t.Fatalf("negated wear still applied: %d", a[2].Damage)
	}
}

// Enchantment protection: calcModifierDamage's numbers, the adjust hook
// after the randomised roll, and fire protection's shorter burns.
func TestProtectionHooks(t *testing.T) {
	fall, fire, arrow := Fall(), Lava(), MobAttack(nil)
	arrow.Projectile = true
	for _, c := range []struct {
		kind, level int
		d           *Damage
		want        int
	}{{0, 1, arrow, 1}, {0, 4, arrow, 5}, {1, 3, fire, 6}, {1, 3, arrow, 0}, {2, 4, fall, 18}, {4, 2, arrow, 5}, {2, 4, OutOfWorld(), 0}} {
		if got := ProtectionEPF(c.kind, c.level, c.d); got != c.want {
			t.Fatalf("EPF kind %d level %d: %d, want %d", c.kind, c.level, got, c.want)
		}
	}
	b := NewPlayer(make([]*inventory.Stack, 4))
	b.ProtectionAdjust = func(d *Damage, i int) int { return i + 5 } // not randomised: 25-5 of 25 left
	b.Hurt(MobAttack(nil), 10, Normal)
	if d := b.Health - 12; d > 1e-5 || d < -1e-5 {
		t.Fatalf("health %v, want 12", b.Health)
	}
	if FireTimeLeft(300, 3) != 165 || FireTimeLeft(100, 1) != 85 || FireTimeLeft(100, 0) != 100 {
		t.Fatal("fire time")
	}
	b.FireTime = func(n int) int { return FireTimeLeft(n, 2) }
	b.SetFire(15)
	if b.Fire != 210 {
		t.Fatalf("fire %d, want 210", b.Fire)
	}
}
