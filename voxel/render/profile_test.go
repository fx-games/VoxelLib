package render

import (
	"os"
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

// TestTerrainSelectLUT: the select variant replaces the tables and their
// lookups in every terrain decoder (engine and pack), keeps their values,
// and every profile's programs compile (as Metal compiles them).
func TestTerrainSelectLUT(t *testing.T) {
	progs := map[string]string{"terrain": terrainWGSL, "terrain solid": terrainSolidWGSL,
		"pack terrain": packTerrainWGSL, "pack terrain solid": packTerrainSolidWGSL, "pack water": packWaterWGSL,
		"pack shadow": packShadowWGSL, "pack shadow solid": packShadowSolidWGSL}
	for name, src := range progs {
		if terrainSource(src, Profile{}) != src {
			t.Errorf("%s: the array variant changed the program", name)
		}
		v := terrainSource(src, Profile{SelectLUT: true})
		if v == src || strings.Contains(v, "ao[") || strings.Contains(v, "shade[") || strings.Contains(v, "array<f32, 4>(0.45") {
			t.Errorf("%s: the select variant keeps a table", name)
		}
		for _, p := range profiles {
			checkMSL(t, p.Name+" "+name, packStore(terrainSource(src, p), ""))
		}
	}
	// The chains give the arrays' values: shade by face 0-6, AO by level 0-3.
	sel := func(c bool, t, f float32) float32 {
		if c {
			return t
		}
		return f
	}
	for n, want := range []float32{0.5, 1.0, 0.8, 0.8, 0.6, 0.6, 1.0} {
		u := uint32(n) // shadeSelect, select(f, t, c) as sel(c, t, f)
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
		"select(select(0.45, 0.65, aoLevel == 1u), select(0.82, 1.0, aoLevel == 3u), aoLevel > 1u)"} {
		if !strings.Contains(shadeSelect+aoSelectWGSL, c) {
			t.Errorf("the test's chain %s is not the shader's", c)
		}
	}
}

// TestProfileEnv: a profile sets its backend knobs, but never over a value
// the environment already has.
func TestProfileEnv(t *testing.T) {
	t.Setenv("VOXELLIB_TEST_SET", "user")
	t.Setenv("VOXELLIB_TEST_UNSET", "")
	os.Unsetenv("VOXELLIB_TEST_UNSET")
	Profile{Env: map[string]string{"VOXELLIB_TEST_SET": "profile", "VOXELLIB_TEST_UNSET": "profile"}}.setEnv()
	if a, b := os.Getenv("VOXELLIB_TEST_SET"), os.Getenv("VOXELLIB_TEST_UNSET"); a != "user" || b != "profile" {
		t.Errorf("set %q, unset %q", a, b)
	}
	if p := profiles[0]; p.Name != "legacy-radeon" || p.Env["GOGPU_VK_NO_TIMELINE"] != "1" || !strings.Contains(p.String(), "GOGPU_VK_NO_TIMELINE=1") {
		t.Errorf("legacy-radeon: %v", p)
	}
}
