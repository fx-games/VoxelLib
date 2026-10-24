package inventory

import "testing"

// A large chest's slots: the upper grid's 27, then the lower's; a
// shift-click that only merges (Transfer) moves stacks between halves.
func TestJoinedTransfer(t *testing.T) {
	up, lo := NewChest(3), NewChest(3)
	j := &Joined{up, lo}
	j.Set(30, New(stone, 5))
	if j.Size() != 54 || lo.Slots[3].Count != 5 || j.Get(30) != lo.Slots[3] {
		t.Fatal("joined slots")
	}
	c := NewChestContainer(&Player{}, j)
	route := func(s *Stack) bool { return c.Merge(s, 0, 27, false) }
	if got := Transfer(c, 30, route); got == nil || got.Count != 5 || lo.Slots[3] != nil || count(up.Slots[0]) != 5 {
		t.Fatalf("transfer %v", got)
	}
	if Transfer(c, 30, route) != nil || Transfer(c, 0, func(*Stack) bool { return false }) != nil {
		t.Fatal("empty slot or refused route moved something")
	}
}
