package mesh

import (
	"fmt"
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// TestConnectOffsets: the in-plane neighbour "left" lies toward u=0 and "top"
// toward v=0 of RenderBlocks' mapping of every face, so a connected icon's
// edges sit on the side of the neighbour that removes them.
func TestConnectOffsets(t *testing.T) {
	for f := voxel.Face(0); f < voxel.FaceCount; f++ {
		var at [2][2][3]int // [u][v] corner position
		for k, uv := range rbTab[f][0] {
			at[uv[0]][uv[1]] = cornerOff[f][k]
		}
		var du, dv [3]int
		for a := range 3 {
			du[a] = at[1][0][a] - at[0][0][a]
			dv[a] = at[0][1][a] - at[0][0][a]
		}
		neg := func(d [3]int) [3]int { return [3]int{-d[0], -d[1], -d[2]} }
		o := voxel.ConnectOffsets[f]
		if o[3] != neg(du) || o[4] != du || o[1] != neg(dv) || o[6] != dv {
			t.Errorf("face %v: left/right/top/bottom %v %v %v %v, RenderBlocks u %v v %v", f, o[3], o[4], o[1], o[6], du, dv)
		}
	}
}

// TestConnect: a row of three connected blocks; each face shows the icon of
// its in-plane mask, the north face mirrored against the south.
func TestConnect(t *testing.T) {
	reg := voxel.NewRegistry()
	c := &voxel.Connect{Tag: "c", Plane: -1}
	for m := range 256 {
		c.Icons = append(c.Icons, fmt.Sprint("m", m))
		c.Mask[m] = uint8(m)
	}
	reg.Register(&voxel.BlockDef{Name: "test:ct", Shape: voxel.ShapeCube, Opaque: true, LightOpacity: 15,
		Textures: [6]string{"x", "x", "x", "x", "x", "x"}, Tags: []string{"c"}, Connect: c})
	w := voxel.NewWorld(reg, 0, 64)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	for x := 1; x <= 3; x++ {
		w.SetBlock(x, 1, 1, reg.State("test:ct", nil))
	}
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	m.table = reg.Table()
	info := reg.Info(reg.State("test:ct", nil))
	for _, tc := range []struct {
		x    int
		f    voxel.Face
		mask int
	}{{2, voxel.FaceSouth, 24}, {2, voxel.FaceNorth, 24}, {2, voxel.FaceUp, 24},
		{1, voxel.FaceSouth, 16}, {1, voxel.FaceNorth, 8}, {3, voxel.FaceSouth, 8}, {1, voxel.FaceWest, 0}} {
		got := m.connectTex(info, idx(tc.x, 1, 1), tc.f)
		if want := reg.Texture(fmt.Sprint("m", tc.mask)); got != want {
			t.Errorf("x=%d %v: texture %d, want m%d (%d)", tc.x, tc.f, got, tc.mask, want)
		}
	}
	// Plane and Below: a thin gate in the x-y plane (Plane z) open over air.
	g := *c
	g.Plane, g.Below = 2, "c"
	reg.Register(&voxel.BlockDef{Name: "test:gate", Shape: voxel.ShapeBoxes, Connect: &g})
	ci := reg.Info(reg.State("test:gate", nil)).Connect
	air := reg.Info(voxel.Air)
	if ci.Joins([3]int{0, 0, 1}, info) || !ci.Joins([3]int{1, -1, 0}, air) || ci.Joins([3]int{1, 1, 0}, air) {
		t.Error("Plane/Below rules")
	}
}
