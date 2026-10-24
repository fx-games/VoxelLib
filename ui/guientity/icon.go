package guientity

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/raster"
)

// ItemIcon rasterizes a held model's GUI icon, px pixels square: m's
// joint 0 under place (the item's gui display entry times its local
// transform, e.g. respack.Shield.GUI.Mat(false)·respack.ShieldLocal) in a
// 16x16 slot (translate(8, 8), scale(16, -16, 16)), lit by Lights in the
// item's frame.
// ponytail: the slot's 3D-item lights are taken as Lights in the item
// frame, not the respack's rotated GUI lighting.
func ItemIcon(m *model.Model, place mgl32.Mat4, tex *image.NRGBA, px int) *image.NRGBA {
	t := raster.NewTarget(px, px)
	f := float32(px) / 16
	view := mgl32.Scale3D(f, f, f).Mul4(mgl32.Translate3D(8, 8, 0)).Mul4(mgl32.Scale3D(16, -16, 16))
	t.Draw(m, []mgl32.Mat4{place}, view, tex, [4]float32{1, 1, 1, 1}, Lights)
	return t.Img
}
