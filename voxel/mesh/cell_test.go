package mesh

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// cellVert is one decoded vertex: section-local position in 1/16 block,
// face, uv in 1/16 tile.
type cellVert struct {
	pos  [3]int
	face voxel.Face
	uv   [2]int
}

func decodeCell(data []uint32) []cellVert {
	var out []cellVert
	for i := 0; i+VertexWords <= len(data); i += VertexWords {
		w0, w1 := data[i], data[i+1]
		var v cellVert
		for a := range 3 {
			v.pos[a] = (int(w0>>(10*a)&1023)*4 + int(w1>>(26+2*a)&3) - 8*128) / 8
		}
		v.face = voxel.Face(w1 >> 22 & 7)
		v.uv = [2]int{int(w1 >> 12 & 31), int(w1 >> 17 & 31)}
		if w1&projectedUV == 0 {
			v.uv[0], v.uv[1] = v.uv[0]*16, v.uv[1]*16
		}
		out = append(out, v)
	}
	return out
}

// BuildCell meshes only the asked cell, culled by its real neighbours:
// the breaking overlay of RenderGlobal.drawBlockDamageTexture is the
// block's own geometry (renderBlockByRenderType) with RenderBlocks' UVs.
func TestBuildCell(t *testing.T) {
	table := []voxel.StateInfo{
		{},
		{Shape: voxel.ShapeCube, Opaque: true, Tex: [6]uint16{1, 1, 1, 1, 1, 1}},
		{Shape: voxel.ShapeBoxes, Tex: [6]uint16{2, 2, 2, 2, 2, 2}, Boxes: []voxel.Box{{Max: [3]float32{16, 8, 16}}}},
	}
	cfg := &Config{RenderBlocksUV: true}
	m := NewMesher()
	for i := range m.Snap.Light {
		m.Snap.Light[i] = 0xF0
	}
	// a stone column at x=5,z=5, y 3..5, and a slab at 8,4,8 on stone
	m.Snap.States[idx(5, 3, 5)], m.Snap.States[idx(5, 4, 5)], m.Snap.States[idx(5, 5, 5)] = 1, 1, 1
	m.Snap.States[idx(8, 3, 8)], m.Snap.States[idx(8, 4, 8)] = 1, 2

	cube := decodeCell(m.BuildCell(table, cfg, 5, 4, 5).Data)
	if len(cube) != 4*4 { // up and down hidden by the column
		t.Fatalf("cube: %d vertices, want 16", len(cube))
	}
	for _, v := range cube {
		if v.face == voxel.FaceUp || v.face == voxel.FaceDown {
			t.Fatalf("cube: face %d drawn against a neighbour", v.face)
		}
		for a := range 3 {
			if lo := []int{5, 4, 5}[a] * 16; v.pos[a] < lo || v.pos[a] > lo+16 {
				t.Fatalf("cube vertex %v outside its cell", v.pos)
			}
		}
		// side faces: v = 16 - y (RenderBlocks renderFaceZNeg..XPos d5/d6)
		if v.uv[1] != 16-(v.pos[1]-64) {
			t.Errorf("cube face %d at %v: v %d", v.face, v.pos, v.uv[1])
		}
	}

	slab := decodeCell(m.BuildCell(table, cfg, 8, 4, 8).Data)
	if len(slab) != 5*4 { // the bottom lies on the stone below
		t.Fatalf("slab: %d vertices, want 20", len(slab))
	}
	for _, v := range slab {
		y := v.pos[1] - 64
		switch v.face {
		case voxel.FaceDown:
			t.Fatal("slab bottom drawn on stone")
		case voxel.FaceUp:
			// renderFaceYPos: u = x, v = z within the block
			if y != 8 || v.uv != [2]int{v.pos[0] - 128, v.pos[2] - 128} {
				t.Errorf("slab top vertex %v uv %v", v.pos, v.uv)
			}
		default:
			if (y != 0 && y != 8) || v.uv[1] != 16-y { // v 8..16: the lower half of the icon
				t.Errorf("slab face %d vertex %v uv %v", v.face, v.pos, v.uv)
			}
		}
	}
	// a full build still meshes everything
	if n := len(m.Build(table, cfg).Data); n <= (len(cube)+len(slab))*VertexWords {
		t.Errorf("Build after BuildCell: %d words", n)
	}
}

// Block meshes a lone block at 0,0,0 with every face.
func TestBlock(t *testing.T) {
	info := voxel.StateInfo{Shape: voxel.ShapeCube, Opaque: true}
	if n := len(Block(info, voxel.StateInfo{}, &Config{}).Data) / VertexWords; n != 24 {
		t.Fatalf("lone cube: %d vertices, want 24", n)
	}
}
