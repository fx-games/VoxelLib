// Package character is a first-person character controller with Minecraft's
// movement model: 20 Hz ticks, walking/sprinting/sneaking, jumping, creative
// flight, swimming and diving in water, wading through lava.
//
// It is independent of the rest of voxellib. The world is described by the
// small World interface (solid blocks and liquids at integer positions) and
// input arrives as a plain Input value, so it works with any terrain source
// and any windowing/input library. Collision uses voxel/collide.
//
// Typical use, once per 50 ms tick:
//
//	c.Tick(world, character.Input{Forward: 1, Yaw: yaw, Pitch: pitch})
//	x, y, z := c.Lerp(alpha) // interpolated feet position for rendering
package character

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Liquid is the kind of liquid filling a block.
type Liquid uint8

const (
	LiquidNone Liquid = iota
	LiquidWater
	LiquidLava
)

// World is what the controller needs to know about the terrain.
type World interface {
	collide.Solids
	Liquid(x, y, z int) Liquid
}

// Input is one tick of player intent.
type Input struct {
	Forward, Strafe float64 // -1..1; +Forward walks along Yaw, +Strafe to the right
	Jump            bool    // jump; swim up; fly up
	Sneak           bool    // sneak (slow, won't walk off edges); sink; fly down
	Sprint          bool    // sprint; sprint-swim (dive along the look direction)
	Using           bool    // using an item (eating, drawing a bow): x0.2, no sprint
	// Yaw and Pitch in radians: yaw 0 looks toward -Z and grows clockwise
	// seen from above; positive pitch looks up (as render.Camera).
	Yaw, Pitch float64
}

// Character is the simulated body. Fields may be set directly, e.g. to
// teleport (also set Prev* to avoid interpolating across the jump).
type Character struct {
	X, Y, Z             float64 // feet centre
	VX, VY, VZ          float64 // blocks per tick
	PrevX, PrevY, PrevZ float64 // position before the last tick

	Width, Height, EyeHeight float64

	Flying bool // creative flight
	NoClip bool // fly through blocks (debug); implies Flying

	// MoveSpeed is the ground acceleration per tick at full input
	// (sprinting adds 30%); 0 means the player's 0.1. A ridden horse uses
	// its movementSpeed attribute; package actors sets AI walkers'.
	MoveSpeed float64

	// State from the last tick.
	OnGround            bool
	HorizontalCollision bool
	InWater, InLava     bool   // body touches the liquid
	EyeIn               Liquid // liquid at eye height (for underwater fog)
	Swimming            bool   // sprint-swimming underwater
	Sprinting           bool   // sprinting this tick (Input.Sprint walking forward, no item in use)
}

// Player dimensions, as Minecraft's.
const (
	PlayerWidth     = 0.6
	PlayerHeight    = 1.8
	PlayerEyeHeight = 1.62
)

// New returns a player-sized character standing at x,y,z.
func New(x, y, z float64) *Character {
	return &Character{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z,
		Width: PlayerWidth, Height: PlayerHeight, EyeHeight: PlayerEyeHeight}
}

// Box returns the collision box.
func (c *Character) Box() collide.AABB { return collide.Box(c.X, c.Y, c.Z, c.Width, c.Height) }

// Eye returns the eye position.
func (c *Character) Eye() (x, y, z float64) { return c.X, c.Y + c.EyeHeight, c.Z }

// Lerp interpolates the feet position between the last two ticks.
func (c *Character) Lerp(alpha float64) (x, y, z float64) {
	return c.PrevX + (c.X-c.PrevX)*alpha, c.PrevY + (c.Y-c.PrevY)*alpha, c.PrevZ + (c.Z-c.PrevZ)*alpha
}

// Teleport moves the character without interpolation and stops it.
func (c *Character) Teleport(x, y, z float64) {
	c.X, c.Y, c.Z = x, y, z
	c.PrevX, c.PrevY, c.PrevZ = x, y, z
	c.VX, c.VY, c.VZ = 0, 0, 0
}

// ToggleFly switches creative flight on or off.
func (c *Character) ToggleFly() {
	c.Flying = !c.Flying
	c.VY = 0
	if !c.Flying {
		c.NoClip = false
	}
}

