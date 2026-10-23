package character

import "math"

// Quake 3 bg_pmove, written in Q3 units scaled so the 56-unit Q3 player is
// as tall as ours (0.9 blocks per 56 units). Speeds are blocks/second inside; a
// 50 ms tick runs quakeSteps pmove steps so friction and acceleration match.
const (
	quakeUnit       = 0.9 / 56.0
	pmAccelerate    = 10.0
	pmAirAccelerate = 1.0
	pmFriction      = 6.0
	pmStopSpeed     = 100.0 * quakeUnit
	pmSpeed         = 320.0 * quakeUnit
	pmDuckScale     = 0.5 // a sneaking player asks for half the speed
	pmJumpVelocity  = 270.0 * quakeUnit
	q3Gravity       = 800.0 * quakeUnit
	pmMinSpeed      = 1.0 * quakeUnit
	quakeSteps      = 6
	quakeDT         = 0.05 / quakeSteps
)

// quakeTick replaces the on-foot branch of Tick for MoveQuake. Gravity has
// no drag; the jump needs the key released first (PM_CheckJump).
func (c *Character) quakeTick(w World, in Input) {
	fwd, strafe := in.Forward, in.Strafe
	if l := math.Hypot(fwd, strafe); l > 1 {
		fwd, strafe = fwd/l, strafe/l
	}
	sy, cy := math.Sin(in.Yaw), math.Cos(in.Yaw)
	wx, wz := fwd*sy+strafe*cy, -fwd*cy+strafe*sy

	grounded := c.OnGround
	if in.Jump && !c.jumpHeld && grounded {
		c.VY = pmJumpVelocity / 20
		grounded = false
	}
	c.jumpHeld = in.Jump

	vx, vz := c.VX*20, c.VZ*20
	for i := 0; i < quakeSteps; i++ {
		vx, vz = pmoveSingle(vx, vz, wx, wz, grounded, in.Sneak, quakeDT)
	}
	c.VX, c.VZ = vx/20, vz/20
	c.move(w, in)
	if c.OnLadder && c.HorizontalCollision {
		c.VY = ladderClimb
	}
	c.VY -= q3Gravity * 0.05 / 20 // blocks/s^2 over one tick, in blocks/tick
}

// pmoveSingle is PM_WalkMove / PM_AirMove for one step on the horizontal
// velocity (vx,vz) with wish direction (wx,wz), zero for no input.
func pmoveSingle(vx, vz, wx, wz float64, grounded, ducked bool, dt float64) (float64, float64) {
	accel := pmAirAccelerate
	if grounded {
		vx, vz = pmFrictionStep(vx, vz, dt)
		accel = pmAccelerate
	}
	if wx == 0 && wz == 0 {
		return vx, vz
	}
	wish := pmSpeed
	if ducked {
		wish *= pmDuckScale
	}
	// PM_Accelerate: only the part of the wish speed not yet reached is added.
	add := wish - (vx*wx + vz*wz)
	if add <= 0 {
		return vx, vz
	}
	as := math.Min(accel*dt*wish, add)
	return vx + wx*as, vz + wz*as
}

// pmFrictionStep is PM_Friction; below pmStopSpeed it bites at a constant rate.
func pmFrictionStep(vx, vz, dt float64) (float64, float64) {
	speed := math.Hypot(vx, vz)
	if speed < pmMinSpeed {
		return 0, 0
	}
	n := math.Max(0, speed-math.Max(speed, pmStopSpeed)*pmFriction*dt) / speed
	return vx * n, vz * n
}
