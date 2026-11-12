package voxel

import "testing"

func TestStrength(t *testing.T) {
	r := NewRegistry()
	r.Register(&BlockDef{Name: "test:thing", Shape: ShapeCube})
	r.Register(&BlockDef{Name: "minecraft:planks#0", Shape: ShapeCube}) // a VoxelMap key
	for name, want := range map[string]Strength{
		"stone":              {1.5, 6},
		"bedrock":            {-1, 3600000},
		"obsidian":           {50, 1200},
		"test:thing":         DefaultStrength,
		"minecraft:planks#0": {2, 3},
	} {
		if got := r.Info(r.Default(name)).Strength; got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
	if !r.Info(r.Default("bedrock")).Strength.Unbreakable() {
		t.Error("bedrock breaks")
	}
}
