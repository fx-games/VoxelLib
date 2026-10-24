package inventory

// Recipe is one merchant trade (MerchantRecipe): Buy, and BuyB if set,
// for Sell, until Uses reaches MaxUses.
type Recipe struct {
	Buy, BuyB, Sell *Stack
	Uses, MaxUses   int
}

// NewRecipe is the three-stack constructor: seven uses. The payment
// counts are capped at their stack limits (the payment slots hold no
// more).
func NewRecipe(buy, buyB, sell *Stack) *Recipe {
	for _, s := range []*Stack{buy, buyB} {
		if s != nil {
			s.Count = min(s.Count, s.MaxStack())
		}
	}
	return &Recipe{Buy: buy, BuyB: buyB, Sell: sell, MaxUses: 7}
}

// SameIDs is hasSameIDsAs: the same items bought and sold, counts aside.
func (r *Recipe) SameIDs(o *Recipe) bool {
	if r.Buy.Item != o.Buy.Item || r.Sell.Item != o.Sell.Item {
		return false
	}
	return r.BuyB == nil && o.BuyB == nil || r.BuyB != nil && o.BuyB != nil && r.BuyB.Item == o.BuyB.Item
}

// SameItems is hasSameItemsAs: SameIDs and r asks less of a payment.
func (r *Recipe) SameItems(o *Recipe) bool {
	return r.SameIDs(o) && (r.Buy.Count < o.Buy.Count || r.BuyB != nil && r.BuyB.Count < o.BuyB.Count)
}

// Disabled is isRecipeDisabled: used up.
func (r *Recipe) Disabled() bool { return r.Uses >= r.MaxUses }

// Recipes is a MerchantRecipeList.
type Recipes []*Recipe

// Find is canRecipeBeUsed: with 0 < i < len the recipe at i if a and b
// pay for it, else the first recipe they pay for; nil for none.
func (l Recipes) Find(a, b *Stack, i int) *Recipe {
	pays := func(r *Recipe) bool {
		if a.Item != r.Buy.Item || a.Count < r.Buy.Count {
			return false
		}
		if r.BuyB == nil {
			return b == nil
		}
		return b != nil && b.Item == r.BuyB.Item && b.Count >= r.BuyB.Count
	}
	if i > 0 && i < len(l) {
		if pays(l[i]) {
			return l[i]
		}
		return nil
	}
	for _, r := range l {
		if pays(r) {
			return r
		}
	}
	return nil
}

// AddWithCheck is addToListWithCheck: a recipe with the same items
// replaces one asking more, is dropped beside one asking no more, and is
// appended otherwise.
func (l *Recipes) AddWithCheck(r *Recipe) {
	for i, o := range *l {
		if r.SameIDs(o) {
			if r.SameItems(o) {
				(*l)[i] = r
			}
			return
		}
	}
	*l = append(*l, r)
}

// Merchant is InventoryMerchant: slots 0 and 1 the payment, 2 the
// result of the trade they pay for.
type Merchant struct {
	Slots [3]*Stack
	// Recipes is getRecipes; Shown is the recipe index the screen shows.
	Recipes func() Recipes
	Shown   int
	// Current is the recipe the result belongs to.
	Current *Recipe
	// Result, if set, is told the result after each reset (the
	// merchant's yes and no sounds).
	Result func(*Stack)
}

func (m *Merchant) Size() int          { return 3 }
func (m *Merchant) Get(i int) *Stack   { return m.Slots[i] }
func (m *Merchant) Limit() int         { return 64 }
func (m *Merchant) Changed()           { m.Reset() }
func (m *Merchant) Set(i int, s *Stack) {
	m.Slots[i] = s
	if s != nil && s.Count > m.Limit() {
		s.Count = m.Limit()
	}
	if i < 2 {
		m.Reset()
	}
}

// Take is decrStackSize: the result goes whole.
func (m *Merchant) Take(i, n int) *Stack {
	if i == 2 {
		s := m.Slots[2]
		m.Slots[2] = nil
		return s
	}
	return take(m, i, n)
}

// Show is setCurrentRecipeIndex.
func (m *Merchant) Show(i int) { m.Shown = i; m.Reset() }

// Reset is resetRecipeAndSlots: the result of what the payment slots
// pay for, trying them both ways round.
func (m *Merchant) Reset() {
	m.Current = nil
	a, b := m.Slots[0], m.Slots[1]
	if a == nil {
		a, b = b, nil
	}
	if a == nil {
		m.Slots[2] = nil
	} else if m.Recipes != nil {
		if l := m.Recipes(); l != nil {
			r := l.Find(a, b, m.Shown)
			if (r == nil || r.Disabled()) && b != nil {
				r = l.Find(b, a, m.Shown)
			}
			m.Slots[2] = nil
			if r != nil && !r.Disabled() {
				m.Current = r
				m.Slots[2] = r.Sell.Copy()
			}
		}
	}
	if m.Result != nil {
		m.Result(m.Slots[2])
	}
}

// pay is func_75230_a: a and b, in that order, pay for r.
func pay(r *Recipe, a, b *Stack) bool {
	if a == nil || a.Item != r.Buy.Item {
		return false
	}
	if r.BuyB != nil && b != nil && r.BuyB.Item == b.Item {
		a.Count -= r.Buy.Count
		b.Count -= r.BuyB.Count
		return true
	}
	if r.BuyB == nil && b == nil {
		a.Count -= r.Buy.Count
		return true
	}
	return false
}

// NewMerchantContainer is ContainerMerchant: 0 and 1 the payment, 2 the
// result (SlotMerchantResult: taking it pays and runs use, useRecipe),
// 3-29 main, 30-38 hotbar. Closing returns the payment to the player
// (Drop).
func NewMerchantContainer(p *Player, m *Merchant, use func(*Recipe)) *Container {
	c := &Container{Player: p}
	c.Add(&Slot{Inv: m, Index: 0})
	c.Add(&Slot{Inv: m, Index: 1})
	c.Add(&Slot{Inv: m, Index: 2, Output: true, OnTake: func(_ *Container, _ *Stack, _ int) {
		r := m.Current
		if r == nil {
			return
		}
		a, b := m.Slots[0], m.Slots[1]
		if pay(r, a, b) || pay(r, b, a) {
			use(r)
			if a != nil && a.Count <= 0 {
				a = nil
			}
			if b != nil && b.Count <= 0 {
				b = nil
			}
			m.Set(0, a)
			m.Set(1, b)
		}
	}})
	addPlayerSlots(c, p)
	c.Transfer = func(c *Container, i int) *Stack {
		return TransferWith(c, i, func(s *Stack) bool {
			switch {
			case i == 2:
				return c.Merge(s, 3, 39, true)
			case i < 2:
				return c.Merge(s, 3, 39, false)
			case i < 30:
				return c.Merge(s, 30, 39, false)
			}
			return c.Merge(s, 3, 30, false)
		})
	}
	c.OnClose = func(c *Container) {
		for i := range 2 {
			if s := m.Slots[i]; s != nil {
				m.Slots[i] = nil
				c.drop(s)
			}
		}
	}
	return c
}
