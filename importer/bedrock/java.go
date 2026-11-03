package bedrock

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png" // jar textures
	"io"
	"sort"
	"strings"
)

// Jar reads blockstates, block models and textures from a Java Edition
// client jar (or anything laid out like one).
type Jar struct {
	z      *zip.ReadCloser
	files  map[string]*zip.File
	models map[string]*jmodel
	alpha  map[string]alphaKind
	sizes  map[string]image.Point
	// Used collects every asset path read (models, textures and their
	// .mcmeta), to copy into a map.
	Used map[string]bool
}

// OpenJar opens a client jar.
func OpenJar(file string) (*Jar, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	j := &Jar{z: z, files: map[string]*zip.File{}, models: map[string]*jmodel{}, alpha: map[string]alphaKind{},
		sizes: map[string]image.Point{}, Used: map[string]bool{}}
	for _, f := range z.File {
		j.files[f.Name] = f
	}
	return j, nil
}

// Close closes the jar.
func (j *Jar) Close() error { return j.z.Close() }

// Read returns a file of the jar.
func (j *Jar) Read(name string) ([]byte, error) {
	f, ok := j.files[name]
	if !ok {
		return nil, fmt.Errorf("jar: no %s", name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Has reports whether the jar holds a file.
func (j *Jar) Has(name string) bool { _, ok := j.files[name]; return ok }

// HasBlock reports whether the jar has a blockstate for a Java block name.
func (j *Jar) HasBlock(name string) bool {
	return j.Has("assets/minecraft/blockstates/" + name + ".json")
}

// Apply is one model of a block state, turned like a blockstate variant.
type Apply struct {
	Model  string `json:"model"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	UVLock bool   `json:"uvlock"`
}

type blockstate struct {
	Variants  map[string]json.RawMessage `json:"variants"`
	Multipart []struct {
		When  json.RawMessage `json:"when"`
		Apply json.RawMessage `json:"apply"`
	} `json:"multipart"`
}

// Models lists the models a state draws: its variant's, or every
// multipart case that holds.
func (j *Jar) Models(s JavaState) ([]Apply, error) {
	data, err := j.Read("assets/minecraft/blockstates/" + s.Name + ".json")
	if err != nil {
		return nil, err
	}
	var bs blockstate
	if err := json.Unmarshal(data, &bs); err != nil {
		return nil, fmt.Errorf("%s: %w", s.Name, err)
	}
	if bs.Variants != nil {
		best, bestN := json.RawMessage(nil), -1
		for key, raw := range bs.Variants {
			n, ok := matchKey(key, s.Props, false)
			if ok && n > bestN {
				best, bestN = raw, n
			}
		}
		if best == nil { // properties the state lacks take the first value listed
			keys := make([]string, 0, len(bs.Variants))
			for k := range bs.Variants {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if _, ok := matchKey(key, s.Props, true); ok {
					best = bs.Variants[key]
					break
				}
			}
		}
		if best == nil {
			return nil, fmt.Errorf("%s: no variant for %v", s.Name, s.Props)
		}
		a, err := firstApply(best)
		if err != nil {
			return nil, err
		}
		return []Apply{a}, nil
	}
	var out []Apply
	for _, c := range bs.Multipart {
		if len(c.When) > 0 && !whenHolds(c.When, s.Props) {
			continue
		}
		a, err := firstApply(c.Apply)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// matchKey checks a variant key ("facing=east,half=top") against props
// and counts its conditions; loose lets properties props lacks match.
func matchKey(key string, props map[string]string, loose bool) (int, bool) {
	if key == "" || key == "normal" {
		return 0, true
	}
	kvs := strings.Split(key, ",")
	for _, kv := range kvs {
		k, v, _ := strings.Cut(kv, "=")
		if p, has := props[k]; p != v && (!loose || has) {
			return 0, false
		}
	}
	return len(kvs), true
}

// whenHolds evaluates a multipart "when": property values ("a|b"), or
// "OR"/"AND" lists of such.
func whenHolds(raw json.RawMessage, props map[string]string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	for k, v := range m {
		switch k {
		case "OR", "AND":
			var list []json.RawMessage
			json.Unmarshal(v, &list)
			any := false
			for _, c := range list {
				h := whenHolds(c, props)
				if k == "AND" && !h {
					return false
				}
				any = any || h
			}
			if k == "OR" && !any {
				return false
			}
		default:
			var want string
			json.Unmarshal(v, &want)
			ok := false
			for _, w := range strings.Split(want, "|") {
				ok = ok || props[k] == w
			}
			if !ok {
				return false
			}
		}
	}
	return true
}

// firstApply reads a variant: one Apply or a weighted list (the first
// is taken).
func firstApply(raw json.RawMessage) (Apply, error) {
	var a Apply
	if len(raw) > 0 && raw[0] == '[' {
		var as []Apply
		if err := json.Unmarshal(raw, &as); err != nil || len(as) == 0 {
			return a, fmt.Errorf("bad variant list %s", raw)
		}
		return as[0], nil
	}
	err := json.Unmarshal(raw, &a)
	return a, err
}

type jface struct {
	Texture   string      `json:"texture"`
	Cullface  string      `json:"cullface"`
	TintIndex *int        `json:"tintindex"`
	UV        *[4]float32 `json:"uv"`
	Rotation  int         `json:"rotation"`
}

type jelement struct {
	From     [3]float32       `json:"from"`
	To       [3]float32       `json:"to"`
	Faces    map[string]jface `json:"faces"`
	Rotation *struct {
		Angle float32 `json:"angle"`
	} `json:"rotation"`
}

type jmodel struct {
	Parent   string            `json:"parent"`
	Textures map[string]texRef `json:"textures"`
	Elements []jelement        `json:"elements"`
}

// texRef is a texture variable: an id or "#var", or (since 26.1) an
// object with the id as "sprite", "force_translucent" asking for the
// translucent layer.
type texRef struct {
	ID          string
	Translucent bool
}

func (t *texRef) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '{' {
		var o struct {
			Sprite      string `json:"sprite"`
			Translucent bool   `json:"force_translucent"`
		}
		err := json.Unmarshal(b, &o)
		t.ID, t.Translucent = o.Sprite, o.Translucent
		return err
	}
	return json.Unmarshal(b, &t.ID)
}

// Model is a resolved block model: its textures (variables merged down
// the parent chain), its elements, and the chain of model ids.
type Model struct {
	Textures map[string]string
	Elements []jelement
	Chain    []string
	// Translucent is set when a texture variable is force_translucent.
	Translucent bool
}

// modelPath turns "minecraft:block/x" or "block/x" into the jar path.
func modelPath(id string) string {
	ns, rel, ok := strings.Cut(id, ":")
	if !ok {
		ns, rel = "minecraft", id
	}
	return "assets/" + ns + "/models/" + rel + ".json"
}

func (j *Jar) model(id string) (*jmodel, error) {
	p := modelPath(id)
	if m, ok := j.models[p]; ok {
		return m, nil
	}
	data, err := j.Read(p)
	if err != nil {
		return nil, err
	}
	m := &jmodel{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	j.models[p] = m
	j.Used[p] = true
	return m, nil
}

// Resolve walks a model's parent chain.
func (j *Jar) Resolve(id string) (*Model, error) {
	r := &Model{Textures: map[string]string{}}
	for depth := 0; id != "" && depth < 16; depth++ {
		_, rel, ok := strings.Cut(id, ":")
		if !ok {
			rel = id
		}
		r.Chain = append(r.Chain, rel)
		if strings.HasPrefix(rel, "builtin/") {
			break
		}
		m, err := j.model(id)
		if err != nil {
			return nil, err
		}
		for k, v := range m.Textures {
			if _, ok := r.Textures[k]; !ok {
				r.Textures[k] = v.ID
				r.Translucent = r.Translucent || v.Translucent
			}
		}
		if r.Elements == nil && len(m.Elements) > 0 {
			r.Elements = m.Elements
		}
		id = m.Parent
	}
	return r, nil
}

// Tex resolves a texture reference ("#side") to a texture id
// ("minecraft:block/stone"), "" if unbound.
func (m *Model) Tex(ref string) string {
	for i := 0; i < 16 && strings.HasPrefix(ref, "#"); i++ {
		ref = m.Textures[ref[1:]]
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return ""
	}
	if !strings.Contains(ref, ":") {
		ref = "minecraft:" + ref
	}
	return ref
}

// Cross reports a plant drawn as two crossed quads.
func (m *Model) Cross() bool {
	for _, c := range m.Chain {
		if c == "block/cross" || c == "block/tinted_cross" {
			return true
		}
	}
	return false
}

// FullCube reports a model that is one unrotated 0..16 cube with all six
// faces, and returns its face textures (down, up, north, south, west,
// east) and whether any face is tinted.
func (m *Model) FullCube() (tex [6]string, tinted, ok bool) {
	if len(m.Elements) != 1 {
		return tex, false, false
	}
	e := m.Elements[0]
	if e.From != [3]float32{0, 0, 0} || e.To != [3]float32{16, 16, 16} || len(e.Faces) != 6 ||
		e.Rotation != nil && e.Rotation.Angle != 0 {
		return tex, false, false
	}
	for i, f := range faceOrder {
		fc, has := e.Faces[f]
		if !has || fc.Rotation != 0 || fc.UV != nil && *fc.UV != [4]float32{0, 0, 16, 16} {
			return tex, false, false
		}
		tex[i] = m.Tex(fc.Texture)
		tinted = tinted || fc.TintIndex != nil && *fc.TintIndex >= 0
	}
	return tex, tinted, true
}

// Solid reports a model that holds a full cube element (grass_block's base
// under its overlay): it hides its neighbours' faces.
func (m *Model) Solid() bool {
	for _, e := range m.Elements {
		if e.From == [3]float32{0, 0, 0} && e.To == [3]float32{16, 16, 16} && len(e.Faces) == 6 {
			return true
		}
	}
	return false
}

// Textures lists the textures the model's faces use.
func (m *Model) FaceTextures() []string {
	var out []string
	for _, e := range m.Elements {
		for _, f := range e.Faces {
			if t := m.Tex(f.Texture); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

var faceOrder = [6]string{"down", "up", "north", "south", "west", "east"}

// TexturePath is a texture id's path in the jar.
func TexturePath(id string) string {
	ns, rel, ok := strings.Cut(id, ":")
	if !ok {
		ns, rel = "minecraft", id
	}
	return "assets/" + ns + "/textures/" + rel + ".png"
}

type alphaKind uint8

const (
	alphaOpaque      alphaKind = iota
	alphaCutout                // some pixels fully transparent
	alphaTranslucent           // some pixels partly transparent
	alphaMissing
)

// Alpha classifies a texture's transparency, and marks it (and its
// animation) used.
func (j *Jar) Alpha(id string) alphaKind {
	p := TexturePath(id)
	if a, ok := j.alpha[p]; ok {
		return a
	}
	a := alphaMissing
	if data, err := j.Read(p); err == nil {
		j.Used[p] = true
		if j.Has(p + ".mcmeta") {
			j.Used[p+".mcmeta"] = true
		}
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			a = alphaOpaque
			b := img.Bounds()
			j.sizes[p] = b.Size()
			for y := b.Min.Y; y < b.Max.Y && a != alphaTranslucent; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					_, _, _, al := img.At(x, y).RGBA()
					if al == 0 {
						a = max(a, alphaCutout)
					} else if al < 0xffff {
						a = alphaTranslucent
						break
					}
				}
			}
		}
	}
	j.alpha[p] = a
	return a
}

// Size is a texture's size in pixels (after Alpha read it).
func (j *Jar) Size(id string) image.Point {
	j.Alpha(id)
	return j.sizes[TexturePath(id)]
}
