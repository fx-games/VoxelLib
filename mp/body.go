package mp

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/weapon"
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
// sneaking. The body faces the look yaw. Gun: a gun is in hand, held as
// Hold (TACZ's third-person animation) raised by Aim (the synced aiming
// progress), else the crossbow is raised, unless the hand is Bare.
type Anim struct {
	Pitch                 float64
	LimbSwing, LimbAmount float64
	Sneak                 bool
	Bare                  bool // nothing in hand: the arms swing with the walk
	Gun                   bool
	Hold                  weapon.ThirdPersonHold
	Aim                   weapon.AimProgress
	// Age is the server tick the pose is at, the idle arm bob's clock: the
	// server records its tick, clients draw at Client.RenderTime, so the
	// arrow hit body and the drawing bob as one.
	Age float64
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
		LimbAmount: a.LimbAmount + (b.LimbAmount-a.LimbAmount)*t, Sneak: b.Sneak, Bare: b.Bare, Gun: b.Gun, Hold: b.Hold,
		Aim: a.Aim + (b.Aim-a.Aim)*weapon.AimProgress(t), Age: a.Age + (b.Age-a.Age)*t}
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

// TACZ ThirdPersonManager's animations: the arms' pitches (added to the
// head's) and yaws. The default one lerps from hold to full aim.
const (
	gunHoldArmPitch    = -1.4
	gunAimArmPitch     = -1.6
	gunHoldRightArmYaw = -0.3
	gunAimRightArmYaw  = -0.35
	gunHoldLeftArmYaw  = 0.8

	minigunBodyYaw     = 0.8 // the body turns, the arms' pivots with it
	minigunRightArmYaw = -1.0
	minigunRightArmPit = -0.1 // added to the body's pitch
	minigunLeftArmYaw  = -0.1
	minigunLeftArmPit  = -1.0
)

// Skin poses the body for a (ModelBiped.setRotationAngles with aimedBow:
// the crossbow is always raised; with a gun, TACZ's third-person animation
// at the end of setupAnim, so it overrides sneaking's arm pitch) and
// returns its skinning matrices.
func (b *Body) Skin(a Anim, dst []mgl32.Mat4) []mgl32.Mat4 {
	pp, head := b.legs(a)
	switch {
	case a.Bare:
		// HumanoidModel.setupAnim: the arms swing against the legs.
		sw, amt := float32(a.LimbSwing*0.6662), float32(a.LimbAmount)
		pp[partRightArm].Rot[0] = cos(sw+math.Pi) * amt
		pp[partLeftArm].Rot[0] = cos(sw) * amt
	case !a.Gun:
		// aimedBow: both arms forward along the look, the left one across.
		pp[partRightArm].Rot = [3]float32{-math.Pi/2 + head, -0.1, 0}
		pp[partLeftArm].Rot = [3]float32{-math.Pi/2 + head, 0.1 + 0.4, 0}
	}
	b.sneak(a)
	bobArms(pp, a.Age)
	if a.Gun {
		b.gunHold(a, head)
	}
	return b.pose.Skin(b.Model, b.Geo, dst)
}

// gunHold is InnerThirdPersonManager.playVanillaAnimation: the hold's
// animateGunHold at no aim, else animateGunAim. The head faces the body
// (netHeadYaw 0: the body follows the look yaw).
func (b *Body) gunHold(a Anim, head float32) {
	pp := b.pose.Parts
	ra, la, body := &pp[partRightArm], &pp[partLeftArm], &pp[partBody]
	switch a.Hold {
	case weapon.HoldMinigun: // animateGunAim is animateGunHold
		body.Rot[1] = minigunBodyYaw
		c, s := cos(-body.Rot[1]), sin(-body.Rot[1])
		ra.Pivot[0], ra.Pivot[2] = ra.Pivot[0]*c, ra.Pivot[0]*s
		la.Pivot[0], la.Pivot[2] = la.Pivot[0]*c, la.Pivot[0]*s
		ra.Rot[0], ra.Rot[1] = minigunRightArmPit+body.Rot[0], minigunRightArmYaw+body.Rot[1]
		la.Rot[0], la.Rot[1] = minigunLeftArmPit+body.Rot[0], minigunLeftArmYaw+body.Rot[1]
	default:
		pitch, rightYaw := float32(gunHoldArmPitch), float32(gunHoldRightArmYaw)
		if t := float32(a.Aim); t > 0 {
			pitch, rightYaw = lerp(t, gunHoldArmPitch, gunAimArmPitch), lerp(t, gunHoldRightArmYaw, gunAimRightArmYaw)
		}
		ra.Rot[0], ra.Rot[1] = pitch+head, rightYaw
		la.Rot[0], la.Rot[1] = pitch+head, gunHoldLeftArmYaw
	}
}

