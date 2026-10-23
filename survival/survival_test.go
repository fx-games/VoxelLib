package survival

import (
	"math/rand/v2"
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

// Vanilla tool materials (Item.ToolMaterial) and pickaxe/shovel definitions.
func pickaxe(level, uses int, eff float32) *inventory.Item {
	return &inventory.Item{Key: "pickaxe", MaxStack: 1, MaxDamage: uses, Tool: &inventory.Tool{
		Class: "pickaxe", Level: level, Efficiency: eff, Materials: []string{"rock", "iron", "anvil"},
		Harvests: []string{"rock", "iron", "anvil"}, BreakDamage: 1, HitDamage: 2}}
}

var (
	woodPick    = pickaxe(0, 59, 2)
	stonePick   = pickaxe(1, 131, 4)
	ironPick    = pickaxe(2, 250, 6)
	diamondPick = pickaxe(3, 1561, 8)
	goldPick    = pickaxe(0, 32, 12)
	woodShovel  = &inventory.Item{Key: "shovel", MaxStack: 1, MaxDamage: 59, Tool: &inventory.Tool{Class: "shovel", Efficiency: 2, BreakDamage: 1}}
	cobble      = &inventory.Item{Key: "test:cobblestone"}

	stone    = &Block{Key: "test:stone", Hardness: 1.5, Material: "rock", NeedsTool: true, HarvestTool: "pickaxe", Drops: []Drop{{Item: cobble}}}
	obsidian = &Block{Key: "test:obsidian", Hardness: 50, Material: "rock", NeedsTool: true, HarvestTool: "pickaxe", HarvestLevel: 3}
	ironOre  = &Block{Key: "test:iron_ore", Hardness: 3, Material: "rock", NeedsTool: true, HarvestTool: "pickaxe", HarvestLevel: 1}
	dirt     = &Block{Key: "test:dirt", Hardness: 0.5, Material: "ground", HarvestTool: "shovel"}
)

func breakTicks(b *Block, held *inventory.Stack, d Dig) int {
	var br Breaker
	s := Strength(b, held, d)
	for t := 1; t < 100000; t++ {
		if br.Tick(1, 2, 3, held, s, false, t == 1) {
			return t
		}
	}
	return -1
}

func TestBreakTimes(t *testing.T) {
	g := Dig{OnGround: true}
	st := func(it *inventory.Item) *inventory.Stack { return inventory.New(it, 1) }
	cases := []struct {
		name  string
		b     *Block
		held  *inventory.Stack
		d     Dig
		ticks int
	}{
		{"stone by hand", stone, nil, g, 151}, // 7.5 s on the wiki; float32 accumulation needs one tick more
		{"stone wooden pickaxe", stone, st(woodPick), g, 23},
		{"stone stone pickaxe", stone, st(stonePick), g, 12},
		{"stone iron pickaxe", stone, st(ironPick), g, 8},
		{"stone diamond pickaxe", stone, st(diamondPick), g, 6},
		{"stone golden pickaxe", stone, st(goldPick), g, 4},
		{"obsidian diamond", obsidian, st(diamondPick), g, 188},
		{"obsidian iron", obsidian, st(ironPick), g, 834},
		{"iron ore wooden pickaxe", ironOre, st(woodPick), g, 151},
		{"dirt by hand", dirt, nil, g, 15},
		{"dirt wooden shovel", dirt, st(woodShovel), g, 8},
		{"stone iron pickaxe jumping", stone, st(ironPick), Dig{}, 38},
		{"stone iron pickaxe underwater", stone, st(ironPick), Dig{OnGround: true, Underwater: true}, 38},
		{"stone iron pickaxe efficiency V", stone, st(ironPick), Dig{OnGround: true, Efficiency: 5}, 2},
		{"stone by hand haste II", stone, nil, Dig{OnGround: true, Haste: 2}, 108},
	}
	for _, c := range cases {
		if got := breakTicks(c.b, c.held, c.d); got != c.ticks {
			t.Errorf("%s: %d ticks, want %d", c.name, got, c.ticks)
		}
	}
	if CanHarvest(ironOre, st(woodPick)) || !CanHarvest(ironOre, st(stonePick)) || CanHarvest(stone, nil) || !CanHarvest(dirt, nil) {
		t.Fatal("harvest levels")
	}
}

func TestBreakerDelayAndInstant(t *testing.T) {
	var br Breaker
	grass := &Block{Hardness: 0, Material: "plants"}
	if !br.Tick(0, 0, 0, nil, Strength(grass, nil, Dig{OnGround: true}), false, true) {
		t.Fatal("hardness 0 must break instantly")
	}
	// after a normal break, 5 ticks pass before the next block takes damage
	s := Strength(stone, inventory.New(diamondPick, 1), Dig{OnGround: true})
	held := inventory.New(diamondPick, 1)
	n := 0
	for !br.Tick(0, 0, 0, held, s, false, n == 0) {
		n++
	}
	ticks := 0
	for !br.Tick(0, 1, 0, held, s, false, false) {
		ticks++
	}
	if ticks+1 != 5+1+6 { // 5 delay, 1 click, 6 damage ticks
		t.Fatalf("second block took %d ticks", ticks+1)
	}
}

func TestBreakBlock(t *testing.T) {
	p := &inventory.Player{}
	p.Main[0] = &inventory.Stack{Item: woodPick, Count: 1, Damage: 59}
	body := NewPlayer(&p.Armor)
	r := rand.New(rand.NewPCG(1, 1))
	out := BreakBlock(stone, p, body, 0, r)
	if len(out.Drops) != 1 || out.Drops[0].Item != cobble || !out.ToolBroke || p.Main[0] != nil {
		t.Fatalf("break: %+v", out)
	}
	if body.Food.Exhaustion != ExhaustMine {
		t.Fatal("mining exhaustion")
	}
	if out := BreakBlock(stone, p, body, 0, r); len(out.Drops) != 0 {
		t.Fatal("stone by hand dropped")
	}
	ore := &Block{Drops: []Drop{{Item: cobble, Min: 1, Max: 1}}, FortuneOre: true, XPMin: 3, XPMax: 7}
	for range 50 {
		out := BreakBlock(ore, p, nil, 3, r)
		if n := out.Drops[0].Count; n < 1 || n > 4 || out.XP < 3 || out.XP > 7 {
			t.Fatalf("fortune III count %d xp %d", n, out.XP)
		}
	}
}

func TestFoodRegenAndStarve(t *testing.T) {
	b := NewPlayer(nil)
	b.Health = 10
	e := Env{Difficulty: Normal, NaturalRegen: true}
	for range 79 {
		b.Tick(e)
	}
	if b.Health != 10 {
		t.Fatal("healed before 80 ticks")
	}
	b.Tick(e)
	if b.Health != 11 || b.Food.Exhaustion != 3 {
		t.Fatalf("regen: health %v exhaustion %v", b.Health, b.Food.Exhaustion)
	}
	// 2 heals = 6 exhaustion: one saturation point gone on the next tick
	for range 81 {
		b.Tick(e)
	}
	if b.Health != 12 || b.Food.Saturation != 4 || b.Food.Level != 20 {
		t.Fatalf("saturation %v level %d", b.Food.Saturation, b.Food.Level)
	}
	// no regeneration below 18 food
	b.Food.Level, b.Food.Saturation = 17, 0
	for range 200 {
		b.Tick(e)
	}
	if b.Health != 12 {
		t.Fatal("regen at food 17")
	}
	// starving on normal stops at 1 health, on easy at 10, hard kills
	for _, c := range []struct {
		d    Difficulty
		stop float32
	}{{Easy, 10}, {Normal, 1}, {Hard, 0}} {
		b := NewPlayer(nil)
		b.Food.Level, b.Food.Saturation = 0, 0
		for range 80 * 25 {
			b.Tick(Env{Difficulty: c.d, NaturalRegen: true})
		}
		if b.Health != c.stop {
			t.Errorf("starve %v: health %v", c.d, b.Health)
		}
	}
	// peaceful: heal every 20 ticks
	p := NewPlayer(nil)
	p.Health, p.Food.Level = 10, 10
	for range 100 {
		p.Tick(Env{Difficulty: Peaceful, NaturalRegen: true})
	}
	if p.Health != 15 {
		t.Fatalf("peaceful regen: %v", p.Health)
	}
}

func TestFoodValues(t *testing.T) {
	f := NewFood()
	f.Level, f.Saturation = 10, 0
	f.Add(8, 0.8) // steak: 8 hunger, 12.8 saturation (capped at level)
	if f.Level != 18 || f.Saturation != 12.8 {
		t.Fatalf("steak: %d %v", f.Level, f.Saturation)
	}
	f.Add(4, 0.3) // apple
	if d := f.Saturation - 15.2; f.Level != 20 || d > 1e-5 || d < -1e-5 {
		t.Fatalf("apple: %d %v", f.Level, f.Saturation)
	}
	g := NewFood()
	g.Moved(1, 0, 0, MoveSprint)
	g.Moved(0, 0, 1, MoveWalk)
	if d := g.Exhaustion - 0.11; d > 1e-6 || d < -1e-6 {
		t.Fatalf("movement exhaustion %v", g.Exhaustion)
	}
	bread := &inventory.Item{Key: "bread", Food: &inventory.Food{Hunger: 5, Saturation: 0.6}}
	b := NewPlayer(nil)
	s := inventory.New(bread, 2)
	if b.CanEat(s) {
		t.Fatal("full player can eat bread")
	}
	b.Food.Level = 10
	eaten := false
	bread.OnEat = func(*inventory.Ctx) { eaten = true }
	b.Eat(s, nil)
	if s.Count != 1 || b.Food.Level != 15 || !eaten || EatTicks(s) != 32 {
		t.Fatal("eat")
	}
}

func armorItem(slot, points, durability int) *inventory.Item {
	return &inventory.Item{Key: "armor", MaxStack: 1, MaxDamage: durability, Armor: &inventory.Armor{Slot: slot, Points: points}}
}

func TestArmor(t *testing.T) {
	var a [4]*inventory.Stack
	// full iron: 2+6+5+2 = 15 points → 60 % absorbed
	a[3] = inventory.New(armorItem(inventory.Helmet, 2, 165), 1)
	a[2] = inventory.New(armorItem(inventory.Chestplate, 6, 240), 1)
	a[1] = inventory.New(armorItem(inventory.Leggings, 5, 225), 1)
	a[0] = inventory.New(armorItem(inventory.Boots, 2, 195), 1)
	b := NewPlayer(&a)
	b.Hurt(MobAttack(nil), 10, Normal)
	if d := b.Health - 16; d > 1e-5 || d < -1e-5 {
		t.Fatalf("full iron vs 10: health %v, want 16", b.Health)
	}
	// each piece wears max(1, int(10·points/25))
	if a[3].Damage != 1 || a[2].Damage != 2 || a[1].Damage != 2 || a[0].Damage != 1 {
		t.Fatalf("wear %d %d %d %d", a[3].Damage, a[2].Damage, a[1].Damage, a[0].Damage)
	}
	// fall damage bypasses armour
	b.HurtResist = 0
	b.Hurt(Fall(), 4)
	if b.Health != 12 || a[2].Damage != 2 {
		t.Fatalf("fall through armour: %v", b.Health)
	}
	// a nearly broken piece absorbs at most its remaining durability
	var c [4]*inventory.Stack
	c[2] = &inventory.Stack{Item: armorItem(inventory.Chestplate, 8, 528), Count: 1, Damage: 528}
	p := NewPlayer(&c)
	p.Hurt(Generic(), 1) // bypasses
	p.HurtResist = 0
	p.Hurt(Cactus(), 10) // absorb capped at 1/25 of a point → 10 - 0.04
	if d := p.Health - (20 - 1 - 10 + 0.04); d > 1e-4 || d < -1e-4 || c[2] != nil {
		t.Fatalf("broken chestplate: health %v, piece %v", p.Health, c[2])
	}
	// mobs use the vanilla formula
	m := NewMob(20)
	m.NaturalArmor = 2
	m.Hurt(PlayerAttack(nil), 10)
	if m.Health != 20-10*23.0/25 {
		t.Fatalf("zombie armour: %v", m.Health)
	}
}

func TestDifficultyAndFrames(t *testing.T) {
	for _, c := range []struct {
		d    Difficulty
		want float32
	}{{Peaceful, 20}, {Easy, 20 - 4}, {Normal, 20 - 6}, {Hard, 20 - 9}} {
		b := NewPlayer(nil)
		b.Hurt(MobAttack(nil), 6, c.d)
		if b.Health != c.want {
			t.Errorf("difficulty %v: %v", c.d, b.Health)
		}
	}
	b := NewPlayer(nil)
	b.Hurt(Generic(), 4)
	b.Hurt(Generic(), 3) // within the frames, weaker: ignored
	b.Hurt(Generic(), 6) // stronger: only the difference
	if b.Health != 14 {
		t.Fatalf("frames: %v", b.Health)
	}
	for range 10 {
		b.Tick(Env{Difficulty: Normal})
	}
	b.Hurt(Generic(), 1) // frames half over: full damage again
	if b.Health != 13 {
		t.Fatalf("after 10 ticks: %v", b.Health)
	}
	c := NewPlayer(nil)
	c.Creative = true
	if c.Hurt(Generic(), 5) || !c.Hurt(OutOfWorld(), 5) {
		t.Fatal("creative")
	}
	// absorption soaks damage first; exhaustion 0.3 from a blockable hit
	d := NewPlayer(nil)
	d.Absorption = 4
	d.Hurt(Cactus(), 6)
	if d.Health != 18 || d.Absorption != 0 || d.Food.Exhaustion != 0.3 {
		t.Fatalf("absorption %v %v %v", d.Health, d.Absorption, d.Food.Exhaustion)
	}
}

func TestFallDrownFire(t *testing.T) {
	b := NewPlayer(nil)
	for range 10 {
		b.Move(-0.5, false, Normal)
	}
	b.Move(0, true, Normal) // 5 blocks: 2 damage
	if b.Health != 18 {
		t.Fatalf("fall: %v", b.Health)
	}
	b.JumpBoost = 2
	b.HurtResist = 0
	b.Land(5, Normal)
	if b.Health != 18 {
		t.Fatal("jump boost II")
	}

	// drowning: 300 air + 20 ticks, then 2 damage every 20 ticks
	d := NewPlayer(nil)
	w := Env{HeadInWater: true, Wet: true, Difficulty: Normal}
	for range 319 {
		d.Tick(w)
	}
	if d.Health != 20 {
		t.Fatal("drowned early")
	}
	d.Tick(w)
	if d.Health != 18 || d.Air != 0 {
		t.Fatalf("drown: %v air %d", d.Health, d.Air)
	}
	d.Tick(Env{})
	if d.Air != 300 {
		t.Fatal("air refill")
	}

	// lava: 4 damage, burning 15 s; burning hurts 1 every 20 ticks
	f := NewPlayer(nil)
	f.Tick(Env{InLava: true, Difficulty: Normal})
	if f.Health != 16 || f.Fire != 300 {
		t.Fatalf("lava: %v fire %d", f.Health, f.Fire)
	}
	for range 300 {
		f.Tick(Env{Difficulty: Normal, NaturalRegen: false})
	}
	// 15 fire hits, but the first lands inside the lava hit's frames (1 <= 4)
	if f.Health != 2 || f.Burning() {
		t.Fatalf("after burning: %v fire %d", f.Health, f.Fire)
	}
	// water puts fire out
	g := NewMob(20)
	g.SetFire(8)
	g.Tick(Env{Wet: true})
	if g.Burning() {
		t.Fatal("wet and burning")
	}
}

func TestDeathRespawn(t *testing.T) {
	b := NewPlayer(nil)
	died := false
	b.OnDeath = func(*Damage) { died = true }
	b.Hurt(OutOfWorld(), 100)
	if !died || !b.Dead || b.Hurt(Generic(), 1) {
		t.Fatal("death")
	}
	b.Food.Level = 3
	b.Respawn()
	if b.Dead || b.Health != 20 || b.Food.Level != 20 || b.Food.Saturation != 5 {
		t.Fatal("respawn")
	}
}

func TestXP(t *testing.T) {
	var x XP
	x.Add(17)
	if x.Level != 1 || x.Progress != 0 {
		t.Fatalf("level 1: %+v", x)
	}
	x.Add(17 * 14)
	if x.Level != 15 || x.BarCap() != 17 {
		t.Fatalf("level 15: %+v", x)
	}
	if x.DeathDrop() != 100 || XPSplit(20) != 17 || XPSplit(2) != 1 {
		t.Fatal("drop/split")
	}
}
