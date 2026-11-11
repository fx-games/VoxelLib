package voxel

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

func same(a, b Box) bool { return a.Min == b.Min && a.Max == b.Max }

// TestStateBoxesCube: a full cube's ray box is its cell (bullets, sight and
// melee stop at stone and grass), air has none.
func TestStateBoxesCube(t *testing.T) {
	r := NewRegistry()
	w := NewWorld(r, 0, 16)
	got := w.StateBoxes(r.State("stone", nil), 2, 3, -4, nil)
	if want := (collide.AABB{MinX: 2, MinY: 3, MinZ: -4, MaxX: 3, MaxY: 4, MaxZ: -3}); len(got) != 1 || got[0] != want {
		t.Fatalf("stone: %v, want %v", got, want)
	}
	if got := w.StateBoxes(Air, 0, 0, 0, nil); len(got) != 0 {
		t.Fatalf("air: %v", got)
	}
}

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

// A client's own stairs: no full-cube collision, a Step the pathfinder
// stands on, and a slab plus the step to walk up.
func TestClientStairsAreSteps(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Stairs("mossy_stairs", "block/mossy_cobblestone"))
	w := NewWorld(reg, -64, 384)
	c := w.NewChunk(ChunkPos{0, 0})
	c.SetRaw(2, 0, 2, reg.Parse("mossy_stairs[facing=east,half=bottom]"))
	w.AddChunk(c)
	info := reg.Info(w.Block(2, 0, 2))
	if info.Collides || !info.Step || !w.Footing(2, 0, 2) || w.Footing(2, 1, 2) {
		t.Fatalf("collides %v step %v footing %v", info.Collides, info.Step, w.Footing(2, 0, 2))
	}
	top := 0.0
	for _, b := range w.Boxes(collide.AABB{MinX: 2, MinY: 0, MinZ: 2, MaxX: 3, MaxY: 1, MaxZ: 3}, nil) {
		if b.MaxY > 0.75 && b.MinX < 2.5 {
			t.Fatalf("the high step is on the west: %+v", b)
		}
		top = max(top, b.MaxY)
	}
	if top != 1 {
		t.Fatalf("stair top %v", top)
	}
}

// A client's own ladder: a plate against the wall behind it that bodies
// climb.
func TestClientLadderClimbs(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Ladder("rope_ladder", ""))
	w := NewWorld(reg, -64, 384)
	c := w.NewChunk(ChunkPos{0, 0})
	c.SetRaw(2, 0, 2, reg.Parse("rope_ladder[facing=north]"))
	c.SetRaw(2, 1, 2, reg.Parse("oak_stairs[facing=east,half=bottom]"))
	w.AddChunk(c)
	info := reg.Info(w.Block(2, 0, 2))
	if !w.Climbable(2, 0, 2) || w.Climbable(2, 1, 2) || info.Collides || info.Tex[0] != reg.Info(reg.Parse("ladder[facing=north]")).Tex[0] {
		t.Fatalf("climbable %v collides %v", w.Climbable(2, 0, 2), info.Collides)
	}
	for _, b := range w.Boxes(collide.AABB{MinX: 2, MinY: 0, MinZ: 2, MaxX: 3, MaxY: 1, MaxZ: 3}, nil) {
		if b.MinY < 1 && b.MinZ < 2.9 {
			t.Fatalf("facing north hugs the south side: %+v", b)
		}
	}
	if !w.StairOrSlab(2, 1, 2) || !w.Air(2, 2, 2) || w.Air(2, 1, 2) {
		t.Fatal("StepBlocks")
	}
}

func TestSpikesPointOutOfTheWall(t *testing.T) {
	for facing, axis := range map[string]int{"north": 2, "south": 2, "east": 0, "west": 0} {
		bs := spikes(facing)
		plate, point := bs[0], bs[1]
		if plate.Max[axis]-plate.Min[axis] != 2 || point.Max[axis]-point.Min[axis] != 8 {
			t.Fatalf("%s: plate %v point %v", facing, plate, point)
		}
		out := map[string]bool{"south": true, "east": true}[facing] // points toward +axis
		if (point.Min[axis] > plate.Min[axis]) != out {
			t.Fatalf("%s: points %v go the wrong way from plate %v", facing, point, plate)
		}
	}
	if !IsSpikes("minecraft:iron_spikes[facing=east]") || IsSpikes("ladder") {
		t.Fatal("IsSpikes")
	}
}
