package ui

import "github.com/memmaker/voxellib/inventory"

// ContainerView is a ContainerModel over an inventory.Container: Item
// turns a stack (nil for an empty slot) into what its slot shows.
type ContainerView struct {
	C    *inventory.Container
	Item func(*inventory.Stack) ItemStack
}

// NewContainerView wraps c; drop (if not nil) becomes c.Drop, receiving
// what the player throws out of the window.
func NewContainerView(c *inventory.Container, item func(*inventory.Stack) ItemStack, drop func(*inventory.Stack)) *ContainerView {
	if drop != nil {
		c.Drop = drop
	}
	return &ContainerView{C: c, Item: item}
}

func (v *ContainerView) Slot(i int) ItemStack {
	if i < 0 || i >= len(v.C.Slots) {
		return ItemStack{}
	}
	return v.Item(v.C.Slots[i].Stack())
}

func (v *ContainerView) Cursor() ItemStack { return v.Item(v.C.Player.Cursor) }

func (v *ContainerView) Click(i, button int, shift bool) {
	mode := inventory.Pickup
	if shift {
		mode = inventory.QuickMove
	}
	if i == SlotOutside {
		i = inventory.Outside
	}
	v.C.Click(i, button, mode)
}

func (v *ContainerView) Drag(slots []int, button int) {
	kind := inventory.DragEven
	if button == 1 {
		kind = inventory.DragOne
	}
	v.C.Click(inventory.Outside, inventory.DragButton(kind, inventory.DragStart), inventory.Drag)
	for _, s := range slots {
		v.C.Click(s, inventory.DragButton(kind, inventory.DragAdd), inventory.Drag)
	}
	v.C.Click(inventory.Outside, inventory.DragButton(kind, inventory.DragEnd), inventory.Drag)
}

var _ ContainerModel = (*ContainerView)(nil)
