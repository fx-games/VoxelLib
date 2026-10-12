package main

// The crossbow: a weapon cut out of an imported model (the repeater
// crossbow the artmob carries), held in first person, firing arrows from
// package projectile that fly in an arc and stick in blocks and creatures.
// Hooks elsewhere: setupCrossbow after the asset import, C to equip,
// left click fires while equipped, tickArrows/drawArrows in the loop.

import (
	"image"
	"image/color"
	"log"
	"math"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/memmaker/voxellib/importer/mcmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
)

// arrowItem is the pickup an arrow becomes when a corpse disappears.
const arrowItem = "arrow"

// crossbowGroup is the bone holding the crossbow in the pack's models.
const crossbowGroup = "repeater_crossbow_arrow"

const (
	reloadTicks = 8 // a repeater: about 2.5 shots a second
	recoilTicks = 4
	// knockback is the push (blocks/tick) a creature gets from a hit.
	knockback = 0.4
)

type crossbowState struct {
	bowModel   int // ModelRenderer handles; -1 = none
	arrowModel int
	equipped   bool
	reload     int // ticks until the next shot
	recoil     int // ticks left of the recoil kick
	arrows     projectile.World
}

// setupCrossbow finds the crossbow in an imported pack and equips it.
func (g *game) setupCrossbow(p *mcmodel.Pack) {
	var bow *model.Model
	for _, m := range p.Models {
		if bow = m.Subtree(crossbowGroup); bow != nil {
			break
		}
	}
	if bow == nil {
		return
	}
	if err := g.ensureModelRenderer(); err != nil {
		log.Printf("crossbow: %v", err)
		return
	}
	var err error
	if g.bowModel, err = g.modelRend.Add(bow); err != nil {
		log.Printf("crossbow: %v", err)
		return
	}
	if g.arrowModel, err = g.modelRend.Add(arrowModel()); err != nil {
		log.Printf("crossbow: %v", err)
		g.bowModel = -1
		return
	}
	g.addItem(arrowItem, arrowSprite())
	g.unscattered = append(g.unscattered, arrowItem)
	g.equipped = true
	g.arrows.Hit = func(a *projectile.Arrow, t projectile.Target) {
		c, ok := t.(*creature)
		if !ok {
			return
		}
		if head := c.sp.shape.head; a.Part >= 0 && a.Part < len(head) && head[a.Part] {
			g.kill(c, a.Yaw)
			return
		}
		if m := c.mob; !m.Static {
			l := math.Hypot(a.PrevX-a.X, a.PrevZ-a.Z) + 1e-9
			m.VX += (a.X - a.PrevX) / l * knockback
			m.VZ += (a.Z - a.PrevZ) / l * knockback
			m.VY = 0.3
		}
	}
}

// crossbowOn reports whether the crossbow is in hand.
func (g *game) crossbowOn() bool { return g.equipped && g.bowModel >= 0 }

// toggleCrossbow (C) puts the crossbow away or takes it out.
func (g *game) toggleCrossbow() {
	if g.bowModel >= 0 {
		g.equipped = !g.equipped
	}
}

// fire shoots an arrow along the view if the crossbow is loaded.
func (g *game) fire() {
	if !g.crossbowOn() || g.reload > 0 {
		return
	}
	yaw, pitch := g.cam.Yaw, g.cam.Pitch
	// The arrow starts like vanilla's (AbstractArrow): at the shooter, 0.1
	// below the eyes. Unlike vanilla it doesn't inherit the shooter's
	// movement, so it flies exactly where the crosshair pointed and doesn't
	// drift along with a strafing player.
	ex, ey, ez := g.player.Eye()
	g.arrows.Shoot(ex, ey-0.1, ez, yaw, pitch, projectile.Speed)
	g.reload, g.recoil = reloadTicks, recoilTicks
}

func (g *game) tickArrows() {
	if g.reload > 0 {
		g.reload--
	}
	if g.recoil > 0 {
		g.recoil--
	}
	if len(g.arrows.Arrows) == 0 {
		return
	}
	targets := make([]projectile.Target, 0, len(g.creatures))
	for _, c := range g.creatures {
		if c.sp.shape == nil {
			c.sp.shape = newShape(c.sp.model)
		}
		if c.dying == 0 {
			targets = append(targets, c)
		}
	}
	g.arrows.Tick(g.terrain, targets)
}

