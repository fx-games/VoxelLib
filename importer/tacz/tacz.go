// Package tacz loads TACZ gun packs in their own formats: Bedrock gun
// models and animations (played as authored by importer/bbmodel), gun and
// ammo data mapped to weapon.Gun, attachments (attach.go, modifier.go),
// slot icons and sounds. Lua state machines and third-person animations
// are not used.
package tacz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/importer/bbmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/weapon"
)

// Pack is a loaded gun pack. Guns and Ammo are sorted by ID; their Icon,
// Model and sound paths are files in FS.
type Pack struct {
	FS   fs.FS
	Guns []weapon.Gun
	Ammo []weapon.Ammo
	// Missing lists sound events the pack's clips name but no file provides
	// (TACZ plays nothing for them either).
	Missing []string

	guns     map[string]gunFiles
	attIndex map[string]attIndex    // by attachment ID
	tags     map[string][]string    // attachment tags by ID ("tacz:muzzle")
	adapters map[string]slotAdapter // by slot adapter ID
	defaults map[string][]*bbmodel.Animation
	internal fs.FS // the mod's own resources (DefaultsDir)
}

// shellFiles names a casing's geometry and texture (resource ids).
type shellFiles struct {
	Model, Texture string
}

type gunFiles struct {
	base                         weapon.Gun // the spec without attachments
	builtin                      map[Slot]builtinSlot
	allow                        []Slot                         // allow_attachment_types
	extMag                       []int                          // extended_mag_ammo_amount by level
	exclusive                    map[string]json.RawMessage     // exclusive_attachments: data by attachment ID
	melee                        taczGunMelee                   // the gun data's melee, before attachments
	adjust                       map[weapon.FireMode]modeAdjust // fire_mode_adjust
	shell                        shellFiles                     // the display's own casing, if any
	tracer                       string                         // the display's ammo tracer_color, if any
	texture, animation, defaults string
}

// The committed gun pack and the mod's own resources (default animations,
// tracer), relative to the repo root.
const (
	PackDir     = "assets/tacz/tacz_default_gun"
	DefaultsDir = "assets/tacz/defaults"
)

// LoadDefault loads the committed gun pack (PackDir, DefaultsDir).
func LoadDefault() (*Pack, error) { return Load(os.DirFS(PackDir), os.DirFS(DefaultsDir)) }

