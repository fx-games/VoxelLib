package mesh

import (
	"testing"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

func TestMeshGenerated(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, -64, 384)
	g := overworld.New(reg, 7)
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

func TestLiquidCorners(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, 0, 64)
	c := w.NewChunk(voxel.ChunkPos{})
	w.AddChunk(c)
	stone, source, film := reg.Parse("stone"), reg.Parse("water"), reg.Parse("water[level=7]")
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			w.SetBlock(x, 0, z, stone)
		}
	}
	w.SetBlock(2, 1, 2, source)
	for _, p := range [][2]int{{6, 6}, {7, 6}, {6, 7}, {7, 7}} { // a 2×2 puddle
		w.SetBlock(p[0], 1, p[1], film)
	}
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	m.table = reg.Table()
	def := reg.Info(source).Def
	at := func(x, z, cx, cz int) int { return m.liquidCorner(idx(x, 1, z), def, cx, cz) }
	if h := at(2, 2, 0, 0); h != 22 { // source: 8/9*10 over weight 13 → 0.68
		t.Errorf("lone source corner %d, want 22", h)
	}
	if h := at(6, 6, 1, 1); h != 4 { // puddle centre: 1/9 all round
		t.Errorf("puddle centre %d, want 4", h)
	}
	if h := at(6, 6, 0, 0); h != 1 { // puddle edge: a film
		t.Errorf("puddle edge %d, want 1", h)
	}
	if h := at(7, 6, 0, 1); h != 4 { // the same centre corner seen from a neighbour
		t.Errorf("shared corner %d, want 4", h)
	}
}
