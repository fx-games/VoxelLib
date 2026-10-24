package inventory

import "testing"

// TestOffHand: the swap key trades the held stack with the off hand;
// Add fills the off hand after the held slot; UseHands falls through to
// the off hand and passes a hand on cooldown; the container's last slot
// is the off hand and swap button 40 reaches it.
func TestOffHand(t *testing.T) {
	shield := &Item{Key: "t:shield", MaxStack: 1, Blocks: Shield()}
	stone := &Item{Key: "t:stone"}
	p := NewPlayer(PlayerSpec{OffHand: true})
	p.Main[0] = &Stack{Item: shield, Count: 1}
	if !p.SwapHands() || p.Off == nil || p.Off.Item != shield || p.Main[0] != nil {
		t.Fatal("swap")
	}
	p.Main[0] = &Stack{Item: stone, Count: 64}
	p.Off = &Stack{Item: stone, Count: 10}
	if !p.Add(&Stack{Item: stone, Count: 5}) || p.Off.Count != 15 {
		t.Fatalf("add: off %d", p.Off.Count)
	}
	p.Off = &Stack{Item: shield, Count: 1}
	use := func(hand int, st *Stack) bool { return st != nil && st.Item.Blocks != nil }
	var got []int
	if !p.UseHands(func(h int, st *Stack) bool { got = append(got, h); return use(h, st) }) || len(got) != 2 || got[1] != OffHand {
		t.Fatalf("fall-through: %v", got)
	}
	p.Cool.Add(shield, 10)
	if p.UseHands(use) {
		t.Fatal("used on cooldown")
	}
	c, _ := NewPlayerContainer(p, nil)
	last := len(c.Slots) - 1
	if c.Slots[last].Index != p.OffSlot() || p.Get(p.OffSlot()) != p.Off {
		t.Fatal("off-hand slot")
	}
	c.Click(9+27, SwapOffHand, Swap) // hotbar 0 <-> off hand
	if p.Off.Item != stone || p.Main[0].Item != shield {
		t.Fatal("swap button 40")
	}
	if NewPlayer(PlayerSpec{}).SwapHands() {
		t.Fatal("no off hand swaps")
	}
}
