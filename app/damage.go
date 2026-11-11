package app

import (
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/mesh"
	"github.com/memmaker/voxellib/voxel/render"
)

// damageOverlay is the mesh of the block being broken, rebuilt only when
// the target or its state changes (the stage only picks the texture).
type damageOverlay struct {
	pos    voxel.BlockPos
	state  voxel.StateID
	mesh   *render.BlockMesh
	built  bool
	mesher *mesh.Mesher
}

// crack is another breaker's DestroyBlockProgress.
type crack struct {
	pos   voxel.BlockPos
	stage int
	tick  int // set at (DestroyBlockProgress.createdAtCloudUpdateTick)
	d     damageOverlay
}

// DestroyBlockPartially shows breaker id's cracks at stage 0..9 on b; any
// other stage clears them (RenderGlobal.destroyBlockInWorldPartially). The
// player's own cracks come from Player.BreakStage instead.
func (a *App) DestroyBlockPartially(id int, b voxel.BlockPos, stage int) {
	if stage < 0 || stage >= 10 {
		if c := a.cracks[id]; c != nil {
			c.d.mesh.Delete()
			delete(a.cracks, id)
		}
		return
	}
	c := a.cracks[id]
	if c == nil {
		if a.cracks == nil {
			a.cracks = map[int]*crack{}
		}
		c = &crack{}
		a.cracks[id] = c
	}
	c.pos, c.stage, c.tick = b, stage, a.Tick
}

// expireCracks drops cracks not renewed for 400 ticks, checked every 20
// (RenderGlobal.updateClouds).
func (a *App) expireCracks() {
	if a.Tick%20 != 0 {
		return
	}
	for id, c := range a.cracks {
		if a.Tick-c.tick > 400 {
			c.d.mesh.Delete()
			delete(a.cracks, id)
		}
	}
}

// drawDamage draws the cracks on the block the player is breaking
// (EntityRenderer.renderWorld "destroyProgress", after the selection box)
// and the other breakers' (RenderGlobal.drawBlockDamageTexture: dropped
// past 32 blocks from the player). The game sets the textures with
// Renderer.SetDamageTextures.
func (a *App) drawDamage(cam *render.Camera, alpha float64) {
	p := a.Player
	stage, b := p.BreakStage(), p.Breaking
	if c := a.Config.Debug.Crack; c > 0 {
		if h, ent, ok := a.Target(); ok && ent == nil {
			stage, b = min(c, 10)-1, h.Block
		}
	}
	a.drawCrack(cam, &a.damage, b, stage)
	px, py, pz := p.Lerp(alpha)
	for id, c := range a.cracks {
		dx, dy, dz := float64(c.pos.X)-px, float64(c.pos.Y)-py, float64(c.pos.Z)-pz
		if dx*dx+dy*dy+dz*dz > 1024 {
			c.d.mesh.Delete()
			delete(a.cracks, id)
			continue
		}
		a.drawCrack(cam, &c.d, c.pos, c.stage)
	}
}

func (a *App) drawCrack(cam *render.Camera, d *damageOverlay, b voxel.BlockPos, stage int) {
	if stage < 0 {
		return
	}
	st := a.Eng.World.Block(b.X, b.Y, b.Z)
	if st == voxel.Air {
		return
	}
	sp := b.Section()
	if !d.built || d.pos != b || d.state != st {
		d.mesh.Delete()
		if d.mesher == nil {
			d.mesher = mesh.NewMesher()
		}
		d.mesher.Snap.Capture(a.Eng.World, sp)
		o := sp.Origin()
		m := d.mesher.BuildCell(a.Eng.World.Reg.Table(), a.Eng.Renderer.MeshConfig(), b.X-o.X, b.Y-o.Y, b.Z-o.Z)
		d.mesh = render.NewBlockMesh(m, true)
		d.pos, d.state, d.built = b, st, true
	}
	o := sp.Origin()
	a.Eng.Renderer.DrawBlockDamage(cam, d.mesh, [3]int{o.X, o.Y, o.Z}, stage)
}
