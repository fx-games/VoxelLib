package tacz

import "github.com/memmaker/voxellib/weapon"

// taczRecoilPush is GunRecoilPushData as the pack writes it: sneak and
// crawl default to ground, projectile_multiplier to 1.
type taczRecoilPush struct {
	Ground               float64  `json:"ground_strength"`
	Air                  float64  `json:"air_strength"`
	Sneak                *float64 `json:"sneak_strength"`
	Crawl                *float64 `json:"crawl_strength"`
	PerProjectile        bool     `json:"per_projectile"`
	ProjectileMultiplier *float64 `json:"projectile_multiplier"`
}

// defaultProjectileMultiplier is GunRecoilPushData's projectileMultiplier default.
const defaultProjectileMultiplier = 1.0

// spec is the push as weapon.RecoilPush; nil without one.
func (t *taczRecoilPush) spec() *weapon.RecoilPush {
	if t == nil {
		return nil
	}
	or := func(v *float64, def float64) float64 {
		if v == nil {
			return def
		}
		return *v
	}
	return &weapon.RecoilPush{Ground: t.Ground, Air: t.Air, Sneak: or(t.Sneak, t.Ground), Crawl: or(t.Crawl, t.Ground),
		PerProjectile: t.PerProjectile, ProjectileMultiplier: or(t.ProjectileMultiplier, defaultProjectileMultiplier)}
}
