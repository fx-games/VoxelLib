package character

import "math"

// Step is the reference's walking-step counter (Entity.moveEntity's
// distanceWalkedOnStepModified and nextStepDistance, which starts at 1).
type Step struct {
	walked float32
	next   int
}

// Moved counts one moveEntity of dx, dy, dz (dy only on a ladder) and
// reports whether it steps: the distance walked, times 0.6, has passed
// the next step and the block stepped on is not air.
func (s *Step) Moved(dx, dy, dz float64, ladder, air bool) bool {
	if s.next == 0 {
		s.next = 1
	}
	if !ladder {
		dy = 0
	}
	s.walked = float32(float64(s.walked) + float64(float32(math.Sqrt(dx*dx+dy*dy+dz*dz)))*0.6)
	if s.walked <= float32(s.next) || air {
		return false
	}
	s.next = int(s.walked) + 1
	return true
}

// SwimVolume is a step's swim sound volume in water: the motion's length
// with the horizontal parts weighted 0.2, times 0.35, at most 1.
func SwimVolume(vx, vy, vz float64) float32 {
	return min(float32(math.Sqrt(vx*vx*0.20000000298023224+vy*vy+vz*vz*0.20000000298023224))*0.35, 1)
}
