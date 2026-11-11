package main

import (
	"testing"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/inventory"
	firearm "github.com/memmaker/voxellib/weapon"
)

// TestGunStackState: a gun's rounds and fire mode live on its stack and
// travel with copies of it.
func TestGunStackState(t *testing.T) {
	s := inventory.New(&inventory.Item{Key: "tacz:test", MaxStack: 1}, 1)
	if _, ok := gunAmmo(s); ok {
		t.Fatal("a new gun has rounds set")
	}
	setGunAmmo(s, 7)
	setGunMode(s, firearm.Auto)
	c := s.Copy()
	setGunAmmo(s, 3)
	if n, _ := gunAmmo(c); n != 7 {
		t.Errorf("copy has %d rounds, want 7", n)
	}
	if m, ok := gunMode(c); !ok || m != firearm.Auto {
		t.Errorf("copy's mode %v %v", m, ok)
	}
}

// TestAmmoStacks: reloads take rounds across stacks in slot order and
// empty stacks leave their slots.
func TestAmmoStacks(t *testing.T) {
	g := &game{App: app.Headless()}
	g.itemReg = inventory.NewRegistry()
	it, _ := g.itemReg.Register(&inventory.Item{Key: "tacz:9mm", MaxStack: 60})
	g.inv().Hold(4) // Add tops up nothing held; the stacks go to slots 0, 1
	if left := g.giveAmmo("tacz:9mm", 100); left != 0 {
		t.Fatalf("%d rounds did not fit", left)
	}
	if g.ammoCount(it.Key) != 100 {
		t.Fatalf("count %d", g.ammoCount(it.Key))
	}
	g.takeAmmo(it.Key, 70)
	if g.inv().Main[0] != nil || g.inv().Main[1].Count != 30 {
		t.Fatalf("slots %v %v", g.inv().Main[0], g.inv().Main[1])
	}
}
