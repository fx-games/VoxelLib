package cuboid

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// Mesh is boxes as one unposed mesh in their own pixel space (Y down, no
// flip or 1/16 scale), every vertex on joint 0: a held model (a shield)
// that the caller places, e.g. render.ShieldLocal times a hand transform.
func Mesh(boxes []Box) model.Mesh {
	var m model.Mesh
	for _, b := range boxes {
		for _, q := range boxQuads(b) {
			n := q.normal()
			if n == (mgl32.Vec3{}) && degenerate(q) {
				continue
			}
			base := uint32(len(m.Verts))
			for k := range 4 {
				m.Verts = append(m.Verts, model.Vertex{Pos: q.pos[k], Normal: n, UV: q.uv[k], Weights: [4]float32{1}})
			}
			m.Indices = append(m.Indices, base, base+1, base+2, base, base+2, base+3)
		}
	}
	return m
}

// ShieldBoxes is the shield model (64x64 texture): the plate, 12x22x1 at
// (-6, -11, -2) from 0, 0, and the handle, 2x6x6 at (-1, -3, -1) from
// 26, 0.
var ShieldBoxes = []Box{
	{Pos: [3]float32{-6, -11, -2}, Size: [3]int{12, 22, 1}, UV: [2]int{0, 0}, TexSize: [2]float32{64, 64}},
	{Pos: [3]float32{-1, -3, -1}, Size: [3]int{2, 6, 6}, UV: [2]int{26, 0}, TexSize: [2]float32{64, 64}},
}
