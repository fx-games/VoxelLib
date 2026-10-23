package app

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
)

// TestViewBob: distanceWalkedModified, cameraYaw/cameraPitch and the arm
// lag per tick, and setupViewBobbing's translation (with its
// -(dist + delta*partial) phase) against hand-worked numbers.
func TestViewBob(t *testing.T) {
	a := &App{}
	p := &Player{Character: character.New(0, 64, 0), a: a}
	p.PrevX, p.X, p.VX, p.OnGround = 0, 0.2, 0.2, true
	p.bobTick(0, 0, false)
	if math.Abs(p.walkDist-0.12) > 1e-12 || math.Abs(p.bobYaw-0.04) > 1e-12 || p.bobPitch != 0 {
		t.Fatalf("walk %v bobYaw %v bobPitch %v", p.walkDist, p.bobYaw, p.bobPitch)
	}
	if m := p.ViewBob(0); !m.ApproxEqual(mgl32.Ident4()) {
		t.Errorf("alpha 0 bob %v, want identity (prev cameraYaw 0)", m)
	}
	c := p.ViewBob(1).Col(3)
	f2 := -0.24 * math.Pi
	if math.Abs(float64(c[0])-math.Sin(f2)*0.02) > 1e-6 || math.Abs(float64(c[1])+math.Abs(math.Cos(f2))*0.04) > 1e-6 {
		t.Errorf("bob translation %v", c)
	}
	// Sneaking on the ground adds no distance; in the air cameraYaw decays
	// (0.064 after the sneak tick) and cameraPitch follows atan(-motionY*0.2)*15.
	p.PrevX, p.X = 0.2, 0.4
	p.bobTick(1, 0, true)
	if math.Abs(p.walkDist-0.12) > 1e-12 || math.Abs(p.armYaw-0.5) > 1e-12 {
		t.Fatalf("sneak walk %v armYaw %v", p.walkDist, p.armYaw)
	}
	p.OnGround, p.VY = false, -0.5
	p.bobTick(1, 0, false)
	want := math.Atan(0.1*1.0000000149011612) * 15 * 0.8
	if math.Abs(p.bobPitch-want) > 1e-9 || math.Abs(p.bobYaw-0.064*0.6) > 1e-12 {
		t.Errorf("air bobPitch %v want %v, bobYaw %v", p.bobPitch, want, p.bobYaw)
	}
	// Arm lag: yaw 1 rad against render arm 0.75 at alpha 1 -> 0.025 rad about Y.
	a.Cam.Yaw = 1
	p.prevBobYaw, p.bobYaw, p.prevBobPitch, p.bobPitch = 0, 0, 0, 0
	v := p.HandPose(1).View
	if w := mgl32.HomogRotate3DY(0.025); !v.ApproxEqualThreshold(w, 1e-6) {
		t.Errorf("arm lag %v, want %v", v, w)
	}
}
