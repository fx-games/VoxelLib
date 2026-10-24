package inventory

import "testing"

// TestMerchant: the result follows the payment either way round, taking
// it pays and counts a use, a used-up trade shows nothing, the shown
// index picks among trades and closing hands the payment back.
func TestMerchant(t *testing.T) {
	gem := &Item{ID: 388, Key: "test:emerald"}
	l := Recipes{NewRecipe(New(stone, 20), nil, New(gem, 1)), NewRecipe(New(gem, 3), New(coal, 1), New(pick, 1))}
	l.AddWithCheck(NewRecipe(New(stone, 18), nil, New(gem, 1))) // cheaper: replaces
	l.AddWithCheck(NewRecipe(New(stone, 30), nil, New(gem, 1))) // dearer: dropped
	if len(l) != 2 || l[0].Buy.Count != 18 {
		t.Fatalf("AddWithCheck: %d recipes, first buys %d", len(l), l[0].Buy.Count)
	}
	if r := NewRecipe(New(pearl, 40), nil, New(gem, 1)); r.Buy.Count != 16 || r.MaxUses != 7 {
		t.Fatalf("NewRecipe cap: %d %d", r.Buy.Count, r.MaxUses)
	}
	var used []*Recipe
	var told int
	m := &Merchant{Recipes: func() Recipes { return l }, Result: func(*Stack) { told++ }}
	p := &Player{}
	var dropped []*Stack
	c := NewMerchantContainer(p, m, func(r *Recipe) { used = append(used, r); r.Uses++ })
	c.Drop = func(s *Stack) { dropped = append(dropped, s) }
	m.Set(1, New(stone, 40)) // only the second slot: still pays
	if m.Current != l[0] || m.Slots[2] == nil || m.Slots[2].Item != gem || told == 0 {
		t.Fatalf("result %v", m.Slots[2])
	}
	c.Click(2, 0, Pickup)
	if len(used) != 1 || m.Slots[1].Count != 22 || p.Cursor == nil || p.Cursor.Item != gem {
		t.Fatalf("trade: used %d, left %d, cursor %v", len(used), m.Slots[1].Count, p.Cursor)
	}
	p.Cursor = nil
	l[0].Uses = l[0].MaxUses
	m.Reset()
	if m.Slots[2] != nil {
		t.Fatal("a used-up trade still shows its result")
	}
	m.Set(0, New(gem, 3))
	m.Set(1, New(coal, 2))
	m.Show(1)
	if m.Current != l[1] {
		t.Fatal("shown trade not picked")
	}
	c.Click(2, 0, QuickMove) // shift-click trades into the inventory
	if len(used) != 2 || m.Slots[0] != nil || m.Slots[1].Count != 1 || p.Main[8] == nil {
		t.Fatalf("shift trade: used %d, %v %v", len(used), m.Slots[0], m.Slots[1])
	}
	c.Close()
	if len(dropped) != 1 || m.Slots[1] != nil {
		t.Fatalf("close dropped %d", len(dropped))
	}
}
