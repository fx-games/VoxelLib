package main

import (
	"log"
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/render"
)

// TACZ's EntityBulletRenderer.renderTracerAmmo: a tracer bullet is the
// basic_bullet model drawn additive and unlit (energySwirl) in the ammo's
// tracer colour, stretched into a trail behind the bullet.
const (
	tracerWidth       = 0.005 // the trail's width near the shooter
	tracerWidthDist   = 3.5   // the width grows with the distance to the shooter's eye past this
	tracerTrail       = 0.85  // trail length per block of velocity
	tracerEyeTrail    = 0.8   // at most this share of the distance to the shooter's eye
	tracerThirdDrop   = 0.2   // others' tracers are drawn this far lower
	tracerMinTicks    = 5     // a younger tracer shows only past tracerMinDist from the eye
	tracerMinDist     = 2
	tracerOffsetRange = 50 // the first-person muzzle offset fades out over this distance
	// tracerModelLift: TACZ draws basic_bullet unflipped, so its Java
	// origin (pivot 24 px less the 16 px cube) puts it at y 0.5..1.5 where
	// the bedrock geometry we parse sits at 0..1.
	tracerModelLift = 0.5
)

type gunTracers struct {
	model  int  // handle (-1: failed to load)
	loaded bool // model tried
	// offsets are the own first-person tracers' muzzle offsets, taken at
	// their first frame (bullet.setFirstPersonRenderOffset).
	offsets map[*projectile.Bullet][3]float64
	insts   []render.ModelInstance
	mats    []mgl32.Mat4
}

// tracer is a bullet to draw as a tracer.
type tracer struct {
	b     *projectile.Bullet
	eye   [3]float64 // the shooter's eye between ticks (getEyePosition(partialTicks))
	eyeAt [3]float64 // the shooter's eye this tick (getEyePosition())
	own   bool       // the local player's, seen in first person
	color [3]float32
}

// drawTracers draws the tracer bullets in flight: the single player's, or
// every player's the server sent.
func (g *game) drawTracers(cam *render.Camera, alpha float64) {
	t := &g.tracerFX
	if !t.loaded {
		t.loaded, t.model = true, -1
		if g.gunPack != nil {
			m, err := g.gunPack.TracerModel()
			if err == nil {
				t.model, err = g.Models.Add(m)
			}
			if err != nil {
				log.Printf("guns: tracer: %v", err)
				t.model = -1
			}
		}
		t.offsets = map[*projectile.Bullet][3]float64{}
	}
	if t.model < 0 {
		return
	}
	var list []tracer
	p := g.Player
	me := [3]float64{p.PrevX + (p.X-p.PrevX)*alpha, p.PrevY + (p.Y-p.PrevY)*alpha + g.Player.EyeHeight, p.PrevZ + (p.Z-p.PrevZ)*alpha}
	meAt := [3]float64{p.X, p.Y + g.Player.EyeHeight, p.Z}
	if g.net == nil {
		for _, b := range g.bullets {
			if b.Tracer {
				list = append(list, tracer{b, me, meAt, !g.watching, b.Gun.Bullet.TracerColor})
			}
		}
	} else {
		views := g.net.cl.Players()
		for _, b := range g.net.cl.Bullets {
			spec := g.gunSpec(b.Gun)
			if !b.Tracer || spec == nil {
				continue
			}
			tr := tracer{b: b.Bullet, color: spec.Bullet.TracerColor}
			if b.Shooter == g.net.cl.Slot() {
				tr.eye, tr.eyeAt, tr.own = me, meAt, !g.watching
			} else {
				i := slices.IndexFunc(views, func(v mp.PlayerView) bool { return v.Slot == b.Shooter })
				if i < 0 {
					continue // the shooter left: TACZ draws no tracer without its owner
				}
				v := views[i]
				tr.eye = [3]float64{v.X, v.Y + character.PlayerEye(v.Sneak), v.Z}
				tr.eyeAt = tr.eye // ponytail: other players' views are not kept per tick
			}
			list = append(list, tr)
		}
	}
	live := map[*projectile.Bullet][3]float64{}
	t.insts, t.mats = t.insts[:0], t.mats[:0]
	for _, tr := range list {
		pos := tr.b.Lerp(alpha)
		d := math.Sqrt(sq(pos[0]-tr.eye[0]) + sq(pos[1]-tr.eye[1]) + sq(pos[2]-tr.eye[2]))
		if tr.b.Age < tracerMinTicks && math.Sqrt(sq(pos[0]-tr.eyeAt[0])+sq(pos[1]-tr.eyeAt[1])+sq(pos[2]-tr.eyeAt[2])) <= tracerMinDist {
			continue
		}
		v := tr.b.Vel
		speed := math.Sqrt(sq(v[0]) + sq(v[1]) + sq(v[2]))
		trail := math.Min(tracerTrail*speed, d*tracerEyeTrail)
		drop := -tracerThirdDrop
		if tr.own {
			drop = 0
			off, ok := t.offsets[tr.b]
			if !ok {
				off = g.muzzleOffset()
			}
			live[tr.b] = off
			k := math.Max(0, tracerOffsetRange-d) / tracerOffsetRange
			for i := range 3 {
				pos[i] += off[i] * k
			}
		}
		w := float32(tracerWidth * math.Max(1, d/tracerWidthDist))
		yaw, pitch := tr.b.LerpRotation(alpha)
		// Ry(yRot-180)·Rx(xRot) turns +Z backwards along the flight.
		m := mgl32.HomogRotate3DY(float32(yaw - math.Pi)).Mul4(mgl32.HomogRotate3DX(float32(pitch))).
			Mul4(mgl32.Translate3D(0, float32(drop), float32(trail/2))).
			Mul4(mgl32.Scale3D(w, w, float32(trail))).
			Mul4(mgl32.Translate3D(0, tracerModelLift, 0))
		t.mats = append(t.mats, m)
		c := tr.color
		t.insts = append(t.insts, render.ModelInstance{Model: t.model, X: pos[0], Y: pos[1], Z: pos[2],
			Color: [4]float32{c[0], c[1], c[2], 1}, Emissive: true, Blend: render.BlendAdditive})
	}
	for i := range t.insts {
		t.insts[i].Transform = &t.mats[i]
	}
	t.offsets = live
	g.Models.Draw(cam, t.insts)
}

// muzzleOffset is the held gun's muzzle relative to the camera as drawn
// this frame, turned into the world by the camera's rotation
// (GunItemRendererWrapper.cacheMuzzlePosition's muzzleRenderOffset, which
// the tracer turns by the camera rotation it stores); zero without one.
// The muzzle's camera-space x and y are scaled by
// tan(worldFov/2)/tan(modelFov/2), so the tracer starts where the hand's
// projection shows the muzzle; z stays.
func (g *game) muzzleOffset() [3]float64 {
	if !g.gunOn() {
		return [3]float64{}
	}
	v := g.heldView()
	local, _, ok := g.muzzleFrames(v)
	if !ok {
		return [3]float64{}
	}
	o := g.gunHandView(v).Mul4(local).Col(3)
	lateral := float32(math.Tan(g.Cam.FOV/2) / math.Tan(float64(g.Eng.Renderer.HandFOV)*math.Pi/360))
	o[0], o[1] = o[0]*lateral, o[1]*lateral
	w := mgl32.LookAtV(mgl32.Vec3{}, g.Cam.Forward(), mgl32.Vec3{0, 1, 0}).Inv().Mul4x1(mgl32.Vec4{o[0], o[1], o[2], 0})
	return [3]float64{float64(w[0]), float64(w[1]), float64(w[2])}
}
