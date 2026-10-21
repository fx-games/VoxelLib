package render

import (
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// MaxBones is the most bones a model drawn by ModelRenderer may have.
const MaxBones = 96

// ModelRenderer draws animated models (package model) with GPU skinning,
// lit by the world's sky/block light like the terrain and fogged alike.
type ModelRenderer struct {
	r      *Renderer
	gpu    gpuModelRenderer
	models []*gpuModel
}

type gpuPart struct {
	first, count int32 // indices
	tex          int
}

// ModelInstance is one model to draw.
type ModelInstance struct {
	Model      int          // handle from Add
	X, Y, Z    float64      // world position of the model origin
	Yaw        float64      // radians, as Camera.Yaw (0 faces -Z... see Draw)
	Pitch      float64      // radians, tilts the model's +Z up (as Camera.Pitch)
	Scale      float32      // 0 means 1
	Skin       []mgl32.Mat4 // from model.Animator.Skin; nil = bind pose
	Sky, Block uint8        // light levels at the model
	// Tilt tips the whole model over by Tilt radians toward the world
	// direction TiltYaw (yaw convention), pivoting at its origin, e.g. a
	// falling corpse.
	Tilt, TiltYaw float64
	// Fade dissolves the model: 0 is solid, 1 is gone (a dither pattern
	// discards that share of its pixels, so no blending or sorting).
	Fade float32
	// Transform, when set, replaces Yaw/Pitch/Tilt/Scale: it maps model
	// space to the world about X,Y,Z (e.g. a weapon in a creature's hand).
	Transform *mgl32.Mat4
}

// TiltMatrix rotates the vertical toward world direction yaw by angle
// radians, about the horizontal axis across that direction.
func TiltMatrix(angle, yaw float64) mgl32.Mat4 {
	if angle == 0 {
		return mgl32.Ident4()
	}
	fx, fz := math.Sin(yaw), -math.Cos(yaw)
	return mgl32.HomogRotate3D(float32(angle), mgl32.Vec3{float32(fz), 0, float32(-fx)})
}

// modelStride is the model vertex size in floats; joints are packed as 4
// bytes into one float slot.
const modelStride = 3 + 3 + 2 + 1 + 4

// modelVertices flattens a model's meshes into one vertex and index buffer.
func modelVertices(m *model.Model) (vb []float32, ib []uint32, parts []gpuPart) {
	for _, me := range m.Meshes {
		base := uint32(len(vb) / modelStride)
		first := int32(len(ib))
		for _, v := range me.Verts {
			j := uint32(v.Joints[0]) | uint32(v.Joints[1])<<8 | uint32(v.Joints[2])<<16 | uint32(v.Joints[3])<<24
			w := v.Weights
			if w == ([4]float32{}) {
				w[0] = 1
			}
			vb = append(vb, v.Pos[0], v.Pos[1], v.Pos[2], v.Normal[0], v.Normal[1], v.Normal[2], v.UV[0], v.UV[1],
				math.Float32frombits(j), w[0], w[1], w[2], w[3])
		}
		for _, i := range me.Indices {
			ib = append(ib, base+i)
		}
		parts = append(parts, gpuPart{first, int32(len(me.Indices)), me.Texture})
	}
	return vb, ib, parts
}

// NewModelRenderer creates an empty model renderer.
func NewModelRenderer(r *Renderer) (*ModelRenderer, error) {
	mr := &ModelRenderer{r: r}
	if err := mr.initGPU(); err != nil {
		return nil, err
	}
	return mr, nil
}

// Add uploads a model and returns its handle.
func (mr *ModelRenderer) Add(m *model.Model) (int, error) {
	if len(m.Bones) > MaxBones {
		return 0, fmt.Errorf("render: model %s has %d bones (max %d)", m.Name, len(m.Bones), MaxBones)
	}
	vb, ib, parts := modelVertices(m)
	texs := m.Textures
	if len(texs) == 0 {
		texs = []image.Image{whiteImage()}
	}
	g := mr.upload(vb, ib, texs)
	g.parts = parts
	for i, old := range mr.models { // reuse a slot freed by Remove
		if old == nil {
			mr.models[i] = g
			return i, nil
		}
	}
	mr.models = append(mr.models, g)
	return len(mr.models) - 1, nil
}

// Remove frees a model's GPU buffers and textures; its handle may be
// reissued by a later Add.
func (mr *ModelRenderer) Remove(handle int) {
	if handle < 0 || handle >= len(mr.models) || mr.models[handle] == nil {
		return
	}
	mr.models[handle].delete()
	mr.models[handle] = nil
}

func whiteImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	copy(img.Pix, []byte{255, 255, 255, 255})
	return img
}