// Head shots: an arrow that strikes a creature's head kills it. Hits are
// exact: arrows test the creature's animated triangles in the pose last
// drawn, and the bone of the triangle struck says whether it was the head.
// A killed creature freezes in its pose, topples over the way the arrow
// was flying (90° over a second), lies still, fades out and is removed;
// the arrows in it then drop as pickups.

const (
	deathTicks = 20 // falling over
	lieTicks   = 10 // lying still
	fadeTicks  = 20 // dissolving
)

// shapeInfo is per-model data for exact hits.
type shapeInfo struct {
	head   []bool  // per bone: the "head" bone or one of its children
	radius float64 // bounding sphere around the feet origin
}

func newShape(m *model.Model) *shapeInfo {
	si := &shapeInfo{head: make([]bool, len(m.Bones))}
	for i, b := range m.Bones {
		si.head[i] = strings.EqualFold(b.Name, "head") || (b.Parent >= 0 && si.head[b.Parent])
	}
	lo, hi := m.Bounds()
	for _, v := range []mgl32.Vec3{lo, hi} {
		si.radius = math.Max(si.radius, float64(v.Len()))
	}
	si.radius *= 1.5 // room for animation to swing limbs out
	return si
}

// Box, Frame, Reach, Intersect and Joint make a creature a
// projectile.Jointed: exact hits, and arrows that ride the bone they hit.
func (c *creature) Box() collide.AABB             { return c.mob.Box() }
func (c *creature) Frame() (x, y, z, yaw float64) { return c.mob.Frame() }

// Reach is the box the creature's animated mesh can occupy, for the
// arrows' broad-phase grid: its bounding sphere around both the drawn and
// the current position.
func (c *creature) Reach() collide.AABB {
	r := c.sp.shape.radius
	x0, y0, z0 := c.mob.X, c.mob.Y, c.mob.Z
	x1, y1, z1 := x0, y0, z0
	if c.drawn {
		x1, y1, z1 = c.drawX, c.drawY, c.drawZ
	}
	return collide.AABB{MinX: min(x0, x1) - r, MinY: min(y0, y1) - r, MinZ: min(z0, z1) - r,
		MaxX: max(x0, x1) + r, MaxY: max(y0, y1) + r, MaxZ: max(z0, z1) + r}
}

// creatureMatrix places a creature's model space in the world, as the
// model renderer does, tipped over by tilt toward tiltYaw.
func creatureMatrix(x, y, z, yaw, tilt, tiltYaw float64) mgl32.Mat4 {
	return mgl32.Translate3D(float32(x), float32(y), float32(z)).Mul4(render.TiltMatrix(tilt, tiltYaw)).
		Mul4(mgl32.HomogRotate3DY(float32(math.Pi - yaw)))
}

// boneMatrix is bone b's skin matrix in the pose last drawn (identity
// before the first draw).
func (c *creature) boneMatrix(b int) mgl32.Mat4 {
	if b >= 0 && b < len(c.skin) {
		return c.skin[b]
	}
	return mgl32.Ident4()
}

// Joint maps bone b's bind-pose model space to the world: the drawn pose
// of the bone, placed at the creature's current position.
func (c *creature) Joint(b int) mgl64.Mat4 {
	return mat64(creatureMatrix(c.mob.X, c.mob.Y, c.mob.Z, c.mob.Yaw, c.deathTilt(1), c.fallYaw).Mul4(c.boneMatrix(b)))
}

func mat64(m mgl32.Mat4) (o mgl64.Mat4) {
	for i := range m {
		o[i] = float64(m[i])
	}
	return o
}

