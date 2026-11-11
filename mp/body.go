package mp

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model"
)

// PlayerGeometry is the reference's ModelPlayer (1.8, 64x64 skins, wide
// arms): head, body, arms and legs, each with its second-layer overlay
// (hat, jacket, sleeves, trousers) as a child 0.5 or 0.25 pixels larger.
func PlayerGeometry() *cuboid.Geometry {
	tex := [2]float32{64, 64}
	g := &cuboid.Geometry{Name: "player", TexSize: [2]int{64, 64}}
	limb := func(name string, pivot [3]float32, uv, overlayUV [2]int, pos [3]float32, size [3]int, inflate float32) {
		g.Parts = append(g.Parts,
			cuboid.Part{Name: name, Parent: -1, Pivot: pivot,
				Boxes: []cuboid.Box{{Pos: pos, Size: size, UV: uv, TexSize: tex}}},
			cuboid.Part{Name: name + "Wear", Parent: len(g.Parts),
				Boxes: []cuboid.Box{{Pos: pos, Size: size, Inflate: inflate, UV: overlayUV, TexSize: tex}}})
		g.Render = append(g.Render, len(g.Parts)-2)
	}
	limb("head", [3]float32{}, [2]int{0, 0}, [2]int{32, 0}, [3]float32{-4, -8, -4}, [3]int{8, 8, 8}, 0.5)
	limb("body", [3]float32{}, [2]int{16, 16}, [2]int{16, 32}, [3]float32{-4, 0, -2}, [3]int{8, 12, 4}, 0.25)
	limb("rightArm", [3]float32{-5, 2, 0}, [2]int{40, 16}, [2]int{40, 32}, [3]float32{-3, -2, -2}, [3]int{4, 12, 4}, 0.25)
	limb("leftArm", [3]float32{5, 2, 0}, [2]int{32, 48}, [2]int{48, 48}, [3]float32{-1, -2, -2}, [3]int{4, 12, 4}, 0.25)
	limb("rightLeg", [3]float32{-1.9, 12, 0}, [2]int{0, 16}, [2]int{0, 32}, [3]float32{-2, 0, -2}, [3]int{4, 12, 4}, 0.25)
	limb("leftLeg", [3]float32{1.9, 12, 0}, [2]int{16, 48}, [2]int{0, 48}, [3]float32{-2, 0, -2}, [3]int{4, 12, 4}, 0.25)
	return g
}

// Part indices in PlayerGeometry (each overlay follows its part).
const (
	partHead     = 0
	partBody     = 2
	partRightArm = 4
	partLeftArm  = 6
	partRightLeg = 8
	partLeftLeg  = 10
)

// Anim is what poses a player's body: the look pitch (radians, up
// positive), the walk cycle (ModelBiped's limbSwing and limbSwingAmount) and
// sneaking. The body faces the look yaw.
type Anim struct {
	Pitch                 float64
	LimbSwing, LimbAmount float64
	Sneak                 bool
}

// Walk advances the walk cycle by one tick in which the body moved dx, dz
// (EntityLivingBase.onUpdate's limbSwing update).
func (a *Anim) Walk(dx, dz float64) {
	d := math.Min(1, math.Hypot(dx, dz)*4)
	a.LimbAmount += (d - a.LimbAmount) * 0.4
	a.LimbSwing += a.LimbAmount
}

// Advance runs the walk cycle over a frame of ticks (fractional) in which
// the body moved dx, dz: Walk at frame rate, for bodies drawn from
// interpolated positions.
func (a *Anim) Advance(dx, dz, ticks float64) {
	if ticks <= 0 {
		return
	}
	d := math.Min(1, math.Hypot(dx, dz)/ticks*4)
	a.LimbAmount += (d - a.LimbAmount) * math.Min(1, 0.4*ticks)
	a.LimbSwing += a.LimbAmount * ticks
}

// Lerp blends two animation states (the previous and current tick).
func (a Anim) Lerp(b Anim, t float64) Anim {
	return Anim{Pitch: a.Pitch + (b.Pitch-a.Pitch)*t, LimbSwing: a.LimbSwing + (b.LimbSwing-a.LimbSwing)*t,
		LimbAmount: a.LimbAmount + (b.LimbAmount-a.LimbAmount)*t, Sneak: b.Sneak}
}

// Body is the player model shared by drawing and hit tests, so an arrow hits
// exactly what is drawn: a bone per part, no texture (each player draws it
// with their skin).
type Body struct {
	Geo   *cuboid.Geometry
	Model *model.Model
	pose  *cuboid.Pose
}

// NewBody builds the player model.
func NewBody() *Body {
	g := PlayerGeometry()
	m, err := cuboid.Build(g, nil)
	if err != nil {
		panic(err) // the geometry is constant
	}
	return &Body{Geo: g, Model: m, pose: g.RestPose()}
}

