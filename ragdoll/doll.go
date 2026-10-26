package ragdoll

import (
	"math"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel/collide"
)

// Joints of a Doll.
const (
	Pelvis = iota
	Neck
	Crown
	ShoulderR
	ShoulderL
	HandR
	HandL
	HipR
	HipL
	FootR
	FootL
	numJoints
)

// Bind is where the joints sit on a Minecraft player model (geometry
// humanoid, in blocks, feet at the origin; on the centre lines, the
// hands and feet a half-thickness in from their ends; its right arm towards -X; New
// mirrors it for a model whose right arm is towards +X).
var Bind = [numJoints]mgl32.Vec3{
	Pelvis: {0, 12. / 16, 0}, Neck: {0, 24. / 16, 0}, Crown: {0, 30. / 16, 0},
	ShoulderR: {-5. / 16, 22. / 16, 0}, ShoulderL: {5. / 16, 22. / 16, 0},
	HandR: {-6. / 16, 14. / 16, 0}, HandL: {6. / 16, 14. / 16, 0},
	HipR: {-1.9 / 16, 12. / 16, 0}, HipL: {1.9 / 16, 12. / 16, 0},
	FootR: {-2. / 16, 2. / 16, 0}, FootL: {2. / 16, 2. / 16, 0},
}

// A part is a rigid piece of the doll: the model bones named after it
// (and their children) move with it, about its pivot towards its tip.
type part struct {
	names      []string
	pivot, tip int
}

var parts = []part{
	{[]string{"body", "torso"}, Neck, Pelvis}, // first: the default
	{[]string{"head"}, Neck, Crown},
	{[]string{"rightarm"}, ShoulderR, HandR},
	{[]string{"leftarm"}, ShoulderL, HandL},
	{[]string{"rightleg"}, HipR, FootR},
	{[]string{"leftleg"}, HipL, FootL},
}

type link struct {
	a, b    int
	rest    float64
	minOnly bool // only keeps them apart
}

// Doll is a verlet ragdoll of a humanoid model. Its joints live in world
// space and collide with the blocks.
type Doll struct {
	Gravity, Drag float64 // per tick; Minecraft's 0.08 and 0.98 by default

	pos, prev [numJoints]mgl32.Vec3
	bind      [numJoints]mgl32.Vec3 // the joints at rest, in the world
	toWorld   mgl32.Mat4            // the model's placement at rest
	links     []link
	bones     []int // model bone -> part
	rise      *rise // getting up (Rise)
	held      [numJoints]bool
	hold      [numJoints]mgl32.Vec3 // where held joints are pinned (Hold), for the next Step
}

// New stands a doll in m's bind pose where toWorld places the model.
func New(m *model.Model, toWorld mgl32.Mat4) *Doll {
	d := &Doll{Gravity: 0.08, Drag: 0.98, toWorld: toWorld}
	mirror := float32(1)
	if r := m.Bone("rightArm"); r >= 0 && m.BindWorld()[r].Col(3).X() > 0 {
		mirror = -1
	}
	for i, p := range Bind {
		p[0] *= mirror
		d.bind[i] = mgl32.TransformCoordinate(p, toWorld)
	}
	d.pos, d.prev = d.bind, d.bind
	torso := []int{Pelvis, Neck, ShoulderR, ShoulderL, HipR, HipL}
	for i, a := range torso {
		for _, b := range torso[i+1:] {
			d.tie(a, b, false)
		}
	}
	for _, l := range [][2]int{{Neck, Crown}, {Crown, ShoulderR}, {Crown, ShoulderL},
		{ShoulderR, HandR}, {ShoulderL, HandL}, {HipR, FootR}, {HipL, FootL}} {
		d.tie(l[0], l[1], false)
	}
	for _, l := range [][2]int{{FootR, Neck}, {FootL, Neck}, {HandR, HipL}, {HandL, HipR}} { // limbs don't fold through the body
		d.tie(l[0], l[1], true)
	}
	d.bones = make([]int, len(m.Bones))
	for i, b := range m.Bones {
		d.bones[i] = -1
		name := strings.ToLower(strings.ReplaceAll(b.Name, "_", ""))
		for k, p := range parts {
			for _, n := range p.names {
				if name == n {
					d.bones[i] = k
				}
			}
		}
		if d.bones[i] < 0 {
			d.bones[i] = 0
			if b.Parent >= 0 {
				d.bones[i] = d.bones[b.Parent]
			}
		}
	}
	return d
}

