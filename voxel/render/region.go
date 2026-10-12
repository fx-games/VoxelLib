package render

import (
	"sort"

	"github.com/go-gl/gl/v3.3-core/gl"
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

func (a *arena) used() int {
	u := a.cap
	for _, s := range a.free {
		u -= s.n
	}
	return u
}

// region owns one VAO/VBO shared by up to 8x4x8 sections. Sections draw
// from it with a single glMultiDrawElementsBaseVertex per layer.
type region struct {
	key      [3]int
	vao, vbo uint32
	arena    arena
	sections map[uint32]*section // by region index
	// scratch for multidraw
	counts []int32
	bases  []int32
}

func newRegion(key [3]int, ebo uint32, initial int) *region {
	r := &region{key: key, sections: map[uint32]*section{}}
	gl.GenVertexArrays(1, &r.vao)
	r.alloc(initial, ebo)
	r.arena.grow(initial)
	return r
}

func (r *region) alloc(capVerts int, ebo uint32) {
	gl.BindVertexArray(r.vao)
	gl.GenBuffers(1, &r.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, capVerts*vertexBytes, nil, gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribIPointerWithOffset(0, 4, gl.UNSIGNED_INT, vertexBytes, 0)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BindVertexArray(0)
}

// ensure guarantees n contiguous free vertices, growing the buffer (with a GPU
// side copy) when needed. Returns the offset.
func (r *region) allocate(n int, ebo uint32) int {
	if off, ok := r.arena.alloc(n); ok {
		return off
	}
	newCap := r.arena.cap * 2
	for newCap-r.arena.cap < n {
		newCap *= 2
	}
	// Read back and re-upload: glCopyBufferSubData is unimplemented in
	// Apple's GL driver. Growth is rare (doubling), so the round trip is fine.
	old, oldCap := r.vbo, r.arena.cap
	tmp := make([]byte, oldCap*vertexBytes)
	gl.BindBuffer(gl.ARRAY_BUFFER, old)
	gl.GetBufferSubData(gl.ARRAY_BUFFER, 0, len(tmp), gl.Ptr(tmp))
	r.alloc(newCap, ebo)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, len(tmp), gl.Ptr(tmp))
	gl.DeleteBuffers(1, &old)
	r.arena.grow(newCap)
	off, _ := r.arena.alloc(n)
	return off
}

func (r *region) upload(off int, data []uint32) {
	if len(data) == 0 {
		return
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	gl.BufferSubData(gl.ARRAY_BUFFER, off*vertexBytes, len(data)*4, gl.Ptr(data))
}

func (r *region) destroy() {
	gl.DeleteBuffers(1, &r.vbo)
	gl.DeleteVertexArrays(1, &r.vao)
}
