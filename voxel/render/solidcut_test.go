package render

import (
	"slices"
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/mesh"
)

func TestSolidCuts(t *testing.T) {
	px := []byte{9, 9, 9, 255, 9, 9, 9, 26}
	if alphaCut(px) {
		t.Error("alpha 26 is kept by the 0.1 test")
	}
	if px[7] = 25; !alphaCut(px) {
		t.Error("alpha 25 is discarded by the 0.1 test")
	}
	// solid group 0: quads with textures 1, 0, 1, 0; group 1: texture 0
	var m mesh.Mesh
	m.Data = make([]uint32, 20*mesh.VertexWords)
	for q, tex := range []uint32{1, 0, 1, 0, 0} {
		for v := range 4 {
			m.Data[(q*4+v)*mesh.VertexWords] = uint32(q) // quad id
			m.Data[(q*4+v)*mesh.VertexWords+1] = tex
		}
	}
	m.Ranges[voxel.LayerSolid][0] = mesh.Range{First: 0, Count: 16}
	m.Ranges[voxel.LayerSolid][1] = mesh.Range{First: 16, Count: 4}
	var nc [mesh.GroupCount]int
	if splitCuts(&m, []bool{false, false}, &nc) || nc[0] != 16 || nc[1] != 4 {
		t.Errorf("no cut textures: nc %v", nc[:2])
	}
	if !splitCuts(&m, []bool{false, true}, &nc) || nc[0] != 8 || nc[1] != 4 {
		t.Errorf("texture 1 cut: nc %v", nc[:2])
	}
	var order []uint32
	for q := range 5 {
		order = append(order, m.Data[q*4*mesh.VertexWords])
	}
	if !slices.Equal(order, []uint32{1, 3, 0, 2, 4}) {
		t.Errorf("quad order %v, want 1 3 0 2 4", order)
	}
	if !splitCuts(&m, []bool{false}, &nc) || nc[0] != 8 {
		t.Error("an unknown texture counts as cut")
	}
}
