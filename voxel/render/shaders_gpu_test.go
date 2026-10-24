package render

import (
	"strings"
	"testing"
)

// TestTerrainSolidWGSL: the solid variant drops exactly the alpha test.
func TestTerrainSolidWGSL(t *testing.T) {
	if strings.Contains(terrainSolidWGSL, "discard") || len(terrainWGSL)-len(terrainSolidWGSL) != len("\tif (c.a < u.params.z) { discard; }\n") {
		t.Fatal("terrainSolidWGSL is not terrainWGSL without its alpha test")
	}
}
