package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/javarand"
)

// The reference's dropped-item poses (RenderItem.doRender,
// renderDroppedItem): the bob, the spin and the per-copy offsets of a
// stack lying in the world. Callers add the returned bob to the item's
// posY (feet + DropYOffset) and draw one model matrix per copy.

// DropYOffset is EntityItem's yOffset (height / 2): its posY above the
// bounding box's bottom.
const DropYOffset = 0.125

// DropCopies is RenderItem.doRender's b0: 1 + one per stack size step
// (>1, >5, >20, >40).
func DropCopies(count int) int {
	n := 1
	for _, s := range [...]int{1, 5, 20, 40} {
		if count > s {
			n++
		}
	}
	return n
}

// FlatCopies is RenderItem.renderDroppedItem's (fancy graphics) b0 for a
// flat item: 1, 2 from 2, 3 from 16, 4 from 32.
func FlatCopies(count int) int {
	switch {
	case count < 2:
		return 1
	case count < 16:
		return 2
	case count < 32:
		return 3
	}
	return 4
}

// dropBob is RenderItem.doRender's f2 for an item age ticks old (partial
// tick included): sin(t/10)*0.1 + 0.1. hoverStart is 0 (the ports' drops
// have none).
func dropBob(t float64) float64 { return math.Sin(t/10)*0.1 + 0.1 }

// BlockDropPoses appends RenderItem.doRender's transforms of a dropped 3D
// block (RenderItem.java:106-147), about the entity's posY (feet +
// DropYOffset) plus the bob: glRotatef(f3, 0, 1, 0), glScalef(scale), and
// per copy after the first a random offset of up to ±0.2 blocks (random
// seeded 187 each render), then base (renderBlockAsItem's). scale is f9
// (0.5 or 0.25 by render type). The bob is returned separately.
func BlockDropPoses(age int, alpha float64, count int, scale float32, base mgl32.Mat4, out []mgl32.Mat4) ([]mgl32.Mat4, float64) {
	t := float64(age) + alpha
	spin := mgl32.HomogRotate3DY(float32(t / 20)) // f3 in radians
	m := spin.Mul4(mgl32.Scale3D(scale, scale, scale))
	var r javarand.Random
	r.SetSeed(187)
	for l := range DropCopies(count) {
		c := m
		if l > 0 {
			f6 := (r.NextFloat()*2 - 1) * 0.2 / scale
			f7 := (r.NextFloat()*2 - 1) * 0.2 / scale
			f8 := (r.NextFloat()*2 - 1) * 0.2 / scale
			c = c.Mul4(mgl32.Translate3D(f6, f7, f8))
		}
		out = append(out, c.Mul4(base))
	}
	return out, dropBob(t)
}

// FlatDropPoses appends RenderItem.doRender and renderDroppedItem's
// transforms (fancy graphics, RenderItem.java:60-110, 256-340) of a
// dropped flat item about posY plus the bob: glScalef(0.5), the spin f3
// about Y, glTranslatef(-0.5, -0.25, -(0.0625+0.021875)*b0/2), then per
// copy glTranslatef(0, 0, 0.084375) or, after the first, a random (x, y)
// of up to ±0.6 (random seeded 187; its z draw is unused); the offsets
// add up. in2D maps the slab to renderItemIn2D's space (ItemRenderer.In2D).
func FlatDropPoses(age int, alpha float64, count int, in2D mgl32.Mat4, out []mgl32.Mat4) ([]mgl32.Mat4, float64) {
	t := float64(age) + alpha
	const step = 0.0625 + 0.021875 // f9 + f10
	n := FlatCopies(count)
	m := mgl32.Scale3D(0.5, 0.5, 0.5).Mul4(mgl32.HomogRotate3DY(float32(t / 20))).
		Mul4(mgl32.Translate3D(-0.5, -0.25, -step*float32(n)/2))
	var r javarand.Random
	r.SetSeed(187)
	for k := range n {
		if k > 0 {
			x := (r.NextFloat()*2 - 1) * 0.3 / 0.5
			y := (r.NextFloat()*2 - 1) * 0.3 / 0.5
			r.NextFloat()
			m = m.Mul4(mgl32.Translate3D(x, y, step))
		} else {
			m = m.Mul4(mgl32.Translate3D(0, 0, step))
		}
		out = append(out, m.Mul4(in2D))
	}
	return out, dropBob(t)
}
