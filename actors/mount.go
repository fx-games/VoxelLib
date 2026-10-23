package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
)

// RideInput is a player rider's steering for one tick. The rider's Yaw
// steers the mount.
type RideInput struct {
	Forward, Strafe float64 // -1..1
	Jump            bool    // held: charges the jump, released: jumps
}

// jumpCharge is EntityPlayerSP's horse jump bar.
type jumpCharge struct {
	counter int
	power   float64
	prev    bool
}

// update advances the bar and returns the jump power sent on release
// (EntityHorse.setJumpPower: 0.4..0.8, 1.0 from 90%), or 0.
func (j *jumpCharge) update(held bool) float64 {
	out := 0.0
	if j.counter < 0 {
		j.counter++
		if j.counter == 0 {
			j.power = 0
		}
	}
	switch {
	case j.prev && !held:
		j.counter = -10
		p := int(j.power * 100)
		if p >= 90 {
			out = 1
		} else if p > 0 {
			out = 0.4 + 0.4*float64(p)/90
		}
	case !j.prev && held:
		j.counter, j.power = 0, 0
	case j.prev:
		j.counter++
		if j.counter < 10 {
			j.power = float64(j.counter) * 0.1
		} else {
			j.power = 0.8 + 2/float64(j.counter-9)*0.1
		}
	}
	j.prev = held
	return out
}

// JumpBar is the charge shown on the HUD while riding (0..1).
func (a *Actor) JumpBar() float64 { return a.jump.power }

// Mount puts rider on mount. It fails if the mount is not rideable or
// already carries someone.
func (m *Manager) Mount(rider, mount *Actor) bool {
	if mount.Mount == nil || mount.Rider != nil || rider.Riding != nil || rider == mount || !mount.Alive() {
		return false
	}
	rider.Riding, mount.Rider = mount, rider
	m.stop(mount)
	rider.jump = jumpCharge{}
	m.seat(rider)
	rider.Body.PrevX, rider.Body.PrevY, rider.Body.PrevZ = rider.Body.X, rider.Body.Y, rider.Body.Z
	return true
}

// Dismount sets the rider down beside its mount, on the first free side.
func (m *Manager) Dismount(rider *Actor) {
	mount := rider.Riding
	if mount == nil {
		return
	}
	rider.Riding, mount.Rider = nil, nil
	mb, rb := mount.Body, rider.Body
	side := mb.Width/2 + rb.Width/2 + 0.1
	for _, ang := range []float64{math.Pi / 2, -math.Pi / 2, math.Pi, 0} {
		y := mount.Yaw + ang
		x, z := mb.X+math.Cos(y)*side, mb.Z+math.Sin(y)*side
		if collide.Fits(m.body(), collide.Box(x, mb.Y, z, rb.Width, rb.Height)) {
			rb.X, rb.Y, rb.Z = x, mb.Y, z
			rb.VX, rb.VY, rb.VZ = 0, 0, 0
			return
		}
	}
	rb.Y = mb.Y + mount.Mount.RideHeight // nowhere beside it: step off on top
}

// seat puts the rider on its mount's back.
func (m *Manager) seat(r *Actor) {
	mt := r.Riding
	rb, mb := r.Body, mt.Body
	rb.PrevX, rb.PrevY, rb.PrevZ = mb.PrevX, mb.PrevY+mt.Mount.RideHeight, mb.PrevZ
	rb.X, rb.Y, rb.Z = mb.X, mb.Y+mt.Mount.RideHeight, mb.Z
	rb.VX, rb.VY, rb.VZ = 0, 0, 0
	rb.OnGround = false
}

// rideInput is EntityHorse.moveEntityWithHeading under a player: the mount
// turns with the rider, strafes at half speed, backs at a quarter, and
// jumps with the charged power (forward boost 0.4 * power).
func (m *Manager) rideInput(mt *Actor) character.Input {
	r := mt.Rider
	in := r.RideInput
	mt.Yaw, mt.HeadYaw = r.Yaw, r.Yaw
	fwd, strafe := in.Forward, in.Strafe*0.5
	if fwd <= 0 {
		fwd *= 0.25
	}
	if p := r.jump.update(in.Jump); p > 0 {
		mt.jumpPower = p
	}
	b := mt.Body
	b.MoveSpeed = mt.Speed
	jump := false
	if mt.jumpPower > 0 && b.OnGround {
		b.VY = mt.Mount.JumpStrength * mt.jumpPower
		if fwd > 0 {
			b.VX += math.Sin(mt.Yaw) * 0.4 * mt.jumpPower
			b.VZ -= math.Cos(mt.Yaw) * 0.4 * mt.jumpPower
		}
		mt.jumpPower = 0
		b.OnGround = false
	} else if b.OnGround && b.HorizontalCollision && fwd > 0 {
		jump = true // ponytail: stands in for the ridden horse's 1.0 step height
	}
	return character.Input{Forward: fwd, Strafe: strafe, Yaw: mt.Yaw, Jump: jump}
}

// HorseStats rolls 1.7.10 EntityHorse attributes: movement speed
// 0.1125..0.3375, jump strength 0.4..1.0, health 15..30.
func HorseStats(rnd interface {
	Float64() float64
	Intn(int) int
}) (speed, jump, health float64) {
	speed = (0.45 + rnd.Float64()*0.3 + rnd.Float64()*0.3 + rnd.Float64()*0.3) * 0.25
	jump = 0.4 + rnd.Float64()*0.2 + rnd.Float64()*0.2 + rnd.Float64()*0.2
	health = float64(15 + rnd.Intn(8) + rnd.Intn(9))
	return
}
