package render

import (
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// TestGlintMatrices checks the glint texture matrices against the GL
// calls they stand for, built with mgl32 as glScalef/glTranslatef/
// glRotatef multiply them on, at a few texture coordinates.
func TestGlintMatrices(t *testing.T) {
	rot := func(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DZ(mgl32.DegToRad(deg)) }
	sc := func(s float32) mgl32.Mat4 { return mgl32.Scale3D(s, s, s) }
	for _, c := range []struct {
		name string
		got  glintAffine
		want mgl32.Mat4
	}{
		{"item 0", itemGlint(4500, 0), sc(0.125).Mul4(mgl32.Translate3D(4, 0, 0)).Mul4(rot(-50))},
		{"item 1", itemGlint(4873+4873/4, 1), sc(0.125).Mul4(mgl32.Translate3D(-float32(4873/4)/4873*8, 0, 0)).Mul4(rot(10))},
		{"armour 0", modelGlint(10, 0), sc(0.33333334).Mul4(rot(30)).Mul4(mgl32.Translate3D(0, 10*0.001*20, 0))},
		{"armour 1", modelGlint(10, 1), sc(0.33333334).Mul4(rot(-30)).Mul4(mgl32.Translate3D(0, 10*0.004*20, 0))},
	} {
		for _, uv := range [][2]float32{{0, 0}, {1, 0}, {0.25, 0.75}} {
			w := c.want.Mul4x1(mgl32.Vec4{uv[0], uv[1], 0, 1})
			a := c.got
			gu, gv := a[0]*uv[0]+a[1]*uv[1]+a[2], a[3]*uv[0]+a[4]*uv[1]+a[5]
			if d := (mgl32.Vec2{gu - w[0], gv - w[1]}); d.Len() > 1e-5 {
				t.Errorf("%s at %v: (%v, %v), want (%v, %v)", c.name, uv, gu, gv, w[0], w[1])
			}
		}
	}
	if !strings.Contains(modelGlintWGSL, "o.guv") || strings.Contains(modelGlintWGSL, "overlay") {
		t.Error("modelGlintWGSL lacks its glint coordinates")
	}
}
