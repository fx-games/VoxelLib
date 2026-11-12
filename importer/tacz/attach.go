package tacz

import (
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"maps"
	"math"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/importer/bbmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/weapon"
)

// Attachments, as TACZ's refit screen installs them: a gun allows the
// attachments its allow_attachments tag list expands to (tacz_tags), of the
// types in its allow_attachment_types. A Choice names the attachment
// installed in each slot and the slot adapter (riser) selected for it; the
// installed attachments' data (modifier.go: extended magazine level,
// silence, melee and the modifiers) changes the gun's spec. A slot without
// one shows its builtin display attachment (data not applied). The default
// choice is builtin_attachments' default_attached. As BedrockGunModel and
// AttachmentRender draw them: an adapter child of attachment_adapter, the
// slot adapter's model in the <type>_pos bone frame and the attachment's
// model above it by the adapter's mount_offset (grafted into the gun model,
// so they animate with the gun), and the bones that depend on the slots
// shown or hidden. Attachment variants are not used (the pack has none).

// Slot is an attachment type (TACZ AttachmentType, lower case as in the files).
type Slot string

const (
	SlotScope       Slot = "scope"
	SlotMuzzle      Slot = "muzzle"
	SlotStock       Slot = "stock"
	SlotGrip        Slot = "grip"
	SlotLaser       Slot = "laser"
	SlotExtendedMag Slot = "extended_mag"
)

// Slots lists the slot types in refit screen order.
var Slots = []Slot{SlotScope, SlotMuzzle, SlotStock, SlotGrip, SlotLaser, SlotExtendedMag}

// Choice is what a gun is refitted with; a slot left out (or "") has none.
type Choice struct {
	Attachments map[Slot]string // installed attachment ID by slot
	// Adapters is the slot adapter ID selected by slot (SlotAdapterHelper's
	// SlotAdapters tag): its allowed_attachments replace the gun's for the slot.
	Adapters map[Slot]string
}

// Clone is a copy of c that shares no maps with it.
func (c Choice) Clone() Choice {
	return Choice{Attachments: maps.Clone(c.Attachments), Adapters: maps.Clone(c.Adapters)}
}

// Attach installs attachment id ("": none) in slot s.
func (c *Choice) Attach(s Slot, id string) {
	if c.Attachments == nil {
		c.Attachments = map[Slot]string{}
	}
	c.Attachments[s] = id
}

// Adapt selects slot adapter id ("": none) for slot s.
func (c *Choice) Adapt(s Slot, id string) {
	if c.Adapters == nil {
		c.Adapters = map[Slot]string{}
	}
	c.Adapters[s] = id
}

// Key is the choice as a string, equal for equal choices (empty slots
// ignored): a map key for things built per choice.
func (c Choice) Key() string {
	var b strings.Builder
	for _, s := range Slots {
		if c.Attachments[s] != "" || c.Adapters[s] != "" {
			fmt.Fprintf(&b, "%s=%s+%s;", s, c.Adapters[s], c.Attachments[s])
		}
	}
	return b.String()
}

// RefitSound names an attachment display sound (SoundManager's keys).
type RefitSound string

const (
	SoundInstall   RefitSound = "install"
	SoundUninstall RefitSound = "uninstall"
)

var refitSounds = []RefitSound{SoundInstall, SoundUninstall}

// attachmentFOV is AttachmentDisplay's default fov.
const attachmentFOV = 70

// maxMagLevel caps extended_mag_level (AttachmentDataUtils.getMagExtendLevel).
const maxMagLevel = 3

// ironView is the gun's own aiming bone (FirstPersonRenderGunEvent's iron sight path).
const ironView = "iron_view"

// tagPrefix marks a tag reference in a tag list.
const tagPrefix = "#"

// allowTag is the tag listing what gun id allows (AttachmentsTagManager).
func allowTag(gun string) string { return "tacz:allow_attachments/" + gun }

// attIndex is an attachment's index entry with its display sounds.
type attIndex struct {
	name, display, data string
	slot                Slot
	sounds              map[RefitSound]string
}

// builtinSlot is a builtin_attachments entry (BuiltInAttachmentSerializer).
type builtinSlot struct {
	installed string // default_attached: installed on a new gun
	display   string // display_attachment: shown while none is installed
}

