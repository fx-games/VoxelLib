package guientity

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/ui"
)

// Biped is ModelBiped(0): head, hat, body, arms and legs on a 64x32 skin.
func Biped() *cuboid.Geometry { return biped(0) }

// biped is ModelBiped(f): every box grown by f, the hat by f more.
func biped(f float32) *cuboid.Geometry {
	part := func(name string, pivot [3]float32, uv [2]int, pos [3]float32, size [3]int, inflate float32, mirror bool) cuboid.Part {
		return cuboid.Part{Name: name, Parent: -1, Pivot: pivot,
			Boxes: []cuboid.Box{{Pos: pos, Size: size, Inflate: inflate, UV: uv, Mirror: mirror, TexSize: [2]float32{64, 32}}}}
	}
	return &cuboid.Geometry{Name: "biped", TexSize: [2]int{64, 32}, Render: []int{0, 1, 2, 3, 4, 5, 6}, Parts: []cuboid.Part{
		part("bipedHead", [3]float32{}, [2]int{0, 0}, [3]float32{-4, -8, -4}, [3]int{8, 8, 8}, f, false),
		part("bipedHeadwear", [3]float32{}, [2]int{32, 0}, [3]float32{-4, -8, -4}, [3]int{8, 8, 8}, f+0.5, false),
		part("bipedBody", [3]float32{}, [2]int{16, 16}, [3]float32{-4, 0, -2}, [3]int{8, 12, 4}, f, false),
		part("bipedRightArm", [3]float32{-5, 2, 0}, [2]int{40, 16}, [3]float32{-3, -2, -2}, [3]int{4, 12, 4}, f, false),
		part("bipedLeftArm", [3]float32{5, 2, 0}, [2]int{40, 16}, [3]float32{-1, -2, -2}, [3]int{4, 12, 4}, f, true),
		part("bipedRightLeg", [3]float32{-1.9, 12, 0}, [2]int{0, 16}, [3]float32{-2, 0, -2}, [3]int{4, 12, 4}, f, false),
		part("bipedLeftLeg", [3]float32{1.9, 12, 0}, [2]int{0, 16}, [3]float32{-2, 0, -2}, [3]int{4, 12, 4}, f, true),
	}}
}

// The armour pieces (ItemArmor.armorType). RenderBiped draws each on a
// ModelBiped showing only its parts: the helmet, chest and boots of a
// material's 64x32 _layer_1.png texture on ModelBiped(1), the leggings of
// its _layer_2.png on ModelBiped(0.5).
const (
	Helmet = iota
	Chest
	Leggings
	Boots
)

var armorParts = [4][]int{{0, 1}, {2, 3, 4}, {2, 5, 6}, {5, 6}} // Biped's part indices
var armorInflate = [4]float32{1, 1, 0.5, 1}

// Armor is a piece of armour drawn over the Player's skin: Tex its layer
// texture, Piece Helmet, Chest, Leggings or Boots.
type Armor struct {
	Tex   *image.NRGBA
	Piece int
}

// Player is the inventory screen's player (GuiInventory.func_147046_a):
// a Biped in Skin wearing Armor, its head following the mouse. Draw is
// InventoryOptions.Model and PlayerGUI.Model: x, y is the box's corner.
type Player struct {
	Skin  *image.NRGBA
	Armor []Armor

	geo   *cuboid.Geometry
	mdl   *model.Model
	pose  *cuboid.Pose
	world []mgl32.Mat4
	gt    Target
	armor [4]*model.Model // the armour pieces' models, built when first worn
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
	white := [4]float32{1, 1, 1, 1}
	ds := []Draw{{Model: p.mdl, Skin: p.mdl.SkinWorld(p.world, nil), Tex: p.Skin, Color: white}}
	for _, a := range p.Armor {
		m := p.armor[a.Piece]
		if m == nil {
			g := biped(armorInflate[a.Piece])
			g.Render = armorParts[a.Piece]
			m, _ = cuboid.Build(g, nil) // a fixed geometry: no error
			p.armor[a.Piece] = m
		}
		ds = append(ds, Draw{Model: m, Skin: m.SkinWorld(p.world, nil), Tex: a.Tex, Color: white})
	}
	p.gt.Draw(ctx, view, ds)
}
