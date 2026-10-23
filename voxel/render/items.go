package render

import (
	"image"
	"image/draw"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/items/extrude"
)

// ItemRenderer draws 2D item sprites extruded into 3D slabs (items/extrude):
// items lying in the world and the item held in first person. All sprites
// share one texture array and one vertex buffer; each item is a range.
type ItemRenderer struct {
	r      *Renderer
	gpu    gpuItemRenderer
	index  map[string]int
	ranges [][2]int32 // first, count per item
	texels []int      // the larger sprite side per item (In2D)
}

// ItemInstance is one item to draw in the world.
type ItemInstance struct {
	Item       string
	X, Y, Z    float64 // centre
	Yaw        float64 // spin around Y, radians
	Scale      float32 // edge length in blocks (Minecraft drops: 0.5)
	Sky, Block uint8   // light levels 0..15 at the item
}

// NewItemRenderer extrudes every sprite and uploads them. names[i] is the
// item ID of imgs[i]. The texture layers are as large as the largest
// sprite; a smaller one sits unscaled in its layer's top-left corner, so
// its texels stay exactly where the extruded side faces expect them.
func NewItemRenderer(r *Renderer, names []string, imgs []image.Image) (*ItemRenderer, error) {
	ir := &ItemRenderer{r: r, index: map[string]int{}}
	size := 8
	for _, img := range imgs {
		size = max(size, img.Bounds().Dx(), img.Bounds().Dy())
	}
	var verts []float32
	layers := make([][]byte, len(imgs))
	for i, img := range imgs {
		ir.index[names[i]] = i
		ir.texels = append(ir.texels, max(img.Bounds().Dx(), img.Bounds().Dy()))
		m := extrude.Extrude(img)
		b := img.Bounds()
		su, sv := float32(b.Dx())/float32(size), float32(b.Dy())/float32(size)
		ir.ranges = append(ir.ranges, [2]int32{int32(len(verts) / 8), int32(len(m.Verts))})
		for _, v := range m.Verts {
			verts = append(verts, v.Pos[0], v.Pos[1], v.Pos[2], v.UV[0]*su, v.UV[1]*sv, v.Normal[0], v.Normal[1], v.Normal[2])
		}
		layer := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.Draw(layer, b.Sub(b.Min), img, b.Min, draw.Src)
		layers[i] = layer.Pix
	}
	if err := ir.initGPU(verts, size, layers); err != nil {
		return nil, err
	}
	return ir, nil
}

// Has reports whether an item ID is known.
func (ir *ItemRenderer) Has(item string) bool { _, ok := ir.index[item]; return ok }

func (ir *ItemRenderer) draw(item string, vp, model mgl32.Mat4, sky, block uint8) {
	i, ok := ir.index[item]
	if !ok {
		return
	}
	ir.drawRange(i, vp.Mul4(model), model, sky, block)
}

// DrawWorld draws items lying in the world. Call after Renderer.Render with
// the same camera.
func (ir *ItemRenderer) DrawWorld(cam *Camera, items []ItemInstance) {
	if len(items) == 0 {
		return
	}
	vp := cam.ViewProj()
	ir.begin(ir.r.CurrentFog())
	for _, it := range items {
		s := it.Scale
		if s == 0 {
			s = 0.5
		}
		model := mgl32.Translate3D(float32(it.X-cam.X), float32(it.Y-cam.Y), float32(it.Z-cam.Z)).
			Mul4(mgl32.HomogRotate3DY(float32(-it.Yaw))).
			Mul4(mgl32.Scale3D(s, s, s))
		ir.draw(it.Item, vp, model, it.Sky, it.Block)
	}
	ir.end()
}

// ItemPose is an item placed by a full transform, e.g. a weapon in a
// creature's hand: M (in blocks, about X,Y,Z) maps the item's unit slab
// (X,Y in [-0.5,0.5], the sprite's left-bottom at -X,-Y) to the world.
type ItemPose struct {
	Item       string
	X, Y, Z    float64
	M          mgl32.Mat4
	Sky, Block uint8
}

// DrawPosed draws items at arbitrary transforms. Call after Renderer.Render
// with the same camera.
func (ir *ItemRenderer) DrawPosed(cam *Camera, items []ItemPose) {
	if len(items) == 0 {
		return
	}
	vp := cam.ViewProj()
	ir.begin(ir.r.CurrentFog())
	for _, it := range items {
		model := mgl32.Translate3D(float32(it.X-cam.X), float32(it.Y-cam.Y), float32(it.Z-cam.Z)).Mul4(it.M)
		ir.draw(it.Item, vp, model, it.Sky, it.Block)
	}
	ir.end()
}

// HandPose describes the first-person held item's motion.
type HandPose struct {
	// View is the hand's modelview before its own placement: renderHand's
	// setupViewBobbing and renderItemInFirstPerson's arm lag; zero: none.
	View  mgl32.Mat4
	Swing float32 // 0..1 swing progress (breaking/placing); 0 = idle
}

