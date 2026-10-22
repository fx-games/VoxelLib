// Package crafting implements 1.7.10 crafting: shaped recipes (mirrored
// matches), shapeless recipes, the two-item repair combine of
// CraftingManager, and furnace smelting with fuel, the 200-tick cook time and
// XP. Recipes are data: the game loads them from JSON (see Load).
package crafting

import (
	"slices"

	"github.com/memmaker/voxellib/inventory"
)

// Option is one acceptable item of an ingredient; Meta inventory.Wildcard
// accepts any meta.
type Option struct {
	Item *inventory.Item
	Meta int
}

// Ingredient lists the alternatives for one cell (an ore dictionary entry
// has several). A nil Ingredient is an empty cell.
type Ingredient []Option

// Accepts reports whether s satisfies the ingredient (OreDictionary.itemMatches,
// non-strict; vanilla recipes compare the same way).
func (in Ingredient) Accepts(s *inventory.Stack) bool {
	for _, o := range in {
		if o.Item == s.Item && (o.Meta == inventory.Wildcard || o.Meta == s.Damage) {
			return true
		}
	}
	return false
}

// Recipe is a crafting recipe.
type Recipe interface {
	Matches(g *inventory.Grid) bool
	Result(g *inventory.Grid) *inventory.Stack
	Size() int
	Shapeless() bool
}

// Shaped is ShapedRecipes / ShapedOreRecipe. Cells holds W×H ingredients row
// by row. Mirrored recipes also match flipped left-right (all vanilla ones).
type Shaped struct {
	W, H     int
	Cells    []Ingredient
	Out      *inventory.Stack
	Mirrored bool
	KeepTag  bool // copy the tag of a tagged input (func_92100_c)
}

func (r *Shaped) Size() int       { return r.W * r.H }
func (r *Shaped) Shapeless() bool { return false }

func (r *Shaped) Matches(g *inventory.Grid) bool {
	for x := 0; x <= g.W-r.W; x++ {
		for y := 0; y <= g.H-r.H; y++ {
			if r.check(g, x, y, false) || r.Mirrored && r.check(g, x, y, true) {
				return true
			}
		}
	}
	return false
}

func (r *Shaped) check(g *inventory.Grid, ox, oy int, mirror bool) bool {
	for x := range g.W {
		for y := range g.H {
			var want Ingredient
			if i, j := x-ox, y-oy; i >= 0 && j >= 0 && i < r.W && j < r.H {
				if mirror {
					i = r.W - i - 1
				}
				want = r.Cells[i+j*r.W]
			}
			have := g.At(x, y)
			if want == nil && have == nil {
				continue
			}
			if want == nil || have == nil || !want.Accepts(have) {
				return false
			}
		}
	}
	return true
}

func (r *Shaped) Result(g *inventory.Grid) *inventory.Stack {
	out := r.Out.Copy()
	if r.KeepTag {
		for _, s := range g.Slots {
			if s != nil && s.Tag != nil {
				out.Tag = s.Copy().Tag
			}
		}
	}
	return out
}

// Shapeless is ShapelessRecipes / ShapelessOreRecipe.
type Shapeless struct {
	In  []Ingredient
	Out *inventory.Stack
}

func (r *Shapeless) Size() int       { return len(r.In) }
func (r *Shapeless) Shapeless() bool { return true }

func (r *Shapeless) Matches(g *inventory.Grid) bool {
	left := slices.Clone(r.In)
	for y := range g.H {
		for x := range g.W {
			s := g.At(x, y)
			if s == nil {
				continue
			}
			i := slices.IndexFunc(left, func(in Ingredient) bool { return in.Accepts(s) })
			if i < 0 {
				return false
			}
			left = slices.Delete(left, i, i+1)
		}
	}
	return len(left) == 0
}

func (r *Shapeless) Result(*inventory.Grid) *inventory.Stack { return r.Out.Copy() }

// Book is a list of recipes for one kind of crafting table, tried in order.
type Book struct {
	Recipes []Recipe
	// NoRepair turns off the two-item repair combine (LOTR's faction tables
	// still have it; the vanilla table does).
	NoRepair bool
}

// SortVanilla orders the recipes as CraftingManager does: shaped before
// shapeless, larger before smaller (stable otherwise).
func (b *Book) SortVanilla() {
	slices.SortStableFunc(b.Recipes, func(a, c Recipe) int {
		if a.Shapeless() != c.Shapeless() {
			if a.Shapeless() {
				return 1
			}
			return -1
		}
		return c.Size() - a.Size()
	})
}

// Match is CraftingManager.findMatchingRecipe. It is an inventory.Matcher.
func (b *Book) Match(g *inventory.Grid) *inventory.Stack {
	var a, c *inventory.Stack
	n := 0
	for _, s := range g.Slots {
		if s != nil {
			if n == 0 {
				a = s
			} else if n == 1 {
				c = s
			}
			n++
		}
	}
	if !b.NoRepair && n == 2 && a.Item == c.Item && a.Count == 1 && c.Count == 1 && !a.Item.NoRepair && a.Item.Damageable() {
		m := a.Item.MaxDamage
		left := (m - a.Damage) + (m - c.Damage) + m*5/100
		return &inventory.Stack{Item: a.Item, Count: 1, Damage: max(m-left, 0)}
	}
	for _, r := range b.Recipes {
		if r.Matches(g) {
			return r.Result(g)
		}
	}
	return nil
}
