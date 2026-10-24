package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
)

// Boat makes an actor a floating vehicle with the reference boat's server
// physics (EntityBoat.onUpdate): it bobs on the share of its box in water,
// its rider paddles it, and it turns toward where it drifts. A boat is no
// living thing: no brain, no fire, no death animation; hits go to its
// damage (HitBoat). The zero numbers are not usable: see VanillaBoat.
type Boat struct {
	YOffset            float64 // posY above the feet (yOffset): vanilla height/2
	MountedY           float64 // getMountedYOffset: vanilla -0.3
	Seat               float64 // the rider sits this far toward the boat's yaw: vanilla 0.4
	MinSpeed, MaxSpeed float64 // speedMultiplier's range: vanilla 0.07..0.35
	Ramp               float64 // speedMultiplier's step divisor: vanilla 35
	// Strafe: strafing turns the paddling direction by 90° and backing
	// paddles backwards (vanilla); else only forward input paddles.
	Strafe bool
	// Fragile: a horizontal collision above 0.2 a tick or landing from
	// over 3 blocks breaks it (BoatCrashed), and Clear runs first on the
	// cells at its corners (vanilla: snow layers and lily pads go).
	Fragile bool
	Clear   func(x, y, z int)
	// OnHit is called on each counted hit (the barrel's knock sound);
	// OnBreak as the boat breaks (the Manager removes it after the tick).
	OnHit   func(a *Actor)
	OnBreak func(a *Actor, how BoatBreak)
	// Move, when set, replaces the floating physics after the hit timers
	// (a rail cart's onUpdate); it reports whether the vehicle broke.
	// Push, when set, is the vehicle's applyEntityCollision with an actor
	// whose box touches its own grown 0.2 sideways: it replaces the even
	// push (two such vehicles run both).
	Move func(a *Actor) bool
	Push func(a, o *Actor)

	Damage float64 // damageTaken: +10 per hit point, -1 a tick, breaks over 40
	Hit    int     // timeSinceHit: 10 on a hit, the renderer's wobble
	Dir    int     // forwardDirection: flips on each hit (1 or -1)

	speed  float64 // speedMultiplier
	fall   float64
	broken bool
}

// BoatBreak tells how a boat broke.
type BoatBreak uint8

const (
	BoatBroken   BoatBreak = iota // knocked apart (damage over 40): drops itself
	BoatCreative                  // knocked apart by a creative player: drops nothing
	BoatCrashed                   // a crash or a fall (Fragile): vanilla drops 3 planks, 2 sticks
)

// VanillaBoat returns the reference boat's numbers (1.5 x 0.6 body).
func VanillaBoat() *Boat {
	return &Boat{YOffset: 0.3, MountedY: -0.3, Seat: 0.4, MinSpeed: 0.07, MaxSpeed: 0.35, Ramp: 35,
		Strafe: true, Fragile: true}
}

// LiquidMeta is the optional World method character.LiquidMeta. Without it
// every liquid fills its cell.
type LiquidMeta = character.LiquidMeta

// inWater is World.isAABBInMaterial(box, water): a water cell in the box
// whose surface (y+1 - meta/8 below meta 8) reaches the box's bottom.
func (m *Manager) inWater(b collide.AABB) bool {
	lm, _ := m.W.(LiquidMeta)
	for x := int(math.Floor(b.MinX)); x < int(math.Floor(b.MaxX+1)); x++ {
		for y := int(math.Floor(b.MinY)); y < int(math.Floor(b.MaxY+1)); y++ {
			for z := int(math.Floor(b.MinZ)); z < int(math.Floor(b.MaxZ+1)); z++ {
				if m.W.Liquid(x, y, z) != character.LiquidWater {
					continue
				}
				top := float64(y + 1)
				if lm != nil {
					if meta := lm.LiquidMeta(x, y, z); meta < 8 {
						top -= float64(meta) / 8
					}
				}
				if top >= b.MinY {
					return true
				}
			}
		}
	}
	return false
}

// InWater reports whether the actor's box is in water as the reference's
// isAABBInMaterial sees it (the barrel's ride check).
func (m *Manager) InWater(a *Actor) bool { return m.inWater(a.Body.Box()) }

