package engine

import "github.com/memmaker/voxellib/voxel"

// SetDynamicLights replaces the dynamic light sources (voxel.DynLight) and
// queues the sections whose light changed for meshing. Sources in chunks
// that are not loaded are dropped (the client has no entities there).
// Main thread; it takes the world read lock.
func (e *Engine) SetDynamicLights(srcs []voxel.DynLight) {
	live := srcs[:0:0]
	for _, s := range srcs {
		if e.loaded[voxel.ChunkPos{X: s.X >> 4, Z: s.Z >> 4}] {
			live = append(live, s)
		}
	}
	ms, n := e.World.MinSection(), e.World.SectionCount()
	for _, p := range e.World.SetDynLights(live) {
		if p.Y >= ms && p.Y < ms+n {
			e.dirty[p] = true
		}
	}
}
