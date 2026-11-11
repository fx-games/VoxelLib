package weapon

import "fmt"

// Guns: the spec of a firearm, as importer/tacz reads it from a TACZ gun pack. Times are seconds, speeds blocks/second, angles degrees.

// FireMode is how a trigger pull fires.
type FireMode uint8

const (
	Semi FireMode = iota
	Auto
	Burst
)

// Bolt is the action that chambers the next round.
type Bolt uint8

const (
	ClosedBolt   Bolt = iota // a round stays chambered: a tactical reload holds mag+1
	OpenBolt                 // no chambered round
	ManualAction             // pump or bolt: the "bolt" clip runs after each shot
)

// ReloadKind is how ammunition goes in.
type ReloadKind uint8

const (
	Magazine  ReloadKind = iota // one feed fills the magazine
	Tube                        // reload_intro, then reload_loop once per round, then reload_end
	Inventory                   // no magazine: shots come straight from the stock (minigun)
)

var (
	fireModeNames   = []string{"semi", "auto", "burst"}
	boltNames       = []string{"closed_bolt", "open_bolt", "manual_action"}
	reloadKindNames = []string{"magazine", "tube", "inventory"}
)

func enumText(names []string, v uint8) ([]byte, error) {
	if int(v) >= len(names) {
		return nil, fmt.Errorf("weapon: enum value %d out of range", v)
	}
	return []byte(names[v]), nil
}

func enumParse(names []string, b []byte) (uint8, error) {
	for i, n := range names {
		if n == string(b) {
			return uint8(i), nil
		}
	}
	return 0, fmt.Errorf("weapon: unknown value %q (want one of %v)", b, names)
}

// FireModes are the modes the gun's selector cycles through: Modes, or
// semi when it lists none.
func (g *Gun) FireModes() []FireMode {
	if len(g.Modes) == 0 {
		return []FireMode{Semi}
	}
	return g.Modes
}

func (m FireMode) MarshalText() ([]byte, error) { return enumText(fireModeNames, uint8(m)) }
func (m *FireMode) UnmarshalText(b []byte) error {
	v, err := enumParse(fireModeNames, b)
	*m = FireMode(v)
	return err
}
func (b Bolt) MarshalText() ([]byte, error) { return enumText(boltNames, uint8(b)) }
func (b *Bolt) UnmarshalText(t []byte) error {
	v, err := enumParse(boltNames, t)
	*b = Bolt(v)
	return err
}
func (k ReloadKind) MarshalText() ([]byte, error) { return enumText(reloadKindNames, uint8(k)) }
func (k *ReloadKind) UnmarshalText(t []byte) error {
	v, err := enumParse(reloadKindNames, t)
	*k = ReloadKind(v)
	return err
}

// Falloff is the damage of a bullet hitting closer than Dist (TACZ
// DistanceDamagePair; "infinite" is math.MaxFloat32).
type Falloff struct {
	Dist   float32 `json:"dist"`
	Damage float32 `json:"damage"`
}

// RecoilKey is a recoil curve point: at T after the shot the camera turns by a
// random angle in [Min, Max].
type RecoilKey struct {
	T   float32 `json:"t"`
	Min float32 `json:"min"`
	Max float32 `json:"max"`
}

// Cue is a sound event played at T seconds into a clip.
type Cue struct {
	T     float32 `json:"t"`
	Sound string  `json:"sound"`
}

