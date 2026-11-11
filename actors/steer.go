package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
)

// Steer is EntityAIControlledByPlayer (the pig's, mutex 7): a player
// rider holding the lure turns the mount toward their look and drives it
// forward at a speed creeping up to Max, with a boost on demand.
type Steer struct {
	Max  float64                   // the task's top speed (pig 0.3)
	Lure func(mt *Actor) bool      // canBeSteered: the rider holds the lure
	Wear func(mt *Actor, r *Actor) // the lure's 1-point wear (not in creative)

	running        bool
	speed          float64
	boosted        bool
	boostT, boostN int
	jump           bool // the jump helper's, done in the second step
}

// Boost is the lure's right click (func_82633_h then func_82632_g):
// false while boosted or slower than 0.3 Max.
func (m *Manager) Boost(mt *Actor) bool {
	s := mt.Mount.Steer
	if s == nil || s.boosted || s.speed <= s.Max*0.3 {
		return false
	}
	s.boosted, s.boostT, s.boostN = true, 0, m.Rand.Intn(841)+140
	return true
}

// steering is the task's shouldExecute, with startExecuting and
// resetTask's zero speed on the edges. A panic (EntityAIPanic, priority
// 1, sharing mutex bit 1) takes over while the mount has a revenge target
// or burns.
func (m *Manager) steering(mt *Actor) bool {
	s := mt.Mount.Steer
	if s == nil {
		return false
	}
	panics := mt.FleeWhenHurt && (mt.Revenge() != nil || mt.OnFire())
	run := mt.Alive() && !panics && mt.Rider != nil && mt.Rider.Player && (s.boosted || s.Lure != nil && s.Lure(mt))
	if run != s.running {
		s.running, s.speed = run, 0
		if !run {
			s.boosted = false
		}
	}
	return run
}

// steerInput is updateTask: the turn, the speed, the boost's sine, the
// jump at a one-block step and moveEntityWithHeading(0, f1); the jump
// goes to the second step, onLivingUpdate's with no input.
// The AI speed is the movementSpeed attribute (landMovementFactor as a
// wander leaves it); the body scales it by the ground's slipperiness.
// No jump from stairs or a slab, or from the air above one (StepBlocks).
func (m *Manager) steerInput(mt *Actor) character.Input {
	s, r, b := mt.Mount.Steer, mt.Rider, mt.Body
	d := math.Max(-5*math.Pi/180, math.Min(5*math.Pi/180, math.Remainder(r.Yaw-mt.Yaw, 2*math.Pi)*0.5))
	mt.Yaw = math.Remainder(mt.Yaw+d, 2*math.Pi)
	if s.speed < s.Max {
		s.speed += (s.Max - s.speed) * 0.01
	}
	s.speed = math.Min(s.speed, s.Max)
	f1 := s.speed
	if s.boosted {
		if s.boostT++; s.boostT-1 > s.boostN {
			s.boosted = false
		}
		f1 += f1 * 1.15 * math.Sin(float64(s.boostT)/float64(s.boostN)*math.Pi)
	}
	f2 := 0.91
	if b.OnGround {
		f2 = 0.6 * 0.91
		if sl, ok := m.body().(character.Slipper); ok {
			f2 = sl.Slipperiness(ifloor(b.X), ifloor(b.Y)-1, ifloor(b.Z)) * 0.91
		}
	}
	f3 := 0.16277136 / (f2 * f2 * f2)
	ai := mt.moveSpeed()
	f8 := f1 * (ai * f3 / math.Max(f1, 1))
	dx, dz := f8*math.Sin(mt.Yaw), -f8*math.Cos(mt.Yaw)
	half := b.Width / 2
	if math.Abs(dx) > math.Abs(dz) {
		dx, dz = dx+math.Copysign(half, dx), 0
	} else {
		dx, dz = 0, dz+math.Copysign(half, dz)
	}
	i, j, k := ifloor(b.X), ifloor(b.Y), ifloor(b.Z)
	l, i1 := ifloor(b.X+dx), ifloor(b.Z+dz)
	w, h := int(b.Width+1), int(b.Height+r.Body.Height+1)
	jump := false
	if (i != l || k != i1) && m.stepJumps(i, j, k) && !m.cellsClear(l, j, i1, w, h) && m.cellsClear(i, j+1, k, w, h) && m.cellsClear(l, j+1, i1, w, h) {
		jump = true // getJumpHelper().setJumping()
	}
	if s.speed >= s.Max*0.5 && m.Rand.Float32() < 0.006 && !s.boosted && s.Wear != nil {
		s.Wear(mt, r)
	}
	b.MoveSpeed = character.SpeedOf(ai * 0.16277136 / (0.546 * 0.546 * 0.546)) // the body applies the ground's own f3
	s.jump = jump
	return character.Input{Forward: f1, Yaw: mt.Yaw}
}

// StepBlocks is an optional World method for the steered jump test:
// StairOrSlab is func_151498_a (render type 10, BlockStairs, or a
// BlockSlab), Air the cell's Material.air.
type StepBlocks interface {
	StairOrSlab(x, y, z int) bool
	Air(x, y, z int) bool
}

// StepJumps is updateTask's flag: whether a body whose feet are in cell
// x, y, z may jump up a step. Not from a stair or slab, nor from the air
// above one: it walks up those. Worlds without StepBlocks always may.
func StepJumps(w any, x, y, z int) bool {
	sb, ok := w.(StepBlocks)
	return !ok || !sb.StairOrSlab(x, y, z) && (!sb.Air(x, y, z) || !sb.StairOrSlab(x, y-1, z))
}

func (m *Manager) stepJumps(x, y, z int) bool { return StepJumps(m.W, x, y, z) }

// cellsClear is PathFinder.func_82565_a's open (1) for a w*h*w point at
// x, y, z, with the world's path cell test.
func (m *Manager) cellsClear(x, y, z, w, h int) bool {
	for cx := x; cx < x+w; cx++ {
		for cy := y; cy < y+h; cy++ {
			for cz := z; cz < z+w; cz++ {
				if m.W.Solid(cx, cy, cz) {
					return false
				}
			}
		}
	}
	return true
}

func ifloor(v float64) int { return int(math.Floor(v)) }
