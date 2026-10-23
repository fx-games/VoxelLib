package mesh

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// renderFaceXNeg on bounds x 0..1, y 2..14, z 3..13 (1/16): the corner
// (minX, maxY, maxZ) gets u = maxZ, v = 16 - maxY (RenderBlocks d4, d5).
func TestRBBox(t *testing.T) {
	pos, uv := RBBox([3]float32{0, 2, 3}, [3]float32{1, 14, 13})
	for k, p := range pos[voxel.FaceWest] {
		if p[0] != 0 {
			t.Fatalf("west corner %d at x %v", k, p[0])
		}
		if p[1] == 14.0/16 && p[2] == 13.0/16 && uv[voxel.FaceWest][k] != [2]float32{13, 2} {
			t.Errorf("west (0,14,13): uv %v", uv[voxel.FaceWest][k])
		}
	}
}
