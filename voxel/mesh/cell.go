package mesh

import "github.com/memmaker/voxellib/voxel"

// BuildCell meshes only the block at section-local x,y,z (0..15) of the
// captured snapshot, with its real neighbours for face culling, connections
// and light: a single block drawn on its own (Minecraft's
// RenderBlocks.renderBlockByRenderType), e.g. the block breaking overlay.
func (m *Mesher) BuildCell(table []voxel.StateInfo, cfg *Config, x, y, z int) *Mesh {
	m.only = idx(x, y, z) + 1
	defer func() { m.only = 0 }()
	return m.Build(table, cfg)
}

// Block meshes info alone, as the only block of an empty, fully lit
// section, at section-local 0,0,0 (vertices in 0..1 plus the parts that
// poke out): a block drawn without a world, e.g. as an item. air is the
// table's state 0 (StateInfo of air).
func Block(info, air voxel.StateInfo, cfg *Config) *Mesh {
	m := NewMesher()
	for i := range m.Snap.Light {
		m.Snap.Light[i] = 15<<4 | 15
	}
	m.Snap.States[idx(0, 0, 0)] = 1
	return m.BuildCell([]voxel.StateInfo{air, info}, cfg, 0, 0, 0)
}