// boatTick is EntityBoat.onUpdate's server branch. It reports whether the
// boat broke.
func (m *Manager) boatTick(a *Actor) bool {
	bt, b := a.Boat, a.Body
	if bt.speed == 0 {
		bt.speed = bt.MinSpeed
	}
	if bt.Hit > 0 {
		bt.Hit--
	}
	if bt.Damage > 0 {
		bt.Damage--
	}
	if bt.Move != nil {
		return bt.Move(a)
	}
	// handleWaterMovement: the current pushes, water resets the fall
	in, wx, wy, wz := character.WaterPush(m.body(), b.Box())
	b.VX, b.VY, b.VZ = b.VX+wx, b.VY+wy, b.VZ+wz
	if in {
		bt.fall = 0
	}
	b.PrevX, b.PrevY, b.PrevZ = b.X, b.Y, b.Z
	box := b.Box()
	water := 0.0
	for i := 0; i < 5; i++ {
		s := box
		s.MinY = box.MinY + (box.MaxY-box.MinY)*float64(i)/5 - 0.125
		s.MaxY = box.MinY + (box.MaxY-box.MinY)*float64(i+1)/5 - 0.125
		if m.inWater(s) {
			water += 1.0 / 5
		}
	}
	was := math.Hypot(b.VX, b.VZ)
	if water < 1 {
		b.VY += 0.04 * (water*2 - 1)
	} else {
		if b.VY < 0 {
			b.VY /= 2
		}
		b.VY += 0.007
	}
	if r := a.Rider; r != nil {
		// moveForward/moveStrafing: the input times 0.98 (onLivingUpdate)
		fwd, strafe, yaw := r.RideInput.Forward*0.98, r.RideInput.Strafe*0.98, r.Yaw
		if !r.Player {
			// an AI rider's moveForward is its AI speed while its navigation
			// walks (EntityMoveHelper.setMoveForward(getAIMoveSpeed)), times 0.98
			fwd, strafe = r.nav.in.Forward*math.Sqrt(r.Body.MoveSpeed*0.98)*0.98, 0
		}
		if bt.Strafe {
			yaw += strafe * math.Pi / 2 // + -moveStrafing*90°, strafe right = -moveStrafing
		} else if fwd <= 0 {
			fwd = 0
		} else {
			fwd = 1 // the barrel paddles at full speed for any forward input
		}
		b.VX += math.Sin(yaw) * bt.speed * fwd * 0.05
		b.VZ -= math.Cos(yaw) * bt.speed * fwd * 0.05
	}
	sp := math.Hypot(b.VX, b.VZ)
	if sp > bt.MaxSpeed {
		k := bt.MaxSpeed / sp
		b.VX, b.VZ, sp = b.VX*k, b.VZ*k, bt.MaxSpeed
	}
	if sp > was && bt.speed < bt.MaxSpeed {
		bt.speed = min(bt.speed+(bt.MaxSpeed-bt.speed)/bt.Ramp, bt.MaxSpeed)
	} else {
		bt.speed = max(bt.speed-(bt.speed-bt.MinSpeed)/bt.Ramp, bt.MinSpeed)
	}
	if bt.Fragile && bt.Clear != nil {
		py := b.Y + bt.YOffset
		for i := 0; i < 4; i++ {
			x := int(math.Floor(b.X + (float64(i%2)-0.5)*0.8))
			z := int(math.Floor(b.Z + (float64(i/2)-0.5)*0.8))
			for dy := 0; dy < 2; dy++ {
				bt.Clear(x, int(math.Floor(py))+dy, z)
			}
		}
	}
	if b.OnGround {
		b.VX, b.VY, b.VZ = b.VX*0.5, b.VY*0.5, b.VZ*0.5
	}
	vx, vy, vz := b.VX, b.VY, b.VZ
	mx, my, mz := collide.Move(m.body(), b.Box(), vx, vy, vz)
	b.X, b.Y, b.Z = b.X+mx, b.Y+my, b.Z+mz
	b.OnGround = vy < 0 && my != vy
	b.HorizontalCollision = mx != vx || mz != vz
	if mx != vx {
		b.VX = 0
	}
	if my != vy {
		b.VY = 0
	}
	if mz != vz {
		b.VZ = 0
	}
	// updateFallState: Entity.fall passes a landing to the rider. A
	// Fragile boat breaks landing after more than 3, its fall grows only
	// above anything but water and small landings don't reset it
	// (handleWaterMovement does, in water).
	switch {
	case b.OnGround && bt.Fragile:
		if bt.fall > 3 {
			m.riderFall(a, bt.fall)
			return m.breakBoat(a, BoatCrashed)
		}
	case b.OnGround:
		m.riderFall(a, bt.fall)
		bt.fall = 0
	case my < 0 && (!bt.Fragile || m.W.Liquid(int(math.Floor(b.X)), int(math.Floor(b.Y+bt.YOffset))-1, int(math.Floor(b.Z))) != character.LiquidWater):
		bt.fall -= my
	}
	if bt.Fragile && b.HorizontalCollision && was > 0.2 {
		return m.breakBoat(a, BoatCrashed)
	}
	b.VX, b.VY, b.VZ = b.VX*0.99, b.VY*0.95, b.VZ*0.99
	// turn toward the drift, at most 20° a tick; the yaw is the reference's
	// atan2(prevZ-z, prevX-x) in degrees, kept here as degrees·π/180 + π
	a.Pitch = 0
	yaw := a.Yaw
	if dx, dz := b.PrevX-b.X, b.PrevZ-b.Z; dx*dx+dz*dz > 0.001 {
		yaw = float64(float32(math.Atan2(dz, dx)*180/math.Pi))*math.Pi/180 + math.Pi
	}
	d := math.Remainder(yaw-a.Yaw, 2*math.Pi)
	d = math.Max(-20*math.Pi/180, math.Min(20*math.Pi/180, d))
	a.Yaw += d
	a.HeadYaw = a.Yaw
	return false
}

