package items

import (
	"math/rand/v2"
	"testing"

	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/survival"
	"github.com/memmaker/voxellib/voxel/collide"
)

func TestStackPickUp(t *testing.T) {
	stone := &inventory.Item{ID: 1, Key: "test:stone"}
	key := func(*inventory.Stack) string { return "stone" }
	r := rand.New(rand.NewPCG(1, 2))
	var w World
	w.TossStack(inventory.New(stone, 3), key, 0.5, 1, 0.5, 0, 0, true, r)
	w.TossStack(nil, key, 0.5, 1, 0.5, 0, 0, true, r)
	w.Spill([]*inventory.Stack{inventory.New(stone, 2)}, key, 0, 0, 0, r)
	w.Spawn("bare", 0.5, 0, 0.5) // no stack: removed
	if len(w.Drops) != 3 || w.Drops[0].PickupDelay != 40 || w.Drops[1].PickupDelay != 10 {
		t.Fatalf("%d drops", len(w.Drops))
	}
	for _, d := range w.Drops {
		d.PickupDelay = 0
	}
	inv := &inventory.Player{}
	body := collide.Box(0.5, 0, 0.5, 0.6, 1.8)
	w.PickUp(body, false, inv, 6000, nil, nil)
	if len(w.Drops) != 2 || inv.Main[0] != nil {
		t.Fatal("dead player picked up, or the bare drop stayed")
	}
	total := 0
	w.PickUp(body, true, inv, 6000, nil, func(_ *Drop, _ *inventory.Stack, n int) { total += n })
	if len(w.Drops) != 0 || total != 5 || inv.Main[0].Count != 5 {
		t.Fatalf("picked %d, %d left", total, len(w.Drops))
	}
	var orbs Orbs
	xp := survival.XP{Level: 2}
	DeathDrops(true, inv, &xp, &w, &orbs, key, 0, 0, 0, r)
	if inv.Main[0] == nil || xp.Level != 2 {
		t.Fatal("keepInventory dropped")
	}
	DeathDrops(false, inv, &xp, &w, &orbs, key, 0, 0, 0, r)
	if inv.Main[0] != nil || xp.Level != 0 || len(w.Drops) != 1 || len(orbs.List) == 0 {
		t.Fatal("death drops")
	}
}
