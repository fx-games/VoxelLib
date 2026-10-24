package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/javarand"
)

// TestBlockDropPoses checks RenderItem.doRender's block chain: b0 copies,
// the bob, the spin about Y and the seed-187 offset of the second copy.
func TestBlockDropPoses(t *testing.T) {
	for n, want := range map[int]int{1: 1, 2: 2, 5: 2, 6: 3, 20: 3, 21: 4, 40: 4, 41: 5, 64: 5} {
		if got := DropCopies(n); got != want {
			t.Errorf("DropCopies(%d) = %d, want %d", n, got, want)
		}
	}
	// renderBlockAsItem's base for render type 0: glRotatef(90) glTranslatef(-0.5)
	base := mgl32.HomogRotate3DY(mgl32.DegToRad(90)).Mul4(mgl32.Translate3D(-0.5, -0.5, -0.5))
	ms, bob := BlockDropPoses(31, 0.5, 2, 0.25, base, nil)
	tt := 31.5
	if len(ms) != 2 || math.Abs(bob-(math.Sin(tt/10)*0.1+0.1)) > 1e-9 {
		t.Fatalf("drop: %d models, bob %v", len(ms), bob)
	}
	// the block corner (1,1,1) lands at R_Y(f3) * 0.25 * (0.5, 0.5, -0.5)
	a := tt / 20
	p := ms[0].Mul4x1(mgl32.Vec4{1, 1, 1, 1})
	want := mgl32.Vec3{float32(0.125*math.Cos(a) - 0.125*math.Sin(a)), 0.125, float32(-0.125*math.Sin(a) - 0.125*math.Cos(a))}
	if !p.Vec3().ApproxEqualThreshold(want, 1e-5) {
		t.Errorf("drop corner %v, want %v", p.Vec3(), want)
	}
	// the second copy sits (rand*2-1)*0.2 blocks off (Random(187))
	var r javarand.Random
	r.SetSeed(187)
	off := mgl32.Vec3{(r.NextFloat()*2 - 1) * 0.2, (r.NextFloat()*2 - 1) * 0.2, (r.NextFloat()*2 - 1) * 0.2}
	d := ms[1].Col(3).Vec3().Sub(ms[0].Col(3).Vec3())
	if !d.ApproxEqualThreshold(mgl32.HomogRotate3DY(float32(a)).Mul4x1(off.Vec4(0)).Vec3(), 1e-5) {
		t.Errorf("copy offset %v, want %v turned", d, off)
	}
}

// TestFlatDropPoses checks RenderItem.renderDroppedItem's flat chain: copy
// counts, the stacked z steps and random (x, y) offsets of seed 187.
func TestFlatDropPoses(t *testing.T) {
	for _, c := range [][2]int{{1, 1}, {2, 2}, {15, 2}, {16, 3}, {31, 3}, {32, 4}, {64, 4}} {
		if got := FlatCopies(c[0]); got != c[1] {
			t.Errorf("FlatCopies(%d) = %d, want %d", c[0], got, c[1])
		}
	}
	ms, bob := FlatDropPoses(0, 0, 20, mgl32.Ident4(), nil)
	if len(ms) != 3 || math.Abs(bob-0.1) > 1e-9 {
		t.Fatalf("copies %d bob %v", len(ms), bob)
	}
	// age 0: no spin; S0.5 then T(-0.5,-0.25,-0.084375*3/2) then +0.084375 z
	o := ms[0].Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	want := mgl32.Vec3{-0.25, -0.125, 0.5 * (0.084375 - 0.084375*1.5)}
	if !o.Vec3().ApproxEqualThreshold(want, 1e-6) {
		t.Errorf("copy 0 origin %v, want %v", o, want)
	}
	var r javarand.Random
	r.SetSeed(187)
	x := (r.NextFloat()*2 - 1) * 0.6
	y := (r.NextFloat()*2 - 1) * 0.6
	o1 := ms[1].Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3()
	want1 := want.Add(mgl32.Vec3{x * 0.5, y * 0.5, 0.084375 * 0.5})
	if !o1.ApproxEqualThreshold(want1, 1e-6) {
		t.Errorf("copy 1 origin %v, want %v", o1, want1)
	}
}
