package tacz

import (
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/importer/bbmodel"
	"github.com/memmaker/voxellib/model"
)

// Attachments: a gun shows the attachment each slot comes with
// (builtin_attachments: default_attached, else display_attachment; there
// is no refit screen, so nothing else is ever installed). As BedrockGunModel
// draws it: an adapter child of attachment_adapter, the attachment's model in
// the <type>_pos bone frame (grafted into the gun model, so it animates with
// the gun), and the bones that depend on the slots shown or hidden.

// Slot types (TACZ AttachmentType, lower case as in the files).
const (
	slotScope       = "scope"
	slotMuzzle      = "muzzle"
	slotStock       = "stock"
	slotGrip        = "grip"
	slotLaser       = "laser"
	slotExtendedMag = "extended_mag"
)

// attachmentFOV is AttachmentDisplay's default fov.
const attachmentFOV = 70

// ironView is the gun's own aiming bone (FirstPersonRenderGunEvent's iron sight path).
const ironView = "iron_view"

var slots = []string{slotScope, slotMuzzle, slotStock, slotGrip, slotLaser, slotExtendedMag}

// attachment is a resolved attachment display.
type attachment struct {
	id         string
	model      string // geometry file ("" for none: an OEM part drawn by its adapter)
	texture    string
	adapter    string // attachment_adapter child shown
	showMount  bool
	showMuzzle bool
	zoom       float32    // scope slot: first magnification (1 for none)
	fov        float32    // scope slot: the hand's field of view while aiming (views_fov[0], else fov)
	view       int        // scopes: scope_view index (0 = "scope_view", n = "scope_view_n")
	idleAdjust float32    // scopes: idle view raised by this many pixels
	magLevel   int        // extended magazines
	melee      *taczMelee // the attachment's melee (bayonets, stocks); nil: none
	// scope, sight: the display's lens kinds (BedrockAttachmentModel isScope/isSight).
	scope, sight bool
}

// shownAttachment picks the attachment a builtin_attachments entry shows on a
// new gun ("" for none), as BuiltInAttachmentSerializer and getBuiltinAttachment.
func shownAttachment(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return "", nil // candidates only
	}
	var o struct {
		Default  string `json:"default_attached"`
		Default2 string `json:"default"`
		Display  string `json:"display_attachment"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return "", err
	}
	switch {
	case o.Default != "":
		return o.Default, nil
	case o.Default2 != "":
		return o.Default2, nil
	}
	return o.Display, nil
}

// loadAttachments resolves a gun's builtin_attachments by slot type.
func (p *Pack) loadAttachments(builtin map[string]json.RawMessage) (map[string]attachment, error) {
	out := map[string]attachment{}
	for slot, raw := range builtin {
		id, err := shownAttachment(raw)
		if err != nil {
			return nil, fmt.Errorf("builtin_attachments %s: %w", slot, err)
		}
		if id == "" {
			continue
		}
		ns, name := split(id)
		var index struct {
			Display, Data, Type string
		}
		if err := readJSON(p.FS, "data/"+ns+"/index/attachments/"+name+".json", &index); err != nil {
			return nil, err
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
		}
		if err := readJSON(p.FS, asset("display/attachments", index.Display)+".json", &disp); err != nil {
			return nil, err
		}
		var data struct {
			MagLevel int        `json:"extended_mag_level"`
			Melee    *taczMelee `json:"melee"`
		}
		if err := readJSON(p.FS, dataPath("attachments", index.Data), &data); err != nil {
			return nil, err
		}
		a := attachment{id: id, adapter: disp.Adapter, showMount: disp.ShowMount == nil || *disp.ShowMount,
			showMuzzle: disp.ShowMuzzle, idleAdjust: disp.IdleAdjust, magLevel: data.MagLevel, melee: data.Melee,
			scope: disp.Scope, sight: disp.Sight}
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
		out[index.Type] = a
	}
	return out, nil
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
					return graftName(slotScope, want), nil
				}
			}
		}
	}
	return graftName(slotScope, "scope_view"), nil
}

// graftName is a grafted attachment bone's name: the slot type and its own
// name, so it never shadows a gun bone.
func graftName(slot, bone string) string { return slot + "/" + bone }

// graftAttachments parses each shown attachment's model and grafts it on
// its slot's <type>_pos bone (after the gun's clips are bound: they never
// address grafted bones).
func (p *Pack) graftAttachments(m *model.Model, atts map[string]attachment) error {
	for slot, a := range atts {
		if a.model == "" {
			continue
		}
		pos := m.Bone(slot + "_pos")
		if pos < 0 {
			continue // the gun has no place for it (TACZ draws nothing either)
		}
		tex, err := readPNG(p.FS, a.texture)
		if err != nil {
			return err
		}
		geo, err := fs.ReadFile(p.FS, a.model)
		if err != nil {
			return err
		}
		subs, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
		if err != nil {
			return fmt.Errorf("%s: %w", a.model, err)
		}
		sub := subs[0]
		for i := range sub.Bones {
			sub.Bones[i].Name = graftName(slot, sub.Bones[i].Name)
		}
		if err := m.Graft(pos, sub); err != nil {
			return err
		}
	}
	return nil
}

// hiddenBone reports whether BedrockGunModel's functional renderers hide a
// bone given the attachments shown.
func hiddenBone(name string, atts map[string]attachment) bool {
	scope, hasScope := atts[slotScope]
	_, laser := atts[slotLaser]
	_, grip := atts[slotGrip]
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
		return atts[slotExtendedMag].magLevel != 0
	case "mag_extended_1", "mag_extended_2", "mag_extended_3":
		return atts[slotExtendedMag].magLevel != int(name[len(name)-1]-'0')
	case "handguard_tactical":
		return !laser && !grip
	case "handguard_default":
		return laser || grip
	}
	if slot, ok := strings.CutSuffix(name, "_default"); ok && slices.Contains(slots, slot) {
		if a, ok := atts[slot]; ok {
			return slot != slotMuzzle || !a.showMuzzle
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
		if i := m.Bone(graftName(slotScope, name)); i >= 0 {
			return part(i)
		}
		return nil
	}
	l.Ring, l.Body = named("ocular_ring"), named("scope_body")
	byNum := map[int]Ocular{}
	for i, b := range m.Bones {
		name, ok := strings.CutPrefix(b.Name, graftName(slotScope, ""))
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
