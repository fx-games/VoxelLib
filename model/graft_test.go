package model

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func oneBone(name string, t mgl32.Vec3, v mgl32.Vec3) *Model {
	m := &Model{Name: name, Bones: []Bone{{Name: name, Parent: -1, T: t, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}}}}
	m.Bones[0].InvBind = m.BindWorld()[0].Inv()
	m.Meshes = []Mesh{{Verts: []Vertex{{Pos: v, Weights: [4]float32{1}}}, Indices: []uint32{0, 0, 0}, Texture: -1}}
	return m
}

// TestGraft: a grafted vertex sits at the parent's frame plus its offset in
// the sub-model, stays put in the bind pose and follows the parent's turn.
func TestGraft(t *testing.T) {
	gun := oneBone("slot", mgl32.Vec3{1, 2, 3}, mgl32.Vec3{})
	gun.Transform(mgl32.Scale3D(2, 2, 2)) // a baked root must not matter
	sub := oneBone("scope", mgl32.Vec3{0, 1, 0}, mgl32.Vec3{0, 1, 1})
	if err := gun.Graft(0, sub); err != nil {
		t.Fatal(err)
	}
	v := gun.Meshes[1].Verts[0]
	if want := (mgl32.Vec3{2, 6, 8}); !v.Pos.ApproxEqual(want) {
		t.Fatalf("grafted vertex at %v, want %v", v.Pos, want)
	}
	skin := NewAnimator(gun).Skin(nil)
	if p := mgl32.TransformCoordinate(v.Pos, skin[v.Joints[0]]); !p.ApproxEqual(v.Pos) {
		t.Fatalf("bind pose moves it to %v", p)
	}
	w := gun.BindWorld()
	w[0] = w[0].Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(90)))
	w[1] = w[0].Mul4(mgl32.Translate3D(0, 1, 0))
	sk := gun.SkinWorld(w, nil)
	if p, want := mgl32.TransformCoordinate(v.Pos, sk[1]), (mgl32.Vec3{4, 6, 6}); !p.ApproxEqualThreshold(want, 1e-4) {
		t.Fatalf("turned slot: vertex at %v, want %v", p, want)
	}
}
