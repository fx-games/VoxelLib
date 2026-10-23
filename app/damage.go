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

// drawDamage draws the cracks on the block the player is breaking
// (EntityRenderer.renderWorld "destroyProgress", after the selection box).
// The game sets the textures with Renderer.SetDamageTextures.
func (a *App) drawDamage() {
	p, d := a.Player, &a.damage
	stage, b := p.BreakStage(), p.Breaking
	if c := a.Config.Debug.Crack; c > 0 {
		if h, ent, ok := a.Target(); ok && ent == nil {
			stage, b = min(c, 10)-1, h.Block
		}
	}
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
	a.Eng.Renderer.DrawBlockDamage(&a.Cam, d.mesh, [3]int{o.X, o.Y, o.Z}, stage)
}
