package crafting

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

const recipesJSON = `{
  "ores": {"plankWood": ["minecraft:planks#*", "lotr:planks#*"], "stickWood": ["minecraft:stick"]},
  "recipes": [
    {"type": "shaped", "pattern": ["XXX", " # ", " # "], "key": {"X": "ore:plankWood", "#": "ore:stickWood"},
     "result": {"item": "minecraft:wooden_pickaxe"}},
    {"type": "shaped", "pattern": ["XX", "X#", " #"], "key": {"X": "minecraft:cobblestone", "#": "minecraft:stick"},
     "result": {"item": "minecraft:stone_axe"}},
    {"type": "shaped", "pattern": ["X ", " X"], "key": {"X": "minecraft:cobblestone"}, "mirrored": false,
     "result": {"item": "minecraft:stick", "count": 9}},
    {"type": "shaped", "pattern": ["X", "X"], "key": {"X": "ore:plankWood"},
     "result": {"item": "minecraft:stick", "count": 4}},
    {"type": "shapeless", "ingredients": ["minecraft:log#*"], "result": {"item": "minecraft:planks#0", "count": 4}},
    {"type": "shapeless", "ingredients": ["minecraft:unknown_thing"], "result": {"item": "minecraft:stick"}}
  ],
  "smelting": [
    {"input": "minecraft:iron_ore#*", "result": {"item": "minecraft:iron_ingot"}, "xp": 0.7},
    {"input": "minecraft:cobblestone", "result": {"item": "minecraft:stone"}, "xp": 0.1}
  ]
}`

func setup(t *testing.T) (*inventory.Registry, *Book, *Smelting) {
	reg := inventory.NewRegistry()
	for _, it := range []*inventory.Item{
		{Key: "minecraft:planks", HasSubtypes: true, BurnTime: 300},
		{Key: "lotr:planks", HasSubtypes: true, BurnTime: 300},
		{Key: "minecraft:log", HasSubtypes: true, BurnTime: 300},
		{Key: "minecraft:stick", BurnTime: 100},
		{Key: "minecraft:cobblestone"},
		{Key: "minecraft:stone"},
		{Key: "minecraft:coal", BurnTime: 1600},
		{Key: "minecraft:iron_ore"},
		{Key: "minecraft:iron_ingot"},
		{Key: "minecraft:wooden_pickaxe", MaxStack: 1, MaxDamage: 59, BurnTime: 200},
		{Key: "minecraft:stone_axe", MaxStack: 1, MaxDamage: 131},
	} {
		if _, err := reg.Register(it); err != nil {
			t.Fatal(err)
		}
	}
	b, sm, warn, err := Load(strings.NewReader(recipesJSON), reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) != 1 || len(b.Recipes) != 5 || len(sm.List) != 2 {
		t.Fatalf("warnings %v, %d recipes, %d smelts", warn, len(b.Recipes), len(sm.List))
	}
	return reg, b, sm
}

func grid(reg *inventory.Registry, w int, rows ...string) *inventory.Grid {
	g := inventory.NewGrid(w, len(rows))
	for y, row := range rows {
		for x, k := range strings.Split(row, ",") {
			if k == "" {
				continue
			}
			name, meta, _ := strings.Cut(k, "#")
			s := inventory.New(reg.Get(name), 1)
			if meta == "3" {
				s.Damage = 3
			}
			g.Slots[x+y*w] = s
		}
	}
	return g
}

func key(s *inventory.Stack) string {
	if s == nil {
		return ""
	}
	return s.Item.Key
}

func TestShaped(t *testing.T) {
	reg, b, _ := setup(t)
	P, S, C := "minecraft:planks", "minecraft:stick", "minecraft:cobblestone"
	L := "lotr:planks#3"
	cases := []struct {
		w    int
		rows []string
		want string
		n    int
	}{
		{3, []string{P + "," + L + "," + P, "," + S + ",", "," + S + ","}, "minecraft:wooden_pickaxe", 1},
		{3, []string{C + "," + C + ",", C + "," + S + ",", "," + S + ","}, "minecraft:stone_axe", 1},
		{3, []string{"," + C + "," + C, "," + S + "," + C, "," + S + ","}, "minecraft:stone_axe", 1}, // mirrored
		{2, []string{C + ",", "," + C}, "minecraft:stick", 9},
		{2, []string{"," + C, C + ","}, "", 0},                // not mirrored
		{2, []string{"," + P, "," + P}, "minecraft:stick", 4}, // any position
		{3, []string{",,", ",," + P, ",," + P}, "minecraft:stick", 4},
		{2, []string{P + "," + P, P + ","}, "", 0}, // pickaxe can't fit 2×2
		{3, []string{P + ",,", "," + S + ",", "," + S + ","}, "", 0},
	}
	for i, c := range cases {
		got := b.Match(grid(reg, c.w, c.rows...))
		if key(got) != c.want || got != nil && got.Count != c.n {
			t.Errorf("case %d: got %v, want %s×%d", i, got, c.want, c.n)
		}
	}
}

