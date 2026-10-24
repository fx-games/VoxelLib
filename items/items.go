// Package items simulates items lying in the world and the "one hand, no
// inventory" pickup rule used by the explorer: walking over an item puts it
// in your hand, and whatever you held is dropped where you stand.
//
// Items are plain string IDs (the explorer uses sprite names such as
// "sword_03"); how they look is the renderer's business (see
// render.ItemRenderer and items/extrude). Physics follow the reference's
// ItemEntity: gravity 0.04/tick, 0.98 air drag, ground friction 0.6.
// Stack drops (stacks.go) carry inventory stacks: block spills, thrown
// and death drops, and pickup into a player inventory.
package items

import (
	"math"
	"math/rand/v2"

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
	// Wounds is the health lost of Health; Fire the burn ticks left.
	Wounds, Fire int
}

// Health is an item entity's health (5).
const Health = 5

// Hurt is the reference's attackEntityFrom for an item: health =
// (int)(health - amount); at 0 it is gone. It reports the death.
func (d *Drop) Hurt(amount float64) bool {
	h := int(float32(Health-d.Wounds) - float32(amount))
	d.Wounds = Health - h
	return d.Dead()
}

// Dead reports a drop hurt to death.
func (d *Drop) Dead() bool { return d.Wounds >= Health }

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
	// Flow, if set, is the current's push on a box (character.WaterPush),
	// added at the start of each drop's tick as Entity.onUpdate does.
	Flow func(collide.AABB) (vx, vy, vz float64)
	// The hazards, each optional, asked of a block: Wet (water: it puts
	// a burning drop out), Lava (handleLavaMovement: 4 damage and 15
	// seconds alight), Burning (fire or lava: 1 a tick and 8 seconds
	// alight unless wet) and Prick (a block's contact damage, a cactus'
	// 1). A burning drop takes 1 every 20 ticks; at 0 health it is gone.
	Wet, Lava, Burning func(x, y, z int) bool
	Prick              func(x, y, z int) float64
	// Rand and Sound, if set, bounce a drop in lava with random.fizz.
	Rand  *rand.Rand
	Sound func(name string, x, y, z float64, volume, pitch float32)
}

// inBox reports whether f holds for a block in b as World.isMaterialInBB
// visits them (floor(min) to floor(max + 1), exclusive).
func inBox(b collide.AABB, f func(x, y, z int) bool) bool {
	if f == nil {
		return false
	}
	for y := int(math.Floor(b.MinY)); y < int(math.Floor(b.MaxY+1)); y++ {
		for z := int(math.Floor(b.MinZ)); z < int(math.Floor(b.MaxZ+1)); z++ {
			for x := int(math.Floor(b.MinX)); x < int(math.Floor(b.MaxX+1)); x++ {
				if f(x, y, z) {
					return true
				}
			}
		}
	}
	return false
}

// prick is doBlockCollisions over the box shrunk 0.001 (both ends
// inclusive): the contact damage of each block.
func (w *World) prick(d *Drop) {
	if w.Prick == nil {
		return
	}
	b := d.Box().Grow(-0.001, -0.001, -0.001)
	for x := int(math.Floor(b.MinX)); x <= int(math.Floor(b.MaxX)); x++ {
		for y := int(math.Floor(b.MinY)); y <= int(math.Floor(b.MaxY)); y++ {
			for z := int(math.Floor(b.MinZ)); z <= int(math.Floor(b.MaxZ)); z++ {
				if n := w.Prick(x, y, z); n > 0 {
					d.Hurt(n)
				}
			}
		}
	}
}

// hazards is the entity base tick's water, burning and lava: the box
// shrunk by (0, 0.4, 0) and 0.001 for water, by (0.1, 0.4, 0.1) for lava
// (inside out for a drop's 0.25 box, as in the reference).
func (w *World) hazards(d *Drop) bool {
	b := d.Box()
	wet := inBox(collide.AABB{MinX: b.MinX + 0.001, MinY: b.MinY + 0.401, MinZ: b.MinZ + 0.001, MaxX: b.MaxX - 0.001, MaxY: b.MaxY - 0.401, MaxZ: b.MaxZ - 0.001}, w.Wet)
	if wet {
		d.Fire = 0
	}
	if d.Fire > 0 {
		if d.Fire%20 == 0 {
			d.Hurt(1)
		}
		d.Fire--
	}
	if inBox(b.Grow(-0.1, -0.4, -0.1), w.Lava) {
		d.Hurt(4)
		d.Fire = max(d.Fire, 15*20)
	}
	return wet
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
	live := w.Drops[:0]
	for _, d := range w.Drops {
		if w.tick(d, s); !d.Dead() {
			live = append(live, d)
		}
	}
	clear(w.Drops[len(live):])
	w.Drops = live
}

func (w *World) tick(d *Drop, s collide.Solids) {
	d.PrevX, d.PrevY, d.PrevZ = d.X, d.Y, d.Z
	if w.Flow != nil {
		vx, vy, vz := w.Flow(d.Box())
		d.VX, d.VY, d.VZ = d.VX+vx, d.VY+vy, d.VZ+vz
	}
	wet := w.hazards(d)
	if d.Dead() {
		return
	}
	d.Age++
	if d.PickupDelay > 0 {
		d.PickupDelay--
	}
	if !collide.Fits(s, d.Box()) {
		d.Y += 0.1
		d.VY = 0
		return
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
	w.prick(d)
	if inBox(d.Box().Grow(-0.001, -0.001, -0.001), w.Burning) { // moveEntity: dealFireDamage(1)
		d.Hurt(1)
		if !wet {
			if d.Fire++; d.Fire == 0 {
				d.Fire = 8 * 20
			}
		}
	} else if d.Fire <= 0 {
		d.Fire = -1 // -fireResistance
	}
	if wet && d.Fire > 0 && w.Sound != nil && w.Rand != nil {
		w.Sound("random.fizz", d.X, d.Y, d.Z, 0.7, 1.6+(w.Rand.Float32()-w.Rand.Float32())*0.4)
		d.Fire = -1
	}
	if (int(d.PrevX) != int(d.X) || int(d.PrevY+0.125) != int(d.Y+0.125) || int(d.PrevZ) != int(d.Z) || d.Age%25 == 0) &&
		w.Lava != nil && w.Lava(int(math.Floor(d.X)), int(math.Floor(d.Y+0.125)), int(math.Floor(d.Z))) && w.Rand != nil {
		d.VY = 0.20000000298023224
		d.VX = float64((w.Rand.Float32() - w.Rand.Float32()) * 0.2)
		d.VZ = float64((w.Rand.Float32() - w.Rand.Float32()) * 0.2)
		if w.Sound != nil {
			w.Sound("random.fizz", d.X, d.Y, d.Z, 0.4, 2+w.Rand.Float32()*0.4)
		}
	}
	f := 0.98
	if d.OnGround {
		f *= 0.6
	}
	d.VX *= f
	d.VY *= 0.98
	d.VZ *= f
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
