package character

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

// solid: 1 a full block, 2 web (isSolid, not blocksMovement), 3 ice
type flowWorld struct{ meta, solid map[[3]int]int }

func (f flowWorld) FlowMaterial(x, y, z int) (bool, bool, bool) {
	switch f.solid[[3]int{x, y, z}] {
	case 2:
		return false, true, false
	case 3:
		return true, true, true
	}
	s := f.Solid(x, y, z)
	return s, s, false
}

func (f flowWorld) Solid(x, y, z int) bool { _, ok := f.solid[[3]int{x, y, z}]; return ok || y < 0 }
func (f flowWorld) Liquid(x, y, z int) Liquid {
	if _, ok := f.meta[[3]int{x, y, z}]; ok {
		return LiquidWater
	}
	return LiquidNone
}
func (f flowWorld) LiquidMeta(x, y, z int) int { return f.meta[[3]int{x, y, z}] }

func TestWaterPush(t *testing.T) {
	box := collide.AABB{MinX: 1.2, MinY: 0, MinZ: 0.2, MaxX: 1.8, MaxY: 1.8, MaxZ: 0.8}
	// source, level 1, level 2 in a row: the current runs +x
	w := flowWorld{meta: map[[3]int]int{{0, 0, 0}: 0, {1, 0, 0}: 1, {2, 0, 0}: 2}}
	if in, vx, vy, vz := WaterPush(w, box); !in || vx != 0.014 || vy != 0 || vz != 0 {
		t.Fatalf("flowing: %v %v %v %v", in, vx, vy, vz)
	}
	// falling water against a wall: straight down
	w = flowWorld{meta: map[[3]int]int{{0, 0, 0}: 0, {1, 0, 0}: 8}, solid: map[[3]int]int{{2, 0, 0}: 1}}
	if _, vx, vy, vz := WaterPush(w, box); vx != 0 || vy != -0.014 || vz != 0 {
		t.Fatalf("falling: %v %v %v", vx, vy, vz)
	}
	// a web beside falling water turns it down too (isSolid), ice does not
	w.solid[[3]int{2, 0, 0}] = 2
	if _, vx, vy, vz := WaterPush(w, box); vx != 0 || vy != -0.014 || vz != 0 {
		t.Fatalf("web: %v %v %v", vx, vy, vz)
	}
	w.solid[[3]int{2, 0, 0}] = 3
	if _, vx, vy, vz := WaterPush(w, box); vx != 0 || vy != 0 || vz != 0 {
		t.Fatalf("ice: %v %v %v", vx, vy, vz)
	}
	// a web does not block the flow over an edge: level 1 beside a web over
	// water at level 3 below runs toward it (3 - (1-8) = 10)
	w = flowWorld{meta: map[[3]int]int{{1, 0, 0}: 1, {2, -1, 0}: 3}, solid: map[[3]int]int{{2, 0, 0}: 2}}
	if _, vx, _, _ := WaterPush(w, box); vx != 0.014 {
		t.Fatalf("web edge: %v", vx)
	}
	w.solid[[3]int{2, 0, 0}] = 1
	if _, vx, _, _ := WaterPush(w, box); vx != 0 {
		t.Fatalf("block edge: %v", vx)
	}
	// still sources: in water, no push
	w = flowWorld{meta: map[[3]int]int{{0, 0, 0}: 0, {1, 0, 0}: 0, {2, 0, 0}: 0}}
	if in, vx, vy, vz := WaterPush(w, box); !in || vx != 0 || vy != 0 || vz != 0 {
		t.Fatalf("still: %v %v %v %v", in, vx, vy, vz)
	}
}
