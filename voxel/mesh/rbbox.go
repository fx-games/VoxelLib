package mesh

import "github.com/memmaker/voxellib/voxel"

// RBBox is RenderBlocks drawing the box lo..hi (in 1/16 of a block) with
// all six renderFace* calls and no uvRotate, as renderBlockAsItem does for
// a standard block: per face (voxel.Face order) the corners in blocks,
// wound as the mesher winds them, and their u,v in 1/16 of the tile.
func RBBox(lo, hi [3]float32) (pos [6][4][3]float32, uv [6][4][2]float32) {
	for f := voxel.Face(0); f < voxel.FaceCount; f++ {
		uv[f] = rbUV(f, lo, hi, 0)
		for k, co := range cornerOff[f] {
			for a := range 3 {
				pos[f][k][a] = (lo[a] + float32(co[a])*(hi[a]-lo[a])) / 16
			}
		}
	}
	return
}
