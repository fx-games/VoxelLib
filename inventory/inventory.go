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
// emptied. An inventory with its own Take(i, n) decides for itself.
func Take(inv Inventory, i, n int) *Stack {
	if t, ok := inv.(interface{ Take(i, n int) *Stack }); ok {
		return t.Take(i, n)
	}
	return take(inv, i, n)
}

func take(inv Inventory, i, n int) *Stack {
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

// EquipSlot is one equipment slot of a player: what it is called, the
// 16x16 icon (resource location) drawn while it is empty, what it takes
// (nil: anything) and its stack size cap (0: the inventory's limit).
type EquipSlot struct {
	Name   string
	Icon   string
	Accept func(*Stack) bool
	Max    int
}

// PlayerSpec shapes a player inventory: its equipment slots, in inventory
// order (the screen's column lists them last first, top to bottom). Nil
// Equip is DefaultEquip.
type PlayerSpec struct {
	Equip []EquipSlot
}

// DefaultEquip is the 1.7.10 armour inventory: boots, leggings,
// chestplate, helmet (armorInventory 0..3), each taking one piece whose
// Item.Armor.Slot matches.
var DefaultEquip = []EquipSlot{armourSlot(Boots, "boots"), armourSlot(Leggings, "leggings"),
	armourSlot(Chestplate, "chestplate"), armourSlot(Helmet, "helmet")}

func armourSlot(slot int, name string) EquipSlot {
	return EquipSlot{Name: name, Icon: "textures/items/empty_armor_slot_" + name + ".png", Max: 1,
		Accept: func(s *Stack) bool { return s != nil && s.Item.Armor != nil && s.Item.Armor.Slot == slot }}
}

// Player is InventoryPlayer plus the cursor stack: 36 main slots (0–8
// hotbar), the equipment slots of Spec (the default: Equip[0] boots ..
// Equip[3] helmet, as in 1.7.10) and the selected hotbar slot. As an
// Inventory, slots 36 and up are the equipment. The zero Player has the
// default spec; NewPlayer makes one with another.
//
// The events run after the slot was written: OnChange for every slot Set
// (container clicks and Add go through it) whose stack changed, then
// OnEquip for an equipment slot (its index in Equip); OnHeld when Hold
// changes the held hotbar slot. A count changing inside a stack (a merge
// topping it up) is no slot write and fires nothing.
type Player struct {
	Main     [36]*Stack
	Equip    []*Stack
	Spec     PlayerSpec
	Held     int    // hotbar index 0–8
	Cursor   *Stack // stack carried by the mouse
	Creative bool
	// Ender is the player's InventoryEnderChest, shared by every ender
	// chest (nil until EnderChest makes it).
	Ender *Grid

	OnChange func(i int, old, new *Stack)
	OnEquip  func(slot int, old, new *Stack)
	OnHeld   func(old, new int)
}

// NewPlayer makes an empty player inventory with spec's equipment slots.
func NewPlayer(spec PlayerSpec) *Player {
	p := &Player{Spec: spec}
	p.equip()
	return p
}

// EquipSlots is the spec's equipment slots (DefaultEquip when unset).
func (p *Player) EquipSlots() []EquipSlot {
	if p.Spec.Equip == nil {
		return DefaultEquip
	}
	return p.Spec.Equip
}

func (p *Player) equip() []*Stack {
	if p.Equip == nil {
		p.Equip = make([]*Stack, len(p.EquipSlots()))
	}
	return p.Equip
}

// Hold selects hotbar slot i (0–8), firing OnHeld when it changed.
func (p *Player) Hold(i int) {
	old := p.Held
	if p.Held = i; old != i && p.OnHeld != nil {
		p.OnHeld(old, i)
	}
}

// UseHeld is the consumption every item use repeats: unless keep (a
// creative player), n is taken from st, the held stack, and the hotbar
// slot is cleared once st has run out. always makes that check even when
// keep left st as it was (clearing a stack already at 0).
func (p *Player) UseHeld(st *Stack, n int, keep, always bool) {
	if keep {
		if always && st.Count <= 0 {
			p.Set(p.Held, nil)
		}
		return
	}
	if st.Count -= n; st.Count <= 0 {
		p.Set(p.Held, nil)
	}
}

// EnderChest is the player's 27-slot ender inventory, made on first use.
func (p *Player) EnderChest() *Grid {
	if p.Ender == nil {
		p.Ender = NewChest(3)
	}
	return p.Ender
}

func (p *Player) Size() int  { return 36 + len(p.EquipSlots()) }
func (p *Player) Limit() int { return 64 }
func (p *Player) Changed()   {}
func (p *Player) Get(i int) *Stack {
	if i >= 36 {
		return p.equip()[i-36]
	}
	return p.Main[i]
}
func (p *Player) Set(i int, s *Stack) {
	old := p.Get(i)
	if i >= 36 {
		p.Equip[i-36] = s
	} else {
		p.Main[i] = s
	}
	if old == s {
		return
	}
	if p.OnChange != nil {
		p.OnChange(i, old, s)
	}
	if i >= 36 && p.OnEquip != nil {
		p.OnEquip(i-36, old, s)
	}
}

// Current returns the held hotbar stack.
func (p *Player) Current() *Stack {
	if p.Held < 0 || p.Held >= 9 {
		return nil
	}
	return p.Main[p.Held]
}

// ArmorIn returns the worn piece for an armour type (Helmet..Boots) of
// the default spec.
func (p *Player) ArmorIn(slot int) *Stack { return p.Get(39 - slot) }

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
		p.Set(j, s.Copy())
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
		p.Set(j, c)
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
			p.Set(i, s.Copy())
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
	for _, s := range p.Equip {
		t += s.ArmorValue()
	}
	return t
}

// DropAll empties main and equipment slots through drop (death without keepInventory).
func (p *Player) DropAll(drop func(*Stack)) {
	for i := range p.Size() {
		if s := p.Get(i); s != nil {
			drop(s)
			p.Set(i, nil)
		}
	}
}