// riderFall is the rider's EntityLivingBase.fall: ceil(fall-3) damage.
func (m *Manager) riderFall(a *Actor, fall float64) {
	if r := a.Rider; r != nil && fall > 3 {
		m.Hurt(r, nil, math.Ceil(fall-3), true)
	}
}

// boatSeat is updateRiderPosition: the rider at posY + getMountedYOffset +
// its getYOffset (a player's sits 0.5 lower: EntityPlayer.getYOffset is
// yOffset - 0.5), Seat toward the boat's yaw.
func boatSeat(r, mt *Actor) (ox, oy, oz float64) {
	bt := mt.Boat
	oy = bt.YOffset + bt.MountedY
	if r.Player {
		oy -= 0.5
	}
	deg := mt.Yaw - math.Pi // back to the reference's degrees (in radians)
	return math.Cos(deg) * bt.Seat, oy, math.Sin(deg) * bt.Seat
}

// HitBoat is EntityBoat.attackEntityFrom: the boat wobbles, takes damage
// ten times amount, and breaks over 40 or at once for a creative player.
// It reports whether the hit counted.
func (m *Manager) HitBoat(a *Actor, amount float64, creative bool) bool {
	bt := a.Boat
	if bt == nil || bt.broken {
		return false
	}
	bt.Dir = -bt.Dir
	if bt.Dir == 0 {
		bt.Dir = -1
	}
	bt.Hit = 10
	bt.Damage += amount * 10
	if bt.OnHit != nil {
		bt.OnHit(a)
	}
	switch {
	case creative:
		m.breakBoat(a, BoatCreative)
	case bt.Damage > 40:
		m.breakBoat(a, BoatBroken)
	}
	return true
}

func (m *Manager) breakBoat(a *Actor, how BoatBreak) bool {
	bt := a.Boat
	bt.broken = true
	if a.Rider != nil {
		m.Dismount(a.Rider)
	}
	if bt.OnBreak != nil {
		bt.OnBreak(a, how)
	}
	m.wrecks = append(m.wrecks, a)
	return true
}

// MoveBody is Entity.moveEntity for a vehicle's body: it moves by v
// against the block boxes it collides with, without stepping, and stops
// the blocked axes.
func (m *Manager) MoveBody(a *Actor, vx, vy, vz float64) {
	b := a.Body
	mx, my, mz := collide.Move(m.body(), b.Box(), vx, vy, vz)
	b.X, b.Y, b.Z = b.X+mx, b.Y+my, b.Z+mz
	b.OnGround = vy < 0 && my != vy
	b.HorizontalCollision = mx != vx || mz != vz
	if mx != vx {
		b.VX = 0
	}
	if my != vy {
		b.VY = 0
	}
	if mz != vz {
		b.VZ = 0
	}
}

// Broken reports whether the boat broke (it leaves at the end of the tick).
func (bt *Boat) Broken() bool { return bt.broken }
