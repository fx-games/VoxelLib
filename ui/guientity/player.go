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
// texture, Piece Helmet, Chest, Leggings or Boots on the biped; or, with
// Geo, its own cube geometry (an armour mod's model in modern part names:
// head, hat, body, right_arm, left_arm, right_leg, left_leg roots, which
// follow the biped's parts) drawn with Tex.
type Armor struct {
	Tex   *image.NRGBA
	Piece int
	Geo   *cuboid.Geometry
}

// bipedPart is the biped part each of a mesh's root parts follows.
var bipedPart = map[string]int{"head": 0, "hat": 1, "body": 2, "right_arm": 3, "left_arm": 4, "right_leg": 5, "left_leg": 6}

// mesh is a Geo armour's built model and pose.
type mesh struct {
	mdl   *model.Model
	pose  *cuboid.Pose
	world []mgl32.Mat4
}

// Player is the inventory screen's player (GuiInventory.func_147046_a):
// a Biped in Skin wearing Armor, its head following the mouse. Draw is
// InventoryOptions.Model and PlayerGUI.Model: x, y is the box's corner.
type Player struct {
	Skin  *image.NRGBA
	Armor []Armor

	geo    *cuboid.Geometry
	mdl    *model.Model
	pose   *cuboid.Pose
	world  []mgl32.Mat4
	gt     Target
	armor  [4]*model.Model // the armour pieces' models, built when first worn
	meshes map[*cuboid.Geometry]*mesh
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
	p.pose.Parts[1].Hidden = false // the hair layer hides under a mesh helmet (it would poke through)
	for _, a := range p.Armor {
		if a.Geo != nil && a.Piece == Helmet {
			p.pose.Parts[1].Hidden = true
		}
	}
	p.world = p.pose.World(p.geo, p.world)
	for i := range p.world {
		p.world[i] = body.Mul4(p.world[i])
	}
	white := [4]float32{1, 1, 1, 1}
	ds := []Draw{{Model: p.mdl, Skin: p.mdl.SkinWorld(p.world, nil), Tex: p.Skin, Color: white}}
	for _, a := range p.Armor {
		if a.Geo != nil {
			if d, ok := p.meshDraw(a, body); ok {
				ds = append(ds, d)
			}
			continue
		}
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

// meshDraw poses a Geo armour like the player (its root parts turn with
// the biped's parts of the same name) under body and draws it with its
// texture. A geometry that does not build draws nothing.
func (p *Player) meshDraw(a Armor, body mgl32.Mat4) (Draw, bool) {
	m := p.meshes[a.Geo]
	if m == nil {
		mdl, err := cuboid.Build(a.Geo, nil)
		if err != nil {
			return Draw{}, false
		}
		m = &mesh{mdl: mdl, pose: a.Geo.RestPose()}
		if p.meshes == nil {
			p.meshes = map[*cuboid.Geometry]*mesh{}
		}
		p.meshes[a.Geo] = m
	}
	m.pose.Root = p.pose.Root
	for i, part := range a.Geo.Parts {
		if bi, ok := bipedPart[part.Name]; ok && part.Parent < 0 {
			m.pose.Parts[i].Rot = p.pose.Parts[bi].Rot
		}
	}
	m.world = m.pose.World(a.Geo, m.world)
	for i := range m.world {
		m.world[i] = body.Mul4(m.world[i])
	}
	return Draw{Model: m.mdl, Skin: m.mdl.SkinWorld(m.world, nil), Tex: a.Tex, Color: [4]float32{1, 1, 1, 1}}, true
}
