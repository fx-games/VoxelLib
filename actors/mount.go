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

// update advances the bar. On release it returns sent and the jump power
// (EntityHorse.setJumpPower: 0.4..0.8, 1.0 from 90%; a tap sends 0.4).
func (j *jumpCharge) update(held bool) (out float64, sent bool) {
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
		} else {
			out = 0.4 + 0.4*float64(p)/90
		}
		sent = true
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
	return
}

// JumpBar is the charge shown on the HUD while riding (0..1).
func (a *Actor) JumpBar() float64 { return a.jump.power }

// Mount puts rider on mount. It fails if the mount is not rideable or
// already carries someone.
func (m *Manager) Mount(rider, mount *Actor) bool {
	if mount.Mount == nil && mount.Boat == nil || mount.Rider != nil || rider.Riding != nil || rider == mount || !mount.Alive() {
		return false
	}
	rider.Riding, mount.Rider = mount, rider
	if mount.Mount != nil {
		m.stop(mount)
		mount.Mount.Eating, mount.Mount.rearing = false, false // EntityHorse.func_110237_h
	}
	rider.jump = jumpCharge{}
	m.seat(rider)
	rider.Body.PrevX, rider.Body.PrevY, rider.Body.PrevZ = rider.Body.X, rider.Body.Y, rider.Body.Z
	return true
}

// playerYOffset is how far the reference's player position sits above
// its feet (yOffset); other bodies stand on their position.
const playerYOffset = 1.62

// Dismount sets the rider down as EntityLivingBase.dismountEntity: one
// block up, on the first neighbour cell (dx -1..1, dz -1..0) where its
// raised box is free and the block at its position's height has a solid
// top; else on the last such cell with a solid top or water one lower;
// else on top of the mount.
func (m *Manager) Dismount(rider *Actor) {
	mount := rider.Riding
	if mount == nil {
		return
	}
	rider.Riding, mount.Rider = nil, nil
	mb, rb := mount.Body, rider.Body
	yo := 0.0
	if rider.Player {
		yo = playerYOffset
	}
	w := m.body()
	py := rb.Y + yo
	// Fallback: mount.boundingBox.minY + height as the rider's position;
	// never below the mount's feet (a player would sink into the floor).
	x, y, z := mb.X, max(mb.Y+mb.Height-yo, mb.Y), mb.Z
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz < 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			bx, bz := int(rb.X+float64(dx)), int(rb.Z+float64(dz)) // (int): truncated, as the reference
			if !collide.Fits(w, collide.Box(rb.X+float64(dx), rb.Y+1, rb.Z+float64(dz), rb.Width, rb.Height)) {
				continue
			}
			if solidTop(w, bx, int(py), bz) {
				rider.place(rb.X+float64(dx), rb.Y+1, rb.Z+float64(dz))
				return
			}
			if solidTop(w, bx, int(py)-1, bz) || w.Liquid(bx, int(py)-1, bz) == character.LiquidWater {
				x, y, z = rb.X+float64(dx), rb.Y+1, rb.Z+float64(dz)
			}
		}
	}
	rider.place(x, y, z)
}

func (a *Actor) place(x, y, z float64) {
	b := a.Body
	b.X, b.Y, b.Z = x, y, z
	b.PrevX, b.PrevY, b.PrevZ = x, y, z
	b.VX, b.VY, b.VZ = 0, 0, 0
}

// solidTop is World.doesBlockHaveSolidTopSurface: the cell holds a box
// spanning its whole top face.
func solidTop(w character.World, x, y, z int) bool {
	cell := collide.Block(x, y, z)
	for _, b := range collide.Colliders(w, cell, nil) {
		if b.MaxY == cell.MaxY && b.MinX <= cell.MinX && b.MaxX >= cell.MaxX && b.MinZ <= cell.MinZ && b.MaxZ >= cell.MaxZ {
			return true
		}
	}
	return false
}

