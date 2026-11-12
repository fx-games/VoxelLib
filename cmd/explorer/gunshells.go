package main

import (
	"log"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/render"
	firearm "github.com/memmaker/voxellib/weapon"
)

// TACZ's ShellRender: each shot throws a casing from every shell bone; the
// casing flies in view space from the bone's pose at its first frame.
const (
	shellBone = "shell"
	maxShells = 128
)

type shell struct {
	spec  *firearm.ShellEjection
	model shellModel
	bone  int // shell bone of the gun's model
	start mgl32.Mat4
	posed bool
	t     float32
	rnd   mgl32.Vec3
}

type shellModel struct{ handle, bones int }

type gunShells struct {
	models map[string]shellModel // by gun ID (handle -1: none)
	live   []shell
	skin   [][]mgl32.Mat4
}

// shellBones are a model's shell, shell_1, shell_2, ... bones.
func shellBones(v *gunView) []int {
	var bs []int
	for i, b := range v.m.Bones {
		if n, ok := strings.CutPrefix(b.Name, shellBone); ok && (n == "" || n[0] == '_') {
			bs = append(bs, i)
		}
	}
	return bs
}

// shellTimer is a casing the state machine throws in t seconds (on: one
// is pending).
type shellTimer struct {
	t  float32
	on bool
}

// scheduleShell runs on each gun event: the shot, the bolt, an empty
// reload's start and reload_end_empty start the casing's timer.
func (g *game) scheduleShell(e firearm.GunEvent) {
	sh := g.gun.Gun.Shell
	if sh == nil {
		return
	}
	tm, at := sh.Timing, (*float32)(nil)
	switch {
	case e.Shot && tm.OnShot(g.gun.FireMode()):
		g.ejectShells(g.gun.Gun, g.heldView())
	case e.Clip == firearm.ClipBolt:
		at = tm.Bolt
		if tm.BoltHip != nil && g.aim < firearm.AimProgress(halfAimed) {
			at = tm.BoltHip
		}
	case e.Clip == firearm.ClipReloadEmpty || e.Clip == firearm.ClipReloadIntroEmpty || e.Clip == firearm.ClipReloadEmptyClip:
		if !tm.OnShot(g.gun.FireMode()) { // the last casing is still in
			at = tm.EmptyReload
		}
	case e.Clip == firearm.ClipReloadEndEmpty:
		at = tm.EndingEmpty
	}
	if at != nil {
		g.shellAt = shellTimer{t: *at, on: true}
	}
}

// tickShellTimer throws the scheduled casing once its time is up.
func (g *game) tickShellTimer(dt float32) {
	if !g.shellAt.on {
		return
	}
	if g.shellAt.t -= dt; g.shellAt.t < 0 {
		g.shellAt.on = false
		g.ejectShells(g.gun.Gun, g.heldView())
	}
}

// ejectShells throws a casing from the held gun's shell bone.
func (g *game) ejectShells(spec *firearm.Gun, v *gunView) {
	if spec.Shell == nil {
		return
	}
	s := &g.shells
	if s.models == nil {
		s.models = map[string]shellModel{}
	}
	h, ok := s.models[spec.ID]
	if !ok {
		h.handle = -1
		m, err := g.gunPack.ShellModel(spec)
		if err == nil {
			h.bones = len(m.Bones)
			h.handle, err = g.Models.Add(m)
		}
		if err != nil {
			log.Printf("guns: %s shell: %v", spec.ID, err)
			h.handle = -1
		}
		s.models[spec.ID] = h
	}
	if h.handle < 0 {
		return
	}
	if bs := shellBones(v); len(bs) > 0 { // the scripts only popShellFrom(0)
		r := spec.Shell.Random
		rnd := mgl32.Vec3{r[0] * g.gunRand.Float32(), r[1] * g.gunRand.Float32(), r[2] * g.gunRand.Float32()}
		s.live = append(s.live, shell{spec: spec.Shell, model: h, bone: bs[0], rnd: rnd})
	}
	if n := len(s.live) - maxShells; n > 0 {
		s.live = s.live[n:]
	}
}

// shellInstances ages the casings by dt and appends them for drawing with
// view; they fly in camera space from the bone's pose at their first frame.
func (g *game) shellInstances(v *gunView, view mgl32.Mat4, dt float32, sky, blk uint8, ins []render.ModelInstance) []render.ModelInstance {
	s := &g.shells
	live := s.live[:0]
	for _, c := range s.live {
		if !c.posed {
			if c.bone >= len(v.skin) {
				continue // the gun changed before the first frame
			}
			c.start, c.posed = view.Mul4(v.skin[c.bone]).Mul4(v.m.Bones[c.bone].InvBind.Inv()), true
		} else {
			c.t += dt
		}
		if c.t < c.spec.Life {
			live = append(live, c)
		}
	}
	s.live = live
	unview := view.Inv()
	for len(s.skin) < len(live) {
		s.skin = append(s.skin, nil)
	}
	for i, c := range live {
		e := c.spec
		t := c.t
		var p mgl32.Vec3
		for k := range 3 {
			p[k] = (e.Velocity[k]+c.rnd[k])*t + 0.5*e.Accel[k]*t*t
		}
		deg := func(k int) float32 { return mgl32.DegToRad(e.Angular[k] * t) }
		// TACZ's T(-x,-y,z)·Rx(-a)·Ry(-b)·Rz(c) in its model frame is this in ours (Rz180 between them).
		m := unview.Mul4(c.start).Mul4(mgl32.Translate3D(p[0], p[1], p[2])).Mul4(mgl32.HomogRotate3DX(deg(0))).
			Mul4(mgl32.HomogRotate3DY(deg(1))).Mul4(mgl32.HomogRotate3DZ(deg(2)))
		sk := s.skin[i][:0]
		for range c.model.bones {
			sk = append(sk, m)
		}
		s.skin[i] = sk
		ins = append(ins, render.ModelInstance{Model: c.model.handle, Skin: sk, Sky: sky, Block: blk})
	}
	return ins
}
