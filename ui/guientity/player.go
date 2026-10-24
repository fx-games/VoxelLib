package guientity

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/ui"
)

// Biped is ModelBiped(0): head, hat, body, arms and legs on a 64x32 skin.
func Biped() *cuboid.Geometry {
	part := func(name string, pivot [3]float32, uv [2]int, pos [3]float32, size [3]int, inflate float32, mirror bool) cuboid.Part {
		return cuboid.Part{Name: name, Parent: -1, Pivot: pivot,
			Boxes: []cuboid.Box{{Pos: pos, Size: size, Inflate: inflate, UV: uv, Mirror: mirror, TexSize: [2]float32{64, 32}}}}
	}
	return &cuboid.Geometry{Name: "biped", TexSize: [2]int{64, 32}, Render: []int{0, 1, 2, 3, 4, 5, 6}, Parts: []cuboid.Part{
		part("bipedHead", [3]float32{}, [2]int{0, 0}, [3]float32{-4, -8, -4}, [3]int{8, 8, 8}, 0, false),
		part("bipedHeadwear", [3]float32{}, [2]int{32, 0}, [3]float32{-4, -8, -4}, [3]int{8, 8, 8}, 0.5, false),
		part("bipedBody", [3]float32{}, [2]int{16, 16}, [3]float32{-4, 0, -2}, [3]int{8, 12, 4}, 0, false),
		part("bipedRightArm", [3]float32{-5, 2, 0}, [2]int{40, 16}, [3]float32{-3, -2, -2}, [3]int{4, 12, 4}, 0, false),
		part("bipedLeftArm", [3]float32{5, 2, 0}, [2]int{40, 16}, [3]float32{-1, -2, -2}, [3]int{4, 12, 4}, 0, true),
		part("bipedRightLeg", [3]float32{-1.9, 12, 0}, [2]int{0, 16}, [3]float32{-2, 0, -2}, [3]int{4, 12, 4}, 0, false),
		part("bipedLeftLeg", [3]float32{1.9, 12, 0}, [2]int{0, 16}, [3]float32{-2, 0, -2}, [3]int{4, 12, 4}, 0, true),
	}}
}

// Player is the inventory screen's player (GuiInventory.func_147046_a):
// a Biped in Skin, its head following the mouse. Draw is
// InventoryOptions.Model and PlayerGUI.Model: x, y is the box's corner.
type Player struct {
	Skin *image.NRGBA

	geo   *cuboid.Geometry
	mdl   *model.Model
	pose  *cuboid.Pose
	world []mgl32.Mat4
	gt    Target
}

func (p *Player) Draw(ctx *ui.Context, x0, y0, mx, my int) {
	if p.mdl == nil {
		p.geo = Biped()
		p.mdl, _ = cuboid.Build(p.geo, nil) // a fixed geometry: no error
		p.pose = p.geo.RestPose()
		p.pose.Root = cuboid.Scale(0.9375, 0.9375, 0.9375) // RenderPlayer's scale
	}
	x, y := x0+26, y0+68 // guiLeft+51, guiTop+75
	dx, dy := float32(x-mx), float32(y-50-my)
	yawOffset, yaw, pitch := Angles(dx, dy)
	view, body := Matrices(float32(x), float32(y), 30, dx, dy)
	head := [3]float32{mgl32.DegToRad(pitch), mgl32.DegToRad(yaw - yawOffset), 0}
	p.pose.Parts[0].Rot, p.pose.Parts[1].Rot = head, head
	p.world = p.pose.World(p.geo, p.world)
	for i := range p.world {
		p.world[i] = body.Mul4(p.world[i])
	}
	p.gt.Draw(ctx, view, []Draw{{Model: p.mdl, Skin: p.mdl.SkinWorld(p.world, nil), Tex: p.Skin, Color: [4]float32{1, 1, 1, 1}}})
}
