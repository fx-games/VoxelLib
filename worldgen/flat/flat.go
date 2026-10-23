// Package flat is a superflat terrain generator: horizontal layers of
// blocks, like the reference's "Superflat" preset. It is the smallest complete
// worldgen.Generator and a template for writing your own.
package flat

import "github.com/memmaker/voxellib/voxel"

// Layer is a run of identical blocks, listed bottom up.
type Layer struct {
	Block voxel.StateID
	Count int
}

// Generator stacks its layers from the bottom of the world.
type Generator struct {
	Layers []Layer
}

// New returns the reference's classic superflat: bedrock, 2 dirt, grass.
func New(reg *voxel.Registry) *Generator {
	return &Generator{Layers: []Layer{
		{reg.Default("bedrock"), 1},
		{reg.Default("dirt"), 2},
		{reg.Parse("grass_block[snowy=false]"), 1},
	}}
}

// Generate implements worldgen.Generator.
func (g *Generator) Generate(w *voxel.World, ch *voxel.Chunk) {
	y := w.MinY
	for _, l := range g.Layers {
		for i := 0; i < l.Count && y < w.MaxY(); i, y = i+1, y+1 {
			for z := 0; z < 16; z++ {
				for x := 0; x < 16; x++ {
					ch.SetRaw(x, y, z, l.Block)
				}
			}
		}
	}
	ch.RecomputeHeights(w.Reg.Table())
}

// Top is the y coordinate of the first air block above the layers.
func (g *Generator) Top(w *voxel.World) int {
	y := w.MinY
	for _, l := range g.Layers {
		y += l.Count
	}
	return y
}

// FindLand implements worldgen.Spawner: every column is the same.
func (g *Generator) FindLand(x, z int) (int, int) { return x, z }
