package render

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/mesh"
)

// A grass-like block: a cube plus an overlay box on its sides. With the
// crack texture only one quad per side may remain (RenderBlocks skips the
// grass side overlay when an override texture is set).
func TestDedupeQuads(t *testing.T) {
	side := [6]uint16{1, 1, 1, 1, 1, 1}
	info := voxel.StateInfo{Shape: voxel.ShapeBoxes, Tex: side, Boxes: []voxel.Box{
		{Max: [3]float32{16, 16, 16}},
		{Max: [3]float32{16, 16, 16}, Skip: 1<<voxel.FaceUp | 1<<voxel.FaceDown},
	}}
	m := mesh.Block(info, voxel.StateInfo{}, &mesh.Config{RenderBlocksUV: true})
	if q := m.Quads(); q != 10 {
		t.Fatalf("mesh: %d quads, want 10", q)
	}
	if q := len(dedupeQuads(append([]uint32(nil), m.Data...))) / (4 * mesh.VertexWords); q != 6 {
		t.Fatalf("deduped: %d quads, want 6", q)
	}
}
