// Package inventory holds the item rules of a Minecraft 1.7.10 style game:
// an item registry, item stacks, inventories and containers with the exact
// slot click semantics of 1.7.10's Container.slotClick.
//
// It is pure Go (no rendering) and knows no game content: the game
// registers its items and wires drops, crafting and furnaces through
// callbacks. World drops and their pickup physics live in package items;
// a game hands a picked-up stack to Player.Add.
package inventory

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
)

// Wildcard is the "any meta" value of 1.7.10 (OreDictionary.WILDCARD_VALUE).
const Wildcard = 32767

// Armour slots, as ItemArmor.armorType.
const (
	Helmet = iota
	Chestplate
	Leggings
	Boots
)

// Tool describes how an item digs. It mirrors ItemTool plus Forge's tool
// classes: a block whose harvest tool is Class is dug at Efficiency and is
// harvestable when Level >= the block's harvest level.
type Tool struct {
	Class      string  // "pickaxe", "axe", "shovel", "sword", "shears", ...
	Level      int     // harvest level (0 wood/gold, 1 stone, 2 iron, 3 diamond)
	Efficiency float32 // dig speed on effective blocks
	// Materials are block materials the tool is also effective on (vanilla
	// func_150893_a: pickaxe rock/iron/anvil, axe wood/plants/vine).
	Materials []string
	// BlockSpeed overrides the speed for single block keys or materials
	// (sword on web 15, shears on leaves 15 and wool 5).
	BlockSpeed map[string]float32
	// Harvests lists materials or block keys the tool can harvest even if the
	// block names no harvest tool (vanilla func_150897_b: pickaxe rock, sword web).
	Harvests []string
	// BreakDamage is the durability lost when breaking a block with hardness
	// != 0 (tools 1, swords 2); HitDamage when hitting an entity (tools 2, swords 1).
	BreakDamage, HitDamage int
}

// Armor is the ItemArmor part of an item.
type Armor struct {
	Slot   int // Helmet .. Boots
	Points int // damageReduceAmount
}

// Food is the ItemFood part of an item.
type Food struct {
	Hunger       int     // healAmount
	Saturation   float32 // saturationModifier
	AlwaysEdible bool
	UseTicks     int // 0 means 32
}

// Ctx is passed to an item's hooks. User and Target are the game's entities.
type Ctx struct {
	Stack        *Stack
	User, Target any
	X, Y, Z      int // block position, if any
	Side         int
}

// Item is one registered item type.
type Item struct {
	ID          int
	Key         string // registry name, e.g. "minecraft:stone_pickaxe"
	MaxStack    int    // 0 means 64
	MaxDamage   int    // durability; 0 = not damageable
	HasSubtypes bool   // meta is a subtype, not damage
	NoRepair    bool   // excluded from the two-item repair recipe
	Tool        *Tool
	Armor       *Armor
	Food        *Food
	Attack      float32 // attack damage modifier (sword 4+material, tools base+material)
	BurnTime    int     // furnace fuel ticks
	// Remainder is left in the crafting grid after crafting (bucket for milk).
	Remainder *Item
	// Hooks the game sets. OnUse returns true if it handled the right click.
	OnUse func(*Ctx) bool
	OnHit func(*Ctx)
	OnEat func(*Ctx)
}

// StackLimit returns the item's max stack size.
func (it *Item) StackLimit() int {
	if it.MaxStack <= 0 {
		return 64
	}
	return it.MaxStack
}

// Damageable reports Item.isDamageable: durability and no subtypes.
func (it *Item) Damageable() bool { return it.MaxDamage > 0 && !it.HasSubtypes }

// Registry maps keys and numeric ids to items.
type Registry struct {
	byKey map[string]*Item
	byID  map[int]*Item
	next  int
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byKey: map[string]*Item{}, byID: map[int]*Item{}, next: 1}
}

// Register adds it; an ID of 0 is assigned the next free id.
func (r *Registry) Register(it *Item) (*Item, error) {
	if _, dup := r.byKey[it.Key]; dup {
		return nil, fmt.Errorf("inventory: item %q registered twice", it.Key)
	}
	if it.ID == 0 {
		for r.byID[r.next] != nil {
			r.next++
		}
		it.ID = r.next
	} else if r.byID[it.ID] != nil {
		return nil, fmt.Errorf("inventory: item id %d of %q already used", it.ID, it.Key)
	}
	r.byKey[it.Key] = it
	r.byID[it.ID] = it
	return it, nil
}

