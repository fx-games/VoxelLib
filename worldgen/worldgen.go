// Package worldgen holds terrain generators for voxellib, kept apart from
// the engine so the terrain algorithm can be swapped freely.
//
// A generator is anything with a Generate method; the engine
// (voxel/engine.Config.Generator) only calls that. Subpackages:
//
//	worldgen/overworld  Minecraft-like terrain: continents, mountains, caves, ores, trees
//	worldgen/flat       superflat layers (the minimal example to copy from)
//
// Writing your own: implement Generator, and optionally Spawner so a game can
// pick a starting point. Generate is called concurrently from several worker
// goroutines, for chunks in any order, and must be deterministic: it may only
// write blocks into the chunk it is given (decorations that cross chunk
// borders, like trees, must be derived from world coordinates so neighbours
// agree), and may read w only for its height range (w.MinY, w.MaxY()). Light
// is computed by the engine afterwards.
package worldgen

import "github.com/memmaker/voxellib/voxel"

// Generator fills a freshly allocated chunk. It matches
// voxel/engine.Generator.
type Generator interface {
	Generate(w *voxel.World, ch *voxel.Chunk)
}

// Spawner is optionally implemented by generators that can suggest a good
// spawn column near x,z.
type Spawner interface {
	FindLand(x, z int) (int, int)
}
