package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Camera is a first-person camera. Position is float64 so worlds far from the
// origin keep full precision; everything sent to the GPU is relative to it.
type Camera struct {
	X, Y, Z    float64
	Yaw, Pitch float64 // radians; yaw 0 looks toward -Z (north)
	FOV        float64 // vertical, radians
	Aspect     float64
	Near, Far  float64
}

// Forward returns the view direction.
func (c *Camera) Forward() mgl32.Vec3 {
	cp := math.Cos(c.Pitch)
	return mgl32.Vec3{float32(math.Sin(c.Yaw) * cp), float32(math.Sin(c.Pitch)), float32(-math.Cos(c.Yaw) * cp)}
}

// ViewProj returns the camera-relative (no translation) view-projection.
func (c *Camera) ViewProj() mgl32.Mat4 {
	f := c.Forward()
	view := mgl32.LookAtV(mgl32.Vec3{}, f, mgl32.Vec3{0, 1, 0})
	proj := mgl32.Perspective(float32(c.FOV), float32(c.Aspect), float32(c.Near), float32(c.Far))
	return proj.Mul4(view)
}

// frustum holds 6 planes (a,b,c,d) in camera-relative space.
type frustum [6]mgl32.Vec4

func makeFrustum(m mgl32.Mat4) (f frustum) {
	row := func(i int) mgl32.Vec4 { return mgl32.Vec4{m.At(i, 0), m.At(i, 1), m.At(i, 2), m.At(i, 3)} }
	r0, r1, r2, r3 := row(0), row(1), row(2), row(3)
	f[0] = r3.Add(r0)
	f[1] = r3.Sub(r0)
	f[2] = r3.Add(r1)
	f[3] = r3.Sub(r1)
	f[4] = r3.Add(r2)
	f[5] = r3.Sub(r2)
	return
}

// box tests an AABB (camera-relative) against the frustum.
func (f *frustum) box(minX, minY, minZ, maxX, maxY, maxZ float32) bool {
	for _, p := range f {
		x, y, z := minX, minY, minZ
		if p[0] > 0 {
			x = maxX
		}
		if p[1] > 0 {
			y = maxY
		}
		if p[2] > 0 {
			z = maxZ
		}
		if p[0]*x+p[1]*y+p[2]*z+p[3] < 0 {
			return false
		}
	}
	return true
}
