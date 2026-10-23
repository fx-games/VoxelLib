package items

import (
	"math"
	"math/rand/v2"

	"github.com/memmaker/voxellib/survival"
	"github.com/memmaker/voxellib/voxel/collide"
)

// Orb is an experience orb, the reference's XP orb entity: 0.5 wide, its
// position the centre of its box.
type Orb struct {
	X, Y, Z             float64
	VX, VY, VZ          float64
	PrevX, PrevY, PrevZ float64
	OnGround            bool
	Value               int
	Color               int // xpColor: ticks lived, drives the render tint
	Age                 int // despawns at OrbLife
	Delay               int // ticks before it can be picked up
	Health              int
	Fire                int
	id, targetColor     int
	target              bool
}

// OrbLife is the age at which an orb vanishes.
const OrbLife = 6000

// Box is the orb's 0.5 box around its centre.
func (o *Orb) Box() collide.AABB {
	return collide.AABB{MinX: o.X - 0.25, MinY: o.Y - 0.25, MinZ: o.Z - 0.25, MaxX: o.X + 0.25, MaxY: o.Y + 0.25, MaxZ: o.Z + 0.25}
}

// Icon is the orb's cell on the 4×4 orb sheet by value.
func (o *Orb) Icon() int {
	for i, v := range []int{2477, 1237, 617, 307, 149, 73, 37, 17, 7, 3} {
		if o.Value >= v {
			return 10 - i
		}
	}
	return 0
}

// Tint is the render colour at partial tick pt: red and blue pulse with
// xpColor, green stays full.
func (o *Orb) Tint(pt float64) (r, g, b float32) {
	c := float32((float64(o.Color) + pt) / 2)
	sin := func(x float32) float32 { return float32(math.Sin(float64(x))) }
	return float32(int((sin(c)+1)*0.5*255)) / 255, 1, float32(int((sin(c+4.1887903)+1)*0.1*255)) / 255
}

// Orbs holds the experience orbs and the world facts their tick needs.
type Orbs struct {
	List []*Orb
	Rand *rand.Rand // made on first use if nil
	// Flow is the current's push on a box (water also puts out a burning orb).
	Flow func(collide.AABB) (vx, vy, vz float64, wet bool)
	// Lava reports a lava block; Slip is a block's slipperiness (0.6 if nil).
	Lava  func(x, y, z int) bool
	Slip  func(x, y, z int) float64
	Sound func(name string, x, y, z float64, vol, pitch float32)
	next  int
}