// Skin poses the body for a (ModelBiped.setRotationAngles with aimedBow:
// the crossbow is always raised) and returns its skinning matrices.
func (b *Body) Skin(a Anim, dst []mgl32.Mat4) []mgl32.Mat4 {
	p := b.pose
	p.Reset(b.Geo)
	pp := p.Parts
	head := float32(-a.Pitch) // the reference pitches down positive
	pp[partHead].Rot[0] = head
	sw, amt := float32(a.LimbSwing*0.6662), float32(a.LimbAmount)
	pp[partRightLeg].Rot[0] = cos(sw) * 1.4 * amt
	pp[partLeftLeg].Rot[0] = cos(sw+math.Pi) * 1.4 * amt
	// aimedBow: both arms forward along the look, the left one across.
	pp[partRightArm].Rot = [3]float32{-math.Pi/2 + head, -0.1, 0}
	pp[partLeftArm].Rot = [3]float32{-math.Pi/2 + head, 0.1 + 0.4, 0}
	if a.Sneak {
		pp[partBody].Rot[0] = 0.5
		pp[partRightArm].Rot[0] += 0.4
		pp[partLeftArm].Rot[0] += 0.4
		pp[partRightLeg].Pivot[1], pp[partRightLeg].Pivot[2] = 9, 4
		pp[partLeftLeg].Pivot[1], pp[partLeftLeg].Pivot[2] = 9, 4
		pp[partHead].Pivot[1] = 1
	}
	return p.Skin(b.Model, b.Geo, dst)
}

func cos(x float32) float32 { return float32(math.Cos(float64(x))) }

// Hand is the right hand's position in model space (the bottom of the
// right arm) in the pose skin, where a held item goes.
func (b *Body) Hand(skin []mgl32.Mat4) mgl32.Vec3 {
	arm := b.Model.Bones[partRightArm].InvBind.Inv()
	if partRightArm < len(skin) {
		arm = skin[partRightArm].Mul4(arm)
	}
	return mgl32.TransformCoordinate(mgl32.Vec3{-1, 10, 0}, arm)
}

// IsHead reports whether a bone (Model part) is the head or the hat.
func IsHead(bone int) bool { return bone == partHead || bone == partHead+1 }

// Matrix places the body's model space in the world at feet x,y,z facing
// yaw, as the model renderer does.
func Matrix(x, y, z, yaw float64) mgl32.Mat4 {
	return mgl32.Translate3D(float32(x), float32(y), float32(z)).Mul4(mgl32.HomogRotate3DY(float32(math.Pi - yaw)))
}

// Raycast intersects the segment p0→p1 (world space) with the body posed by
// skin at x,y,z,yaw and returns the hit point and the bone hit.
func (b *Body) Raycast(skin []mgl32.Mat4, pose *model.Pose, x, y, z, yaw float64, p0, p1 [3]float64) (hit [3]float64, bone int, ok bool) {
	m := Matrix(x, y, z, yaw)
	inv := m.Inv()
	o := mgl32.TransformCoordinate(mgl32.Vec3{float32(p0[0]), float32(p0[1]), float32(p0[2])}, inv)
	e := mgl32.TransformCoordinate(mgl32.Vec3{float32(p1[0]), float32(p1[1]), float32(p1[2])}, inv)
	pose = b.Model.Pose(skin, pose)
	h, ok := b.Model.RaycastPose(pose, o, e.Sub(o), 1)
	if !ok {
		return hit, -1, false
	}
	w := mgl32.TransformCoordinate(h.Pos, m)
	return [3]float64{float64(w[0]), float64(w[1]), float64(w[2])}, h.Bone, true
}

// FirstPersonArms returns skinning matrices that draw only the right
// (right true) or left arm and its sleeve as PlayerRenderer.renderHand does,
// in the reference's model space (pixels/16 in blocks, Y down) rather than
// the body's: setupAnim at age 0 leaves bobArms' ±0.1 roll, xRot is reset
// to 0. A held-item renderer multiplies them by the hand frame (TACZ's
// lefthand_pos/righthand_pos).
func (b *Body) FirstPersonArms(right bool, dst []mgl32.Mat4) []mgl32.Mat4 {
	p := b.pose
	p.Reset(b.Geo)
	arm, roll := partLeftArm, float32(-0.1)
	if right {
		arm, roll = partRightArm, 0.1
	}
	for i := range p.Parts {
		p.Parts[i].Hidden = b.Geo.Parts[i].Parent < 0 && i != arm
	}
	p.Parts[arm].Rot = [3]float32{0, 0, roll}
	dst = p.Skin(b.Model, b.Geo, dst)
	// undo the entity placement: World's base is Ry(180)·S(-1,-1,1)·T(0,-Lift) = S(1,-1,-1)·T(0,-Lift)
	unbase := mgl32.Translate3D(0, cuboid.Lift, 0).Mul4(mgl32.Scale3D(1, -1, -1))
	for i := range dst {
		dst[i] = unbase.Mul4(dst[i])
	}
	return dst
}
