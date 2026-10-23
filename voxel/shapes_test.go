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
	got := Connected(stairs, faces(func(f Face) *StateInfo {
		if f == FaceEast {
			return north
		}
		return air
	}))
	if len(got) != 2 || !same(got[1], bx(8, 8, 0, 16, 16, 8)) {
		t.Fatalf("outer corner = %v", got)
	}
	fence := r.Info(r.Default("oak_fence"))
	stone := r.Info(r.Default("stone"))
	got = Connected(fence, faces(func(f Face) *StateInfo {
		if f == FaceNorth {
			return stone
		}
		return air
	}))
	if len(got) != 3 || got[1].Min[2] != 0 {
		t.Fatalf("fence arms = %v", got)
	}
	// two south-facing chests side by side become the halves of a double chest
	chest := r.Info(r.State("chest", map[string]string{"facing": "south"}))
	got = Connected(chest, faces(func(f Face) *StateInfo {
		if f == FaceWest {
			return chest
		}
		return air
	}))
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

// TestLockBoxes: a uvlocked y=90 turn of a position-textured box is an
// axis-aligned box with RenderBlocks UVs of its new bounds; the north
// face's texture ends up east, as a blockstate y=90 turns it.
func TestLockBoxes(t *testing.T) {
	b := Box{Min: [3]float32{0, 0, 6}, Max: [3]float32{16, 16, 10}}
	uv := RenderBlocksUV(b.Min, b.Max, false)
	b.UV = &uv
	b.Tex[FaceNorth] = "n"
	out := LockBoxes([]Box{b}, 1, 90)[0]
	if len(out.Rot) != 0 || out.Min != [3]float32{6, 0, 0} || out.Max != [3]float32{10, 16, 16} {
		t.Fatalf("locked box %v..%v rot %v", out.Min, out.Max, out.Rot)
	}
	if *out.UV != RenderBlocksUV(out.Min, out.Max, false) {
		t.Errorf("uv %v", *out.UV)
	}
	if out.Tex[FaceEast] != "n" {
		t.Errorf("north texture went to %v", out.Tex)
	}
	b.UVRot[FaceUp] = 1 // not laid out by position: turns with the box
	if out := LockBoxes([]Box{b}, 1, 90)[0]; len(out.Rot) != 1 {
		t.Error("custom-UV box was baked")
	}
}

// TestRotShear: renderTorchAtAngle's lean, full at the foot, none at Origin.y.
func TestRotShear(t *testing.T) {
	r := []Rot{{Axis: 1, Origin: [3]float32{8, 19.2, 8}, Shear: [2]float32{-6.4, 0}}}
	for _, c := range []struct{ in, want [3]float32 }{
		{[3]float32{7, 3.2, 0}, [3]float32{0.6, 3.2, 0}},   // foot: -0.4 block
		{[3]float32{7, 19.2, 16}, [3]float32{7, 19.2, 16}}, // top: none
		{[3]float32{7, 13.2, 7}, [3]float32{4.6, 13.2, 7}}, // cap at 0.625: -0.4*(1-0.625)
	} {
		got := RotatePoint(c.in, r)
		for a := range 3 {
			if d := got[a] - c.want[a]; d > 1e-4 || d < -1e-4 {
				t.Errorf("%v -> %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}
