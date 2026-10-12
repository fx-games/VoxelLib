// Package overworld is a Minecraft-like terrain generator, the default for
// the explorer demo and an example of a full worldgen.Generator.
//
// Height comes from continentalness/erosion/peaks noise run through splines
// (as Minecraft 1.18+), carved by 3D overhang noise and cheese/spaghetti
// caves sampled on a coarse 4x4x4 lattice and trilinearly interpolated. A
// surface pass adds grass/sand/snow, then ores, trees (placed from world
// coordinates so they cross chunk borders consistently) and plants.
//
// The block palette is looked up in the voxel.Registry by Minecraft names in
// New; everything else is self-contained (noise.go has Perlin + octaves).
package overworld