func TestShapelessAndOrder(t *testing.T) {
	reg, b, _ := setup(t)
	g := grid(reg, 3, ",,", ",minecraft:log#3,", ",,")
	if r := b.Match(g); key(r) != "minecraft:planks" || r.Count != 4 || r.Damage != 0 {
		t.Fatalf("planks: %v", r)
	}
	g = grid(reg, 3, "minecraft:log,minecraft:log,", ",,", ",,")
	if r := b.Match(g); r != nil {
		t.Fatal("two logs matched a one-log recipe")
	}
	b.SortVanilla()
	if !b.Recipes[len(b.Recipes)-1].Shapeless() || b.Recipes[0].Size() != 9 {
		t.Fatal("SortVanilla order")
	}
}

func TestRepair(t *testing.T) {
	reg, b, _ := setup(t)
	pick := reg.Get("minecraft:wooden_pickaxe")
	g := inventory.NewGrid(2, 2)
	g.Slots[0] = &inventory.Stack{Item: pick, Count: 1, Damage: 30}
	g.Slots[3] = &inventory.Stack{Item: pick, Count: 1, Damage: 40}
	// (59-30) + (59-40) + 59*5/100 = 29+19+2 = 50 left → damage 9
	if r := b.Match(g); r == nil || r.Damage != 9 {
		t.Fatalf("repair: %v", r)
	}
	b.NoRepair = true
	if b.Match(g) != nil {
		t.Fatal("repair with NoRepair")
	}
}

func TestWorkbenchEndToEnd(t *testing.T) {
	reg, b, _ := setup(t)
	p := &inventory.Player{}
	c, cr := inventory.NewWorkbench(p, b.Match)
	p.Cursor = &inventory.Stack{Item: reg.Get("minecraft:log"), Count: 10, Damage: 2}
	c.Click(5, 0, inventory.Pickup)
	c.Click(0, 0, inventory.QuickMove)
	// 10 logs → 40 planks in the hotbar (merged from the right end)
	if p.Main[8] == nil || p.Main[8].Count != 40 || cr.Grid.Slots[4] != nil {
		t.Fatalf("hotbar 8: %v", p.Main[8])
	}
}

func TestFurnace(t *testing.T) {
	reg, _, sm := setup(t)
	xp := 0
	f := &Furnace{Recipes: sm, Rand: rand.New(rand.NewPCG(1, 2)), OnXP: func(n int) { xp += n }}
	f.Slots[0] = inventory.New(reg.Get("minecraft:iron_ore"), 10)
	f.Slots[1] = inventory.New(reg.Get("minecraft:coal"), 2)
	if !f.Tick() || f.ItemBurn != 1600 || f.Slots[1].Count != 1 {
		t.Fatal("coal not lit")
	}
	for range 198 {
		f.Tick()
	}
	if f.Slots[2] != nil || f.Cook != 199 {
		t.Fatalf("cook %d", f.Cook)
	}
	f.Tick()
	if f.Slots[2] == nil || f.Slots[2].Count != 1 {
		t.Fatal("first ingot after 200 ticks")
	}
	// one coal smelts 8 items: burn runs out after 1600 ticks
	for range 1600 - 200 {
		f.Tick()
	}
	if f.Slots[2].Count != 8 || f.Slots[1].Count != 1 {
		t.Fatalf("after one coal: %d ingots, fuel %d", f.Slots[2].Count, f.Slots[1].Count)
	}
	// container: shift-click the output, XP 10×0.7 = 7
	for range 400 {
		f.Tick()
	}
	if f.Slots[2].Count != 10 || f.Slots[0] != nil {
		t.Fatalf("all smelted: %d", f.Slots[2].Count)
	}
	p := &inventory.Player{}
	c := inventory.NewFurnaceContainer(p, f, f)
	c.Click(2, 0, inventory.QuickMove)
	if xp != 7 || p.Main[8] == nil || p.Main[8].Count != 10 {
		t.Fatalf("xp %d, hotbar %v", xp, p.Main[8])
	}
	// cobble 0.1 xp: 5 items give 0 or 1
	if v := SmeltXP(5, 0.1, f.Rand); v != 0 && v != 1 {
		t.Fatal(v)
	}
	if SmeltXP(3, 1.0, nil) != 3 || SmeltXP(3, 2.0, nil) != 3 {
		t.Fatal("xp >= 1 yields the count in 1.7.10")
	}
	// furnace stops when no fuel and keeps lit state honest
	f.Slots[0] = inventory.New(reg.Get("minecraft:cobblestone"), 1)
	f.Slots[1] = nil
	for f.Burning() {
		f.Tick()
	}
	if f.Cook != 0 {
		t.Fatal("cook progress kept without fuel")
	}
}
