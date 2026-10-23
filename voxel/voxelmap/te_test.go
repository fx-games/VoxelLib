package voxelmap

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
)

func TestTEVariants(t *testing.T) {
	reg := voxel.NewRegistry()
	m := &Map{reg: reg, Blocks: map[string]*Block{
		"t:couch#2":     {Shape: "none", TE: []string{"Rot", "Left"}},
		"t:couch#2|1,1": {Shape: "none"},
		"t:couch#2|3,0": {Shape: "none"},
		"t:plain#0":     {Shape: "none"},
		"t:rack#7":      {Shape: "none", TE: []string{"Rot"}, TEBelow: true},
		"t:rack#7|3":    {Shape: "none"},
	}}
	m.register()
	w := voxel.NewWorld(reg, MinY, Height)
	c := w.NewChunk(voxel.ChunkPos{X: 1, Z: -1})
	couch := m.State("t:couch#2")
	c.SetRaw(1, 5, 2, couch)
	c.SetRaw(2, 5, 2, couch)
	c.SetRaw(3, 5, 2, m.State("t:plain#0"))
	c.SetRaw(3, 6, 2, m.State("t:rack#7")) // above the plain block's tile entity
	c.SetRaw(4, 6, 2, m.State("t:rack#7")) // at a tile entity of its own: not read
	te := func(x, z int, kv ...any) anvil.Compound {
		t := anvil.Compound{"x": int32(16 + x), "y": int32(5), "z": int32(-16 + z)}
		for i := 0; i < len(kv); i += 2 {
			t[kv[i].(string)] = kv[i+1]
		}
		return t
	}
	m.teVariants(c, anvil.Compound{"TileEntities": anvil.List{Type: anvil.TagCompound, Items: []any{
		te(1, 2, "Rot", int32(1), "Left", int8(1)),
		te(2, 2, "Rot", int32(2)), // 2,0: not defined, stays
		te(3, 2, "Rot", int32(3)),
		anvil.Compound{"x": int32(20), "y": int32(6), "z": int32(-14), "Rot": int32(3)},
	}}})
	for x, want := range map[int]string{3: "t:rack#7|3", 4: "t:rack#7"} {
		if got := m.Key(c.Get(x, 6, 2)); got != want {
			t.Errorf("upper x %d: %s, want %s", x, got, want)
		}
	}
	for x, want := range map[int]string{1: "t:couch#2|1,1", 2: "t:couch#2", 3: "t:plain#0"} {
		if got := m.Key(c.Get(x, 5, 2)); got != want {
			t.Errorf("x %d: %s, want %s", x, got, want)
		}
	}
	if BaseKey("t:couch#2|1,1") != "t:couch#2" {
		t.Error("BaseKey")
	}
}
