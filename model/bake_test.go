package model

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// TestBakeKeepsPoses: a Bedrock rig (Euler bones, additive channels) skins the
// same before and after Bake at the sampled frames.
func TestBakeKeepsPoses(t *testing.T) {
	m := &Model{Clips: map[string]*Clip{}, Bones: []Bone{
		{Name: "root", Parent: -1, S: mgl32.Vec3{1, 1, 1}, UseEuler: true, Euler: mgl32.Vec3{0, 0, -2.5}, InvBind: mgl32.Ident4()},
		{Name: "arm", Parent: 0, T: mgl32.Vec3{0, 2, 0}, S: mgl32.Vec3{1, 1, 1}, UseEuler: true, Euler: mgl32.Vec3{10, 0, 0}, InvBind: mgl32.Ident4()},
	}}
	kf := func(a, b mgl32.Vec4) *Keyframes {
		return &Keyframes{Times: []float32{0, 0.5}, Values: []mgl32.Vec4{a, b}, Interp: CatmullRom}
	}
	m.Clips["shoot"] = &Clip{Name: "shoot", Length: 0.5, Channels: []Channel{
		{Bone: 1, Target: Rotation, Additive: true, Track: kf(mgl32.Vec4{}, mgl32.Vec4{30, 45, 0, 0})},
		{Bone: 0, Target: Translation, Additive: true, Track: kf(mgl32.Vec4{}, mgl32.Vec4{0, 0, 1, 0})},
	}}
	skin := func(at float32) []mgl32.Mat4 {
		a := NewAnimator(m)
		a.Play("shoot", true)
		a.Time = at
		return a.Skin(nil)
	}
	times := []float32{0, 0.1, 0.25, 0.5}
	var want [][]mgl32.Mat4
	for _, at := range times {
		want = append(want, skin(at))
	}
	m.Bake(20)
	for i, at := range times {
		for b, g := range skin(at) {
			if !g.ApproxEqualThreshold(want[i][b], 1e-4) {
				t.Fatalf("t=%v bone %d: %v, want %v", at, b, g, want[i][b])
			}
		}
	}
}
