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

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/mcmodel"
	"github.com/memmaker/voxellib/mobs"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/render"
)

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
	g.equipped = true
	g.arrows.Hit = func(a *projectile.Arrow, t projectile.Target) {
		if m, ok := t.(*mobs.Mob); ok && !m.Static {
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
	ex, ey, ez := g.player.Eye()
	fx, fy, fz := math.Sin(yaw)*math.Cos(pitch), math.Sin(pitch), -math.Cos(yaw)*math.Cos(pitch)
	// Start just in front of the eye, slightly below, so it leaves the bow.
	g.arrows.Shoot(ex+fx*0.4, ey+fy*0.4-0.1, ez+fz*0.4, yaw, pitch, projectile.Speed)
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
		targets = append(targets, c.mob)
	}
	g.arrows.Tick(g.terrain, targets)
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
		sky, blk := g.light(x, y, z)
		g.modelInsts = append(g.modelInsts, render.ModelInstance{Model: g.arrowModel, X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
	}
	g.modelRend.Draw(&g.cam, g.modelInsts)
}

// drawCrossbowHand draws the crossbow in the lower right of the view, with
// walking bob and a recoil kick after each shot.
func (g *game) drawCrossbowHand(alpha float64) {
	t := g.walkDist * 1.6
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
	const s = 1.0 / 32                                                     // half the shaft's thickness
	box(mgl32.Vec3{-s, -s, -0.75}, mgl32.Vec3{s, s, -0.1}, 0)              // shaft
	box(mgl32.Vec3{-2 * s, -2 * s, -0.12}, mgl32.Vec3{2 * s, 2 * s, 0}, 1) // head
	box(mgl32.Vec3{-0.002, -4 * s, -0.72}, mgl32.Vec3{0.002, 4 * s, -0.55}, 2)
	box(mgl32.Vec3{-4 * s, -0.002, -0.72}, mgl32.Vec3{4 * s, 0.002, -0.55}, 2)
	box(mgl32.Vec3{-1.2 * s, -1.2 * s, -0.78}, mgl32.Vec3{1.2 * s, 1.2 * s, -0.74}, 3) // nock
	m.Meshes = []model.Mesh{me}
	return m
}
