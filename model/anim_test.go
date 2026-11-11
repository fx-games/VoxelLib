package model

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// A Blend layer halfway through a transition adds half of each clip.
func TestLayerRunFade(t *testing.T) {
	m := &Model{Bones: []Bone{{Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	clip := func(x float32) *Clip {
		return &Clip{Length: 1, Channels: []Channel{{Target: Translation, Additive: true,
			Track: &Keyframes{Times: []float32{0}, Values: []mgl32.Vec4{{x, 0, 0, 0}}}}}}
	}
	a := NewAnimator(m)
	l := Layer{Blend: true, Clip: clip(2)}
	l.Run(clip(4), 1)
	l.Time += 0.5
	a.Layers = []Layer{l}
	a.Skin(nil)
	if got := a.poses[0].t[0]; got != 3 {
		t.Fatalf("x = %v, want 3", got)
	}
}