// Intersect tests the segment against the creature's mesh as last drawn.
// The hit point is then carried into the creature's current frame, so the
// arrow sticks where it was seen to hit.
func (c *creature) Intersect(x0, y0, z0, x1, y1, z1 float64) (x, y, z float64, part int, ok bool) {
	dx, dy, dz, dyaw := c.drawX, c.drawY, c.drawZ, c.drawYaw
	skin := c.skin
	if !c.drawn {
		dx, dy, dz, dyaw = c.mob.X, c.mob.Y, c.mob.Z, c.mob.Yaw
		skin = nil
	}
	if !segNearSphere(x0-dx, y0-dy, z0-dz, x1-dx, y1-dy, z1-dz, c.sp.shape.radius) {
		return 0, 0, 0, -1, false
	}
	inv := creatureMatrix(dx, dy, dz, dyaw, 0, 0).Inv()
	o := mgl32.TransformCoordinate(mgl32.Vec3{float32(x0), float32(y0), float32(z0)}, inv)
	e := mgl32.TransformCoordinate(mgl32.Vec3{float32(x1), float32(y1), float32(z1)}, inv)
	if c.pose == nil || c.poseFrame != c.drawFrame || !c.drawn {
		c.pose = c.sp.model.Pose(skin, c.pose)
		c.poseFrame = c.drawFrame
	}
	h, ok := c.sp.model.RaycastPose(c.pose, o, e.Sub(o), 1)
	if !ok {
		return 0, 0, 0, -1, false
	}
	w := mgl32.TransformCoordinate(h.Pos, creatureMatrix(c.mob.X, c.mob.Y, c.mob.Z, c.mob.Yaw, c.deathTilt(1), c.fallYaw))
	return float64(w[0]), float64(w[1]), float64(w[2]), h.Bone, true
}

// segNearSphere reports whether the segment a→b passes within r of the
// origin.
func segNearSphere(ax, ay, az, bx, by, bz, r float64) bool {
	dx, dy, dz := bx-ax, by-ay, bz-az
	t := 0.0
	if l := dx*dx + dy*dy + dz*dz; l > 0 {
		t = math.Max(0, math.Min(1, -(ax*dx+ay*dy+az*dz)/l))
	}
	px, py, pz := ax+dx*t, ay+dy*t, az+dz*t
	return px*px+py*py+pz*pz <= r*r
}

// kill starts a creature's death: it stops, and its arrows fall out.
func (g *game) kill(c *creature, arrowYaw float64) {
	if c.dying > 0 {
		return
	}
	// Freeze the body where it is: it is no longer ticked, so its previous
	// pose must equal the current one or drawing keeps interpolating
	// between them every tick (the corpse jittered), and Static keeps other
	// mobs from shoving it. Its arrows stay in it until it disappears.
	m := c.mob
	c.dying = 1
	c.fallYaw = arrowYaw // pushed over along the arrow's flight
	m.VX, m.VY, m.VZ = 0, 0, 0
	m.PrevX, m.PrevY, m.PrevZ, m.PrevYaw = m.X, m.Y, m.Z, m.Yaw
	m.Static = true
	g.debugMsg = c.sp.model.Name + ": head shot"
}

// tickDying advances deaths and removes creatures that have fallen over.
func (g *game) tickDying() {
	live := g.creatures[:0]
	for _, c := range g.creatures {
		if c.dying > 0 {
			c.dying++
			if c.dying > deathTicks+lieTicks+fadeTicks {
				g.dropArrows(c)
				continue
			}
		}
		live = append(live, c)
	}
	clear(g.creatures[len(live):])
	g.creatures = live
}

// dropArrows turns the arrows stuck in a disappearing creature into arrow
// pickups where they were; they fall and hover like any dropped item.
func (g *game) dropArrows(c *creature) {
	for _, a := range g.arrows.Detach(c) {
		if g.itemRend != nil && g.itemRend.Has(arrowItem) {
			g.drops.Spawn(arrowItem, a.X, a.Y, a.Z)
		}
	}
}

// deathTilt is how far a dying creature has fallen over (radians).
func (c *creature) deathTilt(alpha float64) float64 {
	if c.dying == 0 {
		return 0
	}
	t := math.Min(1, (float64(c.dying-1)+alpha)/deathTicks)
	return math.Sqrt(t) * math.Pi / 2
}

// deathFade is how far a dead creature has dissolved (0 solid, 1 gone).
func (c *creature) deathFade(alpha float64) float32 {
	if c.dying == 0 {
		return 0
	}
	t := (float64(c.dying-1) + alpha - deathTicks - lieTicks) / fadeTicks
	return float32(math.Max(0, math.Min(1, t)))
}

