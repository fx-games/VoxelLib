package inventory

// Matcher returns the crafting result for a grid, or nil (CraftingManager.findMatchingRecipe).
type Matcher func(g *Grid) *Stack

// Crafting is a crafting grid with its result slot inside a container.
type Crafting struct {
	Grid   *Grid
	Result *Grid // one slot
	Match  Matcher
	// OnCraft, if set, runs once per pickup of the result with the amount
	// produced (SlotCrafting.onCrafting: stats, achievements, quest hooks).
	OnCraft func(s *Stack, n int)
}

// Update recomputes the result (onCraftMatrixChanged).
func (cr *Crafting) Update() {
	var r *Stack
	if cr.Match != nil {
		r = cr.Match(cr.Grid)
	}
	cr.Result.Slots[0] = r
}

func (cr *Crafting) take(c *Container, s *Stack, n int) {
	if cr.OnCraft != nil {
		cr.OnCraft(s, n)
	}
	for i := range cr.Grid.Slots {
		in := cr.Grid.Slots[i]
		if in == nil {
			continue
		}
		Take(cr.Grid, i, 1)
		if in.Item.Remainder == nil {
			continue
		}
		rem := New(in.Item.Remainder, 1)
		if !c.Player.Add(rem) {
			if cr.Grid.Slots[i] == nil {
				cr.Grid.Set(i, rem)
			} else {
				c.drop(rem)
			}
		}
	}
}

func (cr *Crafting) add(c *Container) {
	c.Add(&Slot{Inv: cr.Result, Output: true, OnTake: cr.take})
	for i := range cr.Grid.Slots {
		c.Add(&Slot{Inv: cr.Grid, Index: i})
	}
	cr.Grid.OnChange = cr.Update
	c.CanCollect = func(_ *Stack, sl *Slot) bool { return sl.Inv != Inventory(cr.Result) }
	c.OnClose = func(c *Container) {
		for i, s := range cr.Grid.Slots {
			if s != nil {
				cr.Grid.Slots[i] = nil
				c.drop(s)
			}
		}
		cr.Update()
	}
	cr.Update()
}

func addPlayerSlots(c *Container, p *Player) {
	for i := 9; i < 36; i++ {
		c.Add(&Slot{Inv: p, Index: i})
	}
	for i := range 9 {
		c.Add(&Slot{Inv: p, Index: i})
	}
}

// NewPlayerContainer is ContainerPlayer: slot 0 craft result, 1–4 the 2×2
// grid, 5–8 armour (helmet first), 9–35 main, 36–44 hotbar.
func NewPlayerContainer(p *Player, match Matcher) (*Container, *Crafting) {
	c := &Container{Player: p}
	cr := &Crafting{Grid: NewGrid(2, 2), Result: NewGrid(1, 1), Match: match}
	cr.add(c)
	for i := range 4 {
		c.Add(&Slot{Inv: p, Index: 39 - i, Max: 1, Accept: func(s *Stack) bool {
			return s != nil && s.Item.Armor != nil && s.Item.Armor.Slot == i
		}})
	}
	addPlayerSlots(c, p)
	c.Transfer = func(c *Container, i int) *Stack {
		return transferWith(c, i, func(s *Stack) bool {
			switch {
			case i == 0:
				return c.Merge(s, 9, 45, true)
			case i >= 1 && i < 9:
				return c.Merge(s, 9, 45, false)
			case s.Item.Armor != nil && !c.Slots[5+s.Item.Armor.Slot].Has():
				j := 5 + s.Item.Armor.Slot
				return c.Merge(s, j, j+1, false)
			case i >= 9 && i < 36:
				return c.Merge(s, 36, 45, false)
			case i >= 36 && i < 45:
				return c.Merge(s, 9, 36, false)
			}
			return c.Merge(s, 9, 45, false)
		}, true)
	}
	return c, cr
}

// NewWorkbench is ContainerWorkbench: slot 0 result, 1–9 the 3×3 grid,
// 10–36 main, 37–45 hotbar. Grid items drop on Close.
func NewWorkbench(p *Player, match Matcher) (*Container, *Crafting) {
	c := &Container{Player: p}
	cr := &Crafting{Grid: NewGrid(3, 3), Result: NewGrid(1, 1), Match: match}
	cr.add(c)
	addPlayerSlots(c, p)
	c.Transfer = func(c *Container, i int) *Stack {
		return transferWith(c, i, func(s *Stack) bool {
			switch {
			case i == 0:
				return c.Merge(s, 10, 46, true)
			case i >= 10 && i < 37:
				return c.Merge(s, 37, 46, false)
			case i >= 37 && i < 46:
				return c.Merge(s, 10, 37, false)
			}
			return c.Merge(s, 10, 46, false)
		}, true)
	}
	return c, cr
}

