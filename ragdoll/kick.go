// Package ragdoll throws bodies around like Dark Messiah of Might and Magic:
// Kick launches a character controller, Flight follows it through the air
// and reports what it slams into, and Doll flops a humanoid model's limbs
// as a verlet ragdoll around it. It is GPU-free; the game draws the Doll's
// skin matrices and decides what an Impact does.
package ragdoll

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
)

// Kick launches c along yaw (the character's convention: 0 faces -Z) at
// power blocks per tick, hopping it up by lift.
func Kick(c *character.Character, yaw, power, lift float64) {
	c.VX, c.VZ = math.Sin(yaw)*power, -math.Cos(yaw)*power
	c.VY = math.Max(c.VY, lift)
	c.OnGround = false
}

// Impact is a body slamming into the world.
type Impact struct {
	Speed  float64  // blocks per tick, into what it hit
	Wall   bool     // it hit something sideways
	Ground bool     // it landed
	Cells  [][3]int // the blocks it touches, solid or not (spikes, glass, fire)
}

// Flight follows a thrown body until it comes to rest.
type Flight struct {
	Ticks int // ticks in the air so far
}

// MinImpact is the slowest hit Flight reports, in blocks per tick.
const MinImpact = 0.15

// Tick moves c one tick with no input of its own and reports an impact
// (Speed 0 for none) and whether it has come to rest.
func (f *Flight) Tick(c *character.Character, w character.World) (imp Impact, rest bool) {
	vx, vy, vz := c.VX, c.VY, c.VZ
	c.Tick(w, character.Input{})
	f.Ticks++
	if h := math.Hypot(vx, vz); c.HorizontalCollision && h > MinImpact {
		imp.Speed, imp.Wall = h, true
	}
	if c.OnGround && -vy > MinImpact {
		imp.Speed, imp.Ground = math.Max(imp.Speed, -vy), true
	}
	if imp.Speed > 0 {
		imp.Cells = Touching(c.Box(), 0.15)
	}
	rest = f.Ticks > 3 && c.OnGround && math.Hypot(c.VX, c.VZ) < 0.03
	return imp, rest
}

// Touching lists the blocks a box grown by pad overlaps.
func Touching(b collide.AABB, pad float64) [][3]int {
	var out [][3]int
	for x := int(math.Floor(b.MinX - pad)); x <= int(math.Floor(b.MaxX+pad)); x++ {
		for y := int(math.Floor(b.MinY - pad)); y <= int(math.Floor(b.MaxY+pad)); y++ {
			for z := int(math.Floor(b.MinZ - pad)); z <= int(math.Floor(b.MaxZ+pad)); z++ {
				out = append(out, [3]int{x, y, z})
			}
		}
	}
	return out
}
