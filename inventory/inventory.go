package inventory

// Inventory is IInventory: a fixed row of slots.
type Inventory interface {
	Size() int
	Get(i int) *Stack
	Set(i int, s *Stack)
	Limit() int // max stack size per slot (64)
	Changed()   // markDirty
}

// Take removes up to n items from slot i of inv (IInventory.decrStackSize):
// if the slot holds n or fewer, the slot's own stack is returned and the slot
// emptied.
func Take(inv Inventory, i, n int) *Stack {
	s := inv.Get(i)
	if s == nil {
		return nil
	}
	if s.Count <= n {
		inv.Set(i, nil)
		inv.Changed()
		return s
	}
	t := s.Split(n)
	if s.Count == 0 {
		inv.Set(i, nil)
	}
	inv.Changed()
	return t
}

// Grid is a plain inventory (a chest, a crafting matrix). W and H matter for
// crafting grids only. OnChange, if set, runs after every change (the
// crafting grid recomputes its result through it).
type Grid struct {
	Slots    []*Stack
	W, H     int
	OnChange func()
}

// NewGrid returns an empty grid of w×h slots.
func NewGrid(w, h int) *Grid { return &Grid{Slots: make([]*Stack, w*h), W: w, H: h} }

// NewChest returns a chest inventory of rows×9 slots (3 single, 6 double).
func NewChest(rows int) *Grid { return NewGrid(9, rows) }

func (g *Grid) Size() int        { return len(g.Slots) }
func (g *Grid) Get(i int) *Stack { return g.Slots[i] }
func (g *Grid) Limit() int       { return 64 }
func (g *Grid) Set(i int, s *Stack) {
	if s != nil && s.Count > g.Limit() {
		s.Count = g.Limit()
	}
	g.Slots[i] = s
	g.Changed()
}
func (g *Grid) Changed() {
	if g.OnChange != nil {
		g.OnChange()
	}
}

// At returns the stack in column x, row y, or nil outside the grid
// (InventoryCrafting.getStackInRowAndColumn).
func (g *Grid) At(x, y int) *Stack {
	if x < 0 || y < 0 || x >= g.W || y >= g.H {
		return nil
	}
	return g.Slots[x+y*g.W]
}

// Player is InventoryPlayer plus the cursor stack: 36 main slots (0–8 hotbar),
// 4 armour slots (Armor[0] boots .. Armor[3] helmet, as in 1.7.10) and the
// selected hotbar slot. As an Inventory, slots 36–39 are the armour.
type Player struct {
	Main     [36]*Stack
	Armor    [4]*Stack
	Held     int    // hotbar index 0–8
	Cursor   *Stack // stack carried by the mouse
	Creative bool
}

func (p *Player) Size() int  { return 40 }
func (p *Player) Limit() int { return 64 }
func (p *Player) Changed()   {}
func (p *Player) Get(i int) *Stack {
	if i >= 36 {
		return p.Armor[i-36]
	}
	return p.Main[i]
}
func (p *Player) Set(i int, s *Stack) {
	if i >= 36 {
		p.Armor[i-36] = s
		return
	}
	p.Main[i] = s
}

// Current returns the held hotbar stack.
func (p *Player) Current() *Stack {
	if p.Held < 0 || p.Held >= 9 {
		return nil
	}
	return p.Main[p.Held]
}

// ArmorIn returns the worn piece for an armour type (Helmet..Boots).
func (p *Player) ArmorIn(slot int) *Stack { return p.Armor[3-slot] }

// FirstEmpty returns the first empty main slot or -1.
func (p *Player) FirstEmpty() int {
	for i, s := range p.Main {
		if s == nil {
			return i
		}
	}
	return -1
}

func (p *Player) storeSlot(s *Stack) int {
	for i, m := range p.Main {
		if m != nil && m.Item == s.Item && m.Stackable() && m.Count < m.MaxStack() && m.Count < p.Limit() &&
			(!m.Item.HasSubtypes || m.Damage == s.Damage) && TagsEqual(m, s) {
			return i
		}
	}
	return -1
}

func (p *Player) storePartial(s *Stack) int {
	n := s.Count
	if s.MaxStack() == 1 {
		j := p.FirstEmpty()
		if j < 0 {
			return n
		}
		p.Main[j] = s.Copy()
		return 0
	}
	j := p.storeSlot(s)
	if j < 0 {
		j = p.FirstEmpty()
	}
	if j < 0 {
		return n
	}
	if p.Main[j] == nil {
		c := s.Copy()
		c.Count = 0
		p.Main[j] = c
	}
	m := p.Main[j]
	k := min(n, m.MaxStack()-m.Count, p.Limit()-m.Count)
	if k == 0 {
		return n
	}
	m.Count += k
	return n - k
}

// Add puts s into the main inventory (InventoryPlayer.addItemStackToInventory):
// damaged items take the first empty slot, others top up matching stacks
// first. s.Count is reduced by what fitted; it returns true if anything did.
func (p *Player) Add(s *Stack) bool {
	if s == nil || s.Count == 0 || s.Item == nil {
		return false
	}
	if s.Damaged() {
		if i := p.FirstEmpty(); i >= 0 {
			p.Main[i] = s.Copy()
			s.Count = 0
			return true
		}
		if p.Creative {
			s.Count = 0
			return true
		}
		return false
	}
	var n int
	for {
		n = s.Count
		s.Count = p.storePartial(s)
		if s.Count <= 0 || s.Count >= n {
			break
		}
	}
	if s.Count == n && p.Creative {
		s.Count = 0
		return true
	}
	return s.Count < n
}

// TotalArmor sums the worn armour points (getTotalArmorValue).
func (p *Player) TotalArmor() int {
	t := 0
	for _, s := range p.Armor {
		t += s.ArmorValue()
	}
	return t
}

// DropAll empties main and armour slots through drop (death without keepInventory).
func (p *Player) DropAll(drop func(*Stack)) {
	for i := range p.Main {
		if p.Main[i] != nil {
			drop(p.Main[i])
			p.Main[i] = nil
		}
	}
	for i := range p.Armor {
		if p.Armor[i] != nil {
			drop(p.Armor[i])
			p.Armor[i] = nil
		}
	}
}