type Gun struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Class string     `json:"class"` // display group: pistol, rifle, shotgun, sniper, smg, mg, rpg
	Ammo  string     `json:"ammo"`  // Ammo.ID
	Mag   int        `json:"mag"`
	RPM   float32    `json:"rpm"`
	Modes []FireMode `json:"modes"`
	Bolt  Bolt       `json:"bolt"`
	Burst struct {
		Count int     `json:"count"`
		BPM   float32 `json:"bpm"`
	} `json:"burst"`
	Bullet struct {
		Damage   float32 `json:"damage"`
		Pellets  int     `json:"pellets"`
		Speed    float32 `json:"speed"`
		Gravity  float32 `json:"gravity"`
		Pierce   int     `json:"pierce"`
		Headshot float32 `json:"headshot"`
		// Knockback is the push (blocks/tick) a hit gives, replacing the
		// hit's default (TACZ KnockBackModifier).
		Knockback float32   `json:"knockback"`
		Falloff   []Falloff `json:"falloff"`
	} `json:"bullet"`
	Reload struct {
		Kind ReloadKind `json:"kind"`
		// Magazine: the rounds go in at Feed, the reload ends at Done.
		FeedEmpty    float32 `json:"feed_empty"`
		FeedTactical float32 `json:"feed_tactical"`
		DoneEmpty    float32 `json:"done_empty"`
		DoneTactical float32 `json:"done_tactical"`
	} `json:"reload"`
	Draw    float32 `json:"draw"`
	PutAway float32 `json:"put_away"`
	Aim     float32 `json:"aim"`
	Recoil  struct {
		Pitch []RecoilKey `json:"pitch"`
		Yaw   []RecoilKey `json:"yaw"`
	} `json:"recoil"`
	Inaccuracy struct {
		Stand float32 `json:"stand"`
		Move  float32 `json:"move"`
		Sneak float32 `json:"sneak"`
		Aim   float32 `json:"aim"`
	} `json:"inaccuracy"`
	Zoom float32 `json:"zoom"` // aiming magnification (the iron sight's, or the built-in scope's)
	// ModelFOV is the hand projection's field of view (degrees), AimModelFOV
	// the one it narrows to while aiming (the scope's, or the gun's).
	ModelFOV    float32 `json:"model_fov"`
	AimModelFOV float32 `json:"aim_model_fov"`
	// ThirdPersonScale scales the gun in another player's hand (the display's
	// transform.scale.thirdperson, uniform; 1 without one).
	ThirdPersonScale float32 `json:"third_person_scale"`
	// ThirdPersonHold is how another player holds it (third_person_animation).
	ThirdPersonHold ThirdPersonHold `json:"third_person_hold"`
	// Sight is the bone the camera aims through: iron_view, or the view
	// bone of a built-in scope; IdleRaise lifts the idle view (blocks) by
	// the scope's idle_scope_height_adjust, fading out while aiming.
	Sight     string  `json:"sight"`
	IdleRaise float32 `json:"idle_raise,omitempty"`
	Model     string  `json:"model"` // model file in the pack
	Icon      string  `json:"icon"`  // slot sprite file in the pack
	// HUD sprites (39x13) beside the ammo count; HudEmpty "" tints Hud red when empty.
	Hud      string `json:"hud"`
	HudEmpty string `json:"hud_empty"`
	// Sounds are the display's sound events by name (GunDisplay "sounds",
	// with its defaults); Cues are the animations' own, played alongside.
	Sounds map[GunSound]string `json:"sounds"`
	Cues   map[string][]Cue    `json:"cues"` // clip name → sounds
	// Muzzle is the display's renew muzzle flash; nil for the legacy one.
	Muzzle *MuzzleFlash `json:"muzzle,omitempty"`
	// Shell is the casing ejection; nil: the gun ejects none.
	Shell *ShellEjection `json:"shell,omitempty"`
	// Spin: the barrel spins as minigun_state_machine drives it (the
	// "spin" clip looping while the main track is idle, again on top while firing).
	Spin bool `json:"spin,omitempty"`
	// Heat is the overheating; nil: the gun never heats.
	Heat *Heat `json:"heat,omitempty"`
	// Slide: crouching shows the slide pose (gun data can_slide).
	Slide bool `json:"slide,omitempty"`
	// Melee is the gun's melee attack; nil: it has none.
	Melee *Melee `json:"melee,omitempty"`
	// Unload: the rounds in the gun can be taken out (CommonGunIndex.canUnloadAmmo).
	Unload bool `json:"unload,omitempty"`
	// Silence is the installed muzzle attachment's; nil: none silences the gun.
	Silence *Silence `json:"silence,omitempty"`
	// Weight (kg, with the installed attachments') and MoveSpeed slow the
	// holder (SpeedFactor).
	Weight    float32   `json:"weight,omitempty"`
	MoveSpeed MoveSpeed `json:"move_speed"`
}

// MoveSpeed is a gun data's movement_speed (TACZ MoveSpeed): the holder's
// extra speed fraction normally, while aiming and while reloading.
type MoveSpeed struct {
	Base   float32 `json:"base"`
	Aim    float32 `json:"aim"`
	Reload float32 `json:"reload"`
}

