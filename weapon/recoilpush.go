package weapon

// RecoilPush is TACZ's GunRecoilPushData: a shot pushes the shooter back
// along its look by a strength (blocks/tick) chosen by stance
// (ModernKineticGunScriptAPI.applyRecoilPush). The importer fills Sneak
// and Crawl with Ground when the data leaves them out.
type RecoilPush struct {
	Ground, Air, Sneak, Crawl float64
	// PerProjectile scales the strength by the pellets times
	// ProjectileMultiplier.
	PerProjectile        bool
	ProjectileMultiplier float64
}

// maxRecoilPush is GunRecoilPushData.MAX_RECOIL_PUSH_STRENGTH.
const maxRecoilPush = 4.0

func clampPush(s float64) float64 { return min(max(s, 0), maxRecoilPush) }

// Strength is getFinalStrength: by stance (in the air before crawling
// before sneaking), times the pellets for a per-projectile push, clamped.
func (r *RecoilPush) Strength(onGround, sneaking, crawling bool, pellets int) float64 {
	var s float64
	switch {
	case !onGround:
		s = clampPush(r.Air)
	case crawling:
		s = clampPush(r.Crawl)
	case sneaking:
		s = clampPush(r.Sneak)
	default:
		s = clampPush(r.Ground)
	}
	if r.PerProjectile {
		s *= float64(max(pellets, 1)) * max(r.ProjectileMultiplier, 0)
	}
	return clampPush(s)
}

// Push is the shooter's velocity change for a shot along the unit look
// dir: -dir × Strength (applyRecoilPush's directionFromRotation(pitch,
// yaw).scale(-strength)); zero for no push. TACZ pushes no passenger; there
// are no mounts here, and no crawling stance.
func (r *RecoilPush) Push(dir [3]float64, onGround, sneaking bool, pellets int) [3]float64 {
	if r == nil {
		return [3]float64{}
	}
	s := r.Strength(onGround, sneaking, false, pellets)
	if s <= 0 {
		return [3]float64{}
	}
	return [3]float64{-dir[0] * s, -dir[1] * s, -dir[2] * s}
}