// identBones is the bind pose.
var identBones = func() (b [MaxBones]mgl32.Mat4) {
	for i := range b {
		b[i] = mgl32.Ident4()
	}
	return
}()

// bones returns the skinning matrices to upload: skin, or the bind pose.
func bones(skin []mgl32.Mat4) []mgl32.Mat4 {
	if len(skin) > 0 {
		return skin[:min(len(skin), MaxBones)]
	}
	return identBones[:]
}

// modelLight is the uniform state shared by a batch of model draws.
type modelLight struct {
	vp              mgl32.Mat4
	fog             Fog
	daylight, block float32
}

// Draw renders model instances. Call after Renderer.Render with the same
// camera. A model faces +Z in model space; Yaw turns it like the camera, so
// an instance with the camera's yaw faces the same way as the camera.
func (mr *ModelRenderer) Draw(cam *Camera, insts []ModelInstance) {
	if len(insts) == 0 {
		return
	}
	vp := cam.ViewProj()
	mr.begin(modelLight{vp, mr.r.CurrentFog(), mr.r.Opt.Daylight, mr.r.Opt.BlockBrightness})
	fr := makeFrustum(vp)
	for _, in := range insts {
		if in.Model < 0 || in.Model >= len(mr.models) || mr.models[in.Model] == nil {
			continue
		}
		dx, dy, dz := float32(in.X-cam.X), float32(in.Y-cam.Y), float32(in.Z-cam.Z)
		if !fr.box(dx-3, dy-1, dz-3, dx+3, dy+4, dz+3) {
			continue
		}
		g := mr.models[in.Model]
		s := in.Scale
		if s == 0 {
			s = 1
		}
		// Model +Z must turn to the camera's forward at yaw: forward is
		// (sin yaw, 0, -cos yaw), which is +Z rotated by pi - yaw about Y.
		// Pitch then tilts +Z up about the model's X axis.
		if in.Transform != nil {
			mr.drawModel(g, mgl32.Translate3D(dx, dy, dz).Mul4(*in.Transform), in.Skin, in.Sky, in.Block, in.Fade)
			continue
		}
		model := mgl32.Translate3D(dx, dy, dz).Mul4(TiltMatrix(in.Tilt, in.TiltYaw)).Mul4(mgl32.HomogRotate3DY(float32(math.Pi - in.Yaw))).
			Mul4(mgl32.HomogRotate3DX(float32(-in.Pitch))).Mul4(mgl32.Scale3D(s, s, s))
		mr.drawModel(g, model, in.Skin, in.Sky, in.Block, in.Fade)
	}
	mr.end()
}

// handPass is set between BeginHands and EndHands: the hand draws share
// one depth buffer instead of each clearing it.
var handPass bool

// BeginHands starts drawing several first-person pieces (arms, held items)
// that hide one another by depth: it clears the depth buffer once, and the
// DrawHand calls until EndHands keep it.
func BeginHands() {
	clearDepth()
	handPass = true
}

// EndHands ends BeginHands.
func EndHands() { handPass = false }

// clearHandDepth clears the depth buffer for a hand draw, unless it is
// part of BeginHands.
func clearHandDepth() {
	if !handPass {
		clearDepth()
	}
}

// DrawHand draws a model in first person, over the world (depth is cleared)
// with a fixed 70° projection like ItemRenderer.DrawHand. view places the
// model in view space (camera at the origin looking down -Z, +Y up).
func (mr *ModelRenderer) DrawHand(handle int, aspect float32, view mgl32.Mat4, sky, block uint8) {
	mr.DrawHandSkinned(handle, aspect, view, nil, sky, block)
}

// DrawHandSkinned is DrawHand posed by skin (from model.Animator.Skin).
func (mr *ModelRenderer) DrawHandSkinned(handle int, aspect float32, view mgl32.Mat4, skin []mgl32.Mat4, sky, block uint8) {
	if handle < 0 || handle >= len(mr.models) {
		return
	}
	clearHandDepth()
	proj := mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
	mr.begin(modelLight{proj, Fog{Start: 1e6, End: 2e6}, mr.r.Opt.Daylight, mr.r.Opt.BlockBrightness}) // no fog on the hand
	mr.drawModel(mr.models[handle], view, skin, sky, block, 0)
	mr.end()
}

func toNRGBARect(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