// WeightSpeedMultiplier is the speed fraction a kilogram takes off (TACZ
// SyncConfig WeightSpeedMultiplier's default).
const WeightSpeedMultiplier = 0.015

// SpeedFactor scales the holder's movement speed as TACZ's
// LivingEntitySpeedModifier does: the weight's MULTIPLY_BASE modifier
// (-Weight·WeightSpeedMultiplier) times MoveSpeed's MULTIPLY_TOTAL one
// (Reload while reloading, else Aim while aiming, else Base); the
// attribute never goes below 0.
func (g *Gun) SpeedFactor(aiming, reloading bool) float32 {
	extra := g.MoveSpeed.Base
	switch {
	case reloading:
		extra = g.MoveSpeed.Reload
	case aiming:
		extra = g.MoveSpeed.Aim
	}
	return max(0, (1-g.Weight*WeightSpeedMultiplier)*(1+extra))
}

// MeleeKind is which melee a gun does (its animation and sound).
type MeleeKind uint8

const (
	MeleePush    MeleeKind = iota // the gun's own push, no stock
	MeleeStock                    // a butt stroke with the stock
	MeleeBayonet                  // a muzzle attachment's stab
)

var meleeKindNames = []string{"push", "stock", "bayonet"}

func (k MeleeKind) MarshalText() ([]byte, error) { return enumText(meleeKindNames, uint8(k)) }
func (k *MeleeKind) UnmarshalText(t []byte) error {
	v, err := enumParse(meleeKindNames, t)
	*k = MeleeKind(v)
	return err
}

// Melee is a gun's resolved melee attack (TACZ LivingEntityMelee and
// ModernKineticGunItem.doMelee): the muzzle attachment's, else the stock's,
// else the gun's default. The hit lands Prep seconds in; targets within
// Distance blocks and Angle degrees (the full cone) of the look take Damage
// and Knockback; the next melee waits Cooldown seconds from the start.
type Melee struct {
	Kind      MeleeKind `json:"kind"`
	Distance  float32   `json:"distance"`
	Angle     float32   `json:"angle"`
	Cooldown  float32   `json:"cooldown"`
	Damage    float32   `json:"damage"`
	Knockback float32   `json:"knockback"`
	Prep      float32   `json:"prep"`
}

// Silence is an attachment data's "silence" block (TACZ SilenceModifier):
// the fire sound carries DistanceAddend blocks further (negative: less) than
// the default 64, and with UseSilenceSound the display's silence sounds
// replace shoot and shoot_3p.
type Silence struct {
	DistanceAddend  int  `json:"distance_addend"`
	UseSilenceSound bool `json:"use_silence_sound"`
}

// DefaultFireSoundDistance is how far a shot is heard, in blocks
// (GunConfig.DEFAULT_GUN_FIRE_SOUND_DISTANCE).
const DefaultFireSoundDistance = 64

// SoundDistance is how far the gun's shot is heard by the others, in blocks
// (SilenceModifier: the default plus the muzzle's addend; 0 or less: unheard).
func (g *Gun) SoundDistance() int {
	if g.Silence == nil {
		return DefaultFireSoundDistance
	}
	return DefaultFireSoundDistance + g.Silence.DistanceAddend
}

// ShotSound3P is the sound the others hear for a shot: silence_3p with a
// muzzle that has use_silence_sound, else shoot_3p
// (ModernKineticGunScriptAPI.shoot).
func (g *Gun) ShotSound3P() GunSound {
	if g.Silence != nil && g.Silence.UseSilenceSound {
		return SoundSilence3P
	}
	return SoundShoot3P
}

// FlashHidden reports that a shot shows no muzzle flash, smoke only: a
// silencer with use_silence_sound and no flash_hider (MuzzleParticleManager.resolveShot
// sets flashScale 0; flash_hider is not read, see tacz.UnsupportedData).
func (g *Gun) FlashHidden() bool { return g.Silence != nil && g.Silence.UseSilenceSound }

// GunSound names a display sound (TACZ SoundManager's keys).
type GunSound string

