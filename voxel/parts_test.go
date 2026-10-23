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
	nb := func(m map[Face]*StateInfo) Nb {
		return faces(func(f Face) *StateInfo {
			if o := m[f]; o != nil {
				return o
			}
			return air
		})
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

// faces makes an Nb of a per-face lookup (no diagonal neighbours).
func faces(fn func(Face) *StateInfo) Nb {
	return func(d [3]int) *StateInfo {
		for f := Face(0); f < FaceCount; f++ {
			if f.Off() == d {
				return fn(f)
			}
		}
		panic("faces: diagonal neighbour")
	}
}

// A condition with Dy tests the diagonal neighbour above or below a side.
func TestDiagonalCond(t *testing.T) {
	r := NewRegistry()
	r.Register(&BlockDef{Name: "t:wire", Shape: ShapeBoxes, Boxes: []Box{bx(0, 0, 0, 16, 1, 16)}, Tags: []string{"wire"},
		Parts: []Part{{When: []Cond{{Face: FaceNorth, Dy: -1, Any: []TagTest{{Tag: "wire"}}}}, Boxes: []Box{bx(0, 0, 0, 16, 1, 1)}}}})
	wire, air := r.Info(r.State("t:wire", nil)), r.Info(Air)
	at := func(want [3]int) Nb {
		return func(d [3]int) *StateInfo {
			if d == want {
				return wire
			}
			return air
		}
	}
	if n := len(Connected(wire, at([3]int{0, -1, -1}))); n != 2 {
		t.Errorf("wire below north: %d boxes, want 2", n)
	}
	if n := len(Connected(wire, at([3]int{0, 0, -1}))); n != 1 {
		t.Errorf("wire north: %d boxes, want 1", n)
	}
}

// Set parts raise flags that later parts test: an any-of across neighbours.
func TestPartFlags(t *testing.T) {
	r := NewRegistry()
	flag := func(name string, not bool) []Cond {
		return []Cond{{Flag: true, Any: []TagTest{{Tag: name, Not: not}}}}
	}
	r.Register(&BlockDef{Name: "t:wire", Shape: ShapeBoxes, Boxes: []Box{}, Tags: []string{"wire"}, Parts: []Part{
		{When: []Cond{{Face: FaceWest, Any: []TagTest{{Tag: "wire"}}}}, Set: "x"},
		{When: []Cond{{Face: FaceEast, Any: []TagTest{{Tag: "wire"}}}}, Set: "x"},
		{When: flag("x", false), Boxes: []Box{bx(0, 0, 0, 16, 1, 16)}},
		{When: flag("x", true), Boxes: []Box{bx(5, 0, 5, 11, 1, 11)}, Collision: []Box{}},
	}})
	wire, air := r.Info(r.State("t:wire", nil)), r.Info(Air)
	for _, tc := range []struct {
		f    Face
		want float32
	}{{FaceEast, 0}, {FaceWest, 0}, {FaceNorth, 5}} {
		bs := Connected(wire, faces(func(g Face) *StateInfo {
			if g == tc.f {
				return wire
			}
			return air
		}))
		if len(bs) != 1 || bs[0].Min[0] != tc.want {
			t.Errorf("wire on %v: %v", tc.f, bs)
		}
	}
}
