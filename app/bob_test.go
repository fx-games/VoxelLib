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
	p.bobTick(0, 0, false, false)
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
	p.bobTick(1, 0, true, false)
	if math.Abs(p.walkDist-0.12) > 1e-12 || math.Abs(p.armYaw-0.5) > 1e-12 {
		t.Fatalf("sneak walk %v armYaw %v", p.walkDist, p.armYaw)
	}
	p.OnGround, p.VY = false, -0.5
	p.bobTick(1, 0, false, false)
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

// TestViewBobRideDead: riding, the walk stands, cameraYaw ends the tick
// at 0 with the decayed value as prev and cameraPitch decays to 0; dead
// on the ground, cameraYaw decays though the player moves.
func TestViewBobRideDead(t *testing.T) {
	p := &Player{Character: character.New(0, 64, 0), a: &App{}}
	p.walkDist, p.bobYaw, p.bobPitch = 1, 0.1, 2
	p.PrevX, p.X, p.VX, p.VY, p.OnGround = 0, 0.2, 0.2, -0.5, false
	p.bobTick(0, 0, false, true)
	if p.walkDist != 1 || p.prevWalk != 1 || p.bobYaw != 0 || math.Abs(p.prevBobYaw-0.06) > 1e-12 || math.Abs(p.bobPitch-0.4) > 1e-12 {
		t.Fatalf("riding: walk %v bobYaw %v prev %v pitch %v", p.walkDist, p.bobYaw, p.prevBobYaw, p.bobPitch)
	}
	p.bobYaw, p.OnGround, p.Dead = 0.1, true, true
	p.bobTick(0, 0, false, false)
	if math.Abs(p.bobYaw-0.06) > 1e-12 {
		t.Fatalf("dead bobYaw %v", p.bobYaw)
	}
}

func TestCamEyeSmooths(t *testing.T) {
	p := &Player{Character: character.New(0, 64, 0), a: &App{}}
	p.eyeTick()
	p.EyeHeight = character.PlayerCrouchEyeHeight
	p.eyeTick()
	want := character.PlayerEyeHeight + (character.PlayerCrouchEyeHeight-character.PlayerEyeHeight)*camEyeLerp
	if math.Abs(p.CamEye(1)-want) > 1e-12 || p.CamEye(0) != character.PlayerEyeHeight {
		t.Fatalf("cam eye %v..%v, want %v", p.CamEye(0), p.CamEye(1), want)
	}
}
