package physics

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
)

// TestRaycastBox: a torch-sized box is hit where the ray enters it, rays
// beside it pass on to the block behind, and an origin inside a box hits
// its exit face (collisionRayTrace's behaviour for the start inside).
func TestRaycastBox(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, 0, 256)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	stone := reg.Default("stone")
	reg.Register(&voxel.BlockDef{Name: "test:torch", Shape: voxel.ShapeNone})
	torch := reg.Default("test:torch")
	w.SetBlock(5, 64, 5, torch)
	w.SetBlock(5, 64, 9, stone)
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		fx, fy, fz := float64(x), float64(y), float64(z)
		switch id {
		case torch: // BlockTorch meta 5: (.4,0,.4)-(.6,.6,.6)
			return append(buf, collide.AABB{MinX: fx + .4, MinY: fy, MinZ: fz + .4, MaxX: fx + .6, MaxY: fy + .6, MaxZ: fz + .6})
		case stone:
			return append(buf, collide.AABB{MinX: fx, MinY: fy, MinZ: fz, MaxX: fx + 1, MaxY: fy + 1, MaxZ: fz + 1})
		}
		return buf
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	// Along +z through the torch: enters its north face at z 5.4.
	h, ok := RaycastBox(w, 5.5, 64.3, 2.5, 0, 0, 1, 10, boxes)
	if !ok || h.Block != (voxel.BlockPos{X: 5, Y: 64, Z: 5}) || h.Face != voxel.FaceNorth || !near(h.Dist, 2.9) {
		t.Fatalf("torch hit %+v %v", h, ok)
	}
	// Above the torch box (y 64.8): through to the stone at z 9.
	h, ok = RaycastBox(w, 5.5, 64.8, 2.5, 0, 0, 1, 10, boxes)
	if !ok || h.Block != (voxel.BlockPos{X: 5, Y: 64, Z: 9}) || h.Face != voxel.FaceNorth || !near(h.Dist, 6.5) {
		t.Fatalf("stone hit %+v %v", h, ok)
	}
	// Out of reach.
	if _, ok = RaycastBox(w, 5.5, 64.8, 2.5, 0, 0, 1, 6, boxes); ok {
		t.Fatal("hit beyond maxDist")
	}
	// Down onto the torch top.
	h, ok = RaycastBox(w, 5.5, 70, 5.5, 0, -1, 0, 10, boxes)
	if !ok || h.Face != voxel.FaceUp || !near(h.Dist, 5.4) {
		t.Fatalf("top hit %+v %v", h, ok)
	}
	// From inside the torch box, going west: its west face at x 5.4.
	h, ok = RaycastBox(w, 5.5, 64.3, 5.5, -1, 0, 0, 10, boxes)
	if !ok || h.Face != voxel.FaceWest || !near(h.Dist, 0.1) {
		t.Fatalf("inside hit %+v %v", h, ok)
	}
}