func lerp(t, a, b float32) float32 { return a + (b-a)*t }

// ZombieSkin poses the body as ZombieModel: the walk cycle, then the arms
// held out (AnimationUtils.animateZombieArms) raised higher when aggressive,
// swinging with attack (0..1 through a blow) and bobbing with age (ticks).
func (b *Body) ZombieSkin(a Anim, aggressive bool, attack, age float64, dst []mgl32.Mat4) []mgl32.Mat4 {
	pp, _ := b.legs(a)
	f := float32(math.Sin(attack * math.Pi))
	f1 := float32(math.Sin((1 - (1-attack)*(1-attack)) * math.Pi))
	raise := float32(2.25)
	if aggressive {
		raise = 1.5
	}
	x := -math.Pi/raise + f*1.2 - f1*0.4
	pp[partRightArm].Rot = [3]float32{x, -(0.1 - f*0.6), 0}
	pp[partLeftArm].Rot = [3]float32{x, 0.1 - f*0.6, 0}
	bobArms(pp, age)
	b.sneak(a)
	return b.pose.Skin(b.Model, b.Geo, dst)
}

// legs resets the pose and sets the head pitch and the walk cycle; it
// returns the parts and the head's pitch.
func (b *Body) legs(a Anim) ([]cuboid.PartPose, float32) {
	p := b.pose
	p.Reset(b.Geo)
	pp := p.Parts
	head := float32(-a.Pitch) // the reference pitches down positive
	pp[partHead].Rot[0] = head
	sw, amt := float32(a.LimbSwing*0.6662), float32(a.LimbAmount)
	pp[partRightLeg].Rot[0] = cos(sw) * 1.4 * amt
	pp[partLeftLeg].Rot[0] = cos(sw+math.Pi) * 1.4 * amt
	return pp, head
}

// bobArms is AnimationUtils.bobModelPart on both arms, age in ticks.
func bobArms(pp []cuboid.PartPose, age float64) {
	bz := float32(math.Cos(age*0.09))*0.05 + 0.05
	bx := float32(math.Sin(age*0.067)) * 0.05
	pp[partRightArm].Rot[2] += bz
	pp[partLeftArm].Rot[2] -= bz
	pp[partRightArm].Rot[0] += bx
	pp[partLeftArm].Rot[0] -= bx
}

// sneakRenderOffset is PlayerRenderer.getRenderOffset while crouching
// (0.125 blocks down), in model pixels (y down).
const sneakRenderOffset = 0.125 * 16

// sneak crouches the posed body when a sneaks: 1.20 HumanoidModel.setupAnim
// plus the render offset, applied to the root parts so hit tests see it too.
func (b *Body) sneak(a Anim) {
	if !a.Sneak {
		return
	}
	pp := b.pose.Parts
	pp[partBody].Rot[0] = 0.5
	pp[partRightArm].Rot[0] += 0.4
	pp[partLeftArm].Rot[0] += 0.4
	pp[partRightLeg].Pivot[1], pp[partRightLeg].Pivot[2] = 12.2, 4
	pp[partLeftLeg].Pivot[1], pp[partLeftLeg].Pivot[2] = 12.2, 4
	pp[partHead].Pivot[1] = 4.2
	pp[partBody].Pivot[1] = 3.2
	pp[partRightArm].Pivot[1], pp[partLeftArm].Pivot[1] = 5.2, 5.2
	for i := range pp {
		if b.Geo.Parts[i].Parent < 0 {
			pp[i].Pivot[1] += sneakRenderOffset
		}
	}
}

func cos(x float32) float32 { return float32(math.Cos(float64(x))) }
func sin(x float32) float32 { return float32(math.Sin(float64(x))) }

// Hand is the right hand's position in model space (the bottom of the
// right arm) in the pose skin, where a held item goes.
func (b *Body) Hand(skin []mgl32.Mat4) mgl32.Vec3 {
	return mgl32.TransformCoordinate(mgl32.Vec3{-1, 10, 0}, b.RightArm(skin))
}

// RightArm maps the right arm's part space (the reference's ModelPart
// space, pixels, y down) to model space in the pose skin: translateToHand.
func (b *Body) RightArm(skin []mgl32.Mat4) mgl32.Mat4 {
	arm := b.Model.Bones[partRightArm].InvBind.Inv()
	if partRightArm < len(skin) {
		arm = skin[partRightArm].Mul4(arm)
	}
	return arm
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
