package respack

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
