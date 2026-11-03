package vanillapack

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/memmaker/voxellib/voxel/blockmodel"
)

// clientJar finds a Java Edition client jar to check the pack against:
// $VOXELLIB_JAR, else the newest one Fabric Loom cached.
func clientJar() string {
	if j := os.Getenv("VOXELLIB_JAR"); j != "" {
		return j
	}
	home, _ := os.UserHomeDir()
	js, _ := filepath.Glob(filepath.Join(home, ".gradle/caches/fabric-loom/*/minecraft-client.jar"))
	sort.Strings(js)
	if len(js) == 0 {
		return ""
	}
	return js[len(js)-1]
}

// TestAgainstJar checks that every block the pack generates draws like the
// reference: the same blockstate cases, and models whose elements, faces
// and face textures match the jar's. The jar is only read here.
func TestAgainstJar(t *testing.T) {
	jar := clientJar()
	if jar == "" {
		t.Skip("no client jar (set VOXELLIB_JAR)")
	}
	z, err := zip.OpenReader(jar)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	p := New(texturesOnly{os.DirFS("../../" + DefaultTextures)})
	names, _ := fs.Glob(z, statesDir+"*.json")
	var missing, bad []string
	for _, f := range names {
		name := strings.TrimSuffix(path.Base(f), ".json")
		want, err := drawn(z, name, nil)
		if err != nil {
			t.Fatalf("jar %s: %v", name, err)
		}
		got, err := drawn(p, name, nil)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		if d := diff(want, got); d != "" {
			bad = append(bad, name)
			t.Errorf("%s: %s", name, d)
		}
	}
	t.Logf("%d blocks: %d match, %d differ, %d not generated", len(names), len(names)-len(bad)-len(missing), len(bad), len(missing))
	if len(missing) > 0 {
		t.Logf("not generated: %s", strings.Join(missing, " "))
	}
}

// texturesOnly hides a texture pack's own blockstates and models: the test
// checks the generated ones.
type texturesOnly struct{ fs.FS }

func (t texturesOnly) Open(name string) (fs.File, error) {
	if strings.HasPrefix(name, statesDir) || strings.HasPrefix(name, modelsDir) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return t.FS.Open(name)
}

