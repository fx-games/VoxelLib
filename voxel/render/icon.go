package render

import (
	"github.com/go-gl/mathgl/mgl32"
)

// handProj is the hand draws' projection: the 70° lens, or with Icons a
// flat one, 2 high, in which nothing is foreshortened.
func (r *Renderer) handProj(aspect float32) mgl32.Mat4 {
	if r.Icons {
		return mgl32.Ortho(-aspect, aspect, -1, 1, 0.05, 10)
	}
	return mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
}

// IconView places something whose bounds are lo..hi as an icon for the
// hand draws with Renderer.Icons set (ModelRenderer.DrawHand,
// DrawBlockHand, ItemRenderer.DrawHandModel): turned by yaw, then pitched
// toward the viewer (radians), centred on the screen point x, y
// (normalised device coordinates, -1..1 with y up) and scaled so that it
// fits, in any turn, a square size high (in the same units: 2 is the
// screen's height). It returns the view matrix and the rotation alone,
// which DrawBlockHand takes for its lights.
func IconView(x, y, size, aspect float32, lo, hi mgl32.Vec3, yaw, pitch float32) (view, turn mgl32.Mat4) {
	const d = 2 // in front of the eye, well inside the near and far planes
	ext := hi.Sub(lo).Len()
	if ext == 0 {
		ext = 1
	}
	c := lo.Add(hi).Mul(0.5)
	k := size / ext
	turn = mgl32.HomogRotate3DX(pitch).Mul4(mgl32.HomogRotate3DY(yaw))
	view = mgl32.Translate3D(x*aspect, y, -d).Mul4(turn).Mul4(mgl32.Scale3D(k, k, k)).Mul4(mgl32.Translate3D(-c[0], -c[1], -c[2]))
	return view, turn
}