// pixelsPerBlock turns Blockbench pixels into blocks.
const pixelsPerBlock = 16

// attachment is a resolved attachment display, with its data's effects
// when installed.
type attachment struct {
	id         string
	model      string // geometry file ("" for none: an OEM part drawn by its adapter)
	texture    string
	adapter    string // attachment_adapter child shown
	showMount  bool
	showMuzzle bool
	zoom       float32 // scope slot: first magnification (1 for none)
	fov        float32 // scope slot: the hand's field of view while aiming (views_fov[0], else fov)
	view       int     // scopes: scope_view index (0 = "scope_view", n = "scope_view_n")
	idleAdjust float32 // scopes: idle view raised by this many pixels
	// magLevel: extended magazines (shown also on a display-only one, as DisplayGunModelInstance)
	magLevel int
	silence  *weapon.Silence // installed only
	// flashOffset: the display's muzzle_flash_offset (Blockbench pixels, 0 when negative or not finite)
	flashOffset float32
	melee       *taczMelee // installed only: the attachment's melee (bayonets, stocks); nil: none
	// scope, sight: the display's lens kinds (BedrockAttachmentModel isScope/isSight).
	scope, sight bool
	mods         attMods      // installed only
	slotAdapter  *slotAdapter // the slot adapter it sits on (SlotAdapterHelper.getEffectiveSlotAdapter); nil: none
}

// slotAdapter is a slot adapter (CommonSlotAdapterIndex, ClientSlotAdapterIndex).
type slotAdapter struct {
	name           string
	slot           Slot
	allowed        []string // allowed_attachments: attachment IDs and #tags
	sort           int
	model, texture string  // "" for none
	lift           float32 // mount_offset's y (pixels): the attachment sits this much higher
}

// Slot adapter sort bounds (CommonSlotAdapterIndex.checkIndex).
const maxAdapterSort = 65536

// adapterTagPrefix marks the allow-list tags that list slot adapters
// (SlotAdapterHelper.ADAPTER_TAG_PATH_PREFIX).
const adapterTagPrefix = "adapter/"

// adapterBone prefixes a grafted slot adapter's bones (after the slot type).
const adapterBone = "adapter/"

// loadSlotAdapters reads every slot adapter's index and display.
func (p *Pack) loadSlotAdapters(lang map[string]string) error {
	p.adapters = map[string]slotAdapter{}
	files, err := fs.Glob(p.FS, "data/*/index/slot_adapters/*.json")
	if err != nil {
		return err
	}
	for _, f := range files {
		var in struct {
			Name, Display, Data string
			Slot                Slot
			Allowed             []string `json:"allowed_attachments"`
			Sort                int
		}
		if err := readJSON(p.FS, f, &in); err != nil {
			return err
		}
		if !slices.Contains(Slots, in.Slot) {
			return fmt.Errorf("%s: slot %q", f, in.Slot)
		}
		if _, err := fs.Stat(p.FS, dataPath("slot_adapters", in.Data)); err != nil { // checkData: required, its content unused (SlotAdapterData has no fields)
			return fmt.Errorf("%s: %w", f, err)
		}
		var disp struct {
			Model, Texture string
			Offset         [3]float32 `json:"mount_offset"`
		}
		dispFile := asset("display/slot_adapters", in.Display) + ".json"
		if err := readJSON(p.FS, dispFile, &disp); err != nil {
			return err
		}
		if disp.Offset[0] != 0 || disp.Offset[2] != 0 {
			return fmt.Errorf("%s: mount_offset %v: only a vertical offset is supported", dispFile, disp.Offset)
		}
		a := slotAdapter{name: lang[in.Name], slot: in.Slot, allowed: in.Allowed, sort: min(max(in.Sort, 0), maxAdapterSort), lift: disp.Offset[1]}
		if disp.Model != "" && disp.Texture != "" {
			a.model, a.texture = asset("geo_models", disp.Model)+".json", asset("textures", disp.Texture)+".png"
		}
		ns := strings.Split(f, "/")[1]
		p.adapters[ns+":"+strings.TrimSuffix(path.Base(f), ".json")] = a
	}
	return nil
}

