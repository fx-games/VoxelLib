package render

import (
	"strings"
	"testing"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/msl"
)

// TestPackWGSL compiles every pack program the way the Metal backend does
// (naga WGSL -> IR -> validation -> MSL), so shader errors fail here
// instead of at the first pack frame.
func TestPackWGSL(t *testing.T) {
	progs := map[string]string{
		"terrain": packTerrainWGSL, "water": packWaterWGSL, "shadow": packShadowWGSL,
		"basic": packBasicWGSL, "damage": packDamageWGSL, "weather": packWeatherWGSL,
		"particle":  packParticleWGSL,
		"composite": packCompositeWGSL, "composite1": packComposite1WGSL, "composite2": packComposite2WGSL,
		"composite3": packComposite3WGSL, "composite4": packComposite4WGSL, "final": packFinalWGSL,
		"mip": packMipWGSL,
	}
	for _, hand := range []string{"false", "true"} {
		progs["model/"+hand] = strings.Replace(packModelWGSL, "HAND_FLAG", hand, 1)
		progs["item/"+hand] = strings.Replace(packItemWGSL, "HAND_FLAG", hand, 1)
		progs["block/"+hand] = strings.Replace(packBlockWGSL, "HAND_FLAG", hand, 1)
	}
	for name, src := range progs {
		ast, err := naga.Parse(src)
		if err != nil {
			t.Errorf("%s: parse: %v", name, err)
			continue
		}
		mod, err := naga.LowerWithSource(ast, src)
		if err != nil {
			t.Errorf("%s: lower: %v", name, err)
			continue
		}
		if errs, err := naga.Validate(mod); err != nil || len(errs) > 0 {
			t.Errorf("%s: validate: %v %v", name, err, errs)
			continue
		}
		if _, _, err := msl.Compile(mod, msl.DefaultOptions()); err != nil {
			t.Errorf("%s: msl: %v", name, err)
		}
	}
}
