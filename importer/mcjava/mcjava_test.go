package mcjava

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// biped is ModelBiped's head and right arm.
func biped() *Geometry {
	return &Geometry{Name: "biped", TexSize: [2]int{64, 32}, Render: []int{0, 1}, Parts: []Part{
		{Name: "head", Parent: -1, Boxes: []Box{{Pos: [3]float32{-4, -8, -4}, Size: [3]int{8, 8, 8}, TexSize: [2]float32{64, 32}}}},
		{Name: "rightArm", Parent: -1, Pivot: [3]float32{-5, 2, 0},
			Boxes: []Box{{Pos: [3]float32{-3, -2, -2}, Size: [3]int{4, 12, 4}, UV: [2]int{40, 16}, TexSize: [2]float32{64, 32}}}},
		{Name: "ear", Parent: -1, Boxes: []Box{{Size: [3]int{1, 1, 1}, TexSize: [2]float32{64, 32}}}},
	}}
}

func near(a, b mgl32.Vec3) bool { return a.Sub(b).Len() < 1e-5 }

func matNear(a, b mgl32.Mat4) bool {
	for i := range a {
		if d := a[i] - b[i]; d > 1e-5 || d < -1e-5 {
			return false
		}
	}
	return true
}

func TestBuildPlacesBoxes(t *testing.T) {
	g := biped()
	m, err := Build(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := m.Bounds()
	// Head top at y=-8px → 1.5078125+0.5 blocks; feet of the arm at y=14px.
	if !near(hi, mgl32.Vec3{0.25, Lift + 0.5, 0.25}) {
		t.Errorf("max %v", hi)
	}
	if !near(lo, mgl32.Vec3{-0.5, Lift - 12.0/16, -0.25}) {
		t.Errorf("min %v", lo)
	}
	// 2 drawn boxes × 6 quads × 4 vertices; the ear is not rendered.
	if n := len(m.Meshes[0].Verts); n != 48 {
		t.Errorf("%d verts", n)
	}
	// The front face (Minecraft north, -Z) of the head faces +Z with UV
	// (8,8)-(16,16).
	found := false
	for _, v := range m.Meshes[0].Verts[:24] {
		if near(v.Normal, mgl32.Vec3{0, 0, 1}) && v.UV == (mgl32.Vec2{16.0 / 64, 8.0 / 32}) {
			found = near(v.Pos, mgl32.Vec3{0.25, Lift + 0.5, 0.25}) || found
		}
	}
	if !found {
		t.Error("head front face misplaced")
	}
}

func TestRestPoseSkinIsIdentity(t *testing.T) {
	g := biped()
	g.Parts[1].Rot = [3]float32{0.3, -0.2, 0.1}
	m, _ := Build(g, nil)
	p := g.RestPose()
	for i, s := range p.Skin(m, g, nil) {
		if !matNear(s, mgl32.Ident4()) {
			t.Fatalf("bone %d: %v", i, s)
		}
	}
}

func TestPoseRotatesArm(t *testing.T) {
	g := biped()
	m, _ := Build(g, nil)
	p := g.RestPose()
	// rotateAngleX = -90°: the arm points forward (Minecraft -Z, model +Z).
	p.Parts[1].Rot[0] = -math.Pi / 2
	skin := p.Skin(m, g, nil)
	// The arm's hand end (y=10px below the pivot) moves to z=+10px.
	hand := mgl32.Vec3{-5.0 / 16, Lift - 12.0/16, 0}
	got := mgl32.TransformCoordinate(hand, skin[1])
	want := mgl32.Vec3{-5.0 / 16, Lift - 2.0/16, 10.0 / 16}
	if !near(got, want) {
		t.Errorf("hand at %v, want %v", got, want)
	}
	p.Parts[1].Hidden = true
	if s := p.Skin(m, g, nil)[1]; mgl32.TransformCoordinate(hand, s).Len() != 0 {
		t.Error("hidden part still drawn")
	}
}

func TestMirrorSwapsXAndFlipsQuads(t *testing.T) {
	b := Box{Pos: [3]float32{0, 0, 0}, Size: [3]int{2, 3, 4}, TexSize: [2]float32{64, 32}}
	q := boxQuads(b)
	b.Mirror = true
	mq := boxQuads(b)
	// ModelBox mirror: quad 0 (x+ face, UV of the east side) now sits at x=0.
	if q[0].pos[0][0] != 2 || mq[0].pos[3][0] != 0 {
		t.Errorf("mirror: %v / %v", q[0].pos, mq[0].pos)
	}
	if mq[0].uv[0] != q[0].uv[3] {
		t.Error("flipFace must reverse UVs with the vertices")
	}
}

func TestCopy(t *testing.T) {
	g := &Geometry{Name: "t", Parts: []Part{{Name: "a", Parent: -1}, {Name: "b", Parent: 0}, {Name: "c", Parent: -1}}, Render: []int{0, 2}}
	n := g.Copy(0, -1)
	if n != 3 || len(g.Parts) != 5 || g.Parts[4].Parent != 3 || g.Parts[4].Same != 2 || g.Parts[3].Same != 1 || g.Render[2] != 3 {
		t.Fatalf("%+v", g)
	}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestItem2D(t *testing.T) {
	m := mgl32.Ident4()
	q := boxQuads(Box{Item2D: &Item2D{UV: [4]float32{0, 0, 0.125, 0.125}, W: 8, H: 4, Depth: 0.0625, M: m}})
	if len(q) != 2+2*8+2*4 {
		t.Fatalf("%d quads", len(q))
	}
	// The front quad spans the unit square, in pixels.
	if q[0].pos[2] != (mgl32.Vec3{16, 16, 0}) || q[1].pos[0][2] != -1 {
		t.Fatalf("%v %v", q[0].pos, q[1].pos)
	}
}
