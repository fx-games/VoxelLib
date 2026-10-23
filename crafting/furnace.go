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
	// Chance is the share of smelts that yield Out (0: every one).
	Chance float32
}

// Smelting is FurnaceRecipes.
type Smelting struct{ List []Smelt }

// Find returns the recipe whose input accepts s, or nil.
func (sm *Smelting) Find(s *inventory.Stack) *Smelt {
	for i := range sm.List {
		if (Ingredient{sm.List[i].In}).Accepts(s) {
			return &sm.List[i]
		}
	}
	return nil
}

// Result returns the smelting result for s, or nil.
func (sm *Smelting) Result(s *inventory.Stack) *inventory.Stack {
	if r := sm.Find(s); r != nil {
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
// inventory.Inventory and an inventory.FurnaceRules. With Lanes set it is a
// multi-lane machine instead (see Lanes).
type Furnace struct {
	Slots    []*inventory.Stack // grown to Size on first use
	Burn     int // furnaceBurnTime: ticks of fuel left
	ItemBurn int // currentItemBurnTime: total of the burning fuel item
	Cook     int // furnaceCookTime: 0..199
	Recipes  *Smelting
	// OnXP receives the experience earned when the player takes output
	// (split it into orbs with survival.XPSplit). Rand drives the fractional
	// part; nil uses the global source.
	OnXP func(xp int)
	Rand *rand.Rand
	// CookTime replaces CookTicks when set (forge-like machines that take
	// longer per item).
	CookTime int
	// Output and Produce replace the recipes when set: Output is the largest
	// result the input can give (it gates smelting and the output slot),
	// Produce the result one smelt actually makes (nil: nothing, the input
	// is used up all the same).
	Output, Produce func(in *inventory.Stack) *inventory.Stack
	// Lanes > 0 makes a machine of that many lanes sharing one fuel slot,
	// all smelting together when the cook time is up (the reference mod's
	// forges and ovens). Slots: with Alloy the alloy row 0..n-1, inputs
	// n..2n-1, outputs 2n..3n-1, fuel 3n; without, inputs 0..n-1, outputs
	// n..2n-1, fuel 2n.
	Lanes int
	// Alloy is the result of an input with the item in its lane's alloy
	// slot (nil: none), tried before the plain result.
	Alloy func(in, alloy *inventory.Stack) *inventory.Stack
	// XPFrom is the recipe list Taken looks the XP up in (nil: Recipes).
	XPFrom *Smelting
}

func (f *Furnace) result(in *inventory.Stack) *inventory.Stack {
	if f.Output != nil {
		return f.Output(in)
	}
	return f.Recipes.Result(in)
}

// Duration is the time to smelt one item.
func (f *Furnace) Duration() int {
	if f.CookTime > 0 {
		return f.CookTime
	}
	return CookTicks
}

func (f *Furnace) Size() int {
	switch {
	case f.Lanes == 0:
		return 3
	case f.Alloy != nil:
		return 3*f.Lanes + 1
	}
	return 2*f.Lanes + 1
}

// Grow sizes Slots for the layout, keeping what is there.
func (f *Furnace) Grow() {
	if n := f.Size(); len(f.Slots) < n {
		f.Slots = append(f.Slots, make([]*inventory.Stack, n-len(f.Slots))...)
	}
}

func (f *Furnace) Get(i int) *inventory.Stack    { f.Grow(); return f.Slots[i] }
func (f *Furnace) Set(i int, s *inventory.Stack) { f.Grow(); f.Slots[i] = s }

// In, Out and Fuel are a lane's input and output slot and the fuel slot;
// AlloySlot is a lane's alloy slot (-1 without an alloy row).
func (f *Furnace) In(lane int) int {
	if f.Lanes == 0 {
		return 0
	}
	if f.Alloy != nil {
		return f.Lanes + lane
	}
	return lane
}

func (f *Furnace) Out(lane int) int {
	if f.Lanes == 0 {
		return 2
	}
	return f.In(lane) + f.Lanes
}

func (f *Furnace) AlloySlot(lane int) int {
	if f.Lanes == 0 || f.Alloy == nil {
		return -1
	}
	return lane
}

func (f *Furnace) Fuel() int {
	if f.Lanes == 0 {
		return 1
	}
	return f.Size() - 1
}
func (f *Furnace) Limit() int                    { return 64 }
func (f *Furnace) Changed()                      {}

// Burning reports furnaceBurnTime > 0 (the lit furnace block).
func (f *Furnace) Burning() bool { return f.Burn > 0 }

func (f *Furnace) canSmelt() bool {
	in := f.Slots[0]
	if in == nil {
		return false
	}
	r := f.result(in)
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
	f.Grow()
	if f.Lanes > 0 {
		return f.tickLanes()
	}
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
			if f.Cook == f.Duration() {
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
	var r *inventory.Stack
	if f.Produce != nil {
		r = f.Produce(f.Slots[0])
	} else {
		r = f.Recipes.Result(f.Slots[0])
	}
	switch {
	case r == nil:
	case f.Slots[2] == nil:
		f.Slots[2] = r.Copy()
	case f.Slots[2].Item == r.Item:
		f.Slots[2].Count += r.Count
	}
	f.Slots[0].Count--
	if f.Slots[0].Count <= 0 {
		f.Slots[0] = nil
	}
}

// Smeltable implements inventory.FurnaceRules.
func (f *Furnace) Smeltable(s *inventory.Stack) bool { return f.result(s) != nil }

// Taken implements inventory.FurnaceRules: SlotFurnace.onCrafting's XP.
func (f *Furnace) Taken(_ *inventory.Container, s *inventory.Stack, n int) {
	r := f.XPFrom
	if r == nil {
		r = f.Recipes
	}
	if r == nil {
		return
	}
	if xp := SmeltXP(n, r.XP(s), f.Rand); xp > 0 && f.OnXP != nil {
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

// tickLanes is the multi-lane updateEntity: fuel is lit when any lane can
// smelt, and every lane smelts at once when the cook time is up.
func (f *Furnace) tickLanes() bool {
	was := f.Burn > 0
	if f.Burn > 0 {
		f.Burn--
	}
	fu := f.Fuel()
	if f.Burn == 0 && f.canAnyLane() {
		f.Burn = burnTime(f.Slots[fu])
		f.ItemBurn = f.Burn
		if fuel := f.Slots[fu]; f.Burn > 0 && fuel != nil {
			if fuel.Count--; fuel.Count == 0 {
				f.Slots[fu] = nil
				if fuel.Item.Remainder != nil {
					f.Slots[fu] = inventory.New(fuel.Item.Remainder, 1)
				}
			}
		}
	}
	if f.Burn > 0 && f.canAnyLane() {
		if f.Cook++; f.Cook == f.Duration() {
			f.Cook = 0
			for l := range f.Lanes {
				f.smeltLane(l)
			}
		}
	} else {
		f.Cook = 0
	}
	return was != (f.Burn > 0)
}

func (f *Furnace) canAnyLane() bool {
	for l := range f.Lanes {
		if f.canLane(l) {
			return true
		}
	}
	return false
}

// fits: the output slot takes r (the result's max stack and the limit).
func (f *Furnace) fits(out, r *inventory.Stack) bool {
	if out == nil {
		return true
	}
	n := out.Count + r.Count
	return out.Same(r) && n <= f.Limit() && n <= r.MaxStack()
}

func (f *Furnace) alloyResult(l int) *inventory.Stack {
	if a := f.AlloySlot(l); a >= 0 && f.Slots[a] != nil {
		return f.Alloy(f.Slots[f.In(l)], f.Slots[a])
	}
	return nil
}

func (f *Furnace) canLane(l int) bool {
	in := f.Slots[f.In(l)]
	if in == nil {
		return false
	}
	out := f.Slots[f.Out(l)]
	if r := f.alloyResult(l); r != nil && f.fits(out, r) {
		return true
	}
	r := f.result(in)
	return r != nil && f.fits(out, r)
}

// smeltLane is the reference's smeltItemInSlot: the alloy is made when its
// result matches the output (the size is not checked again), else the
// plain result.
func (f *Furnace) smeltLane(l int) {
	if !f.canLane(l) {
		return
	}
	i, o := f.In(l), f.Out(l)
	add := func(r *inventory.Stack) {
		if f.Slots[o] == nil {
			f.Slots[o] = r.Copy()
		} else if f.Slots[o].Same(r) {
			f.Slots[o].Count += r.Count
		}
	}
	take := func(j int) {
		if f.Slots[j].Count--; f.Slots[j].Count <= 0 {
			f.Slots[j] = nil
		}
	}
	if r := f.alloyResult(l); r != nil && (f.Slots[o] == nil || f.Slots[o].Same(r)) {
		add(r)
		take(i)
		take(f.AlloySlot(l))
		return
	}
	add(f.result(f.Slots[i]))
	take(i)
}