// Base is View, the identity when unset.
func (p HandPose) Base() mgl32.Mat4 {
	if p.View == (mgl32.Mat4{}) {
		return mgl32.Ident4()
	}
	return p.View
}

// DrawHand draws the held item in first person like Minecraft 1.7.10
// draws a flat item: pose.Base(), FirstPersonSwing, FirstPersonHold,
// Equipped2D and In2D (ItemRenderer.renderItemInFirstPerson and
// renderItem), over the world with the hand's own projection
// (DrawHandModel). aspect is the viewport's width/height.
func (ir *ItemRenderer) DrawHand(item string, aspect float32, pose HandPose, sky, block uint8) {
	model := pose.Base().Mul4(FirstPersonSwing(pose.Swing)).Mul4(FirstPersonHold(pose.Swing)).
		Mul4(Equipped2D).Mul4(ir.In2D(item))
	ir.DrawHandModel(item, aspect, model, sky, block)
}

// In2D maps an item's unit slab (see ItemPose) onto the space of
// ItemRenderer.renderItemIn2D(tess, maxU, minV, minU, maxV, w, h,
// 0.0625): the sprite on 0..1 in x and y, its left column at x = 1 (u
// runs against x), the front face at z = 0 and the back at z = -0.0625.
// The slab is one sprite texel thick and centred on z = 0.
func (ir *ItemRenderer) In2D(item string) mgl32.Mat4 {
	t := 16
	if i, ok := ir.index[item]; ok {
		t = ir.texels[i]
	}
	return mgl32.Translate3D(0.5, 0.5, -0.03125).Mul4(mgl32.Scale3D(-1, 1, float32(t)/16))
}

// FirstPersonSwing is renderItemInFirstPerson's swing offset for a held
// item (ItemRenderer.java 1.7.10, before translate(0.56, -0.52, -0.72)):
// translate(-sin(sqrt(s)*pi)*0.4, sin(sqrt(s)*pi*2)*0.2, -sin(s*pi)*0.2).
// An item in use (eat, drink) replaces it with its own motion.
func FirstPersonSwing(swing float32) mgl32.Mat4 {
	sq := float64(swing)
	sq = math.Sqrt(sq)
	return mgl32.Translate3D(-float32(math.Sin(sq*math.Pi))*0.4, float32(math.Sin(sq*math.Pi*2))*0.2,
		-float32(math.Sin(float64(swing)*math.Pi))*0.2)
}

// FirstPersonHold is the rest of renderItemInFirstPerson's placement of a
// held item, after the swing offset: translate(0.7*0.8, -0.65*0.8 -
// (1-equip)*0.6, -0.9*0.8) with the equip animation done (equip 1),
// rotate 45 about Y, the swing rotations -sin(s*s*pi)*20 about Y,
// -sin(sqrt(s)*pi)*20 about Z and *80 about X, and scale 0.4.
func FirstPersonHold(swing float32) mgl32.Mat4 {
	const f13 = 0.8
	f6 := float32(math.Sin(float64(swing*swing) * math.Pi))
	f7 := float32(math.Sin(math.Sqrt(float64(swing)) * math.Pi))
	return mgl32.Translate3D(0.7*f13, -0.65*f13, -0.9*f13).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(45))).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(-f6 * 20))).
		Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(-f7 * 20))).
		Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-f7 * 80))).
		Mul4(mgl32.Scale3D(0.4, 0.4, 0.4))
}

// Equipped2D is ItemRenderer.renderItem's placement of a flat item held
// by an entity (and ForgeHooksClient.renderEquippedItem's before an
// IItemRenderer): translate(0, -0.3, 0), scale 1.5, rotate 50 about Y and
// 335 about Z, translate(-0.9375, -0.0625, 0); then renderItemIn2D (In2D).
var Equipped2D = mgl32.Translate3D(0, -0.3, 0).
	Mul4(mgl32.Scale3D(1.5, 1.5, 1.5)).
	Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(50))).
	Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(335))).
	Mul4(mgl32.Translate3D(-0.9375, -0.0625, 0))

// DrawHandModel draws a held item like DrawHand but placed by the caller:
// model maps the item's unit slab (see ItemPose) into view space (x right,
// y up, looking down -z), e.g. Minecraft's first-person transform chain.
func (ir *ItemRenderer) DrawHandModel(item string, aspect float32, model mgl32.Mat4, sky, block uint8) {
	if _, ok := ir.index[item]; !ok {
		return
	}
	clearHandDepth()
	proj := mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
	ir.begin(Fog{Color: mgl32.Vec3{}, Start: 1e6, End: 2e6}) // no fog on the hand
	ir.draw(item, proj, model, sky, block)
	ir.end()
}
