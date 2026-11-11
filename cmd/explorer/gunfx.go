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
	rend     *render.ParticleRenderer
	effects  map[string]*particles.Effect // by definition file
	sheets   map[string]uint8             // by texture file
	emitters []*particles.Emitter
	world    particles.System
	sprites  []particles.Particle
	last     time.Time
	dt       float32 // last frame time (capped)
}

// muzzleBone is TACZ's MUZZLE_FLASH_ORIGIN_NODE.
const muzzleBone = "muzzle_flash"

// startMuzzle starts a shot's emitters (none when the flash is scaled to 0, as TACZ).
func (g *game) startMuzzle(spec *firearm.Gun) {
	m := spec.Muzzle
	if m == nil || m.Scale <= 0 {
		return
	}
	for _, l := range []struct {
		file string
		vars map[string]float64
	}{{m.Flash, m.FlashVars()}, {m.Smoke, m.SmokeVars()}} {
		if l.file == "" {
			continue
		}
		fx, sheet := g.muzzleEffect(l.file)
		if fx != nil {
			g.fx.emitters = append(g.fx.emitters, fx.Start(l.vars, g.gunRand, sheet))
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

// drawMuzzle moves the emitters to the muzzle (model space: the hand view;
// the world through the camera), ticks them by the frame time and draws the
// world smoke; it returns the hand-space sprites to draw over the gun.
func (g *game) drawMuzzle(v *gunView, view mgl32.Mat4) []particles.Particle {
	f := &g.fx
	dt := float64(f.dt)
	f.sprites = f.sprites[:0]
	if f.rend == nil {
		return nil
	}
	if b := v.m.Bone(muzzleBone); b >= 0 && len(f.emitters) > 0 {
		// TACZ's muzzle frame is our bone frame turned 180° about Z (see gunArms).
		local := v.skin[b].Mul4(v.m.Bones[b].InvBind.Inv()).Mul4(mgl32.HomogRotate3DZ(math.Pi))
		camRot := mgl32.LookAtV(mgl32.Vec3{}, g.Cam.Forward(), mgl32.Vec3{0, 1, 0}).Inv()
		world := mgl32.Translate3D(float32(g.Cam.X), float32(g.Cam.Y), float32(g.Cam.Z)).Mul4(camRot).Mul4(view).Mul4(local)
		live := f.emitters[:0]
		for _, em := range f.emitters {
			em.Local, em.World = frameOf(local), frameOf(world)
			em.Tick(dt, &f.world)
			if !em.Done() {
				live = append(live, em)
				f.sprites = em.Sprites(f.sprites)
			}
		}
		f.emitters = live
	}
	f.world.Tick(dt, nil)
	f.rend.Draw(&g.Cam, f.world.P, 1, g.Light)
	return f.sprites
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
