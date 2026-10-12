package voxel_test

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/gen"
)

func BenchmarkLightChunk(b *testing.B) {
	reg := voxel.NewRegistry()
	g := gen.New(reg, 3)
	var chunks []*voxel.Chunk
	w0 := voxel.NewWorld(reg, -64, 384)
	for cx := -3; cx <= 3; cx++ {
		for cz := -3; cz <= 3; cz++ {
			c := w0.NewChunk(voxel.ChunkPos{X: cx, Z: cz})
			g.Generate(w0, c)
			chunks = append(chunks, c)
		}
	}
	b.ResetTimer()
	n := 0
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		w := voxel.NewWorld(reg, -64, 384)
		for _, c := range chunks {
			c.Lit = false
			w.AddChunk(c)
		}
		b.StartTimer()
		for cx := -2; cx <= 2; cx++ {
			for cz := -2; cz <= 2; cz++ {
				w.TryLightChunk(voxel.ChunkPos{X: cx, Z: cz})
				n++
			}
		}
	}
	b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(n), "us/chunk")
}