func (d *Doll) tie(a, b int, minOnly bool) {
	r := float64(d.bind[a].Sub(d.bind[b]).Len())
	if minOnly {
		r *= 0.8
	}
	d.links = append(d.links, link{a, b, r, minOnly})
}

// Joint is where joint j is now.
func (d *Doll) Joint(j int) mgl32.Vec3 { return d.pos[j] }

// Push adds velocity v (blocks per tick) to every joint; spin adds more
// to the joints above the pelvis, so the body tumbles over.
func (d *Doll) Push(v mgl32.Vec3, spin float32) {
	for i := range d.pos {
		k := 1 + spin*(d.pos[i].Y()-d.pos[Pelvis].Y())
		d.prev[i] = d.prev[i].Sub(v.Mul(k))
	}
}

// Follow drags the whole doll towards the body it stands for: its pelvis
// towards (x, y+0.6, z), by k of the way across, and all the way up or
// down once it is more than a block off (picked up, carried).
func (d *Doll) Follow(x, y, z float64, k float32) {
	off := mgl32.Vec3{float32(x), float32(y) + 0.6, float32(z)}.Sub(d.pos[Pelvis])
	shift := mgl32.Vec3{off.X() * k, 0, off.Z() * k}
	if off.Len() > 1 {
		shift = off.Mul(0.5)
	}
	for i := range d.pos {
		d.pos[i], d.prev[i] = d.pos[i].Add(shift), d.prev[i].Add(shift)
	}
}

// Hold pins joint j at p for the next Step: the rest of the doll hangs
// from it (a body over a shoulder).
func (d *Doll) Hold(j int, p mgl32.Vec3) { d.held[j], d.hold[j] = true, p }

// Pull draws joint j towards p by k of the way, as velocity.
func (d *Doll) Pull(j int, p mgl32.Vec3, k float32) {
	d.prev[j] = d.prev[j].Sub(p.Sub(d.pos[j]).Mul(k))
}

const jointRadius = 2. / 16 // half a limb's thickness: joints are on the bones' centre lines

func (d *Doll) fits(w collide.Solids, j int, p mgl32.Vec3) bool {
	r := jointRadius
	if j == Crown {
		r = 2 * jointRadius // the head is twice as thick
	}
	return collide.Fits(w, collide.Box(float64(p.X()), float64(p.Y())-r, float64(p.Z()), 2*r, 2*r))
}

// Step moves the doll one tick through w.
func (d *Doll) Step(w collide.Solids) {
	for i := range d.pos {
		p, q := d.pos[i], d.prev[i]
		v := p.Sub(q).Mul(float32(d.Drag))
		v[1] -= float32(d.Gravity)
		d.prev[i] = p
		d.pos[i] = d.move(w, i, p, p.Add(v))
		if d.held[i] {
			d.pos[i] = d.hold[i]
		}
	}
	for range 8 {
		for _, l := range d.links {
			a, b := d.pos[l.a], d.pos[l.b]
			delta := b.Sub(a)
			n := delta.Len()
			if n < 1e-6 || l.minOnly && float64(n) >= l.rest || d.held[l.a] && d.held[l.b] {
				continue
			}
			fix := delta.Mul((n - float32(l.rest)) / n) // all of it to a held joint's partner
			if !d.held[l.a] && !d.held[l.b] {
				fix = fix.Mul(0.5)
			}
			if !d.held[l.a] {
				d.pos[l.a] = d.move(w, l.a, a, a.Add(fix))
			}
			if !d.held[l.b] {
				d.pos[l.b] = d.move(w, l.b, b, b.Sub(fix))
			}
		}
	}
	d.held = [numJoints]bool{}
}

