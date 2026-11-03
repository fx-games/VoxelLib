package actors

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/nav"
)

func TestTerrain(t *testing.T) {
	r := voxel.NewRegistry()
	w := voxel.NewWorld(r, 0, 64)
	c := w.NewChunk(voxel.ChunkPos{})
	w.AddChunk(c)
	stone := r.Default("stone")
	for x := range 16 {
		for z := range 16 {
			w.SetBlock(x, 0, z, stone)
		}
	}
	// A fence line across x = 5, with a door at z = 8.
	for z := range 16 {
		w.SetBlock(5, 1, z, r.Default("oak_fence"))
	}
	door := func(half string) voxel.StateID {
		return r.State("oak_door", map[string]string{"facing": "east", "open": "false", "hinge": "left", "half": half})
	}
	w.SetBlock(5, 1, 8, door("lower"))
	w.SetBlock(5, 2, 8, door("upper"))
	tr := &Terrain{W: w}
	if !tr.Solid(5, 1, 3) || !tr.Solid(5, 2, 3) {
		t.Error("fence or the cell above it not solid")
	}
	if tr.BodySolid(5, 1, 3) {
		t.Error("fence a whole cube for bodies")
	}
	if !tr.Solid(5, 1, 20) {
		t.Error("unloaded chunk not solid")
	}
	g := tr.Graph(nav.SizeOf(0.6, 1.8))
	path, ok := g.Path(nav.Pos{X: 2, Y: 1, Z: 2}, nav.Pos{X: 9, Y: 1, Z: 2}, 0)
	if !ok {
		t.Fatal("no path through the door")
	}
	through := false
	for _, p := range path {
		through = through || p == (nav.Pos{X: 5, Y: 1, Z: 8})
	}
	if !through {
		t.Errorf("path %v not through the door", path)
	}
	tr.SetDoor(5, 2, 8, true)
	if _, open := tr.Door(5, 1, 8); !open {
		t.Error("lower half not opened with the upper")
	}
}