// Load reads the gun pack at pack (a tacz_default_gun directory) and the
// mod's own resources at defaults (<kind>_default.animation.json, the tracer).
func Load(pack, defaults fs.FS) (*Pack, error) {
	lang := map[string]string{}
	if err := readJSON(pack, "assets/tacz/lang/en_us.json", &lang); err != nil {
		return nil, err
	}
	p := &Pack{FS: pack, guns: map[string]gunFiles{}, defaults: map[string][]*bbmodel.Animation{}, internal: defaults}
	ammoShell := map[string]shellFiles{} // by ammo ID
	ammoTracer := map[string]string{}    // tracer_color by ammo ID
	for _, d := range []string{"rifle", "pistol"} {
		data, err := fs.ReadFile(defaults, d+"_default.animation.json")
		if err != nil {
			return nil, err
		}
		if p.defaults[d], err = bbmodel.ParseAnimationFile(data); err != nil {
			return nil, fmt.Errorf("%s_default: %w", d, err)
		}
	}
	missing := map[string]bool{}
	sound := func(id string) string {
		if id == "" {
			return ""
		}
		if _, err := fs.Stat(pack, SoundFile(id)); err != nil {
			missing[id] = true
			return ""
		}
		return id
	}

	if err := p.loadAttachmentIndex(lang, sound); err != nil {
		return nil, err
	}
	if err := p.loadSlotAdapters(lang); err != nil {
		return nil, err
	}
	idx, err := fs.Glob(pack, "data/tacz/index/guns/*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range idx {
		if err := p.loadGun(f, lang, sound); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	sort.Slice(p.Guns, func(i, j int) bool { return p.Guns[i].ID < p.Guns[j].ID })

	idx, err = fs.Glob(pack, "data/tacz/index/ammo/*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range idx {
		var in struct {
			Name    string `json:"name"`
			Display string `json:"display"`
			Stack   int    `json:"stack_size"`
		}
		if err := readJSON(pack, f, &in); err != nil {
			return nil, err
		}
		var disp struct {
			Slot   string `json:"slot"`
			Shell  shellFiles
			Tracer string `json:"tracer_color"`
		}
		if err := readJSON(pack, asset("display/ammo", in.Display)+".json", &disp); err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(path.Base(f), ".json")
		ammoShell[id], ammoTracer[id] = disp.Shell, disp.Tracer
		p.Ammo = append(p.Ammo, weapon.Ammo{ID: id, Name: lang[in.Name], Stack: in.Stack, Icon: asset("textures", disp.Slot) + ".png"})
	}
	// Tracer colour: the gun display's (GunDisplayInstance.checkGunAmmo),
	// else the ammo display's (ClientAmmoIndex), else white.
	for i := range p.Guns {
		g := &p.Guns[i]
		gf := p.guns[g.ID]
		text := gf.tracer
		if text == "" {
			text = ammoTracer[g.Ammo]
		}
		c, err := tracerColor(text)
		if err != nil {
			return nil, fmt.Errorf("%s tracer_color: %w", g.ID, err)
		}
		g.Bullet.TracerColor, gf.base.Bullet.TracerColor = c, c
		p.guns[g.ID] = gf
	}
	// A gun without its own casing throws its ammo's.
	for i := range p.Guns {
		g := &p.Guns[i]
		if g.Shell == nil {
			continue
		}
		f := p.guns[g.ID].shell
		if f.Model == "" || f.Texture == "" {
			f = ammoShell[g.Ammo]
		}
		if f.Model == "" || f.Texture == "" {
			g.Shell = nil // TACZ draws nothing without a model
			gf := p.guns[g.ID]
			gf.base.Shell = nil // Gun copies the spec from base (Shell is shared otherwise)
			p.guns[g.ID] = gf
			continue
		}
		g.Shell.Model, g.Shell.Texture = asset("geo_models", f.Model)+".json", asset("textures", f.Texture)+".png"
	}
	for id := range missing {
		p.Missing = append(p.Missing, id)
	}
	sort.Strings(p.Missing)
	return p, nil
}

// tracerColor parses a "#RRGGBB" tracer_color (ColorHex.colorTextToRbgFloatArray);
// "" is white.
func tracerColor(text string) ([3]float32, error) {
	if text == "" {
		return [3]float32{1, 1, 1}, nil
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(text, "#"), 16, 32)
	if err != nil || len(strings.TrimPrefix(text, "#")) != 6 {
		return [3]float32{}, fmt.Errorf("bad colour %q", text)
	}
	return [3]float32{float32(v>>16&0xFF) / 255, float32(v>>8&0xFF) / 255, float32(v&0xFF) / 255}, nil
}

// defaultSounds are GunDisplayInstance.checkSounds' defaults for sounds a
// display's sounds map leaves out.
var defaultSounds = map[weapon.GunSound]string{
	weapon.SoundDryFire:    "tacz:dry_fire",
	weapon.SoundFireSelect: "tacz:fire_select",
	weapon.SoundHeadHit:    "tacz:head_hit",
	weapon.SoundFleshHit:   "tacz:flesh_hit",
	weapon.SoundKill:       "tacz:kill",
}

// SoundFile is the pack file of a sound event ("tacz:ak47/ak47_shoot").
func SoundFile(event string) string {
	ns, p := split(event)
	return "assets/" + ns + "/tacz_sounds/" + p + ".ogg"
}

// Icon reads a gun's or ammo's slot icon.
func (p *Pack) Icon(file string) (image.Image, error) { return readPNG(p.FS, file) }

// Model parses a gun's model with its clips: the gun's animations, plus the
// default animation set's for clips the gun lacks. The bones TACZ hides on a
// gun without attachments are removed.
func (p *Pack) Model(id string, c Choice) (*model.Model, *Lens, error) {
	i := slices.IndexFunc(p.Guns, func(g weapon.Gun) bool { return g.ID == id })
	if i < 0 {
		return nil, nil, fmt.Errorf("tacz: no gun %q", id)
	}
	f := p.guns[id]
	atts, err := p.shown(id, c)
	if err != nil {
		return nil, nil, err
	}
	tex, err := readPNG(p.FS, f.texture)
	if err != nil {
		return nil, nil, err
	}
	geo, err := fs.ReadFile(p.FS, p.Guns[i].Model)
	if err != nil {
		return nil, nil, err
	}
	models, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", id, err)
	}
	m := models[0]
	m.Name = id
	anims, err := p.animations(f)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", id, err)
	}
	for _, a := range anims {
		if _, dup := m.Clips[a.Name]; dup {
			continue // the gun's own clip wins over the default one
		}
		if c, ok := a.Bind(m); ok {
			m.Clips[a.Name] = c
		}
	}
	stripHidden(m, atts)
	if err := p.graftAttachments(m, atts); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", id, err)
	}
	var lens *Lens
	if a, ok := atts[SlotScope]; ok && a.model != "" && m.Bone(string(SlotScope)+"_pos") >= 0 {
		if lens, err = splitLens(m, a); err != nil {
			return nil, nil, err
		}
	}
	return m, lens, nil
}

