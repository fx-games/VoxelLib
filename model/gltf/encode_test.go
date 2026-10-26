package gltf

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model"
)

// A two-bone model with a clip comes back from Encode and Load posed the same.
func TestEncodeRoundTrip(t *testing.T) {
	tex := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	tex.Set(1, 1, color.NRGBA{200, 10, 10, 255})
	m := &model.Model{Name: "rig", Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}
	m.Bones = []model.Bone{
		{Name: "root", Parent: -1, T: mgl32.Vec3{0, 1, 0}, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Translate3D(0, -1, 0)},
		{Name: "arm", Parent: 0, T: mgl32.Vec3{1, 0, 0}, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Translate3D(-1, -1, 0)},
	}
	v := func(x, y, z float32, b uint16) model.Vertex {
		return model.Vertex{Pos: mgl32.Vec3{x, y, z}, Normal: mgl32.Vec3{0, 1, 0}, UV: mgl32.Vec2{0.5, 0.5}, Joints: [4]uint16{b}, Weights: [4]float32{1}}
	}
	m.Meshes = []model.Mesh{{Verts: []model.Vertex{v(0, 1, 0, 0), v(1, 1, 0, 1), v(2, 1, 0, 1)}, Indices: []uint32{0, 1, 2}, Texture: 0}}
	q := mgl32.QuatRotate(1, mgl32.Vec3{0, 0, 1})
	m.Clips["wave"] = &model.Clip{Name: "wave", Length: 1, Channels: []model.Channel{{Bone: 1, Target: model.Rotation,
		Track: &model.Keyframes{Times: []float32{0, 1}, Values: []mgl32.Vec4{{0, 0, 0, 1}, {q.V[0], q.V[1], q.V[2], q.W}}, Quat: true}}}}

	data, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	g, err := Load(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Clips) != 1 || g.Clips["wave"] == nil || len(g.Textures) != 1 || len(g.Meshes) != 1 {
		t.Fatalf("clips %v textures %d meshes %d", g.ClipNames(), len(g.Textures), len(g.Meshes))
	}
	posed := func(m *model.Model) []mgl32.Vec3 {
		a := model.NewAnimator(m)
		a.Play("wave", true)
		a.Update(0.5)
		skin := a.Skin(nil)
		var out []mgl32.Vec3
		for _, v := range m.Meshes[0].Verts {
			out = append(out, mgl32.TransformCoordinate(v.Pos, skin[v.Joints[0]]))
		}
		return out
	}
	want, got := posed(m), posed(g)
	for i := range want {
		if !want[i].ApproxEqualThreshold(got[i], 1e-4) {
			t.Fatalf("vertex %d at %v, want %v", i, got[i], want[i])
		}
	}
}

// A model built from boxes (importer/cuboid) is written and loads back.
func TestWriteFileLoadsBack(t *testing.T) {
	m, err := cuboid.Build(&cuboid.Geometry{Name: "cube", TexSize: [2]int{16, 16}, Render: []int{0}, Parts: []cuboid.Part{
		{Name: "cube", Parent: -1, Boxes: []cuboid.Box{{Size: [3]int{4, 4, 4}, TexSize: [2]float32{16, 16}}}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sub", "cube.gltf")
	if err := WriteFile(path, m); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "cube" || len(got.Meshes) != len(m.Meshes) || len(got.Meshes[0].Verts) != len(m.Meshes[0].Verts) {
		t.Fatalf("loaded %q with %d meshes", got.Name, len(got.Meshes))
	}
}
