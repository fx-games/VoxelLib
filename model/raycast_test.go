package model

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// quad is one bone's unit square in the XY plane at z, facing +Z.
func quad(me *Mesh, z float32, bone uint8) {
	base := uint32(len(me.Verts))
	for _, p := range []mgl32.Vec3{{0, 0, z}, {1, 0, z}, {1, 1, z}, {0, 1, z}} {
		me.Verts = append(me.Verts, Vertex{Pos: p, Joints: [4]uint8{bone}, Weights: [4]float32{1}})
	}
	me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
}

func TestRaycastPosed(t *testing.T) {
	m := &Model{Bones: []Bone{{Name: "body", Parent: -1}, {Name: "head", Parent: 0}}}
	var me Mesh
	quad(&me, 0, 0) // body at z=0
	quad(&me, 1, 1) // head at z=1, nearer a ray coming from +Z
	m.Meshes = []Mesh{me}
	o, d := mgl32.Vec3{0.5, 0.5, 5}, mgl32.Vec3{0, 0, -1}
	h, ok := m.Raycast(nil, o, d, 10)
	if !ok || h.Bone != 1 || h.T != 4 {
		t.Fatalf("bind pose: want head at t=4, got %+v ok=%v", h, ok)
	}
	// Pose the head 2 up, out of the ray: now the body is hit.
	skin := []mgl32.Mat4{mgl32.Ident4(), mgl32.Translate3D(0, 2, 0)}
	h, ok = m.Raycast(skin, o, d, 10)
	if !ok || h.Bone != 0 || h.T != 5 {
		t.Fatalf("posed: want body at t=5, got %+v ok=%v", h, ok)
	}
	if _, ok = m.Raycast(skin, o, d, 4.5); ok {
		t.Error("hit beyond maxT")
	}
}