// ShellModel parses a gun's casing (Gun.Shell must be set).
func (p *Pack) ShellModel(g *weapon.Gun) (*model.Model, error) {
	tex, err := readPNG(p.FS, g.Shell.Texture)
	if err != nil {
		return nil, err
	}
	geo, err := fs.ReadFile(p.FS, g.Shell.Model)
	if err != nil {
		return nil, err
	}
	models, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return nil, fmt.Errorf("%s: %w", g.Shell.Model, err)
	}
	return models[0], nil
}

// TracerModel parses the mod's tracer (InternalAssetLoader's
// DEFAULT_BULLET_MODEL basic_bullet.json with DEFAULT_BULLET_TEXTURE).
func (p *Pack) TracerModel() (*model.Model, error) {
	tex, err := readPNG(p.internal, "basic_bullet.png")
	if err != nil {
		return nil, err
	}
	geo, err := fs.ReadFile(p.internal, "basic_bullet.json")
	if err != nil {
		return nil, err
	}
	models, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return nil, fmt.Errorf("basic_bullet: %w", err)
	}
	return models[0], nil
}

// animations returns the gun's animations followed by its default set's.
func (p *Pack) animations(f gunFiles) ([]*bbmodel.Animation, error) {
	data, err := fs.ReadFile(p.FS, f.animation)
	if err != nil {
		return nil, err
	}
	own, err := bbmodel.ParseAnimationFile(data)
	if err != nil {
		return nil, err
	}
	return append(own, p.defaults[f.defaults]...), nil
}

