package tacz

import (
	"encoding/json"
	"maps"
	"math"
	"os"
	"slices"
	"testing"

	"github.com/memmaker/voxellib/weapon"
)

func loadPack(t *testing.T) *Pack {
	t.Helper()
	p, err := Load(os.DirFS("../../assets/tacz/tacz_default_gun"), os.DirFS("../../assets/tacz/defaults"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRefit installs attachments on the AK-47 and checks the spec and
// model follow the choice, and that every gun builds its spec with every
// attachment and slot adapter it allows.
func TestRefit(t *testing.T) {
	p := loadPack(t)
	const gun, scope, silencer, mag = "ak47", "tacz:scope_acog_ta31", "tacz:muzzle_silencer_ursus", "tacz:extended_mag_2"
	if got := p.AttachmentSlots(gun); !slices.Equal(got, []Slot{SlotScope, SlotMuzzle, SlotStock, SlotExtendedMag}) {
		t.Errorf("%s slots %v", gun, got)
	}
	for s, id := range map[Slot]string{SlotScope: scope, SlotMuzzle: silencer, SlotExtendedMag: mag} {
		if !slices.Contains(p.Allowed(gun, s, ""), id) {
			t.Fatalf("%s %s: %v lacks %s", gun, s, p.Allowed(gun, s, ""), id)
		}
	}
	c := p.DefaultChoice(gun)
	c.Attach(SlotScope, scope)
	c.Attach(SlotMuzzle, silencer)
	c.Attach(SlotExtendedMag, mag)
	g, err := p.Gun(gun, c, firstMode(p, gun))
	if err != nil {
		t.Fatal(err)
	}
	if g.Sight != graftName(SlotScope, "scope_view") || g.Zoom <= 1 || g.Mag != 37 || g.Silence == nil || !g.Silence.UseSilenceSound || g.Silence.DistanceAddend >= 0 {
		t.Errorf("refitted %s: sight %q zoom %v mag %d silence %+v", gun, g.Sight, g.Zoom, g.Mag, g.Silence)
	}
	if m, lens, err := p.Model(gun, c); err != nil || lens == nil || m.Bone(g.Sight) < 0 {
		t.Errorf("refitted %s model: lens %v, %v", gun, lens, err)
	}
	if p.AttachmentSound(scope, SoundInstall) == "" || p.AttachmentSound(scope, SoundUninstall) == "" {
		t.Errorf("%s: no refit sounds", scope)
	}
	if b, err := p.Gun(gun, Choice{Attachments: map[Slot]string{SlotMuzzle: "tacz:bayonet_6h3"}}, firstMode(p, gun)); err != nil || b.Melee == nil || b.Melee.Kind != weapon.MeleeBayonet {
		t.Errorf("%s with a bayonet: melee %+v, %v", gun, b.Melee, err)
	}
	if _, err := p.Gun(gun, Choice{Attachments: map[Slot]string{SlotScope: silencer}}, firstMode(p, gun)); err == nil {
		t.Error("a muzzle in the scope slot was accepted")
	}
	for _, spec := range p.Guns {
		for _, s := range p.AttachmentSlots(spec.ID) {
			for _, ad := range append([]string{""}, p.SlotAdapters(spec.ID, s)...) {
				for _, id := range p.Allowed(spec.ID, s, ad) {
					c := p.DefaultChoice(spec.ID)
					c.Adapt(s, ad)
					c.Attach(s, id)
					if _, err := p.Gun(spec.ID, c, firstMode(p, spec.ID)); err != nil {
						t.Error(err)
					}
				}
			}
		}
	}
}

func TestDefaultMeleeKnockback(t *testing.T) {
	var m taczGunMelee
	if err := json.Unmarshal([]byte(`{"default":{"damage":2}}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.Default == nil || m.Default.Knockback != defaultKnockback || m.Default.Prep != 0.1 || m.Cooldown != 1 {
		t.Fatalf("%+v %+v", m, m.Default)
	}
	if err := json.Unmarshal([]byte(`{"distance":2}`), &m); err != nil || m.Default != nil {
		t.Fatalf("no default: %+v %v", m, err)
	}
}

// near reports whether got is want within float32 rounding.
func near(got, want float32) bool {
	return math.Abs(float64(got-want)) <= 1e-5*math.Max(1, math.Abs(float64(want)))
}

// TestModifiers checks each modifier kind on pack attachments against the
// numbers in their data, as TACZ's eval combines them.
func TestModifiers(t *testing.T) {
	p := loadPack(t)
	scaled := func(keys []weapon.RecoilKey, k float32) []weapon.RecoilKey {
		out := slices.Clone(keys)
		for i := range out {
			out[i].Min, out[i].Max = out[i].Min*k, out[i].Max*k
		}
		return out
	}
	recoilEq := func(a, b []weapon.RecoilKey) bool {
		return slices.EqualFunc(a, b, func(x, y weapon.RecoilKey) bool { return x.T == y.T && near(x.Min, y.Min) && near(x.Max, y.Max) })
	}
	type value struct {
		name      string
		got, want float32
	}
	for _, tc := range []struct {
		gun  string
		atts map[Slot]string
		// want lists the modified values against the unmodified spec b.
		want func(b, g *weapon.Gun) []value
		// recoil is the expected pitch and yaw factors.
		pitch, yaw float32
	}{
		{"ak47", map[Slot]string{SlotMuzzle: "tacz:muzzle_brake_cthulhu"}, func(b, g *weapon.Gun) []value {
			return []value{{"ads addend", g.Aim, b.Aim + 0.02}, {"weight", g.Weight, b.Weight + 0.25},
				{"inaccuracy stand", g.Inaccuracy.Stand, b.Inaccuracy.Stand * 1.1}, {"inaccuracy move", g.Inaccuracy.Move, b.Inaccuracy.Move * 1.1},
				{"aim_inaccuracy", g.Inaccuracy.Aim, b.Inaccuracy.Aim * 0.9}, {"sneak (untouched)", g.Inaccuracy.Sneak, b.Inaccuracy.Sneak}}
		}, 0.85, 0.8},
		{"m1866", map[Slot]string{SlotMuzzle: "tacz:oem_barrel_cannon"}, func(b, g *weapon.Gun) []value {
			return []value{{"ads multiplier", g.Aim, b.Aim * 1.1}, {"ammo_speed", g.Bullet.Speed, b.Bullet.Speed * 1.25},
				{"rpm addend", g.RPM, b.RPM - 115}, {"effective_range", g.Bullet.Falloff[0].Dist, b.Bullet.Falloff[0].Dist * 2},
				{"second falloff (untouched)", g.Bullet.Falloff[1].Dist, b.Bullet.Falloff[1].Dist}}
		}, 0.75, 1},
		{"hm50b", map[Slot]string{SlotMuzzle: "tacz:muzzle_silencer_vulture"}, func(b, g *weapon.Gun) []value {
			return []value{{"head_shot addend", g.Bullet.Headshot, b.Bullet.Headshot + 0.25},
				{"effective_range", g.Bullet.Falloff[0].Dist, b.Bullet.Falloff[0].Dist * 1.25}}
		}, 0.66, 0.75},
		{"hk_mk23", map[Slot]string{SlotLaser: "tacz:laser_peq6"}, func(b, g *weapon.Gun) []value {
			return []value{{"sneak_inaccuracy", g.Inaccuracy.Sneak, b.Inaccuracy.Sneak * 0.35}, {"inaccuracy", g.Inaccuracy.Stand, b.Inaccuracy.Stand * 0.5},
				{"aim_inaccuracy", g.Inaccuracy.Aim, b.Inaccuracy.Aim * 0.5}, {"head_shot", g.Bullet.Headshot, b.Bullet.Headshot + 0.5}}
		}, 1, 1},
		// exclusive_attachments with the deprecated forms: weight, ads_addend,
		// inaccuracy_addend, recoil_modifier (percents)
		{"db_short", map[Slot]string{SlotStock: "tacz:oem_stock_tactical"}, func(b, g *weapon.Gun) []value {
			return []value{{"weight (deprecated)", g.Weight, b.Weight + 2}, {"ads_addend", g.Aim, b.Aim + 0.04},
				{"inaccuracy_addend stand", g.Inaccuracy.Stand, b.Inaccuracy.Stand - 0.125}, {"inaccuracy_addend move", g.Inaccuracy.Move, b.Inaccuracy.Move - 0.125},
				{"sneak (untouched)", g.Inaccuracy.Sneak, b.Inaccuracy.Sneak}}
		}, 0.7, 0.8},
		// two attachments: percents add, multipliers multiply, addends add
		{"db_short", map[Slot]string{SlotStock: "tacz:oem_stock_tactical", SlotExtendedMag: "tacz:ammo_mod_overpressure_recoil"}, func(b, g *weapon.Gun) []value {
			return []value{{"inaccuracy", g.Inaccuracy.Stand, (b.Inaccuracy.Stand - 0.125) * 1.5},
				{"damage", g.Bullet.Damage, b.Bullet.Damage * 1.5}, {"damage falloff", g.Bullet.Falloff[1].Damage, b.Bullet.Falloff[1].Damage * 1.5},
				{"knockback multiplier", g.Bullet.Knockback, 0.875 * 2}}
		}, 0.7 * 1.25, 0.8},
		{"b93r", map[Slot]string{SlotMuzzle: "tacz:muzzle_silencer_wraith"}, func(b, g *weapon.Gun) []value {
			return []value{{"ammo_speed", g.Bullet.Speed, b.Bullet.Speed * 1.2}, {"effective_range", g.Bullet.Falloff[0].Dist, b.Bullet.Falloff[0].Dist * 1.5}}
		}, 1, 1},
		// Lua functions: "if (x > 2) then y = x + 2 else y = x end", "y = 1"
		{"ai_awp", map[Slot]string{SlotExtendedMag: "tacz:ammo_mod_fmj"}, func(b, g *weapon.Gun) []value {
			return []value{{"pierce function (then)", float32(g.Bullet.Pierce), float32(b.Bullet.Pierce + 2)},
				{"damage", g.Bullet.Damage, b.Bullet.Damage * 0.9}, {"ammo_speed", g.Bullet.Speed, b.Bullet.Speed * 1.1}}
		}, 1, 1},
		{"aa12", map[Slot]string{SlotExtendedMag: "tacz:ammo_mod_fmj"}, func(b, g *weapon.Gun) []value {
			return []value{{"pierce function (else)", float32(g.Bullet.Pierce), float32(b.Bullet.Pierce)}}
		}, 1, 1},
		{"ai_awp", map[Slot]string{SlotExtendedMag: "tacz:ammo_mod_he"}, func(b, g *weapon.Gun) []value {
			return []value{{"pierce y = 1", float32(g.Bullet.Pierce), 1}, {"head_shot y = 1", g.Bullet.Headshot, 1},
				{"rpm multiplier (rounded)", g.RPM, float32(math.Floor(float64(b.RPM)*0.85 + 0.5))}}
		}, 1, 1},
		{"ai_awp", map[Slot]string{SlotExtendedMag: "tacz:ammo_mod_hp"}, func(b, g *weapon.Gun) []value {
			return []value{{"pierce multiplier 0", float32(g.Bullet.Pierce), 0}}
		}, 1, 1},
	} {
		g, err := p.Gun(tc.gun, Choice{Attachments: tc.atts}, firstMode(p, tc.gun))
		if err != nil {
			t.Error(err)
			continue
		}
		b := p.guns[tc.gun].base
		for _, v := range tc.want(&b, &g) {
			if !near(v.got, v.want) {
				t.Errorf("%s %v: %s %v, want %v", tc.gun, slices.Sorted(maps.Values(tc.atts)), v.name, v.got, v.want)
			}
		}
		if !recoilEq(g.Recoil.Pitch, scaled(b.Recoil.Pitch, tc.pitch)) || !recoilEq(g.Recoil.Yaw, scaled(b.Recoil.Yaw, tc.yaw)) {
			t.Errorf("%s %v: recoil %v %v, want ×%v ×%v of %v %v", tc.gun, tc.atts, g.Recoil.Pitch, g.Recoil.Yaw, tc.pitch, tc.yaw, b.Recoil.Pitch, b.Recoil.Yaw)
		}
	}
}

// knownUnsupported are the attachment data keys the spec does not use
// (UnsupportedData); a change to that list must be deliberate.
var knownUnsupported = []string{"armor_ignore", "explosion", "flash_hider", "ignite", "lie_inaccuracy", "recoil_push",
	"smoke_count_scale", "smoke_direction", "smoke_directions", "smoke_hider", "smoke_speed_scale", "smoke_spread", "smoke_spread_angle"}

// TestModifierForms checks the forms no pack attachment uses
// (weight_modifier, movement_speed, functions with r) and that unknown or
// unsupported ones fail.
func TestModifierForms(t *testing.T) {
	if got := slices.Sorted(maps.Keys(UnsupportedData)); !slices.Equal(got, knownUnsupported) {
		t.Errorf("UnsupportedData %v, want %v", got, knownUnsupported)
	}
	m, err := parseMods([]byte(`{"weight": 9, "weight_modifier": {"percent": -0.5}, "movement_speed": {"aim": -0.5, "reload": 0.1},
		"rpm": {"function": "IF x >= 2*r THEN y = r ELSEIF -x < -100 then y = (x - 100) / 2 end"}}`))
	if err != nil {
		t.Fatal(err)
	}
	g := weapon.Gun{Weight: 4, RPM: 300, MoveSpeed: weapon.MoveSpeed{Base: -0.1, Aim: 0.2}}
	if err := applyMods(&g, modeAdjust{}, []attMods{m}); err != nil {
		t.Fatal(err)
	}
	// weight_modifier wins over weight; MoveSpeed.of sums and clamps at 0; the
	// function's elseif: -300 < -100, y = 100
	if g.Weight != 2 || g.MoveSpeed != (weapon.MoveSpeed{Base: 0, Aim: 0, Reload: 0.1}) || g.RPM != 100 {
		t.Errorf("weight %v, move speed %+v, rpm %v", g.Weight, g.MoveSpeed, g.RPM)
	}
	for _, bad := range []string{`{"no_such_modifier": 1}`, `{"rpm": {"function": "y = math.max(x, 1)"}}`, `{"rpm": {"function": "z = 1"}}`} {
		if _, err := parseMods([]byte(bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if err := applyMods(&weapon.Gun{}, modeAdjust{}, []attMods{{pitch: modifier{addend: 1, multiplier: 1}, yaw: identity}}); err == nil {
		t.Error("a recoil addend was accepted")
	}
	// no attachments: GunData.getShootInterval still clamps the cached rpm
	fast := weapon.Gun{RPM: 1500}
	if err := applyMods(&fast, modeAdjust{}, nil); err != nil || fast.RPM != maxRPM {
		t.Errorf("rpm 1500 without attachments: %v %v, want %v", fast.RPM, err, maxRPM)
	}
	// EntityKineticBullet clamps the knockback at 0
	kb := weapon.Gun{}
	kb.Bullet.Knockback = 0.5
	if err := applyMods(&kb, modeAdjust{}, []attMods{{knockback: modifier{addend: -1, multiplier: 1}, pitch: identity, yaw: identity}}); err != nil || kb.Bullet.Knockback != 0 {
		t.Errorf("knockback 0.5-1: %v %v, want 0", kb.Bullet.Knockback, err)
	}
}

// TestDamageAt follows EntityKineticBullet.getDamage: the first pair the
// hit is strictly closer than, 0 past the last; a gun without
// damage_adjust has the one pair (Integer.MAX_VALUE, damage).
func TestDamageAt(t *testing.T) {
	p := loadPack(t)
	g, err := p.Gun("m1866", p.DefaultChoice("m1866"), firstMode(p, "m1866"))
	if err != nil {
		t.Fatal(err)
	}
	f := g.Bullet.Falloff
	last := f[len(f)-1]
	if got := g.DamageAt(f[0].Dist); got != f[1].Damage {
		t.Errorf("at the first distance %v: %v, want the second pair's %v", f[0].Dist, got, f[1].Damage)
	}
	if got := g.DamageAt(1e30); last.Dist != math.MaxFloat32 || got != last.Damage {
		t.Errorf("infinite last pair %v: %v, want %v", last.Dist, got, last.Damage)
	}
	var short weapon.Gun
	short.Bullet.Falloff = []weapon.Falloff{{Dist: 10, Damage: 5}}
	if got := short.DamageAt(10); got != 0 {
		t.Errorf("past the last pair: %v, want 0", got)
	}
	for id, pg := range p.guns {
		if b := pg.base.Bullet; len(b.Falloff) == 0 || (b.Falloff[0].Dist == noAdjustRange && b.Falloff[0].Damage != b.Damage) {
			t.Errorf("%s: falloff %v", id, b.Falloff)
		}
	}
}

// TestSlotAdapters raises a sight on a riser: the adapter's model is
// grafted and the sight's view bone sits mount_offset higher.
func TestSlotAdapters(t *testing.T) {
	p := loadPack(t)
	const gun, riser, sight = "aa12", "tacz:riser_rail_high", "tacz:sight_t2"
	if !slices.Contains(p.SlotAdapters(gun, SlotScope), riser) {
		t.Fatalf("%s: adapters %v", gun, p.SlotAdapters(gun, SlotScope))
	}
	if !slices.Contains(p.Allowed(gun, SlotScope, riser), sight) || slices.Contains(p.Allowed(gun, SlotScope, riser), "tacz:scope_acog_ta31") {
		t.Errorf("%s on %s allows %v", gun, riser, p.Allowed(gun, SlotScope, riser))
	}
	if _, err := p.Gun(gun, Choice{Attachments: map[Slot]string{SlotScope: "tacz:scope_acog_ta31"}, Adapters: map[Slot]string{SlotScope: riser}}, firstMode(p, gun)); err == nil {
		t.Error("a scope the riser does not take was accepted")
	}
	viewY := func(c Choice) (float32, int) {
		g, err := p.Gun(gun, c, firstMode(p, gun))
		if err != nil {
			t.Fatal(err)
		}
		m, _, err := p.Model(gun, c)
		if err != nil {
			t.Fatal(err)
		}
		adapterBones := 0
		for _, b := range m.Bones {
			if len(b.Name) > len(graftName(SlotScope, adapterBone)) && b.Name[:len(graftName(SlotScope, adapterBone))] == graftName(SlotScope, adapterBone) {
				adapterBones++
			}
		}
		return m.BindWorld()[m.Bone(g.Sight)].Col(3).Y(), adapterBones
	}
	plain := Choice{Attachments: map[Slot]string{SlotScope: sight}}
	risen := plain.Clone()
	risen.Adapt(SlotScope, riser)
	y0, n0 := viewY(plain)
	y1, n1 := viewY(risen)
	if want := p.adapters[riser].lift / 16; !near(y1-y0, want) || n0 != 0 || n1 == 0 {
		t.Errorf("riser lifts the view %v (want %v); adapter bones %d, %d", y1-y0, want, n0, n1)
	}
}

// firstMode is the fire mode Pack.Guns resolves gun id in.
func firstMode(p *Pack, id string) weapon.FireMode { b := p.guns[id].base; return b.FireModes()[0] }

// TestFireModeAdjust: type_81's semi fire_mode_adjust (damage +2, rpm −330,
// speed +20, head_shot_multiplier +0.25, aim_inaccuracy −0.05,
// other_inaccuracy −0.5) is added to the bases before the attachments
// (ammo_mod_fmj: damage ×0.9 of 11, speed ×1.1 of 345); auto has none.
func TestFireModeAdjust(t *testing.T) {
	type value struct {
		name      string
		got, want float32
	}
	p := loadPack(t)
	const gun = "type_81"
	auto, err := p.Gun(gun, Choice{}, weapon.Auto)
	if err != nil {
		t.Fatal(err)
	}
	semi, err := p.Gun(gun, Choice{}, weapon.Semi)
	if err != nil {
		t.Fatal(err)
	}
	if auto.RPM != 650 || auto.Bullet.Falloff[0].Damage != 9 {
		t.Errorf("auto rpm %v damage %v, want the gun's 650 9", auto.RPM, auto.Bullet.Falloff[0].Damage)
	}
	sb, ab := semi.Bullet, auto.Bullet
	for _, v := range []value{
		{"rpm", semi.RPM, 320}, {"damage", sb.Damage, ab.Damage + 2},
		{"falloff 0", sb.Falloff[0].Damage, 11}, {"falloff 1", sb.Falloff[1].Damage, 9}, {"falloff 2", sb.Falloff[2].Damage, 6.5},
		{"speed", sb.Speed, ab.Speed + 20}, {"headshot", sb.Headshot, ab.Headshot + 0.25},
		{"aim", semi.Inaccuracy.Aim, auto.Inaccuracy.Aim - 0.05}, {"stand", semi.Inaccuracy.Stand, auto.Inaccuracy.Stand - 0.5},
		{"move", semi.Inaccuracy.Move, auto.Inaccuracy.Move - 0.5}, {"sneak", semi.Inaccuracy.Sneak, auto.Inaccuracy.Sneak - 0.5},
	} {
		if !near(v.got, v.want) {
			t.Errorf("semi %s %v, want %v", v.name, v.got, v.want)
		}
	}
	fmj, err := p.Gun(gun, Choice{Attachments: map[Slot]string{SlotExtendedMag: "tacz:ammo_mod_fmj"}}, weapon.Semi)
	if err != nil {
		t.Fatal(err)
	}
	if d, s := fmj.Bullet.Falloff[0].Damage, fmj.Bullet.Speed; !near(d, 9.9) || !near(s, 379.5) {
		t.Errorf("semi + ammo_mod_fmj damage %v speed %v, want 9.9 379.5", d, s)
	}
	if _, err := p.Gun(gun, Choice{}, weapon.Burst); err == nil {
		t.Error("type_81 resolved in burst, which it lacks")
	}
}
