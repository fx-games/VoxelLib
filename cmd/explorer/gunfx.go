package main

import (
	"log"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/voxel/render"
	firearm "github.com/memmaker/voxellib/weapon"
)

// gunFX plays TACZ's "renew" muzzle flash and smoke (MuzzleParticleManager):
// one flash and one smoke emitter per shot at the muzzle_flash bone, the
// flash in hand space, the smoke sent to the world. Emitters tick per frame:
// the flash lives 0.02 s, less than a tick.
type gunFX struct {
	rend          *render.ParticleRenderer
	effects       map[string]*particles.Effect // by definition file
	sheets        map[string]uint8             // by texture file
	emitters      []*particles.Emitter
	remote        []*particles.Emitter // other players' shots, placed in the world
	world         particles.System
	sprites       []particles.Particle // the held gun's, in hand space (fxFrame)
	remoteSprites []particles.Particle // other players' flashes, in the world
	last          time.Time
	dt            float32 // last frame time (capped)
}

// muzzleBone is TACZ's MUZZLE_FLASH_ORIGIN_NODE.
const (
	muzzleBone          = "muzzle_flash"
	thirdPersonHandBone = "thirdperson_hand" // GunModelConstant THIRD_PERSON_HAND_ORIGIN_NODE
)

// startMuzzle starts a shot's emitters (none when the flash is scaled to 0,
// as TACZ; smoke only with a silencer, Gun.FlashHidden): at the held gun's
// muzzle, or for another player's shot (at not nil) fixed at that world frame.
func (g *game) startMuzzle(spec *firearm.Gun, at *particles.Frame) {
	m := spec.Muzzle
	if m == nil || m.Scale <= 0 {
		return
	}
	flash := m.Flash
	if spec.FlashHidden() {
		flash = ""
	}
	for _, l := range []struct {
		file string
		vars map[string]float64
	}{{flash, m.FlashVars()}, {m.Smoke, m.SmokeVars()}} {
		if l.file == "" {
			continue
		}
		fx, sheet := g.muzzleEffect(l.file)
		if fx == nil {
			continue
		}
		em := fx.Start(l.vars, g.gunRand, sheet)
		if at != nil {
			em.Local, em.World = *at, *at
			g.fx.remote = append(g.fx.remote, em)
		} else {
			g.fx.emitters = append(g.fx.emitters, em)
		}
	}
}

// muzzleEffect loads a definition and its texture sheet once (nil on error).
func (g *game) muzzleEffect(file string) (*particles.Effect, uint8) {
	f := &g.fx
	if f.rend == nil {
		var err error
		if f.rend, err = render.NewParticleRenderer(g.Eng.Renderer); err != nil {
			log.Printf("guns: particles: %v", err)
			return nil, 0
		}
		f.effects, f.sheets = map[string]*particles.Effect{}, map[string]uint8{}
	}
	fx, ok := f.effects[file]
	if !ok {
		var err error
		if fx, err = particles.LoadEffect(g.gunPack.FS, file); err != nil {
			log.Printf("guns: %v", err)
		}
		f.effects[file] = fx
	}
	if fx == nil {
		return nil, 0
	}
	sheet, ok := f.sheets[fx.Texture]
	if !ok {
		img, err := g.gunPack.Icon(fx.Texture)
		if err != nil {
			log.Printf("guns: %v", err)
		} else {
			sheet = uint8(len(f.sheets) + 1)
			f.rend.SetSheet(int(sheet), img)
		}
		f.sheets[fx.Texture] = sheet
	}
	return fx, sheet
}

// frameTime measures the frame (fx.dt, capped), once per frame before
// the guns draw.
func (f *gunFX) frameTime() {
	now := time.Now()
	f.dt = float32(math.Min(now.Sub(f.last).Seconds(), maxFrameTime))
	f.last = now
}

// maxFrameTime caps a frame's time step (seconds), e.g. after a stall.
const maxFrameTime = 0.1

// fxFrame advances the gun effects by the frame time, once per frame
// before the views draw them (drawFX, drawGunHand): the held gun's
// emitters follow its muzzle (model space: the hand view; the world
// through the camera), other players' shots stay where they were fired.
func (g *game) fxFrame() {
	f := &g.fx
	if f.rend == nil {
		return
	}
	dt := float64(f.dt)
	f.sprites, f.remoteSprites = f.sprites[:0], f.remoteSprites[:0]
	if g.gunOn() {
		v := g.heldView()
		if local, world, ok := g.muzzleFrames(v); ok && len(f.emitters) > 0 {
			out := mgl32.Translate3D(0, 0, -g.gun.Gun.FlashOffset) // along the muzzle (TACZ's -Z)
			local, world = local.Mul4(out), world.Mul4(out)
			f.emitters = tickEmitters(f.emitters, dt, frameOf(local), frameOf(world), &f.world, &f.sprites)
		}
	}
	live := f.remote[:0]
	for _, em := range f.remote {
		em.Tick(dt, &f.world)
		if !em.Done() {
			live = append(live, em)
			f.remoteSprites = em.Sprites(f.remoteSprites)
		}
	}
	f.remote = live
	f.world.Tick(dt, nil)
}

// muzzleFrames is the held gun's muzzle frame in its hand view (local)
// and in the world as drawn this frame (ok false: no muzzle bone).
func (g *game) muzzleFrames(v *gunView) (local, world mgl32.Mat4, ok bool) {
	b := v.m.Bone(muzzleBone)
	if b < 0 || b >= len(v.skin) {
		return local, world, false
	}
	// TACZ's muzzle frame is our bone frame turned 180° about Z (see gunArms).
	local = v.skin[b].Mul4(v.m.Bones[b].InvBind.Inv()).Mul4(mgl32.HomogRotate3DZ(math.Pi))
	camRot := mgl32.LookAtV(mgl32.Vec3{}, g.Cam.Forward(), mgl32.Vec3{0, 1, 0}).Inv()
	world = mgl32.Translate3D(float32(g.Cam.X), float32(g.Cam.Y), float32(g.Cam.Z)).Mul4(camRot).Mul4(g.gunHandView(v)).Mul4(local)
	return local, world, true
}

// tickEmitters places the held gun's emitters at the muzzle, ticks them
// and keeps the live ones, appending their hand-space sprites.
func tickEmitters(ems []*particles.Emitter, dt float64, local, world particles.Frame, sys *particles.System, sprites *[]particles.Particle) []*particles.Emitter {
	live := ems[:0]
	for _, em := range ems {
		em.Local, em.World = local, world
		em.Tick(dt, sys)
		if !em.Done() {
			live = append(live, em)
			*sprites = em.Sprites(*sprites)
		}
	}
	return live
}

// drawFX draws the world's gun effects for cam: other players' muzzle
// flashes and the smoke and sparks shots left (fxFrame).
func (g *game) drawFX(cam *render.Camera) {
	f := &g.fx
	if f.rend == nil {
		return
	}
	f.rend.Draw(cam, f.remoteSprites, 1, g.Light)
	f.rend.Draw(cam, f.world.P, 1, g.Light)
}

func frameOf(m mgl32.Mat4) particles.Frame {
	var f particles.Frame
	for i := range 3 {
		c := m.Col(i).Vec3()
		f.Basis[i] = [3]float64{float64(c[0]), float64(c[1]), float64(c[2])}
	}
	o := m.Col(3)
	f.Origin = [3]float64{float64(o[0]), float64(o[1]), float64(o[2])}
	return f
}