func (p *Pack) loadGun(file string, lang map[string]string, sound func(string) string) error {
	var index struct {
		Name    string `json:"name"`
		Display string `json:"display"`
		Data    string `json:"data"`
		Type    string `json:"type"`
	}
	if err := readJSON(p.FS, file, &index); err != nil {
		return err
	}
	var disp struct {
		Model, Texture, Slot, Animation string
		Hud                             string
		HudEmpty                        string   `json:"hud_empty"`
		Default                         string   `json:"use_default_animation"`
		Zoom                            *float32 `json:"iron_zoom"`
		StateMachine                    string   `json:"state_machine"`
		ThirdPersonAnimation            string   `json:"third_person_animation"`
		ModelFOV                        *float32 `json:"model_fov"`
		ZoomModelFOV                    *float32 `json:"zoom_model_fov"`
		Transform                       struct {
			Scale struct {
				ThirdPerson *[3]float32 `json:"thirdperson"`
			}
		}
		Sounds map[string]json.RawMessage
		Ammo   struct {
			Tracer string `json:"tracer_color"`
		}
		Muzzle json.RawMessage `json:"muzzle_flash"`
		Shell  *struct {
			shellFiles
			Velocity [3]float32 `json:"initial_velocity"`
			Random   [3]float32 `json:"random_velocity"`
			Accel    [3]float32 `json:"acceleration"`
			Angular  [3]float32 `json:"angular_velocity"`
			Life     float32    `json:"living_time"`
		}
	}
	if err := readJSON(p.FS, asset("display/guns", index.Display)+".json", &disp); err != nil {
		return err
	}
	data := newTaczData()
	if err := readJSON(p.FS, dataPath("guns", index.Data), &data); err != nil {
		return err
	}
	id := strings.TrimSuffix(path.Base(file), ".json")
	spec, err := data.spec(id, lang[index.Name], index.Type)
	if err != nil {
		return err
	}
	files := gunFiles{texture: asset("textures", disp.Texture) + ".png", animation: asset("animations", disp.Animation) + ".animation.json", defaults: disp.Default}
	anims, err := p.animations(files)
	if err != nil {
		return err
	}
	clips := map[string]bool{}
	for _, a := range anims {
		clips[a.Name] = true
	}
	if clips[weapon.ClipReloadIntro] && clips[weapon.ClipReloadLoop] {
		spec.Reload.Kind = weapon.Tube
	}
	raw, err := fs.ReadFile(p.FS, files.animation)
	if err != nil {
		return err
	}
	cues, err := rawCues(raw)
	if err != nil {
		return err
	}
	spec.Cues = map[string][]weapon.Cue{}
	for _, a := range cues {
		for _, c := range a.cues {
			if ev := sound(c.Sound); ev != "" {
				spec.Cues[a.clip] = append(spec.Cues[a.clip], weapon.Cue{T: c.T, Sound: ev})
			}
		}
	}
	spec.Sounds = map[weapon.GunSound]string{}
	for _, name := range weapon.GunSounds {
		id, err := soundRef(disp.Sounds[string(name)])
		if err != nil {
			return fmt.Errorf("sound %s: %w", name, err)
		}
		if id == "" && len(disp.Sounds) > 0 { // checkSounds adds defaults to a sounds map only
			id = defaultSounds[name]
		}
		if ev := sound(id); ev != "" {
			spec.Sounds[name] = ev
		}
	}
	// GunDisplay's defaults where absent.
	spec.Zoom = orDefault(disp.Zoom, gunIronZoom)
	spec.Spin = disp.StateMachine == minigunStateMachine
	spec.ModelFOV, spec.AimModelFOV = orDefault(disp.ModelFOV, gunModelFOV), orDefault(disp.ZoomModelFOV, gunModelFOV)
	spec.ThirdPersonHold = weapon.ParseThirdPersonHold(disp.ThirdPersonAnimation)
	spec.ThirdPersonScale = 1
	if s := disp.Transform.Scale.ThirdPerson; s != nil {
		if s[0] != s[1] || s[1] != s[2] {
			return fmt.Errorf("%s: non-uniform thirdperson scale %v unsupported", file, *s)
		}
		spec.ThirdPersonScale = s[0]
	}
	spec.Model, spec.Icon = asset("geo_models", disp.Model)+".json", asset("textures", disp.Slot)+".png"
	spec.Hud = asset("textures", disp.Hud) + ".png"
	if sh := disp.Shell; sh != nil {
		spec.Shell = &weapon.ShellEjection{Velocity: sh.Velocity, Random: sh.Random, Accel: sh.Accel, Angular: sh.Angular, Life: sh.Life}
		files.shell = sh.shellFiles
	}
	if disp.HudEmpty != "" {
		spec.HudEmpty = asset("textures", disp.HudEmpty) + ".png"
	}
	if spec.Muzzle, err = muzzleFlash(disp.Muzzle); err != nil {
		return fmt.Errorf("%s: muzzle_flash: %w", id, err)
	}
	files.tracer = disp.Ammo.Tracer
	files.base, files.allow, files.extMag, files.exclusive, files.melee, files.adjust = spec, data.Allow, data.ExtMag, data.Exclusive, data.Melee, data.Adjust
	files.builtin = map[Slot]builtinSlot{}
	for slot, raw := range data.Builtin {
		if files.builtin[slot], err = parseBuiltin(raw); err != nil {
			return fmt.Errorf("%s: builtin_attachments %s: %w", id, slot, err)
		}
	}
	p.guns[id] = files
	if spec, err = p.Gun(id, p.DefaultChoice(id), spec.FireModes()[0]); err != nil {
		return err
	}
	p.Guns = append(p.Guns, spec)
	return nil
}

// GunDisplay's defaults.
const (
	gunIronZoom = 1.2
	// minigunStateMachine is the script that spins the barrel.
	minigunStateMachine = "tacz:minigun_state_machine"
	gunModelFOV         = 70
)

// orDefault is *v, or def when the key was absent.
func orDefault(v *float32, def float32) float32 {
	if v == nil {
		return def
	}
	return *v
}

