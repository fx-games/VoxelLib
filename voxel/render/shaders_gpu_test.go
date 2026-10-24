package render

import (
	"strings"
	"testing"

	"github.com/gogpu/naga"
)

// TestTerrainSolidWGSL: the solid variant drops exactly the alpha test.
func TestTerrainSolidWGSL(t *testing.T) {
	if strings.Contains(terrainSolidWGSL, "discard") || len(terrainWGSL)-len(terrainSolidWGSL) != len("\tif (c.a < u.params.z) { discard; }\n") {
		t.Fatal("terrainSolidWGSL is not terrainWGSL without its alpha test")
	}
}

// TestFogEXP: an exponential fog reaches the shaders as fog.x = -density
// (fogFactor's GL_EXP branch: 1-exp(-density*dist)), a linear one as is;
// the terrain program with it still validates.
func TestFogEXP(t *testing.T) {
	if x, y := (Fog{Start: 3, End: 30, Density: 0.1}).gpu(); x != -0.1 || y != 30 {
		t.Errorf("exp fog gpu %v %v", x, y)
	}
	if x, y := (Fog{Start: 3, End: 30}).gpu(); x != 3 || y != 30 {
		t.Errorf("linear fog gpu %v %v", x, y)
	}
	if !strings.Contains(wgslLight, "if (fog.x < 0.0) { return 1.0 - clamp(exp(fog.x * dist), 0.0, 1.0); }") {
		t.Error("fogFactor has no GL_EXP branch")
	}
	ast, err := naga.Parse(terrainWGSL)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := naga.LowerWithSource(ast, terrainWGSL)
	if err != nil {
		t.Fatal(err)
	}
	if errs, err := naga.Validate(mod); err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
}
