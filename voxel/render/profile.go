package render

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/gogpu/gputypes"
)

// A Profile is the renderer's choices for one class of system: backend,
// draw path and shader variants. The pipelines it selects live side by
// side, so tuning one class (say a 2013 Radeon on Windows 7) never changes
// what another (Apple silicon on Metal) runs. Every variant must render
// the same image; profiles trade only speed.
type Profile struct {
	Name string
	// Backend is the backend this class runs fastest on; Init switches to
	// it when the first adapter came from another one and it works. Zero
	// keeps the first.
	Backend gputypes.Backend
	// MultiDraw draws a region's ranges with one indirect multidraw where
	// the device has it, else one draw per range.
	MultiDraw bool
	// SelectLUT picks the terrain's face shade and AO with select chains
	// rather than runtime-indexed arrays, which naga's SPIR-V keeps in
	// memory (a scratch spill on older drivers).
	SelectLUT bool
	// Env sets the backend's own knobs (the wgpu fork's GOGPU_* settings)
	// before the device opens; a variable already set wins.
	Env map[string]string
	// match reports whether the profile suits an adapter; profiles are
	// tried in order and the last matches everything.
	match func(gputypes.AdapterInfo) bool
}

// legacyRadeon matches TeraScale and early GCN Radeons (HD 5000-8000,
// 2009-2014), APUs included.
var legacyRadeon = regexp.MustCompile(`(?i)radeon.*\bhd\s*[5-8][0-9]{3}`)

// profiles, most specific first. Numbers behind a choice come from the
// serialized benches on the hardware it names (see `-bench`).
var profiles = []Profile{
	{
		// Radeon HD 8280E, Windows 7, driver 19.50: GL ran at 108 fps where
		// Vulkan ran at 59 (terrain GPU time twice GL's).
		Name: "legacy-radeon", Backend: gputypes.BackendGL, MultiDraw: true, SelectLUT: true,
		// On Vulkan, driver 19.50 hangs waiting on timeline semaphores.
		Env: map[string]string{"GOGPU_VK_NO_TIMELINE": "1"},
		match: func(a gputypes.AdapterInfo) bool {
			return (a.VendorID == 0x1002 || strings.Contains(a.Vendor, "AMD") || strings.HasPrefix(a.Vendor, "ATI")) &&
				legacyRadeon.MatchString(a.Name)
		},
	},
	{Name: "default", MultiDraw: true, match: func(gputypes.AdapterInfo) bool { return true }},
}

// CurrentProfile is the profile Init chose.
func CurrentProfile() Profile { return dev.profile }

// AdapterName is the GPU and backend Init chose, e.g. "Apple M5 (Metal)".
func AdapterName() string {
	if dev.adapter == nil {
		return "no adapter"
	}
	i := dev.adapter.Info()
	return fmt.Sprintf("%s (%v)", i.Name, i.Backend)
}

// ProfileNames lists the profiles, most specific first.
func ProfileNames() []string {
	var n []string
	for _, p := range profiles {
		n = append(n, p.Name)
	}
	return n
}

// pickProfile returns the profile for an adapter: VOXELLIB_RENDER_PROFILE
// names one outright, else the first that matches. VOXELLIB_NO_MULTIDRAW
// and VOXELLIB_TERRAIN_LUT (array, select) then override single choices,
// for A/B runs.
func pickProfile(a gputypes.AdapterInfo, getenv func(string) string) (Profile, error) {
	var p Profile
	if name := getenv("VOXELLIB_RENDER_PROFILE"); name != "" {
		i := slices.IndexFunc(profiles, func(p Profile) bool { return p.Name == name })
		if i < 0 {
			return p, fmt.Errorf("VOXELLIB_RENDER_PROFILE %q: not one of %s", name, strings.Join(ProfileNames(), ", "))
		}
		p = profiles[i]
	} else {
		p = profiles[slices.IndexFunc(profiles, func(p Profile) bool { return p.match(a) })]
	}
	if getenv("VOXELLIB_NO_MULTIDRAW") != "" {
		p.MultiDraw = false
	}
	switch l := getenv("VOXELLIB_TERRAIN_LUT"); l {
	case "":
	case "array", "select":
		p.SelectLUT = l == "select"
	default:
		return p, fmt.Errorf("VOXELLIB_TERRAIN_LUT %q: not array or select", l)
	}
	return p, nil
}

// setEnv sets p.Env's variables that are not set yet.
func (p Profile) setEnv() {
	for k, v := range p.Env {
		if _, ok := os.LookupEnv(k); !ok {
			os.Setenv(k, v)
		}
	}
}

// String is the profile's name and choices, for logs and bench output.
func (p Profile) String() string {
	lut := "array"
	if p.SelectLUT {
		lut = "select"
	}
	env := ""
	for _, k := range slices.Sorted(maps.Keys(p.Env)) {
		env += ", " + k + "=" + p.Env[k]
	}
	return fmt.Sprintf("%s (multidraw %v, terrain lut %s%s)", p.Name, p.MultiDraw, lut, env)
}
