package inventory

import "testing"

var (
	stone  = &Item{ID: 1, Key: "minecraft:stone"}
	pearl  = &Item{ID: 368, Key: "minecraft:ender_pearl", MaxStack: 16}
	pick   = &Item{ID: 270, Key: "minecraft:wooden_pickaxe", MaxStack: 1, MaxDamage: 59}
	helmet = &Item{ID: 306, Key: "minecraft:iron_helmet", MaxStack: 1, MaxDamage: 165, Armor: &Armor{Slot: Helmet, Points: 2}}
	boots  = &Item{ID: 309, Key: "minecraft:iron_boots", MaxStack: 1, MaxDamage: 195, Armor: &Armor{Slot: Boots, Points: 2}}
	coal   = &Item{ID: 263, Key: "minecraft:coal", BurnTime: 1600}
)

func count(s *Stack) int {
	if s == nil {
		return 0
	}
	return s.Count
}

func TestAddTopsUpThenFirstEmpty(t *testing.T) {
	p := &Player{}
	p.Main[5] = New(stone, 60)
	s := New(stone, 10)
	if !p.Add(s) || s.Count != 0 {
		t.Fatalf("add failed, left %d", s.Count)
	}
	if p.Main[5].Count != 64 || count(p.Main[0]) != 6 {
		t.Fatalf("slot5=%d slot0=%d", p.Main[5].Count, count(p.Main[0]))
	}
	// damaged tool goes to the first empty slot
	d := &Stack{Item: pick, Count: 1, Damage: 3}
	p.Add(d)
	if p.Main[1] == nil || p.Main[1].Damage != 3 {
		t.Fatal("damaged item not in first empty slot")
	}
	// a full inventory keeps the rest
	for i := range p.Main {
		p.Main[i] = New(pick, 1)
	}
	r := New(stone, 5)
	if p.Add(r) || r.Count != 5 {
		t.Fatal("full inventory accepted items")
	}
}

func TestLeftRightClick(t *testing.T) {
	p := &Player{}
	c, _ := NewPlayerContainer(p, nil)
	p.Main[9] = New(stone, 33) // container slot 9
	c.Click(9, 1, Pickup)      // right click takes half rounded up
	if count(p.Cursor) != 17 || p.Main[9].Count != 16 {
		t.Fatalf("right pickup: cursor %d slot %d", count(p.Cursor), p.Main[9].Count)
	}
	c.Click(10, 1, Pickup) // right click on empty puts one
	if p.Main[10].Count != 1 || p.Cursor.Count != 16 {
		t.Fatal("right place")
	}
	c.Click(9, 0, Pickup) // left click merges all
	if p.Main[9].Count != 32 || p.Cursor != nil {
		t.Fatalf("left merge: %d", p.Main[9].Count)
	}
	// swap different items
	p.Cursor = New(coal, 3)
	c.Click(9, 0, Pickup)
	if p.Main[9].Item != coal || p.Cursor.Item != stone || p.Cursor.Count != 32 {
		t.Fatal("swap")
	}
	// merge respects max stack (16 for pearls)
	p.Cursor = New(pearl, 10)
	p.Main[11] = New(pearl, 10)
	c.Click(11, 0, Pickup)
	if p.Main[11].Count != 16 || p.Cursor.Count != 4 {
		t.Fatalf("pearl merge %d/%d", p.Main[11].Count, p.Cursor.Count)
	}
	// outside: right drops one, left drops all
	var dropped int
	c.Drop = func(s *Stack) { dropped += s.Count }
	c.Click(Outside, 1, Pickup)
	c.Click(Outside, 0, Pickup)
	if dropped != 4 || p.Cursor != nil {
		t.Fatalf("dropped %d", dropped)
	}
}

