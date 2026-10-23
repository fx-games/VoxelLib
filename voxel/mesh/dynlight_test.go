package mesh

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// The snapshot holds max(stored block light, dynamic light), its border too.
func TestCaptureFoldsDynLights(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, 0, 64)
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			c := w.NewChunk(voxel.ChunkPos{X: cx, Z: cz})
			c.SetRaw(5, 20, 5, reg.Default("stone"))
			w.AddChunk(c)
		}
	}
	w.SetBlock(1, 20, 1, reg.Default("glowstone")) // stored light, if it emits
	w.SetDynLights([]voxel.DynLight{{X: 15, Y: 20, Z: 8, Level: 12}})
	var s Snapshot
	s.Capture(w, voxel.SectionPos{X: 1, Y: 1, Z: 0})
	if got := s.Light[idx(-1, 4, 8)] & 15; got != 12 {
		t.Fatalf("border (source) %d, want 12", got)
	}
	if got := s.Light[idx(2, 4, 8)] & 15; got != 9 {
		t.Fatalf("x=18: %d, want 9", got)
	}
	s.Capture(w, voxel.SectionPos{X: 0, Y: 1, Z: 0})
	for x := 0; x < 16; x++ {
		_, stored := w.Light(x, 20, 8)
		_, dyn := w.RenderLight(x, 20, 8)
		if got := s.Light[idx(x, 4, 8)] & 15; got != max(stored, dyn) || dyn < stored {
			t.Fatalf("x=%d: snapshot %d, stored %d, render %d", x, got, stored, dyn)
		}
	}
}
