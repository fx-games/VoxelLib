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

// Falloff is the damage of a bullet up to a distance (the last entry has Dist 0: any distance).
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
		Damage   float32   `json:"damage"`
		Pellets  int       `json:"pellets"`
		Speed    float32   `json:"speed"`
		Gravity  float32   `json:"gravity"`
		Pierce   int       `json:"pierce"`
		Headshot float32   `json:"headshot"`
		Falloff  []Falloff `json:"falloff"`
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
	Zoom  float32 `json:"zoom"`  // iron sight magnification
	Model string  `json:"model"` // model file in the pack
	Icon  string  `json:"icon"`  // slot sprite file in the pack
	// HUD sprites (39x13) beside the ammo count; HudEmpty "" tints Hud red when empty.
	Hud      string           `json:"hud"`
	HudEmpty string           `json:"hud_empty"`
	Shoot    string           `json:"shoot"` // sound events
	Dry      string           `json:"dry"`
	Cues     map[string][]Cue `json:"cues"` // clip name → sounds
	// Muzzle is the display's renew muzzle flash; nil for the legacy one.
	Muzzle *MuzzleFlash `json:"muzzle,omitempty"`
	// Shell is the casing ejection; nil: the gun ejects none.
	Shell *ShellEjection `json:"shell,omitempty"`
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