func TestArmourSlots(t *testing.T) {
	p := &Player{}
	c, _ := NewPlayerContainer(p, nil)
	p.Cursor = New(stone, 1)
	c.Click(5, 0, Pickup)
	if p.Armor[3] != nil {
		t.Fatal("stone accepted as helmet")
	}
	p.Cursor = New(helmet, 1)
	c.Click(6, 0, Pickup) // chestplate slot rejects a helmet
	if p.Armor[2] != nil {
		t.Fatal("helmet in chest slot")
	}
	c.Click(5, 0, Pickup)
	if p.Armor[3] == nil || p.Cursor != nil || p.TotalArmor() != 2 {
		t.Fatal("helmet not worn")
	}
}

func TestShiftClickPlayer(t *testing.T) {
	p := &Player{}
	c, _ := NewPlayerContainer(p, nil)
	// main → hotbar
	p.Main[9] = New(stone, 64)
	p.Main[2] = New(stone, 60)
	c.Click(9, 0, QuickMove)
	if p.Main[2].Count != 64 || count(p.Main[0]) != 60 || p.Main[9] != nil {
		t.Fatalf("main→hotbar: %d %d %v", p.Main[2].Count, count(p.Main[0]), p.Main[9])
	}
	// hotbar → main
	c.Click(36, 0, QuickMove) // hotbar 0
	if p.Main[0] != nil || count(p.Main[9]) != 60 {
		t.Fatal("hotbar→main")
	}
	// armour goes to its slot when empty
	p.Main[20] = New(boots, 1)
	c.Click(20, 0, QuickMove)
	if p.Armor[0] == nil || p.Main[20] != nil {
		t.Fatal("boots not equipped")
	}
	// second boots: armour slot taken → hotbar
	p.Main[21] = New(boots, 1)
	c.Click(21, 0, QuickMove)
	if p.Main[21] != nil || p.Main[0] == nil || p.Main[0].Item != boots {
		t.Fatal("second boots not moved to hotbar")
	}
	// armour slot → main inventory
	c.Click(8, 0, QuickMove)
	if p.Armor[0] != nil || p.Main[10] == nil {
		t.Fatalf("unequip: %v", p.Main[10])
	}
}

func TestShiftClickChest(t *testing.T) {
	p := &Player{}
	ch := NewChest(3)
	c := NewChestContainer(p, ch)
	ch.Slots[0] = New(stone, 64)
	c.Click(0, 0, QuickMove) // chest → player, hotbar end first
	if p.Main[8] == nil || p.Main[8].Count != 64 {
		t.Fatal("chest → last hotbar slot")
	}
	c.Click(27+27+8, 0, QuickMove) // hotbar 8 → chest slot 0
	if ch.Slots[0] == nil || p.Main[8] != nil {
		t.Fatal("player → chest")
	}
	if len(NewChestContainer(p, NewChest(6)).Slots) != 54+36 {
		t.Fatal("double chest size")
	}
}

func TestNumberKeyAndThrow(t *testing.T) {
	p := &Player{}
	c, _ := NewPlayerContainer(p, nil)
	p.Main[9] = New(stone, 5)
	p.Main[3] = New(coal, 2)
	c.Click(9, 3, Swap)
	if p.Main[9].Item != coal || p.Main[3].Item != stone {
		t.Fatal("swap with hotbar 3")
	}
	var dropped int
	c.Drop = func(s *Stack) { dropped += s.Count }
	c.Click(9, 0, Throw)
	c.Click(39, 1, Throw) // hotbar 3, whole stack
	if dropped != 6 || p.Main[9].Count != 1 || p.Main[3] != nil {
		t.Fatalf("throw dropped %d", dropped)
	}
}

func TestDragSplit(t *testing.T) {
	p := &Player{}
	c, _ := NewPlayerContainer(p, nil)
	p.Cursor = New(stone, 64)
	c.Click(Outside, DragButton(DragEven, DragStart), Drag)
	for _, i := range []int{9, 10, 11} {
		c.Click(i, DragButton(DragEven, DragAdd), Drag)
	}
	c.Click(Outside, DragButton(DragEven, DragEnd), Drag)
	if p.Main[9].Count != 21 || p.Main[10].Count != 21 || p.Main[11].Count != 21 || p.Cursor.Count != 1 {
		t.Fatalf("even drag: %d %d %d cursor %d", p.Main[9].Count, p.Main[10].Count, p.Main[11].Count, count(p.Cursor))
	}
	c.Click(Outside, DragButton(DragOne, DragStart), Drag)
	c.Click(12, DragButton(DragOne, DragAdd), Drag)
	c.Click(Outside, DragButton(DragOne, DragEnd), Drag)
	if p.Main[12].Count != 1 || p.Cursor != nil {
		t.Fatal("right drag")
	}
}

