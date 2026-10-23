package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// The reference's first-person use poses, applied after FirstPersonHold
// while an item is in use (no swing offset then).

// FirstPersonBlock raises a blade across the view: translate(-0.5, 0.2,
// 0), rotate 30 about Y, -80 about X, 60 about Y.
var FirstPersonBlock = mgl32.Translate3D(-0.5, 0.2, 0).
	Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(30))).
	Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-80))).
	Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(60)))

// FirstPersonDraw is the drawn-bow pose for f10 ticks held (partial tick
// included): the bow turned before the view, pulled back by the draw
// curve of f10/20, trembling once past 0.1 and stretched along its depth
// by a fifth at full draw.
func FirstPersonDraw(f10 float32) mgl32.Mat4 {
	deg := mgl32.DegToRad
	f11 := f10 / 20
	f11 = min((f11*f11+f11*2)/3, 1)
	m := mgl32.HomogRotate3DZ(deg(-18)).Mul4(mgl32.HomogRotate3DY(deg(-12))).
		Mul4(mgl32.HomogRotate3DX(deg(-8))).Mul4(mgl32.Translate3D(-0.9, 0.2, 0))
	if f11 > 0.1 {
		m = m.Mul4(mgl32.Translate3D(0, float32(math.Sin(float64((f10-0.1)*1.3)))*0.01*(f11-0.1), 0))
	}
	return m.Mul4(mgl32.Translate3D(0, 0, f11*0.1)).Mul4(mgl32.HomogRotate3DZ(deg(-335))).
		Mul4(mgl32.HomogRotate3DY(deg(-50))).Mul4(mgl32.Translate3D(0, 0.5, 0)).
		Mul4(mgl32.Scale3D(1, 1, 1+f11*0.2)).Mul4(mgl32.Translate3D(0, -0.5, 0)).
		Mul4(mgl32.HomogRotate3DY(deg(50))).Mul4(mgl32.HomogRotate3DZ(deg(335)))
}