// Movement constants (blocks/tick), from Minecraft's LivingEntity.travel.
const (
	gravity      = 0.08
	airDrag      = 0.98 // vertical, per tick
	groundDrag   = 0.546
	airFriction  = 0.91
	walkAccel    = 0.1
	sprintAccel  = 0.13
	airAccel     = 0.02
	sprintAirAcc = 0.026
	flyAccel     = 0.05
	jumpVelocity = 0.42
	liquidAccel  = 0.02
	stepHeight   = 0.5 // EntityLivingBase.stepHeight
)

// Tick advances one 50 ms step.
func (c *Character) Tick(w World, in Input) {
	c.PrevX, c.PrevY, c.PrevZ = c.X, c.Y, c.Z
	if c.NoClip {
		c.Flying = true
	}
	c.updateLiquids(w)

	fwd, strafe := in.Forward, in.Strafe
	if l := math.Hypot(fwd, strafe); l > 1 {
		fwd, strafe = fwd/l, strafe/l
	}
	fwd, strafe = fwd*0.98, strafe*0.98 // Minecraft scales input by 0.98
	sprint := in.Sprint && in.Forward > 0
	if in.Sneak && !c.Flying && !c.InWater && !c.InLava {
		fwd, strafe = fwd*0.3, strafe*0.3
	}
	if in.Using {
		// EntityPlayerSP.onLivingUpdate: moveForward and moveStrafe x0.2;
		// sprinting needs !isUsingItem and stops below 0.8 forward.
		fwd, strafe, sprint = fwd*0.2, strafe*0.2, false
	}
	c.Sprinting = sprint
	c.Swimming = sprint && c.InWater && c.EyeIn == LiquidWater && !c.Flying

	accel := func(a float64) {
		sy, cy := math.Sin(in.Yaw), math.Cos(in.Yaw)
		c.VX += (fwd*sy + strafe*cy) * a
		c.VZ += (-fwd*cy + strafe*sy) * a
	}

	switch {
	case c.Flying:
		a := flyAccel
		if sprint {
			a *= 2
		}
		accel(a)
		if in.Jump {
			c.VY += flyAccel * 3
		}
		if in.Sneak {
			c.VY -= flyAccel * 3
		}
		c.move(w, in)
		c.VY *= 0.6
		c.VX *= airFriction
		c.VZ *= airFriction
		if c.OnGround && !c.NoClip {
			c.Flying = false
		}

	case c.InWater:
		accel(liquidAccel)
		if c.Swimming {
			// Pull vertical speed toward the look direction (Player.travel).
			look := math.Sin(in.Pitch)
			k := 0.06
			if look < -0.2 {
				k = 0.085
			}
			x, y, z := c.Eye()
			headUnder := w.Liquid(int(math.Floor(x)), int(math.Floor(y+0.9)), int(math.Floor(z))) == LiquidWater
			if look <= 0 || in.Jump || headUnder {
				c.VY += (look - c.VY) * k
			}
		}
		if in.Jump {
			c.VY += 0.04
		}
		if in.Sneak {
			c.VY -= 0.04
		}
		y0 := c.Y
		c.move(w, in)
		drag := 0.8
		if sprint {
			drag = 0.9
		}
		c.VX *= drag
		c.VY *= 0.8
		c.VZ *= drag
		if !c.Swimming {
			c.VY -= gravity / 16
		}
		c.climbOut(w, y0)

	case c.InLava:
		accel(liquidAccel)
		if in.Jump {
			c.VY += 0.04
		}
		if in.Sneak {
			c.VY -= 0.04
		}
		y0 := c.Y
		c.move(w, in)
		c.VX *= 0.5
		c.VY *= 0.5
		c.VZ *= 0.5
		c.VY -= gravity / 4
		c.climbOut(w, y0)

	default:
		a, drag := airAccel, airFriction
		if sprint {
			a = sprintAirAcc
		}
		if c.OnGround {
			a, drag = walkAccel, groundDrag
			if sprint {
				a = sprintAccel
			}
			if c.MoveSpeed > 0 {
				a = c.MoveSpeed
				if sprint {
					a *= sprintAccel / walkAccel
				}
			}
		}
		accel(a)
		if in.Jump && c.OnGround {
			c.VY = jumpVelocity
			if sprint { // sprint-jump boost
				c.VX += math.Sin(in.Yaw) * 0.2
				c.VZ -= math.Cos(in.Yaw) * 0.2
			}
		}
		c.move(w, in)
		c.VY = (c.VY - gravity) * airDrag
		c.VX *= drag
		c.VZ *= drag
	}
}