// Get returns the item with this key, or nil.
func (r *Registry) Get(key string) *Item { return r.byKey[key] }

// ByID returns the item with this numeric id, or nil.
func (r *Registry) ByID(id int) *Item { return r.byID[id] }

// Items returns all items sorted by id.
func (r *Registry) Items() []*Item {
	out := slices.Collect(maps.Values(r.byKey))
	slices.SortFunc(out, func(a, b *Item) int { return a.ID - b.ID })
	return out
}

// Stack is an ItemStack. A nil *Stack is an empty slot. Damage is the meta
// (subtype or durability used). Tag is the NBT-like data (nil = no tag).
type Stack struct {
	Item   *Item
	Count  int
	Damage int
	Tag    map[string]any
}

// New returns a stack of n items with meta 0.
func New(it *Item, n int) *Stack { return &Stack{Item: it, Count: n} }

// Copy returns a deep copy (nil for nil).
func (s *Stack) Copy() *Stack {
	if s == nil {
		return nil
	}
	c := *s
	if s.Tag != nil {
		c.Tag = deepCopy(s.Tag).(map[string]any)
	}
	return &c
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = deepCopy(x)
		}
		return m
	case []any:
		l := make([]any, len(t))
		for i, x := range t {
			l[i] = deepCopy(x)
		}
		return l
	}
	return v
}

// Split takes n items off s into a new stack (ItemStack.splitStack; s may go
// to 0 or below, as in Java).
func (s *Stack) Split(n int) *Stack {
	c := s.Copy()
	c.Count = n
	s.Count -= n
	return c
}

// MaxStack returns the item's stack limit.
func (s *Stack) MaxStack() int { return s.Item.StackLimit() }

// Damageable reports ItemStack.isItemStackDamageable (honours the
// "Unbreakable" tag).
func (s *Stack) Damageable() bool {
	if s.Item.MaxDamage <= 0 {
		return false
	}
	u, _ := s.Tag["Unbreakable"].(bool)
	return !u
}

// Damaged reports a damageable stack with damage > 0.
func (s *Stack) Damaged() bool { return s.Damageable() && s.Damage > 0 }

// Stackable reports ItemStack.isStackable.
func (s *Stack) Stackable() bool { return s.MaxStack() > 1 && (!s.Damageable() || !s.Damaged()) }

// Same reports ItemStack.isItemEqual: same item and meta.
func (s *Stack) Same(o *Stack) bool { return s.Item == o.Item && s.Damage == o.Damage }

// TagsEqual reports ItemStack.areItemStackTagsEqual.
func TagsEqual(a, b *Stack) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Tag == nil {
		return b.Tag == nil
	}
	return reflect.DeepEqual(a.Tag, b.Tag)
}

// Equal reports ItemStack.areItemStacksEqual (count, item, meta, tag).
func Equal(a, b *Stack) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Count == b.Count && a.Same(b) && TagsEqual(a, b)
}

// AddDamage applies n points of wear (ItemStack.damageItem without
// enchantments). It returns true when the stack broke: its count drops by
// one and its damage resets, as in 1.7.10. Creative users should not call it.
func (s *Stack) AddDamage(n int) bool {
	if !s.Damageable() || n <= 0 {
		return false
	}
	s.Damage += n
	if s.Damage <= s.Item.MaxDamage {
		return false
	}
	s.Count--
	if s.Count < 0 {
		s.Count = 0
	}
	s.Damage = 0
	return true
}

// Saved is the serialisable form of a stack.
type Saved struct {
	Item   string         `json:"item"`
	Count  int            `json:"count"`
	Damage int            `json:"damage,omitempty"`
	Tag    map[string]any `json:"tag,omitempty"`
}

// Save returns s in serialisable form.
func (s *Stack) Save() Saved {
	c := s.Copy()
	return Saved{Item: c.Item.Key, Count: c.Count, Damage: c.Damage, Tag: c.Tag}
}

// Load rebuilds a saved stack; it returns nil for an unknown item.
func (r *Registry) Load(v Saved) *Stack {
	it := r.Get(v.Item)
	if it == nil || v.Count <= 0 {
		return nil
	}
	return (&Stack{Item: it, Count: v.Count, Damage: v.Damage, Tag: v.Tag}).Copy()
}
