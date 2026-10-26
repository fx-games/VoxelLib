package bettercombat

// Better Combat 3.2.2's attack animations, played as the mod plays them
// through Player Animation Lib (palib.go: speed gears, mirror, the
// first-person pitch adjustment) over Minecraft's vanilla player pose
// (HumanoidModel, WalkAnimationState, LivingEntity's body yaw) and its
// render chain (LivingEntityRenderer with the lib's mixins,
// ItemInHandLayer, ItemTransform). Ported from VoxelRogue;
// combat_test.go checks it against Minecraft's own classes
// (testdata/vanilla.json).

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// display is an item model's display transform: rotation in degrees
// (applied X, then Y, then Z), translation in 1/16 blocks, scale.
type display struct{ rot, trans, scale mgl32.Vec3 }

// --- Better Combat's attack animation stack ---

// attackStack is AttackAnimationSubStack: the controller behind the speed,
// mirror and adjustment modifiers.
type attackStack struct {
	c      *controller
	speed  *speedMod
	mirror *mirrorMod
	pitch  float32 // the player's xRot in degrees, down positive
}

func newAttackStack() *attackStack {
	s := &attackStack{speed: &speedMod{speed: 1}, mirror: &mirrorMod{}}
	s.c = newController(s.speed, s.mirror, &adjustMod{source: s.adjustment})
	s.c.postSetup = func(b map[string]*advBone) {
		b["torso"].setEnabled(true)
		b["head"].enabled[axRX] = false
	}
	return s
}

// adjustment is AttackAnimationStack.createAttackAdjustment: the body
// follows the look pitch (and lifts a little looking up) in first person;
// body, arms and legs share it in third.
func (s *attackStack) adjustment(bone string, data *animData) (partMod, bool) {
	pitch := d2r(s.pitch)
	var m partMod
	if data.firstPerson {
		if bone != "body" {
			return m, false
		}
		m.rot[0] += pitch
		if pitch < 0 {
			off := math.Abs(math.Sin(float64(pitch)))
			m.off[1] = float32(off * 0.5)
			m.off[2] = -float32(off)
		}
		return m, true
	}
	switch bone {
	case "body":
		m.rot[0] += pitch * 0.75
	case "right_arm", "left_arm":
		m.rot[0] += pitch * 0.25
	case "right_leg", "left_leg":
		m.rot[0] = -float32(float64(pitch) * 0.75)
	default:
		return m, false
	}
	return m, true
}

// play is AbstractClientPlayerEntityMixin.playAttackAnimation: the
// animation's endTick spread over length ticks, the first upswing share
// faster (upswing multiplier 0.5) and the rest slower, then the end at
// the plain speed.
func (s *attackStack) play(a *animation, offHand bool, length, upswing float32) {
	endTick := a.length
	if a.end != nil {
		endTick = *a.end
	}
	speed := endTick / length
	upswingSpeed := speed / (upswing / 0.5)
	downwind := float32(float64(speed) * float64(1-upswing)) // lerp(0, 1-upswing, ...)
	s.speed.speed = speed
	s.mirror.enabled = offHand
	s.speed.set(upswingSpeed, []gear{{float32(length * upswing), downwind}, {length, speed}})
	s.c.trigger(a)
}

// playRaw plays a at its own speed, as Better Combat's previews do.
func (s *attackStack) playRaw(a *animation) {
	s.speed.speed = 1
	s.mirror.enabled = false
	s.speed.set(1, nil)
	s.c.trigger(a)
}

// handle is AvatarAnimManager.handleAnimations: the stack's tick at the end
// of the player's tick and its partial-tick setup before each frame.
func (s *attackStack) handle(pt float32, fullTick, firstPerson bool) {
	if fullTick {
		d := animData{pt: pt, firstPerson: firstPerson}
		s.c.tickLayers(&d)
	}
	if s.c.isActive() {
		d := animData{pt: pt, firstPerson: firstPerson}
		s.c.setupAnim(&d)
	}
}

// transform writes the animation over the vanilla bone b.
func (s *attackStack) transform(b *animBone) { s.c.transform(b) }

// --- Vanilla: Mth, WalkAnimationState, HumanoidModel, LivingEntity ---

var mthSinTab = func() (t [65536]float32) {
	for i := range t {
		t[i] = float32(math.Sin(float64(i) / 10430.378350470453))
	}
	return
}()

func mthSin(v float32) float32 { return mthSinTab[int64(float64(v)*10430.378350470453)&65535] }
func mthCos(v float32) float32 {
	return mthSinTab[int64(float64(v)*10430.378350470453+16384.0)&65535]
}

