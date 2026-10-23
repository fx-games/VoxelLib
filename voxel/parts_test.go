package voxel

import "testing"

func TestParts(t *testing.T) {
	r := NewRegistry()
	arm := bx(7, 6, 0, 9, 9, 7)
	post := bx(6, 0, 6, 10, 16, 10)
	r.Register(&BlockDef{Name: "t:fence", Shape: ShapeBoxes, Boxes: []Box{post}, Tags: []string{"fence"},
		Parts: []Part{
			{When: []Cond{{Face: FaceNorth, Any: []TagTest{{Tag: "fence"}, {Tag: "opaque"}}}}, Boxes: []Box{arm}},
			{When: []Cond{{Face: FaceUp, Any: []TagTest{{Tag: "air", Not: true}}}}, Boxes: []Box{post}, Replace: true, Group: "g"},
			{When: []Cond{{Face: FaceUp, Any: []TagTest{{Tag: "air", Not: true}}}}, Boxes: []Box{arm, arm}, Group: "g"},
		}})
	r.Register(&BlockDef{Name: "t:stone", Shape: ShapeCube, Opaque: true})
	fence, stone := r.Info(r.State("t:fence", nil)), r.Info(r.State("t:stone", nil))
	air := r.Info(Air)
	nb := func(m map[Face]*StateInfo) func(Face) *StateInfo {
		return func(f Face) *StateInfo {
			if o := m[f]; o != nil {
				return o
			}
			return air
		}
	}
	if n := len(Connected(fence, nb(nil))); n != 1 {
		t.Errorf("alone: %d boxes, want 1", n)
	}
	if n := len(Connected(fence, nb(map[Face]*StateInfo{FaceNorth: fence}))); n != 2 {
		t.Errorf("fence north: %d boxes, want 2", n)
	}
	if n := len(Connected(fence, nb(map[Face]*StateInfo{FaceNorth: stone}))); n != 2 {
		t.Errorf("stone north: %d boxes, want 2", n)
	}
	if n := len(Connected(fence, nb(map[Face]*StateInfo{FaceNorth: stone, FaceUp: stone}))); n != 1 {
		t.Errorf("replace+group: %d boxes, want 1", n)
	}
}