// Spawn adds an orb worth value at x, y, z with the reference's random toss.
func (w *Orbs) Spawn(x, y, z float64, value int) *Orb {
	if w.Rand == nil {
		w.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	r := w.Rand
	o := &Orb{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Value: value, Health: 5, id: w.next}
	w.next++
	o.VX = float64(float32(r.Float64()*0.20000000298023224-0.10000000149011612) * 2)
	o.VY = float64(float32(r.Float64()*0.2) * 2)
	o.VZ = float64(float32(r.Float64()*0.20000000298023224-0.10000000149011612) * 2)
	w.List = append(w.List, o)
	return o
}

// Drop spawns n experience as orbs split by XP orb size, as a dying mob, a
// broken block or a furnace output does.
func (w *Orbs) Drop(x, y, z float64, n int) {
	for n > 0 {
		v := survival.XPSplit(n)
		n -= v
		w.Spawn(x, y, z, v)
	}
}

// Player is what an orb tick needs of the player: feet position, eye
// height and whether it can collect.
type Player struct {
	X, Y, Z, Eye float64
	Alive        bool
}

func (w *Orbs) lava(b collide.AABB) bool {
	if w.Lava == nil {
		return false
	}
	hit := false
	b.Blocks(func(x, y, z int) { hit = hit || w.Lava(x, y, z) })
	return hit
}

// Tick advances every orb by one tick (the entity base tick's water and
// fire, then the orb's onUpdate) and drops the dead ones.
func (w *Orbs) Tick(s collide.Solids, p Player) {
	live := w.List[:0]
	for _, o := range w.List {
		if w.tick(o, s, p) {
			live = append(live, o)
		}
	}
	clear(w.List[len(live):])
	w.List = live
}

func ifloor(v float64) int { return int(math.Floor(v)) }

func (w *Orbs) tick(o *Orb, s collide.Solids, p Player) bool {
	r := w.Rand
	// Entity base tick: water, burning, lava.
	if w.Flow != nil {
		vx, vy, vz, wet := w.Flow(o.Box())
		o.VX, o.VY, o.VZ = o.VX+vx, o.VY+vy, o.VZ+vz
		if wet {
			o.Fire = 0
		}
	}
	if o.Fire > 0 {
		if o.Fire%20 == 0 {
			o.Health--
		}
		o.Fire--
	}
	if w.lava(o.Box().Grow(-0.1, -0.4, -0.1)) {
		o.Health -= 4
		o.Fire = max(o.Fire, 300)
	}
	if o.Health <= 0 {
		return false
	}
	if o.Delay > 0 {
		o.Delay--
	}
	o.PrevX, o.PrevY, o.PrevZ = o.X, o.Y, o.Z
	o.VY -= 0.029999999329447746
	if w.Lava != nil && w.Lava(ifloor(o.X), ifloor(o.Y), ifloor(o.Z)) {
		o.VY = 0.20000000298023224
		o.VX = float64((r.Float32() - r.Float32()) * 0.2)
		o.VZ = float64((r.Float32() - r.Float32()) * 0.2)
		if w.Sound != nil {
			w.Sound("random.fizz", o.X, o.Y, o.Z, 0.4, 2+r.Float32()*0.4)
		}
	}
	w.pushOut(o, s)
	const reach = 8.0
	if o.targetColor < o.Color-20+o.id%100 {
		if !o.target || dist2(o, p) > reach*reach {
			o.target = p.Alive && dist2(o, p) < reach*reach
		}
		o.targetColor = o.Color
	}
	if o.target {
		dx, dy, dz := (p.X-o.X)/reach, (p.Y+p.Eye-o.Y)/reach, (p.Z-o.Z)/reach
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if f := 1 - d; f > 0 {
			f *= f
			o.VX += dx / d * f * 0.1
			o.VY += dy / d * f * 0.1
			o.VZ += dz / d * f * 0.1
		}
	}
	mx, my, mz := collide.Move(s, o.Box(), o.VX, o.VY, o.VZ)
	o.OnGround = my != o.VY && o.VY < 0
	if mx != o.VX {
		o.VX = 0
	}
	if my != o.VY {
		o.VY = 0
	}
	if mz != o.VZ {
		o.VZ = 0
	}
	o.X, o.Y, o.Z = o.X+mx, o.Y+my, o.Z+mz
	f := float32(0.98)
	if o.OnGround {
		slip := 0.6
		if w.Slip != nil {
			slip = w.Slip(ifloor(o.X), ifloor(o.Y-0.25)-1, ifloor(o.Z))
		}
		f = float32(slip) * 0.98
	}
	o.VX *= float64(f)
	o.VY *= 0.9800000190734863
	o.VZ *= float64(f)
	if o.OnGround {
		o.VY *= -0.8999999761581421
	}
	o.Color++
	o.Age++
	return o.Age < OrbLife
}

func dist2(o *Orb, p Player) float64 {
	dx, dy, dz := p.X-o.X, p.Y-o.Y, p.Z-o.Z
	return dx*dx + dy*dy + dz*dz
}

// pushOut is the entity push out of blocks at the box's centre: an orb
// stuck in a block or overlapping a solid box is kicked toward the nearest
// open side (up by default).
func (w *Orbs) pushOut(o *Orb, s collide.Solids) {
	x, y, z := ifloor(o.X), ifloor(o.Y), ifloor(o.Z)
	if collide.Fits(s, o.Box()) && !s.Solid(x, y, z) {
		return
	}
	fx, fy, fz := o.X-float64(x), o.Y-float64(y), o.Z-float64(z)
	side, best := 3, 9999.0
	try := func(open bool, d float64, i int) {
		if open && d < best {
			best, side = d, i
		}
	}
	try(!s.Solid(x-1, y, z), fx, 0)
	try(!s.Solid(x+1, y, z), 1-fx, 1)
	try(!s.Solid(x, y+1, z), 1-fy, 3)
	try(!s.Solid(x, y, z-1), fz, 4)
	try(!s.Solid(x, y, z+1), 1-fz, 5)
	v := float64(w.Rand.Float32()*0.2 + 0.1)
	switch side {
	case 0:
		o.VX = -v
	case 1:
		o.VX = v
	case 3:
		o.VY = v
	case 4:
		o.VZ = -v
	case 5:
		o.VZ = v
	}
}

// Collect is the player touching orbs (its box grown 1, 0.5, 1): the first
// ready orb is taken while *cooldown is 0, which it then sets to 2. It
// returns the orb's value, or 0.
func (w *Orbs) Collect(body collide.AABB, cooldown *int) int {
	reach := body.Grow(1, 0.5, 1)
	for i, o := range w.List {
		if o.Delay != 0 || *cooldown != 0 || !reach.Intersects(o.Box()) {
			continue
		}
		*cooldown = 2
		if w.Sound != nil {
			r := w.Rand
			w.Sound("random.orb", o.X, o.Y, o.Z, 0.1, 0.5*((r.Float32()-r.Float32())*0.7+1.8))
		}
		w.List = append(w.List[:i], w.List[i+1:]...)
		return o.Value
	}
	return 0
}
