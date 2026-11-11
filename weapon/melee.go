package weapon

import "math"

// Reaches is ModernKineticGunItem.doMelee's cone test: the apex sits
// Distance behind the attacker's eye along look (a unit vector), so the
// target's eye must lie at least Distance from it and within half the
// Angle of look. The caller also requires the target's box to meet the
// attacker's box grown by Distance, and a clear line of sight.
func (m *Melee) Reaches(eye, look, target [3]float64) bool {
	d := float64(m.Distance)
	v := [3]float64{target[0] - eye[0] + look[0]*d, target[1] - eye[1] + look[1]*d, target[2] - eye[2] + look[2]*d}
	l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if l < d || l == 0 {
		return false
	}
	return math.Acos(min(1, (v[0]*look[0]+v[1]*look[1]+v[2]*look[2])/l))*180/math.Pi < float64(m.Angle)/2
}
