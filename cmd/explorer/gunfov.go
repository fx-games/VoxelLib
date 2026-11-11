package main

// Aiming zoom as TACZ's CameraSetupEvent: the scope view narrows to the
// gun's magnification (applyScopeMagnification) and the hand lens from
// the gun's model_fov to its aiming one (applyGunModelFovModifying), both
// by easeInExpo of the aim progress and smoothed by SecondOrderDynamics.
// Unlike TACZ, the magnification applies only inside a scope's view disc
// (gunFOVState.scope, App.DrawScopeView, gunlens.go): the screen keeps baseFOV, and sights and
// guns without a scope lens are not magnified (docs/tacz.md).

import (
	"math"

	"github.com/memmaker/voxellib/voxel/render"
)

const (
	// baseFOV is the world's field of view (degrees) without a gun aimed.
	baseFOV = 70
	// SecondOrderDynamics' thread steps by fovStep seconds every
	// fovStepPeriod real seconds.
	fovStep       = 0.05
	fovStepPeriod = 0.006
)

// TACZ's dynamics: CameraSetupEvent's FOVs, FirstPersonRenderGunEvent's aiming.
var (
	fovDynamics = dynamics{freq: 0.5, damping: 1.2, response: 0.5}
	aimDynamics = dynamics{freq: 1.2, damping: 1.2, response: 0.5}
)

// dynamics are SecondOrderDynamics' constructor parameters.
type dynamics struct{ freq, damping, response float64 }

// secondOrder is TACZ's SecondOrderDynamics.
type secondOrder struct {
	dynamics
	y, yd, x, target float32
	started          bool
}

func (d *secondOrder) k() (k1, k2, k3 float32) {
	w := 2 * math.Pi * d.freq
	return float32(d.damping / (math.Pi * d.freq)), float32(1 / (w * w)), float32(d.response * d.damping / w)
}

// step advances one fovStep toward the target.
func (d *secondOrder) step() {
	k1, k2, k3 := d.k()
	xd := (d.target - d.x) / fovStep
	y := d.y + fovStep*d.yd
	d.yd += fovStep * (d.x + k3*xd - d.y - k1*d.yd) / k2
	d.x, d.y = d.target, y
}

// value is SecondOrderDynamics.get.
func (d *secondOrder) value() float32 { return d.y + fovStep*d.yd }

// start begins at v (the dynamics' x0).
func (d *secondOrder) start(p dynamics, v float32) {
	*d = secondOrder{dynamics: p, y: v, x: v, target: v, started: true}
}

// gunFOVState smooths the FOVs and the aiming progress the hand pose uses.
type gunFOVState struct {
	world, hand, aim secondOrder
	acc              float32 // real seconds not yet stepped
	scope            float64 // this frame's scope view FOV (radians); 0: none
}

// easeInExpo is TACZ's Easing.easeInExpo.
func easeInExpo(x float32) float32 {
	switch {
	case x <= 0:
		return 0
	case x >= 1:
		return 1
	}
	return float32(math.Pow(2, float64(10*x-10)))
}

// magnificationToFov is TACZ's MathUtil.magnificationToFov (degrees).
func magnificationToFov(mag, fov float32) float32 {
	return float32(2 * math.Atan(math.Tan(float64(fov)*math.Pi/360)/float64(mag)) * 180 / math.Pi)
}

// gunFOV sets the scope view's and hand's fields of view for this frame.
func (g *game) gunFOV(alpha float64) {
	g.fx.frameTime()
	g.applyRecoil(alpha)
	if g.gunOn() {
		g.poseGun(alpha)
		g.cameraView()
	}
	world, hand, aim := float32(baseFOV), float32(render.DefaultHandFOV), float32(0)
	if g.gunOn() {
		spec := g.gun.Gun
		aim = g.aimFrac(alpha)
		p := easeInExpo(aim)
		world = magnificationToFov(1+(spec.Zoom-1)*p, baseFOV)
		hand = spec.ModelFOV + (spec.AimModelFOV-spec.ModelFOV)*p
	}
	s := &g.fov
	if !s.world.started {
		s.world.start(fovDynamics, world)
		s.hand.start(fovDynamics, hand)
		s.aim.start(aimDynamics, aim)
	}
	s.world.target, s.hand.target, s.aim.target = world, hand, aim
	for s.acc += g.fx.dt; s.acc >= fovStepPeriod; s.acc -= fovStepPeriod {
		s.world.step()
		s.hand.step()
		s.aim.step()
	}
	g.Cam.FOV = baseFOV * math.Pi / 180
	s.scope = 0
	if g.opt.fullZoom {
		g.Cam.FOV = float64(s.world.value()) * math.Pi / 180
	} else if g.gunOn() {
		if v := g.gunViews[g.gun.Gun.ID]; v != nil && v.lens != nil && v.lens.scope && s.aim.value() > 0 {
			s.scope = float64(s.world.value()) * math.Pi / 180 // lensViews' disc shows it
		}
	}
	g.Eng.Renderer.HandFOV = s.hand.value()
}
