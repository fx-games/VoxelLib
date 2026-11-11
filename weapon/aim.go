package weapon

import "time"

// AimProgress is how far a gun's sights are raised, 0..1 (TACZ
// ShooterDataHolder.aimingProgress). The server runs it for every player
// and syncs it (AIMING_PROGRESS_KEY); the local player runs its own for
// the first-person view (LocalPlayerAim), with the same step.
type AimProgress float32

// Aimed is full aim: TACZ's InaccuracyType.AIM applies only at exactly 1.
const Aimed AimProgress = 1

// Tick advances p over dt toward 1 while aiming, toward 0 otherwise, at
// one aim time (seconds, with attachment modifiers) per full raise
// (LivingEntityAim.tickAimingProgress: (elapsed ms + 1) / (aimTime·1000)
// per tick; an aim time of 0 raises at once).
func (p AimProgress) Tick(aiming bool, aimTime float32, dt time.Duration) AimProgress {
	step := AimProgress(1)
	if aimTime > 0 {
		step = AimProgress((float32(dt.Milliseconds()) + 1) / (aimTime * 1000))
	}
	if aiming {
		return min(p+step, Aimed)
	}
	return max(p-step, 0)
}
