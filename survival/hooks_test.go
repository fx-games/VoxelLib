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
	b := NewPlayer(&a)
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
