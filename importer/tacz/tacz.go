// Package tacz converts the TACZ default gun pack (Bedrock gun models and
// animations, gun/ammo data, slot icons, sounds) into VoxelLib assets:
// glTF models with baked clips and a weapon.Pack. Lua state machines,
// attachments and third-person animations are not converted.
package tacz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/memmaker/voxellib/importer/bbmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/weapon"
)

// BakeFPS is the sampling rate of the baked clips.
const BakeFPS = 60

// Gun is one converted gun: its spec and its model (clips baked).
type Gun struct {
	Spec  weapon.Gun
	Model *model.Model
	Icon  image.Image
}

// Result is the converted pack. Sounds maps every referenced sound event
// ("tacz:ak47/ak47_shoot") to its .ogg path in the pack.
type Result struct {
	Guns      []Gun
	Ammo      []weapon.Ammo
	AmmoIcons map[string]image.Image
	Sounds    map[string]string
	// Missing lists sound events the pack's clips name but no file provides
	// (TACZ plays nothing for them either).
	Missing []string
}

// Import reads the gun pack at pack (the tacz_default_gun directory) and the
// mod's own assets at mod (assets/tacz: the default rifle/pistol animations).
func Import(pack, mod fs.FS) (*Result, error) {
	lang := map[string]string{}
	if err := readJSON(pack, "assets/tacz/lang/en_us.json", &lang); err != nil {
		return nil, err
	}
	defaults := map[string][]*bbmodel.Animation{}
	for _, d := range []string{"rifle", "pistol"} {
		data, err := fs.ReadFile(mod, "animations/"+d+"_default.animation.json")
		if err != nil {
			return nil, err
		}
		if defaults[d], err = bbmodel.ParseAnimationFile(data); err != nil {
			return nil, fmt.Errorf("%s_default: %w", d, err)
		}
	}
	r := &Result{AmmoIcons: map[string]image.Image{}, Sounds: map[string]string{}}
	missing := map[string]bool{}
	sound := func(id string) string {
		if id == "" {
			return ""
		}
		ns, p := split(id)
		file := "assets/" + ns + "/tacz_sounds/" + p + ".ogg"
		if _, err := fs.Stat(pack, file); err != nil {
			missing[id] = true
			return ""
		}
		ev := ns + ":" + p
		r.Sounds[ev] = file
		return ev
	}

	idx, err := fs.Glob(pack, "data/tacz/index/guns/*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range idx {
		g, err := importGun(pack, f, lang, defaults, sound)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		r.Guns = append(r.Guns, *g)
	}
	sort.Slice(r.Guns, func(i, j int) bool { return r.Guns[i].Spec.ID < r.Guns[j].Spec.ID })

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
		icon, err := readPNG(pack, asset("textures", disp.Slot)+".png")
		if err != nil {
			return nil, fmt.Errorf("ammo %s: %w", id, err)
		}
		r.AmmoIcons[id] = icon
		r.Ammo = append(r.Ammo, weapon.Ammo{ID: id, Name: lang[in.Name], Stack: in.Stack, Icon: "icons/ammo/" + id + ".png"})
	}
	for id := range missing {
		r.Missing = append(r.Missing, id)
	}
	sort.Strings(r.Missing)
	return r, nil
}

func importGun(pack fs.FS, file string, lang map[string]string, defaults map[string][]*bbmodel.Animation, sound func(string) string) (*Gun, error) {
	var index struct {
		Name    string `json:"name"`
		Display string `json:"display"`
		Data    string `json:"data"`
		Type    string `json:"type"`
	}
	if err := readJSON(pack, file, &index); err != nil {
		return nil, err
	}
	var disp struct {
		Model, Texture, Slot, Animation string
		Default                         string  `json:"use_default_animation"`
		Zoom                            float32 `json:"iron_zoom"`
		Sounds                          map[string]json.RawMessage
	}
	if err := readJSON(pack, asset("display/guns", index.Display)+".json", &disp); err != nil {
		return nil, err
	}
	var data taczData
	if err := readJSON(pack, dataPath("guns", index.Data), &data); err != nil {
		return nil, err
	}
	id := strings.TrimSuffix(path.Base(file), ".json")

	tex, err := readPNG(pack, asset("textures", disp.Texture)+".png")
	if err != nil {
		return nil, err
	}
	geo, err := fs.ReadFile(pack, asset("geo_models", disp.Model)+".json")
	if err != nil {
		return nil, err
	}
	models, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return nil, err
	}
	m := models[0]
	m.Name = id
	anims, err := fs.ReadFile(pack, asset("animations", disp.Animation)+".animation.json")
	if err != nil {
		return nil, err
	}
	clips, err := bbmodel.ParseAnimationFile(anims)
	if err != nil {
		return nil, err
	}
	spec, err := data.spec(id, lang[index.Name], index.Type)
	if err != nil {
		return nil, err
	}
	spec.Zoom = disp.Zoom
	spec.Cues = map[string][]weapon.Cue{}
	for _, a := range append(clips, defaults[disp.Default]...) {
		if _, dup := m.Clips[a.Name]; dup {
			continue // the gun's own clip wins over the default one
		}
		if c, ok := a.Bind(m); ok {
			m.Clips[a.Name] = c
		}
	}
	cues, err := rawCues(anims)
	if err != nil {
		return nil, err
	}
	for _, a := range cues {
		for _, c := range a.cues {
			if ev := sound(c.Sound); ev != "" {
				spec.Cues[a.clip] = append(spec.Cues[a.clip], weapon.Cue{T: c.T, Sound: ev})
			}
		}
	}
	if m.Clips["reload_intro"] != nil && m.Clips["reload_loop"] != nil {
		spec.Reload.Kind = weapon.Tube
	}
	shoot, err := soundRef(disp.Sounds["shoot"])
	if err != nil {
		return nil, err
	}
	spec.Shoot, spec.Dry = sound(shoot), sound("tacz:dry_fire")
	stripHidden(m)
	m.Bake(BakeFPS)
	icon, err := readPNG(pack, asset("textures", disp.Slot)+".png")
	if err != nil {
		return nil, err
	}
	spec.Model, spec.Icon = "models/"+id+".gltf", "icons/gun/"+id+".png"
	return &Gun{Spec: spec, Model: m, Icon: icon}, nil
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
