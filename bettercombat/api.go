// Package bettercombat plays Better Combat's weapon attack animations on a
// Minecraft player-shaped body (combat.go, palib.go).
package bettercombat

import (
	"embed"
	"io/fs"
	"math"
	"os"
	"path"

	"github.com/go-gl/mathgl/mgl32"
)

// Animation is a loaded attack animation (a player-animator emote).
type Animation = animation

//go:embed anims/*.json
var builtin embed.FS

// Builtin is Better Combat's attack animations (anims/*.json), by name:
// one_handed_slash_horizontal_right, two_handed_stab_right and the rest.
func Builtin() map[string]*Animation {
	anims, err := load(builtin, "anims")
	if err != nil {
		panic(err)
	}
	return anims
}

// Load reads the animations in dir (*.json), by name.
func Load(dir string) (map[string]*Animation, error) { return load(os.DirFS(dir), ".") }

func load(fsys fs.FS, dir string) (map[string]*Animation, error) {
	files, err := fs.Glob(fsys, path.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	anims := map[string]*Animation{}
	for _, f := range files {
		a, err := loadPlayerAnim(fsys, f)
		if err != nil {
			return nil, err
		}
		anims[a.name] = a
	}
	return anims, nil
}

// Display is an item model's display transform (ItemTransform): rotation
// in degrees (applied X, then Y, then Z), translation in sixteenths of a
// block, scale.
type Display struct{ Rot, Trans, Scale mgl32.Vec3 }

// Handheld is item/handheld's thirdperson_righthand display, what a sword
// is held by.
var Handheld = Display{mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 4, 0.5}, mgl32.Vec3{0.85, 0.85, 0.85}}

func (d Display) internal() display { return display{d.Rot, d.Trans, d.Scale} }

// Part is a ModelPart's pose in the model's space (sixteenths of a block,
// y down, facing -Z, the right arm towards -X): x, y, z, then xRot, yRot,
// zRot in radians, applied Z, Y, X.
type Part = part

// Parts are the humanoid's posed parts, in this order.
const (
	Head = iota
	Body
	RightArm
	LeftArm
	RightLeg
	LeftLeg
)

// Humanoid is one living player-shaped body playing attacks: its walk,
// body yaw and attack animation stack, ticked at 20 Hz as LivingEntity
// ticks.
type Humanoid struct {
	bodyYaw, prevBodyYaw float32
	walk                 walkState
	stack                *attackStack
	age                  int
}

// NewHumanoid returns a body facing yRot (Minecraft degrees: 0 south, +Z).
func NewHumanoid(yRot float32) *Humanoid {
	return &Humanoid{bodyYaw: yRot, prevBodyYaw: yRot, stack: newAttackStack()}
}

// Attack plays a over length ticks, its first upswing share (0.5 for most)
// faster and the rest slower, as Better Combat plays an attack whose hit
// lands at the upswing.
func (h *Humanoid) Attack(a *Animation, length, upswing float32) {
	h.stack.play(a, false, length, upswing)
}

// Attacking reports whether an attack animation plays.
func (h *Humanoid) Attacking() bool { return h.stack.c.isActive() }

// Tick is the end of the body's tick: the walk animation from the tick's
// movement dx, dz (blocks), the animation stack, the body turning after
// the head's yaw yRot (Minecraft degrees), and further toward it while
// attacking.
func (h *Humanoid) Tick(yRot float32, dx, dz float64, firstPerson bool) {
	h.walk.update(min(float32(math.Sqrt(float64(dx*dx)+float64(dz*dz)))*4, 1), 0.4, 1)
	h.stack.handle(0, true, firstPerson)
	h.tickBodyYaw(yRot, dx, dz, h.Attacking())
	h.age++
}

// Pose is the body posed at partial tick pt.
type Pose struct {
	Parts           [6]Part
	body, arm, item [axes]float32
	BodyRot         float32 // the body's yaw, Minecraft degrees
}

// Pose poses the body at partial tick pt with its head at xRot (pitch,
// down positive) and yRot (yaw), Minecraft degrees: the vanilla pose of a
// player holding an item in the right hand, the attack over it. In first
// person the attack doesn't tilt the body with the pitch (the camera
// does: FirstPerson).
func (h *Humanoid) Pose(pt, xRot, yRot float32, firstPerson bool) Pose {
	active := h.Attacking()
	h.stack.pitch = xRot
	if firstPerson {
		h.stack.pitch = 0
	}
	if active {
		h.stack.handle(pt, false, firstPerson)
	}
	bodyRot := h.prevBodyYaw + pt*wrapDegrees(h.bodyYaw-h.prevBodyYaw)
	head, torso, rArm, lArm, rLeg, lLeg := vanillaPose(h.walk.positionAt(pt), h.walk.speedAt(pt), float32(h.age)+pt, h.stack.pitch, wrapDegrees(yRot-bodyRot))
	posed := func(name string, p, init part) *animBone {
		b := vanillaBone(name, p, init)
		if active {
			h.stack.transform(b)
		}
		return b
	}
	body, item := newAnimBone("body"), newAnimBone("right_item")
	if active {
		h.stack.transform(body)
		h.stack.transform(item)
	}
	arm := posed("right_arm", rArm, initRightArm)
	return Pose{BodyRot: bodyRot, body: body.v, arm: arm.v, item: item.v, Parts: [6]Part{
		bonePart(posed("head", head, initHead), initHead),
		bonePart(posed("torso", torso, initBody), initBody),
		bonePart(arm, initRightArm),
		bonePart(posed("left_arm", lArm, initLeftArm), initLeftArm),
		bonePart(posed("right_leg", rLeg, initRightLeg), initRightLeg),
		bonePart(posed("left_leg", lLeg, initLeftLeg), initLeftLeg),
	}}
}

// FirstPerson is the held item's transform from the eye (world
// orientation) in first person, as Player Animation Lib's first-person
// pass draws it with the body following the camera's pitch xRot and the
// head's yaw headYaw from the body (Minecraft degrees): to the item
// model's unit slab before its own -0.5 centring.
func FirstPerson(p Pose, headYaw, xRot float32, d Display) mgl32.Mat4 {
	_, _, item := bcChain(p.body, p.arm, p.item, p.BodyRot, headYaw, xRot, d.internal())
	return item
}

// BodyBone is the attack's move of the whole model, in the entity's frame:
// y up, turned by the body yaw (Rotate Y by 180° - BodyRot), at the feet.
func (p Pose) BodyBone() mgl32.Mat4 { return bodyBone(p.body) }

// HandItem is the item in the right hand from the model's root (ModelPart
// space in blocks: y down, at the neck), to its unit slab before the item
// model's -0.5 centring.
func (p Pose) HandItem(d Display) mgl32.Mat4 {
	_, item := armChain(p.arm, p.item, d.internal())
	return item
}