// SlotAdapters lists the slot adapters a gun takes in a slot, by sort then
// ID (SlotAdapterHelper.getAvailableSlotAdapters: the gun's allow-list tags
// under adapter/, expanded through nested tags).
func (p *Pack) SlotAdapters(gun string, s Slot) []string {
	if f, ok := p.guns[gun]; !ok || !slices.Contains(f.allow, s) {
		return nil
	}
	var roots []string
	for _, e := range p.tags[allowTag(gun)] {
		if tag, ok := strings.CutPrefix(e, tagPrefix); ok {
			if _, path := split(tag); strings.HasPrefix(path, adapterTagPrefix) {
				roots = append(roots, e)
			}
		}
	}
	ids := map[string]bool{}
	p.expandTag(roots, ids, map[string]bool{})
	var out []string
	for id := range ids {
		if a, ok := p.adapters[id]; ok && a.slot == s {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, func(a, b string) int {
		if d := p.adapters[a].sort - p.adapters[b].sort; d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return out
}

// SlotAdapterName is a slot adapter's display name.
func (p *Pack) SlotAdapterName(id string) string { return p.adapters[id].name }

// adapterAllows is CommonSlotAdapterIndex.allowsAttachment.
func (p *Pack) adapterAllows(adapter, att string) bool {
	ids := map[string]bool{}
	p.expandTag(p.adapters[adapter].allowed, ids, map[string]bool{})
	return ids[att]
}

// parseBuiltin reads a builtin_attachments entry.
func parseBuiltin(raw json.RawMessage) (builtinSlot, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return builtinSlot{display: s}, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return builtinSlot{}, nil // candidates only
	}
	var o struct {
		Default  string `json:"default_attached"`
		Default2 string `json:"default"`
		Display  string `json:"display_attachment"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return builtinSlot{}, err
	}
	b := builtinSlot{installed: o.Default, display: o.Display}
	if b.installed == "" {
		b.installed = o.Default2
	}
	return b, nil
}

// loadAttachmentIndex reads every attachment's index and display sounds, and
// the attachment tags.
func (p *Pack) loadAttachmentIndex(lang map[string]string, sound func(string) string) error {
	p.attIndex, p.tags = map[string]attIndex{}, map[string][]string{}
	files, err := fs.Glob(p.FS, "data/*/index/attachments/*.json")
	if err != nil {
		return err
	}
	for _, f := range files {
		var in struct{ Name, Display, Data, Type string }
		if err := readJSON(p.FS, f, &in); err != nil {
			return err
		}
		var disp struct{ Sounds map[string]json.RawMessage }
		if err := readJSON(p.FS, asset("display/attachments", in.Display)+".json", &disp); err != nil {
			return err
		}
		a := attIndex{name: lang[in.Name], display: in.Display, data: in.Data, slot: Slot(in.Type), sounds: map[RefitSound]string{}}
		for _, rs := range refitSounds {
			id, err := soundRef(disp.Sounds[string(rs)])
			if err != nil {
				return fmt.Errorf("%s: sound %s: %w", f, rs, err)
			}
			if ev := sound(id); ev != "" {
				a.sounds[rs] = ev
			}
		}
		ns := strings.Split(f, "/")[1]
		p.attIndex[ns+":"+strings.TrimSuffix(path.Base(f), ".json")] = a
	}
	return fs.WalkDir(p.FS, "data", func(f string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		parts := strings.SplitN(f, "/", 5) // data/<ns>/tacz_tags/attachments/<tag>.json
		if d.IsDir() || len(parts) < 5 || parts[2] != "tacz_tags" || parts[3] != "attachments" {
			return nil
		}
		var list []string
		if err := readJSON(p.FS, f, &list); err != nil {
			return err
		}
		p.tags[parts[1]+":"+strings.TrimSuffix(parts[4], ".json")] = list
		return nil
	})
}

// expandTag is AllowAttachmentTagMatcher.expand: the attachment IDs a tag
// list names, directly or through nested tags (an absent tag is empty).
func (p *Pack) expandTag(list []string, out, visited map[string]bool) {
	for _, e := range list {
		if tag, ok := strings.CutPrefix(e, tagPrefix); ok {
			if !visited[tag] {
				visited[tag] = true
				p.expandTag(p.tags[tag], out, visited)
			}
			continue
		}
		out[e] = true
	}
}

// AttachmentSlots lists the slots of a gun that take attachments, in Slots order.
func (p *Pack) AttachmentSlots(gun string) []Slot {
	var out []Slot
	for _, s := range Slots {
		if len(p.Allowed(gun, s, "")) > 0 || len(p.SlotAdapters(gun, s)) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// Allowed lists the attachment IDs a gun takes in a slot with slot adapter
// adapter ("": none), sorted (SlotAdapterHelper.allowsAttachment: the
// adapter's allowed_attachments, else the gun's; none when
// allow_attachment_types leaves the slot out or the gun doesn't take the adapter).
func (p *Pack) Allowed(gun string, s Slot, adapter string) []string {
	f, ok := p.guns[gun]
	if !ok || !slices.Contains(f.allow, s) {
		return nil
	}
	ids := map[string]bool{}
	if adapter == "" {
		p.expandTag(p.tags[allowTag(gun)], ids, map[string]bool{})
	} else if slices.Contains(p.SlotAdapters(gun, s), adapter) {
		p.expandTag(p.adapters[adapter].allowed, ids, map[string]bool{})
	}
	var out []string
	for id := range ids {
		if a, ok := p.attIndex[id]; ok && a.slot == s {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// DefaultChoice is the attachments a new gun comes with (builtin_attachments' default_attached).
func (p *Pack) DefaultChoice(gun string) Choice {
	c := Choice{}
	for s, b := range p.guns[gun].builtin {
		if b.installed != "" {
			c.Attach(s, b.installed)
		}
	}
	return c
}

// AttachmentName is an attachment's display name.
func (p *Pack) AttachmentName(id string) string { return p.attIndex[id].name }

// AttachmentSound is an attachment's install or uninstall sound event ("": none).
func (p *Pack) AttachmentSound(id string, s RefitSound) string { return p.attIndex[id].sounds[s] }

// shown resolves the attachments a gun shows for a choice by slot: the
// installed ones, else the builtin display attachments.
func (p *Pack) shown(gun string, c Choice) (map[Slot]attachment, error) {
	f, ok := p.guns[gun]
	if !ok {
		return nil, fmt.Errorf("tacz: no gun %q", gun)
	}
	for s, id := range c.Adapters {
		if id != "" && !slices.Contains(p.SlotAdapters(gun, s), id) {
			return nil, fmt.Errorf("%s: %s slot does not take slot adapter %q", gun, s, id)
		}
	}
	for s, id := range c.Attachments {
		if id != "" && !slices.Contains(p.Allowed(gun, s, c.Adapters[s]), id) {
			return nil, fmt.Errorf("%s: %s slot does not take %q", gun, s, id)
		}
	}
	out := map[Slot]attachment{}
	for _, s := range Slots {
		id, installed := c.Attachments[s], true
		if id == "" {
			id, installed = f.builtin[s].display, false
		}
		if id == "" {
			continue
		}
		if t := p.attIndex[id].slot; t != s {
			return nil, fmt.Errorf("%s: %s attachment %q is a %q", gun, s, id, t)
		}
		a, err := p.attachment(id, f.exclusive[id], installed)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", gun, s, err)
		}
		if ad := c.Adapters[s]; ad != "" && p.adapterAllows(ad, id) {
			sa := p.adapters[ad]
			a.slotAdapter = &sa
		}
		out[s] = a
	}
	return out, nil
}

// attachment resolves an attachment's display, and its data (the gun's
// exclusive_attachments entry when it has one) for an installed one.
func (p *Pack) attachment(id string, exclusive json.RawMessage, installed bool) (attachment, error) {
	index, ok := p.attIndex[id]
	if !ok {
		return attachment{}, fmt.Errorf("no attachment %q", id)
	}
	var disp struct {
		Model, Texture string
		Adapter        string
		ShowMount      *bool     `json:"show_mount"`
		ShowMuzzle     bool      `json:"show_muzzle"`
		Zoom           []float32 `json:"zoom"`
		Views          []int     `json:"views"`
		IdleAdjust     float32   `json:"idle_scope_height_adjust"`
		Scope, Sight   bool
		Fov            *float32  `json:"fov"`
		ViewsFov       []float32 `json:"views_fov"`
		FlashOffset    float32   `json:"muzzle_flash_offset"`
	}
	if err := readJSON(p.FS, asset("display/attachments", index.display)+".json", &disp); err != nil {
		return attachment{}, err
	}
	var data struct {
		MagLevel int        `json:"extended_mag_level"`
		Melee    *taczMelee `json:"melee"`
		Silence  *struct {
			DistanceAddend int `json:"distance_addend"` // the deprecated form
			Distance       *struct {
				Addend                        int
				Percent, Multiplier, Function json.RawMessage
			}
			UseSilenceSound bool `json:"use_silence_sound"`
		}
	}
	raw, src := []byte(exclusive), "exclusive_attachments "+id
	if exclusive == nil {
		src = dataPath("attachments", index.data)
		b, err := fs.ReadFile(p.FS, src)
		if err != nil {
			return attachment{}, err
		}
		raw = Clean(b)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return attachment{}, fmt.Errorf("%s: %w", src, err)
	}
	a := attachment{id: id, adapter: disp.Adapter, showMount: disp.ShowMount == nil || *disp.ShowMount,
		showMuzzle: disp.ShowMuzzle, idleAdjust: disp.IdleAdjust, magLevel: min(max(data.MagLevel, 0), maxMagLevel),
		scope: disp.Scope, sight: disp.Sight}
	if f := float64(disp.FlashOffset); f > 0 && !math.IsInf(f, 0) { // AttachmentDisplay's validation
		a.flashOffset = disp.FlashOffset
	}
	if installed {
		a.melee = data.Melee
		var err error
		if a.mods, err = parseMods(raw); err != nil {
			return attachment{}, fmt.Errorf("%s: %w", src, err)
		}
	}
	if sl := data.Silence; sl != nil && installed {
		a.silence = &weapon.Silence{DistanceAddend: sl.DistanceAddend, UseSilenceSound: sl.UseSilenceSound}
		if d := sl.Distance; d != nil {
			if d.Percent != nil || d.Multiplier != nil || d.Function != nil {
				return attachment{}, fmt.Errorf("%s: silence distance: only an addend is supported", id)
			}
			a.silence.DistanceAddend = d.Addend
		}
	}
	if disp.Model != "" {
		a.model, a.texture = asset("geo_models", disp.Model)+".json", asset("textures", disp.Texture)+".png"
	}
	a.zoom, a.fov = 1, attachmentFOV // AttachmentDisplay defaults; getAimingZoom: no zoom list is 1×
	if len(disp.Zoom) > 0 {
		a.zoom = disp.Zoom[0]
	}
	switch {
	case len(disp.ViewsFov) > 0:
		a.fov = disp.ViewsFov[0]
	case disp.Fov != nil:
		a.fov = *disp.Fov
	}
	if len(disp.Views) > 0 {
		a.view = disp.Views[0] - 1
	}
	return a, nil
}

// Gun is a gun's spec with a choice of attachments: the shown scope's zoom,
// hand FOV and view bone, the installed extended magazine's capacity
// (extended_mag_ammo_amount), the installed muzzle's silence, the melee
// (gunMelee: the installed muzzle's, else stock's, else the gun's) and the
// installed attachments' modifiers (applyMods) after fire mode mode's
// fire_mode_adjust (one of the gun's FireModes: TACZ refreshes the cache
// with the item's fire mode). Pack.Guns holds each gun's with its
// DefaultChoice and first fire mode.
func (p *Pack) Gun(id string, c Choice, mode weapon.FireMode) (weapon.Gun, error) {
	atts, err := p.shown(id, c)
	if err != nil {
		return weapon.Gun{}, err
	}
	f := p.guns[id]
	if !slices.Contains(f.base.FireModes(), mode) {
		return weapon.Gun{}, fmt.Errorf("%s: no fire mode %d", id, mode)
	}
	spec := f.base
	spec.Sight = ironView
	if sc, ok := atts[SlotScope]; ok { // IGun.getAimingZoom, CameraSetupEvent.applyGunModelFovModifying
		spec.Zoom, spec.AimModelFOV = sc.zoom, sc.fov
		if sc.model != "" {
			if spec.Sight, err = p.scopeView(sc); err != nil {
				return weapon.Gun{}, fmt.Errorf("%s: %w", id, err)
			}
			spec.IdleRaise = sc.idleAdjust / pixelsPerBlock
		}
	}
	if tm := spec.Reload.Tube; tm != nil && tm.Clip != nil && c.Attachments[SlotScope] != "" {
		t := *tm // kar98_gun_logic: no stripper clip past a scope
		t.Clip = nil
		spec.Reload.Tube = &t
	}
	if c.Attachments[SlotExtendedMag] != "" { // AttachmentDataUtils.getAmmoCountWithAttachment
		if l := atts[SlotExtendedMag].magLevel; l > 0 && len(f.extMag) >= l {
			spec.Mag = f.extMag[l-1]
		}
		if l := atts[SlotExtendedMag].magLevel; l > 0 && len(spec.Reload.XMag) >= l { // xmag_reload_logic
			spec.Reload.MagTiming = spec.Reload.XMag[l-1]
		}
	}
	if c.Attachments[SlotMuzzle] != "" {
		spec.Silence = atts[SlotMuzzle].silence
		spec.FlashOffset = atts[SlotMuzzle].flashOffset / pixelsPerBlock
	}
	spec.Melee = gunMelee(f.melee, atts)
	var mods []attMods // AttachmentDataUtils.getAllAttachmentData: the installed ones, by slot
	for _, s := range Slots {
		if c.Attachments[s] != "" {
			mods = append(mods, atts[s].mods)
		}
	}
	if err := applyMods(&spec, f.adjust[mode], mods); err != nil {
		return weapon.Gun{}, fmt.Errorf("%s: %w", id, err)
	}
	return spec, nil
}

// scopeView is the grafted bone a scope aims through: scope_view_n for the
// view chosen, scope_view for the first (also when it has fewer views).
func (p *Pack) scopeView(a attachment) (string, error) {
	if a.view > 0 {
		var geo struct {
			Geometry []struct {
				Bones []struct{ Name string }
			} `json:"minecraft:geometry"`
		}
		if err := readJSON(p.FS, a.model, &geo); err != nil {
			return "", err
		}
		want := "scope_view_" + strconv.Itoa(a.view)
		for _, g := range geo.Geometry {
			for _, b := range g.Bones {
				if b.Name == want {
					return graftName(SlotScope, want), nil
				}
			}
		}
	}
	return graftName(SlotScope, "scope_view"), nil
}

// graftName is a grafted attachment bone's name: the slot type and its own
// name, so it never shadows a gun bone.
func graftName(slot Slot, bone string) string { return string(slot) + "/" + bone }

// graftAttachments parses each shown attachment's model and grafts it on
// its slot's <type>_pos bone (after the gun's clips are bound: they never
// address grafted bones): first its slot adapter's model, if any, then the
// attachment raised by the adapter's mount_offset (AttachmentRender).
func (p *Pack) graftAttachments(m *model.Model, atts map[Slot]attachment) error {
	for slot, a := range atts {
		pos := m.Bone(string(slot) + "_pos")
		if pos < 0 {
			continue // the gun has no place for it (TACZ draws nothing either)
		}
		var lift float32
		if sa := a.slotAdapter; sa != nil {
			lift = sa.lift / 16
			if sa.model != "" {
				if err := p.graft(m, pos, sa.model, sa.texture, slot, adapterBone, 0); err != nil {
					return err
				}
			}
		}
		if a.model != "" {
			if err := p.graft(m, pos, a.model, a.texture, slot, "", lift); err != nil {
				return err
			}
		}
	}
	return nil
}

// graft parses a geometry file and grafts it on bone pos of m, raised by
// lift blocks, its bones named graftName(slot, prefix+name).
func (p *Pack) graft(m *model.Model, pos int, geoFile, texFile string, slot Slot, prefix string, lift float32) error {
	tex, err := readPNG(p.FS, texFile)
	if err != nil {
		return err
	}
	geo, err := fs.ReadFile(p.FS, geoFile)
	if err != nil {
		return err
	}
	subs, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return fmt.Errorf("%s: %w", geoFile, err)
	}
	sub := subs[0]
	for i := range sub.Bones {
		b := &sub.Bones[i]
		b.Name = graftName(slot, prefix+b.Name)
		if b.Parent < 0 {
			b.T[1] += lift
		}
	}
	for mi := range sub.Meshes {
		for vi := range sub.Meshes[mi].Verts {
			sub.Meshes[mi].Verts[vi].Pos[1] += lift
		}
	}
	return m.Graft(pos, sub)
}

// hiddenBone reports whether BedrockGunModel's functional renderers hide a
// bone given the attachments shown.
func hiddenBone(name string, atts map[Slot]attachment) bool {
	scope, hasScope := atts[SlotScope]
	_, laser := atts[SlotLaser]
	_, grip := atts[SlotGrip]
	switch name {
	case "lefthand_pos", "righthand_pos", "muzzle_flash": // drawn by code
		return true
	case "mount":
		return !hasScope || !scope.showMount
	case "carry", "sight":
		return hasScope
	case "sight_folded":
		return !hasScope
	case "mag_standard":
		return atts[SlotExtendedMag].magLevel != 0
	case "mag_extended_1", "mag_extended_2", "mag_extended_3":
		return atts[SlotExtendedMag].magLevel != int(name[len(name)-1]-'0')
	case "handguard_tactical":
		return !laser && !grip
	case "handguard_default":
		return laser || grip
	}
	if slot, ok := strings.CutSuffix(name, "_default"); ok && slices.Contains(Slots, Slot(slot)) {
		if a, ok := atts[Slot(slot)]; ok {
			return Slot(slot) != SlotMuzzle || !a.showMuzzle
		}
		return false
	}
	return strings.HasSuffix(name, "_pos") // attachment positions
}

// Lens is a scope's parts that TACZ draws with stencil passes
// (BedrockAttachmentModel renderScope/renderSight) instead of with the rest:
// each part is a model on the gun's skeleton (the gun's Skin poses it), and
// the gun model leaves them out. Nil parts are absent.
type Lens struct {
	Scope, Sight bool // the display's lens kinds; neither: the parts are drawn plainly
	Ring, Body   *model.Model
	Oculars      []Ocular       // by their number
	Divisions    []*model.Model // division, division_2, …: the reticle of the ocular with the same index
}

// Ocular is a lens opening: Part fills it, Bone (in the gun model) is its centre.
type Ocular struct {
	Part  *model.Model
	Bone  int
	Scope bool // ocular_scope: the magnified view's opening (ocular, ocular_sight: the sight's)
}

var ocularName = regexp.MustCompile(`^(ocular|ocular_sight|ocular_scope)(?:_(\d+))?$`)

// splitLens moves the scope's lens parts out of m.
func splitLens(m *model.Model, a attachment) (*Lens, error) {
	l := &Lens{Scope: a.scope, Sight: a.sight}
	cut := make([]bool, len(m.Bones))
	part := func(i int) *model.Model {
		keep := make([]bool, len(m.Bones))
		for _, j := range m.Descendants(i) {
			cut[j], keep[j] = true, true
		}
		for j := range keep {
			keep[j] = !keep[j]
		}
		p := *m
		p.Meshes = slices.Clone(m.Meshes)
		stripBones(&p, keep)
		return &p
	}
	named := func(name string) *model.Model {
		if i := m.Bone(graftName(SlotScope, name)); i >= 0 {
			return part(i)
		}
		return nil
	}
	l.Ring, l.Body = named("ocular_ring"), named("scope_body")
	byNum := map[int]Ocular{}
	for i, b := range m.Bones {
		name, ok := strings.CutPrefix(b.Name, graftName(SlotScope, ""))
		mt := ocularName.FindStringSubmatch(name)
		if !ok || mt == nil {
			continue
		}
		n := 1
		if mt[2] != "" {
			n, _ = strconv.Atoi(mt[2])
		}
		byNum[n] = Ocular{Part: part(i), Bone: i, Scope: mt[1] == "ocular_scope"}
	}
	for _, n := range slices.Sorted(maps.Keys(byNum)) {
		l.Oculars = append(l.Oculars, byNum[n])
	}
	for name, n := "division", 2; ; name, n = "division_"+strconv.Itoa(n), n+1 {
		d := named(name)
		if d == nil {
			break
		}
		l.Divisions = append(l.Divisions, d)
	}
	if len(l.Oculars) > 127 {
		return nil, fmt.Errorf("%s: %d oculars (at most 127)", a.id, len(l.Oculars))
	}
	stripBones(m, cut)
	return l, nil
}
