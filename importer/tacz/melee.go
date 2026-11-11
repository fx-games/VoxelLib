package tacz

import (
	"encoding/json"

	"github.com/memmaker/voxellib/weapon"
)

// taczMelee is a melee block (attachment MeleeData, gun GunDefaultMeleeData),
// their defaults where absent.
type taczMelee struct {
	Animation string  `json:"animation_type"` // the gun's default only
	Distance  float32 `json:"distance"`
	Angle     float32 `json:"range_angle"`
	Cooldown  float32 `json:"cooldown"`
	Damage    float32 `json:"damage"`
	Knockback float32 `json:"knockback"`
	Prep      float32 `json:"prep"`
}

func (m *taczMelee) UnmarshalJSON(b []byte) error {
	type plain taczMelee
	p := plain{Distance: 1, Angle: 30, Prep: 0.1}
	err := json.Unmarshal(b, &p)
	*m = taczMelee(p)
	return err
}

// taczGunMelee is a gun data's "melee" (GunMeleeData).
type taczGunMelee struct {
	Distance float32    `json:"distance"`
	Cooldown float32    `json:"cooldown"`
	Default  *taczMelee `json:"default"`
}

func (m *taczGunMelee) UnmarshalJSON(b []byte) error {
	var p struct {
		Distance float32         `json:"distance"`
		Cooldown float32         `json:"cooldown"`
		Default  json.RawMessage `json:"default"`
	}
	p.Distance, p.Cooldown = 1, 1
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*m = taczGunMelee{Distance: p.Distance, Cooldown: p.Cooldown}
	if len(p.Default) == 0 || string(p.Default) == "null" {
		return nil
	}
	// GunDefaultMeleeData: MeleeData's defaults but knockback defaultKnockback.
	d := taczMelee{Distance: 1, Angle: 30, Prep: 0.1, Knockback: defaultKnockback}
	type plain taczMelee
	if err := json.Unmarshal(p.Default, (*plain)(&d)); err != nil {
		return err
	}
	m.Default = &d
	return nil
}

// defaultKnockback is GunDefaultMeleeData's knockback (MeleeData's is 0).
const defaultKnockback = 0.2

// meleeStockAnimation is the default melee's stock animation_type (else a push).
const meleeStockAnimation = "melee_stock"

// gunMelee resolves the gun's melee as LivingEntityMelee: the muzzle
// attachment's, else the stock's, else the gun's default; distances and
// cooldowns add the gun's own. Nil: no melee.
func gunMelee(gun taczGunMelee, atts map[Slot]attachment) *weapon.Melee {
	m, kind := gun.Default, weapon.MeleePush
	switch {
	case atts[SlotMuzzle].melee != nil:
		m, kind = atts[SlotMuzzle].melee, weapon.MeleeBayonet
	case atts[SlotStock].melee != nil:
		m, kind = atts[SlotStock].melee, weapon.MeleeStock
	case m == nil:
		return nil
	case m.Animation == meleeStockAnimation:
		kind = weapon.MeleeStock
	}
	return &weapon.Melee{Kind: kind, Distance: gun.Distance + m.Distance, Angle: m.Angle,
		Cooldown: gun.Cooldown + m.Cooldown, Damage: m.Damage, Knockback: m.Knockback, Prep: m.Prep}
}
