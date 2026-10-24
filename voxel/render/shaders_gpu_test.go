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
	validWGSL(t, terrainWGSL)
}

func validWGSL(t *testing.T, src string) {
	t.Helper()
	ast, err := naga.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := naga.LowerWithSource(ast, src)
	if err != nil {
		t.Fatal(err)
	}
	if errs, err := naga.Validate(mod); err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
}

// TestModelUVOffset: an instance's UVOffset reaches the draw with the
// wrap flag (fract(uv + offset): a repeating texture), none without one;
// the model programs with it validate.
func TestModelUVOffset(t *testing.T) {
	if d := (&ModelInstance{UVOffset: [2]float32{0.25, 0.5}}).params(); d.uvOff != [2]float32{0.25, 0.5} || d.uvWrap != 1 {
		t.Errorf("offset params %v %v", d.uvOff, d.uvWrap)
	}
	if d := (&ModelInstance{}).params(); d.uvWrap != 0 {
		t.Error("wrap without an offset")
	}
	validWGSL(t, modelWGSL)
	validWGSL(t, modelGlintWGSL)
}

// TestModelLightOnly: LightOnly reaches the shader as fog.w 0.5, between
// the lit (0) and emissive (1) thresholds; Emissive wins.
func TestModelLightOnly(t *testing.T) {
	for _, c := range []struct {
		in   ModelInstance
		want float32
	}{{ModelInstance{}, 0}, {ModelInstance{LightOnly: true}, 0.5}, {ModelInstance{Emissive: true}, 1}, {ModelInstance{Emissive: true, LightOnly: true}, 1}} {
		if got := c.in.params().lightMode(); got != c.want {
			t.Errorf("%+v: mode %v, want %v", c.in, got, c.want)
		}
	}
	for _, s := range []string{modelWGSL, packModelWGSL} {
		if !strings.Contains(s, "u.fog.w > 0.75") {
			t.Error("a model program's emissive threshold is not above light only")
		}
	}
}
