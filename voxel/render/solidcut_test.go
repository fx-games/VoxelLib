package render

import (
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
	var m mesh.Mesh
	m.Data = make([]uint32, 8*mesh.VertexWords)
	m.Data[4*mesh.VertexWords+1] = 1 // second quad: texture 1
	m.Ranges[voxel.LayerSolid][0] = mesh.Range{First: 0, Count: 8}
	if solidCuts(&m, []bool{false, false}) || !solidCuts(&m, []bool{false, true}) || !solidCuts(&m, []bool{false}) {
		t.Error("solidCuts misreads the quads' textures")
	}
}
