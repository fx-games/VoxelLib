package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// IconView places something whose bounds are lo..hi as an icon for the
// hand draws (ModelRenderer.DrawHand, DrawBlockHand, ItemRenderer.
// DrawHandModel, all with their fixed 70° projection): turned by yaw, then
// pitched toward the viewer (radians), centred on the screen point x, y
// (normalised device coordinates, -1..1 with y up) and scaled so that it
// fits, in any turn, a square size high (in the same units: 2 is the
// screen's height), seen as if straight ahead wherever it is on the
// screen. It returns the view matrix and the rotation alone,
// which DrawBlockHand takes for its lights.
func IconView(x, y, size, aspect float32, lo, hi mgl32.Vec3, yaw, pitch float32) (view, turn mgl32.Mat4) {
	const d = 2                                // in front of the eye, well inside the hand's near and far planes
	t := d * float32(math.Tan(35*math.Pi/180)) // half the screen height at d
	ext := hi.Sub(lo).Len()
	if ext == 0 {
		ext = 1
	}
	c := lo.Add(hi).Mul(0.5)
	at := mgl32.Vec3{x * t * aspect, y * t, -d}
	// Off the screen's centre the eye sees the icon at a slant: face it
	// toward the eye, and make up for its distance, so every cell shows
	// the same view at the same size.
	face := mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, at.Normalize()).Mat4()
	k := size * t / ext * at.Len() / d
	turn = face.Mul4(mgl32.HomogRotate3DX(pitch)).Mul4(mgl32.HomogRotate3DY(yaw))
	view = mgl32.Translate3D(at[0], at[1], at[2]).Mul4(turn).Mul4(mgl32.Scale3D(k, k, k)).Mul4(mgl32.Translate3D(-c[0], -c[1], -c[2]))
	return view, turn
}
