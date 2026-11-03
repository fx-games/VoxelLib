package render

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestPickProfile: a Radeon HD gets the legacy profile on both backends
// (GL names the vendor, Vulkan gives its PCI id); newer Radeons and other
// GPUs the default; the environment names a profile or overrides a choice.
func TestPickProfile(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		a    gputypes.AdapterInfo
		want string
	}{
		{gputypes.AdapterInfo{Name: "AMD Radeon HD 8280E", Vendor: "ATI Technologies Inc.", Backend: gputypes.BackendGL}, "legacy-radeon"},
		{gputypes.AdapterInfo{Name: "AMD Radeon HD 8280E", VendorID: 0x1002, Backend: gputypes.BackendVulkan}, "legacy-radeon"},
		{gputypes.AdapterInfo{Name: "AMD Radeon(TM) HD 7660D", VendorID: 0x1002}, "legacy-radeon"},
		{gputypes.AdapterInfo{Name: "AMD Radeon RX 6800", VendorID: 0x1002}, "default"},
		{gputypes.AdapterInfo{Name: "Apple M2", Backend: gputypes.BackendMetal}, "default"},
		{gputypes.AdapterInfo{Name: "NVIDIA GeForce RTX 4090", VendorID: 0x10de}, "default"},
	} {
		if p, err := pickProfile(c.a, env()); err != nil || p.Name != c.want {
			t.Errorf("%s: %v %v, want %s", c.a.Name, p, err, c.want)
		}
	}
	m2 := gputypes.AdapterInfo{Name: "Apple M2"}
	p, err := pickProfile(m2, env("VOXELLIB_RENDER_PROFILE", "legacy-radeon", "VOXELLIB_NO_MULTIDRAW", "1", "VOXELLIB_TERRAIN_LUT", "array"))
	if err != nil || p.Name != "legacy-radeon" || p.Backend != gputypes.BackendGL || p.MultiDraw || p.SelectLUT {
		t.Errorf("overridden: %v %v", p, err)
	}
	if _, err := pickProfile(m2, env("VOXELLIB_RENDER_PROFILE", "fastest")); err == nil {
		t.Error("unknown profile accepted")
	}
	if _, err := pickProfile(m2, env("VOXELLIB_TERRAIN_LUT", "texture")); err == nil {
		t.Error("unknown terrain lut accepted")
	}
}

// TestTerrainSelectLUT: the select variant replaces both tables and their
// lookup in every terrain program, keeps their values, and compiles in
// every profile (as Metal compiles it).
func TestTerrainSelectLUT(t *testing.T) {
	for _, src := range []string{terrainWGSL, terrainSolidWGSL} {
		for _, r := range terrainLUTs {
			if strings.Count(src, r[0]) != 1 {
				t.Fatalf("terrain program lacks %q", r[0])
			}
		}
		if terrainSource(src, Profile{}) != src {
			t.Error("the array variant changed the program")
		}
		v := terrainSource(src, Profile{SelectLUT: true})
		if strings.Contains(v, "= array<") {
			t.Error("the select variant keeps an array")
		}
		validWGSL(t, v)
	}
	for _, p := range profiles {
		checkMSL(t, p.Name+" terrain", terrainSource(terrainWGSL, p))
		checkMSL(t, p.Name+" terrain solid", terrainSource(terrainSolidWGSL, p))
	}
	// The chains give the arrays' values: shade by face 0-6, AO by level 0-3.
	sel := func(c bool, t, f float32) float32 {
		if c {
			return t
		}
		return f
	}
	for n, want := range []float32{0.5, 1.0, 0.8, 0.8, 0.6, 0.6, 1.0} {
		u := uint32(n) // the chain in terrainLUTs, select(f, t, c) as sel(c, t, f)
		if got := sel(u > 3, sel(u > 5, 1.0, 0.6), sel(u > 1, 0.8, sel(u == 1, 1.0, 0.5))); got != want {
			t.Errorf("shade %d: %v, want %v", n, got, want)
		}
	}
	for a, want := range []float32{0.45, 0.65, 0.82, 1.0} {
		u := uint32(a)
		if got := sel(u > 1, sel(u == 3, 1.0, 0.82), sel(u == 1, 0.65, 0.45)); got != want {
			t.Errorf("ao %d: %v, want %v", a, got, want)
		}
	}
	for _, c := range []string{"select(select(select(0.5, 1.0, n == 1u), 0.8, n > 1u), select(0.6, 1.0, n > 5u), n > 3u)",
		"select(select(0.45, 0.65, a == 1u), select(0.82, 1.0, a == 3u), a > 1u)"} {
		if !strings.Contains(terrainLUTs[1][1], c) {
			t.Errorf("the test's chain %s is not the shader's", c)
		}
	}
}
