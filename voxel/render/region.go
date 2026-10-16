package render

import (
	"sort"
	"unsafe"

	"github.com/memmaker/voxellib/platform/gl"
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

// region is one fixed-size VAO/VBO page shared by sections of the same 8x4x8
// region. Sections draw from it with one glMultiDrawElementsBaseVertex per
// layer. A full region gets another page rather than growing: growing needs a
// GPU read-back on macOS (no glCopyBufferSubData), which stalls the frame.
type region struct {
	key      [3]int
	vao, vbo uint32
	arena    arena
	sections map[uint32]*section // by region index
	dead     bool
	// scratch for multidraw
	counts []int32
	bases  []int32
}

func newRegion(key [3]int, ebo uint32, initial int) *region {
	r := &region{key: key, sections: map[uint32]*section{}}
	gl.GenVertexArrays(1, &r.vao)
	gl.BindVertexArray(r.vao)
	gl.GenBuffers(1, &r.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, initial*vertexBytes, nil, gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribIPointerWithOffset(0, 4, gl.UNSIGNED_INT, vertexBytes, 0)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BindVertexArray(0)
	r.arena.grow(initial)
	return r
}

// upload writes vertices into a range no in-flight draw reads: freshly
// allocated spans only, since freed spans are retired for a few frames before
// reuse. That lets it map unsynchronized; glBufferSubData into a buffer the
// GPU is still drawing from makes Apple's driver stall the next draw call.
func (r *region) upload(off int, data []uint32) {
	if len(data) == 0 {
		return
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	ptr := gl.MapBufferRange(gl.ARRAY_BUFFER, off*vertexBytes, len(data)*4,
		gl.MAP_WRITE_BIT|gl.MAP_UNSYNCHRONIZED_BIT|gl.MAP_INVALIDATE_RANGE_BIT)
	if ptr == nil {
		gl.BufferSubData(gl.ARRAY_BUFFER, off*vertexBytes, len(data)*4, gl.Ptr(data))
		return
	}
	copy(unsafe.Slice((*uint32)(ptr), len(data)), data)
	gl.UnmapBuffer(gl.ARRAY_BUFFER)
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
	gl.DeleteBuffers(1, &r.vbo)
	gl.DeleteVertexArrays(1, &r.vao)
}
