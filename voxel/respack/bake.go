package respack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Bake writes the pack's minecraft blockstates as they are and every block
// model they use flattened (textures resolved, nearest elements inline, no
// parent; cross plants marked "shape": "cross") under dst, laid out like a
// pack (dst/assets/minecraft/...). It is the import step that turns a
// client jar into voxel/shapes (tools/shapegen); nothing calls it at run
// time. It returns the number of blockstates written.
func (p *Pack) Bake(dst string) (int, error) {
	files := map[string]bool{}
	for _, src := range p.sources {
		fs, err := fs.Glob(src, "assets/minecraft/blockstates/*.json")
		if err != nil {
			return 0, err
		}
		for _, f := range fs {
			files[f] = true
		}
	}
	models := map[string]bool{}
	for f := range files {
		data, err := p.readFile(f)
		if err != nil {
			return 0, err
		}
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		var bs blockstate
		if err := json.Unmarshal(data, &bs); err != nil {
			return 0, fmt.Errorf("%s: %w", f, err)
		}
		ids, err := stateModels(&bs)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", f, err)
		}
		for _, id := range ids {
			models[id] = true
		}
		if err := writeFile(filepath.Join(dst, f), data); err != nil {
			return 0, err
		}
	}
	for id := range models {
		data, err := p.flatModel(id)
		if err != nil {
			return 0, fmt.Errorf("model %s: %w", id, err)
		}
		ns, rel := splitID(id)
		if err := writeFile(filepath.Join(dst, "assets", ns, "models", rel+".json"), data); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

// stateModels are the model ids a blockstate's variants and multipart
// entries name.
func stateModels(bs *blockstate) ([]string, error) {
	var raws []json.RawMessage
	for _, v := range bs.Variants {
		raws = append(raws, v)
	}
	if len(bs.Multipart) > 0 {
		var parts []part
		if err := json.Unmarshal(bs.Multipart, &parts); err != nil {
			return nil, err
		}
		for _, pt := range parts {
			raws = append(raws, pt.Apply)
		}
	}
	var ids []string
	for _, raw := range raws {
		var vs []variant
		if len(raw) > 0 && raw[0] == '[' {
			if err := json.Unmarshal(raw, &vs); err != nil {
				return nil, err
			}
		} else {
			var v variant
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			vs = []variant{v}
		}
		for _, v := range vs {
			ids = append(ids, v.Model)
		}
	}
	return ids, nil
}

// flatModel is model id as one parentless model (see Bake).
func (p *Pack) flatModel(id string) ([]byte, error) {
	tex, elems, err := p.resolve(id)
	if err != nil {
		return nil, err
	}
	m := model{Textures: map[string]texRef{}, Elements: elems}
	for k := range tex {
		if t := lookupTex(tex, "#"+k); t != "" {
			m.Textures[k] = texRef(t)
		}
	}
	if p.flat(id) {
		ao := false
		m.AO = &ao
	}
	if p.cross(id) {
		m.Shape = shapeCross
	}
	if c, err := p.tintColor(id); err != nil {
		return nil, err
	} else if c != 0 {
		m.TintColor = fmt.Sprintf("#%06X", c)
	}
	return json.MarshalIndent(m, "", " ")
}

func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o644)
}
