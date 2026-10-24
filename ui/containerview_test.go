package ui

import (
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

// ContainerView turns GUI clicks and drags into slotClick calls.
func TestContainerView(t *testing.T) {
	apple := &inventory.Item{ID: 260, Key: "test:apple"}
	p := &inventory.Player{}
	chest := inventory.NewChest(3)
	chest.Slots[0] = inventory.New(apple, 10)
	var dropped []*inventory.Stack
	v := NewContainerView(inventory.NewChestContainer(p, chest), func(s *inventory.Stack) ItemStack {
		if s == nil {
			return ItemStack{}
		}
		return ItemStack{Count: s.Count}
	}, func(s *inventory.Stack) { dropped = append(dropped, s) })
	if v.Slot(0).Count != 10 || v.Slot(-1).Count != 0 || v.Slot(99).Count != 0 {
		t.Fatal("slots")
	}
	v.Click(0, 0, false) // pick up 10 apples
	if v.Cursor().Count != 10 {
		t.Fatal("cursor")
	}
	v.Drag([]int{2, 3}, 0)
	if p.Cursor != nil || chest.Slots[2].Count != 5 || chest.Slots[3].Count != 5 {
		t.Fatalf("drag: cursor %v, %v %v", p.Cursor, chest.Slots[2], chest.Slots[3])
	}
	v.Click(3, 0, true) // shift: into the player inventory
	if chest.Slots[3] != nil {
		t.Error("shift-click did not move the apples")
	}
	v.Click(2, 0, false)
	v.Click(SlotOutside, 1, false)
	if len(dropped) != 1 || dropped[0].Count != 1 || p.Cursor.Count != 4 {
		t.Errorf("drop one: %v, cursor %v", dropped, p.Cursor)
	}
}