// drawArrows draws flying and stuck arrows, and the crossbow in first
// person. Call after the world, before the held item.
func (g *game) drawArrows(alpha float64) {
	if g.arrowModel < 0 {
		return
	}
	g.modelInsts = g.modelInsts[:0]
	for _, a := range g.arrows.Arrows {
		x, y, z, yaw, pitch := a.Lerp(alpha)
		var fade float32
		if c, ok := a.In.(*creature); ok {
			fade = c.drawFade
			if b, pos, d, ok := a.Attached(); ok && c.drawn {
				// Ride the bone with this frame's animation and position.
				m := mat64(creatureMatrix(c.drawX, c.drawY, c.drawZ, c.drawYaw, c.drawTilt, c.fallYaw).Mul4(c.boneMatrix(b)))
				p := mgl64.TransformCoordinate(pos, m)
				x, y, z = p[0], p[1], p[2]
				yaw, pitch = projectile.Orient(mgl64.TransformNormal(d, m))
			}
		}
		// A fresh shot isn't drawn for its first two ticks while within 3.5
		// blocks of the camera, so it doesn't fill the screen as it leaves
		// the eye. Vanilla does this for snowballs, fireballs, wind charges
		// and eyes of ender (tickCount < 2 && distSq < 12.25).
		if !a.Stuck && a.Age < 2 {
			dx, dy, dz := x-g.cam.X, y-g.cam.Y, z-g.cam.Z
			if dx*dx+dy*dy+dz*dz < 3.5*3.5 {
				continue
			}
		}
		// Light the arrow from its middle: a stuck tip sits inside a solid
		// block, whose light is 0, which drew every stuck arrow black.
		back := 0.4 * math.Cos(pitch)
		sky, blk := g.light(x-math.Sin(yaw)*back, y-0.4*math.Sin(pitch), z+math.Cos(yaw)*back)
		g.modelInsts = append(g.modelInsts, render.ModelInstance{Model: g.arrowModel, X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk, Fade: fade})
	}
	g.modelRend.Draw(&g.cam, g.modelInsts)
}

// drawCrossbowHand draws the crossbow in the lower right of the view, with
// walking bob and a recoil kick after each shot.
func (g *game) drawCrossbowHand(alpha float64) {
	t := g.bobPhase(alpha)
	bobX, bobY := float32(math.Sin(t))*0.02, -float32(math.Abs(math.Cos(t)))*0.03
	kick := float32(0)
	if g.recoil > 0 {
		kick = (float32(g.recoil) - float32(alpha)) / recoilTicks
	}
	view := mgl32.Translate3D(0.42+bobX, -0.36+bobY, -0.75+0.12*kick).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(8))). // aim toward the crosshair
		Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(4 + 8*kick))).
		Mul4(bowHold).
		Mul4(mgl32.Scale3D(0.55, 0.55, 0.55))
	sky, blk := g.light(g.cam.X, g.cam.Y, g.cam.Z)
	g.modelRend.DrawHand(g.bowModel, float32(g.cam.Aspect), view, sky, blk)
}

// bowHold turns the cut-out crossbow (flat in its XY plane, stock running
// diagonally) so it points into the screen, bow arms level.
var bowHold = mgl32.HomogRotate3DY(mgl32.DegToRad(-90)).
	Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-90))).
	Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(45)))

