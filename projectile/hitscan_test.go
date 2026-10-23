package projectile

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
)

type fakeBlocks struct{}

func (fakeBlocks) Block(x, y, z int) voxel.StateID { return voxel.StateID(1) }

// wall: a full block at x=5 only (state ids are ignored, position decides).
func wallBoxes(x, y, z int, _ voxel.StateID, buf []collide.AABB) []collide.AABB {
	if x == 5 && y == 0 && z == 0 {
		buf = append(buf, collide.Block(x, y, z))
	}
	return buf
}

type boxT struct{ b collide.AABB }

func (t boxT) Box() collide.AABB             { return t.b }
func (t boxT) Frame() (x, y, z, yaw float64) { return 0, 0, 0, 0 }

type shapeT struct {
	boxT
	at float64 // x of a thin wall
}

func (s shapeT) Intersect(x0, y0, z0, x1, y1, z1 float64) (float64, float64, float64, int, bool) {
	if (x0-s.at)*(x1-s.at) <= 0 && x0 != x1 {
		f := (s.at - x0) / (x1 - x0)
		return s.at, y0 + f*(y1-y0), z0 + f*(z1-z0), 7, true
	}
	return 0, 0, 0, -1, false
}

func TestHitscan(t *testing.T) {
	o, d := [3]float64{0.5, 0.5, 0.5}, [3]float64{2, 0, 0} // unnormalized on purpose
	front := boxT{mk(2, 0, 0, 3, 1, 1)}
	behind := boxT{mk(8, 0, 0, 9, 1, 1)}

	h, ok := Hitscan(fakeBlocks{}, wallBoxes, o, d, 20, []Target{behind})
	if !ok || !h.Block || h.Dist != 4.5 || h.Face.Normal().X != -1 || h.Normal != ([3]float64{-1, 0, 0}) {
		t.Fatalf("block: %+v %v", h, ok)
	}
	h, ok = Hitscan(fakeBlocks{}, wallBoxes, o, d, 20, []Target{behind, front})
	if !ok || h.Block || h.Target != Target(front) || h.Dist != 1.5 || h.Normal != ([3]float64{-1, 0, 0}) || h.Part != -1 {
		t.Fatalf("box target in front of block: %+v", h)
	}
	s := shapeT{boxT{}, 3}
	h, ok = Hitscan(fakeBlocks{}, wallBoxes, o, d, 20, []Target{s, front})
	if !ok || h.Target != Target(front) {
		t.Fatalf("nearest wins: %+v", h)
	}
	h, _ = Hitscan(fakeBlocks{}, wallBoxes, o, d, 20, []Target{s})
	if h.Target != Target(s) || h.Part != 7 || h.X != 3 || h.Dist != 2.5 {
		t.Fatalf("shape: %+v", h)
	}
	if _, ok = Hitscan(fakeBlocks{}, wallBoxes, o, d, 3, []Target{behind}); ok {
		t.Fatal("hit beyond maxDist")
	}
	if _, ok = Hitscan(fakeBlocks{}, wallBoxes, o, d, 20, []Target{boxT{mk(0, 0, 0, 1, 1, 1)}}); !ok {
		t.Fatal("expected block hit; start-inside target is passed through")
	} else if h, _ := Hitscan(fakeBlocks{}, wallBoxes, o, d, 20, []Target{boxT{mk(0, 0, 0, 1, 1, 1)}}); !h.Block {
		t.Fatal("started inside a box target: should pass through")
	}
}

func mk(x0, y0, z0, x1, y1, z1 float64) collide.AABB {
	return collide.AABB{MinX: x0, MinY: y0, MinZ: z0, MaxX: x1, MaxY: y1, MaxZ: z1}
}
