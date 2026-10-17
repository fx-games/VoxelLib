package render

import (
	"sort"

	"github.com/memmaker/voxellib/voxel/mesh"
)

const vertexBytes = mesh.VertexWords * 4

// span is a free run of vertices in a region buffer.
type span struct{ off, n int }

// arena is a first-fit allocator over a vertex buffer, with coalescing.
type arena struct {
	free []span // sorted by off
	cap  int
}

func (a *arena) alloc(n int) (int, bool) {
	for i, s := range a.free {
		if s.n >= n {
			off := s.off
			if s.n == n {
				a.free = append(a.free[:i], a.free[i+1:]...)
			} else {
				a.free[i] = span{s.off + n, s.n - n}
			}
			return off, true
		}
	}
	return 0, false
}

func (a *arena) release(off, n int) {
	if n == 0 {
		return
	}
	i := sort.Search(len(a.free), func(i int) bool { return a.free[i].off > off })
	a.free = append(a.free, span{})
	copy(a.free[i+1:], a.free[i:])
	a.free[i] = span{off, n}
	// merge with next, then previous
	if i+1 < len(a.free) && a.free[i].off+a.free[i].n == a.free[i+1].off {
		a.free[i].n += a.free[i+1].n
		a.free = append(a.free[:i+1], a.free[i+2:]...)
	}
	if i > 0 && a.free[i-1].off+a.free[i-1].n == a.free[i].off {
		a.free[i-1].n += a.free[i].n
		a.free = append(a.free[:i], a.free[i+1:]...)
	}
}

func (a *arena) grow(newCap int) {
	a.release(a.cap, newCap-a.cap)
	a.cap = newCap
}

// retired is a freed span the GPU may still be reading; it returns to the
// arena after retireFrames frames.
type retired struct {
	reg    *region
	off, n int
	frame  uint64
}

const retireFrames = 4

// pageVerts is the vertex capacity of a region page (2 MiB).
const pageVerts = 1 << 17

func (a *arena) used() int {
	u := a.cap
	for _, s := range a.free {
		u -= s.n
	}
	return u
}

// region is one fixed-size vertex buffer page shared by sections of the same
// 8x4x8 region. Sections draw from it with one multidraw per layer. A full
// region gets another page rather than growing: growing needs a GPU
// read-back on macOS GL (no glCopyBufferSubData), which stalls the frame.
type region struct {
	key      [3]int
	gpu      gpuRegion
	arena    arena
	sections map[uint32]*section // by region index
	dead     bool
}

func (r *Renderer) newRegion(key [3]int, initial int) *region {
	reg := &region{key: key, sections: map[uint32]*section{}}
	reg.gpu = r.newGPURegion(initial)
	reg.arena.grow(initial)
	return reg
}

// allocPair allocates two spans in this page, or neither.
func (r *region) allocPair(a, b int) (int, int, bool) {
	offA, ok := r.arena.alloc(a)
	if !ok {
		return 0, 0, false
	}
	if b == 0 {
		return offA, 0, true
	}
	offB, ok := r.arena.alloc(b)
	if !ok {
		r.arena.release(offA, a)
		return 0, 0, false
	}
	return offA, offB, true
}

func (r *region) destroy() {
	r.dead = true
	r.gpu.destroy()
}
