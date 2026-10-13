package voxel

import "testing"

func same(a, b Box) bool { return a.Min == b.Min && a.Max == b.Max }

func TestShapeBoxes(t *testing.T) {
	r := NewRegistry()
	slab := r.Info(r.State("smooth_stone_slab", map[string]string{"type": "top"}))
	if slab.Shape != ShapeBoxes || !same(slab.Boxes[0], bx(0, 8, 0, 16, 16, 16)) {
		t.Fatalf("top slab = %v", slab.Boxes)
	}
	// open door with a left hinge, facing north, swings to the east side
	door := r.Info(r.State("oak_door", map[string]string{"facing": "north", "open": "true", "hinge": "left", "half": "upper"}))
	if !same(door.Boxes[0], thin("east", 3)) || texName(r, door.Tex[0]) != "block/oak_door_top" {
		t.Fatalf("door = %v %q", door.Boxes, texName(r, door.Tex[0]))
	}
	stairs := r.Info(r.State("oak_stairs", map[string]string{"facing": "east", "half": "bottom"}))
	if len(stairs.Boxes) != 2 || !same(stairs.Boxes[1], bx(8, 8, 0, 16, 16, 16)) {
		t.Fatalf("stairs = %v", stairs.Boxes)
	}
	// a north-facing stair in front of an east-facing one makes an outer corner
	north := r.Info(r.State("oak_stairs", map[string]string{"facing": "north", "half": "bottom"}))
	air := r.Info(Air)
	got := Connected(stairs, func(f Face) *StateInfo {
		if f == FaceEast {
			return north
		}
		return air
	})
	if len(got) != 2 || !same(got[1], bx(8, 8, 0, 16, 16, 8)) {
		t.Fatalf("outer corner = %v", got)
	}
	fence := r.Info(r.Default("oak_fence"))
	stone := r.Info(r.Default("stone"))
	got = Connected(fence, func(f Face) *StateInfo {
		if f == FaceNorth {
			return stone
		}
		return air
	})
	if len(got) != 3 || got[1].Min[2] != 0 {
		t.Fatalf("fence arms = %v", got)
	}
	// two south-facing chests side by side become the halves of a double chest
	chest := r.Info(r.State("chest", map[string]string{"facing": "south"}))
	got = Connected(chest, func(f Face) *StateInfo {
		if f == FaceWest {
			return chest
		}
		return air
	})
	if !same(got[0], bx(0, 0, 1, 15, 10, 15)) {
		t.Fatalf("left chest half = %v", got[0])
	}
	// the wall torch's top leans away from the wall (east)
	wt := r.Info(r.State("wall_torch", map[string]string{"facing": "east"}))
	top := RotatePoint([3]float32{0, 13.5, 8}, wt.Boxes[0].Rot)
	if top[0] < 3 || top[0] > 4.5 {
		t.Fatalf("wall torch top at x=%v", top[0])
	}
}

func texName(r *Registry, i uint16) string { n, _ := r.TextureNames(); return n[i] }

func TestWallSignHangsOnWall(t *testing.T) {
	// facing north: text faces north, so the board backs onto the south wall.
	b := wallSign("north")[0]
	if p := RotatePoint(b.Min, b.Rot); p[2] < 15.5 { // vanilla leaves a 1/3 px gap
		t.Fatalf("board back not against the south wall: %v", p)
	}
}
