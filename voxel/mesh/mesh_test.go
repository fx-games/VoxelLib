package mesh

import (
	"testing"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/gen"
)

func TestMeshGenerated(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, -64, 384)
	g := gen.New(reg, 7)
	for cx := -2; cx <= 2; cx++ {
		for cz := -2; cz <= 2; cz++ {
			c := w.NewChunk(voxel.ChunkPos{X: cx, Z: cz})
			g.Generate(w, c)
			w.AddChunk(c)
		}
	}
	start := time.Now()
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			w.TryLightChunk(voxel.ChunkPos{X: cx, Z: cz})
		}
	}
	t.Logf("light: %v/chunk", time.Since(start)/9)
	m := NewMesher()
	quads, n := 0, 0
	start = time.Now()
	for sy := -4; sy < 20; sy++ {
		m.Snap.Capture(w, voxel.SectionPos{X: 0, Y: sy, Z: 0})
		res := m.Build(reg.Table(), nil)
		quads += res.Quads()
		n++
	}
	t.Logf("mesh: %v/section, %d quads in column", time.Since(start)/time.Duration(n), quads)
	if quads == 0 {
		t.Fatal("no quads")
	}
}
