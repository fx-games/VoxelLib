package mp

import (
	"sync/atomic"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/explosion"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/weapon"
)

// Block damage is the server's: shots and blasts add break progress to a
// block (0..1, PlayerControllerMP.curBlockDamageMP's scale), shown on
// every client as the cracks of a block being broken (destroy_stage,
// RenderGlobal.destroyBlockInWorldPartially), and the block breaks at 1.
// Progress a block has not gained for crackExpireTicks is forgotten, as
// the client's DestroyBlockProgress expires unrenewed cracks.
const (
	// shotHardnessScale is the bullet damage that breaks a block of
	// hardness 1: a hit adds damage / (hardness × shotHardnessScale),
	// as one tick of bare-hand digging adds 1 / (hardness × 30)
	// (Block.getPlayerRelativeBlockHardness), so a shot of damage d
	// does d ticks of digging. TACZ bullets break no blocks; this is ours.
	shotHardnessScale = 30
	// crackExpireTicks is RenderGlobal's DestroyBlockProgress expiry.
	crackExpireTicks = 400
	// crackStages are the destroy_stage textures, 0..crackStages-1.
	crackStages = 10
	// NoCrack is a BlockDamage stage that clears the cracks.
	NoCrack = -1
)

// BlockDamage is a block's cracks: stage 0..9, NoCrack for none. ID is the
// server's id for that block's damage (DestroyBlockPartially's breaker id).
type BlockDamage struct {
	ID      uint32
	X, Y, Z int
	Stage   int
}

func (d BlockDamage) encode() []byte {
	b := wbuf{msgBlockDamage}
	b.u32(d.ID)
	b.i32(int32(d.X))
	b.i32(int32(d.Y))
	b.i32(int32(d.Z))
	b.u8(uint8(d.Stage + 1))
	return b
}

func decodeBlockDamage(r *rbuf) BlockDamage {
	return BlockDamage{ID: r.u32(), X: int(r.i32()), Y: int(r.i32()), Z: int(r.i32()), Stage: int(r.u8()) - 1}
}

// damaged is one block's progress.
type damaged struct {
	id       uint32
	progress float32
	stage    int
	tick     uint32 // last gained
}

// crackStage is the cracks of progress p (Player.BreakStage: int(p*10)-1).
func crackStage(p float32) int { return min(int(p*crackStages)-1, crackStages-1) }

// damageBlock adds progress to the block at pos; at 1 it breaks. Air,
// liquids and unbreakable blocks take none.
func (s *Server) damageBlock(pos voxel.BlockPos, progress float32) {
	st := s.world.Block(pos.X, pos.Y, pos.Z)
	if st == voxel.Air || s.world.Liquid(pos.X, pos.Y, pos.Z) != character.LiquidNone || s.cfg.Reg.Info(st).Strength.Unbreakable() || progress <= 0 {
		return
	}
	d := s.damaged[pos]
	if d == nil {
		s.nextID++
		d = &damaged{id: s.nextID, stage: NoCrack}
		s.damaged[pos] = d
	}
	d.progress += progress
	d.tick = atomic.LoadUint32(&s.tick)
	if d.progress >= 1 {
		s.setBlock(BlockEdit{X: pos.X, Y: pos.Y, Z: pos.Z, State: s.cfg.Reg.StateString(voxel.Air)})
		return
	}
	if st := crackStage(d.progress); st != d.stage {
		d.stage = st
		s.broadcast(BlockDamage{ID: d.id, X: pos.X, Y: pos.Y, Z: pos.Z, Stage: st}.encode())
	}
}

// clearDamage forgets the block's progress and its cracks.
func (s *Server) clearDamage(pos voxel.BlockPos) {
	d := s.damaged[pos]
	if d == nil {
		return
	}
	delete(s.damaged, pos)
	if d.stage != NoCrack {
		s.broadcast(BlockDamage{ID: d.id, X: pos.X, Y: pos.Y, Z: pos.Z, Stage: NoCrack}.encode())
	}
}

// expireDamage forgets progress not gained for crackExpireTicks.
func (s *Server) expireDamage(tick uint32) {
	for pos, d := range s.damaged {
		if tick-d.tick > crackExpireTicks {
			s.clearDamage(pos)
		}
	}
}

// shotBlock is a bullet stopped by the block of h: damage/(hardness ×
// shotHardnessScale) progress (hardness 0 breaks at once).
func (s *Server) shotBlock(b *projectile.Bullet, h physics.Hit) {
	hard := s.cfg.Reg.Info(h.State).Strength.Hardness
	at := [3]float64{float64(h.Block.X) + 0.5, float64(h.Block.Y) + 0.5, float64(h.Block.Z) + 0.5}
	s.damageBlock(h.Block, b.Damage(at, false)/(max(hard, 0)*shotHardnessScale))
}

// blastBlocks is ProjectileExplosion's block part with destroy_block: the
// vanilla rays (explosion.Blast, strength = the radius) break every block
// they pass, and the blocks they reach but cannot pass take the share of
// their resistance the ray had left as progress. Nothing drops.
func (s *Server) blastBlocks(e *weapon.Explosion, at [3]float64) {
	if !e.DestroyBlock {
		return
	}
	b := explosion.Blast{X: at[0], Y: at[1], Z: at[2], Size: e.Radius}
	picked, cracked := b.Rays(s.rng, func(x, y, z int) (float32, bool) {
		st := s.world.Block(x, y, z)
		return s.cfg.Reg.Info(st).Strength.Resistance, st == voxel.Air
	})
	for _, p := range picked {
		pos := voxel.BlockPos{X: p[0], Y: p[1], Z: p[2]}
		if st := s.world.Block(pos.X, pos.Y, pos.Z); st != voxel.Air && !s.cfg.Reg.Info(st).Strength.Unbreakable() {
			s.setBlock(BlockEdit{X: pos.X, Y: pos.Y, Z: pos.Z, State: s.cfg.Reg.StateString(voxel.Air)})
		}
	}
	for p, f := range cracked {
		s.damageBlock(voxel.BlockPos{X: p[0], Y: p[1], Z: p[2]}, f)
	}
}