// wrapDegrees is Mth.wrapDegrees for a float.
func wrapDegrees(v float32) float32 {
	v = float32(math.Mod(float64(v), 360))
	if v >= 180 {
		v -= 360
	}
	if v < -180 {
		v += 360
	}
	return v
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

var asinTab, cosTab = func() (asin, cos [257]float64) {
	for i := range asin {
		asin[i] = math.Asin(float64(i) / 256)
		cos[i] = math.Cos(asin[i])
	}
	return
}()

// mthAtan2 is Mth.atan2, Minecraft's table approximation.
func mthAtan2(y, x float64) float64 {
	fracBias := math.Float64frombits(4805340802404319232)
	d2 := float64(x*x) + float64(y*y)
	if math.IsNaN(d2) {
		return math.NaN()
	}
	negY := y < 0
	if negY {
		y = -y
	}
	negX := x < 0
	if negX {
		x = -x
	}
	steep := y > x
	if steep {
		x, y = y, x
	}
	rinv := fastInvSqrt(d2)
	x *= rinv
	y *= rinv
	yp := fracBias + y
	idx := int(int32(math.Float64bits(yp)))
	phi, cPhi := asinTab[idx], cosTab[idx]
	sPhi := yp - fracBias
	sd := float64(y*cPhi) - float64(x*sPhi)
	d := (6 + float64(sd*sd)) * sd * 0.16666666666666666
	theta := phi + d
	if steep {
		theta = math.Pi/2 - theta
	}
	if negX {
		theta = math.Pi - theta
	}
	if negY {
		theta = -theta
	}
	return theta
}

// fastInvSqrt is Mth.fastInvSqrt.
func fastInvSqrt(x float64) float64 {
	xhalf := 0.5 * x
	i := int64(math.Float64bits(x))
	i = 6910469410427058090 - (i >> 1)
	x = math.Float64frombits(uint64(i))
	return x * (1.5 - float64(xhalf*x*x))
}

// walkState is WalkAnimationState.
type walkState struct{ speedOld, speed, position float32 }

func (w *walkState) update(target, factor, scale float32) {
	w.speedOld = w.speed
	w.speed += float32((target - w.speed) * factor)
	w.position += float32(w.speed * scale)
}

func (w *walkState) positionAt(pt float32) float32 { return w.position - float32(w.speed*(1-pt)) }
func (w *walkState) speedAt(pt float32) float32 {
	return min(w.speedOld+float32(pt*(w.speed-w.speedOld)), 1)
}

// part is a ModelPart's pose: x, y, z, xRot, yRot, zRot.
type part [6]float32

// The humanoid parts' initial poses.
var (
	initHead     = part{}
	initBody     = part{}
	initRightArm = part{-5, 2, 0}
	initLeftArm  = part{5, 2, 0}
	initRightLeg = part{-1.9, 12, 0}
	initLeftLeg  = part{1.9, 12, 0}
)

// vanillaPose is HumanoidModel.setupAnim for the player standing with an
// item in the right hand and nothing in the left, not attacking (Better
// Combat zeroes the vanilla attack time). xRot, yRot are the head's in
// degrees.
func vanillaPose(walkPos, walkSpeed, age, xRot, yRot float32) (head, body, rightArm, leftArm, rightLeg, leftLeg part) {
	head[3] = xRot * float32(math.Pi/180)
	head[4] = yRot * float32(math.Pi/180)
	rightArm, leftArm, rightLeg, leftLeg = initRightArm, initLeftArm, initRightLeg, initLeftLeg
	wp := float32(walkPos * 0.6662)
	rightArm[3] = mthCos(wp+piF) * 2 * walkSpeed * 0.5 / 1
	leftArm[3] = mthCos(wp) * 2 * walkSpeed * 0.5 / 1
	rightLeg[3] = mthCos(wp) * 1.4 * walkSpeed / 1
	leftLeg[3] = mthCos(wp+piF) * 1.4 * walkSpeed / 1
	rightLeg[4], leftLeg[4], rightLeg[5], leftLeg[5] = 0.005, -0.005, 0.005, -0.005
	// poseRightArm ITEM (poseLeftArm EMPTY does nothing)
	rightArm[3] = float32(rightArm[3]*0.5) - float32(math.Pi/10)
	rightArm[4] = 0
	// AnimationUtils.bobModelPart
	rightArm[5] += float32(mthCos(age*0.09)*0.05) + 0.05
	rightArm[3] += float32(mthSin(age*0.067) * 0.05)
	leftArm[5] += -(float32(mthCos(age*0.09)*0.05) + 0.05)
	leftArm[3] += -float32(mthSin(age*0.067) * 0.05)
	return
}

// vanillaBone is RenderUtil.copyVanillaPart: a part's pose as the bone the
// animation writes over (its position relative to the initial pose, y up).
func vanillaBone(name string, p, init part) *animBone {
	b := newAnimBone(name)
	b.v[axPX], b.v[axPY], b.v[axPZ] = p[0]-init[0], -(p[1] - init[1]), p[2]-init[2]
	b.v[axRX], b.v[axRY], b.v[axRZ] = p[3], p[4], p[5]
	return b
}

// bonePart is RenderUtil.translatePartToBone: the bone back as a part's pose.
func bonePart(b *animBone, init part) part {
	return part{b.v[axPX] + init[0], -b.v[axPY] + init[1], b.v[axPZ] + init[2], b.v[axRX], b.v[axRY] + init[4], b.v[axRZ]}
}

// tickBodyYaw is LivingEntity.tick's turning of the body after the head
// (yRot) and the tick's movement (xd, zd), then Better Combat's extra
// turn toward the head while an attack plays.
func (h *Humanoid) tickBodyYaw(yRot float32, xd, zd float64, attacking bool) {
	h.prevBodyYaw = h.bodyYaw
	sideDist := float32(float64(xd*xd) + float64(zd*zd))
	t := h.bodyYaw
	if sideDist > 0.0025000002 {
		walkDir := float32(float32(mthAtan2(zd, xd))*(180/piF)) - 90
		diff := abs32(wrapDegrees(yRot) - walkDir)
		if 95 < diff && diff < 265 {
			t = walkDir - 180
		} else {
			t = walkDir
		}
	}
	h.bodyYaw = tickHeadTurn(h.bodyYaw, yRot, t)
	for h.bodyYaw-h.prevBodyYaw < -180 {
		h.prevBodyYaw -= 360
	}
	for h.bodyYaw-h.prevBodyYaw >= 180 {
		h.prevBodyYaw += 360
	}
	if attacking {
		h.bodyYaw = tickHeadTurn(h.bodyYaw, yRot, yRot)
	}
}

// tickHeadTurn is LivingEntity.tickHeadTurn: the body turns 30% toward t
// and never more than 50° from the head.
func tickHeadTurn(bodyYaw, yRot, t float32) float32 {
	d := wrapDegrees(t - bodyYaw)
	bodyYaw += float32(d * 0.3)
	headDiff := wrapDegrees(yRot - bodyYaw)
	if abs32(headDiff) > 50 {
		sign := float32(0)
		if headDiff != 0 {
			sign = float32(math.Copysign(1, float64(headDiff)))
		}
		bodyYaw += headDiff - float32(sign*50)
	}
	return bodyYaw
}

// --- The render chain ---

const eyeHeight = 1.62 // the player's, standing

// rotateZYX is JOML's Matrix4f.rotateZYX: m * Rz * Ry * Rx.
func rotateZYX(m mgl32.Mat4, z, y, x float32) mgl32.Mat4 {
	return m.Mul4(mgl32.HomogRotate3DZ(z)).Mul4(mgl32.HomogRotate3DY(y)).Mul4(mgl32.HomogRotate3DX(x))
}

// partMat is ModelPart.translateAndRotate (unscaled).
func partMat(p part) mgl32.Mat4 {
	return rotateZYX(mgl32.Translate3D(p[0]/16, p[1]/16, p[2]/16), p[5], p[4], p[3])
}

// bcChain is the first-person render chain from the camera (at the eye,
// world orientation) to the model's root, the right arm and the weapon's
// model space (before the item model's own -0.5 centring):
// EntityRenderDispatcher, LivingEntityRenderer.submit with Player
// Animation Lib's doTranslations mixin, AvatarRenderer.scale,
// translateToHand with the lib's PlayerModelMixin, ItemInHandLayer with
// its ItemInHandLayerMixin and the model's thirdperson_righthand
// ItemTransform. body, arm and item are the animated bones, bodyRot the
// body yaw in degrees, disp the display's rotation (degrees), translation
// (sixteenths) and scale. headYaw (the head's yaw from the body) and
// xRot (the pitch), in degrees, turn the body about the eye with the
// camera: the lib's firstPersonFollowsCamera, which Better Combat leaves
// off, so that the weapon stays on screen looking up or down.
func bcChain(body, arm, item [axes]float32, bodyRot, headYaw, xRot float32, disp display) (root, armM, itemM mgl32.Mat4) {
	m := mgl32.Translate3D(0, -eyeHeight, 0)
	m = m.Mul4(mgl32.HomogRotate3DY((180 - bodyRot) * (piF / 180)))
	m = m.Mul4(mgl32.Scale3D(-1, -1, 1))
	// doTranslations (first person pass)
	m = m.Mul4(mgl32.Scale3D(-1, -1, 1)).Mul4(mgl32.Translate3D(0, eyeHeight, 0))
	m = m.Mul4(mgl32.HomogRotate3DY(-headYaw * (piF / 180))).Mul4(mgl32.HomogRotate3DX(-xRot * (piF / 180)))
	m = m.Mul4(mgl32.Translate3D(0, -eyeHeight, 0))
	m = m.Mul4(bodyBone(body)).Mul4(mgl32.Scale3D(-1, -1, 1))
	m = m.Mul4(mgl32.Scale3D(0.9375, 0.9375, 0.9375)).Mul4(mgl32.Translate3D(0, -1.501, 0))
	root = m
	a, i := armChain(arm, item, disp)
	return root, root.Mul4(a), root.Mul4(i)
}

// bodyBone is the lib's doTranslations of the body bone: the whole model
// moved and turned about its middle, in the entity's frame (y up, turned
// by the body yaw, at the feet).
func bodyBone(body [axes]float32) mgl32.Mat4 {
	m := mgl32.Translate3D(-body[axPX]/16, body[axPY]/16+0.75, body[axPZ]/16)
	m = rotateZYX(m, body[axRZ], -body[axRY], -body[axRX])
	return m.Mul4(mgl32.Scale3D(body[axSX], body[axSY], body[axSZ])).Mul4(mgl32.Translate3D(0, -0.75, 0))
}

// armChain is the render chain from the model's root (ModelPart space in
// blocks, y down, at the neck) to the right arm and the item in its hand:
// translateToHand with the lib's PlayerModelMixin, ItemInHandLayer with
// its ItemInHandLayerMixin and the item's thirdperson_righthand
// ItemTransform.
func armChain(arm, item [axes]float32, disp display) (armM, itemM mgl32.Mat4) {
	m := mgl32.Ident4()
	// translateToHand: the right arm part posed by its bone
	p := bonePart(&animBone{v: arm}, initRightArm)
	m = m.Mul4(mgl32.Translate3D(p[0]/16, p[1]/16, p[2]/16))
	if p[3] != 0 || p[4] != 0 || p[5] != 0 {
		m = rotateZYX(m, p[5], p[4], p[3])
	}
	m = m.Mul4(mgl32.Translate3D(0, (arm[axSY]-1)*0.609375, (arm[axSZ]-1)*0.0625))
	armM = m
	// ItemInHandLayer with the item bone's location, rotation and scale
	m = m.Mul4(mgl32.Translate3D(item[axPX]/16, -item[axPY]/16, item[axPZ]/16))
	m = m.Mul4(mgl32.HomogRotate3DX(-90 * (piF / 180))).Mul4(mgl32.HomogRotate3DY(180 * (piF / 180)))
	m = m.Mul4(mgl32.Translate3D(1.0/16, 2.0/16, -10.0/16))
	if item[axRZ] != 0 {
		m = m.Mul4(mgl32.HomogRotate3DZ(-item[axRY]))
	}
	if item[axRY] != 0 {
		m = m.Mul4(mgl32.HomogRotate3DY(-item[axRZ]))
	}
	if item[axRX] != 0 {
		m = m.Mul4(mgl32.HomogRotate3DX(-item[axRX]))
	}
	m = m.Mul4(mgl32.Scale3D(item[axSX], item[axSY], item[axSZ]))
	// ItemTransform.apply, right hand: translation in sixteenths clamped to ±5
	tr := disp.trans.Mul(0.0625)
	for i := range tr {
		tr[i] = min(max(tr[i], -5), 5)
	}
	m = m.Mul4(mgl32.Translate3D(tr[0], tr[1], tr[2]))
	m = m.Mul4(mgl32.HomogRotate3DX(disp.rot[0] * (piF / 180))).Mul4(mgl32.HomogRotate3DY(disp.rot[1] * (piF / 180))).Mul4(mgl32.HomogRotate3DZ(disp.rot[2] * (piF / 180)))
	itemM = m.Mul4(mgl32.Scale3D(disp.scale[0], disp.scale[1], disp.scale[2]))
	return
}

// attackPose is the player model posed by the attack animation: the held
// item's unit slab (render.ItemPose) and the model's root relative to the
