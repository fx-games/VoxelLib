package ui

import (
	"testing"

	"github.com/memmaker/voxellib/inventory"
)

// TestOffHandLayout: the off-hand slot is the layout's last, at
// InventoryMenu's 77, 62, with the shield slot icon.
func TestOffHandLayout(t *testing.T) {
	lay := InventoryLayout(InventoryOptions{OffHand: true})
	i := len(lay.Slots) - 1
	if lay.Slots[i] != [2]int{77, 62} || lay.SlotIcons[i] != inventory.OffHandIcon || i != 45 {
		t.Fatalf("slot %d at %v icon %q", i, lay.Slots[i], lay.SlotIcons[i])
	}
}