// drawn is what a blockstate draws: per case (a variant key, or a
// multipart condition), the models applied, each resolved to its faces.
// Cases keep doesn't take (nil: all) are left out.
func drawn(fsys fs.FS, name string, keep func(key string) bool) (map[string][]string, error) {
	data, err := fs.ReadFile(fsys, statesDir+name+".json")
	if err != nil {
		return nil, err
	}
	var bs struct {
		Variants  map[string]json.RawMessage
		Multipart []struct {
			When  json.RawMessage
			Apply json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &bs); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	add := func(key string, raw json.RawMessage) error {
		if keep != nil && !keep(key) {
			return nil
		}
		var vs []struct {
			Model  string
			X, Y   int
			UVLock bool
			Weight int
		}
		if raw[0] != '[' {
			raw = json.RawMessage("[" + string(raw) + "]")
		}
		if err := json.Unmarshal(raw, &vs); err != nil {
			return err
		}
		for _, v := range vs {
			lines, err := resolved(fsys, v.Model)
			if err != nil {
				return fmt.Errorf("%s: %w", v.Model, err)
			}
			w := max(v.Weight, 1)
			out[key] = append(out[key], fmt.Sprintf("model %s x%d y%d uvlock %v weight %d", strings.TrimPrefix(v.Model, "minecraft:"), v.X, v.Y, v.UVLock, w))
			out[key] = append(out[key], lines...)
		}
		return nil
	}
	for k, raw := range bs.Variants {
		if err := add("variant "+sortedKey(k), raw); err != nil {
			return nil, err
		}
	}
	for i, c := range bs.Multipart {
		var when any
		json.Unmarshal(c.When, &when)
		w, _ := json.Marshal(when) // map keys sorted
		if err := add(fmt.Sprintf("part %d when %s", i, w), c.Apply); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func sortedKey(k string) string {
	ps := strings.Split(k, ",")
	sort.Strings(ps)
	return strings.Join(ps, ",")
}

// lenient model: textures may be ids or {"sprite": id}.
type tmodel struct {
	Parent           string
	AmbientOcclusion *bool
	TextureSize      *[2]float64 `json:"texture_size"`
	Textures         map[string]any
	Elements         []blockmodel.Elem
}

// resolved lists a model's elements and faces with the textures resolved
// through its parents, UVs as drawn.
func resolved(fsys fs.FS, id string) ([]string, error) {
	tex := map[string]string{}
	var elems []blockmodel.Elem
	var ao *bool
	var size *[2]float64
	for depth := 0; id != "" && depth < 16; depth++ {
		ns, rel, ok := strings.Cut(id, ":")
		if !ok {
			ns, rel = "minecraft", id
		}
		if strings.HasPrefix(rel, "builtin/") {
			break
		}
		data, err := fs.ReadFile(fsys, "assets/"+ns+"/models/"+rel+".json")
		if err != nil {
			return nil, err
		}
		var m tmodel
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		for k, v := range m.Textures {
			if _, ok := tex[k]; ok {
				continue
			}
			switch v := v.(type) {
			case string:
				tex[k] = v
			case map[string]any:
				tex[k], _ = v["sprite"].(string)
			}
		}
		if elems == nil && len(m.Elements) > 0 {
			elems = m.Elements
		}
		if ao == nil {
			ao = m.AmbientOcclusion
		}
		if size == nil {
			size = m.TextureSize
		}
		id = m.Parent
	}
	look := func(ref string) string {
		for i := 0; i < 16 && strings.HasPrefix(ref, "#"); i++ {
			ref = tex[ref[1:]]
		}
		return strings.TrimPrefix(ref, "minecraft:")
	}
	lines := []string{fmt.Sprintf("ao %v size %v", ao == nil || *ao, size)}
	for _, e := range elems {
		l := fmt.Sprintf("  box %v %v shade %v light %d", e.From, e.To, e.Shade == nil || *e.Shade, e.LightEmission)
		if r := e.Rotation; r != nil {
			l += fmt.Sprintf(" rot %+v", *r)
		}
		lines = append(lines, l)
		for i, n := range blockmodel.FaceNames {
			f, ok := e.Faces[n]
			if !ok {
				continue
			}
			uv := blockmodel.DefaultUV(i, e.From, e.To)
			if f.UV != nil {
				uv = *f.UV
			}
			c := f.Cullface
			if c == "bottom" {
				c = "down"
			}
			tint := -1
			if f.TintIndex != nil {
				tint = *f.TintIndex
			}
			lines = append(lines, fmt.Sprintf("    %s %s uv %v cull %s rot %d tint %d", n, look(f.Texture), uv, c, f.Rotation, tint))
		}
	}
	return lines, nil
}

func diff(want, got map[string][]string) string {
	var keys []string
	for k := range want {
		keys = append(keys, k)
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, g := want[k], got[k]
		if w == nil {
			return "extra case " + k
		}
		if g == nil {
			return "no case " + k
		}
		for i := range max(len(w), len(g)) {
			var a, b string
			if i < len(w) {
				a = w[i]
			}
			if i < len(g) {
				b = g[i]
			}
			if a != b {
				return fmt.Sprintf("%s:\n  want %s\n  got  %s", k, a, b)
			}
		}
	}
	return ""
}

// TestTextures checks that every texture the pack's blocks draw (the
// texture pack's own blockstates and models over the generated ones) is in
// the texture pack, for the reference's block names.
func TestTextures(t *testing.T) {
	jar := clientJar()
	if jar == "" {
		t.Skip("no client jar (set VOXELLIB_JAR)")
	}
	z, err := zip.OpenReader(jar)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	p := New(os.DirFS("../../" + DefaultTextures))
	names, _ := fs.Glob(z, statesDir+"*.json")
	missing := map[string][]string{}
	for _, f := range names {
		name := strings.TrimSuffix(path.Base(f), ".json")
		want, err := drawn(z, name, nil)
		if err != nil {
			t.Fatalf("jar %s: %v", name, err)
		}
		// the texture pack's blockstates may have cases of their own
		// (and their multipart cases in another order)
		d, err := drawn(p, name, func(k string) bool { return want[k] != nil || strings.HasPrefix(k, "part ") })
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for k := range want {
			if d[k] == nil && strings.HasPrefix(k, "variant ") {
				t.Errorf("%s: no case %s", name, k)
				break
			}
		}
		for _, lines := range d {
			for _, l := range lines {
				ws := strings.Fields(l)
				if !strings.HasPrefix(l, "    ") || len(ws) < 2 {
					continue
				}
				if _, err := fs.Stat(p, "assets/minecraft/textures/"+ws[1]+".png"); err != nil && !slices.Contains(missing[ws[1]], name) {
					missing[ws[1]] = append(missing[ws[1]], name)
				}
			}
		}
	}
	for _, tex := range slices.Sorted(maps.Keys(missing)) {
		t.Errorf("no texture %s (%s)", tex, strings.Join(missing[tex], " "))
	}
}