// climbOut gives the upward kick that lets a swimmer climb onto a ledge
// (LivingEntity.travel: horizontal collision with free space 0.6 higher).
func (c *Character) climbOut(w World, y0 float64) {
	if !c.HorizontalCollision {
		return
	}
	box := c.Box().Offset(c.VX, c.VY+0.6-(c.Y-y0), c.VZ)
	if collide.Fits(w, box) && !c.touches(w, box, LiquidNone, true) {
		c.VY = 0.3
	}
}

func (c *Character) move(w World, in Input) {
	dx, dy, dz := c.VX, c.VY, c.VZ
	if c.NoClip {
		c.X, c.Y, c.Z = c.X+dx, c.Y+dy, c.Z+dz
		c.OnGround, c.HorizontalCollision = false, false
		return
	}
	box := c.Box()
	// Sneaking on the ground never walks off an edge (Player.maybeBackOffFromEdge).
	if in.Sneak && c.OnGround && !c.Flying && dy <= 0 {
		const step = 0.05
		free := func(x, z float64) bool { return collide.Fits(w, box.Offset(x, -1, z)) }
		for dx != 0 && free(dx, 0) {
			dx = shrink(dx, step)
		}
		for dz != 0 && free(0, dz) {
			dz = shrink(dz, step)
		}
		for dx != 0 && dz != 0 && free(dx, dz) {
			dx, dz = shrink(dx, step), shrink(dz, step)
		}
	}
	mx, my, mz := collide.Move(w, box, dx, dy, dz)
	// Entity.moveEntity's step: blocked sideways while on (or landing on)
	// the ground, retry raised by stepHeight, then settle back down; keep
	// whichever goes further horizontally.
	if (c.OnGround || my != dy && dy < 0) && (mx != dx || mz != dz) {
		boxes := collide.Colliders(w, box.Expand(dx, stepHeight, dz), nil)
		b, sy := box, collide.ClipY(boxes, box, stepHeight)
		b = b.Offset(0, sy, 0)
		sx := collide.ClipX(boxes, b, dx)
		b = b.Offset(sx, 0, 0)
		sz := collide.ClipZ(boxes, b, dz)
		b = b.Offset(0, 0, sz)
		sy += collide.ClipY(boxes, b, -stepHeight)
		if mx*mx+mz*mz < sx*sx+sz*sz {
			mx, my, mz = sx, sy, sz
		}
	}
	c.OnGround = dy < 0 && my != dy
	c.HorizontalCollision = mx != dx || mz != dz
	if mx != dx {
		c.VX = 0
	}
	if mz != dz {
		c.VZ = 0
	}
	if my != dy {
		c.VY = 0
	}
	c.X, c.Y, c.Z = c.X+mx, c.Y+my, c.Z+mz
}

func shrink(v, step float64) float64 {
	switch {
	case v < step && v >= -step:
		return 0
	case v > 0:
		return v - step
	default:
		return v + step
	}
}

func (c *Character) updateLiquids(w World) {
	box := c.Box().Grow(-0.001, -0.001, -0.001)
	c.InWater = c.touches(w, box, LiquidWater, false)
	c.InLava = c.touches(w, box, LiquidLava, false)
	// A liquid's open surface sits at 7/8 of the block, so the eye is only
	// under it below that height (or when more liquid is stacked on top).
	x, y, z := c.Eye()
	bx, by, bz := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	c.EyeIn = w.Liquid(bx, by, bz)
	if c.EyeIn != LiquidNone && y-float64(by) > 0.875 && w.Liquid(bx, by+1, bz) != c.EyeIn {
		c.EyeIn = LiquidNone
	}
}

// touches reports whether box overlaps liquid l (any liquid when any is set).
func (c *Character) touches(w World, box collide.AABB, l Liquid, any bool) bool {
	hit := false
	box.Blocks(func(x, y, z int) {
		if hit {
			return
		}
		got := w.Liquid(x, y, z)
		hit = (any && got != LiquidNone) || (!any && got == l)
	})
	return hit
}
