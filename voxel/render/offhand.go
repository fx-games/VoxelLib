package render

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/respack"
)

// ArmTransform is applyItemArmTransform: translate(±0.56, -0.52 -
// lowered*0.6, -0.72), x negated for the left arm; lowered is 1 minus the
// hand's equip height (0 when fully raised).
func ArmTransform(left bool, lowered float32) mgl32.Mat4 {
	x := float32(0.56)
	if left {
		x = -x
	}
	return mgl32.Translate3D(x, -0.52-lowered*0.6, -0.72)
}

// FirstPersonShield places the shield mesh in first person:
// ArmTransform, the hand's display entry (raised or held) and
// respack.ShieldLocal. A raised shield gets no swing.
func FirstPersonShield(left, raised bool, lowered float32) mgl32.Mat4 {
	d := respack.Shield
	if raised {
		d = respack.ShieldBlocking
	}
	return ArmTransform(left, lowered).Mul4(d.First(left)).Mul4(respack.ShieldLocal)
}

// ThirdPersonHeld is ItemInHandLayer's placement after the arm's
// translateToHand (in the arm's pixel-to-block space): rotate -90 about
// X and 180 about Y, translate(±1/16, 2/16, -10/16) (x negated in the
// left hand), then the hand's third-person entry.
func ThirdPersonHeld(left bool, d respack.HeldDisplay) mgl32.Mat4 {
	x := float32(1.0 / 16)
	if left {
		x = -x
	}
	return mgl32.HomogRotate3DX(mgl32.DegToRad(-90)).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(180))).
		Mul4(mgl32.Translate3D(x, 2.0/16, -10.0/16)).
		Mul4(d.Third(left))
}

// MirrorHand turns a right-hand transform m into the left hand's:
// S·m·S with S = scale(-1, 1, 1), negating x translations and the Y and
// Z turns, as the reference's invert = -1 does (winding kept).
func MirrorHand(m mgl32.Mat4) mgl32.Mat4 {
	s := mgl32.Scale3D(-1, 1, 1)
	return s.Mul4(m).Mul4(s)
}
