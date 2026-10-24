package render

import "github.com/go-gl/mathgl/mgl32"

// Display is one display entry of a respack item model (ItemTransform):
// rotation in degrees (X·Y·Z, rotationXYZ), translation in pixels (the
// model file's units) and scale (zero: 1).
type Display struct {
	Rot, Trans, Scale [3]float32
}

// Mat is ItemTransform.apply: translate (x negated in the left hand),
// rotate (y and z negated in the left hand), scale, then translate(-0.5,
// -0.5, -0.5) to the model's centre.
func (d Display) Mat(left bool) mgl32.Mat4 {
	tx, ry, rz := d.Trans[0], d.Rot[1], d.Rot[2]
	if left {
		tx, ry, rz = -tx, -ry, -rz
	}
	s := d.Scale
	if s == ([3]float32{}) {
		s = [3]float32{1, 1, 1}
	}
	return mgl32.Translate3D(tx/16, d.Trans[1]/16, d.Trans[2]/16).
		Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(d.Rot[0]))).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(ry))).
		Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(rz))).
		Mul4(mgl32.Scale3D(s[0], s[1], s[2])).
		Mul4(mgl32.Translate3D(-0.5, -0.5, -0.5))
}

// HeldDisplay is a held model's display entries.
type HeldDisplay struct {
	FirstRight, FirstLeft, ThirdRight, ThirdLeft, GUI Display
}

// Shield is the shield's item model and ShieldBlocking its raised one.
var (
	Shield = HeldDisplay{
		ThirdRight: Display{Rot: [3]float32{0, 90, 0}, Trans: [3]float32{10, 6, -4}},
		ThirdLeft:  Display{Rot: [3]float32{0, 90, 0}, Trans: [3]float32{10, 6, 12}},
		FirstRight: Display{Rot: [3]float32{0, 180, 5}, Trans: [3]float32{-10, 1.75, -10}, Scale: [3]float32{1.25, 1.25, 1.25}},
		FirstLeft:  Display{Rot: [3]float32{0, 180, 5}, Trans: [3]float32{10, 0, -10}, Scale: [3]float32{1.25, 1.25, 1.25}},
		GUI:        Display{Rot: [3]float32{15, -25, -5}, Trans: [3]float32{2, 3, 0}, Scale: [3]float32{0.65, 0.65, 0.65}},
	}
	ShieldBlocking = HeldDisplay{
		ThirdRight: Display{Rot: [3]float32{45, 155, 0}, Trans: [3]float32{-3.49, 11, -2}},
		ThirdLeft:  Display{Rot: [3]float32{45, 155, 0}, Trans: [3]float32{11.51, 7, 2.5}},
		FirstRight: Display{Rot: [3]float32{0, 180, -5}, Trans: [3]float32{-15, 3.25, -11}, Scale: [3]float32{1.25, 1.25, 1.25}},
		FirstLeft:  Display{Rot: [3]float32{0, 180, -5}, Trans: [3]float32{5, 5, -11}, Scale: [3]float32{1.25, 1.25, 1.25}},
		GUI:        Shield.GUI,
	}
)

// First is the first-person entry of the hand.
func (h HeldDisplay) First(left bool) mgl32.Mat4 {
	if left {
		return h.FirstLeft.Mat(true)
	}
	return h.FirstRight.Mat(false)
}

// Third is the third-person entry of the hand.
func (h HeldDisplay) Third(left bool) mgl32.Mat4 {
	if left {
		return h.ThirdLeft.Mat(true)
	}
	return h.ThirdRight.Mat(false)
}

// ShieldLocal maps cuboid.ShieldBoxes' pixel mesh into its item model
// space: scale(1, -1, -1) and 1/16.
var ShieldLocal = mgl32.Scale3D(1.0/16, -1.0/16, -1.0/16)

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
// ShieldLocal. A raised shield gets no swing.
func FirstPersonShield(left, raised bool, lowered float32) mgl32.Mat4 {
	d := Shield
	if raised {
		d = ShieldBlocking
	}
	return ArmTransform(left, lowered).Mul4(d.First(left)).Mul4(ShieldLocal)
}

// ThirdPersonHeld is ItemInHandLayer's placement after the arm's
// translateToHand (in the arm's pixel-to-block space): rotate -90 about
// X and 180 about Y, translate(±1/16, 2/16, -10/16) (x negated in the
// left hand), then the hand's third-person entry.
func ThirdPersonHeld(left bool, d HeldDisplay) mgl32.Mat4 {
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
