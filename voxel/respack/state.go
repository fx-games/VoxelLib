package respack

import (
	"encoding/json"
	"errors"
	"path"
	"strings"

	"github.com/memmaker/voxellib/voxel"
)

// Resolver returns the voxel.ModelResolver that reads blockstates and
// models from the pack: every state's geometry, from its variant or the
// multipart parts its properties select.
func (p *Pack) Resolver() voxel.ModelResolver { return p.resolveState }

// resolveState turns a state into its voxel.Model: a full cube or a cross
// plant as such, any other model as its elements (voxel boxes) turned by the
// blockstate's x/y rotation (with uvlock). A state the pack has no geometry
// for (builtin/entity models) takes entityPack's, or nothing when listed in
// emptyStates.
func (p *Pack) resolveState(name string, props map[string]string) (voxel.Model, bool) {
	if m, ok := p.packState(name, props); ok || p == entityPack {
		return m, ok
	}
	if m, ok := entityPack.packState(name, props); ok {
		return m, true
	}
	if drawsNothing(name, props) {
		return voxel.Model{Shape: voxel.ShapeNone}, true
	}
	return voxel.Model{}, false
}

// packState is resolveState from this pack's own files alone.
func (p *Pack) packState(name string, props map[string]string) (voxel.Model, bool) {
	ns, rel := splitID(name)
	data, err := p.readFile(path.Join("assets", ns, "blockstates", rel+".json"))
	if err != nil {
		return voxel.Model{}, false
	}
	var bs blockstate
	if json.Unmarshal(data, &bs) != nil {
		return voxel.Model{}, false
	}
	var vs []variant
	switch {
	case len(bs.Variants) > 0:
		v, err := pickVariant(&bs, props)
		if err != nil {
			return voxel.Model{}, false
		}
		vs = []variant{*v}
	case len(bs.Multipart) > 0:
		if vs, err = pickParts(bs.Multipart, props); err != nil {
			return voxel.Model{}, false
		}
	default:
		return voxel.Model{}, false
	}
	if len(vs) == 1 {
		if m, ok := p.simpleModel(vs[0]); ok {
			return m, true
		}
	}
	m := voxel.Model{Shape: voxel.ShapeBoxes}
	for _, v := range vs {
		bs, err := p.ModelBoxes(v.Model, nil)
		if err != nil || len(bs) == 0 && len(vs) == 1 { // builtin/entity (chests, pots): no geometry here
			return voxel.Model{}, false
		}
		turn := voxel.RotateBoxes
		if v.UVLock {
			turn = voxel.LockBoxes
		}
		bs = turn(turn(bs, 0, float32(v.X)), 1, float32(v.Y))
		m.Boxes = append(m.Boxes, bs...)
		if m.Tex[0] == "" {
			m.Tex = all(p.particle(v.Model, bs))
		}
	}
	return m, true
}

// simpleModel is a variant drawn as a cross plant or a full cube (the
// first full-cube element: vanilla grass_block adds a tinted overlay cube
// the base cube stands for), with its face rules; ok=false for anything else.
func (p *Pack) simpleModel(v variant) (voxel.Model, bool) {
	tex, elems, chain, err := p.resolve(v.Model)
	if err != nil {
		return voxel.Model{}, false
	}
	for _, c := range chain {
		if c == "block/cross" || c == "block/tinted_cross" {
			return voxel.Model{Shape: voxel.ShapeCross, Tex: all(lookupTex(tex, "#cross"))}, true
		}
	}
	if len(elems) == 0 {
		return voxel.Model{}, false
	}
	for _, e := range elems {
		if e.From != [3]float32{0, 0, 0} || e.To != [3]float32{16, 16, 16} {
			return voxel.Model{}, false
		}
	}
	e := elems[0]
	var t [6]string
	var rules voxel.FaceRules
	for f := range rules.Cull {
		rules.Cull[f] = voxel.FaceCount
	}
	for fn, face := range e.Faces {
		f, ok := voxel.FaceByName(fn)
		if !ok {
			continue
		}
		t[f] = lookupTex(tex, faceRef(face.Texture))
		if c, ok := voxel.FaceByName(face.Cullface); ok {
			rules.Cull[f] = c
		}
		if face.TintIndex != nil && *face.TintIndex >= 0 {
			rules.Tinted |= 1 << f
		}
	}
	return voxel.Model{Shape: voxel.ShapeCube, Tex: rotateFaces(t, v.X, v.Y), Rules: rotateRules(rules, v.X, v.Y)}, true
}

// particle is a model's "particle" texture, else the first face texture
// of its boxes: what stands for the block as a whole (icons, particles).
func (p *Pack) particle(id string, bs []voxel.Box) string {
	if tex, _, _, err := p.resolve(id); err == nil {
		if t := lookupTex(tex, "#particle"); t != "" {
			return t
		}
	}
	for _, b := range bs {
		for f, t := range b.Tex {
			if t != "" && b.Skip&(1<<f) == 0 {
				return t
			}
		}
	}
	return ""
}

// part is one multipart entry: apply when the condition holds.
type part struct {
	When  json.RawMessage `json:"when"`
	Apply json.RawMessage `json:"apply"`
}

// pickParts is the variants of every multipart entry whose "when" holds
// for props (all of them for an entry without one).
func pickParts(raw json.RawMessage, props map[string]string) ([]variant, error) {
	var parts []part
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	var out []variant
	for _, pt := range parts {
		ok, err := holds(pt.When, props)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		v, err := firstVariant(pt.Apply)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("no multipart entry applies")
	}
	return out, nil
}

// holds evaluates a multipart "when": {"k": "a|b", ...} (every key
// matches one of its values), {"OR": [...]} or {"AND": [...]}; absent holds.
func holds(raw json.RawMessage, props map[string]string) (bool, error) {
	if len(raw) == 0 {
		return true, nil
	}
	var c map[string]json.RawMessage
	if err := json.Unmarshal(raw, &c); err != nil {
		return false, err
	}
	for k, v := range c {
		switch k {
		case "OR", "AND":
			var subs []json.RawMessage
			if err := json.Unmarshal(v, &subs); err != nil {
				return false, err
			}
			any := false
			for _, s := range subs {
				ok, err := holds(s, props)
				if err != nil {
					return false, err
				}
				if k == "AND" && !ok {
					return false, nil
				}
				any = any || ok
			}
			if k == "OR" && !any {
				return false, nil
			}
		default:
			var want any
			if err := json.Unmarshal(v, &want); err != nil {
				return false, err
			}
			if !valueMatches(props[k], want) {
				return false, nil
			}
		}
	}
	return true, nil
}

// valueMatches is a "when" value test: "a|b" alternatives, or a JSON bool.
func valueMatches(have string, want any) bool {
	switch w := want.(type) {
	case bool:
		return have == map[bool]string{true: "true", false: "false"}[w]
	case string:
		for _, alt := range strings.Split(w, "|") {
			if alt == have {
				return true
			}
		}
	}
	return false
}

// firstVariant is a variant, or the first of a weighted list.
func firstVariant(raw json.RawMessage) (variant, error) {
	var v variant
	if len(raw) > 0 && raw[0] == '[' {
		var vs []variant
		if err := json.Unmarshal(raw, &vs); err != nil || len(vs) == 0 {
			return v, errors.New("bad variant list")
		}
		return vs[0], nil
	}
	return v, json.Unmarshal(raw, &v)
}
