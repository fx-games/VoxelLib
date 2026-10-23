package mcjava

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// PartPose is a ModelRenderer's animated state.
type PartPose struct {
	Pivot  [3]float32 // rotationPoint, pixels
	Rot    [3]float32 // rotateAngle, radians
	Offset [3]float32 // offsetX/Y/Z, blocks
	Hidden bool       // isHidden or !showModel: the part and its children are not drawn
	// Pre is the GL transform render() applies around this part's
	// render call (glPushMatrix; glScalef...; part.render(); glPopMatrix),
	// in the parent's frame in GL units (blocks), e.g. LOTR dwarves'
	// glScalef(1.25,1,1) around the body. Zero means identity.
	Pre mgl32.Mat4
	// Post is a GL transform applied after the part's rotation, to its
	// boxes and children: ModelScaleRenderer.render's glScalef(config)
	// after postRender (CustomNPC's scaled limbs). Zero means identity.
	Post mgl32.Mat4
	// Own makes a copy part (Part.Same) use its own Pivot, Rot, Offset and
	// Hidden instead of its original's, for render() code that changes a
	// part between repeated draws (the CustomNPC dragon's neck). A copy
	// always uses its own Pre.
	Own bool
}

// Pose is everything that places a model's parts in one frame.
type Pose struct {
	Parts []PartPose
	// Root is preRenderCallback's transform (after glScalef(-1,-1,1),
	// before the lift), e.g. glScalef(0.9375,...). Zero means identity.
	Root mgl32.Mat4
	// Corpse is rotateCorpse's transform after the yaw turn (death
	// tipping, Dinnerbone), in Minecraft's entity frame. Zero means
	// identity.
	Corpse mgl32.Mat4
}

// RestPose returns the pose the constructor left the parts in.
func (g *Geometry) RestPose() *Pose {
	p := &Pose{Parts: make([]PartPose, len(g.Parts))}
	p.Reset(g)
	return p
}

// Reset puts every part back to its constructed state and clears the
// GL transforms. Models whose setRotationAngles only sets some fields
// rely on the rest of the state persisting between frames, as in
// Minecraft; call Reset only where a model's code would start fresh.
func (p *Pose) Reset(g *Geometry) {
	if len(p.Parts) != len(g.Parts) {
		p.Parts = make([]PartPose, len(g.Parts))
	}
	for i, gp := range g.Parts {
		p.Parts[i] = PartPose{Pivot: gp.Pivot, Rot: gp.Rot, Offset: gp.Offset, Hidden: gp.Hidden}
	}
	p.Root, p.Corpse = mgl32.Mat4{}, mgl32.Mat4{}
}

func orIdent(m mgl32.Mat4) mgl32.Mat4 {
	if m == (mgl32.Mat4{}) {
		return mgl32.Ident4()
	}
	return m
}

// World computes each part's world matrix in model space (model package
// conventions), as ModelRenderer.render would place it. Hidden parts and
// their children get a zero-scale matrix.
func (p *Pose) World(g *Geometry, dst []mgl32.Mat4) []mgl32.Mat4 {
	n := len(g.Parts)
	if cap(dst) < n {
		dst = make([]mgl32.Mat4, n)
	}
	dst = dst[:n]
	// Ry(180) · corpse · glScalef(-1,-1,1) · root · lift · scale(1/16)
	ry := mgl32.HomogRotate3DY(math.Pi)
	base := ry.Mul4(orIdent(p.Corpse)).Mul4(mgl32.Scale3D(-1, -1, 1)).Mul4(orIdent(p.Root)).
		Mul4(mgl32.Translate3D(0, -Lift, 0))
	hidden := make([]bool, n)
	for i, gp := range g.Parts {
		pp := p.Parts[i]
		if gp.Same > 0 && !pp.Own {
			pre := pp.Pre
			pp = p.Parts[gp.Same-1] // a copy moves with its original (and its Post)
			pp.Pre = pre
		}
		parent := base
		if gp.Parent >= 0 {
			parent = dst[gp.Parent]
			hidden[i] = hidden[gp.Parent]
		}
		hidden[i] = hidden[i] || pp.Hidden
		if hidden[i] {
			dst[i] = mgl32.Scale3D(0, 0, 0)
			continue
		}
		// Parent frames are in blocks for roots (GL units) and pixels below:
		// ModelRenderer translates by rotationPoint*scale, so children of a
		// part live in the part's frame scaled by 1/16 as well. Work in GL
		// units throughout: T(offset) · T(pivot/16) · Rz·Ry·Rx, boxes /16.
		local := orIdent(pp.Pre).Mul4(mgl32.Translate3D(pp.Offset[0], pp.Offset[1], pp.Offset[2])).
			Mul4(mgl32.Translate3D(pp.Pivot[0]/16, pp.Pivot[1]/16, pp.Pivot[2]/16)).
			Mul4(eulerMC(pp.Rot).Mat4()).Mul4(orIdent(pp.Post))
		dst[i] = parent.Mul4(local)
	}
	// The bones' frames are in pixels: append the 1/16 box scale.
	s := mgl32.Scale3D(1.0/16, 1.0/16, 1.0/16)
	for i := range dst {
		if !hidden[i] {
			dst[i] = dst[i].Mul4(s)
		}
	}
	return dst
}

// Skin computes skinning matrices for a model built from g by Build.
func (p *Pose) Skin(m *model.Model, g *Geometry, dst []mgl32.Mat4) []mgl32.Mat4 {
	return m.SkinWorld(p.World(g, nil), dst)
}

// Angles sets a part's rotation.
func (p *Pose) Angles(part int, x, y, z float32) { p.Parts[part].Rot = [3]float32{x, y, z} }

// Point sets a part's rotation point.
func (p *Pose) Point(part int, x, y, z float32) { p.Parts[part].Pivot = [3]float32{x, y, z} }

// GL helpers for porting render() code: they build the same matrices as
// the GL11 calls (angles in degrees, like glRotatef).

// Translate is glTranslatef.
func Translate(x, y, z float32) mgl32.Mat4 { return mgl32.Translate3D(x, y, z) }

// Scale is glScalef.
func Scale(x, y, z float32) mgl32.Mat4 { return mgl32.Scale3D(x, y, z) }

// Rotate is glRotatef(deg, x, y, z).
func Rotate(deg, x, y, z float32) mgl32.Mat4 {
	return mgl32.HomogRotate3D(deg*math.Pi/180, mgl32.Vec3{x, y, z}.Normalize())
}