// The display sounds the game plays (SoundPlayManager's callers).
const (
	SoundShoot          GunSound = "shoot"    // the shooter's shot
	SoundShoot3P        GunSound = "shoot_3p" // a shot heard by everyone else
	SoundDryFire        GunSound = "dry_fire" // trigger on an empty or overheat-locked gun
	SoundDraw           GunSound = "draw"
	SoundPutAway        GunSound = "put_away"
	SoundReloadEmpty    GunSound = "reload_empty" // reload start with no round in the barrel
	SoundReloadTactical GunSound = "reload_tactical"
	SoundInspect        GunSound = "inspect"
	SoundInspectEmpty   GunSound = "inspect_empty"
	SoundBolt           GunSound = "bolt"
	SoundFireSelect     GunSound = "fire_select"
	SoundHeadHit        GunSound = "head_hit" // the shooter's hit marks
	SoundFleshHit       GunSound = "flesh_hit"
	SoundKill           GunSound = "kill"
	SoundMeleePush      GunSound = "melee_push"
	SoundMeleeStock     GunSound = "melee_stock"
	SoundMeleeBayonet   GunSound = "melee_bayonet"
	SoundUnload         GunSound = "unload"
	SoundSilence        GunSound = "silence"    // the shooter's shot with a silencing muzzle
	SoundSilence3P      GunSound = "silence_3p" // that shot heard by everyone else
)

// GunSounds lists the display sounds the game plays.
var GunSounds = []GunSound{SoundShoot, SoundShoot3P, SoundDryFire, SoundDraw, SoundPutAway,
	SoundReloadEmpty, SoundReloadTactical, SoundInspect, SoundInspectEmpty, SoundBolt,
	SoundFireSelect, SoundHeadHit, SoundFleshHit, SoundKill,
	SoundMeleePush, SoundMeleeStock, SoundMeleeBayonet, SoundUnload, SoundSilence, SoundSilence3P}

// Heat is a gun data's "heat" block (TACZ GunHeatData, sp_heat.lua): each
// shot adds PerShot up to Max, where the gun locks; Delay seconds after the
// last shot (Lock when locked) it cools each tick by Cooling·t/10, t the
// seconds since the shot, the lock lifting at 0. The heat fraction lerps the fire rate's and the
// inaccuracy's factors between [0] and [1].
type Heat struct {
	Max        float32    `json:"max"`
	PerShot    float32    `json:"per_shot"`
	Cooling    float32    `json:"cooling"`
	Delay      float32    `json:"delay"`
	Lock       float32    `json:"lock"`
	RPM        [2]float32 `json:"rpm"`
	Inaccuracy [2]float32 `json:"inaccuracy"`
}

// ShellEjection is a gun display's "shell" block (TACZ ShellEjection):
// each shot throws a casing from the "shell" bone along
// (Velocity + random·Random)·t + Accel·t²/2 (blocks, TACZ's frame),
// turning Angular degrees per second, for Life seconds.
type ShellEjection struct {
	Velocity [3]float32 `json:"velocity"`
	Random   [3]float32 `json:"random"`
	Accel    [3]float32 `json:"accel"`
	Angular  [3]float32 `json:"angular"`
	Life     float32    `json:"life"`
	// Model and Texture are the casing's geometry and texture files in the
	// pack (the gun's own, else its ammo's).
	Model   string `json:"model"`
	Texture string `json:"texture"`
}

type Ammo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stack int    `json:"stack"`
	Icon  string `json:"icon"`
}

// ThirdPersonHold is how another player holds the gun: the display's
// third_person_animation, a TACZ ThirdPersonManager animation (an unknown
// name is the default one, as ThirdPersonManager.getAnimation).
type ThirdPersonHold uint8

const (
	HoldDefault ThirdPersonHold = iota // arms forward along the look, raised a little more while aiming
	HoldMinigun                        // body turned, the gun at the hip; aiming changes nothing
)

var thirdPersonHoldNames = []string{"default", "minigun"}

// ParseThirdPersonHold maps a display's third_person_animation name to its hold.
func ParseThirdPersonHold(name string) ThirdPersonHold {
	if v, err := enumParse(thirdPersonHoldNames, []byte(name)); err == nil {
		return ThirdPersonHold(v)
	}
	return HoldDefault
}

func (h ThirdPersonHold) MarshalText() ([]byte, error) {
	return enumText(thirdPersonHoldNames, uint8(h))
}
func (h *ThirdPersonHold) UnmarshalText(t []byte) error {
	v, err := enumParse(thirdPersonHoldNames, t)
	*h = ThirdPersonHold(v)
	return err
}
