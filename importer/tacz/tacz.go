// Package tacz loads TACZ gun packs in their own formats: Bedrock gun
// models and animations (played as authored by importer/bbmodel), gun and
// ammo data mapped to weapon.Gun, slot icons and sounds. Lua state machines,
// attachments and third-person animations are not used.
package tacz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"path"
	"slices"
	"sort"
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
	defaults map[string][]*bbmodel.Animation
}

type gunFiles struct {
	texture, animation, defaults string
}

// Load reads the gun pack at pack (a tacz_default_gun directory) and the
// default rifle/pistol animations at defaults (<kind>_default.animation.json).
func Load(pack, defaults fs.FS) (*Pack, error) {
	lang := map[string]string{}
	if err := readJSON(pack, "assets/tacz/lang/en_us.json", &lang); err != nil {
		return nil, err
	}
	p := &Pack{FS: pack, guns: map[string]gunFiles{}, defaults: map[string][]*bbmodel.Animation{}}
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
			Slot string `json:"slot"`
		}
		if err := readJSON(pack, asset("display/ammo", in.Display)+".json", &disp); err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(path.Base(f), ".json")
		p.Ammo = append(p.Ammo, weapon.Ammo{ID: id, Name: lang[in.Name], Stack: in.Stack, Icon: asset("textures", disp.Slot) + ".png"})
	}
	for id := range missing {
		p.Missing = append(p.Missing, id)
	}
	sort.Strings(p.Missing)
	return p, nil
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
func (p *Pack) Model(id string) (*model.Model, error) {
	i := slices.IndexFunc(p.Guns, func(g weapon.Gun) bool { return g.ID == id })
	if i < 0 {
		return nil, fmt.Errorf("tacz: no gun %q", id)
	}
	f := p.guns[id]
	tex, err := readPNG(p.FS, f.texture)
	if err != nil {
		return nil, err
	}
	geo, err := fs.ReadFile(p.FS, p.Guns[i].Model)
	if err != nil {
		return nil, err
	}
	models, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	m := models[0]
	m.Name = id
	anims, err := p.animations(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	for _, a := range anims {
		if _, dup := m.Clips[a.Name]; dup {
			continue // the gun's own clip wins over the default one
		}
		if c, ok := a.Bind(m); ok {
			m.Clips[a.Name] = c
		}
	}
	stripHidden(m)
	return m, nil
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
		HudEmpty                        string  `json:"hud_empty"`
		Default                         string  `json:"use_default_animation"`
		Zoom                            float32 `json:"iron_zoom"`
		Sounds                          map[string]json.RawMessage
	}
	if err := readJSON(p.FS, asset("display/guns", index.Display)+".json", &disp); err != nil {
		return err
	}
	var data taczData
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
	shoot, err := soundRef(disp.Sounds["shoot"])
	if err != nil {
		return err
	}
	spec.Shoot, spec.Dry = sound(shoot), sound("tacz:dry_fire")
	spec.Zoom = disp.Zoom
	spec.Model, spec.Icon = asset("geo_models", disp.Model)+".json", asset("textures", disp.Slot)+".png"
	spec.Hud = asset("textures", disp.Hud) + ".png"
	if disp.HudEmpty != "" {
		spec.HudEmpty = asset("textures", disp.HudEmpty) + ".png"
	}
	p.Guns = append(p.Guns, spec)
	p.guns[id] = files
	return nil
}

// taczData is the part of a TACZ gun data file we convert.
type taczData struct {
	Ammo  string   `json:"ammo"`
	Mag   int      `json:"ammo_amount"`
	RPM   float32  `json:"rpm"`
	Bolt  string   `json:"bolt"`
	Modes []string `json:"fire_mode"`
	Burst struct {
		Count int     `json:"count"`
		BPM   float32 `json:"bpm"`
	} `json:"burst_data"`
	Bullet struct {
		Damage  float32 `json:"damage"`
		Amount  int     `json:"bullet_amount"`
		Speed   float32 `json:"speed"`
		Gravity float32 `json:"gravity"`
		Pierce  int     `json:"pierce"`
		Extra   struct {
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
	Draw    float32 `json:"draw_time"`
	PutAway float32 `json:"put_away_time"`
	Aim     float32 `json:"aim_time"`
	Recoil  struct {
		Pitch, Yaw []struct {
			Time  float32
			Value [2]float32
		}
	}
	Inaccuracy struct{ Stand, Move, Sneak, Aim float32 }
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
	b := &g.Bullet
	b.Damage, b.Pellets, b.Speed, b.Gravity, b.Pierce = d.Bullet.Damage, max(d.Bullet.Amount, 1), d.Bullet.Speed, d.Bullet.Gravity, d.Bullet.Pierce
	b.Headshot = d.Bullet.Extra.Headshot
	for _, a := range d.Bullet.Extra.Adjust {
		var dist float32 // "infinite" stays 0: any distance
		if string(a.Distance) != `"infinite"` {
			if err := json.Unmarshal(a.Distance, &dist); err != nil {
				return g, fmt.Errorf("damage_adjust distance %s: %w", a.Distance, err)
			}
		}
		b.Falloff = append(b.Falloff, weapon.Falloff{Dist: dist, Damage: a.Damage})
	}
	r := &g.Reload
	if d.Reload.Type == "inventory" {
		r.Kind = weapon.Inventory
	}
	r.FeedEmpty, r.FeedTactical = d.Reload.Feed.Empty, d.Reload.Feed.Tactical
	r.DoneEmpty, r.DoneTactical = d.Reload.Cooldown.Empty, d.Reload.Cooldown.Tactical
	g.Draw, g.PutAway, g.Aim = d.Draw, d.PutAway, d.Aim
	for _, k := range d.Recoil.Pitch {
		g.Recoil.Pitch = append(g.Recoil.Pitch, weapon.RecoilKey{T: k.Time, Min: k.Value[0], Max: k.Value[1]})
	}
	for _, k := range d.Recoil.Yaw {
		g.Recoil.Yaw = append(g.Recoil.Yaw, weapon.RecoilKey{T: k.Time, Min: k.Value[0], Max: k.Value[1]})
	}
	in := d.Inaccuracy
	g.Inaccuracy.Stand, g.Inaccuracy.Move, g.Inaccuracy.Sneak, g.Inaccuracy.Aim = in.Stand, in.Move, in.Sneak, in.Aim
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

// hiddenBone reports the bones TACZ's BedrockGunModel hides on a gun with
// no attachments fitted (the subtree goes with it).
func hiddenBone(name string) bool {
	switch name {
	case "mag_extended_1", "mag_extended_2", "mag_extended_3", "mount", "sight_folded",
		"handguard_tactical", "muzzle_flash", "lefthand_pos", "righthand_pos":
		return true
	}
	return strings.HasSuffix(name, "_pos")
}

// stripHidden removes the triangles (and vertices) of hidden bones and their descendants,
// and of attachment_adapter's children (no adapter is rendered without attachments).
func stripHidden(m *model.Model) {
	hide := make([]bool, len(m.Bones))
	for i, b := range m.Bones {
		hide[i] = hiddenBone(b.Name) || b.Parent >= 0 && (hide[b.Parent] || m.Bones[b.Parent].Name == "attachment_adapter")
	}
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
