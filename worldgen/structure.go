package worldgen

import "github.com/memmaker/voxellib/voxel"

// Structure is a kind of game-defined building that a generator places.
//
// Chunks generate in any order and on several goroutines, so a structure
// is never built "once": every chunk it overlaps builds it again and keeps
// only the blocks inside itself (Region drops the rest). That is
// deterministic as long as Plan and Build depend on nothing but the seed and
// the start chunk: in particular Build must not adapt to terrain it reads
// outside the chunk (ask the generator's own height function instead).
type Structure interface {
	// Plan returns the instances whose start lies in chunk p (usually
	// none). It must be deterministic and safe for concurrent use.
	Plan(seed int64, p voxel.ChunkPos) []Instance
	// Reach is how many chunks an instance may extend from its start
	// chunk in x and z.
	Reach() int
}

// Instance is one planned structure.
type Instance interface {
	// Build writes the whole structure through r, in world coordinates.
	Build(r *Region)
}

// Region is the chunk being generated, addressed in world coordinates.
// Writes and reads outside the chunk are ignored (reads return air).
type Region struct {
	W *voxel.World
	C *voxel.Chunk
}

// Inside reports whether world column x,z belongs to the chunk.
func (r *Region) Inside(x, z int) bool { return x>>4 == r.C.Pos.X && z>>4 == r.C.Pos.Z }

// Set places a block if it lies in the chunk.
func (r *Region) Set(x, y, z int, id voxel.StateID) {
	if r.Inside(x, z) && y >= r.W.MinY && y < r.W.MaxY() {
		r.C.SetRaw(x&15, y, z&15, id)
	}
}

// Get returns the block at x,y,z if it lies in the chunk, else air.
func (r *Region) Get(x, y, z int) voxel.StateID {
	if !r.Inside(x, z) {
		return voxel.Air
	}
	return r.C.Get(x&15, y, z&15)
}

// PlaceStructures builds into ch every instance of the given structures
// that starts within its Reach of ch. Instances are built in a fixed
// order (start chunk by z then x, then argument order, then plan order),
// so overlapping structures resolve the same whichever chunk generates
// first. Call it after the terrain, then ch.RecomputeHeights.
//
// ponytail: Plan runs for (2·Reach+1)² chunks per generated chunk and
// Build for every candidate; cache plans or add instance bounds if that
// shows up in profiles.
func PlaceStructures(seed int64, w *voxel.World, ch *voxel.Chunk, structures ...Structure) {
	reach := 0
	for _, s := range structures {
		reach = max(reach, s.Reach())
	}
	r := &Region{W: w, C: ch}
	for dz := -reach; dz <= reach; dz++ {
		for dx := -reach; dx <= reach; dx++ {
			p := ch.Pos.Add(dx, dz)
			for _, s := range structures {
				if n := s.Reach(); dx < -n || dx > n || dz < -n || dz > n {
					continue
				}
				for _, in := range s.Plan(seed, p) {
					in.Build(r)
				}
			}
		}
	}
}
