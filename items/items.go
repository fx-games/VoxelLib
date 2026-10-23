// Package items simulates items lying in the world and the "one hand, no
// inventory" pickup rule used by the explorer: walking over an item puts it
// in your hand, and whatever you held is dropped where you stand.
//
// Items are plain string IDs (the explorer uses sprite names such as
// "sword_03"); how they look is the renderer's business (see
// render.ItemRenderer and items/extrude). Physics follow the reference's
// ItemEntity: gravity 0.04/tick, 0.98 air drag, ground friction 0.6.
// The package depends only on voxel/collide.
package items

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Drop is an item entity lying in (or falling through) the world.
type Drop struct {
	Item                string
	X, Y, Z             float64 // bottom centre
	VX, VY, VZ          float64
	PrevX, PrevY, PrevZ float64
	OnGround            bool
	Age                 int // ticks, drives bob and spin
	// PickupDelay ticks must pass before the drop can be collected; a drop
	// with WaitLeave is only collectable once the collector stopped touching
	// it, so a swapped-out item doesn't jump straight back into the hand.
	PickupDelay int
	WaitLeave   bool
	// Payload is the game's item data (e.g. a whole stack with count,
	// damage and tags); Item stays the key that draws it.
	Payload any
}

// Size of a drop's collision box (the reference: 0.25).
const Size = 0.25

// Box returns the drop's collision box.
func (d *Drop) Box() collide.AABB { return collide.Box(d.X, d.Y, d.Z, Size, Size) }

// Bob height: the centre floats hoverLift above the drop's feet and bobs
// ±hoverBob, so a sprite drawn up to 0.5 tall (the reference's drop size)
// keeps its lowest point 0.05 above the ground.
const (
	hoverBob  = 0.1
	hoverLift = 0.25 + hoverBob + 0.05
)

// Pose returns the interpolated render position, the bob-raised height of
// the item's centre and its spin angle in radians.
func (d *Drop) Pose(alpha float64) (x, y, z, spin float64) {
	t := float64(d.Age) + alpha
	x = d.PrevX + (d.X-d.PrevX)*alpha
	y = d.PrevY + (d.Y-d.PrevY)*alpha + hoverLift + hoverBob*math.Sin(t/10)
	z = d.PrevZ + (d.Z-d.PrevZ)*alpha
	return x, y, z, t / 20
}

// World holds the drops.
type World struct {
	Drops []*Drop
}

// Spawn adds a drop at rest.
func (w *World) Spawn(item string, x, y, z float64) *Drop {
	d := &Drop{Item: item, X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Age: len(w.Drops) * 37}
	w.Drops = append(w.Drops, d)
	return d
}

// Toss drops an item from a holder at eye position (x,y,z) looking along
// yaw/pitch (radians, as character.Input), with the reference's throw speed.
func (w *World) Toss(item string, x, y, z, yaw, pitch float64) *Drop {
	d := w.Spawn(item, x, y-0.3, z)
	d.VX = math.Sin(yaw) * math.Cos(pitch) * 0.3
	d.VZ = -math.Cos(yaw) * math.Cos(pitch) * 0.3
	d.VY = math.Sin(pitch)*0.3 + 0.1
	d.PickupDelay = 40
	return d
}

// Tick advances every drop by one 50 ms tick. Drops inside solid terrain
// (e.g. a block was placed on them) are pushed up.
func (w *World) Tick(s collide.Solids) {
	for _, d := range w.Drops {
		d.PrevX, d.PrevY, d.PrevZ = d.X, d.Y, d.Z
		d.Age++
		if d.PickupDelay > 0 {
			d.PickupDelay--
		}
		if !collide.Fits(s, d.Box()) {
			d.Y += 0.1
			d.VY = 0
			continue
		}
		d.VY -= 0.04
		mx, my, mz := collide.Move(s, d.Box(), d.VX, d.VY, d.VZ)
		d.OnGround = d.VY < 0 && my > d.VY
		if mx != d.VX {
			d.VX = 0
		}
		if mz != d.VZ {
			d.VZ = 0
		}
		if my != d.VY {
			d.VY = 0
		}
		d.X, d.Y, d.Z = d.X+mx, d.Y+my, d.Z+mz
		f := 0.98
		if d.OnGround {
			f *= 0.6
		}
		d.VX *= f
		d.VY *= 0.98
		d.VZ *= f
	}
}

// Collect applies the pickup rule for a collector with box body holding
// *held ("" for an empty hand). The first touching drop goes into the hand
// and the previously held item is dropped at (x,y,z). It returns the picked
// item, or "" if nothing was picked up.
func (w *World) Collect(body collide.AABB, held *string, x, y, z float64) string {
	reach := body.Grow(1, 0.5, 1) // the reference's pickup range
	for i, d := range w.Drops {
		touch := reach.Intersects(d.Box())
		if d.WaitLeave {
			if !touch {
				d.WaitLeave = false
			}
			continue
		}
		if !touch || d.PickupDelay > 0 {
			continue
		}
		w.Drops = append(w.Drops[:i], w.Drops[i+1:]...)
		old := *held
		*held = d.Item
		if old != "" {
			n := w.Spawn(old, x, y, z)
			n.WaitLeave = true
		}
		return d.Item
	}
	return ""
}