// move takes a joint from p towards to, one axis at a time, stopping at
// blocks; a joint that touches one drags (friction).
func (d *Doll) move(w collide.Solids, j int, p, to mgl32.Vec3) mgl32.Vec3 {
	if d.fits(w, j, to) {
		return to
	}
	if !d.fits(w, j, p) { // pushed inside: up and out
		for up := float32(0.05); up <= 1; up += 0.05 {
			if q := p.Add(mgl32.Vec3{0, up, 0}); d.fits(w, j, q) {
				return q
			}
		}
		return to
	}
	q := p
	for _, a := range [3]int{1, 0, 2} {
		c := q
		c[a] = to[a]
		if d.fits(w, j, c) {
			q = c
		}
	}
	return p.Add(q.Sub(p).Mul(0.6))
}

// Skin is the doll's pose alpha of the way from the last tick to this
// one, as skin matrices for its model drawn with instance matrix inst
// (model to world).
func (d *Doll) Skin(inst mgl32.Mat4, alpha float32, dst []mgl32.Mat4) []mgl32.Mat4 {
	var pos [numJoints]mgl32.Vec3
	for i := range pos {
		pos[i] = d.prev[i].Add(d.pos[i].Sub(d.prev[i]).Mul(alpha))
	}
	torso := basis(pos).Mul3(basis(d.bind).Transpose())
	mats := make([]mgl32.Mat4, len(parts))
	inv := inst.Inv()
	for k, p := range parts {
		r := torso
		if k > 0 {
			from := torso.Mul3x1(d.bind[p.tip].Sub(d.bind[p.pivot]))
			to := pos[p.tip].Sub(pos[p.pivot])
			r = mgl32.QuatBetweenVectors(from.Normalize(), to.Normalize()).Mat4().Mat3().Mul3(torso)
		}
		cp, bp := pos[p.pivot], d.bind[p.pivot]
		mats[k] = inv.Mul4(mgl32.Translate3D(cp.X(), cp.Y(), cp.Z())).Mul4(r.Mat4()).
			Mul4(mgl32.Translate3D(-bp.X(), -bp.Y(), -bp.Z())).Mul4(d.toWorld)
	}
	dst = dst[:0]
	for _, k := range d.bones {
		dst = append(dst, mats[k])
	}
	return dst
}

// basis is the torso's orientation: side (right to left shoulder), up
// (pelvis to neck) and their cross.
func basis(j [numJoints]mgl32.Vec3) mgl32.Mat3 {
	up := j[Neck].Sub(j[Pelvis]).Normalize()
	side := j[ShoulderL].Sub(j[ShoulderR])
	side = side.Sub(up.Mul(side.Dot(up))).Normalize()
	return mgl32.Mat3FromCols(side, up, side.Cross(up))
}

// rise is a doll getting up: from where it lay to its standing pose.
type rise struct {
	from   [numJoints]mgl32.Vec3
	to     [numJoints]mgl32.Vec3
	turn   mgl32.Quat // the torso lying, relative to standing
	ground float32
	tick   int
	ticks  int
}

// Turn is how far (radians about +Y, anticlockwise seen from above) the
// doll would face from where New placed it once it swung its torso
// upright the shortest way from how it lies now. Turning Rise's toWorld
// by it gets the doll up without spinning round: off its back it faces
// its feet, off its front its head.
func (d *Doll) Turn() float64 {
	b := basis(d.pos)
	q := mgl32.QuatBetweenVectors(b.Col(1), mgl32.Vec3{0, 1, 0})
	if b.Col(1).Y() < -0.999 { // on its head: over forwards
		q = mgl32.QuatRotate(math.Pi, b.Col(0))
	}
	f, r := q.Rotate(b.Col(2)), basis(d.bind).Col(2)
	return math.Atan2(float64(r.Z()*f.X()-r.X()*f.Z()), float64(r.X()*f.X()+r.Z()*f.Z()))
}

