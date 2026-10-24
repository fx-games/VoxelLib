package items

import (
	"math"
	"math/rand/v2"

	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/survival"
	"github.com/memmaker/voxellib/voxel/collide"
)

// Drops of inventory stacks: Payload holds the *inventory.Stack and Item
// its key (the renderer's name for it).

// Spill drops stacks from a broken block at x, y, z
// (Block.dropBlockAsItem: offset rand*0.7+0.15, pickup delay 10).
func (w *World) Spill(sts []*inventory.Stack, key func(*inventory.Stack) string, x, y, z int, r *rand.Rand) {
	for _, st := range sts {
		d := w.Spawn(key(st), float64(x)+r.Float64()*0.7+0.15, float64(y)+r.Float64()*0.7+0.15,
			float64(z)+r.Float64()*0.7+0.15)
		d.Payload, d.PickupDelay = st, 10
	}
}

// TossStack throws st (named by key) from x, y, z as EntityPlayer.dropItem
// does (pickup delay 40): along yaw, pitch as Toss, or with scatter (a
// death drop) a random way at up to 0.5 per tick. Nothing is thrown for a
// nil or empty stack.
func (w *World) TossStack(st *inventory.Stack, key func(*inventory.Stack) string, x, y, z, yaw, pitch float64, scatter bool, r *rand.Rand) {
	if st == nil || st.Count <= 0 {
		return
	}
	var d *Drop
	if scatter {
		d = w.Spawn(key(st), x, y, z)
		f := r.Float64() * 0.5
		a := r.Float64() * 2 * math.Pi
		d.VX, d.VZ, d.VY = -math.Sin(a)*f, math.Cos(a)*f, 0.2
	} else {
		d = w.Toss(key(st), x, y, z, yaw, pitch)
	}
	d.Payload, d.PickupDelay = st, 40
}

// DeathDrops is EntityPlayer.onDeath's drops at the player's feet x, y,
// z: without keep (the keepInventory game rule) every stack of inv is
// tossed scattered from y+1.32 (inventory.dropAllItems), orbs gets the
// XP's death drop (min(level*7, 100)) and xp resets; with keep the player
// keeps both (getExperiencePoints 0, clonePlayer).
func DeathDrops(keep bool, inv *inventory.Player, xp *survival.XP, w *World, orbs *Orbs,
	key func(*inventory.Stack) string, x, y, z float64, r *rand.Rand) {
	if keep {
		return
	}
	inv.DropAll(func(st *inventory.Stack) { w.TossStack(st, key, x, y+1.32, z, 0, 0, true, r) })
	orbs.Drop(x, y, z, xp.DeathDrop())
	*xp = survival.XP{}
}

// PickUp is EntityItem's lifespan and onCollideWithPlayer for stack
// drops: a drop without a stack, or lifespan ticks old, is removed. When
// alive, a drop without a pickup delay touching body grown by (1, 0.5, 1)
// goes into inv: before runs first (pickup hooks that may take some of
// it), got after n > 0 items moved, and the drop is removed once empty.
func (w *World) PickUp(body collide.AABB, alive bool, inv *inventory.Player, lifespan int,
	before func(d *Drop, st *inventory.Stack), got func(d *Drop, st *inventory.Stack, n int)) {
	box := body.Grow(1, 0.5, 1)
	live := w.Drops[:0]
	for _, d := range w.Drops {
		st, _ := d.Payload.(*inventory.Stack)
		if st == nil || d.Age >= lifespan {
			continue
		}
		if d.PickupDelay == 0 && alive && box.Intersects(d.Box()) {
			n := st.Count
			if before != nil {
				before(d, st)
			}
			inv.Add(st)
			if k := n - st.Count; k > 0 && got != nil {
				got(d, st, k)
			}
			if st.Count <= 0 {
				continue
			}
		}
		live = append(live, d)
	}
	clear(w.Drops[len(live):])
	w.Drops = live
}