// seat puts the rider on its mount's back; a rearing horse tips it back
// 0.7 and up 0.15 times the rearing amount (EntityHorse.updateRiderPosition).
func (m *Manager) seat(r *Actor) {
	mt := r.Riding
	rb, mb := r.Body, mt.Body
	var ox, oy, oz float64
	if mt.Boat != nil {
		ox, oy, oz = boatSeat(r, mt)
	} else if oy = mt.Mount.RideHeight; mt.Mount.Anim.PrevRear > 0 {
		a := mt.Mount.Anim.PrevRear
		ox, oy, oz = -math.Sin(mt.Yaw)*0.7*a, oy+0.15*a, math.Cos(mt.Yaw)*0.7*a
	}
	rb.PrevX, rb.PrevY, rb.PrevZ = mb.PrevX+ox, mb.PrevY+oy, mb.PrevZ+oz
	rb.X, rb.Y, rb.Z = mb.X+ox, mb.Y+oy, mb.Z+oz
	rb.VX, rb.VY, rb.VZ = 0, 0, 0
	rb.OnGround = false
}

// Controlled reports whether a player rider steers mt (a saddled mount).
func (mt *Actor) Controlled() bool {
	return mt.Mount != nil && mt.Mount.Saddled && mt.Rider != nil && mt.Rider.Player
}

// Rear makes a horse rear for 20 ticks (EntityHorse.makeHorseRear); it
// stops eating.
func (mt *Actor) Rear() {
	if s := mt.Mount; s != nil {
		s.rearTicks, s.rearing, s.Eating, s.eatTicks = 1, true, false, 0
	}
}

// OpenMouth opens a horse's mouth for 30 ticks (openHorseMouth: on its
// sounds and when eating).
func (mt *Actor) OpenMouth() {
	if s := mt.Mount; s != nil {
		s.mouthTicks = 1
	}
}

// Rearing reports whether the horse is rearing.
func (mt *Actor) Rearing() bool { return mt.Mount != nil && mt.Mount.rearing }

// mountTick is a mount's EntityHorse.onUpdate/onLivingUpdate part: the
// rearing, eating and mouth timers and animation values, the 1 in 900
// chance a tick to heal 1 (also LOTRMountFunctions.update), grazing.
func (m *Manager) mountTick(a *Actor) {
	s := a.Mount
	if s.mouthTicks > 0 && s.mouthTicks+1 > 30 {
		s.mouthTicks = 0
	} else if s.mouthTicks > 0 {
		s.mouthTicks++
	}
	if s.rearTicks > 0 {
		if s.rearTicks++; s.rearTicks > 20 {
			s.rearTicks, s.rearing = 0, false
		}
	}
	if m.Rand.Intn(900) == 0 {
		a.Health = math.Min(a.MaxHealth, a.Health+1)
	}
	if s.Horse && !s.Eating && a.Rider == nil && s.Graze != nil && m.Rand.Intn(300) == 0 && s.Graze(a) {
		s.Eating, s.rearing = true, false
	}
	if s.Eating {
		if s.eatTicks++; s.eatTicks > 50 {
			s.eatTicks, s.Eating = 0, false
		}
	}
	an := &s.Anim
	an.PrevHeadLean = an.HeadLean
	if s.Eating {
		an.HeadLean = math.Min(1, an.HeadLean+(1-an.HeadLean)*0.4+0.05)
	} else {
		an.HeadLean = math.Max(0, an.HeadLean+(0-an.HeadLean)*0.4-0.05)
	}
	an.PrevRear = an.Rear
	if s.rearing {
		an.PrevHeadLean, an.HeadLean = 0, 0
		an.Rear = math.Min(1, an.Rear+(1-an.Rear)*0.4+0.05)
	} else {
		s.jumpRear = false
		an.Rear = math.Max(0, an.Rear+(0.8*an.Rear*an.Rear*an.Rear-an.Rear)*0.6-0.05)
	}
	an.PrevMouth = an.Mouth
	if s.mouthTicks > 0 {
		an.Mouth = math.Min(1, an.Mouth+(1-an.Mouth)*0.7+0.05)
	} else {
		an.Mouth = math.Max(0, an.Mouth+(0-an.Mouth)*0.7-0.05)
	}
}

