package weapon

import (
	"fmt"
	"math"
)

// MuzzleFlash is a gun display's "renew" muzzle_flash block (TACZ
// MuzzleFlash), resolved for a gun without a muzzle attachment.
type MuzzleFlash struct {
	// Flash and Smoke are the first-person particle definition files in
	// the pack; "" when the display sets them to null (no such layer).
	Flash string `json:"flash"`
	Smoke string `json:"smoke"`
	// Scale sizes the flash (v.muzzle_scale); 0 hides flash and smoke (TACZ creates neither emitter).
	Scale float32 `json:"scale"`
	// FlashLight is the gun's minimum block light while firing (0..15).
	FlashLight int `json:"flash_light"`
	// SmokeScale, SpeedScale and SmokeCount feed v.smoke_size, v.smoke_vel (and v.muzzle_speed)
	// and v.smoke_count; 0 lets the definition use its built-in default.
	SmokeScale float32 `json:"smoke_scale"`
	SpeedScale float32 `json:"speed_scale"`
	SmokeCount float32 `json:"smoke_count"`
	// Direction is the smoke's emission direction; nil scatters it evenly.
	Direction *SmokeDirection `json:"direction,omitempty"`
}

// MaxSmokeDirections is SmokeDirectionData.MAX_DIRECTIONS.
const MaxSmokeDirections = 4

// SmokeDirection is SmokeDirectionData.Config in the display's intuitive
// space (+X right, +Y up, -Z out of the muzzle), unit vectors.
type SmokeDirection struct {
	Dirs [][3]float32 `json:"dirs"`
	// Spread is a lateral offset (vector mode) or a cone half angle in
	// degrees (AngleMode).
	Spread    float32 `json:"spread"`
	AngleMode bool    `json:"angle_mode"`
}

// FlashVars are the Molang variables MuzzleParticleManager.createFlashEmitter
// sets on the flash emitter.
func (m MuzzleFlash) FlashVars() map[string]float64 {
	return map[string]float64{"muzzle_scale": float64(m.Scale), "muzzle_speed": float64(m.SpeedScale)}
}

// SmokeVars are the Molang variables createSmokeEmitter and
// writeSmokeDirectionVariables set on the smoke emitter.
func (m MuzzleFlash) SmokeVars() map[string]float64 {
	v := map[string]float64{"smoke_size": float64(m.SmokeScale), "smoke_vel": float64(m.SpeedScale),
		"smoke_count": float64(m.SmokeCount)}
	d := m.Direction
	n := 0
	if d != nil {
		n = len(d.Dirs)
		v["smoke_spread"] = float64(d.Spread)
		if d.AngleMode {
			v["smoke_angle_mode"] = 1
		}
	}
	v["smoke_dir_n"] = float64(n)
	for i := range MaxSmokeDirections {
		var dir, u, w [3]float32
		if i < n {
			// resolveSmokeDirection undoes the gun pose's (x, y, z) → (-x, -y, z) flip.
			dir = [3]float32{-d.Dirs[i][0], -d.Dirs[i][1], d.Dirs[i][2]}
			x, y, z := dir[0], dir[1], dir[2]
			if lat := float32(math.Hypot(float64(x), float64(y))); lat > 1e-6 {
				u = [3]float32{-y / lat, x / lat, 0}
			} else {
				u = [3]float32{1, 0, 0}
			}
			w = [3]float32{y*u[2] - z*u[1], z*u[0] - x*u[2], x*u[1] - y*u[0]}
		}
		for k, axis := range [3]string{"x", "y", "z"} {
			v[fmt.Sprintf("smoke_dir%d_%s", i, axis)] = float64(dir[k])
			v[fmt.Sprintf("smoke_u%d_%s", i, axis)] = float64(u[k])
			v[fmt.Sprintf("smoke_v%d_%s", i, axis)] = float64(w[k])
		}
	}
	return v
}
