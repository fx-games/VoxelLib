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
	l.Run(clip(4), LoopOnce, 1)
	l.Time += 0.5
	a.Layers = []Layer{l}
	a.Skin(nil)
	if got := a.poses[0].t[0]; got != 3 {
		t.Fatalf("x = %v, want 3", got)
	}
}

// A layer's play type, not its clip's loop flag, decides when it ends:
// a looping clip run once is Done (TACZ PLAY_ONCE_STOP), a repeat wraps.
func TestLayerPlayOverridesClipLoop(t *testing.T) {
	once := Layer{Clip: &Clip{Length: 1, Loop: LoopRepeat}, Play: LoopOnce}
	once.Advance(1.5)
	if !once.Done() {
		t.Fatal("LoopOnce layer of a looping clip not Done past its end")
	}
	rep := Layer{Clip: &Clip{Length: 1, Loop: LoopOnce}, Play: LoopRepeat}
	rep.Advance(1.5)
	if rep.Done() || rep.Time != 0.5 {
		t.Fatalf("LoopRepeat layer: Done %v, Time %v; want false, 0.5", rep.Done(), rep.Time)
	}
}