// taczData is the part of a TACZ gun data file we convert.
type taczData struct {
	Builtin   map[Slot]json.RawMessage       `json:"builtin_attachments"`
	Allow     []Slot                         `json:"allow_attachment_types"`
	ExtMag    []int                          `json:"extended_mag_ammo_amount"`
	Exclusive map[string]json.RawMessage     `json:"exclusive_attachments"`
	Ammo      string                         `json:"ammo"`
	Mag       int                            `json:"ammo_amount"`
	RPM       float32                        `json:"rpm"`
	Bolt      string                         `json:"bolt"`
	Modes     []string                       `json:"fire_mode"`
	Adjust    map[weapon.FireMode]modeAdjust `json:"fire_mode_adjust"`
	Burst     struct {
		Count int     `json:"count"`
		BPM   float32 `json:"bpm"`
	} `json:"burst_data"`
	Bullet struct {
		Damage         float32 `json:"damage"`
		Amount         int     `json:"bullet_amount"`
		Speed          float32 `json:"speed"`
		Gravity        float32 `json:"gravity"`
		Friction       float32 `json:"friction"`
		Life           float32 `json:"life"`
		Pierce         int     `json:"pierce"`
		TracerInterval int     `json:"tracer_count_interval"`
		// Knockback is BulletData's knockback (default 0: TACZ bullets
		// replace the vanilla push with it).
		Knockback float32 `json:"knockback"`
		Extra     struct {
			Headshot float32 `json:"head_shot_multiplier"`
			Adjust   []struct {
				Distance json.RawMessage `json:"distance"` // a number or "infinite"
				Damage   float32         `json:"damage"`
			} `json:"damage_adjust"`
		} `json:"extra_damage"`
	} `json:"bullet"`
	Reload struct {
		Type     string
		Feed     struct{ Empty, Tactical float32 }
		Cooldown struct{ Empty, Tactical float32 }
	}
	Draw      float32          `json:"draw_time"`
	PutAway   float32          `json:"put_away_time"`
	Aim       float32          `json:"aim_time"`
	Weight    float32          `json:"weight"`
	MoveSpeed weapon.MoveSpeed `json:"movement_speed"`
	Recoil    struct {
		Pitch, Yaw []struct {
			Time  float32
			Value [2]float32
		}
	}
	Inaccuracy  struct{ Stand, Move, Sneak, Aim float32 }
	Heat        *taczHeat
	Melee       taczGunMelee
	CanSlide    *bool  `json:"can_slide"` // GunData's default: true
	Script      string `json:"script"`
	AllowUnload bool   `json:"allow_ammo_unload"`
}

// taczHeat is a gun's "heat" block, GunHeatData's defaults where absent.
type taczHeat struct {
	Max      float32 `json:"max"`
	PerShot  float32 `json:"per_shot"`
	Cooling  float32 `json:"cooling_multiplier"`
	Delay    float32 `json:"cooling_delay"`  // ms
	Lock     float32 `json:"over_heat_time"` // ms
	MinInacc float32 `json:"min_inaccuracy"`
	MaxInacc float32 `json:"max_inaccuracy"`
	MinRPM   float32 `json:"min_rpm_mod"`
	MaxRPM   float32 `json:"max_rpm_mod"`
}

func (h *taczHeat) UnmarshalJSON(b []byte) error {
	type plain taczHeat
	p := plain{Max: 100, PerShot: 3, Cooling: 1, Delay: 1000, Lock: 3000, MinInacc: 1, MaxInacc: 1, MinRPM: 1, MaxRPM: 1}
	err := json.Unmarshal(b, &p)
	*h = taczHeat(p)
	return err
}

// BulletData's defaults.
const (
	bulletDamage   = 5
	bulletSpeed    = 5
	bulletFriction = 0.01
	bulletLife     = 10 // seconds
)

// newTaczData is a gun data holding BulletData's defaults, to read a file over.
func newTaczData() taczData {
	var d taczData
	b := &d.Bullet
	b.Damage, b.Amount, b.Speed, b.Friction, b.Life, b.Pierce, b.TracerInterval = bulletDamage, 1, bulletSpeed, bulletFriction, bulletLife, 1, weapon.NoTracer
	// ExtraDamage's default. ponytail: a gun without extra_damage has
	// HeadShotModifier's 0 in TACZ (a head hit deals nothing); every pack gun has one.
	b.Extra.Headshot = 1
	return d
}