func TestDoubleClickCollect(t *testing.T) {
	p := &Player{}
	c, _ := NewPlayerContainer(p, nil)
	p.Main[9] = New(stone, 64) // full stacks are taken last
	p.Main[10] = New(stone, 10)
	p.Main[0] = New(stone, 20)
	p.Cursor = New(stone, 30)
	c.Click(11, 0, PickupAll)
	if p.Cursor.Count != 64 || p.Main[10] != nil || p.Main[0] != nil || p.Main[9].Count != 60 {
		t.Fatalf("collect: cursor %d, 9=%d", p.Cursor.Count, count(p.Main[9]))
	}
}

func TestCraftingResultSlot(t *testing.T) {
	planks := &Item{ID: 5, Key: "minecraft:planks"}
	log := &Item{ID: 17, Key: "minecraft:log"}
	match := func(g *Grid) *Stack {
		n := 0
		for _, s := range g.Slots {
			if s != nil {
				if s.Item != log {
					return nil
				}
				n++
			}
		}
		if n != 1 {
			return nil
		}
		return New(planks, 4)
	}
	p := &Player{}
	c, cr := NewPlayerContainer(p, match)
	crafted := 0
	cr.OnCraft = func(_ *Stack, n int) { crafted += n }
	p.Cursor = New(log, 3)
	c.Click(1, 0, Pickup)
	if c.Slots[0].Stack() == nil {
		t.Fatal("no result")
	}
	c.Click(0, 0, Pickup) // take 4 planks
	c.Click(0, 0, Pickup) // add 4 more to the cursor
	if count(p.Cursor) != 8 || cr.Grid.Slots[0].Count != 1 {
		t.Fatalf("cursor %d grid %d", count(p.Cursor), count(cr.Grid.Slots[0]))
	}
	p.Cursor = nil
	c.Click(0, 0, QuickMove) // crafts the rest
	if cr.Grid.Slots[0] != nil || count(p.Main[8]) != 4 || crafted != 12 {
		t.Fatalf("shift craft: hotbar8=%d crafted=%d", count(p.Main[8]), crafted)
	}
	// the result can't be collected by double click
	p.Cursor = New(log, 1)
	c.Click(1, 0, Pickup)
	p.Cursor = New(planks, 1)
	c.Click(20, 0, PickupAll)
	if c.Slots[0].Stack() == nil || p.Cursor.Count != 5 {
		t.Fatalf("collect touched result: cursor %d", p.Cursor.Count)
	}
}

func TestFurnaceShiftClick(t *testing.T) {
	ore := &Item{ID: 15, Key: "minecraft:iron_ore"}
	p := &Player{}
	f := NewGrid(3, 1)
	c := NewFurnaceContainer(p, f, rules{ore})
	p.Main[9] = New(ore, 5)
	p.Main[10] = New(coal, 5)
	p.Main[11] = New(stone, 5)
	c.Click(3, 0, QuickMove)
	c.Click(4, 0, QuickMove)
	c.Click(5, 0, QuickMove)
	if count(f.Slots[0]) != 5 || count(f.Slots[1]) != 5 || p.Main[0] == nil {
		t.Fatal("furnace routing")
	}
}

type rules struct{ ore *Item }

func (r rules) Smeltable(s *Stack) bool       { return s.Item == r.ore }
func (r rules) Taken(*Container, *Stack, int) {}

func TestDamage(t *testing.T) {
	s := New(pick, 1)
	for range 59 {
		if s.AddDamage(1) {
			t.Fatal("broke early")
		}
	}
	if !s.AddDamage(1) || s.Count != 0 {
		t.Fatal("wooden pickaxe must break on use 60")
	}
}
