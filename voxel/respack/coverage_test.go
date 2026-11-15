package respack

import (
	"encoding/json"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// drawnElsewhere are the blocks with no block model by design: invisible
// ones and fluids (voxel's liquid mesher draws those).
var drawnElsewhere = map[string]bool{
	"air": true, "cave_air": true, "void_air": true, "light": true, "barrier": true,
	"structure_void": true, "moving_piston": true, "water": true, "lava": true, "bubble_column": true,
}

// TestShapeCoverage is the gap list: every state of every blockstate in the
// jar ($VOXELLIB_JAR) resolves to geometry whose textures exist. A failure
// names a shape to add to the shape collection (voxel/shapes).
func TestShapeCoverage(t *testing.T) {
	jar := os.Getenv("VOXELLIB_JAR")
	if jar == "" {
		t.Skip("set VOXELLIB_JAR to a Java Edition client jar")
	}
	p, err := Open(jar)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	files, err := fs.Glob(p.sources[0], "assets/minecraft/blockstates/*.json")
	if err != nil {
		t.Fatal(err)
	}
	gaps := map[string]string{}
	for _, f := range files {
		name := strings.TrimSuffix(f[strings.LastIndex(f, "/")+1:], ".json")
		if drawnElsewhere[name] {
			continue
		}
		data, err := p.readFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var bs blockstate
		if err := json.Unmarshal(data, &bs); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, props := range sampleStates(t, &bs) {
			if why := p.gap(name, props); why != "" {
				gaps[name] = why
				break
			}
		}
	}
	names := make([]string, 0, len(gaps))
	for n := range gaps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t.Errorf("%s: %s", n, gaps[n])
	}
	t.Logf("%d blockstates, %d gaps", len(files), len(gaps))
}

// gap is why a state does not draw ("" when it does).
func (p *Pack) gap(name string, props map[string]string) string {
	m, ok := p.Resolver()(name, props)
	switch {
	case !ok:
		return "no geometry"
	case m.Shape == voxel.ShapeBoxes && len(m.Boxes) == 0:
		return "no boxes"
	}
	var texs []string
	for _, b := range m.Boxes {
		for f, tex := range b.Tex {
			if b.Skip&(1<<f) == 0 {
				texs = append(texs, tex)
			}
		}
	}
	if m.Shape != voxel.ShapeBoxes {
		texs = append(texs, m.Tex[:]...)
	}
	for _, tex := range texs {
		if tex == "" {
			continue
		}
		if _, err := p.Texture(tex); err != nil {
			return "texture " + tex + ": " + err.Error()
		}
	}
	return ""
}

// sampleStates are the states a blockstate names: each variant key, and for
// multipart each entry's condition (first alternative) on a full state.
func sampleStates(t *testing.T, bs *blockstate) []map[string]string {
	var out []map[string]string
	for key := range bs.Variants {
		out = append(out, parseProps(key))
	}
	if len(bs.Multipart) == 0 {
		return out
	}
	var parts []part
	if err := json.Unmarshal(bs.Multipart, &parts); err != nil {
		t.Fatal(err)
	}
	// each entry's condition over a base state of every key's first value
	conds, base := []map[string]string{}, map[string]string{}
	for _, pt := range parts {
		var c map[string]any
		if json.Unmarshal(pt.When, &c) != nil {
			continue // OR/AND lists: covered by their plain entries
		}
		props := map[string]string{}
		for k, v := range c {
			if s, ok := v.(string); ok {
				props[k] = strings.Split(s, "|")[0]
				if _, seen := base[k]; !seen {
					base[k] = props[k]
				}
			}
		}
		conds = append(conds, props)
	}
	for _, c := range conds {
		props := map[string]string{}
		for k, v := range base {
			props[k] = v
		}
		for k, v := range c {
			props[k] = v
		}
		out = append(out, props)
	}
	return out
}

func parseProps(key string) map[string]string {
	props := map[string]string{}
	for _, kv := range strings.Split(key, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			props[k] = v
		}
	}
	return props
}