// NewChestContainer is ContainerChest for any rows×9 inventory: chest slots
// first, then main and hotbar. Shift-click moves chest → player (from the
// hotbar end backwards) and player → chest.
func NewChestContainer(p *Player, chest Inventory) *Container {
	c := &Container{Player: p}
	n := chest.Size()
	for i := range n {
		c.Add(&Slot{Inv: chest, Index: i})
	}
	addPlayerSlots(c, p)
	c.Transfer = func(c *Container, i int) *Stack {
		return transferWith(c, i, func(s *Stack) bool {
			if i < n {
				return c.Merge(s, n, len(c.Slots), true)
			}
			return c.Merge(s, 0, n, false)
		}, false)
	}
	return c
}

// FurnaceRules lets a furnace container ask about recipes without
// importing the crafting package.
type FurnaceRules interface {
	Smeltable(*Stack) bool
	// Taken runs when the player takes n smelted items (XP, stats).
	Taken(c *Container, s *Stack, n int)
}

// NewFurnaceContainer is ContainerFurnace: 0 input, 1 fuel, 2 output, 3–29
// main, 30–38 hotbar. Shift-click routes smeltables to the input and fuel
// (Item.BurnTime > 0) to the fuel slot.
func NewFurnaceContainer(p *Player, furnace Inventory, r FurnaceRules) *Container {
	c := &Container{Player: p}
	c.Add(&Slot{Inv: furnace, Index: 0})
	c.Add(&Slot{Inv: furnace, Index: 1})
	c.Add(&Slot{Inv: furnace, Index: 2, Output: true, OnTake: r.Taken})
	addPlayerSlots(c, p)
	c.Transfer = func(c *Container, i int) *Stack {
		return transferWith(c, i, func(s *Stack) bool {
			switch {
			case i == 2:
				return c.Merge(s, 3, 39, true)
			case i == 0 || i == 1:
				return c.Merge(s, 3, 39, false)
			case r.Smeltable(s):
				return c.Merge(s, 0, 1, false)
			case s.Item.BurnTime > 0:
				return c.Merge(s, 1, 2, false)
			case i >= 3 && i < 30:
				return c.Merge(s, 30, 39, false)
			case i >= 30 && i < 39:
				return c.Merge(s, 3, 30, false)
			}
			return true // vanilla: no route, nothing moved, falls through
		}, true)
	}
	return c
}

// NewHorseContainer is ContainerHorseInventory (LOTR's mount containers):
// 0 saddle, 1 armour, then with chest 15 chest slots (inventory indices
// 2..16), then 27 main and 9 hotbar. saddle and armour decide what the
// two gear slots take; armour nil (a mount that wears none) takes nothing.
// Shift-click: armour to slot 1 when empty, saddles to slot 0, the rest
// into the chest.
func NewHorseContainer(p *Player, inv Inventory, chest bool, saddle, armour func(*Stack) bool) *Container {
	c := &Container{Player: p}
	c.Add(&Slot{Inv: inv, Index: 0, Accept: func(s *Stack) bool { return saddle(s) && inv.Get(0) == nil }})
	c.Add(&Slot{Inv: inv, Index: 1, Accept: func(s *Stack) bool { return armour != nil && armour(s) }})
	n := 2
	if chest {
		for i := 2; i < 17; i++ {
			c.Add(&Slot{Inv: inv, Index: i})
		}
		n = 17
	}
	addPlayerSlots(c, p)
	c.Transfer = func(c *Container, i int) *Stack {
		return transferWith(c, i, func(s *Stack) bool {
			switch {
			case i < n:
				return c.Merge(s, n, len(c.Slots), true)
			case c.Slots[1].Accept(s) && !c.Slots[1].Has():
				return c.Merge(s, 1, 2, false)
			case c.Slots[0].Accept(s):
				return c.Merge(s, 0, 1, false)
			}
			return n > 2 && c.Merge(s, 2, n, false)
		}, false)
	}
	return c
}
