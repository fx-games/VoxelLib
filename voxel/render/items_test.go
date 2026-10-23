package render

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// TestFirstPersonItem checks DrawHand's chain against Minecraft 1.7.10's
// renderItemInFirstPerson + renderItem + renderItemIn2D, worked out by hand
// with OpenGL's glTranslatef/glRotatef/glScalef matrices: renderItemIn2D's
// corners (0,0,0), (1,1,0) and (1,0,-0.0625) in view space.
func TestFirstPersonItem(t *testing.T) {
	ir := &ItemRenderer{index: map[string]int{"a": 0}, texels: []int{16}}
	const h = float32(1) / 32 // half the slab of a 16 texel sprite
	// our slab corners for renderItemIn2D's (0,0,0), (1,1,0), (1,0,-0.0625):
	// u runs against x, the front face is renderItemIn2D's z = 0
	corners := []mgl32.Vec3{{0.5, -0.5, h}, {-0.5, 0.5, h}, {-0.5, -0.5, -h}}
	for _, c := range []struct {
		swing float32
		want  []mgl32.Vec3
	}{
		{0, []mgl32.Vec3{{0.605813, -0.436264, -0.196354}, {0.536319, -0.14605, -0.990675}, {0.521062, -0.689835, -0.734801}}},
		{0.5, []mgl32.Vec3{{0.109636, -0.237383, -0.720335}, {0.161803, -0.780457, -1.370219}, {0.271291, -0.813327, -0.780017}}},
	} {
		m := FirstPersonSwing(c.swing).Mul4(FirstPersonHold(c.swing)).Mul4(Equipped2D).Mul4(ir.In2D("a"))
		for i, p := range corners {
			got := m.Mul4x1(p.Vec4(1)).Vec3()
			if got.Sub(c.want[i]).Len() > 1e-5 {
				t.Errorf("swing %v corner %d: %v, want %v", c.swing, i, got, c.want[i])
			}
		}
	}
	// a 32 texel sprite is half as thick per texel: still 0.0625 in all
	m := ir.In2D("a")
	ir.texels[0] = 32
	if a, b := m.Mul4x1(mgl32.Vec4{0, 0, -h, 1}), ir.In2D("a").Mul4x1(mgl32.Vec4{0, 0, -h / 2, 1}); a.Sub(b).Len() > 1e-7 || a.Z() != -0.0625 {
		t.Errorf("32 texel back face at %v, 16 texel at %v", b, a)
	}
}
