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

// DrawHand draws the held item in the lower right of the view, over the
// world (depth is cleared) with its own fixed 70° projection, like
// Minecraft's first-person item. aspect is the viewport's width/height.
func (ir *ItemRenderer) DrawHand(item string, aspect float32, pose HandPose, sky, block uint8) {
	sw := float32(math.Sin(float64(pose.Swing) * math.Pi))
	sw2 := float32(math.Sin(math.Sqrt(float64(pose.Swing)) * math.Pi))
	model := pose.Base().Mul4(mgl32.Translate3D(0.5-0.25*sw2, -0.38+0.15*sw2, -0.9-0.15*sw)).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(-70 + 20*sw))).
		Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(20))).
		Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-80 * sw2))).
		Mul4(mgl32.Scale3D(0.42, 0.42, 0.42))
	ir.DrawHandModel(item, aspect, model, sky, block)
}

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