func (d *taczData) spec(id, name, class string) (weapon.Gun, error) {
	g := weapon.Gun{ID: id, Name: name, Class: class, Ammo: strings.TrimPrefix(d.Ammo, "tacz:"), Mag: d.Mag, RPM: d.RPM}
	switch d.Bolt {
	case "open_bolt":
		g.Bolt = weapon.OpenBolt
	case "manual_action":
		g.Bolt = weapon.ManualAction
	}
	for _, m := range d.Modes {
		var fm weapon.FireMode
		if fm.UnmarshalText([]byte(m)) == nil {
			g.Modes = append(g.Modes, fm)
		}
	}
	g.Burst.Count, g.Burst.BPM = d.Burst.Count, d.Burst.BPM
	g.Slide = d.CanSlide == nil || *d.CanSlide
	b := &g.Bullet
	b.Damage, b.Pellets, b.Speed, b.Gravity, b.Pierce = d.Bullet.Damage, max(d.Bullet.Amount, 1), d.Bullet.Speed, d.Bullet.Gravity, d.Bullet.Pierce
	b.Friction, b.Life, b.TracerInterval = d.Bullet.Friction, d.Bullet.Life, d.Bullet.TracerInterval
	b.Headshot, b.Knockback = d.Bullet.Extra.Headshot, d.Bullet.Knockback
	for _, a := range d.Bullet.Extra.Adjust {
		dist := float32(math.MaxFloat32) // DistanceDamagePairSerializer: "infinite"
		if string(a.Distance) != `"infinite"` {
			if err := json.Unmarshal(a.Distance, &dist); err != nil {
				return g, fmt.Errorf("damage_adjust distance %s: %w", a.Distance, err)
			}
		}
		b.Falloff = append(b.Falloff, weapon.Falloff{Dist: dist, Damage: a.Damage})
	}
	if len(b.Falloff) == 0 { // DamageModifier: the one pair (Integer.MAX_VALUE, damage)
		b.Falloff = []weapon.Falloff{{Dist: noAdjustRange, Damage: b.Damage}}
	}
	r := &g.Reload
	if d.Reload.Type == "inventory" {
		r.Kind = weapon.Inventory
	}
	// canUnloadAmmo (the pack's scripted guns all allow it, so no script's unload_ammo is consulted)
	g.Unload = r.Kind != weapon.Inventory && (d.Script == "" || d.AllowUnload)
	r.FeedEmpty, r.FeedTactical = d.Reload.Feed.Empty, d.Reload.Feed.Tactical
	r.DoneEmpty, r.DoneTactical = d.Reload.Cooldown.Empty, d.Reload.Cooldown.Tactical
	g.Draw, g.PutAway, g.Aim = d.Draw, d.PutAway, d.Aim
	g.Weight, g.MoveSpeed = d.Weight, d.MoveSpeed
	for _, k := range d.Recoil.Pitch {
		g.Recoil.Pitch = append(g.Recoil.Pitch, weapon.RecoilKey{T: k.Time, Min: k.Value[0], Max: k.Value[1]})
	}
	for _, k := range d.Recoil.Yaw {
		g.Recoil.Yaw = append(g.Recoil.Yaw, weapon.RecoilKey{T: k.Time, Min: k.Value[0], Max: k.Value[1]})
	}
	in := d.Inaccuracy
	g.Inaccuracy.Stand, g.Inaccuracy.Move, g.Inaccuracy.Sneak, g.Inaccuracy.Aim = in.Stand, in.Move, in.Sneak, in.Aim
	if h := d.Heat; h != nil {
		g.Heat = &weapon.Heat{Max: h.Max, PerShot: h.PerShot, Cooling: h.Cooling, Delay: h.Delay / 1000, Lock: h.Lock / 1000,
			RPM: [2]float32{h.MinRPM, h.MaxRPM}, Inaccuracy: [2]float32{h.MinInacc, h.MaxInacc}}
	}
	return g, nil
}

type clipCues struct {
	clip string
	cues []weapon.Cue
}

