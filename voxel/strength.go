package voxel

import "github.com/memmaker/voxellib/resloc"

// Strength is how hard a block is to break: the reference's
// Block.getBlockHardness (below 0 unbreakable, 0 breaks at once) and
// getExplosionResistance(null), which eats explosion rays.
type Strength struct {
	Hardness   float32
	Resistance float32
}

// Unbreakable reports a hardness below 0 (bedrock): no digging, shot or
// blast breaks the block.
func (s Strength) Unbreakable() bool { return s.Hardness < 0 }

// DefaultStrength is a block's with neither BlockDef.Strength nor a
// vanilla entry (VoxelLib and mod blocks): stone's.
var DefaultStrength = Strength{Hardness: 1.5, Resistance: 6}

// strengthOf is def's Strength: its own, else the vanilla block of its
// name (base: a VoxelMap key without its "#meta"), else DefaultStrength.
func strengthOf(def *BlockDef, base string) Strength {
	if def.Strength != nil {
		return *def.Strength
	}
	if ns, p := resloc.Split(base); ns == resloc.Default {
		if s, ok := vanillaStrength[p]; ok {
			return s
		}
	}
	return DefaultStrength
}