// Rise starts the doll getting up over ticks ticks, into its model's
// bind pose where toWorld places it standing: the torso swings up about
// the pelvis while the pelvis lifts off the floor, and the limbs follow
// by one-bone IK (each aims at its target and keeps its length): the
// feet plant under the hips and the hands push off the floor, then hang.
func (d *Doll) Rise(toWorld mgl32.Mat4, ticks int) {
	r := &rise{from: d.pos, ticks: max(ticks, 1)}
	for i, p := range d.bind {
		r.to[i] = mgl32.TransformCoordinate(mgl32.TransformCoordinate(p, d.toWorld.Inv()), toWorld)
	}
	lying, standing := mgl32.Mat4ToQuat(basis(d.pos).Mat4()), mgl32.Mat4ToQuat(basis(r.to).Mat4())
	r.turn = lying.Mul(standing.Inverse())
	r.ground = min(r.to[FootR].Y(), r.to[FootL].Y()) - jointRadius
	d.rise = r
}

// Rising reports whether the doll is getting up (Rise, until RiseStep has
// run its ticks).
func (d *Doll) Rising() bool { return d.rise != nil && d.rise.tick < d.rise.ticks }

// RiseStep moves a rising doll one tick on its way up.
func (d *Doll) RiseStep() {
	r := d.rise
	if r == nil || r.tick >= r.ticks {
		return
	}
	r.tick++
	t := float32(r.tick) / float32(r.ticks)
	ease := func(a, b float32) float32 { // smoothstep of t over [a, b]
		x := min(1, max(0, (t-a)/(b-a)))
		return x * x * (3 - 2*x)
	}
	d.prev = d.pos
	pelvis := lerp(r.from[Pelvis], r.to[Pelvis], ease(0.2, 1))
	turn := mgl32.QuatSlerp(r.turn, mgl32.QuatIdent(), ease(0, 0.75))
	for _, j := range []int{Pelvis, Neck, Crown, ShoulderR, ShoulderL, HipR, HipL} { // the torso and head, rigid
		d.pos[j] = pelvis.Add(turn.Rotate(r.to[j].Sub(r.to[Pelvis])))
	}
	for _, l := range [][2]int{{HipR, FootR}, {HipL, FootL}} { // plant the feet under the hips
		d.aim(l[0], l[1], lerp(r.from[l[1]], r.to[l[1]], ease(0, 0.5)))
	}
	for _, l := range [][2]int{{ShoulderR, HandR}, {ShoulderL, HandL}} { // push off the floor, then hang
		s := d.pos[l[0]]
		push := mgl32.Vec3{s.X(), r.ground + jointRadius, s.Z()}
		d.aim(l[0], l[1], lerp(lerp(r.from[l[1]], push, ease(0, 0.25)), r.to[l[1]], ease(0.5, 1)))
	}
	if r.tick == r.ticks {
		d.pos, d.prev = r.to, r.to
	}
}

// aim points the limb from joint a at target, keeping its length (one-bone
// IK), and keeps its end out of the floor.
func (d *Doll) aim(a, b int, target mgl32.Vec3) {
	n := float32(d.bind[b].Sub(d.bind[a]).Len())
	dir := target.Sub(d.pos[a])
	if dir.Len() < 1e-5 {
		return
	}
	p := d.pos[a].Add(dir.Normalize().Mul(n))
	if floor := d.rise.ground + jointRadius; p.Y() < floor { // along the floor, still n long
		o := d.pos[a]
		h := mgl32.Vec2{p.X() - o.X(), p.Z() - o.Z()}
		dy := floor - o.Y()
		if h.Len() < 1e-3 { // straight down: out from the body
			h = mgl32.Vec2{o.X() - d.pos[Pelvis].X(), o.Z() - d.pos[Pelvis].Z()}
		}
		if r := n*n - dy*dy; r > 0 && h.Len() > 1e-5 {
			h = h.Normalize().Mul(float32(math.Sqrt(float64(r))))
		}
		p = mgl32.Vec3{o.X() + h.X(), floor, o.Z() + h.Y()}
	}
	d.pos[b] = p
}

func lerp(a, b mgl32.Vec3, t float32) mgl32.Vec3 { return a.Add(b.Sub(a).Mul(t)) }