// buck is EntityAIRunAroundLikeCrazy (LOTREntityAIUntamedPanic): an
// untamed mount under a player runs to random spots 5 blocks around at
// speed 1.2; each tick on the way, at 1 in 50, it is tamed with chance
// Temper/MaxTemper, else its temper rises 5 and it throws the rider.
func (m *Manager) buck(a *Actor) {
	n := &a.nav
	if !n.has {
		ang, d := m.Rand.Float64()*2*math.Pi, m.Rand.Float64()*5
		goal := Vec{a.Body.X + math.Cos(ang)*d, a.Body.Y + 4, a.Body.Z + math.Sin(ang)*d}
		p, ok := m.ground(m.graphFor(a), goal)
		if !ok {
			m.stop(a)
			return
		}
		m.plan(a, center(p))
	}
	if m.moveTo(a, n.goal, 1.2) || n.stuck >= 3 {
		m.stop(a)
		n.stuck = 0
	}
	if m.Rand.Intn(50) != 0 {
		return
	}
	s, r := a.Mount, a.Rider
	if s.MaxTemper > 0 && m.Rand.Intn(s.MaxTemper) < s.Temper {
		s.Tame, s.Owner = true, r.ID
		s.event(a, MountTamed)
		return
	}
	s.Temper = min(s.Temper+5, max(s.MaxTemper, 0))
	m.Dismount(r)
	s.event(a, MountBucked)
}

// MountEvent is what a mount's OnEvent hears about.
type MountEvent uint8

const (
	MountTamed  MountEvent = iota // a player rider tamed it (hearts)
	MountBucked                   // it threw its rider (angry rearing, smoke)
	MountJumped                   // a ridden horse jumped forward (mob.horse.jump)
	MountLanded                   // it landed after falling more than 1 block (mob.horse.land)
)

func (s *MountStats) event(mt *Actor, ev MountEvent) {
	if s.OnEvent != nil {
		s.OnEvent(mt, ev)
	}
}

// rideInput is EntityHorse.moveEntityWithHeading (LOTRMountFunctions.move)
// under a player on a saddled mount: it turns with the rider, strafes at
// half speed, backs at a quarter, steps 1.0 high, accelerates
// movementSpeed*0.1 in the air, stands while rearing on the ground, and a
// horse jumps with the charged power (forward boost 0.4 * power).
func (m *Manager) rideInput(mt *Actor) character.Input {
	r := mt.Rider
	s := mt.Mount
	in := r.RideInput
	mt.Yaw, mt.HeadYaw = r.Yaw, r.Yaw
	fwd, strafe := in.Forward, in.Strafe*0.5
	if fwd <= 0 {
		fwd *= 0.25
	}
	b := mt.Body
	power := 0.0
	if s.Horse {
		if p, sent := r.jump.update(in.Jump); sent {
			power = p
			s.jumpRear = true
			mt.Rear()
		}
		if b.OnGround && power == 0 && s.rearing && !s.jumpRear {
			fwd, strafe = 0, 0
		}
	}
	b.MoveSpeed = mt.moveSpeed()
	b.StepHeight, b.AirAccel = 1, b.MoveSpeed*0.1
	// a power sent in the air is dropped on landing, never used
	if power > 0 && b.OnGround {
		b.VY = s.JumpStrength * power
		if fwd > 0 {
			b.VX += math.Sin(mt.Yaw) * 0.4 * power
			b.VZ -= math.Cos(mt.Yaw) * 0.4 * power
			s.event(mt, MountJumped)
		}
		b.OnGround = false
	}
	jump := b.InWater && mt.CanSwim // EntityAISwimming keeps running
	return character.Input{Forward: fwd, Strafe: strafe, Yaw: mt.Yaw, Jump: jump}
}

// moveSpeed is the movementSpeed attribute's value: Speed, times the
// effects' modifiers and 1.3 while sprinting (setSprinting's modifier).
func (a *Actor) moveSpeed() float64 {
	v := a.Speed * (1 + a.speedOff)
	if a.Sprinting {
		return v * 1.3
	}
	return v
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
