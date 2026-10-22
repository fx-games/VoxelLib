package crafting

import (
	"math"
	"math/rand/v2"

	"github.com/memmaker/voxellib/inventory"
)

// Smelt is one furnace recipe.
type Smelt struct {
	In  Option
	Out *inventory.Stack
	XP  float32
}

// Smelting is FurnaceRecipes.
type Smelting struct{ List []Smelt }

func (sm *Smelting) find(s *inventory.Stack) *Smelt {
	for i := range sm.List {
		if (Ingredient{sm.List[i].In}).Accepts(s) {
			return &sm.List[i]
		}
	}
	return nil
}

// Result returns the smelting result for s, or nil.
func (sm *Smelting) Result(s *inventory.Stack) *inventory.Stack {
	if r := sm.find(s); r != nil {
		return r.Out
	}
	return nil
}

// XP returns the experience per smelted item s (looked up by the output, as
// func_151398_b does).
func (sm *Smelting) XP(out *inventory.Stack) float32 {
	for _, r := range sm.List {
		if (Ingredient{{r.Out.Item, r.Out.Damage}}).Accepts(out) {
			return r.XP
		}
	}
	return 0
}

// CookTicks is the time to smelt one item.
const CookTicks = 200

// Furnace is TileEntityFurnace: slot 0 input, 1 fuel, 2 output. It is an
// inventory.Inventory and an inventory.FurnaceRules.
type Furnace struct {
	Slots    [3]*inventory.Stack
	Burn     int // furnaceBurnTime: ticks of fuel left
	ItemBurn int // currentItemBurnTime: total of the burning fuel item
	Cook     int // furnaceCookTime: 0..199
	Recipes  *Smelting
	// OnXP receives the experience earned when the player takes output
	// (split it into orbs with survival.XPSplit). Rand drives the fractional
	// part; nil uses the global source.
	OnXP func(xp int)
	Rand *rand.Rand
}

func (f *Furnace) Size() int                     { return 3 }
func (f *Furnace) Get(i int) *inventory.Stack    { return f.Slots[i] }
func (f *Furnace) Set(i int, s *inventory.Stack) { f.Slots[i] = s }
func (f *Furnace) Limit() int                    { return 64 }
func (f *Furnace) Changed()                      {}

// Burning reports furnaceBurnTime > 0 (the lit furnace block).
func (f *Furnace) Burning() bool { return f.Burn > 0 }

func (f *Furnace) canSmelt() bool {
	in := f.Slots[0]
	if in == nil {
		return false
	}
	r := f.Recipes.Result(in)
	if r == nil {
		return false
	}
	out := f.Slots[2]
	if out == nil {
		return true
	}
	if !out.Same(r) {
		return false
	}
	n := out.Count + r.Count
	return n <= f.Limit() && n <= out.MaxStack()
}

// Tick runs one updateEntity. It returns true when the lit state changed.
func (f *Furnace) Tick() (litChanged bool) {
	was := f.Burn > 0
	if f.Burn > 0 {
		f.Burn--
	}
	if f.Burn != 0 || f.Slots[1] != nil && f.Slots[0] != nil {
		if f.Burn == 0 && f.canSmelt() {
			f.Burn = burnTime(f.Slots[1])
			f.ItemBurn = f.Burn
			if f.Burn > 0 && f.Slots[1] != nil {
				fuel := f.Slots[1]
				fuel.Count--
				if fuel.Count == 0 {
					f.Slots[1] = nil
					if fuel.Item.Remainder != nil {
						f.Slots[1] = inventory.New(fuel.Item.Remainder, 1)
					}
				}
			}
		}
		if f.Burn > 0 && f.canSmelt() {
			f.Cook++
			if f.Cook == CookTicks {
				f.Cook = 0
				f.smelt()
			}
		} else {
			f.Cook = 0
		}
	}
	return was != (f.Burn > 0)
}

func burnTime(s *inventory.Stack) int {
	if s == nil {
		return 0
	}
	return s.Item.BurnTime
}

func (f *Furnace) smelt() {
	r := f.Recipes.Result(f.Slots[0])
	if f.Slots[2] == nil {
		f.Slots[2] = r.Copy()
	} else if f.Slots[2].Item == r.Item {
		f.Slots[2].Count += r.Count
	}
	f.Slots[0].Count--
	if f.Slots[0].Count <= 0 {
		f.Slots[0] = nil
	}
}

// Smeltable implements inventory.FurnaceRules.
func (f *Furnace) Smeltable(s *inventory.Stack) bool { return f.Recipes.Result(s) != nil }

// Taken implements inventory.FurnaceRules: SlotFurnace.onCrafting's XP.
func (f *Furnace) Taken(_ *inventory.Container, s *inventory.Stack, n int) {
	if xp := SmeltXP(n, f.Recipes.XP(s), f.Rand); xp > 0 && f.OnXP != nil {
		f.OnXP(xp)
	}
}

// SmeltXP is the XP for taking n items worth xp each. As in 1.7.10, a value
// of 1 or more yields n (not n·xp); below 1 the fraction is rolled.
func SmeltXP(n int, xp float32, rng *rand.Rand) int {
	switch {
	case xp == 0:
		return 0
	case xp < 1:
		v := float32(n) * xp
		j := int(math.Floor(float64(v)))
		r := rand.Float32
		if rng != nil {
			r = rng.Float32
		}
		if j < int(math.Ceil(float64(v))) && r() < v-float32(j) {
			j++
		}
		return j
	}
	return n
}