// arrowModel builds a small arrow pointing along +Z with its tip at the
// origin: a shaft, a head and fletching, coloured from a 4-pixel texture.
func arrowModel() *model.Model {
	tex := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	tex.Set(0, 0, color.NRGBA{120, 86, 52, 255})   // shaft
	tex.Set(1, 0, color.NRGBA{170, 170, 175, 255}) // head
	tex.Set(2, 0, color.NRGBA{235, 235, 235, 255}) // fletching
	tex.Set(3, 0, color.NRGBA{70, 70, 75, 255})    // nock
	m := &model.Model{Name: "arrow", Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: "arrow", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	var me model.Mesh
	box := func(lo, hi mgl32.Vec3, px int) {
		u := (float32(px) + 0.5) / 4
		for _, f := range [6]struct {
			n mgl32.Vec3
			c [4]mgl32.Vec3
		}{
			{mgl32.Vec3{1, 0, 0}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {hi[0], hi[1], lo[2]}, {hi[0], hi[1], hi[2]}, {hi[0], lo[1], hi[2]}}},
			{mgl32.Vec3{-1, 0, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], hi[1], hi[2]}, {lo[0], hi[1], lo[2]}, {lo[0], lo[1], lo[2]}}},
			{mgl32.Vec3{0, 1, 0}, [4]mgl32.Vec3{{lo[0], hi[1], lo[2]}, {lo[0], hi[1], hi[2]}, {hi[0], hi[1], hi[2]}, {hi[0], hi[1], lo[2]}}},
			{mgl32.Vec3{0, -1, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], lo[1], lo[2]}, {hi[0], lo[1], lo[2]}, {hi[0], lo[1], hi[2]}}},
			{mgl32.Vec3{0, 0, 1}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {hi[0], lo[1], hi[2]}, {hi[0], hi[1], hi[2]}, {lo[0], hi[1], hi[2]}}},
			{mgl32.Vec3{0, 0, -1}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {lo[0], lo[1], lo[2]}, {lo[0], hi[1], lo[2]}, {hi[0], hi[1], lo[2]}}},
		} {
			base := uint32(len(me.Verts))
			for _, c := range f.c {
				me.Verts = append(me.Verts, model.Vertex{Pos: c, Normal: f.n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
			}
			me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
		}
	}
	const s = 1.0 / 32                                         // half the shaft's thickness
	box(mgl32.Vec3{-s, -s, -0.75}, mgl32.Vec3{s, s, -0.28}, 0) // shaft, ends inside the head's base
	// Head: an elongated four-sided pyramid, its point at the origin.
	pyramid := func(base, tip float32, r float32, px int) {
		u := (float32(px) + 0.5) / 4
		apex := mgl32.Vec3{0, 0, tip}
		c := [4]mgl32.Vec3{{-r, -r, base}, {r, -r, base}, {r, r, base}, {-r, r, base}}
		tri := func(a, b, c mgl32.Vec3) {
			n := b.Sub(a).Cross(c.Sub(a)).Normalize()
			i := uint32(len(me.Verts))
			for _, p := range []mgl32.Vec3{a, b, c} {
				me.Verts = append(me.Verts, model.Vertex{Pos: p, Normal: n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
			}
			me.Indices = append(me.Indices, i, i+1, i+2)
		}
		for k := 0; k < 4; k++ {
			tri(c[k], c[(k+1)%4], apex)
		}
		tri(c[0], c[2], c[1]) // base
		tri(c[0], c[3], c[2])
	}
	pyramid(-0.3, 0, 2*s, 1)
	box(mgl32.Vec3{-0.002, -4 * s, -0.72}, mgl32.Vec3{0.002, 4 * s, -0.55}, 2)
	box(mgl32.Vec3{-4 * s, -0.002, -0.72}, mgl32.Vec3{4 * s, 0.002, -0.55}, 2)
	box(mgl32.Vec3{-1.2 * s, -1.2 * s, -0.78}, mgl32.Vec3{1.2 * s, 1.2 * s, -0.74}, 3) // nock
	m.Meshes = []model.Mesh{me}
	return m
}

// arrowSprite draws a 16×16 item sprite of an arrow, tip to the upper
// right like Minecraft's arrow item.
func arrowSprite() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	shaft := color.NRGBA{120, 86, 52, 255}
	head := color.NRGBA{170, 170, 175, 255}
	dark := color.NRGBA{90, 90, 96, 255}
	feather := color.NRGBA{235, 235, 235, 255}
	for i := 3; i <= 11; i++ { // shaft along the diagonal
		img.Set(i, 15-i, shaft)
	}
	for _, p := range [][2]int{{12, 3}, {13, 2}, {14, 1}, {11, 2}, {13, 4}, {12, 1}, {14, 3}} {
		img.Set(p[0], p[1], head)
	}
	img.Set(12, 2, dark)
	img.Set(13, 3, dark)
	for _, p := range [][2]int{{1, 12}, {2, 11}, {3, 14}, {4, 13}, {2, 13}, {1, 14}, {2, 14}, {1, 13}} {
		img.Set(p[0], p[1], feather)
	}
	return img
}