// rawCues reads each clip's sound_effects (time → {effect}) in time order.
func rawCues(data []byte) ([]clipCues, error) {
	var f struct {
		Animations map[string]struct {
			Sounds map[string]struct {
				Effect string `json:"effect"`
			} `json:"sound_effects"`
		} `json:"animations"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	var out []clipCues
	for name, a := range f.Animations {
		c := clipCues{clip: name}
		for t, s := range a.Sounds {
			var at float32
			if _, err := fmt.Sscan(t, &at); err != nil {
				return nil, fmt.Errorf("%s: sound time %q: %w", name, t, err)
			}
			c.cues = append(c.cues, weapon.Cue{T: at, Sound: s.Effect})
		}
		sort.Slice(c.cues, func(i, j int) bool { return c.cues[i].T < c.cues[j].T })
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].clip < out[j].clip })
	return out, nil
}

// soundRef reads a display sound: an id string or {"id": ...} (absent: "").
func soundRef(raw json.RawMessage) (string, error) {
	if raw == nil {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var o struct{ ID string }
	err := json.Unmarshal(raw, &o)
	return o.ID, err
}

// stripHidden removes the triangles (and vertices) of bones hidden with the
// attachments shown and their descendants, and of attachment_adapter's
// children but the adapters of those attachments.
func stripHidden(m *model.Model, atts map[Slot]attachment) {
	adapters := map[string]bool{}
	for _, a := range atts {
		adapters[a.adapter] = a.adapter != ""
	}
	hide := make([]bool, len(m.Bones))
	for i, b := range m.Bones {
		hide[i] = hiddenBone(b.Name, atts) || b.Parent >= 0 && (hide[b.Parent] || m.Bones[b.Parent].Name == "attachment_adapter" && !adapters[b.Name])
	}
	stripBones(m, hide)
}

// stripBones removes the triangles (and vertices) bound to the bones marked.
func stripBones(m *model.Model, hide []bool) {
	for mi := range m.Meshes {
		me := &m.Meshes[mi]
		var verts []model.Vertex
		var idx []uint32
		remap := map[uint32]uint32{}
		for t := 0; t+2 < len(me.Indices); t += 3 {
			if hide[me.Verts[me.Indices[t]].Joints[0]] {
				continue
			}
			for _, v := range me.Indices[t : t+3] {
				nv, ok := remap[v]
				if !ok {
					nv = uint32(len(verts))
					remap[v] = nv
					verts = append(verts, me.Verts[v])
				}
				idx = append(idx, nv)
			}
		}
		me.Verts, me.Indices = verts, idx
	}
}

// asset turns "tacz:gun/ak47_geo" into "assets/tacz/<dir>/gun/ak47_geo".
func asset(dir, id string) string {
	ns, p := split(id)
	return "assets/" + ns + "/" + dir + "/" + p
}

// dataPath turns "tacz:ak47_data" into "data/tacz/data/<kind>/ak47_data.json".
func dataPath(kind, id string) string {
	ns, p := split(id)
	return "data/" + ns + "/data/" + kind + "/" + p + ".json"
}

func split(id string) (ns, p string) {
	ns, p, ok := strings.Cut(id, ":")
	if !ok {
		return "tacz", id
	}
	return ns, p
}

func readPNG(fsys fs.FS, name string) (image.Image, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(data))
}

// readJSON reads a TACZ JSON file, which may hold // and /* */ comments and
// trailing commas.
func readJSON(fsys fs.FS, name string, v any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(Clean(data), v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Clean removes comments and trailing commas outside strings.
func Clean(src []byte) []byte {
	var out []byte
	inStr := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case inStr:
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				i++
				out = append(out, src[i])
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			i--
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := bytes.Index(src[i+2:], []byte("*/"))
			if end < 0 {
				i = len(src)
			} else {
				i += end + 3
			}
		case c == '}' || c == ']':
			t := len(out) - 1
			for t >= 0 && (out[t] == ' ' || out[t] == '\t' || out[t] == '\n' || out[t] == '\r') {
				t--
			}
			if t >= 0 && out[t] == ',' {
				out = append(out[:t], out[t+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// Default first-person particle ids (MuzzleFlash.DEFAULT_FIRST_PERSON_*).
const (
	defaultFlashParticle = "tacz:muzzle_flash"
	defaultSmokeParticle = "tacz:muzzle_flash_smoke"
)

// muzzleFlash reads a display's muzzle_flash block as TACZ's MuzzleFlash
// does (absent fields keep its defaults, a null particle id disables that
// layer); nil unless its type is "renew".
func muzzleFlash(raw json.RawMessage) (*weapon.MuzzleFlash, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	flash, smoke := defaultFlashParticle, defaultSmokeParticle
	m := struct {
		Type             string          `json:"type"`
		Scale            float32         `json:"scale"`
		FlashLight       int             `json:"flash_light_level"`
		Flash            *string         `json:"first_person_flash"`
		Smoke            *string         `json:"first_person_smoke"`
		SpeedScale       float32         `json:"initial_speed_scale"`
		SmokeCount       float32         `json:"smoke_particle_count"`
		SmokeScale       float32         `json:"smoke_scale"`
		SmokeDirection   json.RawMessage `json:"smoke_direction"`
		SmokeDirections  json.RawMessage `json:"smoke_directions"`
		SmokeSpreadAngle *float32        `json:"smoke_spread_angle"`
		SmokeSpread      *float32        `json:"smoke_spread"`
	}{Scale: 1, FlashLight: 15, Flash: &flash, Smoke: &smoke, SpeedScale: 1}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m.Type != "renew" {
		return nil, nil
	}
	file := func(id *string) string {
		if id == nil {
			return ""
		}
		return asset("particle_definitions", *id) + ".particle.json"
	}
	return &weapon.MuzzleFlash{Flash: file(m.Flash), Smoke: file(m.Smoke), Scale: m.Scale,
		FlashLight: min(15, max(0, m.FlashLight)), SpeedScale: m.SpeedScale, SmokeCount: m.SmokeCount,
		SmokeScale: max(0, m.SmokeScale),
		Direction:  smokeDirection(m.SmokeDirections, m.SmokeSpreadAngle, m.SmokeDirection, m.SmokeSpread)}, nil
}

// Smoke direction defaults (SmokeDirectionData).
const (
	defaultSmokeSpread      = 0.35
	defaultSmokeSpreadAngle = 20.0
)

// smokeDirection is SmokeDirectionData.parse: angle directions win over
// vectors; anything malformed is nil (even scatter).
func smokeDirection(angles json.RawMessage, spreadAngle *float32, vectors json.RawMessage, spread *float32) *weapon.SmokeDirection {
	if dirs := smokeAngles(angles); dirs != nil {
		s := float32(defaultSmokeSpreadAngle)
		if spreadAngle != nil && *spreadAngle >= 0 && *spreadAngle <= 180 {
			s = *spreadAngle
		}
		return &weapon.SmokeDirection{Dirs: dirs, Spread: s, AngleMode: true}
	}
	if dirs := smokeVectors(vectors); dirs != nil {
		s := float32(defaultSmokeSpread)
		if spread != nil && *spread >= 0 {
			s = *spread
		}
		return &weapon.SmokeDirection{Dirs: dirs, Spread: s}
	}
	return nil
}

// smokeAngles is SmokeDirectionData.parseAngles: one {plane_angle,
// out_angle} object or a list of them, in degrees.
func smokeAngles(raw json.RawMessage) [][3]float32 {
	type angle struct {
		Plane *float64 `json:"plane_angle"`
		Out   *float64 `json:"out_angle"`
	}
	var list []angle
	if json.Unmarshal(raw, &list) != nil {
		var one angle
		if json.Unmarshal(raw, &one) != nil {
			return nil
		}
		list = []angle{one}
	}
	if len(list) == 0 {
		return nil
	}
	var dirs [][3]float32
	for _, a := range list[:min(len(list), weapon.MaxSmokeDirections)] {
		if a.Plane == nil || a.Out == nil || *a.Plane < -180 || *a.Plane > 180 || *a.Out < -90 || *a.Out > 90 {
			return nil
		}
		p, o := *a.Plane*math.Pi/180, *a.Out*math.Pi/180
		dirs = append(dirs, [3]float32{float32(math.Cos(o) * math.Cos(p)), float32(math.Cos(o) * math.Sin(p)), float32(-math.Sin(o))})
	}
	return dirs
}

// smokeVectors is SmokeDirectionData.parseDirections: [x, y, z] or a list
// of them, normalised.
func smokeVectors(raw json.RawMessage) [][3]float32 {
	var list [][3]float64
	if json.Unmarshal(raw, &list) != nil {
		var one [3]float64
		if json.Unmarshal(raw, &one) != nil {
			return nil
		}
		list = [][3]float64{one}
	}
	if len(list) == 0 {
		return nil
	}
	var dirs [][3]float32
	for _, d := range list[:min(len(list), weapon.MaxSmokeDirections)] {
		l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
		if l <= 1e-6 {
			return nil
		}
		dirs = append(dirs, [3]float32{float32(d[0] / l), float32(d[1] / l), float32(d[2] / l)})
	}
	return dirs
}
