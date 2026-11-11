package actors

import (
	"math"
	"testing"
)

// TestKnockbackMotion: LivingEntity.knockback halves the motion and pushes
// away from the source, lifts on the ground (capped), and power 0 (a TACZ
// gun without knockback) changes nothing.
func TestKnockbackMotion(t *testing.T) {
	if vx, vy, vz := KnockbackMotion(0.2, -0.1, 0, true, 0.3, 1, 0); math.Abs(vx+0.2) > 1e-12 || math.Abs(vy-0.25) > 1e-12 || vz != 0 {
		t.Errorf("ground push: %v %v %v", vx, vy, vz)
	}
	if _, vy, _ := KnockbackMotion(0, 0.5, 0, true, 0.4, 0, 1); vy != knockbackMaxLift {
		t.Errorf("lift %v, want the cap", vy)
	}
	if _, vy, _ := KnockbackMotion(0, -0.3, 0, false, 0.4, 0, 1); vy != -0.3 {
		t.Errorf("airborne vy %v changed", vy)
	}
	if vx, vy, vz := KnockbackMotion(0.2, 0.1, 0.3, true, 0, 1, 0); vx != 0.2 || vy != 0.1 || vz != 0.3 {
		t.Errorf("power 0 moved: %v %v %v", vx, vy, vz)
	}
}
